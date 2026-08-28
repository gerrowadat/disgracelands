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

// These exercise the improved editor's own commands end to end, through
// `tedit`, which needs no setup beyond a logged-in character.
// internal/session/editor.go is the port and
// internal/session/editoracle_test.go is where each command is checked
// against the C case by case; what these add is that the wiring — the
// StateEditing dispatch, the buffer surviving between commands, the save
// actually landing — works over a socket.
//
// docs/deviations.md's "improved line editor" entry has the fuller story:
// CONFIG_IMPROVED_EDITOR was hardcoded on in the archived server, so these
// were never a stock/optional feature.

// TestImprovedEditorHelpAndInvalidOption: /h lists all eleven commands,
// every one of which works, and anything else is "Invalid option."
func TestImprovedEditorHelpAndInvalidOption(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	c := dialClient(t, addr)
	c.create("Zod", "swordfish", "m", "w")

	c.send("tedit motd")
	c.expect("Edit file below:")

	c.send("/h")
	c.expect("Editor command formats:")
	for _, want := range []string{"/a ", "/c ", "/d#", "/e#", "/f ", "/fi ", "/h ", "/i#", "/l ", "/n ", "/r ", "/ra ", "/s "} {
		if !c.seen(want) {
			t.Errorf("editor help dropped %q:\n%s", want, c.transcript())
		}
	}

	c.send("/x")
	c.expect("Invalid option.")
}

// TestImprovedEditorLineCommands walks /n, /i, /e and /d over one buffer, in
// that order, so each sees what the last one left behind.
func TestImprovedEditorLineCommands(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	c := dialClient(t, addr)
	c.create("Zod", "swordfish", "m", "w")

	c.send("tedit motd")
	c.expect("Edit file below:")
	c.send("/c")
	c.expect("Current buffer cleared.")

	c.send("alpha")
	c.send("beta")
	c.send("gamma")

	// The line number goes on a line of its own: that is the C's
	// "%4d:\r\n" (improved-edit.c:325), not a transcription slip.
	c.send("/n")
	c.expect("   1:\r\nalpha\r\n   2:\r\nbeta\r\n   3:\r\ngamma\r\n")

	c.send("/i 2 inserted")
	c.expect("Line inserted.")
	c.send("/e 4 replaced")
	c.expect("Line changed.")
	c.send("/d 1")
	c.expect("1 line deleted.")

	c.send("/l")
	c.expect("inserted\r\nbeta\r\nreplaced\r\n\r\n3 lines shown.")

	c.send("/s")
	c.expect("Saved.")

	var motd string
	inWorld(t, srv, func(_ *game.Live) { motd = srv.text.MOTD() })
	if motd != "inserted\r\nbeta\r\nreplaced\r\n" {
		t.Errorf("Text.MOTD() = %q, want what the line commands left in the buffer", motd)
	}
}

// TestImprovedEditorFormatAndReplace: /f rewraps the whole buffer and /r
// substitutes across it, both of which need the buffer to be one string
// rather than a list of lines.
func TestImprovedEditorFormatAndReplace(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	c := dialClient(t, addr)
	c.create("Zod", "swordfish", "m", "w")

	c.send("tedit motd")
	c.expect("Edit file below:")
	c.send("/c")
	c.expect("Current buffer cleared.")

	c.send("the quick brown")
	c.send("fox. it jumped")

	// Three lines' worth of words become one, capitalised at the start and
	// after the full stop, with the C's two spaces between sentences.
	c.send("/f")
	c.expect("Text formatted without indent.")
	c.send("/l")
	c.expect("The quick brown fox.  It jumped")

	c.send("/ra 'o' '0'")
	c.expect("Replaced 2 occurances of 'o' with '0'.")
	c.send("/r 'quick' 'slow'")
	c.expect("Replaced 1 occurance of 'quick' with 'slow'.")

	c.send("/s")
	c.expect("Saved.")

	var motd string
	inWorld(t, srv, func(_ *game.Live) { motd = srv.text.MOTD() })
	if !strings.Contains(motd, "The slow br0wn f0x.  It jumped") {
		t.Errorf("Text.MOTD() = %q, want the formatted and substituted text", motd)
	}
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

// Every line typed into the editor gets a `] ` back (issue #192).
//
// make_prompt's second branch is `else if (d->str) strcpy(prompt, "] ")`
// (comm.c:1008) — keyed off the pointer being written to rather than off the
// connection state, and before the CON_PLAYING test. The port was silent
// while typing, so a player had no sign the server had taken the line.
func TestTheEditorPromptsForEveryLine(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	c.send("tedit motd")
	c.expect("Edit file below:")
	// The command's own tail sends the first one: prompt() resolves to "] "
	// as soon as the editor is entered.
	c.expectCount("] ", 1)

	c.send("first line")
	c.expectCount("] ", 2)
	c.send("second line")
	c.expectCount("] ", 3)

	// An editor *command* prompts too — it is still a line typed at a
	// descriptor with d->str set.
	c.send("/l")
	c.expectCount("] ", 4)

	c.send("/a")
	c.expect("Edit aborted.")
}

// And the description editor at the menu, which is the same mechanism: the
// C's CON_EXDESC sets d->str like any other string_write caller.
func TestTheDescriptionEditorPromptsForEveryLine(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	c.send("quit")
	c.expectCount("Make your choice:", 2)

	c.send("2")
	c.expect("Enter the new text you'd like others to see")
	c.expectCount("] ", 1)

	c.send("A tall figure.")
	c.expectCount("] ", 2)

	c.send("@")
	c.expectCount("Make your choice:", 3)
}
