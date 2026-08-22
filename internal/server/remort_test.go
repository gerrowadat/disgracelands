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
)

// remort and redeem, the local mechanic the IS_<CLASS> macros exist for.

func remortPair(t *testing.T, srv *Server, addr string) (god, mortal *client) {
	t.Helper()
	god = dialClient(t, addr)
	god.create("Zod", "swordfish", "m", "w")
	mortal = dialClient(t, addr)
	mortal.create("Bystander", "swordfish", "f", "w")
	return god, mortal
}

// TestRemortGrantsAClassAndSaysSo, and the character gains it for real: the
// vector is what every IS_<CLASS> check in the game reads.
func TestRemortGrantsAClassAndSaysSo(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	god, mortal := remortPair(t, srv, addr)

	god.send("remort Bystander thief")
	god.expect("Bystander remorted to become a thief!")
	god.expect("This player now has access to skills/spells of: thief warrior.")

	mortal.expect("You gain the skills and privileges of a thief!")

	var isThief, isWarrior bool
	inWorld(t, srv, func(w *game.Live) {
		rec := w.Find("Bystander").Record
		isThief, isWarrior = game.IsThief(rec), game.IsWarrior(rec)
	})
	if !isThief || !isWarrior {
		t.Errorf("after remorting: thief=%v warrior=%v, want both", isThief, isWarrior)
	}
}

// TestRemortWithNoClassReports what somebody already has, which is the
// one-argument form.
func TestRemortWithNoClassReports(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	god, _ := remortPair(t, srv, addr)

	god.send("remort Bystander")
	god.expect("Player currently has access to skills/spells of: warrior.")
}

// TestRemortRefusesTheCurrentClass, in both directions: you cannot remort
// somebody into what they already are, and you cannot undo it either.
func TestRemortRefusesTheCurrentClass(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	god, _ := remortPair(t, srv, addr)

	god.send("remort Bystander warrior")
	god.expect("But Bystander is already a warrior!")

	god.send("remort Bystander -warrior")
	god.expectCount("But Bystander is already a warrior!", 2)
}

// TestRemortUndo takes a granted class back.
func TestRemortUndo(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	god, mortal := remortPair(t, srv, addr)

	god.send("remort Bystander cleric")
	god.expect("Bystander remorted to become a cleric!")

	god.send("remort Bystander -cleric")
	god.expect("Bystander is no longer a cleric.")
	mortal.expect("You sink to the ground, aghast, as you feel your clerichood slip away!")

	var isCleric bool
	inWorld(t, srv, func(w *game.Live) {
		isCleric = game.IsCleric(w.Find("Bystander").Record)
	})
	if isCleric {
		t.Error("the cleric bit survived the undo")
	}
}

// TestRemortRejectsAnUnknownClass. The C compares against pc_class_snames with
// strcasecmp, so "mag" is not "mage".
func TestRemortRejectsAnUnknownClass(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	god, _ := remortPair(t, srv, addr)

	for _, arg := range []string{"wizard", "mag", "magic"} {
		god.send("remort Bystander " + arg)
		god.settle()
	}
	god.expectCount("Invalid class.", 3)
}

// TestRemortNeedsAnImplementor: the level is part of matching, so a mortal
// typing it gets "Huh?!?" rather than a refusal.
func TestRemortNeedsAnImplementor(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	_, mortal := remortPair(t, srv, addr)

	mortal.send("remort Zod thief")
	mortal.expect("Huh?!?")
}

// TestRedeemLiftsTheFallenState, and refuses somebody who has not fallen.
func TestRedeemLiftsTheFallenState(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	god, mortal := remortPair(t, srv, addr)

	god.send("redeem Bystander")
	god.expect("Your victim has not fallen!")

	if err := srv.engine.DoSync(context.Background(), func(w *game.Live) {
		rec := w.Find("Bystander").Record
		game.SetSpecFlags(rec, game.SpecFlagsOf(rec).Set(game.PaladinFallen))
	}); err != nil {
		t.Fatal(err)
	}

	god.send("redeem Bystander")
	god.expect("Redeemed.")
	mortal.expect("You feel your paladinly powers restored!")

	var fallen bool
	inWorld(t, srv, func(w *game.Live) {
		fallen = game.SpecFlagsOf(w.Find("Bystander").Record).Has(game.PaladinFallen)
	})
	if fallen {
		t.Error("the fallen flag survived a redeem")
	}
}
