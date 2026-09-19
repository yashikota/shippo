package daemon

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"net"
	"sync"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/ringbuf"

	shippobpf "github.com/yashikota/shippo/internal/daemon/bpf"
)

const (
	ActionListenStart = 0
	ActionListenStop  = 1
)

type Event struct {
	Port   uint16
	Family uint8
	Action uint8
	Addr   [4]uint32
}

func (e *Event) IsLocalhost() bool {
	if e.Family == 2 { // AF_INET
		// The BPF object and ring buffer decoder both use little endian.
		return e.Addr[0] == 0x0100007f
	}
	if e.Family == 10 { // AF_INET6
		// ::1 as four little-endian words of network address bytes.
		return e.Addr[0] == 0 && e.Addr[1] == 0 && e.Addr[2] == 0 &&
			e.Addr[3] == 0x01000000
	}
	return false
}

func (e *Event) AddrString() string {
	if e.Family == 2 {
		ip := make(net.IP, 4)
		binary.LittleEndian.PutUint32(ip, e.Addr[0])
		return ip.String()
	}
	if e.Family == 10 {
		ip := make(net.IP, 16)
		for i := 0; i < 4; i++ {
			binary.LittleEndian.PutUint32(ip[i*4:], e.Addr[i])
		}
		return ip.String()
	}
	return "unknown"
}

type Monitor struct {
	objs      *shippobpf.ShippoObjects
	links     []link.Link
	reader    *ringbuf.Reader
	eventCh   chan Event
	done      chan struct{}
	closeOnce sync.Once
}

func NewMonitor() (*Monitor, error) {
	spec, err := ebpf.LoadCollectionSpecFromReader(bytes.NewReader(shippobpf.Program))
	if err != nil {
		return nil, fmt.Errorf("load ebpf spec: %w", err)
	}

	objs, err := shippobpf.LoadShippo(spec)
	if err != nil {
		return nil, fmt.Errorf("load ebpf objects: %w", err)
	}

	links, err := objs.AttachAll()
	if err != nil {
		objs.Close()
		return nil, fmt.Errorf("attach tracepoint/sock/inet_sock_set_state: %w", err)
	}

	reader, err := ringbuf.NewReader(objs.Events)
	if err != nil {
		for _, l := range links {
			l.Close()
		}
		objs.Close()
		return nil, fmt.Errorf("open ring buffer: %w", err)
	}

	return &Monitor{
		objs:    objs,
		links:   links,
		reader:  reader,
		eventCh: make(chan Event, 64),
		done:    make(chan struct{}),
	}, nil
}

func (m *Monitor) Events() <-chan Event {
	return m.eventCh
}

func (m *Monitor) Run() {
	defer close(m.eventCh)
	log.Println("monitor: waiting for events...")

	for {
		record, err := m.reader.Read()
		if err != nil {
			if errors.Is(err, ringbuf.ErrClosed) {
				return
			}
			log.Printf("ring buffer read error: %v", err)
			continue
		}

		if Verbose {
			log.Printf("raw event: len=%d data=%x", len(record.RawSample), record.RawSample)
		}

		if len(record.RawSample) < 20 { // sizeof(struct event) = 2+1+1+16 = 20
			log.Printf("short record: %d bytes", len(record.RawSample))
			continue
		}

		var ev Event
		ev.Port = binary.LittleEndian.Uint16(record.RawSample[0:2])
		ev.Family = record.RawSample[2]
		ev.Action = record.RawSample[3]
		ev.Addr[0] = binary.LittleEndian.Uint32(record.RawSample[4:8])
		ev.Addr[1] = binary.LittleEndian.Uint32(record.RawSample[8:12])
		ev.Addr[2] = binary.LittleEndian.Uint32(record.RawSample[12:16])
		ev.Addr[3] = binary.LittleEndian.Uint32(record.RawSample[16:20])

		select {
		case m.eventCh <- ev:
		case <-m.done:
			return
		}
	}
}

func (m *Monitor) Close() {
	m.closeOnce.Do(func() {
		close(m.done)
		m.reader.Close()
		for _, l := range m.links {
			l.Close()
		}
		m.objs.Close()
	})
}
