// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"github.com/gerrowadat/disgracelands/internal/game"
)

// The combat round, ported from perform_violence and the messaging half of
// damage() (fight.c).

// pulseViolence is PULSE_VIOLENCE: a round every two seconds.
const pulseViolence = 2 * pulsesPerSecond

// combatant answers what the combat formulas need about a character.
type combatant struct {
	c *game.Character
}

func (f combatant) IsNPC() bool { return f.c.IsNPC() }

func (f combatant) Position() game.Position { return f.c.Position }

func (f combatant) Wielded() *game.Object { return f.c.Equipment[game.WearWield] }

func (f combatant) Sanctuary() bool {
	return f.c.Record != nil && f.c.Record.AffectFlags.Has(game.AffectSanctuary)
}

// performViolence runs one round, porting perform_violence.
func (s *Server) performViolence(w *game.Live) {
	for _, c := range w.Combatants() {
		victim := c.Fighting

		// Somebody fled, died, or was moved. The C checks both.
		if victim == nil || victim.Room != c.Room {
			w.StopFighting(c)
			continue
		}

		if c.Position < game.PosFighting {
			if c.IsNPC() {
				// A mobile gets to its feet rather than sitting the fight
				// out, which is why a sleeping mobile is dangerous to wake.
				c.Position = game.PosFighting
				s.toRoomExcept(w, c, "%s scrambles to their feet!\r\n", c.Name)
			} else {
				c.Tell("You can't fight while sitting!!\r\n")
				continue
			}
		}

		s.hit(w, c, victim)
	}
}

// hit is one swing and everything that follows from it.
func (s *Server) hit(w *game.Live, attacker, victim *game.Character) {
	if attacker.Record == nil || victim.Record == nil {
		return
	}

	af, vf := combatant{attacker}, combatant{victim}
	swing := game.Attack(attacker.Record, victim.Record, af, vf, s.rng)

	if !swing.Hit {
		attacker.Tell("You miss %s.\r\n", victim.Name)
		victim.Tell("%s misses you.\r\n", attacker.Name)
		s.toRoomExcept(w, attacker, "%s misses %s.\r\n", attacker.Name, victim.Name, victim)
		s.startFighting(w, attacker, victim)
		return
	}

	dam := game.ApplyDamage(swing.Damage, victim.Record, vf)
	victim.Record.Points.Hit -= dam

	attacker.Tell("You hit %s. [%d]\r\n", victim.Name, dam)
	victim.Tell("%s hits you. [%d]\r\n", attacker.Name, dam)
	s.toRoomExcept(w, attacker, "%s hits %s.\r\n", attacker.Name, victim.Name, victim)

	s.startFighting(w, attacker, victim)

	victim.Position = game.UpdatePosition(victim.Record, victim.Position)
	s.announcePosition(w, victim)

	// Somebody stunned or worse stops swinging back.
	if victim.Position <= game.PosStunned && victim.Fighting != nil {
		w.StopFighting(victim)
	}

	if victim.Position == game.PosDead {
		s.award(attacker, victim)
		s.kill(w, victim)
	}
}

// startFighting puts both parties into the fight if they are not already,
// porting the middle of damage().
func (s *Server) startFighting(w *game.Live, attacker, victim *game.Character) {
	if attacker == victim {
		return
	}
	if attacker.Position > game.PosStunned && attacker.Fighting == nil {
		w.SetFighting(attacker, victim)
	}
	if victim.Position > game.PosStunned && victim.Fighting == nil {
		w.SetFighting(victim, attacker)
	}
}

// announcePosition says what a blow did, in the words damage() uses.
func (s *Server) announcePosition(w *game.Live, victim *game.Character) {
	switch victim.Position {
	case game.PosMortallyWounded:
		victim.Tell("You are mortally wounded, and will die soon, if not aided.\r\n")
		s.toRoomExcept(w, victim, "%s is mortally wounded, and will die soon, if not aided.\r\n", victim.Name)
	case game.PosIncapacitated:
		// "incapacitated an will slowly die" — the typo is the C's, and it is
		// what players read for seven years.
		victim.Tell("You are incapacitated an will slowly die, if not aided.\r\n")
		s.toRoomExcept(w, victim, "%s is incapacitated and will slowly die, if not aided.\r\n", victim.Name)
	case game.PosStunned:
		victim.Tell("You're stunned, but will probably regain consciousness again.\r\n")
		s.toRoomExcept(w, victim, "%s is stunned, but will probably regain consciousness again.\r\n", victim.Name)
	case game.PosDead:
		victim.Tell("You are dead!  Sorry...\r\n")
		s.toRoomExcept(w, victim, "%s is dead!  R.I.P.\r\n", victim.Name)
	default:
		if victim.Record != nil && victim.Record.Points.Hit < victim.Record.Points.MaxHit/4 {
			victim.Tell("You wish that your wounds would stop BLEEDING so much!\r\n")
		}
	}
}

// award gives the killer their experience, porting solo_gain — or, if they
// are in a group, group_gain.
func (s *Server) award(killer, victim *game.Character) {
	if killer == victim || killer.Record == nil || victim.Record == nil {
		return
	}

	if killer.Grouped() {
		s.awardGroup(killer, victim)
		return
	}

	exp, message := game.ExperienceForKill(
		killer.Record, victim.Record, killer.IsNPC(), victim.IsNPC())
	killer.Tell("%s", message)

	if exp != 0 {
		if out := game.GainExperience(killer.Record, exp, s.rng); out.Capped {
			killer.Tell("You can only understand so much...\r\n")
		} else if out.Levels == 1 {
			killer.Tell("You rise a level!\r\n")
		} else if out.Levels > 1 {
			killer.Tell("You rise %d levels!\r\n", out.Levels)
		}
	}
}

// awardGroup splits the kill among everybody grouped with the killer who is
// in the room with them, porting group_gain.
//
// Everyone present shares, whether or not they hit anything — a member asleep
// in the corner gets the same cut as the one who did the killing. That is the
// C's rule and it is what makes a group worth being in.
func (s *Server) awardGroup(killer, victim *game.Character) {
	members := killer.GroupMembers(killer.Room)
	if len(members) == 0 {
		return
	}

	share := game.GroupShare(victim.Record, victim.IsNPC(), int32(len(members))) //nolint:gosec // a room's worth
	for _, member := range members {
		if member.Record == nil {
			continue
		}

		// A player killing a player earns nothing, and the message says so.
		cut := share
		if !member.IsNPC() && !victim.IsNPC() {
			cut = 0
		}
		member.Tell("%s", game.GroupShareMessage(member.Record, cut))
		if cut == 0 {
			continue
		}

		if out := game.GainExperience(member.Record, cut, s.rng); out.Capped {
			member.Tell("You can only understand so much...\r\n")
		} else if out.Levels == 1 {
			member.Tell("You rise a level!\r\n")
		} else if out.Levels > 1 {
			member.Tell("You rise %d levels!\r\n", out.Levels)
		}
	}
}

// kill removes a dead character from the fight and leaves a body.
func (s *Server) kill(w *game.Live, victim *game.Character) {
	// Everyone swinging at them stops.
	for _, c := range w.Combatants() {
		if c.Fighting == victim {
			w.StopFighting(c)
		}
	}
	w.StopFighting(victim)

	s.die(w, victim)
}

// toRoomExcept tells everyone in a character's room except them, and except
// anyone else named.
func (s *Server) toRoomExcept(w *game.Live, c *game.Character, format string, args ...any) {
	var excluded []*game.Character
	if n := len(args); n > 0 {
		if extra, ok := args[n-1].(*game.Character); ok {
			excluded = append(excluded, extra)
			args = args[:n-1]
		}
	}

	for _, other := range w.Occupants(c.Room) {
		if other == c {
			continue
		}
		skip := false
		for _, e := range excluded {
			if other == e {
				skip = true
			}
		}
		if !skip {
			other.Tell(format, args...)
		}
	}
}
