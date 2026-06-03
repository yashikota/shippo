//go:build ignore

package main

import "github.com/boratanrikulu/gobee/bpf"

//bpf:license GPL

const (
	afInet      = uint16(2)
	afInet6     = uint16(10)
	ipProtoTCP  = uint16(6)
	tcpListen   = int32(10)
	actionStart = uint8(0)
	actionStop  = uint8(1)
)

// InetSockSetStateCtx mirrors trace_event_raw_inet_sock_set_state.
type InetSockSetStateCtx struct {
	TraceCommon [8]uint8
	Skaddr      uint64
	Oldstate    int32
	Newstate    int32
	Sport       uint16
	Dport       uint16
	Family      uint16
	Protocol    uint16
	Saddr       uint32
	Daddr       uint32
	SaddrV6     [4]uint32
	DaddrV6     [4]uint32
}

type Event struct {
	Port   uint16
	Family uint8
	Action uint8
	Addr   [4]uint32
}

var Events = bpf.RingBuf[Event]{MaxEntries: 4096}

func emitEvent(tp *InetSockSetStateCtx, action uint8) {
	e, ok := Events.Reserve()
	if !ok {
		return
	}

	e.Port = tp.Sport
	e.Family = uint8(tp.Family)
	e.Action = action
	e.Addr[0] = 0
	e.Addr[1] = 0
	e.Addr[2] = 0
	e.Addr[3] = 0

	if tp.Family == afInet {
		e.Addr[0] = tp.Saddr
	} else if tp.Family == afInet6 {
		e.Addr[0] = tp.SaddrV6[0]
		e.Addr[1] = tp.SaddrV6[1]
		e.Addr[2] = tp.SaddrV6[2]
		e.Addr[3] = tp.SaddrV6[3]
	}

	Events.Submit(e)
}

//bpf:section tracepoint/sock/inet_sock_set_state
func OnInetSockSetState(ctx *InetSockSetStateCtx) bpf.TpReturn {
	if ctx.Protocol != ipProtoTCP {
		return bpf.TpOk
	}
	if ctx.Newstate == tcpListen {
		emitEvent(ctx, actionStart)
		return bpf.TpOk
	}
	if ctx.Oldstate == tcpListen {
		emitEvent(ctx, actionStop)
		return bpf.TpOk
	}

	return bpf.TpOk
}

func main() {}
