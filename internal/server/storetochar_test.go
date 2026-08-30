// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"testing"
	"time"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// storedAs edits a logged-out character's record on disk, which is the only
// way to set up what store_to_char reacts to: both of the behaviours below
// key off what is *in the file* at the moment it is read.
func storedAs(t *testing.T, srv *Server, name string, edit func(*game.PlayerRecord)) {
	t.Helper()
	rec, err := srv.players.Load(t.Context(), name)
	if err != nil {
		t.Fatalf("loading %s: %v", name, err)
	}
	edit(rec)
	if err := srv.players.Save(t.Context(), rec); err != nil {
		t.Fatalf("saving %s: %v", name, err)
	}
}

// TestAnHourAwayComesBackWhole is db.c:2276-2287, the last thing
// store_to_char does:
//
//	if (!AFF_FLAGGED(ch, AFF_POISON) &&
//	      time(0) - st->last_logon >= SECS_PER_REAL_HOUR) {
//	  GET_HIT(ch) = GET_MAX_HIT(ch);
//	  GET_MOVE(ch) = GET_MAX_MOVE(ch);
//	  GET_MANA(ch) = GET_MAX_MANA(ch);
//	}
//
// A player who logged off hurt and came back the next day came back whole
// on the archived server, and came back exactly as hurt here (#295).
//
// End to end through a real login, because that is where it goes wrong: the
// value it reads is the *stored* last logon, and Enter overwrites that field
// the moment the character reaches the world.
func TestAnHourAwayComesBackWhole(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	first := dialClient(t, addr)
	first.create("Zod", "swordfish", "m", "m")
	whole := figuresOf(t, srv, "Zod")
	first.send("quit")
	first.expect("Goodbye")
	waitForLogout(t, srv, "Zod")

	storedAs(t, srv, "Zod", func(rec *game.PlayerRecord) {
		rec.Points.Hit, rec.Points.Mana, rec.Points.Move = 1, 2, 3
		rec.LastLogon = time.Now().UTC().Add(-2 * time.Hour)
	})

	back := dialClient(t, addr)
	back.login("Zod", "swordfish")
	got := figuresOf(t, srv, "Zod")
	if got.hit != whole.maxHit || got.mana != whole.maxMana || got.move != whole.maxMove {
		t.Errorf("after two hours away: %d/%d hp, %d/%d mana, %d/%d mv; want each pool full",
			got.hit, whole.maxHit, got.mana, whole.maxMana, got.move, whole.maxMove)
	}
}

// The same character back after five minutes stays exactly as hurt as they
// left. Asserted separately because a "restore" that simply always fired
// would pass the test above and be a different game: the hour is the whole
// of the rule, and a player who quits to dodge a fight and comes straight
// back does not get healed for it.
func TestComingStraightBackDoesNotHeal(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	first := dialClient(t, addr)
	first.create("Zod", "swordfish", "m", "m")
	first.send("quit")
	first.expect("Goodbye")
	waitForLogout(t, srv, "Zod")

	storedAs(t, srv, "Zod", func(rec *game.PlayerRecord) {
		rec.Points.Hit, rec.Points.Mana, rec.Points.Move = 1, 2, 3
		rec.LastLogon = time.Now().UTC().Add(-5 * time.Minute)
	})

	back := dialClient(t, addr)
	back.login("Zod", "swordfish")
	if got := figuresOf(t, srv, "Zod"); got.hit != 1 || got.mana != 2 || got.move != 3 {
		t.Errorf("after five minutes away: %d hp, %d mana, %d mv; want 1/2/3 unchanged",
			got.hit, got.mana, got.move)
	}
}

// Poison holds the restore off however long you stay away -- the C's own
// exemption, and the only reason the condition is not simply an elapsed
// time. It reads the flags *after* the stored affects have gone back on,
// which is the only point at which anybody is poisoned: poison is an
// affect, so before affect_to_char the flag is not there to read.
func TestPoisonKeepsYouHurtHoweverLongYouAreAway(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	first := dialClient(t, addr)
	first.create("Zod", "swordfish", "m", "m")
	first.send("quit")
	first.expect("Goodbye")
	waitForLogout(t, srv, "Zod")

	storedAs(t, srv, "Zod", func(rec *game.PlayerRecord) {
		rec.Points.Hit, rec.Points.Mana, rec.Points.Move = 1, 2, 3
		rec.LastLogon = time.Now().UTC().Add(-48 * time.Hour)
		rec.Affects = append(rec.Affects, game.Affect{
			Type: game.SpellPoison, Duration: 20, Bits: game.AffectPoison,
		})
	})

	back := dialClient(t, addr)
	back.login("Zod", "swordfish")
	if got := figuresOf(t, srv, "Zod"); got.hit != 1 || got.mana != 2 || got.move != 3 {
		t.Errorf("a poisoned character back after two days: %d hp, %d mana, %d mv; "+
			"want 1/2/3 unchanged", got.hit, got.mana, got.move)
	}
}

// TestLoadingRaisesMaximumManaToTheFloor is db.c:2254-2255, which runs on
// every load and before the affects go back on:
//
//	if (ch->points.max_mana < 100)
//	  ch->points.max_mana = 100;
//
// Both figures have to move. RealMaxMana is the base RecomputeAffects
// rebuilds the live value from, so raising only the live one would last
// until the first spell landed -- which is what the second half of this
// test is for.
func TestLoadingRaisesMaximumManaToTheFloor(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	first := dialClient(t, addr)
	first.create("Zod", "swordfish", "m", "m")
	first.send("quit")
	first.expect("Goodbye")
	waitForLogout(t, srv, "Zod")

	storedAs(t, srv, "Zod", func(rec *game.PlayerRecord) {
		rec.Points.MaxMana, rec.RealMaxMana = 40, 40
		rec.Points.Mana = 40
		// Recent, so the restore above cannot be what fills the pool.
		rec.LastLogon = time.Now().UTC()
	})

	back := dialClient(t, addr)
	back.login("Zod", "swordfish")
	if got := figuresOf(t, srv, "Zod"); got.maxMana != game.MinMaxMana {
		t.Errorf("maximum mana loaded as %d, want the floor of %d", got.maxMana, game.MinMaxMana)
	}

	back.send("cast 'armor' zod")
	back.expect("You feel someone protecting you.")
	if got := figuresOf(t, srv, "Zod"); got.maxMana != game.MinMaxMana {
		t.Errorf("maximum mana fell back to %d after a recompute, want the floor of %d "+
			"to have been written to the base as well", got.maxMana, game.MinMaxMana)
	}
}
