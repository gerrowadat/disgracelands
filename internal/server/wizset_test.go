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

// `set`, end to end.

// recordOf reads a character's record on the world goroutine.
func recordOf(t *testing.T, srv *Server, name string, f func(*game.PlayerRecord)) {
	t.Helper()
	inWorld(t, srv, func(w *game.Live) {
		who := w.Find(name)
		if who == nil || who.Record == nil {
			t.Errorf("%s is not in the world", name)
			return
		}
		f(who.Record)
	})
}

func TestSetNumbers(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	god := dialClient(t, addr)
	god.create("Adjuster", "turntheknobs", "m", "w")

	victim := dialClient(t, addr)
	victim.create("Adjusted", "beingchanged", "m", "w")
	setLevel(t, srv, "Adjusted", 10)

	god.send("set adjusted gold 5000")
	god.expect("Adjusted's gold set to 5000.")
	recordOf(t, srv, "Adjusted", func(rec *game.PlayerRecord) {
		if rec.Points.Gold != 5000 {
			t.Errorf("gold is %d, want 5000", rec.Points.Gold)
		}
	})

	// The ranges clamp — but the *acknowledgement lies*. perform_set builds
	// the message from the raw value before the switch runs, and RANGE
	// clamps inside it, so `set x gold -100` reports -100 and stores 0.
	// docs/weirdnumbers.md has the entry.
	god.send("set adjusted gold -100")
	god.expect("Adjusted's gold set to -100.")
	recordOf(t, srv, "Adjusted", func(rec *game.PlayerRecord) {
		if rec.Points.Gold != 0 {
			t.Errorf("gold is %d after being set to -100, want 0", rec.Points.Gold)
		}
	})

	// A mortal's abilities cap at 18, a greater god's at 25 — and again the
	// message reports what was asked for.
	god.send("set adjusted str 25")
	god.expect("Adjusted's str set to 25.")
	recordOf(t, srv, "Adjusted", func(rec *game.PlayerRecord) {
		if rec.RealAbilities.Strength != 18 {
			t.Errorf("strength is %d, want 18 for a mortal", rec.RealAbilities.Strength)
		}
	})

	// `hit` floors at -9, not 0: a god may set somebody *dying*. Only the
	// acknowledgement is asserted — a character left at -9 is at the mercy
	// of the next regeneration tick, so reading the stored value afterwards
	// is a race with the game rather than a test of the clamp.
	god.send("set adjusted hit -50")
	god.expect("Adjusted's hit set to -50.")
}

func TestSetBinaryFields(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	god := dialClient(t, addr)
	god.create("Flagger", "flipthebits", "m", "w")

	victim := dialClient(t, addr)
	victim.create("Flagged", "gettingflipped", "m", "w")
	setLevel(t, srv, "Flagged", 10)

	god.send("set flagged brief")
	god.expect("Value must be 'on' or 'off'.")

	god.send("set flagged brief on")
	god.expect("Brief ON for Flagged.")
	recordOf(t, srv, "Flagged", func(rec *game.PlayerRecord) {
		if !rec.Preferences.Has(game.PrefBrief) {
			t.Error("brief was not set")
		}
	})

	god.send("set flagged brief off")
	god.expect("Brief OFF for Flagged.")

	// `siteok` is the flag a `select` ban admits, and this is the only thing
	// that sets it.
	god.send("set flagged siteok on")
	god.expect("Siteok ON for Flagged.")
	recordOf(t, srv, "Flagged", func(rec *game.PlayerRecord) {
		if !rec.PlayerFlags.Has(game.PlayerSiteOK) {
			t.Error("siteok was not set")
		}
	})

	// `nosummon` is inverted: the flag is SUMMONABLE.
	god.send("set flagged nosummon on")
	god.expect("Nosummon OFF for Flagged.")
	recordOf(t, srv, "Flagged", func(rec *game.PlayerRecord) {
		if !rec.Preferences.Has(game.PrefSummonable) {
			t.Error("nosummon on did not set SUMMONABLE")
		}
	})
}

func TestSetMiscFields(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	god := dialClient(t, addr)
	god.create("Editor", "changethings", "m", "w")

	victim := dialClient(t, addr)
	victim.create("Edited", "beingedited", "m", "w")
	setLevel(t, srv, "Edited", 10)

	god.send("set edited title the Magnificent")
	god.expect("Edited's title is now: the Magnificent")

	// parse_class reads only the first letter, so `c` and `cleric` agree.
	god.send("set edited class c")
	god.expect("Okay.")
	recordOf(t, srv, "Edited", func(rec *game.PlayerRecord) {
		if rec.Class != game.ClassCleric {
			t.Errorf("class is %d, want cleric", rec.Class)
		}
	})

	god.send("set edited class zzz")
	god.expect("That is not a class.")

	god.send("set edited sex female")
	god.expect("Okay.")

	god.send("set edited sex sideways")
	god.expect("Must be 'male', 'female', or 'neutral'.")

	// The three conditions take a number or "off".
	god.send("set edited hunger 12")
	god.expect("Edited's hunger set to 12.")

	god.send("set edited hunger off")
	god.expect("Edited's hunger now off.")

	god.send("set edited hunger sideways")
	god.expect("Must be 'off' or a value from 0 to 24.")

	// `loadroom` takes a room or "off".
	god.send("set edited loadroom 3001")
	god.expect("Edited will enter at room #3001.")
	recordOf(t, srv, "Edited", func(rec *game.PlayerRecord) {
		if !rec.PlayerFlags.Has(game.PlayerLoadRoom) || rec.LoadRoom != MortalStartRoom {
			t.Errorf("the load room is %d, flagged %v", rec.LoadRoom,
				rec.PlayerFlags.Has(game.PlayerLoadRoom))
		}
	})

	god.send("set edited loadroom 99999")
	god.expect("That room does not exist!")

	god.send("set edited loadroom off")
	god.expect("Okay.")
}

// `set <name> room` moves them, which is a different thing from `teleport`
// only in that it does not announce itself.
func TestSetRoom(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	god := dialClient(t, addr)
	god.create("Mover", "putthemthere", "m", "w")

	victim := dialClient(t, addr)
	victim.create("Moved", "beingputthere", "m", "w")
	setLevel(t, srv, "Moved", 10)

	god.send("set moved room 3017")
	god.expect("Moved's room set to 3017.")
	if got := roomOf(t, srv, "Moved"); got != MageGuildRoom {
		t.Errorf("they are in %d, want the guild at %d", got, MageGuildRoom)
	}

	god.send("set moved room 99999")
	god.expect("No room exists with that number.")
}

// Each field carries its own level, on top of the command's.
func TestSetFieldsHaveTheirOwnLevels(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	first := dialClient(t, addr)
	first.create("Topmost", "thefirstgod", "m", "w")

	god := dialClient(t, addr)
	god.create("Middling", "onlyagod", "m", "w")
	setLevel(t, srv, "Middling", game.LevelGod)

	victim := dialClient(t, addr)
	victim.create("Ordinary", "justamortal", "m", "w")
	setLevel(t, srv, "Ordinary", 10)

	// `gold` is LVL_GOD and works.
	god.send("set ordinary gold 100")
	god.expect("Ordinary's gold set to 100.")

	// `maxhit` is LVL_GRGOD and does not.
	god.send("set ordinary maxhit 5000")
	god.expect("You are not godly enough for that!")

	// `level` is LVL_IMPL.
	god.send("set ordinary level 20")
	god.expectCount("You are not godly enough for that!", 2)
}

func TestSetRefusals(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	god := dialClient(t, addr)
	god.create("Careful", "mindtherules", "m", "w")

	dog := aMobile(t, srv, "Careful")
	if dog == nil {
		t.Fatal("no mobile")
	}

	god.send("set")
	god.expect("Usage: set <victim> <field> <value>")

	god.send("set nobodyatall gold 5")
	god.expect("There is no such player.")

	god.send("set careful nonsensefield 5")
	god.expect("Can't set that!")

	// A PC-only field on a mobile. The two messages are the way round they
	// look wrong: a *mobile* victim of a PC-only field is told "You can't do
	// that to a beast!".
	god.send("set dog bank 500")
	god.expect("You can't do that to a beast!")

	// You cannot freeze yourself.
	god.send("set careful frozen on")
	god.expect("Better not -- could be a long winter!")
}

// An implementor may set anybody, including somebody of their own level;
// a lesser god may not.
func TestSetRespectsRank(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	first := dialClient(t, addr)
	first.create("Impl", "thehighest", "m", "w")

	other := dialClient(t, addr)
	other.create("Equal", "sameasyou", "m", "w")
	setLevel(t, srv, "Equal", game.LevelGod)

	third := dialClient(t, addr)
	third.create("Rival", "alsoagod", "m", "w")
	setLevel(t, srv, "Rival", game.LevelGod)

	// An implementor may.
	first.send("set equal gold 10")
	first.expect("Equal's gold set to 10.")

	// A god may not touch another god of the same level.
	other.send("set rival gold 10")
	other.expect("Maybe that's not such a great idea...")

	// But may touch themselves.
	other.send("set equal gold 20")
	other.expect("Equal's gold set to 20.")
}

func TestMortalsCannotSet(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	first := dialClient(t, addr)
	first.create("Authority", "runstheplace", "m", "w")

	c := dialClient(t, addr)
	c.create("Powerless", "nothingtodo", "m", "w")
	setLevel(t, srv, "Powerless", 10)

	c.send("set powerless gold 1000000")
	c.expect("Huh?!?")
	recordOf(t, srv, "Powerless", func(rec *game.PlayerRecord) {
		if rec.Points.Gold >= 1000000 {
			t.Error("a mortal set their own gold")
		}
	})
}
