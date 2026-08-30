// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"context"
	"strings"
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
//
// Since #262 a remort also *makes* you that class, so undoing one has to go
// through a class you are not currently — you cannot un-remort what you are,
// which is the command's own first guard and always was. Bystander goes
// warrior → cleric → warrior, and only then is the cleric bit removable.
func TestRemortUndo(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	god, mortal := remortPair(t, srv, addr)

	god.send("remort Bystander cleric")
	god.expect("Bystander remorted to become a cleric!")

	god.send("remort Bystander warrior")
	god.expect("Bystander remorted to become a warrior!")

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

// TestRemortDoesTheHomework is issue #262: `remort` used to set a bit and
// stop, leaving an implementor to `set class`, `set level` and the rest by
// hand. It now leaves a character ready to play.
//
// The steps are the ones that were actually typed, recovered from the notes
// of whoever ran the game and quoted in game.Remort. TestRemortFollowsTheHandRunProcedure
// below checks each of them individually; this one checks the shape.
func TestRemortDoesTheHomework(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	god, mortal := remortPair(t, srv, addr)

	// Give Bystander a body worth losing, so "back to level one" is
	// something the test can actually see. Set directly rather than through
	// `set`: this test is about remort, and coupling it to another command's
	// wording would make it fail for the wrong reason.
	var beforeHit int32
	inWorld(t, srv, func(w *game.Live) {
		rec := w.Find("Bystander").Record
		rec.Level = 20
		rec.Points.MaxHit = 500
		rec.RealMaxHit = 500
		beforeHit = rec.Points.MaxHit
	})

	god.send("remort Bystander mage")
	god.expect("Bystander remorted to become a mage!")
	mortal.expect("you still are.")

	var level, maxHit int32
	var class game.Class
	var isWarrior, isMage bool
	inWorld(t, srv, func(w *game.Live) {
		rec := w.Find("Bystander").Record
		level, maxHit, class = rec.Level, rec.Points.MaxHit, rec.Class
		isWarrior, isMage = game.IsWarrior(rec), game.IsMagicUser(rec)
	})

	if class != game.ClassMagicUser {
		t.Errorf("class = %d, want ClassMagicUser: remort is supposed to set up the new class", class)
	}
	if level != 1 {
		t.Errorf("level = %d, want 1", level)
	}
	if maxHit >= beforeHit {
		t.Errorf("max hit = %d, was %d at level 20: the body did not go back with the level", maxHit, beforeHit)
	}
	// The whole point of the mechanic: the class you were is still yours.
	if !isWarrior {
		t.Error("Bystander stopped being a warrior; remorting is supposed to keep what you have been")
	}
	if !isMage {
		t.Error("Bystander is not a magic user, having just remorted into it")
	}
}

// TestPaladinTakesAWarriorAndACleric. Paladin is earned rather than chosen:
// the rule is that you must have been both, and the C never enforced it —
// `do_remort` has no such check, so it was kept by hand. #262.
func TestPaladinTakesAWarriorAndACleric(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	god, _ := remortPair(t, srv, addr)

	// Bystander is a warrior and nothing else yet.
	god.send("remort Bystander paladin")
	god.expect("has not earned paladinhood")

	var class game.Class
	inWorld(t, srv, func(w *game.Live) {
		class = w.Find("Bystander").Record.Class
	})
	if class == game.ClassPaladin {
		t.Fatal("the refusal let them through anyway")
	}

	// Become a cleric, which leaves the warrior bit set behind. Now both.
	god.send("remort Bystander cleric")
	god.expect("Bystander remorted to become a cleric!")

	god.send("remort Bystander paladin")
	god.expect("Bystander remorted to become a paladin!")

	inWorld(t, srv, func(w *game.Live) {
		class = w.Find("Bystander").Record.Class
	})
	if class != game.ClassPaladin {
		t.Errorf("class = %d, want ClassPaladin once both halves are in the vector", class)
	}
}

// TestAPaladinCanRemortOnward. Paladin is where remorting *arrives*, not
// where it stops: you can go on to another class afterwards, and the vector
// remembers the paladinhood — its mask is 16 (class.c:82) like every other
// class's. An earlier version of this command refused, on the mistaken
// grounds that paladin had no bit to be remembered by.
func TestAPaladinCanRemortOnward(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	god, _ := remortPair(t, srv, addr)

	god.send("remort Bystander cleric")
	god.expect("Bystander remorted to become a cleric!")
	god.send("remort Bystander paladin")
	god.expect("Bystander remorted to become a paladin!")

	god.send("remort Bystander thief")
	god.expect("Bystander remorted to become a thief!")

	var class game.Class
	var listed string
	inWorld(t, srv, func(w *game.Live) {
		class = w.Find("Bystander").Record.Class
	})
	if class != game.ClassThief {
		t.Errorf("class = %d, want ClassThief: a paladin may remort onward", class)
	}
	// And the listing says paladin, because the bit is set and saved.
	listed = god.transcript()
	if !strings.Contains(listed, "paladin") {
		t.Error("the class listing never mentions paladin, though the bit is set")
	}
}

// TestRemortIntoAClassAlreadyHeld. The C refused this — the bit was already
// set — and that guard is gone with #262: having been a mage before is no
// reason to refuse to *make* somebody a mage now, since remorting is a class
// change and not only a grant of abilities.
func TestRemortIntoAClassAlreadyHeld(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	god, _ := remortPair(t, srv, addr)

	god.send("remort Bystander mage")
	god.expect("Bystander remorted to become a mage!")

	// Now a mage with the warrior bit. Back to warrior, which they have
	// "already" been — the case the old second guard rejected.
	god.send("remort Bystander warrior")
	god.expect("Bystander remorted to become a warrior!")

	var class game.Class
	var isMage bool
	inWorld(t, srv, func(w *game.Live) {
		rec := w.Find("Bystander").Record
		class, isMage = rec.Class, game.IsMagicUser(rec)
	})
	if class != game.ClassWarrior {
		t.Errorf("class = %d, want ClassWarrior", class)
	}
	if !isMage {
		t.Error("the mage bit was lost on the way back to warrior")
	}
}

// TestUndoingARemortNobodyHadIsRefused. The C's XOR meant `remort x -mage`
// on somebody who had never been a mage *granted* it (docs/weirdnumbers.md).
// The guard added with #262 makes that unreachable through this command.
func TestUndoingARemortNobodyHadIsRefused(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	god, _ := remortPair(t, srv, addr)

	god.send("remort Bystander -mage")
	god.expect("But Bystander has never been a mage.")

	var isMage bool
	inWorld(t, srv, func(w *game.Live) {
		isMage = game.IsMagicUser(w.Find("Bystander").Record)
	})
	if isMage {
		t.Error("undoing a remort nobody had granted it; that is the XOR bug reaching a player")
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
		game.SetSpecFlags(rec, game.SpecFlagsOf(rec).With(game.PaladinFallen))
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

// TestRemortFollowsTheHandRunProcedure checks each line of the sequence a
// wizard used to type by hand (#262), because "does the homework" is only
// meaningful against a specific list of homework:
//
//	set player class whatever
//	set player lessons 0
//	advance player 1
//	set player maxmana 100
//	set player maxmove 100
//	(hp's should be okay)
//	set player prime-stat-from-previous-class 18
func TestRemortFollowsTheHandRunProcedure(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	god, _ := remortPair(t, srv, addr)

	// Bystander is a warrior (remortPair creates them "w"), so the previous
	// class's prime requisite is strength. Give them a body and a stack of
	// practices to lose, and a practised skill that must *not* be lost.
	inWorld(t, srv, func(w *game.Live) {
		rec := w.Find("Bystander").Record
		rec.Level = 20
		rec.SpellsToLearn = 30
		rec.Points.MaxMana = 400
		rec.Points.MaxMove = 400
		rec.RealMaxMana = 400
		rec.RealMaxMove = 400
		if rec.Skills == nil {
			rec.Skills = map[game.SpellID]int32{}
		}
		rec.Skills[game.SkillBash] = 75
	})

	god.send("remort Bystander cleric")
	god.expect("Bystander remorted to become a cleric!")

	var (
		class                    game.Class
		level, lessons           int32
		maxMana, maxMove         int32
		strength, strPct, wisdom int32
		bash                     int32
		isWarrior, isCleric      bool
	)
	inWorld(t, srv, func(w *game.Live) {
		rec := w.Find("Bystander").Record
		class, level, lessons = rec.Class, rec.Level, rec.SpellsToLearn
		maxMana, maxMove = rec.Points.MaxMana, rec.Points.MaxMove
		strength, strPct = rec.Abilities.Strength, rec.Abilities.StrengthPercentile
		wisdom = rec.Abilities.Wisdom
		bash = rec.Skills[game.SkillBash]
		isWarrior, isCleric = game.IsWarrior(rec), game.IsCleric(rec)
	})

	// set player class whatever
	if class != game.ClassCleric {
		t.Errorf("class = %d, want ClassCleric", class)
	}
	// set player lessons 0 — and then `advance player 1`, which grants one
	// level's worth on the way past (AdvanceLevel's own `SpellsToLearn +=`).
	// So the number to expect is not zero: it is "the thirty are gone and a
	// level-one character's allowance is there", which is what that order of
	// operations produces and what the wizard typing it got.
	if lessons == 0 || lessons > 10 {
		t.Errorf("lessons = %d, want the banked thirty gone and one level's worth granted", lessons)
	}
	// advance player 1
	if level != 1 {
		t.Errorf("level = %d, want 1", level)
	}
	// set player maxmana 100 / set player maxmove 100
	if maxMana != 100 {
		t.Errorf("max mana = %d, want 100", maxMana)
	}
	if maxMove != 100 {
		t.Errorf("max move = %d, want 100 (the notes say 100, not init_char's 82)", maxMove)
	}
	// set player prime-stat-from-previous-class 18 — Bystander was a
	// warrior, so strength, and `set str` clears the percentile with it.
	if strength != 18 {
		t.Errorf("strength = %d, want 18: the previous class's prime requisite is pinned so the old class stays worth having", strength)
	}
	if strPct != 0 {
		t.Errorf("strength percentile = %d, want 0: `set str 18` is 18/00", strPct)
	}
	// And the roll happened for the *new* class, so wisdom — a cleric's own
	// prime requisite — got the best of the six.
	if wisdom < 12 {
		t.Errorf("wisdom = %d, suspiciously low for a freshly rolled cleric: was the roll done for the old class?", wisdom)
	}
	// do_start has no memset: a practised skill survives it, and losing
	// every skill on a remort would defeat the mechanic entirely.
	if bash != 75 {
		t.Errorf("bash = %d, want 75: do_start does not wipe the skills array (utils.h:325 sets one element)", bash)
	}
	if !isWarrior || !isCleric {
		t.Errorf("warrior=%v cleric=%v, want both", isWarrior, isCleric)
	}
}
