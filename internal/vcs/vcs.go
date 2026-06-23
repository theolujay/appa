// Package vcs reads the version embedded in the binary at build time
// by runtime/debug.ReadBuildInfo. The version is set via `go build
// -ldflags=-X=...` or by the VCS tooling that Go 1.18+ uses when
// building from a tagged commit inside a git repository.
package vcs

import (
	"runtime/debug"
	"strings"
)

func Version() string {
	bi, ok := debug.ReadBuildInfo()
	if ok {
		return bi.Main.Version
	}
	return ""
}

// DockerTag returns a Docker-safe version tag by replacing
// characters invalid in image references (e.g. '+') with hyphens.
func DockerTag() string {
	v := Version()
	return strings.ReplaceAll(v, "+", "-")
}
