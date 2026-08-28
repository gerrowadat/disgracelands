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

// Issue #212: the `<DoC>` cyan broadcasts. Four of them, all through
// send_to_all_color (comm.c:2256) — a new player arriving, somebody gaining
// a level, somebody dying in a death trap (done with #209) and somebody
// remorting. Only the last reached players at all, and it went out with no
// colour and no exclusions.
//
// These are what the mud sounded like: somebody levelling was an event
// everybody saw, wherever they were standing.

// "A voice whispers in your ear, 'All hail %s, a newcomer!'"
// (interpreter.c:1608-1610), sent from nanny the moment the character is
// made.
func TestANewPlayerIsHailedToTheWholeGame(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	sitting := dialClient(t, addr)
	sitting.create("Zod", "swordfish", "m", "w")

	newcomer := dialClient(t, addr)
	newcomer.create("Fresh", "justarrived", "f", "w")

	sitting.settle()
	if !sitting.seen("A voice whispers in your ear, 'All hail Fresh, a newcomer!'") {
		t.Errorf("nobody was told about the new player:\n%s", sitting.transcript())
	}

	// Not to the newcomer. The C walks descriptor_list for CON_PLAYING and
	// theirs is CON_RMOTD, sitting on the message of the day; here they are
	// not in the world yet, and the two agree without a special case.
	if newcomer.seen("All hail Fresh, a newcomer!") {
		t.Errorf("the newcomer was hailed to themselves:\n%s", newcomer.transcript())
	}
}

// KCYN, and it is a *threshold* on the reader (comm.c:2263): the escape is
// there for anybody at C_NRM or above and absent for anybody who has turned
// colour off. Two readers, one message.
func TestTheHailIsCyanForWhoeverAskedForColour(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	coloured := dialClient(t, addr)
	coloured.create("Zod", "swordfish", "m", "w")

	plain := dialClient(t, addr)
	plain.create("Plain", "noescapes", "m", "w")
	plain.send("color off")
	plain.expect("is now Off.")
	coloured.settle()

	mark := len(plain.wire())

	newcomer := dialClient(t, addr)
	newcomer.create("Fresh", "justarrived", "f", "w")
	coloured.settle()
	plain.settle()

	if !strings.Contains(string(coloured.wire()), "\x1b[36m") {
		t.Error("the hail was not cyan for a reader on full colour")
	}
	tail := string(plain.wire()[mark:])
	if !strings.Contains(tail, "All hail Fresh") {
		t.Errorf("a reader with colour off did not get the hail at all:\n%s", tail)
	}
	if strings.Contains(tail, "\x1b[") {
		t.Errorf("a reader with colour off got escapes:\n%q", tail)
	}
}

// send_to_all_color's exclusion: "Doesn't echo if a player is writing"
// (comm.c:2265). Somebody halfway through a board post is not shouted into.
// The check was dead until #214 made PLR_WRITING real.
func TestAnnouncementsSkipSomebodyWhoIsWriting(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	writer := dialClient(t, addr)
	writer.create("Zod", "swordfish", "m", "w")
	observer := dialClient(t, addr)
	observer.create("Watcher", "stillreading", "m", "w")

	writer.send("tedit motd")
	writer.expect("Edit file below:")

	newcomer := dialClient(t, addr)
	newcomer.create("Fresh", "justarrived", "f", "w")

	// The observer is the barrier: they are in the world and not writing, so
	// once their settle() has been through the world goroutine the broadcast
	// loop has finished. `expect` is not a barrier for anybody else's buffer,
	// which is why this cannot just settle the writer — and the writer is in
	// the editor anyway, where settle()'s `time` would be a line of text.
	observer.settle()
	if !observer.seen("All hail Fresh, a newcomer!") {
		t.Fatalf("the control reader did not get the hail at all:\n%s", observer.transcript())
	}

	// Closing the editor writes to the writer's own socket, so anything the
	// broadcast had queued for them would be in the transcript ahead of it.
	writer.send("/a")
	writer.expect("Edit aborted.")
	if writer.seen("All hail Fresh, a newcomer!") {
		t.Errorf("a broadcast interrupted somebody mid-edit:\n%s", writer.transcript())
	}

	// And it comes back once the editor closes: the exclusion is the flag,
	// not the connection.
	second := dialClient(t, addr)
	second.create("Later", "afterwards", "m", "w")
	writer.settle()
	if !writer.seen("All hail Later, a newcomer!") {
		t.Errorf("broadcasts did not resume after the editor closed:\n%s", writer.transcript())
	}
}

// gain_exp's `if (is_altered)` block (limits.c:306-318), reached by `advance`
// through gain_exp_regardless' identical copy at :361-370.
//
// Two things this asserts that the port did not do before #212: the whole
// game hears it, and **the victim is told "You rise a level!"**. The second
// was in the C at every caller of gain_exp and here at only one of the three
// — a kill said it, the cityguard's award and `advance` did not.
func TestGainingLevelsIsAnnouncedToTheWholeGame(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	god, mortal := twoInARoom(t, srv, addr)

	bystander := dialClient(t, addr)
	bystander.create("Watcher", "somewhereelse", "m", "w")
	moveTo(t, srv, "Watcher", ShopRoom)
	bystander.settle()

	god.send("advance Bystander 5")
	god.expect("Okay.")

	mortal.settle()
	if !mortal.seen("You rise 4 levels!") {
		t.Errorf("the character was not told they had risen:\n%s", mortal.transcript())
	}

	// A room away and still heard, which is the point of send_to_all_color
	// over act(): this is not a room message.
	bystander.settle()
	if !bystander.seen("A voice whispers in your ear, 'Bystander has gained 4 levels!!!'") {
		t.Errorf("the game was not told about the levels:\n%s", bystander.transcript())
	}
}

// One level takes the singular branch, and it is a different sentence rather
// than the plural one with a 1 in it: "has gained a level!" against "has
// gained %d levels!!!" — one exclamation mark against three.
func TestGainingOneLevelHasItsOwnSentence(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	god, mortal := twoInARoom(t, srv, addr)

	god.send("advance Bystander 2")
	god.expect("Okay.")

	mortal.settle()
	if !mortal.seen("You rise a level!") {
		t.Errorf("the singular message was not sent:\n%s", mortal.transcript())
	}
	god.settle()
	if !god.seen("A voice whispers in your ear, 'Bystander has gained a level!'") {
		t.Errorf("the singular broadcast was not sent:\n%s", god.transcript())
	}
	if god.seen("levels!!!") {
		t.Errorf("one level took the plural branch:\n%s", god.transcript())
	}
}

// **The two copies of the block in the C are not identical.** gain_exp's
// whisper ends `!'\r\n` (limits.c:311) and gain_exp_regardless' ends `!'`
// with no newline at all (limits.c:368), so an `advance` runs the whisper
// straight into whatever is written next. Kept, under the fidelity rule.
func TestTheAdvanceWhisperHasNoTrailingNewline(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	god, _ := twoInARoom(t, srv, addr)

	god.send("advance Bystander 2")
	god.expect("Okay.")
	god.settle()

	const whisper = "'Bystander has gained a level!'"
	text := god.transcript()
	i := strings.Index(text, whisper)
	if i < 0 {
		t.Fatalf("no whisper in the transcript:\n%s", text)
	}
	if after := text[i+len(whisper):]; strings.HasPrefix(after, "\r\n") {
		t.Errorf("gain_exp_regardless' whisper ended with a newline; the C's does not:\n%q",
			after[:min(20, len(after))])
	}
}

// A kill goes through gain_exp proper, whose whisper *does* end in a
// newline — the other side of the same difference.
func TestTheKillWhisperEndsWithANewline(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	watcher := dialClient(t, addr)
	watcher.create("Zod", "swordfish", "m", "w")

	// Just under the next threshold, because gain_exp's second cap is the
	// local one: no single kill awards more than a tenth of the band between
	// this level and the next (docs/investigations/non-stock-features.md), so
	// a level has to be within reach before the kill rather than handed over
	// by it.
	killer, _ := place(t, srv, fighterRecord("Welmar", 2, 500), MortalStartRoom)
	killer.Record.Points.Exp = game.LevelExperience(game.ClassWarrior, 3) - 1
	victim, _ := place(t, srv, fighterRecord("a large dog", 30, 10), MortalStartRoom)
	victim.Record.Points.Exp = 5_000_000
	victim.NPC = true

	inWorld(t, srv, func(w *game.Live) { srv.award(w, killer, victim) })
	watcher.settle()

	text := watcher.transcript()
	i := strings.Index(text, "has gained")
	if i < 0 {
		t.Fatalf("the kill did not announce a level:\n%s", text)
	}
	if !strings.Contains(text[i:], "!'\r\n") {
		t.Errorf("gain_exp's whisper did not end with a newline:\n%q", text[i:])
	}
}
