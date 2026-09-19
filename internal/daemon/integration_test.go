//go:build integration && linux

package daemon

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// These tests deliberately fail if BPF cannot be loaded or attached. Run them
// with task test:integration in an isolated network namespace, as CI does.
func TestIntegrationMonitor(t *testing.T) {
	kernel, err := exec.Command("uname", "-srvm").CombinedOutput()
	if err != nil {
		t.Fatalf("read kernel version: %v", err)
	}
	t.Logf("test kernel: %s", kernel)
	m, err := NewMonitor()
	if err != nil {
		t.Fatalf("load and attach actual BPF program: %+v", err)
	}
	go m.Run()
	t.Cleanup(func() {
		m.Close()
		timer := time.NewTimer(5 * time.Second)
		defer timer.Stop()
		for {
			select {
			case _, ok := <-m.Events():
				if !ok {
					return
				}
			case <-timer.C:
				t.Error("monitor did not stop")
				return
			}
		}
	})
	for _, tc := range []struct {
		network, host string
		family        uint8
		local         bool
	}{
		{"tcp4", "127.0.0.1", 2, true},
		{"tcp6", "::1", 10, true},
		{"tcp4", "0.0.0.0", 2, false},
	} {
		t.Run(tc.host, func(t *testing.T) {
			ln, err := net.Listen(tc.network, net.JoinHostPort(tc.host, "0"))
			if err != nil {
				t.Fatal(err)
			}
			defer ln.Close()
			port := uint16(ln.Addr().(*net.TCPAddr).Port)
			check := func(action uint8) {
				t.Helper()
				timer := time.NewTimer(5 * time.Second)
				defer timer.Stop()
				for {
					select {
					case ev, ok := <-m.Events():
						if !ok {
							t.Fatal("event stream closed")
						}
						if ev.Port != port || ev.Action != action || ev.Family != tc.family {
							continue
						}
						if ev.AddrString() != tc.host || ev.IsLocalhost() != tc.local {
							t.Fatalf("decoded event: %+v address=%s localhost=%v", ev, ev.AddrString(), ev.IsLocalhost())
						}
						return
					case <-timer.C:
						t.Fatalf("no BPF event for %s:%d action=%d", tc.host, port, action)
					}
				}
			}
			check(ActionListenStart)
			if err := ln.Close(); err != nil {
				t.Fatal(err)
			}
			check(ActionListenStop)
		})
	}
}

func TestIntegrationMonitorCloseWithBackpressure(t *testing.T) {
	m, err := NewMonitor()
	if err != nil {
		t.Fatalf("load BPF: %+v", err)
	}
	defer m.Close()
	m.eventCh = make(chan Event) // nobody receives: Run must still be stoppable
	done := make(chan struct{})
	go func() { m.Run(); close(done) }()
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	m.Close()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("monitor blocked on event delivery after Close")
	}
}

func TestIntegrationDaemon(t *testing.T) {
	bin := os.Getenv("SHIPPO_TEST_BINARY")
	if bin == "" {
		t.Fatal("SHIPPO_TEST_BINARY must name the built daemon; use task test:integration")
	}
	dir := fakeTailscale(t)
	t.Setenv("SHIPPO_PORTS", "")
	listen := func(address string) net.Listener {
		t.Helper()
		ln, err := net.Listen("tcp4", address)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { ln.Close() })
		return ln
	}
	ln := listen("127.0.0.1:0")
	address := ln.Addr().String()
	port := ln.Addr().(*net.TCPAddr).Port
	denied := listen("127.0.0.1:0")
	deniedPort := denied.Addr().(*net.TCPAddr).Port
	config := filepath.Join(dir, "config.json")
	writeConfig := func(contents string) {
		t.Helper()
		// Editors commonly replace files using an atomic rename.
		if err := os.WriteFile(config+".new", []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(config+".new", config); err != nil {
			t.Fatal(err)
		}
	}
	allow := fmt.Sprintf(`{"ports":["%d"]}`, port)
	writeConfig(allow)
	logPath := filepath.Join(dir, "daemon.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	defer logFile.Close()
	cmd := exec.Command(bin, "--config", config, "--log-file", "", "daemon")
	cmd.Stdout, cmd.Stderr = logFile, logFile
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	stopped := false
	t.Cleanup(func() {
		if !stopped {
			_ = cmd.Process.Kill()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Error("daemon did not exit")
			}
		}
		if t.Failed() {
			data, _ := os.ReadFile(logPath)
			t.Logf("daemon log:\n%s", data)
		}
	})
	await := func(description string, predicate func() bool) {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			if predicate() {
				return
			}
			select {
			case err := <-done:
				stopped = true
				t.Fatalf("daemon exited while waiting for %s: %v", description, err)
			case <-time.After(20 * time.Millisecond):
			}
		}
		t.Fatalf("timeout waiting for %s", description)
	}
	await("config watcher", func() bool {
		data, _ := os.ReadFile(logPath)
		return strings.Contains(string(data), "watching config:")
	})
	serveCall := fmt.Sprintf("serve --bg --https=%d http://localhost:%d", port, port)
	offCall := fmt.Sprintf("serve --https=%d off", port)
	count := func(want string) int {
		n := 0
		for _, line := range tailCalls(t, dir) {
			if line == want {
				n++
			}
		}
		return n
	}
	await("initial sync", func() bool { return count(serveCall) == 1 })
	if err := ln.Close(); err != nil {
		t.Fatal(err)
	}
	await("LISTEN stop", func() bool { return count(offCall) == 1 })
	listen(address)
	await("LISTEN start", func() bool { return count(serveCall) == 2 })
	writeConfig(`{"ports":[]}`)
	await("empty allowlist removal", func() bool { return count(offCall) == 2 })
	writeConfig(allow)
	await("config addition", func() bool { return count(serveCall) == 3 })
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		stopped = true
		if err != nil {
			t.Fatalf("daemon shutdown: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("daemon shutdown timed out")
	}
	if count(offCall) != 3 {
		t.Fatalf("shutdown did not remove publication: %v", tailCalls(t, dir))
	}
	for _, call := range tailCalls(t, dir) {
		if strings.Contains(call, fmt.Sprintf("--https=%d", deniedPort)) {
			t.Fatalf("denied port published: %s", call)
		}
	}
	if _, err := URLs(); err == nil {
		t.Fatal("active URLs remained after shutdown")
	}
}
