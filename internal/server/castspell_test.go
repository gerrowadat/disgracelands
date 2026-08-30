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

// The three parts of cast_spell that had no port at all (#306): the
// per-spell minimum position, the charmed caster's refusal to touch their
// master, and the "Okay." and say_spell that end the function.

// TestASpellHasItsOwnMinimumPosition, on top of the cast command's.
//
// The command's own POS_SITTING from cmd_info[] already worked; what was
// missing is the spell's floor above it, so a resting mage could cast
// anything. Each position has its own sentence and they are asserted
// together, because a switch that fell through to the default would still
// refuse -- correctly, in the wrong words.
func TestASpellHasItsOwnMinimumPosition(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "m")

	// detect magic is POS_STANDING, so every position below it is refused.
	//
	// Only two of the five branches are reachable from the `cast` command,
	// and that is the C's arrangement rather than a gap: cmd_info[] gives
	// `cast` a minimum of POS_SITTING, so a sleeping or resting caster is
	// turned away by the interpreter before do_cast runs at all, and the
	// sleeping/resting sentences belong to cast_spell's other entry point
	// -- the one its own comment names, "recommended entry point for spells
	// cast by NPCs via specprocs" (spell_parser.c:470-471).
	for _, tc := range []struct {
		position game.Position
		want     string
	}{
		{game.PosSitting, "You can't do this sitting!"},
		{game.PosFighting, "Impossible!  You can't concentrate enough!"},
	} {
		inWorld(t, srv, func(w *game.Live) { w.Find("Zod").Position = tc.position })
		c.send("cast 'detect magic'")
		c.expect(tc.want)
	}

	// And a POS_FIGHTING spell is castable from exactly the position that
	// refused the standing one, which is what makes the floor per-spell
	// rather than a blanket rule.
	inWorld(t, srv, func(w *game.Live) { w.Find("Zod").Position = game.PosFighting })
	c.send("cast 'armor'")
	c.expect("You feel someone protecting you.")
}

// TestACharmedCasterWillNotCastAtTheirMaster.
//
// The charm alone is not the check: a charmed pet can still cast at anybody
// else in the room, and only the master is off limits.
func TestACharmedCasterWillNotCastAtTheirMaster(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	master := dialClient(t, addr)
	master.create("Zod", "swordfish", "m", "m")
	pet := dialClient(t, addr)
	pet.create("Welmar", "hunter2!", "m", "m")
	moveTo(t, srv, "Welmar", ImmortStartRoom)

	inWorld(t, srv, func(w *game.Live) {
		charmed, leader := w.Find("Welmar"), w.Find("Zod")
		w.AddFollower(charmed, leader)
		charmed.Record.AffectFlags = charmed.Record.AffectFlags.With(game.AffectCharm)
		// A level-1 mage knows nothing; blindness is what is being cast
		// because it is TAR_NOT_SELF and so cannot be confused with the
		// self-cast refusals next door.
		charmed.Record.Level = 30
		if charmed.Record.Skills == nil {
			charmed.Record.Skills = map[game.SpellID]int32{}
		}
		charmed.Record.Skills[game.SpellBlindness] = 100
	})

	pet.send("cast 'blindness' zod")
	pet.expect("You are afraid you might hurt your master!")

	// Somebody else is still fair game.
	dog := spawnDog(t, srv, ImmortStartRoom)
	pet.send("cast 'blindness' dog")
	pet.settle()
	if !pet.seen("You are afraid you might hurt your master!") {
		t.Fatal("the master refusal never happened")
	}
	inWorld(t, srv, func(w *game.Live) {
		_ = dog // the roll may save; what matters is that it was not refused
	})
	if got := pet.transcript(); countOf(got, "You are afraid you might hurt your master!") != 1 {
		t.Errorf("casting at somebody who is not the master was refused too:\n%s", got)
	}
}

// TestCastingIsHeardByTheRoom is say_spell, which nobody heard at all.
//
// The class comparison is the point of it: a bystander who shares the
// caster's class hears the spell's real name and everybody else hears the
// syllable table's gibberish, so casting is partial information rather than
// either a secret or an announcement.
func TestCastingIsHeardByTheRoom(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	caster := dialClient(t, addr)
	caster.create("Zod", "swordfish", "m", "m")
	// A cleric, so a different class from the magic-user casting.
	stranger := dialClient(t, addr)
	stranger.create("Welmar", "hunter2!", "m", "c")
	moveTo(t, srv, "Welmar", ImmortStartRoom)

	caster.send("cast 'armor'")
	// cast_spell's own line to the caster, before anything the spell says.
	caster.expect("Okay.")
	// A different class hears the syllables, not the name. "armor" is
	// "ar" + "mor" -- see game.ScrambleSpellName.
	stranger.expect("Zod closes his eyes and utters the words, 'abrazak'.")

	// The same class hears the real thing.
	sameClass := dialClient(t, addr)
	sameClass.create("Mage", "hunter2!", "m", "m")
	moveTo(t, srv, "Mage", ImmortStartRoom)
	caster.send("cast 'armor'")
	sameClass.expect("Zod closes his eyes and utters the words, 'armor'.")
}

// A spell aimed at somebody else names them to the room, and addresses them
// directly in their own line -- which is the warning a target gets.
func TestTheTargetOfASpellIsToldDirectly(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	caster := dialClient(t, addr)
	caster.create("Zod", "swordfish", "m", "m")
	target := dialClient(t, addr)
	target.create("Welmar", "hunter2!", "m", "c")
	moveTo(t, srv, "Welmar", ImmortStartRoom)

	caster.send("cast 'armor' welmar")
	target.expect("Zod stares at you and utters the words, 'abrazak'.")
}

// countOf is strings.Count, named for what the assertion above means.
func countOf(haystack, needle string) int { return strings.Count(haystack, needle) }
