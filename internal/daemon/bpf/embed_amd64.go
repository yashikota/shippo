//go:build linux && amd64

package bpf

import _ "embed"

//go:embed bin/x86/shippo.bpf.o
var Program []byte
