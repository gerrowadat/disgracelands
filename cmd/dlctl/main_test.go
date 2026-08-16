// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package main

import (
	"strings"
	"testing"
)

func TestUnimplementedCommandsNameTheirPhase(t *testing.T) {
	// A stub that fails silently or with "not found" would be worse than no
	// stub at all: the point is to say where the work is.
	//
	// Every real command is now implemented, so this injects one rather than
	// depending on which happen to be stubs — the mechanism is what matters
	// and it will be needed again for the phases still to come.
	commands = append(commands, command{
		name: "future thing", summary: "Something from a later phase", phase: 9,
	})
	defer func() { commands = commands[:len(commands)-1] }()

	err := run([]string{"future", "thing"})
	if err == nil {
		t.Fatal("run([future thing]) succeeded, want a not-implemented error")
	}
	for _, want := range []string{"future thing", "Phase 9", "go-port-plan.md"} {
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
	// "pfile dump" must not be mistaken for an unknown "pfile" command.
	// Asking for a character that does not exist proves it reached the
	// implementation: only that can produce a not-found error.
	//
	// Note that listing a *missing* directory is not an error — a roster
	// that does not exist yet is an empty one, which is the normal
	// fresh-install state — so this asks for a name instead.
	err := run([]string{"pfile", "dump", "--player-dir", "does/not/exist", "--name", "nobody"})
	if err == nil {
		t.Fatal("run([pfile dump --name=nobody]) succeeded, want a not-found error")
	}
	if strings.Contains(err.Error(), "unknown command") {
		t.Errorf("\"pfile dump\" was not dispatched: %v", err)
	}
}

func TestListingAMissingRosterIsNotAnError(t *testing.T) {
	// A blank roster is how a fresh install starts, and the C server creates
	// the files on demand. Reporting it as a failure would make every new
	// deployment look broken.
	if err := run([]string{"pfile", "dump", "--player-dir", "does/not/exist"}); err != nil {
		t.Errorf("listing a missing roster = %v, want success", err)
	}
}

func TestBarePfileIsUnknown(t *testing.T) {
	err := run([]string{"pfile"})
	if err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Errorf("run([pfile]) = %v, want an unknown-command error", err)
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
