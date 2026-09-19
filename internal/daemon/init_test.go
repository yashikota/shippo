package daemon

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func initTestPath(t *testing.T) string {
	t.Helper()
	old := ConfigPath
	ConfigPath = filepath.Join(t.TempDir(), "config", "config.json")
	t.Cleanup(func() { ConfigPath = old })
	t.Setenv("SHIPPO_PORTS", "")
	t.Setenv("SUDO_USER", "")
	return ConfigPath
}

func TestInitSelections(t *testing.T) {
	for _, tc := range []struct {
		name  string
		ports []int
		input string
		want  []string
	}{
		{"candidates and manual ranges", []int{3000, 5173}, "2,2\n8000 - 8999, 5173\ny\n", []string{"5173", "8000-8999"}},
		{"no running servers", nil, "4000,9000-9010\nyes\n", []string{"4000", "9000-9010"}},
		{"no default selections", []int{3000}, "\n\ny\n", []string{}},
		{"empty config", nil, "\ny\n", []string{}},
		{"explicit wildcard", nil, "*\ny\n", []string{"*"}},
		{"wildcard from candidates", []int{3000, 5173}, "*\ny\n", []string{"*"}},
		{"wildcard replaces individual selections", []int{3000, 5173}, "1\n*,4000\ny\n", []string{"*"}},
		{"invalid candidate retried", []int{3000, 5173}, "3\n1,2\n\ny\n", []string{"3000", "5173"}},
		{"invalid manual input retried", nil, "4000,nope\n0\n65536\n100-50\n4000\ny\n", []string{"4000"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := initTestPath(t)
			var output bytes.Buffer
			if err := initConfig(strings.NewReader(tc.input), &output, tc.ports); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read saved config: %v; output=%s", err, &output)
			}
			var got configFile
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got.Ports, tc.want) {
				t.Fatalf("ports = %v, want %v", got.Ports, tc.want)
			}
			if strings.Contains(tc.input, "*") && !strings.Contains(output.String(), "ALL localhost ports") {
				t.Fatal("wildcard not identified in confirmation")
			}
		})
	}
}

func TestInitCancellationPreservesExistingConfig(t *testing.T) {
	for _, input := range []string{"", "4000\n", "4000\nn\n", "4000\n\n", "4000,invalid\n"} {
		t.Run(input, func(t *testing.T) {
			path := initTestPath(t)
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				t.Fatal(err)
			}
			original := []byte(`{"ports":["9000"]}`)
			if err := os.WriteFile(path, original, 0600); err != nil {
				t.Fatal(err)
			}
			var output bytes.Buffer
			_ = initConfig(strings.NewReader(input), &output, nil)
			got, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(got, original) {
				t.Fatalf("existing config changed: %s, %v", got, err)
			}
			if !strings.Contains(output.String(), "Existing config:") {
				t.Fatal("existing config not disclosed")
			}
		})
	}
}

func TestInitRequiresExplicitSave(t *testing.T) {
	path := initTestPath(t)
	if err := initConfig(strings.NewReader("4000\n\n"), &bytes.Buffer{}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("created config without confirmation: %v", err)
	}
	if err := initConfig(strings.NewReader("4000\n"), &bytes.Buffer{}, nil); err == nil {
		t.Fatal("EOF before confirmation must fail")
	}
}

func TestInitRejectsEnvironmentOverride(t *testing.T) {
	path := initTestPath(t)
	t.Setenv("SHIPPO_PORTS", "*")
	err := initConfig(strings.NewReader("4000\ny\n"), &bytes.Buffer{}, nil)
	if err == nil || !strings.Contains(err.Error(), "SHIPPO_PORTS") {
		t.Fatalf("error = %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("created ignored config: %v", err)
	}
}

func TestInitReportsSaveFailure(t *testing.T) {
	path := initTestPath(t)
	if err := os.WriteFile(filepath.Dir(path), []byte("not a directory"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := initConfig(strings.NewReader("4000\ny\n"), &bytes.Buffer{}, nil); err == nil {
		t.Fatal("save failure ignored")
	}
}
