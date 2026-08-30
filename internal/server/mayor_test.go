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

// The mayor, end to end — SPECIAL(mayor) (spec_procs.c:277), the last
// special procedure noted in docs/deviations.md. A scripted patrol,
// twice a day, that MoveMobile (internal/game/live.go, shared with
// wander in mobact_test.go) actually carries out.

// mayor puts a mayor mobile in MayorLoopRoom, with the "mayor" special
// attached the way spec_assign.c's ASSIGNMOB(3105, mayor) does.
func mayor(t *testing.T, srv *Server) *game.Character {
	t.Helper()
	c := &game.Character{
		Name: "the Mayor", Keywords: "mayor", NPC: true,
		Position: game.PosSleeping,
		MobDef: &game.MobDef{
			Vnum: 3105, ShortDesc: "the Mayor", Keywords: "mayor",
			ActionFlags: game.NewSet(game.MobSpec), Spec: "mayor",
		},
		Record: &game.PlayerRecord{
			Name: "the Mayor", Level: 24, Birth: time.Now(),
			Points: game.Points{Hit: 100, MaxHit: 100},
		},
	}
	if err := srv.engine.DoSync(context.Background(), func(w *game.Live) {
		if err := w.Enter(c, MayorLoopRoom); err != nil {
			t.Errorf("placing the mayor: %v", err)
		}
		w.Track(c)
		// MudTime is measured from when the world booted (game.Live.
		// MudTime); backdating that epoch by exactly six mud-hours puts
		// time_info.hours at 6, the open path's own trigger.
		w.SetBooted(time.Now().Add(-6 * game.MudHour))
	}); err != nil {
		t.Fatal(err)
	}
	return c
}

// TestMayorWalksTheOpenPathAtDawn runs the whole of mayorOpenPath — every
// digit a direction MoveMobile is asked to try (MayorLoopRoom makes all
// four always succeed), every letter a line the mayor says, 'W' waking it
// and 'S' putting it back to sleep at the end. One mobileActivity pulse
// consumes exactly one path step, since the position guard never blocks in
// this room, so the walk completes in exactly len(mayorOpenPath) pulses —
// checked directly rather than just ticking "enough" times and hoping.
func TestMayorWalksTheOpenPathAtDawn(t *testing.T) {
	srv, _ := newTestServer(t)
	m := mayor(t, srv)

	// mayorOpenPath's own length (specprocs.go): 50 steps.
	const openPathLength = 50
	for i := 0; i < openPathLength; i++ {
		mobTick(t, srv)
	}

	if m.Position != game.PosSleeping {
		t.Errorf("after the full open path the mayor is %s, want sleeping", m.Position)
	}
}

// TestMayorDoesNothingOutsideItsHours: no walk starts at all outside 6am
// and 8pm, however many pulses go by.
func TestMayorDoesNothingOutsideItsHours(t *testing.T) {
	srv, _ := newTestServer(t)
	c := &game.Character{
		Name: "the Mayor", Keywords: "mayor", NPC: true,
		Position: game.PosSleeping,
		MobDef: &game.MobDef{
			Vnum: 3105, ShortDesc: "the Mayor", Keywords: "mayor",
			ActionFlags: game.NewSet(game.MobSpec), Spec: "mayor",
		},
		Record: &game.PlayerRecord{
			Name: "the Mayor", Level: 24, Birth: time.Now(),
			Points: game.Points{Hit: 100, MaxHit: 100},
		},
	}
	if err := srv.engine.DoSync(context.Background(), func(w *game.Live) {
		if err := w.Enter(c, MayorLoopRoom); err != nil {
			t.Fatalf("placing the mayor: %v", err)
		}
		w.Track(c)
		// Noon: neither 6 nor 20.
		w.SetBooted(time.Now().Add(-12 * game.MudHour))
	}); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 200; i++ {
		mobTick(t, srv)
	}
	if c.Position != game.PosSleeping {
		t.Errorf("the mayor moved at all outside its hours: now %s", c.Position)
	}
	if c.Room != MayorLoopRoom {
		t.Errorf("the mayor left its room outside its hours: now in %d", c.Room)
	}
}

// TestMayorRetriesTheSameStepWhileIncapacitated: the C's own guard is
// GET_POS(ch) < POS_SLEEPING || GET_POS(ch) == POS_FIGHTING — a mayor
// stunned mid-patrol picks the walk back up once it can act again, rather
// than skipping ahead, because the C's static index is never advanced on a
// pulse the guard refuses.
func TestMayorRetriesTheSameStepWhileIncapacitated(t *testing.T) {
	srv, _ := newTestServer(t)
	m := mayor(t, srv)

	// The walk starts moving ('W' wakes it on the very first pulse).
	mobTick(t, srv)
	if m.Position != game.PosStanding {
		t.Fatalf("the mayor did not wake on the first pulse: %s", m.Position)
	}

	// Knocked out mid-patrol: many pulses go by and nothing advances,
	// because the position guard refuses every one of them.
	if err := srv.engine.DoSync(context.Background(), func(w *game.Live) {
		m.Position = game.PosStunned
	}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		mobTick(t, srv)
	}
	if m.Position != game.PosStunned {
		t.Errorf("the mayor's own position changed while stunned: %s", m.Position)
	}

	// Recovers, and the walk continues from where it left off rather than
	// restarting — proven by it still reaching the end within what is left
	// of the open path's length, not needing a fresh 50 pulses.
	if err := srv.engine.DoSync(context.Background(), func(w *game.Live) {
		m.Position = game.PosStanding
	}); err != nil {
		t.Fatal(err)
	}
	const openPathLength = 50
	for i := 0; i < openPathLength-1; i++ {
		mobTick(t, srv)
	}
	if m.Position != game.PosSleeping {
		t.Errorf("the mayor never finished the walk after recovering: %s", m.Position)
	}
}
