// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package session

import (
	"strings"
	"time"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// The four combat skills, ported from act.offensive.c.
//
// All four share a shape — check you know it, find a victim, roll
// number(1, 101) against your percentage — and all four differ in the
// details, which is where the game is. Each also sets a wait state, and the
// wait is most of what balances them: a kick is three combat rounds of lag
// for level/2 damage.

// doKick, porting do_kick.
//
// The roll is the interesting part: `((10 - ac/10) * 2) + number(1, 101)`.
// The victim's armour class is *added to the difficulty*, so kicking a
// well-armoured target is harder — which is not true of bash or backstab, and
// makes kick the only one of the four that cares what the victim is wearing.
func doKick(c *Context) error {
	rec := c.Character.Record
	if c.Character.IsNPC() || rec == nil || rec.Skills[game.SkillKick] == 0 {
		c.Send("You have no idea how.\r\n")
		return nil
	}

	victim := c.skillTarget(c.Arg, "Kick who?\r\n")
	if victim == nil {
		return nil
	}
	if victim == c.Character {
		c.Send("Aren't we funny today...\r\n")
		return nil
	}

	armour := game.ComputeArmorClass(victim.Record, combatantOf(victim))
	percent := (10-armour/10)*2 + c.RNG.Number(1, 101)

	if percent > rec.Skills[game.SkillKick] {
		c.Violence.SkillDamage(c.World, c.Character, victim, 0, game.SkillKick)
	} else {
		c.Violence.SkillDamage(c.World, c.Character, victim, rec.Level/2, game.SkillKick)
	}

	c.Character.Wait(3, c.roundLength())
	return nil
}

// doBash, porting do_bash. Knocks the victim over on a hit and knocks the
// basher over on a miss.
func doBash(c *Context) error {
	rec := c.Character.Record
	if c.Character.IsNPC() || rec == nil || rec.Skills[game.SkillBash] == 0 {
		c.Send("You have no idea how.\r\n")
		return nil
	}

	room := c.World.Room(c.Character.Room)
	if room != nil && room.Flags.Has(game.RoomPeaceful) {
		c.Send("This room just has such a peaceful, easy feeling...\r\n")
		return nil
	}
	if c.Character.Equipment[game.WearWield] == nil {
		c.Send("You need to wield a weapon to make it a success.\r\n")
		return nil
	}

	victim := c.skillTarget(c.Arg, "Bash who?\r\n")
	if victim == nil {
		return nil
	}
	if victim == c.Character {
		c.Send("Aren't we funny today...\r\n")
		return nil
	}

	percent := c.RNG.Number(1, 101)
	if victim.HasMobFlag(game.MobNoBash) {
		// A tree cannot be knocked over, so the roll is made to fail.
		percent = 101
	}

	if percent > rec.Skills[game.SkillBash] {
		c.Violence.SkillDamage(c.World, c.Character, victim, 0, game.SkillBash)
		// Missing a bash puts *you* on the floor.
		c.Character.Position = game.PosSitting
	} else {
		c.Violence.SkillDamage(c.World, c.Character, victim, 1, game.SkillBash)
		// Only if they are still here: the C's comment explains that a victim
		// who wimps out flees before this runs, and setting them sitting
		// first would stop the bash landing at all.
		if victim.Room == c.Character.Room && victim.Position > game.PosStunned {
			victim.Position = game.PosSitting
			victim.Wait(1, c.roundLength())
		}
	}

	c.Character.Wait(2, c.roundLength())
	return nil
}

// doBackstab, porting do_backstab.
//
// The multiplier is the whole point: backstab_mult scales from 2 at low level
// to 6 near immortality, and 20 for an immortal.
func doBackstab(c *Context) error {
	rec := c.Character.Record
	if c.Character.IsNPC() || rec == nil || rec.Skills[game.SkillBackstab] == 0 {
		c.Send("You have no idea how to do that.\r\n")
		return nil
	}

	if strings.TrimSpace(c.Arg) == "" {
		c.Send("Backstab who?\r\n")
		return nil
	}
	victim := c.findInRoom(c.Arg)
	if victim == nil {
		c.Send("Backstab who?\r\n")
		return nil
	}
	if victim == c.Character {
		c.Send("How can you sneak up on yourself?\r\n")
		return nil
	}

	weapon := c.Character.Equipment[game.WearWield]
	if weapon == nil {
		c.Send("You need to wield a weapon to make it a success.\r\n")
		return nil
	}
	// Value 3 is the weapon's attack type, and only a piercing one will do.
	if weapon.Values[3] != game.AttackPierce {
		c.Send("Only piercing weapons can be used for backstabbing.\r\n")
		return nil
	}
	if victim.Fighting != nil {
		c.Send("You can't backstab a fighting person -- they're too alert!\r\n")
		return nil
	}

	// An aware mobile turns and attacks instead of being stabbed.
	if victim.HasMobFlag(game.MobAware) && victim.Position.Awake() {
		c.Send("You notice %s lunging at you!\r\n", victim.Name)
		victim.Tell("You notice %s lunging at you!\r\n", c.Character.Name)
		c.World.SetFighting(victim, c.Character)
		c.Character.Wait(2, c.roundLength())
		return nil
	}

	percent := c.RNG.Number(1, 101)
	if victim.Position.Awake() && percent > rec.Skills[game.SkillBackstab] {
		c.Violence.SkillDamage(c.World, c.Character, victim, 0, game.SkillBackstab)
	} else {
		// A sleeping victim is stabbed regardless of the roll.
		damage := game.BackstabMultiplier(rec.Level) *
			(game.Strength(rec.Abilities.Strength, rec.Abilities.StrengthPercentile).ToDamage +
				rec.Points.DamRoll + c.RNG.Dice(weapon.Values[1], weapon.Values[2]))
		c.Violence.SkillDamage(c.World, c.Character, victim, max(1, damage), game.SkillBackstab)
	}

	c.Character.Wait(2, c.roundLength())
	return nil
}

// doRescue, porting do_rescue: take somebody else's fight onto yourself.
func doRescue(c *Context) error {
	rec := c.Character.Record
	if c.Character.IsNPC() || rec == nil || rec.Skills[game.SkillRescue] == 0 {
		c.Send("You have no idea how to do that.\r\n")
		return nil
	}

	if strings.TrimSpace(c.Arg) == "" {
		c.Send("Whom do you want to rescue?\r\n")
		return nil
	}
	victim := c.findInRoom(c.Arg)
	if victim == nil {
		c.Send("Whom do you want to rescue?\r\n")
		return nil
	}
	if victim == c.Character {
		c.Send("What about fleeing instead?\r\n")
		return nil
	}
	if c.Character.Fighting == victim {
		c.Send("How can you rescue someone you are trying to kill?\r\n")
		return nil
	}

	// Whoever is attacking them.
	var attacker *game.Character
	for _, other := range c.World.Occupants(c.Character.Room) {
		if other.Fighting == victim {
			attacker = other
			break
		}
	}
	if attacker == nil {
		c.Send("But nobody is fighting %s!\r\n", victim.Name)
		return nil
	}

	if c.RNG.Number(1, 101) > rec.Skills[game.SkillRescue] {
		c.Send("You fail the rescue!\r\n")
		return nil
	}

	c.Send("Banzai!  To the rescue...\r\n")
	victim.Tell("You are rescued by %s, you are confused!\r\n", c.Character.Name)
	for _, other := range c.World.Occupants(c.Character.Room) {
		if other != c.Character && other != victim {
			other.Tell("%s heroically rescues %s!\r\n", c.Character.Name, victim.Name)
		}
	}

	if victim.Fighting == attacker {
		c.World.StopFighting(victim)
	}
	c.World.StopFighting(attacker)
	c.World.StopFighting(c.Character)

	c.World.SetFighting(c.Character, attacker)
	c.World.SetFighting(attacker, c.Character)

	// The lag lands on the *rescued* character, not the rescuer — they are
	// "confused", as the message says.
	victim.Wait(2, c.roundLength())
	return nil
}

// roundLength is how long a combat round is, which is what a wait state is
// counted in. PULSE_VIOLENCE is two seconds.
func (c *Context) roundLength() time.Duration { return 2 * time.Second }

// skillTarget finds who a skill is aimed at, falling back to whoever the
// character is already fighting — which is what makes `kick` with no argument
// work mid-fight.
func (c *Context) skillTarget(arg, missing string) *game.Character {
	if name := strings.TrimSpace(arg); name != "" {
		if victim := c.findInRoom(name); victim != nil {
			return victim
		}
	}
	if c.Character.Fighting != nil && c.Character.Fighting.Room == c.Character.Room {
		return c.Character.Fighting
	}
	c.Send("%s", missing)
	return nil
}

// doHide, porting do_hide (act.other.c).
//
// It says you tried whatever happens, and says nothing at all about whether
// it worked — you find out by whether anybody reacts to you. The flag is
// cleared first, so a failed attempt un-hides somebody who was already
// hidden.
func doHide(c *Context) error {
	rec := c.Character.Record
	if c.Character.IsNPC() || rec == nil || rec.Skills[game.SkillHide] == 0 {
		c.Send("You have no idea how to do that.\r\n")
		return nil
	}

	c.Send("You attempt to hide yourself.\r\n")
	c.Character.SetHidden(false)

	// 101 is a complete failure however good you are, and dexterity moves the
	// bar by as much as sixty points either way.
	if c.RNG.Number(1, 101) > rec.Skills[game.SkillHide]+
		game.DexteritySkills(rec.Abilities.Dexterity).Hide {
		return nil
	}
	c.Character.SetHidden(true)
	return nil
}

// doSneak, porting do_sneak.
//
// Sneaking is an affect rather than a bare flag, so it runs out — after as
// many mud hours as the sneaker has levels — where hiding lasts until
// something breaks it.
func doSneak(c *Context) error {
	rec := c.Character.Record
	if c.Character.IsNPC() || rec == nil || rec.Skills[game.SkillSneak] == 0 {
		c.Send("You have no idea how to do that.\r\n")
		return nil
	}

	c.Send("Okay, you'll try to move silently for a while.\r\n")
	game.RemoveAffectsOf(rec, game.SkillSneak)

	if c.RNG.Number(1, 101) > rec.Skills[game.SkillSneak]+
		game.DexteritySkills(rec.Abilities.Dexterity).Sneak {
		return nil
	}
	game.AddAffect(rec, game.Affect{
		Type:     game.SkillSneak,
		Duration: rec.Level,
		Bits:     game.AffectSneak,
	})
	return nil
}
