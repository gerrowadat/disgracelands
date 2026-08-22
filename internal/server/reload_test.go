// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"os"
	"path/filepath"
	"testing"
)

// `reload` re-reads a canned text file without restarting, porting do_reboot —
// which does not reboot anything, despite the name.

// TestReloadPicksUpAnEditedFile.
func TestReloadPicksUpAnEditedFile(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	c.send("motd")
	c.expect("Mortal news.")

	path := filepath.Join(srv.text.dir, motdFile)
	if err := os.WriteFile(path, []byte("Stop press.\r\n"), 0o600); err != nil {
		t.Fatalf("rewriting the motd: %v", err)
	}

	// Still the old text until told otherwise: the file is read at boot and
	// nothing watches it.
	c.send("motd")
	c.expectCount("Mortal news.", 2)

	c.send("reload motd")
	c.expect("Okay.")
	c.send("motd")
	c.expect("Stop press.")
}

// TestReloadAll does the lot in one go.
func TestReloadAll(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	if err := os.WriteFile(filepath.Join(srv.text.dir, motdFile),
		[]byte("Everything changed.\r\n"), 0o600); err != nil {
		t.Fatalf("rewriting the motd: %v", err)
	}

	c.send("reload all")
	c.expect("Okay.")
	c.send("motd")
	c.expect("Everything changed.")
}

// TestReloadRejectsAnUnknownName, which is the C's else branch.
func TestReloadRejectsAnUnknownName(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	for _, arg := range []string{"", "banana", "worlds"} {
		c.send("reload " + arg)
		c.settle()
	}
	c.expectCount("Unknown reload option.", 3)
}

// TestReloadNeedsAnImplementor: the level is part of matching, so a mortal
// gets "Huh?!?" rather than a refusal.
func TestReloadNeedsAnImplementor(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	_, mortal := twoInARoom(t, srv, addr)

	mortal.send("reload news")
	mortal.expect("Huh?!?")
}

// TestReloadAllLeavesXhelpAlone. The C's `all` is twelve
// file_to_string_alloc calls and does not include the help database, so
// `reload all` followed by `reload xhelp` is a real sequence rather than a
// redundant one.
func TestReloadAllLeavesXhelpAlone(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	c.send("reload xhelp")
	c.expect("Okay.")
}
