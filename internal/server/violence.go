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

// hit is one swing and everything that follows from it, including the
// message — dam_message/skill_message's weapon-type half
// (fight.c:591-620, :703-767), ported in full for this one call site. A
// miss is not a separate code path: the C calls damage(ch, victim, 0,
// w_type) for one (fight.c:1067-1068), the same function a real hit goes
// through, which is why this does too.
func (s *Server) hit(w *game.Live, attacker, victim *game.Character) {
	if attacker.Record == nil || victim.Record == nil {
		return
	}
	if s.refusesDamage(w, attacker, victim) {
		return
	}

	af, vf := combatant{attacker}, combatant{victim}
	swing := game.Attack(attacker.Record, victim.Record, af, vf, s.rng)
	attackType := game.WeaponAttackType(af.Wielded())

	amount := swing.Damage
	if !swing.Hit {
		amount = 0
	}
	dam := game.ApplyDamage(amount, victim.Record, vf)

	s.applyDamage(w, attacker, victim, dam, func() {
		s.sendCombatMessage(w, attacker, victim, af.Wielded(), dam, attackType)
	})
}

// sendCombatMessage is the messaging half of damage() (fight.c:855-871),
// for a weapon attack — the only kind an ordinary combat-round swing ever
// is, which is the only case this covers (kick, bash, backstab and every
// spell still print their own message, unaffected — see Damage's own doc
// comment).
//
// Called from applyDamage's hook, after victim.Position has already been
// updated — the same point damage() calls dam_message/skill_message from,
// right after its own update_pos and before the position-announcement
// text ("mortally wounded", etc.).
func (s *Server) sendCombatMessage(w *game.Live, attacker, victim *game.Character, weapon *game.Object, dam, attackType int32) {
	// damage()'s dispatch: a miss or a death blow prefers a registered
	// message; anything else always uses the compiled severity table.
	if dam == 0 || victim.Position == game.PosDead {
		if fm, ok := s.text.FightMessages().Pick(attackType, s.rng); ok {
			var set game.MsgSet
			switch {
			case dam != 0:
				set = fm.Die
			case attacker != victim:
				set = fm.Miss
			default:
				// skill_message sends nothing for a self-inflicted miss
				// (fight.c:752's `else if (ch != vict)` has no self case)
				// — not reachable through performViolence in practice,
				// kept for fidelity to the guard itself.
				return
			}
			s.sendMsgSet(w, attacker, victim, weapon, set)
			return
		}
	}
	s.sendDamageMessage(w, attacker, victim, dam, attackType)
}

// sendMsgSet sends a registered skill_message MsgSet to all three
// audiences, porting its act() calls (fight.c:719-758) minus colour —
// nothing in this port emits colour yet (docs/deviations.md). An empty
// field is '#', no message for that audience: internal/game.Act's own
// callers already treat an empty format as "send nothing"
// (internal/session/social.go's act()), the same posture here.
func (s *Server) sendMsgSet(w *game.Live, attacker, victim *game.Character, weapon *game.Object, set game.MsgSet) {
	args := game.ActArgs{Actor: attacker, Victim: victim, Obj: weapon}
	if set.Attacker != "" {
		attacker.Tell("%s", w.Act(set.Attacker, args, attacker))
	}
	if set.Victim != "" {
		victim.Tell("%s", w.Act(set.Victim, args, victim))
	}
	if set.Room != "" {
		s.actToRoomExcept(w, attacker, victim, args, set.Room)
	}
}

// sendDamageMessage is dam_message (fight.c:591-620) minus colour.
func (s *Server) sendDamageMessage(w *game.Live, attacker, victim *game.Character, dam, attackType int32) {
	args := game.ActArgs{Actor: attacker, Victim: victim}
	attacker.Tell("%s", w.Act(game.DamageMessage(dam, attackType, game.AudienceChar), args, attacker))
	victim.Tell("%s", w.Act(game.DamageMessage(dam, attackType, game.AudienceVictim), args, victim))
	s.actToRoomExcept(w, attacker, victim, args, game.DamageMessage(dam, attackType, game.AudienceRoom))
}

// actToRoomExcept renders an Act message once per listener in a room —
// the codes resolve differently for each of them — skipping the two
// combatants themselves. TO_NOTVICT (fight.c).
func (s *Server) actToRoomExcept(w *game.Live, exclude1, exclude2 *game.Character, args game.ActArgs, format string) {
	for _, other := range w.Occupants(exclude1.Room) {
		if other == exclude1 || other == exclude2 {
			continue
		}
		other.Tell("%s", w.Act(format, args, other))
	}
}

// Damage implements session.Violence: everything damage() does apart from the
// messages, which the command that caused it prints in its own words.
//
// It exists because until now every command that could hurt somebody applied
// the damage itself — a kick, a bash, a spell — and none of them handled what
// happens when the hit points run out. A kick could kill a mobile and leave it
// standing there dead, with no corpse and no experience for anybody. There is
// one path now, and this is it.
//
// The returned figure is the damage actually taken, after sanctuary and the
// rest, because that is the number the caller prints in its `[n]`.
func (s *Server) Damage(w *game.Live, attacker, victim *game.Character, amount int32) int32 {
	if victim == nil || victim.Record == nil || s.refusesDamage(w, attacker, victim) {
		return 0
	}
	dam := game.ApplyDamage(amount, victim.Record, combatant{victim})
	s.applyDamage(w, attacker, victim, dam, nil)
	return dam
}

// refusesDamage is the front of damage(): the checks that stop a blow before
// anything is computed, in the C's order (fight.c:795 and :802).
//
// One function called from both entry points, because the C has one. `hit`
// had its own copy of the peaceful-room check, and the shopkeeper check added
// beside it in Damage alone did nothing for a punch — which is exactly the
// divergence this shape prevents.
func (s *Server) refusesDamage(w *game.Live, attacker, victim *game.Character) bool {
	return s.refusedByPeace(w, attacker, victim) ||
		s.refusedByShopkeeper(w, attacker, victim)
}

// refusedByPeace is damage()'s peaceful-room check. It lives here rather than
// in each command because the C puts it here: every route to hurting somebody
// passes through damage(), so one check covers all of them.
//
// Note whose room is tested — the *attacker's*. They are always in the same
// room in practice, but a spell cast from outside one would be stopped by the
// caster's peace rather than the victim's.
func (s *Server) refusedByPeace(w *game.Live, attacker, victim *game.Character) bool {
	if attacker == nil || attacker == victim {
		return false
	}
	room := w.Room(attacker.Room)
	if room == nil || !room.Flags.Has(game.RoomPeaceful) {
		return false
	}
	attacker.Tell("This room just has such a peaceful, easy feeling...\r\n")
	return true
}

// applyDamage is the tail of damage(): take the hit points off, start the
// fight, work out what position that leaves them in, and deal with the body.
//
// onDamaged, if not nil, runs at the exact point damage() itself calls
// dam_message/skill_message — right after update_pos, before the
// position-announcement text. Only s.hit passes one: every other caller
// (kick, bash, spells, via Damage) prints its own message elsewhere and
// leaves this nil, unaffected by anything a weapon-swing message does.
func (s *Server) applyDamage(w *game.Live, attacker, victim *game.Character, dam int32, onDamaged func()) {
	victim.Record.Points.Hit -= dam

	if attacker != nil {
		s.startFighting(w, attacker, victim)
	}

	// Attacking a pet ends the arrangement: "If you attack a pet, it hates
	// your guts", says the C, and stop_follower's charmed branch is where
	// "You realize that $N is a jerk!" comes from.
	if attacker != nil && victim.Master == attacker {
		s.stopFollowing(w, victim)
	}

	victim.Position = game.UpdatePosition(victim.Record, victim.Position)

	if onDamaged != nil {
		onDamaged()
	}

	s.announcePosition(w, victim)

	// Somebody stunned or worse stops swinging back.
	if victim.Position <= game.PosStunned && victim.Fighting != nil {
		w.StopFighting(victim)
	}

	if victim.Position == game.PosDead {
		if attacker != nil {
			s.award(attacker, victim)
		}
		s.kill(w, victim)
	}
}

// Swing implements session.Violence: one attack, right now, rather than
// waiting for the round. `hit` and `assist` both do this.
func (s *Server) Swing(w *game.Live, attacker, victim *game.Character) {
	s.hit(w, attacker, victim)
}

// stopFollowing detaches a follower and says so, which the server needs
// because a blow can end the arrangement. The wording is stop_follower's, and
// the charmed version of it is the one anybody remembers.
func (s *Server) stopFollowing(w *game.Live, follower *game.Character) {
	leader := follower.Master
	if leader == nil {
		return
	}

	if follower.Charmed() {
		follower.Tell("You realize that %s is a jerk!\r\n", leader.Name)
		leader.Tell("%s hates your guts!\r\n", follower.Name)
	} else {
		follower.Tell("You stop following %s.\r\n", leader.Name)
		leader.Tell("%s stops following you.\r\n", follower.Name)
	}
	w.StopFollowing(follower)
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
		s.announceGain(killer, game.GainExperience(killer.Record, exp, s.rng))
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

		s.announceGain(member, game.GainExperience(member.Record, cut, s.rng))
	}
}

// announceGain says what an award did. The cap and the levelling are
// independent — the C sends both, and a kill big enough to be capped is
// exactly the kind that levels somebody, so folding them into one branch
// swallowed "You rise a level!" precisely when it mattered.
func (s *Server) announceGain(who *game.Character, out game.ExpGain) {
	if out.Capped {
		who.Tell("You can only understand so much...\r\n")
	}
	switch {
	case out.Levels == 1:
		who.Tell("You rise a level!\r\n")
	case out.Levels > 1:
		who.Tell("You rise %d levels!\r\n", out.Levels)
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

// refusedByShopkeeper is ok_damage_shopkeeper (shop.c:941), checked in
// damage() right after the peaceful-room test (fight.c:802).
//
// A shopkeeper whose shop is not flagged WILL_FIGHT cannot be hurt: they slap
// you and tell you to get out. Being in damage() rather than in `kill` is the
// point — a fireball is refused the same way a punch is.
//
// The charm check comes first and inverts the whole thing: a *charmed*
// shopkeeper takes damage normally, because the alternative is an invincible
// mobile fighting on somebody's behalf.
func (s *Server) refusedByShopkeeper(w *game.Live, attacker, victim *game.Character) bool {
	if attacker == nil || victim == nil || attacker == victim || !victim.IsNPC() {
		return false
	}
	if victim.Record != nil && victim.Record.AffectFlags.Has(game.AffectCharm) {
		return false
	}
	shop := w.ShopFor(victim)
	if shop == nil || shop.Flags.Has(game.ShopWillFight) {
		return false
	}

	// do_action(victim, GET_NAME(ch), cmd_slap, 0), then a tell.
	attacker.Tell("%s slaps you in the face!\r\n", victim.Name)
	for _, other := range w.Occupants(victim.Room) {
		if other != attacker && other != victim {
			other.Tell("%s slaps %s in the face!\r\n", victim.Name, attacker.Name)
		}
	}
	attacker.Tell("%s tells you, 'Get out of here before I call the guards!'\r\n", victim.Name)
	return true
}
