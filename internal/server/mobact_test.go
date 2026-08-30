// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"context"
	"testing"
	"time"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// mobile puts a mobile into a room with the given flags.
func mobile(t *testing.T, srv *Server, name string, flags game.Flags, room game.RoomVnum) *game.Character {
	t.Helper()

	c := &game.Character{
		Name:     name,
		Keywords: name,
		NPC:      true,
		Position: game.PosStanding,
		MobDef:   &game.MobDef{Vnum: 999, ShortDesc: name, Keywords: name, ActionFlags: flags},
		Record: &game.PlayerRecord{
			Name: name, Level: 10, Birth: time.Now(),
			Points: game.Points{Hit: 100, MaxHit: 100},
		},
	}
	if err := srv.engine.DoSync(context.Background(), func(w *game.Live) {
		if err := w.Enter(c, room); err != nil {
			t.Errorf("placing the mobile: %v", err)
		}
		w.Track(c)
	}); err != nil {
		t.Fatal(err)
	}
	return c
}

// mobTick runs one mobile-activity pass.
func mobTick(t *testing.T, srv *Server) {
	t.Helper()
	if err := srv.engine.DoSync(context.Background(), srv.mobileActivity); err != nil {
		t.Fatalf("running mobile activity: %v", err)
	}
}

// TestASentinelStaysPut, however many pulses go by.
func TestASentinelStaysPut(t *testing.T) {
	srv, _ := newTestServer(t)
	mob := mobile(t, srv, "a shopkeeper", game.MobSentinel, MortalStartRoom)

	for i := 0; i < 200; i++ {
		mobTick(t, srv)
	}
	if mob.Room != MortalStartRoom {
		t.Errorf("a sentinel wandered to room %d", mob.Room)
	}
}

// TestAnOrdinaryMobileWanders, given enough pulses. The roll is one number
// from nineteen against six directions, so it moves about a third of the
// time it is considered.
func TestAnOrdinaryMobileWanders(t *testing.T) {
	srv, _ := newTestServer(t)
	mob := mobile(t, srv, "a large dog", 0, MortalStartRoom)

	var moved bool
	for i := 0; i < 300 && !moved; i++ {
		mobTick(t, srv)
		moved = mob.Room != MortalStartRoom
	}
	if !moved {
		t.Error("a mobile with no flags never wandered in 300 pulses")
	}
}

// TestAMobileWillNotWalkIntoANoMobRoom.
func TestAMobileWillNotWalkIntoANoMobRoom(t *testing.T) {
	srv, _ := newTestServer(t)

	if err := srv.engine.DoSync(context.Background(), func(w *game.Live) {
		w.Room(ImmortStartRoom).Flags = game.NewSet(game.RoomNoMob)
	}); err != nil {
		t.Fatal(err)
	}

	mob := mobile(t, srv, "a large dog", 0, MortalStartRoom)
	for i := 0; i < 400; i++ {
		mobTick(t, srv)
		if mob.Room != MortalStartRoom {
			t.Fatalf("a mobile walked into a NOMOB room (%d)", mob.Room)
		}
	}
}

// TestAClosedDoorStopsAMobile.
func TestAClosedDoorStopsAMobile(t *testing.T) {
	srv, _ := newTestServer(t)

	if err := srv.engine.DoSync(context.Background(), func(w *game.Live) {
		exit := w.Room(MortalStartRoom).Exits[game.North]
		exit.State = exit.State.With(game.ExitIsDoor, game.ExitClosed)
	}); err != nil {
		t.Fatal(err)
	}

	mob := mobile(t, srv, "a large dog", 0, MortalStartRoom)
	for i := 0; i < 400; i++ {
		mobTick(t, srv)
		if mob.Room != MortalStartRoom {
			t.Fatal("a mobile walked through a closed door")
		}
	}
}

// TestAnAggressiveMobileAttacks a player who walks in.
func TestAnAggressiveMobileAttacks(t *testing.T) {
	srv, _ := newTestServer(t)

	mob := mobile(t, srv, "a guard dog", game.MobSentinel|game.MobAggressive, MortalStartRoom)
	victim, victimClient := place(t, srv, fighterRecord("Welmar", 5, 200), MortalStartRoom)

	mobTick(t, srv)

	if mob.Fighting != victim {
		t.Fatal("an aggressive mobile did not attack the player in its room")
	}
	if !victimClient.said("a guard dog attacks you!") {
		t.Error("the victim was not told")
	}
	if victim.Fighting != nil {
		// The victim fights back on the first blow, not on the declaration.
		t.Log("the victim is already fighting back")
	}
}

// TestAnAggressiveMobileIgnoresOtherMobiles, or the world would tear itself
// apart on the first pulse.
func TestAnAggressiveMobileIgnoresOtherMobiles(t *testing.T) {
	srv, _ := newTestServer(t)

	aggressor := mobile(t, srv, "a guard dog", game.MobSentinel|game.MobAggressive, MortalStartRoom)
	mobile(t, srv, "a rabbit", game.MobSentinel, MortalStartRoom)

	for i := 0; i < 20; i++ {
		mobTick(t, srv)
	}
	if aggressor.Fighting != nil {
		t.Errorf("an aggressive mobile attacked %s", aggressor.Fighting.Name)
	}
}

// TestNoHassleWalksThroughUnmolested, which is how an immortal inspects a
// zone.
func TestNoHassleWalksThroughUnmolested(t *testing.T) {
	srv, _ := newTestServer(t)

	mob := mobile(t, srv, "a guard dog", game.MobSentinel|game.MobAggressive, MortalStartRoom)
	rec := fighterRecord("Zod", 34, 500)
	rec.Preferences = rec.Preferences.Set(game.PrefNoHassle)
	place(t, srv, rec, MortalStartRoom)

	for i := 0; i < 20; i++ {
		mobTick(t, srv)
	}
	if mob.Fighting != nil {
		t.Error("an aggressive mobile attacked somebody with nohassle set")
	}
}

// TestAlignmentSelectiveAggression: each of the three flags picks its own.
func TestAlignmentSelectiveAggression(t *testing.T) {
	for _, tc := range []struct {
		name      string
		flag      game.Flags
		alignment int32
		attacks   bool
	}{
		{"evil-hater meets an evil player", game.MobAggrEvil, -1000, true},
		{"evil-hater meets a good player", game.MobAggrEvil, 1000, false},
		{"evil-hater meets a neutral player", game.MobAggrEvil, 0, false},
		{"good-hater meets a good player", game.MobAggrGood, 1000, true},
		{"good-hater meets an evil player", game.MobAggrGood, -1000, false},
		{"neutral-hater meets a neutral player", game.MobAggrNeutral, 0, true},
		{"neutral-hater meets a good player", game.MobAggrNeutral, 1000, false},
		// The boundaries: 350 is good, 349 is not; -350 is evil, -349 is not.
		{"evil-hater meets alignment -350", game.MobAggrEvil, -350, true},
		{"evil-hater meets alignment -349", game.MobAggrEvil, -349, false},
		{"good-hater meets alignment 350", game.MobAggrGood, 350, true},
		{"good-hater meets alignment 349", game.MobAggrGood, 349, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := newTestServer(t)

			mob := mobile(t, srv, "a paladin's hound", game.MobSentinel|tc.flag, MortalStartRoom)
			rec := fighterRecord("Welmar", 5, 200)
			rec.Alignment = tc.alignment
			place(t, srv, rec, MortalStartRoom)

			mobTick(t, srv)

			if got := mob.Fighting != nil; got != tc.attacks {
				t.Errorf("attacked = %v, want %v", got, tc.attacks)
			}
		})
	}
}

// TestAWimpyAggressiveMobileOnlyAttacksTheSleeping.
func TestAWimpyAggressiveMobileOnlyAttacksTheSleeping(t *testing.T) {
	srv, _ := newTestServer(t)

	mob := mobile(t, srv, "a jackal", game.MobSentinel|game.MobAggressive|game.MobWimpy, MortalStartRoom)
	victim, _ := place(t, srv, fighterRecord("Welmar", 5, 200), MortalStartRoom)

	mobTick(t, srv)
	if mob.Fighting != nil {
		t.Fatal("a wimpy mobile attacked somebody who was awake")
	}

	victim.Position = game.PosSleeping
	mobTick(t, srv)
	if mob.Fighting != victim {
		t.Error("a wimpy mobile did not attack a sleeping player")
	}
}

// TestAHolyShieldTurnsAwayEvil. Local rule; see the non-stock features note.
func TestAHolyShieldTurnsAwayEvil(t *testing.T) {
	srv, _ := newTestServer(t)

	mob := mobile(t, srv, "a demon", game.MobSentinel|game.MobAggressive, MortalStartRoom)
	mob.Record.Alignment = -1000

	rec := fighterRecord("Welmar", 5, 200)
	rec.AffectFlags = rec.AffectFlags.Set(game.AffectHolyShield)
	place(t, srv, rec, MortalStartRoom)

	for i := 0; i < 20; i++ {
		mobTick(t, srv)
	}
	if mob.Fighting != nil {
		t.Error("an evil mobile attacked somebody under a holy shield")
	}

	// A good mobile is not turned away by it.
	good := mobile(t, srv, "a zealot", game.MobSentinel|game.MobAggressive, MortalStartRoom)
	good.Record.Alignment = 1000

	mobTick(t, srv)
	if good.Fighting == nil {
		t.Error("a good mobile was turned away by a holy shield")
	}
}

// TestAScavengerTakesTheMostValuableThing, and leaves the worthless.
func TestAScavengerTakesTheMostValuableThing(t *testing.T) {
	srv, _ := newTestServer(t)

	mob := mobile(t, srv, "a beggar", game.MobSentinel|game.MobScavenger, MortalStartRoom)

	var cheap, dear *game.Object
	if err := srv.engine.DoSync(context.Background(), func(w *game.Live) {
		cheap = w.NewObject(testRingVnum)
		cheap.Cost = 5
		w.ObjectToRoom(cheap, MortalStartRoom)

		dear = w.NewObject(testSwordVnum)
		dear.Cost = 500
		w.ObjectToRoom(dear, MortalStartRoom)
	}); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 300 && len(mob.Carrying) == 0; i++ {
		mobTick(t, srv)
	}

	if len(mob.Carrying) == 0 {
		t.Fatal("a scavenger picked nothing up in 300 pulses")
	}
	if mob.Carrying[0] != dear {
		t.Errorf("the scavenger took %s, want the more valuable one", mob.Carrying[0].Name())
	}
}

// TestAScavengerIgnoresWorthlessThings. The C's `max` starts at 1, so an
// object worth nothing is never taken — which is why a busy room's floor
// fills with junk rather than being swept clean.
func TestAScavengerIgnoresWorthlessThings(t *testing.T) {
	srv, _ := newTestServer(t)

	mob := mobile(t, srv, "a beggar", game.MobSentinel|game.MobScavenger, MortalStartRoom)

	if err := srv.engine.DoSync(context.Background(), func(w *game.Live) {
		junk := w.NewObject(testRingVnum)
		junk.Cost = 0
		w.ObjectToRoom(junk, MortalStartRoom)
	}); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 300; i++ {
		mobTick(t, srv)
	}
	if len(mob.Carrying) != 0 {
		t.Error("a scavenger picked up something worth nothing")
	}
}

// TestABusyMobileDoesNothingElse: fighting or asleep means no wandering, no
// scavenging, no picking a new fight.
func TestABusyMobileDoesNothingElse(t *testing.T) {
	srv, _ := newTestServer(t)

	mob := mobile(t, srv, "a large dog", game.MobScavenger, MortalStartRoom)
	victim, _ := place(t, srv, fighterRecord("Welmar", 5, 200), MortalStartRoom)

	if err := srv.engine.DoSync(context.Background(), func(w *game.Live) {
		w.SetFighting(mob, victim)
		obj := w.NewObject(testSwordVnum)
		obj.Cost = 500
		w.ObjectToRoom(obj, MortalStartRoom)
	}); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 200; i++ {
		mobTick(t, srv)
	}
	if mob.Room != MortalStartRoom {
		t.Error("a fighting mobile wandered off")
	}
	if len(mob.Carrying) != 0 {
		t.Error("a fighting mobile scavenged")
	}

	// And a sleeping one is equally idle.
	sleeper := mobile(t, srv, "a drunk", game.MobScavenger, ImmortStartRoom)
	sleeper.Position = game.PosSleeping
	if err := srv.engine.DoSync(context.Background(), func(w *game.Live) {
		obj := w.NewObject(testSwordVnum)
		obj.Cost = 500
		w.ObjectToRoom(obj, ImmortStartRoom)
	}); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 200; i++ {
		mobTick(t, srv)
	}
	if len(sleeper.Carrying) != 0 || sleeper.Room != ImmortStartRoom {
		t.Error("a sleeping mobile did something")
	}
}

func TestTheMobilePulseIsEveryTenSeconds(t *testing.T) {
	srv, _ := newTestServer(t)

	var found bool
	for _, p := range srv.Periodic() {
		if p.Name != "mobile-activity" {
			continue
		}
		found = true
		if p.Every != 10*pulsesPerSecond {
			t.Errorf("mobile activity runs every %d pulses, want %d", p.Every, 10*pulsesPerSecond)
		}
	}
	if !found {
		t.Error("mobile activity is not scheduled")
	}
}
