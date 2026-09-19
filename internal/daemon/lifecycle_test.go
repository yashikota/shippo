package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
)

func testReconciler(t *testing.T) (*Reconciler, string) {
	t.Helper()
	dir := fakeTailscale(t)
	matchers, raw := parsePortSpecs("3000")
	r := NewReconciler(&Config{matchers: matchers, raw: raw})
	r.currentPorts = func() map[int]bool { return map[int]bool{3000: true, 4000: true} }
	return r, dir
}

func listenEvent(port uint16, action uint8) Event {
	return Event{Port: port, Action: action, Family: 2, Addr: [4]uint32{0x0100007f}}
}

func TestReconcilerLifecycle(t *testing.T) {
	r, dir := testReconciler(t)
	r.InitialSync()
	r.HandleEvent(listenEvent(3000, ActionListenStart))                   // duplicate
	r.HandleEvent(listenEvent(4000, ActionListenStart))                   // denied
	r.HandleEvent(Event{Port: 3000, Action: ActionListenStop, Family: 2}) // wildcard
	r.HandleEvent(listenEvent(3000, ActionListenStop))                    // another listener remains
	if got := len(tailCalls(t, dir)); got != 1 {
		t.Fatalf("got %d commands before last listener closes", got)
	}
	r.currentPorts = func() map[int]bool { return nil }
	r.HandleEvent(listenEvent(3000, ActionListenStop))
	r.HandleEvent(listenEvent(3000, ActionListenStop)) // duplicate
	if len(r.served) != 0 {
		t.Fatalf("remaining ports: %v", r.served)
	}
	want := []string{"serve --bg --https=3000 http://localhost:3000", "serve --https=3000 off"}
	if got := tailCalls(t, dir); !reflect.DeepEqual(got, want) {
		t.Fatalf("commands = %v, want %v", got, want)
	}
}

func TestReconcilerRetriesFailedOperations(t *testing.T) {
	r, dir := testReconciler(t)
	setTailFailure(t, dir, true)
	r.InitialSync()
	if r.served[3000] {
		t.Fatal("failed serve marked successful")
	}
	setTailFailure(t, dir, false)
	r.Reconcile()
	if !r.served[3000] {
		t.Fatal("serve was not retried")
	}
	r.config.SetMatchers(nil, nil)
	setTailFailure(t, dir, true)
	r.Reconcile()
	if !r.served[3000] {
		t.Fatal("failed removal marked successful")
	}
	setTailFailure(t, dir, false)
	r.Reconcile()
	if r.served[3000] {
		t.Fatal("removal was not retried")
	}
	if len(tailCalls(t, dir)) != 4 {
		t.Fatalf("commands = %v", tailCalls(t, dir))
	}
}

func TestReconcilerConfigAndShutdown(t *testing.T) {
	r, dir := testReconciler(t)
	r.InitialSync()
	matchers, raw := parsePortSpecs("4000")
	r.config.SetMatchers(matchers, raw)
	r.Reconcile()
	if !reflect.DeepEqual(r.served, map[int]bool{4000: true}) {
		t.Fatalf("served = %v", r.served)
	}
	r.Shutdown()
	r.Shutdown()
	if len(r.served) != 0 {
		t.Fatalf("shutdown left %v", r.served)
	}
	if len(tailCalls(t, dir)) != 4 {
		t.Fatalf("commands = %v", tailCalls(t, dir))
	}
}

func TestReconcilerConcurrentConfigAndEvents(t *testing.T) {
	r, _ := testReconciler(t)
	var workers sync.WaitGroup
	for i := 0; i < 4; i++ {
		workers.Go(func() {
			for j := 0; j < 20; j++ {
				r.config.SetMatchers([]portMatcher{exactMatcher{3000}}, []string{"3000"})
				r.Reconcile()
				r.HandleEvent(listenEvent(3000, ActionListenStart))
				r.Shutdown()
			}
		})
	}
	workers.Wait()
	r.Shutdown()
}

func TestReadProcNetFiltersListeners(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tcp")
	data := "sl local_address rem_address st\n"
	for i, row := range []string{
		"0100007F:0BB8 00000000:0000 0A",                         // localhost:3000
		"0100007F:0FA0 00000000:0000 01",                         // established
		"00000000:1388 00000000:0000 0A",                         // wildcard
		"0101A8C0:1770 00000000:0000 0A",                         // LAN
		"00000000000000000000000001000000:1B58 00000000:0000 0A", // ::1:7000
		"broken", "0100007F:XXXX 00000000:0000 0A",
	} {
		data += fmt.Sprintf("%d: %s\n", i, row)
	}
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	if got := readProcNet(path); !reflect.DeepEqual(got, map[int]bool{3000: true, 7000: true}) {
		t.Fatalf("ports = %v", got)
	}
}

func TestReadConfigDistinguishesEmptyAndMalformed(t *testing.T) {
	old := ConfigPath
	ConfigPath = filepath.Join(t.TempDir(), "config.json")
	t.Cleanup(func() { ConfigPath = old })
	for _, data := range []string{`{"ports":[]}`, `{"ports":["3000"]}`} {
		if err := os.WriteFile(ConfigPath, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		if _, _, err := readConfigFile(); err != nil {
			t.Fatalf("valid config: %v", err)
		}
	}
	if err := os.WriteFile(ConfigPath, []byte(`{"ports":`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := readConfigFile(); err == nil {
		t.Fatal("partial write accepted")
	}
}
