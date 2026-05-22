PREFIX  ?= $(HOME)/bin
SERVICE ?= $(HOME)/.config/systemd/user/shippo.service
LIBBPF_INCLUDE ?= $(shell ls -d /usr/src/linux-headers-$(shell uname -r)/tools/bpf/resolve_btfids/libbpf/include 2>/dev/null || echo /usr/include)

export BPF_CFLAGS ?= -O2 -g -Wall -I$(LIBBPF_INCLUDE)
export GOARCH ?= $(shell go env GOARCH)

.PHONY: generate build install enable disable status logs clean vmlinux

generate:
	go generate ./...

build: generate
	go build -o shippo .

install: build
	mkdir -p $(PREFIX)
	cp shippo $(PREFIX)/shippo
	mkdir -p $(dir $(SERVICE))
	cp shippo.service $(SERVICE)

enable: install
	systemctl --user daemon-reload
	systemctl --user enable --now shippo.service

disable:
	systemctl --user disable --now shippo.service

status:
	systemctl --user status shippo.service
	tailscale serve status

logs:
	journalctl --user -u shippo.service -f

vmlinux:
	bpftool btf dump file /sys/kernel/btf/vmlinux format c > bpf/vmlinux.h

clean:
	rm -f shippo
	rm -f shippo_*_bpfel.go shippo_*_bpfel.o
