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

// PLR_WRITING (issue #214). The C sets it in string_write (modify.c:100-101)
// and clears it in string_add's cleanup (:218-219), so anybody in the line
// editor carries it for as long as they are in there. Nothing in this port
// set it, which left every check on it dead: `tell` never refused, the
// channels never skipped, and the room never said "(writing)".
//
// `tedit` is the editor these use because it needs no setup beyond being an
// implementor, which the first character on the roster already is.

// writingFlag reads the bit off the world goroutine.
func writingFlag(t *testing.T, srv *Server, name string) (writing, mailing bool) {
	t.Helper()
	inWorld(t, srv, func(w *game.Live) {
		who := w.Find(name)
		if who == nil || who.Record == nil {
			t.Error("the character is not in the world")
			return
		}
		writing = who.Record.PlayerFlags.Has(game.PlayerWriting)
		mailing = who.Record.PlayerFlags.Has(game.PlayerMailing)
	})
	return writing, mailing
}

// The bit goes on when the editor opens and comes off when it saves.
func TestTheEditorSetsAndClearsPlayerWriting(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	c := dialClient(t, addr)
	c.create("Zod", "swordfish", "m", "w")

	if writing, _ := writingFlag(t, srv, "Zod"); writing {
		t.Fatal("PLR_WRITING was already set before the editor opened")
	}

	c.send("tedit motd")
	c.expect("Edit file below:")
	if writing, _ := writingFlag(t, srv, "Zod"); !writing {
		t.Error("PLR_WRITING was not set on entering the editor")
	}

	c.send("/s")
	c.expect("Saved.")
	if writing, _ := writingFlag(t, srv, "Zod"); writing {
		t.Error("PLR_WRITING survived a save")
	}
}

// ...and comes off on an abort too: the C clears it for STRINGADD_ABORT and
// STRINGADD_SAVE alike (modify.c:194, :218-219).
func TestAbortingTheEditorClearsPlayerWriting(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	c := dialClient(t, addr)
	c.create("Zod", "swordfish", "m", "w")

	c.send("tedit motd")
	c.expect("Edit file below:")
	if writing, _ := writingFlag(t, srv, "Zod"); !writing {
		t.Fatal("PLR_WRITING was not set, so the abort below proves nothing")
	}

	c.send("/a")
	c.expect("Edit aborted.")
	if writing, _ := writingFlag(t, srv, "Zod"); writing {
		t.Error("PLR_WRITING survived an abort")
	}
}

// `tell` refuses somebody mid-edit rather than interrupting them
// (act.comm.c:184).
func TestTellRefusesSomebodyWhoIsWriting(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	god, mortal := twoInARoom(t, srv, addr)

	god.send("tedit motd")
	god.expect("Edit file below:")
	// The god's own reply is not a barrier for the flag write, which
	// happens on the world goroutine — but the mortal's next command runs
	// there too, behind it, so its answer is.
	mortal.send("tell Zod hello")
	mortal.expect("He's writing a message right now; try again later.")

	// And once the editor closes, the tell lands.
	god.send("/a")
	god.expect("Edit aborted.")
	mortal.send("tell Zod hello again")
	mortal.expect("You tell Zod, 'hello again'")
}

// A channel skips anybody mid-edit: `!PLR_FLAGGED(i->character,
// PLR_WRITING)` sits in do_gen_comm's own send loop (act.comm.c:531),
// alongside the mute preference and the soundproof room.
func TestAChannelSkipsSomebodyWhoIsWriting(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	god, mortal := twoInARoom(t, srv, addr)

	god.send("tedit motd")
	god.expect("Edit file below:")

	mortal.send("gossip anybody there")
	mortal.expect("You gossip, 'anybody there'")

	god.send("/l")
	god.expect("1 line shown.")
	if god.seen("anybody there") {
		t.Errorf("a channel reached somebody in the editor:\n%s", god.transcript())
	}

	// The same channel does reach them once they are out, so the skip
	// above is the flag and not the gossip failing to go anywhere.
	god.send("/a")
	god.expect("Edit aborted.")
	mortal.send("gossip still there")
	god.expect("Bystander gossips, 'still there'")
}

// The room says so: list_one_char's " (writing)" (act.informative.c:306).
func TestTheRoomShowsWhoIsWriting(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	god, mortal := twoInARoom(t, srv, addr)

	god.send("tedit motd")
	god.expect("Edit file below:")

	mortal.send("look")
	mortal.expect("(writing)")

	god.send("/a")
	god.expect("Edit aborted.")
	mortal.send("look")
	mortal.settle()
	// Only the most recent `look` counts: the earlier one above is still
	// in the transcript and does say "(writing)".
	latest := mortal.transcript()
	if i := strings.LastIndex(latest, "The Temple Of Midgaard"); i >= 0 {
		latest = latest[i:]
	}
	if strings.Contains(latest, "(writing)") {
		t.Errorf("the room still called somebody a writer after they left "+
			"the editor:\n%s", mortal.transcript())
	}
}

// `mail` sets PLR_MAILING as well as PLR_WRITING, and the C is explicit
// about the division of labour: `SET_BIT(PLR_FLAGS(ch), PLR_MAILING); /*
// string_write() sets writing. */` (mail.c:567). Both come off together in
// the editor's cleanup (modify.c:218-219).
//
// Nothing reads PLR_MAILING yet — do_who's "(mailing)" annotation
// (act.informative.c:1174) is part of an annotation block this port's
// do_who does not have at all — so this asserts on the record directly.
func TestMailSetsPlayerMailingAsWellAsWriting(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	sender := dialClient(t, addr)
	sender.create("Sender", "postagepaid", "m", "m")

	recipient := dialClient(t, addr)
	recipient.create("Recipient", "waitingfor", "m", "m")
	recipient.send("quit")
	recipient.expect("Goodbye")
	recipient.close()
	waitForLogout(t, srv, "Recipient")

	withPostmaster(t, srv, "Sender")
	setGold(t, srv, "Sender", 1000)

	sender.send("mail Recipient")
	sender.expect("Write your message, use @ on a new line when done.")

	writing, mailing := writingFlag(t, srv, "Sender")
	if !writing {
		t.Error("PLR_WRITING was not set for a letter")
	}
	if !mailing {
		t.Error("PLR_MAILING was not set for a letter")
	}

	sender.send("Come back, all is forgiven.")
	sender.send("@")
	sender.expect("Message sent!")

	writing, mailing = writingFlag(t, srv, "Sender")
	if writing || mailing {
		t.Errorf("after the letter went, writing=%v mailing=%v, want both false",
			writing, mailing)
	}
}

// `wiznet` leaves a god in the editor alone: `!PLR_FLAGGED(d->character,
// PLR_WRITING | PLR_MAILING)` (act.wizard.c:1960). And `wiznet @` marks
// them "(Writing)" (act.wizard.c:1907-1911).
func TestWiznetSkipsAndMarksSomebodyWhoIsWriting(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	god, other := twoInARoom(t, srv, addr)

	// twoInARoom's second character is a mortal; wiznet needs two gods.
	setLevel(t, srv, "Bystander", game.LevelGod)
	other.send("wiznet @")
	other.expect("Gods online:")

	god.send("tedit motd")
	god.expect("Edit file below:")

	other.send("wiznet @")
	other.expect("Zod (Writing)")

	other.send("wiznet anyone about")
	other.settle()

	god.send("/l")
	god.expect("1 line shown.")
	if god.seen("anyone about") {
		t.Errorf("wiznet reached a god in the editor:\n%s", god.transcript())
	}

	god.send("/a")
	god.expect("Edit aborted.")
	other.send("wiznet @")
	other.settle()
	if strings.Contains(lastBlock(other.transcript(), "Gods online:"), "(Writing)") {
		t.Errorf("a god was still marked as writing after leaving the editor:\n%s",
			other.transcript())
	}
	other.send("wiznet back now")
	god.expect("back now")
}

// lastBlock returns the transcript from its final occurrence of marker.
func lastBlock(transcript, marker string) string {
	if i := strings.LastIndex(transcript, marker); i >= 0 {
		return transcript[i:]
	}
	return transcript
}
