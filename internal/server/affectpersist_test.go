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

// figures is the handful of numbers RecomputeAffects derives, read off a
// character in the world.
type figures struct {
	maxHit, maxMana, maxMove int32
	hit, mana, move          int32
	str, con                 int32
	armor                    int32
}

func figuresOf(t *testing.T, srv *Server, name string) figures {
	t.Helper()
	var f figures
	var found bool
	inWorld(t, srv, func(w *game.Live) {
		c := w.Find(name)
		if c == nil || c.Record == nil {
			return
		}
		found = true
		r := c.Record
		f = figures{
			maxHit: r.Points.MaxHit, maxMana: r.Points.MaxMana, maxMove: r.Points.MaxMove,
			hit: r.Points.Hit, mana: r.Points.Mana, move: r.Points.Move,
			str: r.Abilities.Strength, con: r.Abilities.Constitution,
			armor: r.Points.Armor,
		}
	})
	if !found {
		t.Fatalf("%s is not in the world", name)
	}
	return f
}

// TestAnAffectOnALoadedCharacterKeepsTheirFigures is the regression for a
// character logging in and being emptied by the first spell they cast.
//
// A PlayerRecord holds each derived figure twice — the live one and the
// unaffected one RecomputeAffects rebuilds it from — and nothing on the way
// in from disk ever populated the second. A loaded character therefore held
// correct numbers and a base of zero, and stayed that way right up until
// something recomputed: `cast 'armor'` on yourself set max hit points, mana,
// movement and every ability to nothing at once, and sleeping did not help
// because regeneration clamps to a maximum that is now zero.
//
// The fix is game.SnapshotReal at the end of every store's decode, and
// RecomputeAffects once in Authenticate — store_to_char's own two halves
// (db.c:2245-2246, 2270-2273). This test is deliberately end to end: it is
// the login that was missing them, so a unit test of either half would have
// passed while the server stayed broken.
func TestAnAffectOnALoadedCharacterKeepsTheirFigures(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	first := dialClient(t, addr)
	first.create("Zod", "swordfish", "m", "m")
	before := figuresOf(t, srv, "Zod")
	first.send("quit")
	first.expect("Goodbye")
	waitForLogout(t, srv, "Zod")

	// Back in as an existing character, which is the path that was broken:
	// creation snapshots the real values, loading did not.
	second := dialClient(t, addr)
	second.login("Zod", "swordfish")

	atLogin := figuresOf(t, srv, "Zod")
	if atLogin.maxHit != before.maxHit || atLogin.maxMana != before.maxMana || atLogin.maxMove != before.maxMove {
		t.Errorf("logging back in changed the pools: got %+v, want %+v", atLogin, before)
	}
	if atLogin.str != before.str || atLogin.con != before.con {
		t.Errorf("logging back in changed the abilities: got %+v, want %+v", atLogin, before)
	}

	second.send("cast 'armor' zod")
	second.expect("You feel someone protecting you.")

	after := figuresOf(t, srv, "Zod")
	if after.maxHit != before.maxHit || after.maxMana != before.maxMana || after.maxMove != before.maxMove {
		t.Errorf("armor changed the pools: got %+v, want %+v", after, before)
	}
	// Not compared against before: Zod is the first character and so an
	// implementor, rolled with 25s that affect_total clamps to 18 the first
	// time it runs (docs/weirdnumbers.md, and the C does the same at the
	// same moment). The bug being guarded against left these at *zero*.
	if after.str < 18 || after.con < 18 {
		t.Errorf("armor emptied the abilities: got %+v", after)
	}
	if after.hit <= 0 || after.mana <= 0 || after.move <= 0 {
		t.Errorf("armor emptied a pool: %+v", after)
	}
	// And it did land: armor is a flat -20 to armour class (affectspells.go,
	// spell_armor), so a spell that changed nothing at all would pass every
	// assertion above.
	if want := before.armor - 20; after.armor != want {
		t.Errorf("armour class is %d, want %d", after.armor, want)
	}
}

// TestASavedAffectIsNotAppliedTwice is the other half, and the reason the
// save path writes game.BaseRecord rather than the live record.
//
// char_to_store strips a character before writing them, with the comment
// "remove the affections so that the raw values are stored; otherwise the
// effects are doubled when the char logs back in" (db.c:2319-2324). Saving
// the affected figures instead makes them the base the next login applies
// the same affect to — so a character who quits under armor comes back at
// -40, then -60, for as long as the spell keeps outliving the logout.
func TestASavedAffectIsNotAppliedTwice(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	first := dialClient(t, addr)
	first.create("Zod", "swordfish", "m", "m")
	base := figuresOf(t, srv, "Zod")

	first.send("cast 'armor' zod")
	first.expect("You feel someone protecting you.")
	affected := figuresOf(t, srv, "Zod")
	if want := base.armor - 20; affected.armor != want {
		t.Fatalf("armour class is %d, want %d", affected.armor, want)
	}

	// Armor lasts 24 ticks, so it is still on the record that gets written.
	first.send("quit")
	first.expect("Goodbye")
	waitForLogout(t, srv, "Zod")

	second := dialClient(t, addr)
	second.login("Zod", "swordfish")

	back := figuresOf(t, srv, "Zod")
	if back.armor != affected.armor {
		t.Errorf("armour class after a relog is %d, want %d (base %d)",
			back.armor, affected.armor, base.armor)
	}
	if back.maxHit != base.maxHit || back.maxMana != base.maxMana || back.maxMove != base.maxMove {
		t.Errorf("the pools drifted across a relog: got %+v, want %+v", back, base)
	}
}
