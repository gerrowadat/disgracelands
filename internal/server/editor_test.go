// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"strings"
	"testing"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// These exercise the five improved-editor commands this port answers —
// /a, /c, /h, /l and /s (internal/session/menu.go's editorCommand,
// improved-edit.c) — through `tedit`, which needs no setup beyond a
// logged-in character. docs/deviations.md's "improved line editor" entry
// has the fuller story: CONFIG_IMPROVED_EDITOR was hardcoded on in the
// archived server, so these were never a stock/optional feature.

// TestImprovedEditorHelpAndInvalidOption: /h lists only the five commands
// this port answers, not the six (/d, /e, /f, /i, /n, /r) it does not —
// advertising one of those would promise something that then says
// "Invalid option." when typed, which /x proves it does.
func TestImprovedEditorHelpAndInvalidOption(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	c := dialClient(t, addr)
	c.create("Zod", "swordfish", "m", "w")

	c.send("tedit motd")
	c.expect("Edit file below:")

	c.send("/h")
	c.expect("Editor command formats:")
	for _, want := range []string{"/a ", "/c ", "/h ", "/l ", "/s "} {
		if !c.seen(want) {
			t.Errorf("editor help dropped %q, one of the five commands this port answers:\n%s", want, c.transcript())
		}
	}
	for _, unwanted := range []string{"/d ", "/e ", "/f ", "/i ", "/n ", "/r "} {
		if c.seen(unwanted) {
			t.Errorf("editor help advertised %q, which this port does not implement:\n%s", unwanted, c.transcript())
		}
	}

	c.send("/x")
	c.expect("Invalid option.")
}

// TestImprovedEditorClearAndList: /c reports differently on an empty
// buffer than a full one, and /l lists either the whole buffer (no range
// header) or a range of it (with one), porting parse_action's
// PARSE_LIST_NORM (improved-edit.c:215).
func TestImprovedEditorClearAndList(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	c := dialClient(t, addr)
	c.create("Zod", "swordfish", "m", "w")

	// beginEditorSeeded starts the buffer with the field's current
	// content (tedit's own seeded-buffer behaviour), so the first /c has
	// something to clear.
	c.send("tedit motd")
	c.expect("Edit file below:")
	c.send("/c")
	c.expect("Current buffer cleared.")

	c.send("/c")
	c.expect("Current buffer empty.")

	c.send("First line.")
	c.send("Second line.")

	c.send("/l")
	c.expect("First line.")
	c.expect("Second line.")
	c.expect("2 lines shown.")
	if c.seen("Current buffer range") {
		t.Errorf("a full-buffer /l printed a range header:\n%s", c.transcript())
	}

	c.send("/l 2")
	c.expect("Current buffer range [2 - 2]:")
	c.expect("1 line shown.")
}

// TestImprovedEditorAbortDiscardsBuffer: /a leaves the file untouched —
// unlike '@' or /s, which both save — porting string_add's
// STRINGADD_ABORT case (modify.c:163) and tedit_string_cleanup's own
// "Edit aborted." (tedit.c:54-57).
func TestImprovedEditorAbortDiscardsBuffer(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	c := dialClient(t, addr)
	c.create("Zod", "swordfish", "m", "w")

	c.send("tedit motd")
	c.expect("Edit file below:")
	c.send("Never saved.")
	c.send("/a")
	c.expect("Edit aborted.")

	var motd string
	inWorld(t, srv, func(_ *game.Live) { motd = srv.text.MOTD() })
	if strings.Contains(motd, "Never saved.") {
		t.Errorf("Text.MOTD() = %q, an aborted edit should not have changed it", motd)
	}

	// /s saves exactly like '@' does, and a /c first proves it saves
	// only what is in the buffer at that point, not the seeded content
	// underneath it.
	c.send("tedit motd")
	c.expect("Edit file below:")
	c.send("/c")
	c.expectCount("Current buffer cleared.", 1)
	c.send("Saved via slash.")
	c.send("/s")
	c.expect("Saved.")

	inWorld(t, srv, func(_ *game.Live) { motd = srv.text.MOTD() })
	if !strings.Contains(motd, "Saved via slash.") {
		t.Errorf("Text.MOTD() = %q, want the /s-saved line", motd)
	}
	if strings.Contains(motd, "Mortal news.") {
		t.Errorf("Text.MOTD() = %q, /c should have cleared the seeded original before /s saved", motd)
	}
}
