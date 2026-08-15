package main

import (
	"strings"
	"testing"
)

func TestUnimplementedCommandsNameTheirPhase(t *testing.T) {
	// A stub that fails silently or with "not found" would be worse than no
	// stub at all: the point is to say where the work is.
	err := run([]string{"pfile", "convert", "--in=x"})
	if err == nil {
		t.Fatal("run([pfile convert]) succeeded, want a not-implemented error")
	}
	for _, want := range []string{"pfile convert", "Phase 2", "go-port-plan.md"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to mention %q", err, want)
		}
	}
}

func TestUnknownCommandIsRejected(t *testing.T) {
	err := run([]string{"summon", "puff"})
	if err == nil {
		t.Fatal("run([summon puff]) succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "unknown command") {
		t.Errorf("error = %q, want it to say the command is unknown", err)
	}
}

func TestMultiWordCommandsMatchBeforeTheirPrefix(t *testing.T) {
	// "world lint" must not be mistaken for an unknown "world" command.
	err := run([]string{"world", "lint"})
	if err == nil || !strings.Contains(err.Error(), "world lint") {
		t.Errorf("run([world lint]) = %v, want a not-implemented error naming \"world lint\"", err)
	}
}

func TestVersionRuns(t *testing.T) {
	if err := run([]string{"version"}); err != nil {
		t.Errorf("run([version]) = %v, want success", err)
	}
}

func TestHelpRuns(t *testing.T) {
	for _, arg := range []string{"help", "--help", "-h"} {
		if err := run([]string{arg}); err != nil {
			t.Errorf("run([%s]) = %v, want success", arg, err)
		}
	}
	if err := run(nil); err != nil {
		t.Errorf("run(nil) = %v, want success", err)
	}
}

func TestEveryCommandIsRunnableOrPhased(t *testing.T) {
	// Guards against adding a command with neither an implementation nor a
	// phase, which would produce "lands in Phase 0" and help nobody.
	for _, c := range commands {
		if c.run == nil && c.phase == 0 {
			t.Errorf("command %q has no implementation and no phase", c.name)
		}
		if c.summary == "" {
			t.Errorf("command %q has no summary", c.name)
		}
	}
}
