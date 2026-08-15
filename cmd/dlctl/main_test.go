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
	// "pfile convert" must not be mistaken for an unknown "pfile" command.
	err := run([]string{"pfile", "convert"})
	if err == nil || !strings.Contains(err.Error(), "pfile convert") {
		t.Errorf("run([pfile convert]) = %v, want a not-implemented error naming \"pfile convert\"", err)
	}
}

func TestBareGroupNameIsUnknown(t *testing.T) {
	// "world" on its own is not a command; only "world <something>" is.
	err := run([]string{"world"})
	if err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Errorf("run([world]) = %v, want an unknown-command error", err)
	}
}

func TestWorldLintDispatches(t *testing.T) {
	// Not a test of linting — that lives with the parser — but of dispatch:
	// a real subcommand must reach its implementation rather than falling
	// through to "unknown command" or a phase stub.
	err := run([]string{"world", "lint", "--world-dir", "does/not/exist"})
	if err == nil {
		t.Fatal("world lint on a missing directory succeeded, want an error")
	}
	if strings.Contains(err.Error(), "unknown command") || strings.Contains(err.Error(), "not implemented") {
		t.Errorf("world lint did not reach its implementation: %v", err)
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
