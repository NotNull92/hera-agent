package cmd

import (
	"os"

	"github.com/mattn/go-isatty"
)

// agentOutputMode reports whether stdout is being consumed by a non-human
// reader (pipe, redirect, CI, agent harness). When true, callers should:
//   - emit compact JSON instead of indented (no decorative whitespace)
//   - suppress decorative stderr lines (compiling banners, update notices)
//
// Override with HERA_AGENT_QUIET=1 to force agent mode even on a TTY,
// or HERA_AGENT_QUIET=0 to force human mode even when piped.
func agentOutputMode() bool {
	switch os.Getenv("HERA_AGENT_QUIET") {
	case "1":
		return true
	case "0":
		return false
	}
	fd := os.Stdout.Fd()
	if isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd) {
		return false
	}
	return true
}
