// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/obs"
	"github.com/gerrowadat/disgracelands/internal/session"
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
		// The mobile's own special procedure runs first, and — this is the
		// part that matters — *before* the fighting and awake checks below.
		// A snake bites and a mobile mage casts only while fighting, so a
		// special placed after those checks would never fire at all.
		if !s.noSpecials && mob.HasMobFlag(game.MobSpec) {
			if session.PulseSpecial(w, mob, s.rng, s) {
				continue
			}
		}

		// A mobile in a fight is busy, and one asleep is asleep.
		if mob.Fighting != nil || !mob.Position.Awake() {
			continue
		}

		s.scavenge(w, mob)
		s.wander(w, mob)
		s.beAggressive(w, mob)
		s.avengeItself(w, mob)
		s.helpOthers(w, mob)
	}
}

// helpOthers is the "Helper Mobs" pass, the last thing mobile_activity does
// (mobact.c:259-272), and the whole of MOB_HELPER: a flagged mobile joins a
// fight one of its neighbours is in.
//
// Three of the conditions are the entire character of the flag and are worth
// reading as rules rather than as guards:
//
//   - it helps **mobiles** only (`!IS_NPC(vict)` skips a player), so the
//     flag never makes a guard rescue an adventurer;
//   - only against a **player** (`IS_NPC(FIGHTING(vict))` skips a
//     mob-on-mob fight), so two flagged mobiles fighting each other do not
//     drag the whole room in;
//   - and it never joins a fight that is already **against itself**
//     (`ch == FIGHTING(vict)`), which would be swinging at the person
//     already swinging at it.
//
// One neighbour per helper per pulse: the C's inner loop sets `found` and
// stops. That bounds less than it sounds like it does, and the correction
// belongs here rather than only in the test — `found` is reset once per
// *mobile*, so it stops one helper joining two fights and does nothing to
// stop every helper in the room joining the same one. A room of guards
// really does all pile in at once. A blind or charmed helper sits it out.
func (s *Server) helpOthers(w *game.Live, mob *game.Character) {
	if !mob.HasMobFlag(game.MobHelper) ||
		mob.HasAffect(game.AffectBlind) || mob.HasAffect(game.AffectCharm) {
		return
	}

	for _, other := range w.Occupants(mob.Room) {
		if other == mob || !other.IsNPC() || other.Fighting == nil {
			continue
		}
		if other.Fighting.IsNPC() || other.Fighting == mob {
			continue
		}

		// act("$n jumps to the aid of $N!", FALSE, ch, 0, vict, TO_ROOM):
		// everybody in the room except the helper, the one being helped
		// included. hide_invisible is FALSE, so it is not filtered on
		// sight -- an unseen helper still produces the line, with $n
		// resolving to "someone" for whoever cannot see them, which is
		// game.Act's own job.
		s.actToRoomExcept(w, mob, nil,
			game.ActArgs{Actor: mob, Victim: other}, "$n jumps to the aid of $N!")
		s.hit(w, mob, other.Fighting)
		return
	}
}

// avengeItself is the "Mob Memory" pass (mobact.c:163-181): a MOB_MEMORY
// mobile attacks anybody in the room it is holding a grudge against.
//
// Before the helper pass and after the aggressive one, which is the C's own
// order -- so an aggressive mobile that also remembers you picks its target
// by aggression first, and this only reaches somebody it had no other
// reason to attack.
//
// Three skips, and two of them do work the aggressive pass's do not. CAN_SEE
// means a mobile cannot avenge itself on somebody it cannot see, so
// invisibility hides you from a grudge as well as from a wandering monster.
// PRF_NOHASSLE keeps gods out of it, and is tested here as well as in
// Character.Remember, because a god can turn it on *after* being remembered
// -- which is exactly when they would want to.
//
// One per pulse, like the helper pass, and with the same caveat: the bound
// is per mobile, not per room.
//
// **What is deliberately not ported** is the local <DoC> extension that
// follows it (mobact.c:183-215), where mobiles 14217 and 14203 hunt
// remembered players across the whole descriptor list rather than the room,
// one time in five, and 14217 summons them. Those vnums live in the real
// game data, which this repository does not ship (docs/investigations/), so
// there is nothing here to test it against. See #380.
func (s *Server) avengeItself(w *game.Live, mob *game.Character) {
	if !mob.HasMobFlag(game.MobMemory) || len(mob.Memory) == 0 {
		return
	}

	for _, other := range w.Occupants(mob.Room) {
		if other.IsNPC() || !w.CanSee(mob, other) {
			continue
		}
		if other.Record != nil && other.Record.Preferences.Has(game.PrefNoHassle) {
			continue
		}
		if !mob.Remembers(other) {
			continue
		}

		s.actToRoomExcept(w, mob, nil, game.ActArgs{Actor: mob},
			"'Hey!  You're the fiend that attacked me!!!', exclaims $n.")
		s.hit(w, mob, other)
		return
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

// wander moves a mobile one room, porting the movement branch. The move
// itself, and what it does and does not check, is game.Live.MoveMobile —
// shared with the mayor's own scripted patrol (specprocs.go), the other
// caller that needed a mobile's do_simple_move rather than a player's.
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
	w.MoveMobile(mob, game.Direction(roll))
}

// beAggressive attacks somebody, porting the aggressive branch.
func (s *Server) beAggressive(w *game.Live, mob *game.Character) {
	if !mob.HasAnyMobFlag(game.MobAggrToAlign.With(game.MobAggressive)) || mob.Fighting != nil {
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
		if _, message := w.SetFighting(mob, victim); message != "" {
			s.wizlog(obs.LogBrief, game.LevelImmortal, "%s", message)
		}
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
