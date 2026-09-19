package daemon

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

var (
	machineName     string
	machineNameOnce sync.Once
)

func getMachineName() string {
	machineNameOnce.Do(func() {
		out, err := exec.Command("tailscale", "status", "--self", "--json").Output()
		if err != nil {
			machineName = "localhost"
			return
		}
		var status struct {
			Self struct {
				DNSName string `json:"DNSName"`
			} `json:"Self"`
		}
		if err := json.Unmarshal(out, &status); err != nil {
			machineName = "localhost"
			return
		}
		machineName = strings.TrimSuffix(status.Self.DNSName, ".")
	})
	return machineName
}

func serve(port int) error {
	// localhost lets the proxy reach IPv4-only and IPv6-only listeners.
	target := fmt.Sprintf("http://localhost:%d", port)
	httpsPort := fmt.Sprintf("%d", port)
	url := fmt.Sprintf("https://%s:%s", getMachineName(), httpsPort)

	log.Printf("serve %s at %s", target, url)

	cmd := exec.Command(
		"tailscale", "serve",
		"--bg",
		"--https="+httpsPort,
		target,
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("serve failed for port %d: %w\n%s", port, err, out)
	}

	addActiveURL(url)
	return nil
}

var (
	activeURLs   []string
	activeURLsMu sync.Mutex
)

func stateDir() string {
	dir := os.TempDir()
	return filepath.Join(dir, "shippo")
}

func writeState() {
	dir := stateDir()
	_ = os.MkdirAll(dir, 0755)

	// Write all active URLs
	content := strings.Join(activeURLs, "\n")
	if content != "" {
		content += "\n"
	}
	_ = os.WriteFile(filepath.Join(dir, "urls"), []byte(content), 0644)

	// Write last served URL
	if len(activeURLs) > 0 {
		last := activeURLs[len(activeURLs)-1]
		_ = os.WriteFile(filepath.Join(dir, "last"), []byte(last+"\n"), 0644)
	} else {
		_ = os.Remove(filepath.Join(dir, "last"))
	}
}

func addActiveURL(url string) {
	activeURLsMu.Lock()
	defer activeURLsMu.Unlock()
	activeURLs = append(activeURLs, url)
	writeState()
}

func removeActiveURL(port int) {
	activeURLsMu.Lock()
	defer activeURLsMu.Unlock()
	suffix := fmt.Sprintf(":%d", port)
	filtered := activeURLs[:0]
	for _, u := range activeURLs {
		if !strings.HasSuffix(u, suffix) {
			filtered = append(filtered, u)
		}
	}
	activeURLs = filtered
	writeState()
}

// URLs returns all currently served URLs.
func URLs() ([]string, error) {
	data, err := os.ReadFile(filepath.Join(stateDir(), "urls"))
	if err != nil {
		return nil, fmt.Errorf("no active URLs (daemon may not be running)")
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return nil, fmt.Errorf("no active URLs")
	}
	return lines, nil
}

// LastURL returns the most recently served URL.
func LastURL() (string, error) {
	data, err := os.ReadFile(filepath.Join(stateDir(), "last"))
	if err != nil {
		return "", fmt.Errorf("no recent URL (daemon may not have served anything yet)")
	}
	return strings.TrimSpace(string(data)), nil
}

func unserve(port int) error {
	httpsPort := fmt.Sprintf("%d", port)

	log.Printf("unserve https://%s:%s", getMachineName(), httpsPort)

	cmd := exec.Command(
		"tailscale", "serve",
		"--https="+httpsPort,
		"off",
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("unserve failed for port %d: %w\n%s", port, err, out)
	}

	removeActiveURL(port)
	return nil
}
