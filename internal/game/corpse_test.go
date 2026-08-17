// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import (
	"strings"
	"testing"
)

// TestMakingACorpse takes everything the character had.
func TestMakingACorpse(t *testing.T) {
	l := objectWorld()
	welmar := newCharacter("Welmar")
	welmar.Record.Points.Gold = 250
	if err := l.Enter(welmar, 3001); err != nil {
		t.Fatal(err)
	}

	carried := l.NewObject(101)
	l.ObjectToChar(carried, welmar)
	wielded := l.NewObject(100)
	l.ObjectToChar(wielded, welmar)
	if !l.Equip(wielded, welmar, WearWield) {
		t.Fatal("could not wield the sword")
	}

	corpse := l.MakeCorpse(welmar)

	if !IsCorpse(corpse) {
		t.Error("the corpse does not test as one")
	}
	if corpse.ShortDesc != "the corpse of Welmar" {
		t.Errorf("short description is %q", corpse.ShortDesc)
	}
	if !strings.Contains(corpse.Description, "Welmar") {
		t.Errorf("room description is %q", corpse.Description)
	}
	if corpse.Timer != PlayerCorpseTime {
		t.Errorf("timer is %d, want a player's %d", corpse.Timer, PlayerCorpseTime)
	}
	if !corpse.Takeable() {
		t.Error("a corpse should be takeable")
	}
	// Capacity zero: you take things out of a corpse, you do not put them in.
	if corpse.Values[0] != 0 {
		t.Errorf("capacity is %d, want 0", corpse.Values[0])
	}

	// Everything they had is inside, and nothing is left on them.
	if len(welmar.Carrying) != 0 {
		t.Errorf("the body is still carrying %d things", len(welmar.Carrying))
	}
	for pos, o := range welmar.Equipment {
		if o != nil {
			t.Errorf("the body is still wearing %s at slot %d", o.Name(), pos)
		}
	}
	if welmar.Record.Points.Gold != 0 {
		t.Errorf("the body still has %d gold", welmar.Record.Points.Gold)
	}

	if len(corpse.Contents) != 3 {
		t.Fatalf("the corpse holds %d things, want the bag, the sword and the coins", len(corpse.Contents))
	}
	for _, o := range corpse.Contents {
		assertOnePlace(t, l, o)
	}

	var gold *Object
	for _, o := range corpse.Contents {
		if o.Type == ItemMoney {
			gold = o
		}
	}
	if gold == nil {
		t.Fatal("the corpse holds no coins")
	}
	if gold.Values[0] != 250 {
		t.Errorf("the pile is %d coins, want 250", gold.Values[0])
	}

	// And the corpse is on the floor where they fell.
	assertOnePlace(t, l, corpse)
	if corpse.Location != InRoom || corpse.Room != 3001 {
		t.Errorf("the corpse is %v in room %d", corpse.Location, corpse.Room)
	}
}

// TestACorpseWeighsWhatTheBodyDid, body and belongings together.
func TestACorpseWeighsWhatTheBodyDid(t *testing.T) {
	l := objectWorld()
	welmar := newCharacter("Welmar") // 150 lbs
	if err := l.Enter(welmar, 3001); err != nil {
		t.Fatal(err)
	}

	sword := l.NewObject(100) // 10
	l.ObjectToChar(sword, welmar)

	corpse := l.MakeCorpse(welmar)
	if corpse.Weight != 160 {
		t.Errorf("the corpse weighs %d, want 160", corpse.Weight)
	}
}

// TestAMobileCorpseRotsFaster, which is the only thing config.c distinguishes.
func TestAMobileCorpseRotsFaster(t *testing.T) {
	l := objectWorld()
	mob := newCharacter("a large dog")
	mob.NPC = true
	if err := l.Enter(mob, 3001); err != nil {
		t.Fatal(err)
	}

	corpse := l.MakeCorpse(mob)
	if corpse.Timer != NPCCorpseTime {
		t.Errorf("timer is %d, want a mobile's %d", corpse.Timer, NPCCorpseTime)
	}
	if NPCCorpseTime >= PlayerCorpseTime {
		t.Error("a mobile's corpse should not outlast a player's")
	}
}

// TestACorpseDecaysAndSpills. The contents go on the floor rather than
// vanishing with it, which is what makes dying survivable.
func TestACorpseDecaysAndSpills(t *testing.T) {
	l := objectWorld()
	welmar := newCharacter("Welmar")
	if err := l.Enter(welmar, 3001); err != nil {
		t.Fatal(err)
	}

	sword := l.NewObject(100)
	l.ObjectToChar(sword, welmar)
	corpse := l.MakeCorpse(welmar)

	// Not yet.
	for i := int32(1); i < PlayerCorpseTime; i++ {
		if decayed := l.DecayObjects(); len(decayed) != 0 {
			t.Fatalf("the corpse decayed after %d ticks, want %d", i, PlayerCorpseTime)
		}
	}

	decayed := l.DecayObjects()
	if len(decayed) != 1 {
		t.Fatalf("%d objects decayed, want 1", len(decayed))
	}
	if decayed[0].Corpse != corpse || decayed[0].Room != 3001 {
		t.Errorf("decayed %+v, want the corpse in room 3001", decayed[0])
	}

	floor := l.RoomObjects(3001)
	if len(floor) != 1 || floor[0] != sword {
		t.Fatalf("the floor holds %d things, want the sword the corpse spilled", len(floor))
	}
	assertOnePlace(t, l, sword)

	// And it is not reported a second time.
	if decayed := l.DecayObjects(); len(decayed) != 0 {
		t.Errorf("the corpse decayed twice: %+v", decayed)
	}
}

// TestACarriedCorpseDecaysInYourHands, which the C says out loud.
func TestACarriedCorpseDecaysInYourHands(t *testing.T) {
	l := objectWorld()
	welmar := newCharacter("Welmar")
	looter := newCharacter("Zod")
	if err := l.Enter(welmar, 3001); err != nil {
		t.Fatal(err)
	}
	if err := l.Enter(looter, 3001); err != nil {
		t.Fatal(err)
	}

	sword := l.NewObject(100)
	l.ObjectToChar(sword, welmar)
	corpse := l.MakeCorpse(welmar)
	l.ObjectToChar(corpse, looter)

	var decayed []DecayResult
	for i := int32(0); i < PlayerCorpseTime; i++ {
		decayed = l.DecayObjects()
	}

	if len(decayed) != 1 || decayed[0].CarriedBy != looter {
		t.Fatalf("decayed %+v, want it in Zod's hands", decayed)
	}
	// The contents end up with whoever was holding it.
	if len(looter.Carrying) != 1 || looter.Carrying[0] != sword {
		t.Errorf("Zod is carrying %d things, want the sword", len(looter.Carrying))
	}
}

// TestOnlyCorpsesDecay: nothing else in the stock game has a timer.
func TestOnlyCorpsesDecay(t *testing.T) {
	l := objectWorld()

	sword := l.NewObject(100)
	sword.Timer = 1
	l.ObjectToRoom(sword, 3001)

	for i := 0; i < 5; i++ {
		if decayed := l.DecayObjects(); len(decayed) != 0 {
			t.Fatalf("a sword decayed: %+v", decayed)
		}
	}
	if sword.Timer != 1 {
		t.Errorf("the sword's timer moved to %d", sword.Timer)
	}
}

func TestMakeMoney(t *testing.T) {
	l := objectWorld()

	one := l.MakeMoney(1)
	if one.ShortDesc != "a gold coin" {
		t.Errorf("one coin is %q", one.ShortDesc)
	}
	if !strings.Contains(one.Description, "One miserable gold coin") {
		t.Errorf("one coin on the floor is %q", one.Description)
	}

	many := l.MakeMoney(250)
	if many.ShortDesc != "250 gold coins" {
		t.Errorf("250 coins is %q", many.ShortDesc)
	}
	if many.Values[0] != 250 {
		t.Errorf("the pile holds %d, want 250", many.Values[0])
	}
	if !many.Takeable() {
		t.Error("coins should be takeable")
	}
}

// TestAPennilessCorpseHoldsNoCoins.
func TestAPennilessCorpseHoldsNoCoins(t *testing.T) {
	l := objectWorld()
	welmar := newCharacter("Welmar")
	if err := l.Enter(welmar, 3001); err != nil {
		t.Fatal(err)
	}

	corpse := l.MakeCorpse(welmar)
	if len(corpse.Contents) != 0 {
		t.Errorf("a corpse with nothing on it holds %d things", len(corpse.Contents))
	}
}
