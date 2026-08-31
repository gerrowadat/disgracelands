// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"github.com/gerrowadat/disgracelands/internal/colour"
	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/obs"
	"github.com/gerrowadat/disgracelands/internal/session"
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
// is.
//
// Called from applyDamage's hook, after victim.Position has already been
// updated — the same point damage() calls dam_message/skill_message from,
// right after its own update_pos and before the position-announcement
// text ("mortally wounded", etc.).
func (s *Server) sendCombatMessage(w *game.Live, attacker, victim *game.Character, weapon *game.Object, dam, attackType int32) {
	// damage()'s dispatch: a miss or a death blow prefers a registered
	// message; anything else always uses the compiled severity table.
	if dam == 0 || victim.Position == game.PosDead {
		if s.sendSkillMessage(w, attacker, victim, weapon, dam, attackType) {
			return
		}
	}
	s.sendDamageMessage(w, attacker, victim, dam, attackType)
}

// sendSkillMessage is skill_message itself (fight.c:703-767) minus
// colour: looks up skillType (a spell/skill number, or — reached only
// from sendCombatMessage — a weapon type) in the loaded misc/messages
// table and sends whichever of Die/Miss/Hit applies, reporting whether it
// found anything to send at all.
//
// Shared by two very different callers with two very different fallback
// postures: sendCombatMessage (the weapon-swing path) falls back to
// dam_message when this returns false, because damage()'s dispatch gives
// weapon attacks that fallback; SkillDamage does not fall back to
// anything, because skill_message alone is the whole story for a kick, a
// bash, a backstab or a spell — a silent swing is the C's own behaviour
// for an attack type nothing is registered for, not a bug to paper over.
func (s *Server) sendSkillMessage(w *game.Live, attacker, victim *game.Character, weapon *game.Object, dam, skillType int32) bool {
	fm, ok := s.text.FightMessages().Pick(skillType, s.rng)
	if !ok {
		return false
	}
	var set game.MsgSet
	switch {
	case dam != 0 && victim.Position == game.PosDead:
		set = fm.Die
	case dam != 0:
		set = fm.Hit
	case attacker != victim:
		set = fm.Miss
	default:
		// skill_message sends nothing for a self-inflicted miss
		// (fight.c:752's `else if (ch != vict)` has no self case).
		return true
	}
	s.sendMsgSet(w, attacker, victim, weapon, set)
	return true
}

// sendMsgSet sends a registered skill_message MsgSet to all three
// audiences, porting its act() calls (fight.c:719-758). An empty field is
// '#', no message for that audience: internal/game.Act's own callers already
// treat an empty format as "send nothing"
// (internal/session/social.go's act()), the same posture here.
//
// The colour is the C's, and so is who does *not* get any: the attacker's
// line is wrapped in CCYEL and the victim's in CCRED (fight.c:679-712), both
// at C_CMP, and the room's is wrapped in nothing at all. A bystander watching
// a fight sees it in plain text on the real server.
func (s *Server) sendMsgSet(w *game.Live, attacker, victim *game.Character, weapon *game.Object, set game.MsgSet) {
	args := game.ActArgs{Actor: attacker, Victim: victim, Obj: weapon}
	if set.Attacker != "" {
		attacker.TellAt(colour.Complete, "{{yellow}}%s{{/}}", w.Act(set.Attacker, args, attacker))
	}
	if set.Victim != "" {
		victim.TellAt(colour.Complete, "{{red}}%s{{/}}", w.Act(set.Victim, args, victim))
	}
	if set.Room != "" {
		s.actToRoomExcept(w, attacker, victim, args, set.Room)
	}
}

// sendDamageMessage is dam_message (fight.c:591-644), colour included: yellow
// to whoever swung, red to whoever was hit, nothing to the room. See
// sendMsgSet.
func (s *Server) sendDamageMessage(w *game.Live, attacker, victim *game.Character, dam, attackType int32) {
	args := game.ActArgs{Actor: attacker, Victim: victim}
	attacker.TellAt(colour.Complete, "{{yellow}}%s{{/}}",
		w.Act(game.DamageMessage(dam, attackType, game.AudienceChar), args, attacker))
	victim.TellAt(colour.Complete, "{{red}}%s{{/}}",
		w.Act(game.DamageMessage(dam, attackType, game.AudienceVictim), args, victim))
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
// messages, which the command that caused it prints in its own words. Kick,
// bash, backstab and every spell all moved to SkillDamage once they got a
// real message of their own to send instead; the two callers left —
// slay's raw_kill and quitting while mortally wounded — print their own
// message and want no other, which is why this stays exported rather
// than folding entirely into SkillDamage.
//
// It exists because until now every command that could hurt somebody applied
// the damage itself — a kick, a bash, a spell — and none of them handled what
// happens when the hit points run out. A kick could kill a mobile and leave it
// standing there dead, with no corpse and no experience for anybody. There is
// one path now, and this is it.
//
// The returned figure is the damage actually taken, after sanctuary and the
// rest, because that is the number a caller with no message of its own to
// send yet still needs, to print its own `[n]`.
func (s *Server) Damage(w *game.Live, attacker, victim *game.Character, amount int32) int32 {
	if victim == nil || victim.Record == nil || s.refusesDamage(w, attacker, victim) {
		return 0
	}
	dam := game.ApplyDamage(amount, victim.Record, combatant{victim})
	s.applyDamage(w, attacker, victim, dam, nil)
	return dam
}

// SkillDamage implements session.Violence: Damage plus the messaging
// damage() gives every non-weapon attack (fight.c:854's `!IS_WEAPON`
// branch) — kick, bash, backstab, and every offensive spell, whose own
// mag_damage ends with `return (damage(ch, victim, dam, spellnum))`
// (magic.c:294), the identical dispatch with the spell number standing
// in for a skill's. amount 0 is a miss, the same "not a separate code
// path" rule the ordinary weapon swing already follows: do_kick/do_bash/
// do_backstab all call damage(ch, vict, 0, SKILL_*) for one
// (act.offensive.c), not a bespoke miss branch of their own.
func (s *Server) SkillDamage(w *game.Live, attacker, victim *game.Character, amount int32, skillType game.SpellID) int32 {
	if victim == nil || victim.Record == nil || s.refusesDamage(w, attacker, victim) {
		return 0
	}
	vf := combatant{victim}
	dam := game.ApplyDamage(amount, victim.Record, vf)
	s.applyDamage(w, attacker, victim, dam, func() {
		// skill_message's own weap = GET_EQ(ch, WEAR_WIELD) (fight.c:711):
		// whatever the attacker happens to be wielding, regardless of
		// whether this particular attack used it — a kick with a sword in
		// hand still carries the sword as $o/$p if a message uses it.
		// skillType narrows here rather than in sendSkillMessage: that
		// function's own domain is the union of spell/skill numbers and
		// TypeHit-scaled weapon types (its doc comment above), because
		// sendCombatMessage reaches it with the second. SkillDamage only
		// ever has the first.
		s.sendSkillMessage(w, attacker, victim, combatant{attacker}.Wielded(), dam, skillType.Number())
	})
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
// position-announcement text. s.hit and SkillDamage both pass one now —
// every non-weapon attacker (kick, bash, backstab, every spell) goes
// through SkillDamage's own hook, the same skill_message call whichever
// of them it is. The two remaining Damage (nil onDamaged) callers, slay
// and quitting while mortally wounded, already print their own message
// and call Damage purely to finish the kill with no attacker — neither
// wants anything skill_message would send.
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

	s.positionAftermath(w, attacker, victim, dam)

	// Somebody stunned or worse stops swinging back.
	if victim.Position <= game.PosStunned && victim.Fighting != nil {
		w.StopFighting(victim)
	}

	if victim.Position == game.PosDead {
		if attacker != nil {
			s.award(w, attacker, victim)
		}
		// mudlog(buf2, BRF, LVL_IMMORT, TRUE) (fight.c:953), and only for
		// a dead *player* — `if (!IS_NPC(victim))` (fight.c:938) — so the
		// mobiles a fight is mostly made of are not logged. The "PKILL: "
		// prefix is a `<DoC>` local addition and goes on when the killer
		// is a player too (fight.c:940-949): the one line an immortal
		// most wanted to see, marked so it could be grepped for. Bare
		// LVL_IMMORT, no GET_INVIS_LEV() — a wizinvis god killing
		// somebody is reported like anybody else.
		if !victim.IsNPC() && attacker != nil {
			prefix := ""
			if !attacker.IsNPC() {
				prefix = "PKILL: "
			}
			room := "nowhere"
			if def := w.Room(victim.Room); def != nil {
				room = def.Name
			}
			s.wizlog(obs.LogBrief, game.LevelImmortal, "%s%s killed by %s at %s",
				prefix, victim.Name, attacker.Name, room)
		}
		s.kill(w, victim, attacker)
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
//
// check_killer's own mudlog line and set_fighting's sanctioned-pkill one
// (fight.c:219-233, :268-273, #213) come back from SetFighting itself as a
// string, "" when there is nothing to log — internal/game has no logger to
// put it in, the same reason gain_exp's mudlog sits at its callers instead
// (see announceGain).
func (s *Server) startFighting(w *game.Live, attacker, victim *game.Character) {
	if attacker == victim {
		return
	}
	if attacker.Position > game.PosStunned && attacker.Fighting == nil {
		if _, message := w.SetFighting(attacker, victim); message != "" {
			s.wizlog(obs.LogBrief, game.LevelImmortal, "%s", message)
		}
	}
	if victim.Position > game.PosStunned && victim.Fighting == nil {
		if _, message := w.SetFighting(victim, attacker); message != "" {
			s.wizlog(obs.LogBrief, game.LevelImmortal, "%s", message)
		}
	}
}

// positionAftermath is the switch on the victim's position that damage()
// ends with (fight.c:876-912): what a blow did, in the words damage() uses,
// and — in the `default` branch, for a victim still on their feet — the two
// calls to do_flee that make wimpiness mean anything.
//
// It was announcePosition and did only the first half until #375. The two
// belong in one function because the C has them in one switch and because
// the MOB_WIMPY flee shares its condition with the bleeding warning: split
// them and the quarter-of-max-hit threshold is written twice, free to
// drift.
func (s *Server) positionAftermath(w *game.Live, attacker, victim *game.Character, dam int32) {
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
		// >= POS_SLEEPING, which is what the C's `default` covers: every
		// position above POS_STUNNED, the four below it each having a case
		// of their own above. Both flees below are inside it, so nobody
		// stunned or worse runs anywhere.

		// "That really did HURT!" (fight.c:895-896), and the two
		// quarters in this branch are not the same quarter. This one is
		// a quarter of your maximum taken *in one blow*, said however
		// healthy you are; the one below is having less than a quarter
		// of your maximum *left*, said however small the blow was. A
		// full-health character taking a huge hit gets the first and not
		// the second; a nearly-dead one taking a scratch gets the second
		// and not the first.
		//
		// dam is the figure after sanctuary and the rest, which is what
		// the C compares too: it halves for AFF_SANCTUARY at the top of
		// damage() and everything downstream sees the reduced number. So
		// sanctuary can take a blow under the threshold and silence this
		// line, which is correct and is the same on the real server.
		if victim.Record != nil && dam > victim.Record.Points.MaxHit/4 {
			victim.Tell("That really did HURT!\r\n")
		}

		if victim.Record != nil && victim.Record.Points.Hit < victim.Record.Points.MaxHit/4 {
			// Red, at C_SPR — the lowest threshold there is, so anybody
			// with colour on at all sees this one (fight.c:851). It is
			// the warning that you are about to die.
			victim.TellAt(colour.Sparse,
				"{{red}}You wish that your wounds would stop BLEEDING so much!{{/}}\r\n")

			// MOB_WIMPY, and it is *inside* the bleeding branch rather
			// than beside it (fight.c:903-904): a wimpy mobile runs at
			// the same quarter-of-max-hit mark that prints the warning,
			// not at a threshold of its own. MOB_FLAGGED is
			// `IS_NPC(ch) && IS_SET(...)` (utils.h:214), which
			// HasMobFlag reproduces by way of MobFlags() returning
			// nothing for a character with no prototype — so this can
			// never fire for a player.
			if hitBySomebodyElse(attacker, victim) && victim.HasMobFlag(game.MobWimpy) {
				s.flee(w, victim)
			}
		}

		// The player's own wimp level (fight.c:906-910), and the whole of
		// #375: `wimpy` set it, `toggle` displayed it and the record saved
		// it, and nothing in the game ever read it, so a player who had
		// asked to run away at 30 hit points stood there and died instead.
		//
		// Every clause is the C's, including the one that cannot fail.
		// `GET_HIT(victim) > 0` is redundant here and it is worth knowing
		// why rather than reading it as a guard against something: this
		// is the `default` branch, so the position is above POS_STUNNED,
		// and update_pos only ever returns one of those for a character
		// with hit points left (game.UpdatePosition's first two cases).
		// Nought or fewer is POS_STUNNED at best and never arrives here.
		// Kept because it is the C's and costs nothing to keep.
		//
		// `victim != ch` is the clause that does work: it is why hurting
		// yourself never makes you run away, and it is how point_update's
		// bleeding and poison are exempt — they are damage(ch, ch, ...)
		// in the C, and suffer() passes the victim as their own attacker
		// here for exactly that reason.
		if !victim.IsNPC() && victim.Record != nil && victim.Record.WimpLevel != 0 &&
			hitBySomebodyElse(attacker, victim) &&
			victim.Record.Points.Hit < victim.Record.WimpLevel &&
			victim.Record.Points.Hit > 0 {
			victim.Tell("You wimp out, and attempt to flee!\r\n")
			s.flee(w, victim)
		}
	}
}

// hitBySomebodyElse is the C's `ch != victim` guard on both of the flees
// above (fight.c:903, :906 — spelled `victim != ch` on the second, the same
// test written the other way round), plus the one thing the C has no
// equivalent of.
//
// damage() in the C always has an attacker: `ch` is a parameter and every
// caller passes one, so `ch != victim` is only ever asking "did I do this to
// myself", which is how point_update's bleeding and poison — damage(ch, ch,
// TYPE_SUFFERING) — are exempt. This port also allows a nil attacker,
// meaning "nothing did this to them", and that is the same kind of thing as
// hurting yourself rather than the opposite of it. Nobody flees from nobody.
//
// Behaviour-neutral today: the only nil-attacker caller is `quit` while
// mortally wounded (session.doQuit), which takes every remaining hit point
// and one more, so the victim is dead and the switch above never reaches
// either branch.
func hitBySomebodyElse(attacker, victim *game.Character) bool {
	return attacker != nil && attacker != victim
}

// flee runs do_flee for somebody who did not type it, which is what
// damage() does at both of the call sites above.
//
// The session is fetched rather than passed because only one thing in
// fleeing wants it — the GMCP room info do_flee's own `look` sends — and
// the character is the thing both callers have. A mobile has no session
// and a linkdead player's is gone; both get a nil, which session.Flee
// takes, and their output falls back to the character's own client the
// way every other message to a bodiless character does.
func (s *Server) flee(w *game.Live, who *game.Character) {
	sess, _ := who.Client.(*session.Session)
	session.Flee(w, who, sess, s.rng)
}

// award gives the killer their experience, porting solo_gain — or, if they
// are in a group, group_gain.
func (s *Server) award(w *game.Live, killer, victim *game.Character) {
	if killer == victim || killer.Record == nil || victim.Record == nil {
		return
	}

	if killer.Grouped() {
		s.awardGroup(w, killer, victim)
		return
	}

	exp, message := game.ExperienceForKill(
		killer.Record, victim.Record, killer.IsNPC(), victim.IsNPC())
	killer.Tell("%s", message)

	if exp != 0 {
		s.announceGain(w, killer, game.GainExperience(killer.Record, exp, s.rng))
	}
}

// awardGroup splits the kill among everybody grouped with the killer who is
// in the room with them, porting group_gain.
//
// Everyone present shares, whether or not they hit anything — a member asleep
// in the corner gets the same cut as the one who did the killing. That is the
// C's rule and it is what makes a group worth being in.
func (s *Server) awardGroup(w *game.Live, killer, victim *game.Character) {
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

		s.announceGain(w, member, game.GainExperience(member.Record, cut, s.rng))
	}
}

// announceGain says what an award did. The cap and the levelling are
// independent — the C sends both, and a kill big enough to be capped is
// exactly the kind that levels somebody, so folding them into one branch
// swallowed "You rise a level!" precisely when it mattered.
func (s *Server) announceGain(w *game.Live, who *game.Character, out game.ExpGain) {
	if out.Capped {
		who.Tell("You can only understand so much...\r\n")
	}
	if out.Levels > 0 && who.Record != nil {
		// mudlog(buf, BRF, MAX(LVL_IMMORT, GET_INVIS_LEV(ch)), TRUE)
		// (limits.c:299-305), inside gain_exp's own `if (is_altered)` and
		// ahead of "You rise a level!". It lives in gain_exp in the C, so
		// every caller gets it; internal/game has no logger to put it in
		// (that is the package's whole rule), so it sits at the callers
		// that can actually raise somebody instead — here, at the
		// cityguard's award (internal/session/specprocs.go), and at
		// `advance` (internal/session/wizchange.go), which reaches
		// gain_exp_regardless' identical copy at limits.c:357.
		s.wizlogInvis(obs.LogBrief, game.LevelImmortal, who,
			"%s advanced %d level%s to level %d.",
			who.Name, out.Levels, pluralS(int(out.Levels)), who.Record.Level)
	}
	// "You rise a level!" and the `<DoC>` cyan broadcast that goes with it,
	// which are two lines of the same block in the C and are one call here
	// (game.Live.AnnounceLevelGain) so that the three call sites this port
	// reaches gain_exp from cannot drift apart. Before #212 this one said
	// the first and none of them said the second.
	w.AnnounceLevelGain(who, out.Levels)
}

// kill removes a dead character from the fight and leaves a body.
//
// killer is carried through to die() for the log and nothing else — every
// other consequence of who did it (the experience, the alignment, the
// PKILL line) has already happened in damage() by the time this is called.
func (s *Server) kill(w *game.Live, victim *game.Character, killer *game.Character) {
	// Everyone swinging at them stops.
	for _, c := range w.Combatants() {
		if c.Fighting == victim {
			w.StopFighting(c)
		}
	}
	w.StopFighting(victim)

	s.die(w, victim, killer)
}

// RawKill implements session.Violence: raw_kill (fight.c:381).
//
// Everything death does with none of what damage() does first — no position
// announcement, no experience, no alignment change. `kill` typed by an
// implementor is the only thing in the C that reaches it this way;
// everything else arrives through damage().
//
// killer is the implementor who typed it. The C has no such argument —
// raw_kill(victim) is its whole signature — because nothing in the C wanted
// to know: the one line that names a killer is in damage(), which raw_kill
// is reached instead of, so an implementor's `kill` is unattributed
// everywhere the C logs anything. That is exactly the gap #370 is about,
// and it is a gap in the server's own log rather than in anything a player
// sees, so closing it takes nothing away from the C's behaviour.
func (s *Server) RawKill(w *game.Live, killer, victim *game.Character) {
	if victim == nil {
		return
	}
	s.kill(w, victim, killer)
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
