// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package session

import (
	"strings"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// do_steal and do_track, ported from act.other.c:1013 and graph.c:158.
//
// The last two thief skills, and the two that needed machinery of their own:
// stealing needs the shopkeeper check and the player-thieving setting, and
// tracking needs the only graph algorithm in the server.

// PlayerThievingAllowed is `pt_allowed = NO` (config.c:58).
//
// It is worth stating what this does, because it is not a discouragement: a
// steal from another *player* is set to a flat 101% — a complete failure —
// before anything else is considered. Nobody on this server ever picked
// another player's pocket, however good a thief they were.
var PlayerThievingAllowed = false

// maxStolenGold is the cap in do_steal, and it is 1782. Not 1000, not 2000.
// Nobody has ever explained it.
const maxStolenGold int32 = 1782

// doSteal is do_steal (act.other.c:1013).
func doSteal(c *Context) error {
	who := c.Character
	if who.IsNPC() || skillOf(who, game.SkillSteal) == 0 {
		c.Send("You have no idea how to do that.\r\n")
		return nil
	}
	if room := c.World.Room(who.Room); room != nil && room.Flags.Has(game.RoomPeaceful) {
		c.Send("This room just has such a peaceful, easy feeling...\r\n")
		return nil
	}

	objName, victName, _ := twoArguments(c.Arg)
	victim := c.World.FindInRoom(who, who.Room, victName)
	switch victim {
	case nil:
		c.Send("Steal what from who?\r\n")
		return nil
	case who:
		c.Send("Come on now, that's rather stupid!\r\n")
		return nil
	}

	// 101% is a complete failure whatever the skill.
	percent := c.RNG.Number(1, 101) -
		game.DexteritySkills(dexterityOf(who)).PickPockets

	if victim.Position < game.PosSleeping {
		// "ALWAYS SUCCESS, unless heavy object." A negative roll can still be
		// pushed back over the skill by a heavy enough item, which is what
		// the comment means.
		percent = -1
	}
	if !victim.Position.Awake() {
		percent -= 50
	}
	// Immortals, shopkeepers, and — because pt_allowed is NO — every other
	// player. All three collapse into the same flat failure.
	if levelOf(victim) >= game.LevelImmortal ||
		(!PlayerThievingAllowed && !victim.IsNPC()) ||
		c.World.ShopFor(victim) != nil {
		percent = 101
	}

	caught := false
	if !strings.EqualFold(objName, "coins") && !strings.EqualFold(objName, "gold") {
		caught = stealObject(c, victim, objName, percent)
	} else {
		caught = stealGold(c, victim, percent)
	}

	// Only a mobile fights back, and only if it is awake to notice.
	if caught && victim.IsNPC() && victim.Position.Awake() {
		c.Violence.Swing(c.World, victim, who)
	}
	return nil
}

// stealObject is the object half of do_steal, returning whether they were
// caught.
func stealObject(c *Context, victim *game.Character, name string, percent int32) bool {
	who := c.Character

	obj := c.findObject(victim.Carrying, name)
	if obj == nil {
		// Not carried — try what they are wearing.
		worn, slot := findWorn(victim, name)
		if worn == nil {
			c.Send("%s hasn't got that item.\r\n", capitaliseFirst(victim.Subject()))
			return false
		}
		// Equipment can only be taken off somebody who is out cold, and then
		// it is taken with no roll at all: a stunned victim is robbed blind
		// however bad a thief you are.
		if victim.Position > game.PosStunned {
			c.Send("Steal the equipment now?  Impossible!\r\n")
			return false
		}
		c.Send("You unequip %s and steal it.\r\n", worn.Name())
		c.announceExcept(victim, "%s steals %s from %s.\r\n", who.Name, worn.Name(), victim.Name)
		c.World.Unequip(victim, slot)
		c.World.ObjectToChar(worn, who)
		return false
	}

	// A heavy thing is harder, and this is the addition that can pull even a
	// sleeping victim's percent back above the skill.
	percent += obj.Weight

	if percent > skillOf(who, game.SkillSteal) {
		c.Send("Oops..\r\n")
		victim.Tell("%s tried to steal something from you!\r\n", who.Name)
		c.announceExcept(victim, "%s tries to steal something from %s.\r\n", who.Name, victim.Name)
		return true
	}

	// The two capacity checks are the C's, and note what they do *not* do:
	// failing either says nothing at all in the weight case, so a thief who
	// is too laden gets silence and keeps their skill roll. Reproduced.
	if len(who.Carrying)+1 < carryCount(who) {
		if who.CarriedWeight()+obj.Weight < carryWeight(who) {
			c.World.ExtractFromChar(obj)
			c.World.ObjectToChar(obj, who)
			c.Send("Got it!\r\n")
		}
	} else {
		c.Send("You cannot carry that much.\r\n")
	}
	return false
}

// stealGold is the coin half of do_steal.
func stealGold(c *Context, victim *game.Character, percent int32) bool {
	who := c.Character

	// A sleeping victim's purse is taken with no roll: the skill check is
	// only made if they are awake.
	if victim.Position.Awake() && percent > skillOf(who, game.SkillSteal) {
		c.Send("Oops..\r\n")
		victim.Tell("You discover that %s has %s hands in your wallet.\r\n",
			who.Name, who.Possessive())
		c.announceExcept(victim, "%s tries to steal gold from %s.\r\n", who.Name, victim.Name)
		return true
	}

	// A tenth of what they have, at most — and never more than 1782.
	// Named amount rather than gold: gold() is the accessor, and shadowing it
	// here would work but read badly.
	amount := (gold(victim) * c.RNG.Number(1, 10)) / 100
	amount = min(maxStolenGold, amount)
	if amount <= 0 {
		c.Send("You couldn't get any gold...\r\n")
		return false
	}

	addGold(who, amount)
	addGold(victim, -amount)
	if amount > 1 {
		c.Send("Bingo!  You got %d gold coins.\r\n", amount)
	} else {
		c.Send("You manage to swipe a solitary gold coin.\r\n")
	}
	return false
}

// doTrack is do_track (graph.c:158).
func doTrack(c *Context) error {
	who := c.Character
	if who.IsNPC() || skillOf(who, game.SkillTrack) == 0 {
		c.Send("You have no idea how.\r\n")
		return nil
	}
	name, _ := oneArgument(c.Arg)
	if name == "" {
		c.Send("Whom are you trying to track?\r\n")
		return nil
	}
	victim := c.findAnywhere(name)
	if victim == nil {
		c.Send("No one is around by that name.\r\n")
		return nil
	}
	if victim.Record != nil && victim.Record.AffectFlags.Has(game.AffectNoTrack) {
		c.Send("You sense no trail.\r\n")
		return nil
	}

	// 101 is a complete failure whatever the proficiency — and note the roll
	// is number(0, 101) against the skill, so a skill of 0 fails always and a
	// skill of 100 still fails one time in 102.
	if c.RNG.Number(0, 101) >= skillOf(who, game.SkillTrack) {
		// A wrong answer rather than no answer: ten tries at a random
		// direction you can actually walk, and if all ten fail you are sent
		// whichever way the last roll picked — into a wall.
		dir := game.Direction(0)
		for tries := 10; tries > 0; tries-- {
			dir = game.Direction(c.RNG.Number(0, game.NumDirections-1)) //nolint:gosec // a direction index, bounded by the roll
			if exit := c.World.Exit(who.Room, dir); exit != nil && exit.ToRoom != game.NoRoom {
				break
			}
		}
		c.Send("You sense a trail %s from here!\r\n", dir)
		return nil
	}

	switch step := c.World.FindFirstStep(who.Room, victim.Room); step {
	case game.BFSError:
		c.Send("Hmm.. something seems to be wrong.\r\n")
	case game.BFSAlreadyThere:
		c.Send("You're already in the same room!!\r\n")
	case game.BFSNoPath:
		c.Send("You can't sense a trail to %s from here.\r\n", victim.Objective())
	default:
		// step is one of the six directions here: every other value of
		// find_first_step is a named constant handled above.
		c.Send("You sense a trail %s from here!\r\n", game.Direction(step)) //nolint:gosec // a direction, checked by the cases above
	}
	return nil
}

// findWorn returns the first piece of equipment answering to a name, and
// which slot it is in.
func findWorn(who *game.Character, name string) (*game.Object, game.WearPosition) {
	for slot, obj := range who.Equipment {
		if obj != nil && obj.Matches(name) {
			return obj, game.WearPosition(slot) //nolint:gosec // a slot index, bounded by the array
		}
	}
	return nil, -1
}

// announceExcept tells the room, leaving out the actor and one other person —
// act()'s TO_NOTVICT.
func (c *Context) announceExcept(victim *game.Character, format string, args ...any) {
	for _, other := range c.World.Occupants(c.Character.Room) {
		if other != c.Character && other != victim {
			other.Tell(format, args...)
		}
	}
}

// skillOf is GET_SKILL.
//
// Skills is a *map*, so there is no bound to check: a missing entry is zero,
// which is exactly "not learned". An earlier version of this guarded with
// `int(skill) >= len(...)`, which on a map is the number of entries rather
// than a capacity — so a character who knew six skills was reported as
// knowing nothing about skill 139. The tests missed it because they set every
// skill to zero rather than deleting them, which left the map large.
func skillOf(who *game.Character, skill int32) int32 {
	if who == nil || who.Record == nil {
		return 0
	}
	return who.Record.Skills[skill]
}

func dexterityOf(who *game.Character) int32 {
	if who == nil || who.Record == nil {
		return 0
	}
	return who.Record.Abilities.Dexterity
}
