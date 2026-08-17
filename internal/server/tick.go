// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"time"

	"github.com/gerrowadat/disgracelands/internal/engine"
	"github.com/gerrowadat/disgracelands/internal/game"
)

// The periodic work of the game, hung off the pulse the way the C hangs
// everything off PULSE_* (structs.h:512).
//
// A pulse is 100ms. The names below are the C's constants converted to pulse
// counts, so `PULSE_ZONE (10 RL_SEC)` at ten passes a second is 100 pulses.

const (
	// pulsesPerSecond is PASSES_PER_SEC at the C's OPT_USEC of 100ms.
	pulsesPerSecond = 10

	// pulseTick is how often the mud clock advances an hour and everybody
	// regenerates. The C calls this from heartbeat() on SECS_PER_MUD_HOUR.
	pulseTick = game.SecondsPerMudHour * pulsesPerSecond
)

// Periodic returns the scheduled work for the engine.
func (s *Server) Periodic() []engine.Periodic {
	return []engine.Periodic{
		{Name: "violence", Every: pulseViolence, Run: s.performViolence},
		{Name: "point-update", Every: pulseTick, Run: s.pointUpdate},
	}
}

// regenContext answers what the regeneration formulas need to know about a
// character that is not in their record: whether they are a mobile, what they
// are doing, and what room they are standing in.
type regenContext struct {
	character *game.Character
	room      *game.RoomDef
}

func (r regenContext) IsNPC() bool { return r.character.IsNPC() }

func (r regenContext) Position() game.Position { return r.character.Position }

func (r regenContext) Poisoned() bool {
	if r.character.Record == nil {
		return false
	}
	return r.character.Record.AffectFlags.Has(game.AffectPoison)
}

// GoodRegen is the local ROOM_GOOD_REGEN flag, which doubles every kind of
// regeneration. See docs/investigations/non-stock-features.md.
func (r regenContext) GoodRegen() bool {
	return r.room != nil && r.room.Flags.Has(game.RoomGoodRegen)
}

// pointUpdate is the mud-hourly tick, porting point_update (limits.c:457).
//
// Order matters and is the C's: conditions first, so a character who runs out
// of food this tick regenerates at the reduced rate this tick, not next.
func (s *Server) pointUpdate(w *game.Live) {
	now := time.Now()

	for _, c := range w.Players() {
		rec := c.Record
		if rec == nil {
			continue
		}

		if !c.IsNPC() {
			for _, cond := range []game.Condition{game.CondFull, game.CondDrunk, game.CondThirst} {
				if change := game.GainCondition(rec, cond, -1); change.Message != "" {
					c.Tell("%s", change.Message)
				}
			}
		}

		ctx := regenContext{character: c, room: w.Room(c.Room)}

		switch {
		case c.Position.Conscious():
			rec.Points.Hit = min(rec.Points.Hit+game.HitGain(rec, ctx, now), rec.Points.MaxHit)
			rec.Points.Mana = min(rec.Points.Mana+game.ManaGain(rec, ctx, now), rec.Points.MaxMana)
			rec.Points.Move = min(rec.Points.Move+game.MoveGain(rec, ctx, now), rec.Points.MaxMove)

			// Poison does two points a tick and a character can die of it,
			// which is why this is here and not with the rest of combat.
			if rec.AffectFlags.Has(game.AffectPoison) {
				s.suffer(w, c, 2)
				if c.Position == game.PosDead {
					continue
				}
			}

			if c.Position <= game.PosStunned {
				c.Position = game.UpdatePosition(rec, c.Position)
			}

		case c.Position == game.PosIncapacitated:
			s.suffer(w, c, 1)

		case c.Position == game.PosMortallyWounded:
			s.suffer(w, c, 2)
		}
	}

	// Then the objects, as point_update does: corpses count down and the
	// ones that reach zero rot away, spilling what was in them.
	for _, decayed := range w.DecayObjects() {
		s.announceDecay(w, decayed)
	}
}

// suffer takes hit points from a character with nothing attacking them —
// bleeding out, or poison — which is what point_update's calls to
// damage(ch, ch, n, TYPE_SUFFERING) do.
func (s *Server) suffer(w *game.Live, c *game.Character, amount int32) {
	c.Record.Points.Hit -= amount
	c.Position = game.UpdatePosition(c.Record, c.Position)

	if c.Position == game.PosDead {
		s.die(w, c)
	}
}

// die leaves a body and puts the character back on their feet somewhere else,
// porting die() and raw_kill() (fight.c) as far as this phase goes.
//
// Experience loss on death and the loss of the killer's alignment are combat's
// business and arrive with it.
func (s *Server) die(w *game.Live, c *game.Character) {
	s.logger.Info("a character died", "character", c.Name, "room", c.Room)

	c.Tell("You are dead!  Sorry...\r\n")
	for _, other := range w.Occupants(c.Room) {
		if other != c {
			other.Tell("%s is dead!  R.I.P.\r\n", c.Name)
		}
	}

	w.MakeCorpse(c)
	w.StopFighting(c)

	if c.IsNPC() {
		w.Remove(c)
		return
	}

	// A dead player wakes up at the temple with one hit point, which is the
	// C's arrangement: raw_kill leaves them at POS_STANDING and the death
	// trap handling sends them to their load room.
	c.Record.Points.Hit = 1
	c.Position = game.PosStanding
	if err := w.Enter(c, MortalStartRoom); err != nil {
		s.logger.Error("moving a dead character to the temple", "character", c.Name, "error", err)
	}
}

// announceDecay says the corpse rotted, in whichever place it was.
func (s *Server) announceDecay(w *game.Live, d game.DecayResult) {
	const message = "A quivering horde of maggots consumes %s.\r\n"

	switch {
	case d.CarriedBy != nil:
		d.CarriedBy.Tell("%s decays in your hands.\r\n", d.Corpse.Name())
	case d.Room != game.NoRoom:
		for _, c := range w.Occupants(d.Room) {
			c.Tell(message, d.Corpse.Name())
		}
	}
}
