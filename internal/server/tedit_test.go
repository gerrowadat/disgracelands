// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// TestTeditEditsAndPersistsMOTD is end to end: the current motd is shown,
// typed lines are appended after it (string_write's own seeded-buffer
// behaviour, not a fresh compose), "@" saves — the in-memory Text.MOTD()
// changes immediately, a second connection sees it without a restart, and
// the file on disk changes too.
func TestTeditEditsAndPersistsMOTD(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	god := dialClient(t, addr)
	// The first character on the roster is an implementor (twoInARoom's
	// own doc comment, visibility_test.go) — every teditField's level,
	// including the strictest (LevelImplementor), is reachable.
	god.create("Zod", "swordfish", "m", "w")

	god.send("tedit motd")
	god.expect("Edit file below:")
	// expect, not seen: the content is a separate write after "Edit file
	// below:", and seen() only checks what has already been read, not
	// what is still in flight (CLAUDE.md's own note on expect not being
	// a barrier for anybody else's buffer applies here too, to the same
	// client's own later bytes).
	god.expect("Mortal news.")

	god.send("A new message of the day.")
	god.send("@")
	god.expect("Saved.")

	// The appended text is there alongside the original — append, not
	// replace, matching string_write's own behaviour when the buffer it
	// is handed already has something in it.
	var motd string
	inWorld(t, srv, func(_ *game.Live) { motd = srv.text.MOTD() })
	if !strings.Contains(motd, "Mortal news.") || !strings.Contains(motd, "A new message of the day.") {
		t.Errorf("Text.MOTD() = %q, want both the original and the appended line", motd)
	}

	// A second connection sees the change immediately, no restart needed.
	other := dialClient(t, addr)
	other.create("Bystander", "swordfish", "f", "w")
	if !other.seen("A new message of the day.") {
		t.Errorf("a second login did not see the updated motd:\n%s", other.transcript())
	}

	// Persisted to disk too, off the world goroutine.
	srv.WaitForWrites()
	b, err := os.ReadFile(filepath.Join(srv.text.dir, motdFile))
	if err != nil {
		t.Fatalf("reading %s: %v", motdFile, err)
	}
	if !strings.Contains(string(b), "A new message of the day.") {
		t.Errorf("motd on disk = %q, want the appended line", string(b))
	}
}

// TestTeditRefusesAFieldBelowTheCallersLevel: a GreaterGod (LevelGreaterGod,
// the command's own table gate) reaches `tedit motd` but not `tedit
// credits` (LevelImplementor) — the C's own two-stage check.
func TestTeditRefusesAFieldBelowTheCallersLevel(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	c := dialClient(t, addr)
	c.create("Zod", "swordfish", "m", "w")
	setLevel(t, srv, "Zod", game.LevelGreaterGod)

	c.send("tedit credits")
	c.expect("You are not godly enough for that!")
}

// TestTeditListsReachableFields: no argument lists whichever fields the
// caller's own level can reach.
func TestTeditListsReachableFields(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	c := dialClient(t, addr)
	c.create("Zod", "swordfish", "m", "w")
	setLevel(t, srv, "Zod", game.LevelGreaterGod)

	c.send("tedit")
	c.expect("Files available to be edited:")
	if !c.seen("motd") || c.seen("credits") {
		t.Errorf("tedit's listing did not match a GreaterGod's own reach:\n%s", c.transcript())
	}
}

// TestTeditUnrecognisedFieldIsRefused.
func TestTeditUnrecognisedFieldIsRefused(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	c := dialClient(t, addr)
	c.create("Zod", "swordfish", "m", "w")

	c.send("tedit nonsense")
	c.expect("Invalid text editor option.")
}
