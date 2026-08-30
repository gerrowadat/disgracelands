// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import "testing"

// seeWorld is darkWorld's lit room and its dark cellar, with two characters in
// whichever one the test wants.
func seePair(t *testing.T, room RoomVnum) (*Live, *Character, *Character) {
	t.Helper()
	l := darkWorld(t)
	atHour(l, 12)
	return l, inRoom(t, l, "Watcher", room), inRoom(t, l, "Target", room)
}

// TestYouAlwaysSeeYourself. SELF is the first test in CAN_SEE and it beats
// everything: blind, invisible, in the dark, all of it.
func TestYouAlwaysSeeYourself(t *testing.T) {
	l, watcher, _ := seePair(t, 3003) // the dark cellar
	watcher.Record.AffectFlags = watcher.Record.AffectFlags.
		Set(AffectBlind | AffectInvisible | AffectHide)

	if !l.CanSee(watcher, watcher) {
		t.Error("a blind invisible hiding character could not see themselves")
	}
}

// TestDarknessStopsYouSeeingPeople, and infravision restores it. This is
// LIGHT_OK, which takes infravision and not holylight.
func TestDarknessStopsYouSeeingPeople(t *testing.T) {
	l, watcher, target := seePair(t, 3003)

	if l.CanSee(watcher, target) {
		t.Error("saw somebody in a pitch dark room")
	}

	watcher.Record.AffectFlags = watcher.Record.AffectFlags.Set(AffectInfravision)
	if !l.CanSee(watcher, target) {
		t.Error("infravision did not show somebody in the dark")
	}
}

// TestALitTorchLetsYouSee ties the two halves together: the light count is
// what makes the room lit, and the room being lit is what LIGHT_OK asks.
func TestALitTorchLetsYouSee(t *testing.T) {
	l, watcher, target := seePair(t, 3003)

	if l.CanSee(watcher, target) {
		t.Fatal("saw somebody in a pitch dark room")
	}
	target.Equipment[WearLight] = l.NewObject(3040)
	if !l.CanSee(watcher, target) {
		t.Error("the target's own torch did not light the room")
	}
}

// TestBlindnessStopsYouSeeingInDaylight.
func TestBlindnessStopsYouSeeingInDaylight(t *testing.T) {
	l, watcher, target := seePair(t, 3001) // the lit temple

	if !l.CanSee(watcher, target) {
		t.Fatal("could not see somebody in a lit room")
	}
	watcher.Record.AffectFlags = watcher.Record.AffectFlags.Set(AffectBlind)
	if l.CanSee(watcher, target) {
		t.Error("a blind character could see")
	}
	// Infravision is no help against blindness: LIGHT_OK tests AFF_BLIND
	// first and returns.
	watcher.Record.AffectFlags = watcher.Record.AffectFlags.Set(AffectInfravision)
	if l.CanSee(watcher, target) {
		t.Error("infravision cured blindness")
	}
}

// TestInvisibilityAndItsCounter.
func TestInvisibilityAndItsCounter(t *testing.T) {
	l, watcher, target := seePair(t, 3001)

	target.Record.AffectFlags = target.Record.AffectFlags.Set(AffectInvisible)
	if l.CanSee(watcher, target) {
		t.Error("saw an invisible character")
	}
	watcher.Record.AffectFlags = watcher.Record.AffectFlags.Set(AffectDetectInvis)
	if !l.CanSee(watcher, target) {
		t.Error("detect invisible did not defeat invisibility")
	}
}

// TestHidingAndItsCounter. Note the pairing: hiding is beaten by *sense life*,
// not by detect invisible, and the two do not cross over.
func TestHidingAndItsCounter(t *testing.T) {
	l, watcher, target := seePair(t, 3001)

	target.Record.AffectFlags = target.Record.AffectFlags.Set(AffectHide)
	if l.CanSee(watcher, target) {
		t.Error("saw a hidden character")
	}

	watcher.Record.AffectFlags = watcher.Record.AffectFlags.Set(AffectDetectInvis)
	if l.CanSee(watcher, target) {
		t.Error("detect invisible found a hidden character; sense life is what does that")
	}

	watcher.Record.AffectFlags = watcher.Record.AffectFlags.Set(AffectSenseLife)
	if !l.CanSee(watcher, target) {
		t.Error("sense life did not find a hidden character")
	}
}

// TestSneakingDoesNotConcealYou. AFF_SNEAK is not in INVIS_OK at all: it hides
// your *movement*, not you, so somebody sneaking in front of you is plainly
// visible. Easy to assume otherwise, since the three are granted together.
func TestSneakingDoesNotConcealYou(t *testing.T) {
	l, watcher, target := seePair(t, 3001)

	target.Record.AffectFlags = target.Record.AffectFlags.Set(AffectSneak)
	if !l.CanSee(watcher, target) {
		t.Error("a sneaking character was invisible standing still")
	}
}

// TestHolylightSeesEverything — darkness, invisibility and hiding at once, in
// one step, because IMM_CAN_SEE puts it beside the whole mortal test rather
// than inside it.
func TestHolylightSeesEverything(t *testing.T) {
	l, watcher, target := seePair(t, 3003)
	target.Record.AffectFlags = target.Record.AffectFlags.Set(AffectInvisible | AffectHide)

	if l.CanSee(watcher, target) {
		t.Fatal("saw an invisible hidden character in the dark")
	}
	watcher.Record.Preferences = watcher.Record.Preferences.Set(PrefHolylight)
	if !l.CanSee(watcher, target) {
		t.Error("holylight did not see through darkness, invisibility and hiding")
	}
}

// TestHolylightDoesNotDefeatInvisLevel is the one thing it cannot do, and the
// reason is structural: the invis-level test sits *outside* IMM_CAN_SEE, so
// holylight never reaches it.
func TestHolylightDoesNotDefeatInvisLevel(t *testing.T) {
	l, watcher, target := seePair(t, 3001)
	watcher.Record.Preferences = watcher.Record.Preferences.Set(PrefHolylight)
	watcher.Record.Level = 32
	target.Record.Level = 34
	target.Record.InvisLevel = 34

	if l.CanSee(watcher, target) {
		t.Error("a lesser god saw a greater one who was invis, through holylight")
	}

	// Equal to the invis level is enough: the test is `<`.
	watcher.Record.Level = 34
	if !l.CanSee(watcher, target) {
		t.Error("an equal-level god could not see an invis god")
	}
}

// TestAMobileHasNoInvisLevel. GET_INVIS_LEV reads player_specials, so the C
// writes `IS_NPC(obj) ? 0 :` out longhand — a mobile can never be
// invis-levelled, whatever its record happens to hold.
func TestAMobileHasNoInvisLevel(t *testing.T) {
	l, watcher, target := seePair(t, 3001)
	target.NPC = true
	target.Record.InvisLevel = 34
	watcher.Record.Level = 1

	if !l.CanSee(watcher, target) {
		t.Error("a mobile's invis level hid it from a mortal")
	}
}

// TestLightOkAsksTheSubjectsRoom, not the object's — so standing in the dark
// stops you seeing somebody standing in daylight, which is what makes `where`
// useless to a mortal in an unlit cave.
func TestLightOkAsksTheSubjectsRoom(t *testing.T) {
	l := darkWorld(t)
	atHour(l, 12)
	watcher := inRoom(t, l, "Watcher", 3003) // dark cellar
	target := inRoom(t, l, "Target", 3001)   // lit temple

	if l.CanSee(watcher, target) {
		t.Error("somebody in the dark saw somebody in the light")
	}
	if !l.CanSee(target, watcher) {
		t.Error("somebody in the light could not see somebody in the dark")
	}
}

// TestPers is the name substitution act() uses per audience.
func TestPers(t *testing.T) {
	l, watcher, target := seePair(t, 3001)

	if got := l.Pers(target, watcher); got != "Target" {
		t.Errorf("Pers = %q, want %q", got, "Target")
	}
	target.Record.AffectFlags = target.Record.AffectFlags.Set(AffectInvisible)
	if got := l.Pers(target, watcher); got != "someone" {
		t.Errorf("Pers of an invisible character = %q, want %q", got, "someone")
	}
	// And they still know their own name.
	if got := l.Pers(target, target); got != "Target" {
		t.Errorf("Pers of yourself = %q, want %q", got, "Target")
	}
}

// TestAnObjectHeldBySomebodyUnseenIsUnseen is CAN_SEE_OBJ_CARRIER, and it is
// the part of object visibility that is not a property of the object.
func TestAnObjectHeldBySomebodyUnseenIsUnseen(t *testing.T) {
	l, watcher, target := seePair(t, 3001)

	sword := l.NewObject(3043)
	l.ObjectToChar(sword, target)
	if !l.CanSeeObj(watcher, sword) {
		t.Fatal("could not see an ordinary sword in somebody's hands")
	}

	target.Record.AffectFlags = target.Record.AffectFlags.Set(AffectInvisible)
	if l.CanSeeObj(watcher, sword) {
		t.Error("an invisible character's sword was still visible")
	}

	// On the floor it has no carrier, so it comes back into view.
	l.ObjectToRoom(sword, 3001)
	if !l.CanSeeObj(watcher, sword) {
		t.Error("a sword on the floor was hidden by its former owner")
	}
}

// TestAnInvisibleObject.
func TestAnInvisibleObject(t *testing.T) {
	l, watcher, _ := seePair(t, 3001)

	sword := l.NewObject(3043)
	l.ObjectToRoom(sword, 3001)
	sword.ExtraFlags = sword.ExtraFlags.With(ItemInvisible)

	if l.CanSeeObj(watcher, sword) {
		t.Error("saw an invisible object")
	}
	watcher.Record.AffectFlags = watcher.Record.AffectFlags.Set(AffectDetectInvis)
	if !l.CanSeeObj(watcher, sword) {
		t.Error("detect invisible did not reveal an invisible object")
	}
}

// TestObjsSubstitution.
func TestObjsSubstitution(t *testing.T) {
	l, watcher, _ := seePair(t, 3001)

	sword := l.NewObject(3043)
	l.ObjectToRoom(sword, 3001)
	if got := l.Objs(sword, watcher); got != "a sword" {
		t.Errorf("Objs = %q, want %q", got, "a sword")
	}
	sword.ExtraFlags = sword.ExtraFlags.With(ItemInvisible)
	if got := l.Objs(sword, watcher); got != "something" {
		t.Errorf("Objs of an invisible object = %q, want %q", got, "something")
	}
}

// TestRealLevelWithoutASession is the ordinary case: no client, so no switch,
// so the character's own level.
func TestRealLevelWithoutASession(t *testing.T) {
	ch := newCharacter("Zod")
	ch.Record.Level = 21
	if got := ch.RealLevel(); got != 21 {
		t.Errorf("RealLevel = %d, want 21", got)
	}
}

// switchedClient is a game.Client that reports being switched, standing in for
// a session without pulling the session package in here.
type switchedClient struct{ level int32 }

func (switchedClient) Send(string, ...any)                {}
func (s switchedClient) SwitchedFromLevel() (int32, bool) { return s.level, true }

// TestRealLevelWhileSwitched. A god switched into a rat keeps their own level
// for the invis-level test and nothing else — which is the only reason
// GET_REAL_LEVEL exists.
func TestRealLevelWhileSwitched(t *testing.T) {
	l, _, target := seePair(t, 3001)

	rat := inRoom(t, l, "Rat", 3001)
	rat.NPC = true
	rat.Record.Level = 2
	rat.Client = switchedClient{level: 34}

	target.Record.Level = 33
	target.Record.InvisLevel = 33

	if !l.CanSee(rat, target) {
		t.Error("a god switched into a rat lost sight of a lesser invis god")
	}
	// The body's own level is still 2, and that is what everything else uses.
	if got := rat.Level(); got != 2 {
		t.Errorf("the switched body's Level = %d, want 2", got)
	}
}
