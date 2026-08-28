// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"context"
	"testing"

	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/persist/player"
)

// `save`, end to end, and specifically do_save's duplication guard
// (act.other.c:173-186): with auto_save on, a mortal's `save` writes their
// aliases and nothing else, because the periodic sweep is already
// authoritative and letting anyone force a write is how two coordinated
// clients — or a client and a crash — duplicate items.

// savedRecord reads what is actually on disk, after any write the last
// command started has finished.
//
// The inWorld call is not decoration. `save` runs on the world goroutine and
// calls s.background() there, so a DoSync that has returned is ordered after
// that writes.Add(1); calling WaitForWrites straight after a socket-level
// expect() would race the Add instead, which is sync.WaitGroup's own
// documented unsafe case. See CLAUDE.md's testing traps.
func savedRecord(t *testing.T, srv *Server, store player.Store, name string) *game.PlayerRecord {
	t.Helper()
	inWorld(t, srv, func(*game.Live) {})
	srv.WaitForWrites()
	rec, err := store.Load(context.Background(), name)
	if err != nil {
		t.Fatalf("loading %s from disk: %v", name, err)
	}
	return rec
}

// aliasNamed returns the replacement stored for an alias, and whether it is
// there at all. The replacement keeps the leading space any_one_arg left on
// it (see internal/persist/player/binary/aliasfile.go), so callers compare
// against " brief" rather than "brief".
func aliasNamed(rec *game.PlayerRecord, name string) (string, bool) {
	for _, a := range rec.Aliases {
		if a.Name == name {
			return a.Replacement, true
		}
	}
	return "", false
}

// TestSaveWithAutoSaveOnWritesOnlyAliases is the guard itself: the mortal is
// told "Saving aliases.", the alias they just defined reaches disk, and the
// gold they acquired since the last write does not.
func TestSaveWithAutoSaveOnWritesOnlyAliases(t *testing.T) {
	srv, store := newTestServer(t)
	c := mortalClient(t, srv, listening(t, srv))

	// The archive's own setting, and the default, but say so out loud: this
	// test is about what the flag does.
	orig := game.Tuning()
	t.Cleanup(func() { game.SetTuning(orig) })
	tuning := orig
	tuning.AutoSave = true
	game.SetTuning(tuning)

	before := savedRecord(t, srv, store, "Mortal")

	c.send("alias b brief")
	c.expect("Alias added.")
	// setGold (mail_test.go) writes the record in the world and nothing
	// else, which is the lever this whole file needs: it opens a gap between
	// the world and the disk that `save` either closes or does not.
	setGold(t, srv, "Mortal", 4242)

	c.send("save")
	c.expect("Saving aliases.")

	after := savedRecord(t, srv, store, "Mortal")
	if repl, ok := aliasNamed(after, "b"); !ok || repl != " brief" {
		t.Errorf("alias b on disk = %q (present=%v), want %q", repl, ok, " brief")
	}
	if after.Points.Gold != before.Points.Gold {
		t.Errorf("gold on disk = %d after a guarded save, want it left at %d",
			after.Points.Gold, before.Points.Gold)
	}
}

// TestSaveWithAutoSaveOffWritesTheCharacter is the other half of
// config.c:150. Turn the sweep off and nothing is authoritative any more, so
// `save` goes back to being a real save — and says so.
func TestSaveWithAutoSaveOffWritesTheCharacter(t *testing.T) {
	srv, store := newTestServer(t)
	c := mortalClient(t, srv, listening(t, srv))

	orig := game.Tuning()
	t.Cleanup(func() { game.SetTuning(orig) })
	tuning := orig
	tuning.AutoSave = false
	game.SetTuning(tuning)

	c.send("alias b brief")
	c.expect("Alias added.")
	setGold(t, srv, "Mortal", 4242)

	c.send("save")
	c.expect("Saving Mortal and aliases.")

	rec := savedRecord(t, srv, store, "Mortal")
	if rec.Points.Gold != 4242 {
		t.Errorf("gold on disk = %d after an unguarded save, want 4242", rec.Points.Gold)
	}
	if repl, ok := aliasNamed(rec, "b"); !ok || repl != " brief" {
		t.Errorf("alias b on disk = %q (present=%v), want %q", repl, ok, " brief")
	}
}

// TestSaveGuardStopsAtLevelImmortalInclusive is the `<=` in
// `GET_LEVEL(ch) <= LVL_IMMORT`, which the C's comment spells out: it assumes
// guest immortals are not trustworthy, so level 31 is inside the guard and
// only 32 and up are let through. A `<` there — which is what the comment
// says to use if guest advances are disabled — would flip this test.
func TestSaveGuardStopsAtLevelImmortalInclusive(t *testing.T) {
	srv, store := newTestServer(t)
	// The first character on an empty roster is promoted to Implementor.
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	orig := game.Tuning()
	t.Cleanup(func() { game.SetTuning(orig) })
	tuning := orig
	tuning.AutoSave = true
	game.SetTuning(tuning)

	setGold(t, srv, "Zod", 4242)
	c.send("save")
	c.expect("Saving Zod and aliases.")
	if rec := savedRecord(t, srv, store, "Zod"); rec.Points.Gold != 4242 {
		t.Fatalf("gold on disk = %d after an Implementor's save, want 4242", rec.Points.Gold)
	}

	setLevel(t, srv, "Zod", game.LevelImmortal)
	setGold(t, srv, "Zod", 9999)
	c.send("save")
	c.expect("Saving aliases.")
	if rec := savedRecord(t, srv, store, "Zod"); rec.Points.Gold != 4242 {
		t.Errorf("gold on disk = %d after a level-%d save, want the guard to have left it at 4242",
			rec.Points.Gold, game.LevelImmortal)
	}
}
