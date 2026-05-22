package daemon

import (
	"bufio"
	"encoding/hex"
	"log"
	"os"
	"strconv"
	"strings"
)

type Reconciler struct {
	config *Config
	served map[int]bool
}

func NewReconciler(config *Config) *Reconciler {
	return &Reconciler{
		config: config,
		served: map[int]bool{},
	}
}

// InitialSync reads current LISTEN ports from /proc/net/tcp{,6} and serves allowed ones.
func (r *Reconciler) InitialSync() {
	ports := detectCurrentPorts()
	for port := range ports {
		if r.config.IsAllowed(port) {
			serve(port)
			r.served[port] = true
		}
	}
}

// Reconcile re-evaluates all served ports against the current config.
// Serves new allowed ports and unserves ports no longer allowed.
func (r *Reconciler) Reconcile() {
	current := detectCurrentPorts()

	// Serve new ports that are now allowed
	for port := range current {
		if r.config.IsAllowed(port) && !r.served[port] {
			log.Printf("config change: serving port %d", port)
			serve(port)
			r.served[port] = true
		}
	}

	// Unserve ports that are no longer allowed
	for port := range r.served {
		if !r.config.IsAllowed(port) {
			log.Printf("config change: unserving port %d", port)
			unserve(port)
			delete(r.served, port)
		}
	}
}

// HandleEvent processes an eBPF event.
func (r *Reconciler) HandleEvent(ev Event) {
	if Verbose {
		log.Printf("event: action=%d family=%d port=%d addr=[%08x %08x %08x %08x] localhost=%v",
			ev.Action, ev.Family, ev.Port, ev.Addr[0], ev.Addr[1], ev.Addr[2], ev.Addr[3], ev.IsLocalhost())
	}

	if !ev.IsLocalhost() {
		return
	}

	port := int(ev.Port)

	switch ev.Action {
	case ActionListenStart:
		if r.config.IsAllowed(port) && !r.served[port] {
			log.Printf("listen detected: port %d (%s)", port, ev.AddrString())
			serve(port)
			r.served[port] = true
		}
	case ActionListenStop:
		if r.served[port] {
			log.Printf("listen stopped: port %d (%s)", port, ev.AddrString())
			unserve(port)
			delete(r.served, port)
		}
	}
}

// Shutdown unserves all currently served ports.
func (r *Reconciler) Shutdown() {
	for port := range r.served {
		unserve(port)
	}
}

func detectCurrentPorts() map[int]bool {
	ports := map[int]bool{}
	for _, path := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		ps := readProcNet(path)
		for p := range ps {
			ports[p] = true
		}
	}
	return ports
}

func readProcNet(path string) map[int]bool {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()

	ports := map[int]bool{}
	scanner := bufio.NewScanner(file)

	first := true
	for scanner.Scan() {
		if first {
			first = false
			continue
		}

		fields := strings.Fields(scanner.Text())
		if len(fields) < 4 {
			continue
		}

		state := fields[3]
		if state != "0A" { // TCP_LISTEN
			continue
		}

		localAddr := fields[1]
		hostHex, portHex, ok := strings.Cut(localAddr, ":")
		if !ok {
			continue
		}

		if !isLocalhostHex(hostHex) {
			continue
		}

		port64, err := strconv.ParseInt(portHex, 16, 32)
		if err != nil {
			continue
		}

		ports[int(port64)] = true
	}

	return ports
}

func isLocalhostHex(s string) bool {
	// /proc/net/tcp: 127.0.0.1 is stored as 0100007F (little-endian)
	if s == "0100007F" {
		return true
	}

	// /proc/net/tcp6: ::1 is stored as 00000000000000000000000001000000
	// (4 groups of 4 bytes, each group in little-endian)
	if len(s) == 32 {
		decoded, err := hex.DecodeString(s)
		if err != nil {
			return false
		}
		// ::1 in 4-byte LE groups: 00000000 00000000 00000000 01000000
		for i := 0; i < 12; i++ {
			if decoded[i] != 0 {
				return false
			}
		}
		return decoded[12] == 1 && decoded[13] == 0 && decoded[14] == 0 && decoded[15] == 0
	}

	return false
}
