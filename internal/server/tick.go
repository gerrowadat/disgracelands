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
		// Every pulse, and last: the C flushes output — appending a prompt
		// to it — after the commands and the heartbeat of the same pass
		// (comm.c:851-869), so a prompt reflects everything that happened
		// in the pulse it follows. See Session.PromptIfOwed. #385.
		{Name: "prompts", Every: 1, Run: s.flushPrompts},
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

// flushPrompts gives a prompt to every connection that has been written to
// since its last one — the C's output-flush half of game_loop, which is
// where a prompt comes from for anything the player did not type.
//
// On the world goroutine, which is what makes it safe: a prompt is built
// from live hit points, mana and movement.
func (s *Server) flushPrompts(_ *game.Live) {
	for _, sess := range s.connections.list() {
		sess.PromptIfOwed()
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
	//
	// Both extra arguments are what damage(ch, ch, n, TYPE_SUFFERING) makes
	// them, and neither is a formality:
	//
	//   - the victim is their own attacker, which is exactly what stops
	//     poison and bleeding out making a wimpy character run away —
	//     positionAftermath's two flees are guarded on `ch != victim`;
	//   - amount is the blow's size, so a large enough tick would say
	//     "That really did HURT!". None is large enough (poison is two
	//     points, bleeding one), but that is left to the arithmetic to
	//     say rather than being decided here.
	//
	// The C gets both for free from the arguments it was always passing;
	// spelling them out here keeps the two the same shape.
	s.positionAftermath(w, c, c, amount)

	if c.Position == game.PosDead {
		// No killer: nothing was attacking them. The C spells this as
		// damage(ch, ch, ...), so a reader of *its* code would say the
		// victim killed themselves; the server log says nothing rather
		// than saying that, because "killed by Bob" for a Bob who bled
		// out is worse than silence.
		s.die(w, c, nil)
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

// die leaves a body and puts the character back on their feet somewhere else,
// porting die() and raw_kill() (fight.c) as far as this phase goes.
//
// killer is whoever brought them down, or nil when nothing did — see
// logDeath, which is the only thing that reads it. Experience loss on death
// and the loss of the killer's alignment are combat's business and arrive
// with it.
func (s *Server) die(w *game.Live, c *game.Character, killer *game.Character) {
	s.logDeath(c, killer)

	// death_cry, and *only* death_cry. raw_kill's own announcement
	// (fight.c:389) is the cry; "$n is dead!  R.I.P." belongs to damage()'s
	// position switch (fight.c:891) and is sent there, once, before die() is
	// ever called. Sending it here too is what made a kill say it twice —
	// and made an implementor's `kill`, which reaches raw_kill without going
	// through damage() at all, say it when the C says nothing of the kind.
	w.DeathCry(c)

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

// logDeath writes the server log's record of a death: what died, where,
// and who killed it when anybody did.
//
// This is the operator's log and not the C's mudlog. The two are
// complementary and neither replaces the other. fight.c:953's "%s killed by
// %s at %s" is ported at damage() (violence.go) as a wizlog: it goes to
// immortals watching the syslog, and it fires only for a dead *player* with
// a killer, because the C explicitly skips it for a mobile
// (`if (!IS_NPC(victim))`, fight.c:938) — a fight is mostly dead mobiles and
// the C did not want its log full of them. This one fires for every death
// there is, which is why until #370 it read "a character died" for all of
// them and said nothing else: a log full of dead rats and a log recording
// that somebody's character had been killed were the same line, and neither
// named who did it.
//
// A mobile is named by its short description, the only name it has, and the
// prototype vnum goes with it — "the guildmaster" is not unique across the
// world and 3020 is. The player half needs no vnum: a name is the key.
//
// The victim's kind is in the message rather than an attribute, because the
// message is a field too under --log-format=json, so a `character_type`
// beside it would say the same thing twice. The killer's kind has to be an
// attribute; there is nowhere else for it, and it is the difference between
// a player killed by a mobile and a player killed by another player.
//
// A nil killer means nothing was attacking them, which is reachable only
// from suffer(): poison, or bleeding out.
func (s *Server) logDeath(victim, killer *game.Character) {
	message := "a player died"
	if victim.IsNPC() {
		message = "a mobile died"
	}

	attrs := []any{"character", victim.Name, "room", victim.Room}
	if victim.IsNPC() && victim.MobDef != nil {
		attrs = append(attrs, "vnum", victim.MobDef.Vnum)
	}

	if killer != nil {
		kind := "player"
		if killer.IsNPC() {
			kind = "mobile"
		}
		attrs = append(attrs, "killer", killer.Name, "killer_type", kind)
		if killer.IsNPC() && killer.MobDef != nil {
			attrs = append(attrs, "killer_vnum", killer.MobDef.Vnum)
		}
	}

	s.logger.Info(message, attrs...)
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
