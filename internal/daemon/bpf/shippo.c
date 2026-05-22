//go:build ignore

#include "vmlinux.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_core_read.h>
#include <bpf/bpf_tracing.h>

#define AF_INET 2
#define AF_INET6 10
#define TCP_LISTEN 10

#define ACTION_LISTEN_START 0
#define ACTION_LISTEN_STOP 1

struct event {
	__u16 port;
	__u8 family;
	__u8 action;
	__u32 addr[4];
};

struct {
	__uint(type, BPF_MAP_TYPE_RINGBUF);
	__uint(max_entries, 4096);
} events SEC(".maps");

// LISTEN 開始: inet_csk_listen_start(struct sock *sk)
SEC("kprobe/inet_csk_listen_start")
int kprobe_inet_csk_listen_start(struct pt_regs *ctx) {
	struct sock *sk = (struct sock *)PT_REGS_PARM1(ctx);

	struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
	if (!e)
		return 0;

	e->action = ACTION_LISTEN_START;
	e->family = BPF_CORE_READ(sk, __sk_common.skc_family);
	e->port = BPF_CORE_READ(sk, __sk_common.skc_num);
	e->addr[0] = 0;
	e->addr[1] = 0;
	e->addr[2] = 0;
	e->addr[3] = 0;

	if (e->family == AF_INET) {
		e->addr[0] = BPF_CORE_READ(sk, __sk_common.skc_rcv_saddr);
	} else if (e->family == AF_INET6) {
		BPF_CORE_READ_INTO(e->addr, sk,
				   __sk_common.skc_v6_rcv_saddr.in6_u.u6_addr32);
	}

	bpf_ringbuf_submit(e, 0);
	return 0;
}

// LISTEN 終了: tcp_set_state で LISTEN から別の状態への遷移
SEC("kprobe/tcp_set_state")
int kprobe_tcp_set_state(struct pt_regs *ctx) {
	struct sock *sk = (struct sock *)PT_REGS_PARM1(ctx);
	int new_state = (int)PT_REGS_PARM2(ctx);

	__u8 old_state = BPF_CORE_READ(sk, __sk_common.skc_state);

	// Only care about transitions FROM LISTEN
	if (old_state != TCP_LISTEN || new_state == TCP_LISTEN)
		return 0;

	struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
	if (!e)
		return 0;

	e->action = ACTION_LISTEN_STOP;
	e->family = BPF_CORE_READ(sk, __sk_common.skc_family);
	e->port = BPF_CORE_READ(sk, __sk_common.skc_num);
	e->addr[0] = 0;
	e->addr[1] = 0;
	e->addr[2] = 0;
	e->addr[3] = 0;

	if (e->family == AF_INET) {
		e->addr[0] = BPF_CORE_READ(sk, __sk_common.skc_rcv_saddr);
	} else if (e->family == AF_INET6) {
		BPF_CORE_READ_INTO(e->addr, sk,
				   __sk_common.skc_v6_rcv_saddr.in6_u.u6_addr32);
	}

	bpf_ringbuf_submit(e, 0);
	return 0;
}

char _license[] SEC("license") = "GPL";
