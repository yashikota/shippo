package daemon

import (
	"testing"
)

func TestEventIsLocalhost_IPv4(t *testing.T) {
	tests := []struct {
		name string
		addr [4]uint32
		want bool
	}{
		{"127.0.0.1 (LE)", [4]uint32{0x0100007f, 0, 0, 0}, true},
		{"1.0.0.127 is not loopback", [4]uint32{0x7f000001, 0, 0, 0}, false},
		{"192.168.1.1", [4]uint32{0x0101a8c0, 0, 0, 0}, false},
		{"0.0.0.0", [4]uint32{0, 0, 0, 0}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ev := Event{Family: 2, Addr: tt.addr}
			if got := ev.IsLocalhost(); got != tt.want {
				t.Errorf("IsLocalhost() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEventIsLocalhost_IPv6(t *testing.T) {
	tests := []struct {
		name string
		addr [4]uint32
		want bool
	}{
		{"::100:0 is not loopback", [4]uint32{0, 0, 0, 1}, false},
		{"::1 (raw network order)", [4]uint32{0, 0, 0, 0x01000000}, true},
		{"::", [4]uint32{0, 0, 0, 0}, false},
		{"not loopback", [4]uint32{0, 0, 0, 2}, false},
		{"fe80::1", [4]uint32{0x000080fe, 0, 0, 1}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ev := Event{Family: 10, Addr: tt.addr}
			if got := ev.IsLocalhost(); got != tt.want {
				t.Errorf("IsLocalhost() = %v, want %v", got, tt.want)
			}
		})
	}
}
