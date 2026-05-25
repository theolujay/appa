// Package vcs reads the version embedded in the binary at build time
// by runtime/debug.ReadBuildInfo. The version is set via `go build
// -ldflags=-X=...` or by the VCS tooling that Go 1.18+ uses when
// building from a tagged commit inside a git repository.
package vcs

import (
	"runtime/debug"
)

func Version() string {
	bi, ok := debug.ReadBuildInfo()
	if ok {
		return bi.Main.Version
	}
	return ""
}
