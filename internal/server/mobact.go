// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"github.com/gerrowadat/disgracelands/internal/game"
)

// What mobiles do when nobody is watching, ported from mobile_activity
// (mobact.c).
//
// Everything here happens on one pulse every ten seconds, in one pass over
// every mobile in the world. The order within a mobile's turn is the C's, and
// it matters: a scavenger picks something up *before* it decides whether to
// wander off with it.

// pulseMobile is PULSE_MOBILE: every ten seconds.
const pulseMobile = 10 * pulsesPerSecond

// wanderRoll is the C's `number(0, 18) < NUM_OF_DIRS`. A mobile picks a
// number from nineteen and moves only if it lands on one of the six
// directions, so it wanders roughly a third of the time it is considered —
// and the direction it picks may be a wall, which it then does not take.
const wanderRoll = 18

// scavengeRoll is `!number(0, 10)`: one chance in eleven per pulse.
const scavengeRoll = 10

// mobileActivity runs one pass over the world's mobiles.
func (s *Server) mobileActivity(w *game.Live) {
	for _, mob := range w.Mobiles() {
		// A mobile in a fight is busy, and one asleep is asleep. Special
		// procedures would run before this check; they arrive with the
		// scripting seam.
		if mob.Fighting != nil || !mob.Position.Awake() {
			continue
		}

		s.scavenge(w, mob)
		s.wander(w, mob)
		s.beAggressive(w, mob)
	}
}

// scavenge picks up the most valuable thing on the floor, porting the
// scavenger branch.
//
// The C's `max` starts at 1, not 0, so an object worth nothing is never taken
// — which is why the floor of a busy room fills with worthless junk rather
// than being swept clean.
func (s *Server) scavenge(w *game.Live, mob *game.Character) {
	if !mob.HasMobFlag(game.MobScavenger) {
		return
	}
	floor := w.RoomObjects(mob.Room)
	if len(floor) == 0 || s.rng.Number(0, scavengeRoll) != 0 {
		return
	}

	var best *game.Object
	max := int32(1)
	for _, obj := range floor {
		if obj.Takeable() && obj.Cost > max {
			best, max = obj, obj.Cost
		}
	}
	if best == nil {
		return
	}

	w.ObjectToChar(best, mob)
	for _, other := range w.Occupants(mob.Room) {
		if other != mob {
			other.Tell("%s gets %s.\r\n", mob.Name, best.Name())
		}
	}
}

// wander moves a mobile one room, porting the movement branch.
func (s *Server) wander(w *game.Live, mob *game.Character) {
	if mob.HasMobFlag(game.MobSentinel) || mob.Position != game.PosStanding {
		return
	}

	// One number from nineteen; anything above the six directions means it
	// stays put this pulse. Checked before the conversion, since Direction is
	// a narrow type.
	roll := s.rng.Number(0, wanderRoll)
	if roll < 0 || roll >= game.NumDirections {
		return
	}
	dir := game.Direction(roll)

	exit := w.Exit(mob.Room, dir)
	if exit == nil || exit.ToRoom == game.NoRoom {
		return
	}
	// A closed door stops a mobile, which is what doors are for.
	if exit.State.Has(game.ExitClosed) {
		return
	}

	destination := w.Room(exit.ToRoom)
	if destination == nil || destination.Flags.HasAny(game.RoomNoMob|game.RoomDeathTrap) {
		return
	}
	if mob.HasMobFlag(game.MobStayZone) {
		if here := w.Room(mob.Room); here == nil || here.Zone != destination.Zone {
			return
		}
	}

	from := mob.Room
	if err := w.Enter(mob, exit.ToRoom); err != nil {
		return
	}
	for _, other := range w.Occupants(from) {
		other.Tell("%s leaves %s.\r\n", mob.Name, dir)
	}
	for _, other := range w.Occupants(mob.Room) {
		if other != mob {
			other.Tell("%s has arrived.\r\n", mob.Name)
		}
	}
}

// beAggressive attacks somebody, porting the aggressive branch.
func (s *Server) beAggressive(w *game.Live, mob *game.Character) {
	if !mob.HasMobFlag(game.MobAggressive|game.MobAggrToAlign) || mob.Fighting != nil {
		return
	}

	for _, victim := range w.Occupants(mob.Room) {
		if victim.IsNPC() || victim.Record == nil {
			continue
		}
		// PRF_NOHASSLE is how an immortal walks through a zone unmolested.
		if victim.Record.Preferences.Has(game.PrefNoHassle) {
			continue
		}
		// A local rule: an evil mobile will not touch somebody under a holy
		// shield. See docs/investigations/non-stock-features.md.
		if game.IsEvil(mob.Record) && victim.Record.AffectFlags.Has(game.AffectHolyShield) {
			continue
		}
		// A wimpy aggressive mobile only picks on the sleeping.
		if mob.HasMobFlag(game.MobWimpy) && victim.Position.Awake() {
			continue
		}

		if !s.aggressiveTowards(mob, victim) {
			continue
		}

		mob.Tell("You attack %s!\r\n", victim.Name)
		victim.Tell("%s attacks you!\r\n", mob.Name)
		for _, other := range w.Occupants(mob.Room) {
			if other != mob && other != victim {
				other.Tell("%s attacks %s!\r\n", mob.Name, victim.Name)
			}
		}
		w.SetFighting(mob, victim)
		return
	}
}

// aggressiveTowards reports whether this mobile attacks this victim, given
// the four aggression flags.
func (s *Server) aggressiveTowards(mob, victim *game.Character) bool {
	switch {
	case mob.HasMobFlag(game.MobAggressive):
		return true
	case mob.HasMobFlag(game.MobAggrEvil) && game.IsEvil(victim.Record):
		return true
	case mob.HasMobFlag(game.MobAggrGood) && game.IsGood(victim.Record):
		return true
	case mob.HasMobFlag(game.MobAggrNeutral) && game.IsNeutral(victim.Record):
		return true
	}
	return false
}
