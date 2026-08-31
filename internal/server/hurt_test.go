// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"testing"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// "That really did HURT!" (fight.c:895-896), the last of the four things in
// damage()'s `default` branch and the only one this port did not have (#376).
//
// The branch has two quarter-of-maximum tests and they are not the same
// test, which is the whole of what these pin down. This one is a quarter
// taken *in one blow*; the bleeding warning below it is having less than a
// quarter *left*.

const hurtLine = "That really did HURT!"

// TestABigBlowSaysItHurt: over a quarter of maximum in one go.
func TestABigBlowSaysItHurt(t *testing.T) {
	srv, _ := newTestServer(t)

	attacker, _ := place(t, srv, fighterRecord("Zod", 30, 500), MortalStartRoom)
	victim, victimClient := place(t, srv, fighterRecord("Welmar", 5, 200), MortalStartRoom)

	// 51 of a maximum of 200: over the quarter, and leaves them on 149,
	// which is well over a quarter *left* — so this is the HURT line and
	// not the bleeding one.
	hitFor(t, srv, attacker, victim, 51)

	if !victimClient.said(hurtLine) {
		t.Error("a blow of over a quarter of maximum hit points said nothing")
	}
	if victimClient.said("BLEEDING") {
		t.Error("the bleeding warning fired for a character on three quarters health")
	}
}

// TestASmallBlowSaysNothing: exactly a quarter is not over a quarter. The
// C's test is `>`, and an off-by-one here is invisible at every other size.
func TestASmallBlowSaysNothing(t *testing.T) {
	srv, _ := newTestServer(t)

	attacker, _ := place(t, srv, fighterRecord("Zod", 30, 500), MortalStartRoom)
	victim, victimClient := place(t, srv, fighterRecord("Welmar", 5, 200), MortalStartRoom)

	hitFor(t, srv, attacker, victim, 50)

	if victimClient.said(hurtLine) {
		t.Error("a blow of exactly a quarter said it hurt; the C's test is `>`")
	}
}

// TestTheTwoQuartersAreDifferentQuarters is the point of the pair.
//
// A nearly-dead character taking a scratch gets the bleeding warning and
// not the HURT line: the blow is tiny, and what is left is not. Getting
// these two conditions confused would produce a server that says both
// together or neither, and both readings look right in isolation.
func TestTheTwoQuartersAreDifferentQuarters(t *testing.T) {
	srv, _ := newTestServer(t)

	attacker, _ := place(t, srv, fighterRecord("Zod", 30, 500), MortalStartRoom)
	victim, victimClient := place(t, srv, fighterRecord("Welmar", 5, 200), MortalStartRoom)

	inWorld(t, srv, func(_ *game.Live) { victim.Record.Points.Hit = 40 })
	hitFor(t, srv, attacker, victim, 1)

	if victimClient.said(hurtLine) {
		t.Error("a one-point scratch said it really did hurt")
	}
	if !victimClient.said("BLEEDING") {
		t.Error("a character under a quarter of maximum was not warned they were bleeding")
	}
}

// TestSanctuaryCanSilenceTheHurtLine: the C compares the damage *after*
// sanctuary halves it (fight.c halves at the top of damage() and everything
// downstream sees the reduced number), so a blow that would have said this
// can be quietened by the spell along with the damage it did.
//
// Worth a test of its own because it is the one place the figure being
// compared is not the figure the attacker rolled, and a port that compared
// the raw amount would pass every other test here.
func TestSanctuaryCanSilenceTheHurtLine(t *testing.T) {
	srv, _ := newTestServer(t)

	attacker, _ := place(t, srv, fighterRecord("Zod", 30, 500), MortalStartRoom)
	victim, victimClient := place(t, srv, fighterRecord("Welmar", 5, 200), MortalStartRoom)
	inWorld(t, srv, func(_ *game.Live) {
		victim.Record.AffectFlags = victim.Record.AffectFlags.With(game.AffectSanctuary)
	})

	// 51 would say it; halved to 25 it must not.
	hitFor(t, srv, attacker, victim, 51)

	if victimClient.said(hurtLine) {
		t.Error("sanctuary halved the damage but the HURT line was judged on the full amount")
	}
}

// TestPoisonNeverSaysItHurt: suffer() passes its own amount as the blow, so
// this is the arithmetic being left to decide rather than a special case —
// two points is not a quarter of anybody's maximum.
func TestPoisonNeverSaysItHurt(t *testing.T) {
	srv, _ := newTestServer(t)

	victim, victimClient := place(t, srv, fighterRecord("Welmar", 5, 200), MortalStartRoom)
	inWorld(t, srv, func(w *game.Live) { srv.suffer(w, victim, 2) })

	if victimClient.said(hurtLine) {
		t.Error("a two-point poison tick said it really did hurt")
	}
}
