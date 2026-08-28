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

// round runs one combat round synchronously.
func round(t *testing.T, srv *Server) {
	t.Helper()
	if err := srv.engine.DoSync(context.Background(), srv.performViolence); err != nil {
		t.Fatalf("running a combat round: %v", err)
	}
}

// fighterRecord is a character built to win or lose predictably.
//
// The real values are snapshotted, as they are for any character the game
// itself builds: without them the first recompute — a spell landing, a shield
// going on — would rebuild the character from zeroes.
func fighterRecord(name string, level, hit int32) *game.PlayerRecord {
	rec := &game.PlayerRecord{
		Name: name, Class: game.ClassWarrior, Level: level,
		Birth:      time.Now(),
		Conditions: [3]int32{0, 24, 24},
		Abilities: game.Abilities{
			Strength: 18, Intelligence: 18, Wisdom: 18,
			Dexterity: 12, Constitution: 12, Charisma: 12,
		},
		// Movement points, because walking costs them now: a fixture with
		// none is exhausted before it takes a step, and every test that
		// walks one of these anywhere depends on it having some.
		Points: game.Points{Hit: hit, MaxHit: hit, Armor: 100, Move: 82, MaxMove: 82},
	}
	game.SnapshotReal(rec)
	return rec
}

// TestAFightRunsOnTheViolencePulse, and both sides swing.
func TestAFightRunsOnTheViolencePulse(t *testing.T) {
	srv, _ := newTestServer(t)

	attacker, attackerClient := place(t, srv, fighterRecord("Zod", 30, 500), MortalStartRoom)
	victim, victimClient := place(t, srv, fighterRecord("Welmar", 5, 200), MortalStartRoom)
	attacker.Record.Points.HitRoll = 100

	if err := srv.engine.DoSync(context.Background(), func(w *game.Live) {
		w.SetFighting(attacker, victim)
	}); err != nil {
		t.Fatal(err)
	}

	before := victim.Record.Points.Hit
	round(t, srv)

	if victim.Record.Points.Hit >= before {
		t.Errorf("the victim is on %d hit points, was %d — no blow landed",
			victim.Record.Points.Hit, before)
	}
	// The exact wording now varies by damage tier and weapon verb
	// (internal/game/damage_messages.go), so this checks for the name
	// every tier's text includes rather than one fixed phrase — HitRoll
	// guarantees a landed blow, so it is always the hit text, never a
	// miss.
	if !attackerClient.said("Welmar") {
		t.Error("the attacker was not told they hit")
	}
	if !victimClient.said("Zod") {
		t.Error("the victim was not told they were hit")
	}

	// Being hit puts the victim into the fight too.
	if victim.Fighting != attacker {
		t.Error("the victim did not fight back")
	}
	if victim.Position != game.PosFighting {
		t.Errorf("the victim is %s, want fighting", victim.Position)
	}
}

// TestAFightEndsWhenSomebodyDies, with a corpse and an experience award.
func TestAFightEndsWhenSomebodyDies(t *testing.T) {
	srv, _ := newTestServer(t)

	attacker, attackerClient := place(t, srv, fighterRecord("Zod", 30, 500), MortalStartRoom)
	attacker.Record.Points.HitRoll = 100
	attacker.Record.Points.DamRoll = 200
	attacker.Record.Points.Exp = game.LevelExperience(game.ClassWarrior, 30)

	victim, _ := place(t, srv, fighterRecord("a large dog", 5, 20), MortalStartRoom)
	victim.NPC = true
	victim.Record.Points.Exp = 5000

	if err := srv.engine.DoSync(context.Background(), func(w *game.Live) {
		w.SetFighting(attacker, victim)
	}); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 5 && victim.Position != game.PosDead; i++ {
		round(t, srv)
	}

	if victim.Position != game.PosDead {
		t.Fatalf("the dog is %s on %d hit points after five rounds",
			victim.Position, victim.Record.Points.Hit)
	}
	if !attackerClient.said("experience") {
		t.Error("the killer was told nothing about experience")
	}

	// The fight is over on both sides, and there is a body.
	if attacker.Fighting != nil {
		t.Error("the attacker is still fighting a corpse")
	}
	if err := srv.engine.DoSync(context.Background(), func(w *game.Live) {
		var found bool
		for _, o := range w.RoomObjects(MortalStartRoom) {
			if game.IsCorpse(o) {
				found = true
			}
		}
		if !found {
			t.Error("no corpse was left")
		}
		// A dead mobile is removed from the world.
		if w.Find("a large dog") != nil {
			t.Error("the dead mobile is still in the world")
		}
	}); err != nil {
		t.Fatal(err)
	}
}

// TestKillingAPlayerIsWorthNothing. This is the local rule, and the message
// is the local tone.
func TestKillingAPlayerIsWorthNothing(t *testing.T) {
	srv, _ := newTestServer(t)

	killer, killerClient := place(t, srv, fighterRecord("Zod", 20, 500), MortalStartRoom)
	killer.Record.Points.Exp = game.LevelExperience(game.ClassWarrior, 20)
	before := killer.Record.Points.Exp

	victim, _ := place(t, srv, fighterRecord("Welmar", 25, 10), MortalStartRoom)
	victim.Record.Points.Exp = 5_000_000

	// On the world goroutine: award reaches AnnounceLevelGain, which walks
	// the player list to broadcast a level (#212).
	inWorld(t, srv, func(w *game.Live) { srv.award(w, killer, victim) })

	if killer.Record.Points.Exp != before {
		t.Errorf("killing a player awarded %d experience",
			killer.Record.Points.Exp-before)
	}
	if !killerClient.said("You receive no experience! HA!.") {
		t.Error("the killer was not given the local message")
	}
}

// TestKillingAMobileIsWorthSomething, by contrast.
func TestKillingAMobileIsWorthSomething(t *testing.T) {
	srv, _ := newTestServer(t)

	killer, killerClient := place(t, srv, fighterRecord("Zod", 20, 500), MortalStartRoom)
	killer.Record.Points.Exp = game.LevelExperience(game.ClassWarrior, 20)
	before := killer.Record.Points.Exp

	victim, _ := place(t, srv, fighterRecord("a large dog", 25, 10), MortalStartRoom)
	victim.NPC = true
	victim.Record.Points.Exp = 300000

	// On the world goroutine: award reaches AnnounceLevelGain, which walks
	// the player list to broadcast a level (#212).
	inWorld(t, srv, func(w *game.Live) { srv.award(w, killer, victim) })

	if killer.Record.Points.Exp <= before {
		t.Error("killing a mobile awarded nothing")
	}
	if !killerClient.said("experience points") {
		t.Error("the killer was not told what they earned")
	}
}

// TestAFightStopsWhenTheVictimLeaves, which is the check perform_violence
// makes before every swing.
func TestAFightStopsWhenTheVictimLeaves(t *testing.T) {
	srv, _ := newTestServer(t)

	attacker, _ := place(t, srv, fighterRecord("Zod", 30, 500), MortalStartRoom)
	victim, _ := place(t, srv, fighterRecord("Welmar", 5, 200), MortalStartRoom)

	if err := srv.engine.DoSync(context.Background(), func(w *game.Live) {
		w.SetFighting(attacker, victim)
		// Walk away.
		if err := w.Enter(victim, ImmortStartRoom); err != nil {
			t.Errorf("moving the victim: %v", err)
		}
	}); err != nil {
		t.Fatal(err)
	}

	round(t, srv)

	if attacker.Fighting != nil {
		t.Error("the attacker is still fighting somebody who left the room")
	}
	if attacker.Position != game.PosStanding {
		t.Errorf("the attacker is %s, want standing", attacker.Position)
	}
}

// TestASittingPlayerCannotFightButAMobileGetsUp.
func TestASittingPlayerCannotFightButAMobileGetsUp(t *testing.T) {
	srv, _ := newTestServer(t)

	sitter, sitterClient := place(t, srv, fighterRecord("Welmar", 30, 500), MortalStartRoom)
	target, _ := place(t, srv, fighterRecord("Zod", 5, 500), MortalStartRoom)

	if err := srv.engine.DoSync(context.Background(), func(w *game.Live) {
		w.SetFighting(sitter, target)
	}); err != nil {
		t.Fatal(err)
	}
	sitter.Position = game.PosSitting

	before := target.Record.Points.Hit
	round(t, srv)

	if !sitterClient.said("You can't fight while sitting!!") {
		t.Error("a sitting player was not told they cannot fight")
	}
	if target.Record.Points.Hit != before {
		t.Error("a sitting player landed a blow")
	}

	// A mobile in the same position gets to its feet instead.
	mob, _ := place(t, srv, fighterRecord("a large dog", 30, 500), ImmortStartRoom)
	mob.NPC = true
	prey, _ := place(t, srv, fighterRecord("Bystander", 5, 500), ImmortStartRoom)

	if err := srv.engine.DoSync(context.Background(), func(w *game.Live) {
		w.SetFighting(mob, prey)
	}); err != nil {
		t.Fatal(err)
	}
	mob.Position = game.PosSitting

	round(t, srv)

	if mob.Position == game.PosSitting {
		t.Error("a mobile stayed sitting rather than scrambling to its feet")
	}
}

// TestTheViolencePulseIsEveryTwoSeconds, which is PULSE_VIOLENCE.
func TestTheViolencePulseIsEveryTwoSeconds(t *testing.T) {
	srv, _ := newTestServer(t)

	var found bool
	for _, p := range srv.Periodic() {
		if p.Name != "violence" {
			continue
		}
		found = true
		if p.Every != 2*pulsesPerSecond {
			t.Errorf("the violence round runs every %d pulses, want %d",
				p.Every, 2*pulsesPerSecond)
		}
	}
	if !found {
		t.Error("the violence round is not scheduled")
	}
}
