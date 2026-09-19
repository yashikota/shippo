package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yashikota/shippo/internal/daemon"
)

func TestInitCommandHonorsConfigFlagWithoutPrivileges(t *testing.T) {
	old := daemon.ConfigPath
	t.Cleanup(func() { daemon.ConfigPath = old })
	t.Setenv("SHIPPO_PORTS", "")
	path := filepath.Join(t.TempDir(), "config.json")
	cmd := newCommand()
	var output bytes.Buffer
	cmd.Writer = &output
	// Cancel regardless of whether /proc contains candidate ports.
	cmd.Reader = strings.NewReader("\n\nn\n")
	if err := cmd.Run(context.Background(), []string{"shippo", "--config", path, "init"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Config: "+path) {
		t.Fatalf("config flag ignored: %s", &output)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("cancelled init created config: %v", err)
	}
}
