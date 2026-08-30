// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import (
	"fmt"
	"sort"
	"time"
)

// Who is fighting whom, and what a kill is worth.
//
// The C keeps a `combat_list` threaded through char_data's next_fighting
// pointer, with a global `next_combat_list` so that a character extracted
// mid-iteration does not strand the loop. That is a linked list maintained by
// hand for the sake of one iteration per two seconds; here the world keeps the
// set and hands out a snapshot.

// PKAllowed is config.c's pk_allowed (config.c:55). Off, as it is on the
// archive's own settings, which is why check_killer and the friendly-fire
// refusals scattered through internal/session (offensive.go, spells.go) all
// have something to do. A local constant rather than a GameTuning field for
// the same reason as the rest of config.c that tuning.go's own doc comment
// lists beside it: this is one that stays a constant on purpose.
const PKAllowed = false

// SetFighting starts a fight, porting set_fighting (fight.c:237-276).
//
// Returns false if the character is already fighting — the C core-dumps in
// that case, which is a strong statement about how much it expects it not
// to happen — and the mudlog line the caller should hand to wizlog at
// obs.LogBrief/LevelImmortal (mirroring both of set_fighting's own
// `mudlog(buf, BRF, LVL_IMMORT, TRUE)` call sites), or "" if nothing
// happened worth logging. internal/game has no logger of its own to call
// wizlog from directly — see docs/deviations.md on gain_exp for the same
// shape.
func (l *Live) SetFighting(c, victim *Character) (bool, string) {
	if c == nil || victim == nil || c == victim || c.Fighting != nil {
		return false, ""
	}

	c.Fighting = victim
	c.Position = PosFighting

	// Being attacked wakes you up. The C removes the sleep spell rather than
	// just the flag, which matters when the spell would otherwise re-apply
	// it.
	if c.Record != nil {
		c.Record.AffectFlags = c.Record.AffectFlags.Without(AffectSleep)
	}

	if l.fighting == nil {
		l.fighting = map[*Character]bool{}
	}
	l.nextFightSeq++
	c.fightSeq = l.nextFightSeq
	l.fighting[c] = true

	return true, l.checkKillerOrSanction(c, victim)
}

// checkKillerOrSanction is set_fighting's own tail (fight.c:256-274): a room
// flagged ROOM_PKILL is a sanctioned brawl — "Okay! Let's get it on!" to the
// attacker, "Looks like %s wants a little..." to the victim, and a mudlog
// line if both are players — and everywhere else, unless the mud runs with
// pk_allowed on, an unprovoked attack on another player runs [checkKiller]
// instead.
func (l *Live) checkKillerOrSanction(c, victim *Character) string {
	if room := l.Room(c.Room); room != nil && room.Flags.Has(RoomPKill) {
		c.Tell("Okay! Let's get it on!\r\n")
		victim.Tell("Looks like %s wants a little...\r\n", c.Name)

		// "Only mudlog if both protagonists are players" (fight.c:269).
		if !c.IsNPC() && !victim.IsNPC() {
			return fmt.Sprintf("%s started sanctioned pkill on %s at %s.",
				c.Name, victim.Name, room.Name)
		}
		return ""
	}

	if PKAllowed {
		return ""
	}
	return l.checkKiller(c, victim)
}

// checkKiller is check_killer (fight.c:219-233): the first unprovoked blow
// against another player flags the attacker PLR_KILLER, once — a victim who
// is already a killer or a thief is fair game and sets nothing, and an
// attacker who already carries the flag has nothing more to set. IS_NPC and
// self-attack are excluded too, kept even though every current call site
// through [Live.SetFighting] has already ruled self-attack out, because
// check_killer is the C's own choke point for this and a future caller
// should not have to remember to.
func (l *Live) checkKiller(c, victim *Character) string {
	if victim.Record != nil && victim.Record.PlayerFlags.HasAny(PlayerKiller, PlayerThief) {
		return ""
	}
	if (c.Record != nil && c.Record.PlayerFlags.Has(PlayerKiller)) ||
		c.IsNPC() || victim.IsNPC() || c == victim {
		return ""
	}

	c.Record.PlayerFlags = c.Record.PlayerFlags.With(PlayerKiller)
	c.Tell("If you want to be a PLAYER KILLER, so be it...\r\n")

	name := ""
	if room := l.Room(victim.Room); room != nil {
		name = room.Name
	}
	return fmt.Sprintf("PC Killer bit set on %s for initiating attack on %s at %s.",
		c.Name, victim.Name, name)
}

// StopFighting takes a character out of combat, porting stop_fighting.
func (l *Live) StopFighting(c *Character) {
	if c == nil {
		return
	}
	delete(l.fighting, c)
	c.Fighting = nil
	c.Position = PosStanding
	if c.Record != nil {
		c.Position = UpdatePosition(c.Record, c.Position)
	}
}

// Combatants returns everyone currently fighting.
//
// A snapshot, because the round removes people from the fight as they die and
// iterating the live set while it changes is the bug `next_combat_list`
// exists to work around in the C.
func (l *Live) Combatants() []*Character {
	out := make([]*Character, 0, len(l.fighting))
	for c := range l.fighting {
		out = append(out, c)
	}
	// The map's iteration order is random, and a combat round whose order of
	// blows varies run to run cannot be compared against anything. Ordering
	// by the sequence characters joined the fight is the closest thing to the
	// C's list, and it makes a round reproducible.
	sort.Slice(out, func(i, j int) bool { return out[i].fightSeq < out[j].fightSeq })
	return out
}

// ExperienceForKill is what a kill is worth, porting solo_gain (fight.c).
//
// **Player kills are worth nothing.** That is a local rule, not stock: the C
// here sets the award to zero unless one side or the other is a mobile, and
// then skips gain_exp entirely for the same reason. The message a player gets
// for killing another player is "You receive no experience! HA!." — which is
// the tone of the thing.
func ExperienceForKill(killer, victim *PlayerRecord, killerIsNPC, victimIsNPC bool) (award int32, message string) {
	exp := min(MaxExpGainPerKill, victim.Points.Exp/3)

	// A level-difference bonus, capped differently for mobiles and players:
	// a mobile counts at most four levels of difference, a player eight.
	cap := int32(8)
	if killerIsNPC {
		cap = 4
	}
	exp += max(0, (exp*min(cap, victim.Level-killer.Level))/8)

	if killerIsNPC || victimIsNPC {
		exp = max(exp, 1)
	} else {
		exp = 0
	}

	switch {
	case exp > 1:
		// The message reports the *capped* figure, which is what the player
		// will actually receive once gain_exp applies the tenth-of-a-band
		// limit.
		band := LevelExperience(killer.Class, killer.Level+1) -
			LevelExperience(killer.Class, killer.Level)
		shown := exp
		if limit := band / 10; shown > limit {
			shown = limit
		}
		message = fmt.Sprintf("You receive %d experience points.\r\n", shown)
	case exp == 0:
		message = "You receive no experience! HA!.\r\n"
	default:
		message = "You receive one lousy experience point.\r\n"
	}

	// gain_exp is only called when one side is a mobile, so a player kill
	// awards nothing at all rather than awarding zero.
	if !killerIsNPC && !victimIsNPC {
		return 0, message
	}
	return exp, message
}

// GroupShare is one member's cut of a kill, porting group_gain's arithmetic
// (fight.c:468).
//
//	tot_gain = (GET_EXP(victim) / 3) + tot_members - 1
//	base     = MAX(1, tot_gain / tot_members)
//
// The `+ tot_members - 1` is the standard trick for making integer division
// round *up*, so three people splitting ten points get four each — twelve
// points out of a ten point kill. A group therefore earns more in total than
// a soloist would, which is the incentive to group, and it looks accidental
// until you notice it is not.
//
// Note what is missing compared with solo_gain: no level-difference bonus at
// all. Killing something far above your level is worth more alone than it is
// in a group.
func GroupShare(victim *PlayerRecord, victimIsNPC bool, members int32) int32 {
	if members < 1 {
		return 0
	}

	total := (victim.Points.Exp / 3) + members - 1
	// Killing a player cannot mint experience out of nothing.
	if !victimIsNPC {
		total = min(MaxExpLossPerDeath*2/3, total)
	}
	return max(1, total/members)
}

// GroupShareMessage is what perform_group_gain tells one member, which is
// worded differently from the solo message and reports the *capped* figure.
func GroupShareMessage(member *PlayerRecord, share int32) string {
	switch {
	case share > 1:
		band := LevelExperience(member.Class, member.Level+1) -
			LevelExperience(member.Class, member.Level)
		if limit := band / 10; share > limit {
			share = limit
		}
		return fmt.Sprintf("You receive your share of experience -- %d points.\r\n", share)
	case share == 0:
		return "You receive your share of experience -- Nothing! Ha!!\r\n"
	}
	return "You receive your share of experience -- one measly little point!\r\n"
}

// Wait sets a character's lag, porting WAIT_STATE.
//
// The unit is combat rounds — PULSE_VIOLENCE — because that is what every
// call site in the C counts in: `WAIT_STATE(ch, PULSE_VIOLENCE * 3)`.
func (c *Character) Wait(rounds int32, roundLength time.Duration) {
	if c == nil || rounds <= 0 {
		return
	}
	until := time.Now().Add(time.Duration(rounds) * roundLength)
	if until.After(c.BusyUntil) {
		c.BusyUntil = until
	}
}

// WaitRemaining is how long until they may act again.
func (c *Character) WaitRemaining() time.Duration {
	if c == nil {
		return 0
	}
	return time.Until(c.BusyUntil)
}

// Weapon attack types, from spells.h. A weapon's fourth value is its type,
// stored as an offset from TYPE_HIT — so a piercing weapon carries 11.
const (
	AttackHit      int32 = 0
	AttackSting    int32 = 1
	AttackWhip     int32 = 2
	AttackSlash    int32 = 3
	AttackBite     int32 = 4
	AttackBludgeon int32 = 5
	AttackCrush    int32 = 6
	AttackPound    int32 = 7
	AttackClaw     int32 = 8
	AttackMaul     int32 = 9
	AttackThrash   int32 = 10
	AttackPierce   int32 = 11
	AttackBlast    int32 = 12
	AttackPunch    int32 = 13
	AttackStab     int32 = 14
)

// BackstabMultiplier is backstab_mult (class.c), and the reason a thief is
// worth having. It runs 2 through 6 across the mortal levels and then jumps
// to 20 for an immortal — not 7, not a continuation of the curve.
func BackstabMultiplier(level int32) int32 {
	switch {
	case level <= 0:
		return 1
	case level <= 7:
		return 2
	case level <= 13:
		return 3
	case level <= 20:
		return 4
	case level <= 28:
		return 5
	case level < LevelImmortal:
		return 6
	}
	return 20
}
