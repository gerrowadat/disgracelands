// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"context"
	"strings"
	"testing"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// `users` lists connections rather than players, which is the whole point of
// it: somebody stuck at the password prompt shows here and not in `who`.

// TestUsersListsEveryConnection, including one that has not logged in.
func TestUsersListsEveryConnection(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	god, _ := twoInARoom(t, srv, addr)

	// A third connection left sitting at the name prompt.
	lurker := dialClient(t, addr)
	lurker.expect("By what name")

	mark := len(god.transcript())
	god.send("users")
	god.expect("visible sockets connected.")
	out := god.transcript()[mark:]

	for _, want := range []string{
		"Num Class   Name         State          Idl Login@   Site",
		"Zod", "Bystander",
		// The one at the name prompt, named from connected_types.
		"Get name",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("users output is missing %q:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "3 visible sockets connected.") {
		t.Errorf("users did not count three sockets:\n%s", out)
	}
}

// TestUsersMinusPlayingHidesTheLoginPrompt, which is what `-p` is for.
func TestUsersMinusPlayingHidesTheLoginPrompt(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	god, _ := twoInARoom(t, srv, addr)

	lurker := dialClient(t, addr)
	lurker.expect("By what name")

	mark := len(god.transcript())
	god.send("users -p")
	god.expect("visible sockets connected.")
	out := god.transcript()[mark:]

	if strings.Contains(out, "Get name") {
		t.Errorf("-p still listed a connection that is not playing:\n%s", out)
	}
	if !strings.Contains(out, "2 visible sockets connected.") {
		t.Errorf("-p did not count two sockets:\n%s", out)
	}
}

// TestUsersFilters covers `-n` and `-l`, and the bad-flag answer.
func TestUsersFilters(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	god, _ := twoInARoom(t, srv, addr)

	mark := len(god.transcript())
	god.send("users -n Bystander")
	god.expect("visible sockets connected.")
	out := god.transcript()[mark:]
	if !strings.Contains(out, "Bystander") || strings.Contains(out, "Zod ") {
		t.Errorf("-n did not narrow to one name:\n%s", out)
	}

	// A level range that excludes the mortal.
	mark = len(god.transcript())
	god.send("users -l 30-34")
	god.expect("1 visible sockets connected.")
	out = god.transcript()[mark:]
	if strings.Contains(out, "Bystander") {
		t.Errorf("-l 30-34 listed a level 1 character:\n%s", out)
	}

	god.send("users -z")
	god.expect("format: users [-l minlevel[-maxlevel]]")
}

// TestUsersShowsSwitched. A god switched into somebody else is reported as the
// character the *connection* belongs to, in the "Switched" state.
//
// It takes a second immortal to look, because a switched god cannot run
// `users` themselves: the level is part of matching and while switched they
// have the body's level, so the command is invisible to them and they get
// "Huh?!?". That is the C, and it is the same reason the interpreter refuses
// them their own commands.
func TestUsersShowsSwitched(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	god, watcher := twoInARoom(t, srv, addr)
	spawnDog(t, srv, MortalStartRoom)

	if err := srv.engine.DoSync(context.Background(), func(w *game.Live) {
		w.Find("Bystander").Record.Level = game.LevelImplementor
	}); err != nil {
		t.Fatal(err)
	}

	god.send("switch dog")
	god.expect("Okay.")
	watcher.settle()

	mark := len(watcher.transcript())
	watcher.send("users")
	watcher.expect("visible sockets connected.")
	out := watcher.transcript()[mark:]

	if !strings.Contains(out, "Switched") {
		t.Errorf("a switched god was not reported as Switched:\n%s", out)
	}
	if !strings.Contains(out, "Zod") {
		t.Errorf("a switched god was not reported under their own name:\n%s", out)
	}
}

// TestUsersIsImmortalOnly — the level is part of matching, so a mortal typing
// it gets "Huh?!?".
func TestUsersIsImmortalOnly(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	_, mortal := twoInARoom(t, srv, addr)

	mortal.send("users")
	mortal.expect("Huh?!?")
}

// TestUsersHidesTheInvisible, because the listing test is CAN_SEE like
// everything else.
func TestUsersHidesTheInvisible(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	god, _ := twoInARoom(t, srv, addr)

	affect(t, srv, "Bystander", game.AffectInvisible)

	mark := len(god.transcript())
	god.send("users")
	god.expect("visible sockets connected.")
	out := god.transcript()[mark:]
	if strings.Contains(out, "Bystander") {
		t.Errorf("an invisible player was listed:\n%s", out)
	}
}
