// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"slices"
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
	p := []engine.Periodic{
		{Name: "violence", Every: pulseViolence, Run: s.performViolence},
		{Name: "mobile-activity", Every: pulseMobile, Run: s.mobileActivity},
		{Name: "zone-update", Every: pulseZone, Run: s.zoneUpdate},
		{Name: "point-update", Every: pulseTick, Run: s.pointUpdate},
	}
	// --freeze-mobiles drops the pulse rather than making mobileActivity
	// return early, so the C's `if (!(pulse % PULSE_MOBILE) &&
	// !freeze_mobiles)` and this agree about the one thing that matters:
	// no dice are rolled. A version that entered the function and returned
	// would still have to be read carefully to be sure of that.
	if s.freezeMobiles {
		p = slices.DeleteFunc(p, func(e engine.Periodic) bool { return e.Name == "mobile-activity" })
	}
	return p
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
	// weather_and_time(1) comes first on this pulse in the C, ahead of
	// affect_update and point_update (comm.c:934), and the order matters for
	// one reason only: it rolls. Five draws a mud hour, sometimes six.
	s.weatherAndTime(w)

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

		// Affects age on the same tick, porting affect_update (limits.c).
		for _, expired := range game.AgeAffects(rec) {
			if expired.Message != "" {
				c.Tell("%s\r\n", expired.Message)
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

	// The C reaches this through damage(), which announces the new position
	// before deciding whether anybody died (fight.c:877). Bleeding out is not
	// a quieter way to die than being hit — it is the same function — so
	// "You are dead!  Sorry..." and "$n is dead!  R.I.P." come from here, the
	// same place they come from in a fight, rather than from die().
	s.announcePosition(w, c)

	if c.Position == game.PosDead {
		s.die(w, c)
	}
}

// weatherAndTime is weather_and_time(1) (weather.c:29): another_hour's four
// announcements, then weather_change.
//
// The hour is derived here rather than incremented, so "the hour just ticked
// over" is "the hour this pulse lands on" — the pulse is one mud hour long by
// construction (pulseTick), which is the same thing said differently.
func (s *Server) weatherAndTime(w *game.Live) {
	// --freeze-weather is the C's `weather_and_time(0)`: the hour still
	// advances, nothing is announced, and no dice are rolled. Only the
	// parity harness sets it.
	if s.freezeWeather {
		return
	}
	outdoors := w.Outdoors()

	if line := game.SunriseMessage(w.MudTime().Hours); line != "" {
		for _, c := range outdoors {
			c.Tell("%s", line)
		}
	}
	for _, line := range w.AdvanceWeather(s.rng) {
		for _, c := range outdoors {
			c.Tell("%s", line)
		}
	}
}

// deathCry is death_cry (fight.c:367): the room hears whose it was, and every
// room one step away hears that it was somebody.
//
// The neighbours are reached through CAN_GO, so a closed door muffles it —
// the same condition `exits` uses to decide what to list. Two exits leading
// to the same room send it there twice, which the C does not guard against
// and neither does this.
func (s *Server) deathCry(w *game.Live, c *game.Character) {
	for _, other := range w.Occupants(c.Room) {
		if other == c {
			continue
		}
		other.Tell("%s", w.Act("Your blood freezes as you hear $n's death cry.",
			game.ActArgs{Actor: c}, other))
	}

	room := w.Room(c.Room)
	if room == nil {
		return
	}
	for dir := game.Direction(0); dir < game.NumDirections; dir++ {
		exit := room.Exits[dir]
		if exit == nil || exit.ToRoom == game.NoRoom || exit.State.Has(game.ExitClosed) {
			continue
		}
		for _, other := range w.Occupants(exit.ToRoom) {
			other.Tell("Your blood freezes as you hear someone's death cry.\r\n")
		}
	}
}

// die leaves a body and puts the character back on their feet somewhere else,
// porting die() and raw_kill() (fight.c) as far as this phase goes.
//
// Experience loss on death and the loss of the killer's alignment are combat's
// business and arrive with it.
func (s *Server) die(w *game.Live, c *game.Character) {
	s.logger.Info("a character died", "character", c.Name, "room", c.Room)

	// death_cry, and *only* death_cry. raw_kill's own announcement
	// (fight.c:389) is the cry; "$n is dead!  R.I.P." belongs to damage()'s
	// position switch (fight.c:891) and is sent there, once, before die() is
	// ever called. Sending it here too is what made a kill say it twice —
	// and made an implementor's `kill`, which reaches raw_kill without going
	// through damage() at all, say it when the C says nothing of the kind.
	s.deathCry(w, c)

	w.MakeCorpse(c)
	w.StopFighting(c)

	// Dying dissolves every following relationship they were part of, porting
	// die_follower. Without it a leader's follower list keeps a pointer to
	// somebody who is no longer in the world, and the next step they take
	// tries to drag a corpse along.
	for _, orphan := range w.DieFollower(c) {
		orphan.Tell("You stop following %s.\r\n", c.Name)
	}

	if c.IsNPC() {
		// Out of the mobile list as well as the room, so the zone's
		// population cap frees up and the next reset can replace it.
		w.RemoveMobile(c)
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
