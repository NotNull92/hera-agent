package cmd

import "testing"

func TestAgentOutputMode_EnvOverride(t *testing.T) {
	t.Setenv("HERA_AGENT_QUIET", "1")
	if !agentOutputMode() {
		t.Fatal("HERA_AGENT_QUIET=1 should force agent mode regardless of TTY state")
	}
}

func TestAgentOutputMode_EnvForceHuman(t *testing.T) {
	t.Setenv("HERA_AGENT_QUIET", "0")
	if agentOutputMode() {
		t.Fatal("HERA_AGENT_QUIET=0 should force human mode regardless of TTY state")
	}
}

func TestAgentOutputMode_DefaultsToAgentWhenPiped(t *testing.T) {
	// `go test` runs with stdout connected to a pipe (non-TTY) and no env
	// override, so the default should evaluate to agent mode.
	t.Setenv("HERA_AGENT_QUIET", "")
	if !agentOutputMode() {
		t.Fatal("expected agent mode when stdout is piped and no env override is set")
	}
}
