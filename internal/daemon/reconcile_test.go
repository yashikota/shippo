package daemon

import (
	"testing"
)

func TestIsLocalhostHex(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"0100007F", true},                          // 127.0.0.1 IPv4
		{"00000000", false},                         // 0.0.0.0
		{"0101A8C0", false},                         // 192.168.1.1
		{"00000000000000000000000001000000", true},   // ::1 IPv6
		{"00000000000000000000000000000000", false},  // :: IPv6
		{"0000000000000000FFFF00000100007F", false},  // ::ffff:127.0.0.1 (mapped)
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := isLocalhostHex(tt.input); got != tt.want {
				t.Errorf("isLocalhostHex(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
