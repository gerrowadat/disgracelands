// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// recorder is a game.Client that keeps what was said to it.
type recorder struct {
	mu    sync.Mutex
	lines []string
}

func (r *recorder) Send(format string, args ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(args) > 0 {
		r.lines = append(r.lines, fmt.Sprintf(format, args...))
		return
	}
	r.lines = append(r.lines, format)
}

func (r *recorder) said(s string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return strings.Contains(strings.Join(r.lines, ""), s)
}

// place puts a character into the world for a tick test.
func place(t *testing.T, srv *Server, rec *game.PlayerRecord, room game.RoomVnum) (*game.Character, *recorder) {
	t.Helper()

	client := &recorder{}
	c := &game.Character{
		Name: rec.Name, Record: rec, Client: client,
		Position: game.PosStanding,
	}
	if err := srv.engine.DoSync(context.Background(), func(w *game.Live) {
		if err := w.Enter(c, room); err != nil {
			t.Errorf("entering the world: %v", err)
		}
	}); err != nil {
		t.Fatal(err)
	}
	return c, client
}

// TestPointUpdateHealsAndFeeds runs the mud-hourly tick directly, which is
// what the engine calls every 750 pulses.
func TestPointUpdateHealsAndFeeds(t *testing.T) {
	srv, _ := newTestServer(t)

	rec := &game.PlayerRecord{
		Name: "Welmar", Class: game.ClassWarrior, Level: 10,
		Birth:      time.Now(),
		Conditions: [3]int32{0, 24, 24},
		Points: game.Points{
			Hit: 10, MaxHit: 100,
			Mana: 10, MaxMana: 100,
			Move: 10, MaxMove: 100,
		},
	}
	c, _ := place(t, srv, rec, MortalStartRoom)

	tick(t, srv)

	if rec.Points.Hit <= 10 {
		t.Errorf("hit points are %d, want more than the 10 they started with", rec.Points.Hit)
	}
	if rec.Points.Move <= 10 {
		t.Errorf("movement is %d, want more than 10", rec.Points.Move)
	}
	if rec.Conditions[game.CondFull] != 23 || rec.Conditions[game.CondThirst] != 23 {
		t.Errorf("conditions are %v, want food and drink down by one", rec.Conditions)
	}
	if c.Position != game.PosStanding {
		t.Errorf("position is %s, want standing", c.Position)
	}
}

// TestRegenerationStopsAtTheMaximum.
func TestRegenerationStopsAtTheMaximum(t *testing.T) {
	srv, _ := newTestServer(t)

	rec := &game.PlayerRecord{
		Name: "Welmar", Class: game.ClassWarrior, Level: 10,
		Birth:      time.Now(),
		Conditions: [3]int32{0, 24, 24},
		Points: game.Points{
			Hit: 99, MaxHit: 100,
			Mana: 100, MaxMana: 100,
			Move: 100, MaxMove: 100,
		},
	}
	place(t, srv, rec, MortalStartRoom)

	tick(t, srv)

	if rec.Points.Hit != 100 || rec.Points.Mana != 100 || rec.Points.Move != 100 {
		t.Errorf("points are %d/%d/%d, want everything capped at its maximum",
			rec.Points.Hit, rec.Points.Mana, rec.Points.Move)
	}
}

// TestHungerIsAnnouncedOnceItBites.
func TestHungerIsAnnouncedOnceItBites(t *testing.T) {
	srv, _ := newTestServer(t)

	rec := &game.PlayerRecord{
		Name: "Welmar", Class: game.ClassWarrior, Level: 10,
		Birth:      time.Now(),
		Conditions: [3]int32{0, 1, 1},
		Points:     game.Points{Hit: 10, MaxHit: 100, Move: 10, MaxMove: 100},
	}
	_, client := place(t, srv, rec, MortalStartRoom)

	tick(t, srv)

	if !client.said("You are hungry.") {
		t.Error("a starving character was not told they are hungry")
	}
	if !client.said("You are thirsty.") {
		t.Error("a parched character was not told they are thirsty")
	}
}

// TestADyingCharacterBleeds, which is what point_update's calls to damage
// with TYPE_SUFFERING do: one point a tick when incapacitated, two when
// mortally wounded, and no way to stop it without help.
func TestADyingCharacterBleeds(t *testing.T) {
	for _, tc := range []struct {
		name     string
		hit      int32
		position game.Position
		lose     int32
	}{
		{"incapacitated", -4, game.PosIncapacitated, 1},
		{"mortally wounded", -7, game.PosMortallyWounded, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := newTestServer(t)

			rec := &game.PlayerRecord{
				Name: "Welmar", Class: game.ClassWarrior, Level: 10,
				Birth:      time.Now(),
				Conditions: [3]int32{0, 24, 24},
				Points:     game.Points{Hit: tc.hit, MaxHit: 100},
			}
			c, _ := place(t, srv, rec, MortalStartRoom)
			c.Position = tc.position

			tick(t, srv)

			if want := tc.hit - tc.lose; rec.Points.Hit != want {
				t.Errorf("hit points are %d, want %d", rec.Points.Hit, want)
			}
		})
	}
}

// TestBleedingOutReachesDead, and says so to the room.
func TestBleedingOutReachesDead(t *testing.T) {
	srv, _ := newTestServer(t)

	rec := &game.PlayerRecord{
		Name: "Welmar", Class: game.ClassWarrior, Level: 10,
		Birth:      time.Now(),
		Conditions: [3]int32{0, 24, 24},
		Points:     game.Points{Hit: -10, MaxHit: 100},
	}
	dying, dyingClient := place(t, srv, rec, MortalStartRoom)
	dying.Position = game.PosMortallyWounded

	watcher := &game.PlayerRecord{Name: "Zod", Class: game.ClassWarrior, Level: 34}
	_, watcherClient := place(t, srv, watcher, MortalStartRoom)

	tick(t, srv)

	if dying.Position != game.PosDead {
		t.Errorf("position is %s at %d hit points, want dead",
			dying.Position, rec.Points.Hit)
	}
	if !dyingClient.said("You are dead!") {
		t.Error("the dying character was not told")
	}
	if !watcherClient.said("Welmar is dead!") {
		t.Error("the room was not told")
	}
}

// TestImmortalsDoNotStarve on the tick either.
func TestImmortalsDoNotStarve(t *testing.T) {
	srv, _ := newTestServer(t)

	rec := &game.PlayerRecord{
		Name: "Zod", Class: game.ClassWarrior, Level: game.LevelImplementor,
		Birth:      time.Now(),
		Conditions: [3]int32{-1, -1, -1},
		Points:     game.Points{Hit: 100, MaxHit: 500},
	}
	_, client := place(t, srv, rec, ImmortStartRoom)

	for i := 0; i < 5; i++ {
		tick(t, srv)
	}

	if rec.Conditions != [3]int32{-1, -1, -1} {
		t.Errorf("conditions are %v, want them left alone", rec.Conditions)
	}
	if client.said("You are hungry.") {
		t.Error("an immortal was told they are hungry")
	}
}

// TestTheTickIsScheduledOnTheMudHour. The number is not arbitrary: a mud
// hour is 75 seconds and a pulse is a tenth of a second, so the tick runs
// every 750 pulses and everything timed off it inherits that.
func TestTheTickIsScheduledOnTheMudHour(t *testing.T) {
	srv, _ := newTestServer(t)

	work := srv.Periodic()
	if len(work) == 0 {
		t.Fatal("the server scheduled no periodic work")
	}

	var found bool
	for _, p := range work {
		if p.Name != "point-update" {
			continue
		}
		found = true
		if want := uint64(game.SecondsPerMudHour * pulsesPerSecond); p.Every != want {
			t.Errorf("point-update runs every %d pulses, want %d", p.Every, want)
		}
	}
	if !found {
		t.Error("point-update is not scheduled")
	}
}

// tick runs one point_update synchronously.
func tick(t *testing.T, srv *Server) {
	t.Helper()
	if err := srv.engine.DoSync(context.Background(), srv.pointUpdate); err != nil {
		t.Fatalf("running the tick: %v", err)
	}
}
