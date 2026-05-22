package daemon

import (
	"testing"
)

func TestParsePortSpecs(t *testing.T) {
	tests := []struct {
		input   string
		wantRaw []string
	}{
		{"3000", []string{"3000"}},
		{"3000,5173,8000", []string{"3000", "5173", "8000"}},
		{"3000-3999", []string{"3000-3999"}},
		{"*", []string{"*"}},
		{"3000, 5173, 8000-8999, *", []string{"3000", "5173", "8000-8999", "*"}},
		{"", nil},
		{"0", nil},
		{"65536", nil},
		{"abc", nil},
		{"100-50", nil}, // invalid range (low > high)
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			_, raw := parsePortSpecs(tt.input)
			if len(raw) != len(tt.wantRaw) {
				t.Errorf("parsePortSpecs(%q) raw = %v, want %v", tt.input, raw, tt.wantRaw)
				return
			}
			for i := range raw {
				if raw[i] != tt.wantRaw[i] {
					t.Errorf("parsePortSpecs(%q) raw[%d] = %q, want %q", tt.input, i, raw[i], tt.wantRaw[i])
				}
			}
		})
	}
}

func TestWildcardMatcher(t *testing.T) {
	m := wildcardMatcher{}
	if !m.Match(1) {
		t.Error("wildcard should match port 1")
	}
	if !m.Match(65535) {
		t.Error("wildcard should match port 65535")
	}
}

func TestExactMatcher(t *testing.T) {
	m := exactMatcher{port: 3000}
	if !m.Match(3000) {
		t.Error("exact(3000) should match 3000")
	}
	if m.Match(3001) {
		t.Error("exact(3000) should not match 3001")
	}
}

func TestRangeMatcher(t *testing.T) {
	m := rangeMatcher{low: 8000, high: 8999}

	tests := []struct {
		port int
		want bool
	}{
		{7999, false},
		{8000, true},
		{8500, true},
		{8999, true},
		{9000, false},
	}

	for _, tt := range tests {
		if got := m.Match(tt.port); got != tt.want {
			t.Errorf("range(8000-8999).Match(%d) = %v, want %v", tt.port, got, tt.want)
		}
	}
}

func TestConfigIsAllowed(t *testing.T) {
	matchers, raw := parsePortSpecs("3000,8000-8999")
	config := &Config{matchers: matchers, raw: raw}

	tests := []struct {
		port int
		want bool
	}{
		{3000, true},
		{3001, false},
		{8000, true},
		{8500, true},
		{8999, true},
		{9000, false},
	}

	for _, tt := range tests {
		if got := config.IsAllowed(tt.port); got != tt.want {
			t.Errorf("IsAllowed(%d) = %v, want %v", tt.port, got, tt.want)
		}
	}
}

func TestConfigIsAllowedWildcard(t *testing.T) {
	matchers, raw := parsePortSpecs("*")
	config := &Config{matchers: matchers, raw: raw}

	for _, port := range []int{1, 80, 3000, 8080, 65535} {
		if !config.IsAllowed(port) {
			t.Errorf("wildcard config should allow port %d", port)
		}
	}
}
