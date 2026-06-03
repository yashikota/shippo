PREFIX  ?= $(HOME)/bin
SERVICE ?= $(HOME)/.config/systemd/user/shippo.service
LIBBPF_INCLUDE ?= $(shell ls -d /usr/src/linux-headers-$(shell uname -r)/tools/bpf/resolve_btfids/libbpf/include 2>/dev/null || echo /usr/include)
HOST_ARCH ?= $(shell uname -m)
MULTIARCH_INCLUDE ?= /usr/include/$(HOST_ARCH)-linux-gnu
GOBEE ?= $(shell command -v gobee 2>/dev/null)
GOBEE_TRANSLATE = $(if $(GOBEE),$(GOBEE),go run github.com/boratanrikulu/gobee/cmd/gobee)
CLANG ?= clang
LLVM_STRIP ?= llvm-strip
BPF_DIR := internal/daemon/bpf
BPF_SRC_DIR := $(BPF_DIR)/src
BPF_OBJECTS := $(BPF_DIR)/bin/x86/shippo.bpf.o $(BPF_DIR)/bin/arm64/shippo.bpf.o

export BPF_CFLAGS ?= -Wall -Werror -O2 -g -target bpfel -I$(MULTIARCH_INCLUDE) -I$(LIBBPF_INCLUDE)

.PHONY: generate translate bpf-compile bpf-objects build install enable disable status logs clean

generate: translate
	$(MAKE) bpf-objects

translate:
	$(GOBEE_TRANSLATE) translate --bindings-dir ./$(BPF_DIR) ./$(BPF_SRC_DIR)

bpf-compile: translate
	$(MAKE) bpf-objects

bpf-objects: $(BPF_OBJECTS)

$(BPF_DIR)/bin/x86/shippo.bpf.o: $(BPF_SRC_DIR)/shippo.bpf.c
	@mkdir -p $(dir $@)
	$(CLANG) $(BPF_CFLAGS) -D__TARGET_ARCH_x86 -c $< -o $@
	$(LLVM_STRIP) -g $@

$(BPF_DIR)/bin/arm64/shippo.bpf.o: $(BPF_SRC_DIR)/shippo.bpf.c
	@mkdir -p $(dir $@)
	$(CLANG) $(BPF_CFLAGS) -D__TARGET_ARCH_arm64 -c $< -o $@
	$(LLVM_STRIP) -g $@

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

clean:
	rm -f shippo
	rm -f $(BPF_OBJECTS)
	rm -f $(BPF_SRC_DIR)/shippo.bpf.c $(BPF_SRC_DIR)/shippo.bpf.c.map
	rm -f $(BPF_DIR)/shippo_bindings.go
