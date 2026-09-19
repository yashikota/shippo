//go:build tools

package main

import (
	_ "github.com/boratanrikulu/bpfvet/pkg/analyzer"
	_ "github.com/boratanrikulu/gobee/bpf"
	_ "github.com/boratanrikulu/gobee/cmd/gobee/app"
)
