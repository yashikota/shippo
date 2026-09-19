package daemon

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

// Only the external CLI is replaced: argument construction, exit handling and
// state files all use the production implementation. No Tailnet is contacted.
func fakeTailscale(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	script := `#!/bin/sh
if [ "$*" = 'status --self --json' ]; then
  printf '%s\n' '{"Self":{"DNSName":"test.example.ts.net."}}'
  exit 0
fi
printf '%s\n' "$*" >> "$FAKE_TAILSCALE_DIR/calls"
if [ -f "$FAKE_TAILSCALE_DIR/fail" ]; then
  echo 'Access denied: serve config denied' >&2
  exit 1
fi
`
	if err := os.WriteFile(filepath.Join(dir, "tailscale"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_TAILSCALE_DIR", dir)
	t.Setenv("TMPDIR", dir)
	machineName, machineNameOnce = "", sync.Once{}
	activeURLs = nil
	t.Cleanup(func() {
		machineName, machineNameOnce = "", sync.Once{}
		activeURLs = nil
	})
	return dir
}

func tailCalls(t *testing.T, dir string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "calls"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(string(data)), "\n")
}

func setTailFailure(t *testing.T, dir string, fail bool) {
	t.Helper()
	path := filepath.Join(dir, "fail")
	if fail {
		if err := os.WriteFile(path, nil, 0600); err != nil {
			t.Fatal(err)
		}
	} else if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
}

func TestServeCommandAndState(t *testing.T) {
	dir := fakeTailscale(t)
	if err := serve(3000); err != nil {
		t.Fatal(err)
	}
	want := []string{"https://test.example.ts.net:3000"}
	if got, err := URLs(); err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("URLs() = %v, %v; want %v", got, err, want)
	}
	if got, err := LastURL(); err != nil || got != want[0] {
		t.Fatalf("LastURL() = %q, %v", got, err)
	}
	setTailFailure(t, dir, true)
	if err := unserve(3000); err == nil {
		t.Fatal("unserve must report permission failure")
	}
	if got, err := URLs(); err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("failed removal lost state: %v, %v", got, err)
	}
	setTailFailure(t, dir, false)
	if err := unserve(3000); err != nil {
		t.Fatal(err)
	}
	if _, err := URLs(); err == nil {
		t.Fatal("URLs retained after removal")
	}
	if _, err := LastURL(); err == nil {
		t.Fatal("last URL retained after removal")
	}
	wantCalls := []string{
		"serve --bg --https=3000 http://localhost:3000",
		"serve --https=3000 off",
		"serve --https=3000 off",
	}
	if got := tailCalls(t, dir); !reflect.DeepEqual(got, wantCalls) {
		t.Fatalf("commands = %v, want %v", got, wantCalls)
	}
}

func TestServeFailureDoesNotPublishURL(t *testing.T) {
	dir := fakeTailscale(t)
	setTailFailure(t, dir, true)
	if err := serve(3000); err == nil || !strings.Contains(err.Error(), "Access denied") {
		t.Fatalf("serve error = %v", err)
	}
	if _, err := URLs(); err == nil {
		t.Fatal("failed serve recorded a URL")
	}
	if _, err := LastURL(); err == nil {
		t.Fatal("failed serve recorded last URL")
	}
}
