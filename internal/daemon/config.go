package daemon

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
)

// portMatcher represents a port matching rule.
type portMatcher interface {
	Match(port int) bool
	String() string
}

type wildcardMatcher struct{}

func (w wildcardMatcher) Match(_ int) bool { return true }
func (w wildcardMatcher) String() string   { return "*" }

type exactMatcher struct{ port int }

func (e exactMatcher) Match(port int) bool { return port == e.port }
func (e exactMatcher) String() string      { return strconv.Itoa(e.port) }

type rangeMatcher struct{ low, high int }

func (r rangeMatcher) Match(port int) bool { return port >= r.low && port <= r.high }
func (r rangeMatcher) String() string      { return fmt.Sprintf("%d-%d", r.low, r.high) }

type Config struct {
	mu       sync.RWMutex
	matchers []portMatcher
	raw      []string // original string representations for serialization
}

func (c *Config) IsAllowed(port int) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, m := range c.matchers {
		if m.Match(port) {
			return true
		}
	}
	return false
}

func (c *Config) SetMatchers(matchers []portMatcher, raw []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.matchers = matchers
	c.raw = raw
}

func (c *Config) GetRaw() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	cp := make([]string, len(c.raw))
	copy(cp, c.raw)
	return cp
}

func (c *Config) String() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return strings.Join(c.raw, ", ")
}

// Ports returns explicit port set (for status display). For wildcard, returns nil.
func (c *Config) Ports() map[int]bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	ports := map[int]bool{}
	for _, m := range c.matchers {
		switch v := m.(type) {
		case wildcardMatcher:
			return nil // wildcard = all ports
		case exactMatcher:
			ports[v.port] = true
		case rangeMatcher:
			for p := v.low; p <= v.high; p++ {
				ports[p] = true
			}
		}
	}
	return ports
}

type configFile struct {
	Ports []string `json:"ports"`
}

var defaultPortsRaw = []string{}

func LoadConfig() *Config {
	// 1. Environment variable
	if env := os.Getenv("SHIPPO_PORTS"); env != "" {
		matchers, raw := parsePortSpecs(env)
		if len(matchers) > 0 {
			return &Config{matchers: matchers, raw: raw}
		}
	}

	// 2. Config file
	if matchers, raw := loadConfigFile(); len(matchers) > 0 {
		return &Config{matchers: matchers, raw: raw}
	}

	// 3. Defaults
	matchers, raw := parsePortSpecs(strings.Join(defaultPortsRaw, ","))
	return &Config{matchers: matchers, raw: raw}
}

// parsePortSpecs parses a comma-separated string of port specs.
// Supported formats: "*", "3000", "3000-3999"
func parsePortSpecs(s string) ([]portMatcher, []string) {
	var matchers []portMatcher
	var raw []string

	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}

		if p == "*" {
			matchers = append(matchers, wildcardMatcher{})
			raw = append(raw, "*")
			continue
		}

		if low, high, ok := strings.Cut(p, "-"); ok {
			l, err1 := strconv.Atoi(strings.TrimSpace(low))
			h, err2 := strconv.Atoi(strings.TrimSpace(high))
			if err1 == nil && err2 == nil && l > 0 && h < 65536 && l <= h {
				matchers = append(matchers, rangeMatcher{low: l, high: h})
				raw = append(raw, p)
				continue
			}
		}

		if n, err := strconv.Atoi(p); err == nil && n > 0 && n < 65536 {
			matchers = append(matchers, exactMatcher{port: n})
			raw = append(raw, p)
		}
	}

	return matchers, raw
}

// parsePorts parses comma-separated port numbers (used by add/remove commands).
func parsePorts(s string) map[int]bool {
	ports := map[int]bool{}
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if n, err := strconv.Atoi(p); err == nil && n > 0 && n < 65536 {
			ports[n] = true
		}
	}
	return ports
}

// ConfigPath can be set to override the default config file location.
var ConfigPath string

// ConfigFilePath returns the path to the config file.
func ConfigFilePath() string {
	return configFilePath()
}

func configFilePath() string {
	if ConfigPath != "" {
		return ConfigPath
	}
	home := realUserHome()
	if home == "" {
		return ""
	}
	return filepath.Join(home, ".config", "shippo", "config.json")
}

// realUserHome returns the home directory of the real user (not root when using sudo).
func realUserHome() string {
	// If running under sudo, use the original user's home
	if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" {
		return filepath.Join("/home", sudoUser)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
}

func loadConfigFile() ([]portMatcher, []string) {
	path := configFilePath()
	if path == "" {
		return nil, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil
	}

	var cf configFile
	if err := json.Unmarshal(data, &cf); err != nil {
		return nil, nil
	}

	return parsePortSpecs(strings.Join(cf.Ports, ","))
}

func saveConfigFile(config *Config) error {
	path := configFilePath()
	if path == "" {
		return os.ErrNotExist
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	cf := configFile{Ports: config.GetRaw()}

	data, err := json.MarshalIndent(cf, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return err
	}

	// If running under sudo, chown to the real user
	chownToRealUser(dir)
	chownToRealUser(path)
	return nil
}

func chownToRealUser(path string) {
	sudoUser := os.Getenv("SUDO_USER")
	if sudoUser == "" {
		return
	}
	u, err := user.Lookup(sudoUser)
	if err != nil {
		return
	}
	uid, _ := strconv.Atoi(u.Uid)
	gid, _ := strconv.Atoi(u.Gid)
	_ = os.Chown(path, uid, gid)
}

// watchConfig uses fsnotify to watch the config file for changes and reloads automatically.
func watchConfig(config *Config, reconciler *Reconciler) {
	path := configFilePath()
	if path == "" {
		return
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Printf("cannot create config dir: %v", err)
		return
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("cannot create config watcher: %v", err)
		return
	}

	if err := watcher.Add(dir); err != nil {
		log.Printf("cannot watch config dir: %v", err)
		watcher.Close()
		return
	}

	log.Printf("watching config: %s", path)
	base := filepath.Base(path)

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if filepath.Base(event.Name) != base {
				continue
			}
			if event.Op&(fsnotify.Write|fsnotify.Create) == 0 {
				continue
			}

			matchers, raw := loadConfigFile()
			if matchers == nil {
				continue
			}

			log.Printf("config reloaded: %s", strings.Join(raw, ", "))
			config.SetMatchers(matchers, raw)
			reconciler.Reconcile()

		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Printf("config watcher error: %v", err)
		}
	}
}
