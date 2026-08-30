// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package session

import (
	"fmt"

	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/obs"
)

// The wizard commands that change things, ported from act.wizard.c.
//
// `load` and `purge` make and unmake, `advance` and `restore` and `zreset`
// reach into what already exists, and `do_wizutil` is seven commands sharing
// one "find a player and check you outrank them" preamble.

// doLoad is do_load (act.wizard.c:1237).
func doLoad(c *Context) error {
	what, number, _ := twoArguments(c.Arg)
	if what == "" || number == "" || !isNumber(number) {
		c.Send("Usage: load { obj | mob } <number>\r\n")
		return nil
	}

	switch {
	case isPrefixOf(what, "mob"):
		mob := c.World.SpawnMobile(game.MobVnum(atoi(number)), c.Character.Room, c.RNG)
		if mob == nil {
			c.Send("There is no monster with that number.\r\n")
			return nil
		}
		c.announce("%s makes a quaint, magical gesture with one hand.\r\n", c.Character.Name)
		c.announceExcept(mob, "%s has created %s!\r\n", c.Character.Name, mob.Name)
		c.Send("You create %s.\r\n", mob.Name)

	case isPrefixOf(what, "obj"):
		obj := c.World.NewObject(game.ObjVnum(atoi(number)))
		if obj == nil {
			c.Send("There is no object with that number.\r\n")
			return nil
		}
		// `load_into_inventory` is YES in config.c, so a loaded object goes
		// into the loader's hands rather than onto the floor. Handy, and the
		// reason nobody ever had to `get` what they had just made.
		c.World.ObjectToChar(obj, c.Character)
		c.announce("%s makes a strange magical gesture.\r\n", c.Character.Name)
		c.announce("%s has created %s!\r\n", c.Character.Name, obj.Name())
		c.Send("You create %s.\r\n", obj.Name())

	default:
		c.Send("That'll have to be either 'obj' or 'mob'.\r\n")
	}
	return nil
}

// doPurge is do_purge (act.wizard.c:1331): destroy one thing, or everything
// in the room.
func doPurge(c *Context) error {
	name, _ := oneArgument(c.Arg)

	if name != "" {
		if victim := c.findInRoom(name); victim != nil {
			// You cannot purge somebody your own level or above — and note
			// the test is `<=`, so two implementors cannot purge each other.
			if !victim.IsNPC() && levelOf(c.Character) <= levelOf(victim) {
				c.Send("Fuuuuuuuuu!\r\n")
				return nil
			}
			c.announceExcept(victim, "%s disintegrates %s.\r\n", c.Character.Name, victim.Name)
			// mudlog(buf, BRF, MAX(LVL_GOD, GET_INVIS_LEV(ch)), TRUE)
			// (act.wizard.c:1348-1349), and only for a player — the C
			// guards it with `if (!IS_NPC(vict))`, so purging a mobile is
			// unremarkable and purging a person is not.
			if !victim.IsNPC() {
				c.wizlogInvis(obs.LogBrief, game.LevelGod, c.Character,
					"(GC) %s has purged %s.", c.Character.Name, victim.Name)
			}
			c.purge(victim)
			c.Send("Okay.\r\n")
			return nil
		}
		obj := c.findObject(c.World.RoomObjects(c.Character.Room), name)
		if obj == nil {
			c.Send("Nothing here by that name.\r\n")
			return nil
		}
		c.announce("%s destroys %s.\r\n", c.Character.Name, obj.Name())
		c.World.ExtractObject(obj)
		c.Send("Okay.\r\n")
		return nil
	}

	// No argument: everything in the room that is not a player.
	c.announce("%s gestures... You are surrounded by scorching flames!\r\n", c.Character.Name)
	for _, who := range c.World.Occupants(c.Character.Room) {
		who.Tell("The world seems a little cleaner.\r\n")
	}

	for _, victim := range append([]*game.Character(nil), c.World.Occupants(c.Character.Room)...) {
		if !victim.IsNPC() {
			continue
		}
		c.purge(victim)
	}
	for _, obj := range append([]*game.Object(nil), c.World.RoomObjects(c.Character.Room)...) {
		c.World.ExtractObject(obj)
	}
	return nil
}

// purge removes a character and everything they were carrying.
//
// A purged *player* is disconnected without their record being written — the
// C sets CON_CLOSE and detaches the descriptor before extract_char, so
// nothing saves them. That is the point: purging a player is how a god
// unmakes one.
func (c *Context) purge(victim *game.Character) {
	for _, obj := range append([]*game.Object(nil), victim.Carrying...) {
		c.World.ExtractObject(obj)
	}
	for _, obj := range victim.Equipment {
		c.World.ExtractObject(obj)
	}
	if closer, ok := victim.Client.(interface{ Close() }); ok && closer != nil {
		closer.Close()
	}
	c.World.Remove(victim)
}

// doRestore is do_restore (act.wizard.c:1514).
func doRestore(c *Context) error {
	name, _ := oneArgument(c.Arg)
	if name == "" {
		c.Send("Whom do you wish to restore?\r\n")
		return nil
	}
	victim := c.findAnywhere(name)
	if victim == nil {
		c.Send("%s", noPerson)
		return nil
	}
	if !victim.IsNPC() && levelOf(victim) >= levelOf(c.Character) {
		c.Send("They don't need your help.\r\n")
		return nil
	}
	rec := victim.Record
	if rec == nil {
		c.Send("They don't need your help.\r\n")
		return nil
	}

	rec.Points.Hit, rec.Points.Mana, rec.Points.Move =
		rec.Points.MaxHit, rec.Points.MaxMana, rec.Points.MaxMove
	victim.Position = game.UpdatePosition(rec, victim.Position)

	// A greater god restoring an immortal does rather more than heal them:
	// every skill to 100, and for another greater god every ability to 25
	// with a strength addendum of 100. This is how a new immortal is fitted
	// out, and it is the only place in the game those numbers are set.
	if !victim.IsNPC() && levelOf(c.Character) >= game.LevelGreaterGod {
		if rec.Level >= game.LevelImmortal {
			for skill := int32(1); skill <= game.MaxSkills; skill++ {
				rec.Skills[skill] = 100
			}
		}
		if rec.Level >= game.LevelGreaterGod {
			rec.RealAbilities = game.Abilities{
				Strength: 25, StrengthPercentile: 100, Intelligence: 25,
				Wisdom: 25, Dexterity: 25, Constitution: 25, Charisma: 25,
			}
		}
	}
	game.RecomputeAffects(rec)

	c.Send("Okay.\r\n")
	victim.Tell("You have been fully healed by %s!\r\n", c.Character.Name)
	return nil
}

// doZreset is do_zreset (act.wizard.c:1977).
func doZreset(c *Context) error {
	arg, _ := oneArgument(c.Arg)
	if arg == "" {
		c.Send("You must specify a zone.\r\n")
		return nil
	}

	if arg[0] == '*' {
		for _, zone := range c.World.Zones() {
			c.World.ResetZone(zone, c.RNG)
		}
		c.Send("Reset world.\r\n")
		// mudlog(buf, NRM, MAX(LVL_GRGOD, GET_INVIS_LEV(ch)), TRUE)
		// (act.wizard.c:1991-1992). Greater god, not god: resetting every
		// zone at once is loud enough that the C raises the bar for who
		// hears about it.
		c.wizlogInvis(obs.LogNormal, game.LevelGreaterGod, c.Character,
			"(GC) %s reset entire world.", c.Character.Name)
		return nil
	}

	var target *game.ZoneDef
	if arg[0] == '.' {
		target = c.World.ZoneOf(c.Character.Room)
	} else {
		want := game.ZoneVnum(atoi(arg))
		for _, zone := range c.World.Zones() {
			if zone.Vnum == want {
				target = zone
				break
			}
		}
	}
	if target == nil {
		c.Send("Invalid zone number.\r\n")
		return nil
	}

	c.World.ResetZone(target, c.RNG)
	c.Send("Reset zone %d (#%d): %s.\r\n", target.Vnum, target.Vnum, target.Name)
	// mudlog(buf, NRM, MAX(LVL_GRGOD, GET_INVIS_LEV(ch)), TRUE)
	// (act.wizard.c:2007-2008). The C's own text prints the zone *rnum*
	// here, the table index; this port has no rnums, so it prints the vnum,
	// the same substitution the reply above already makes.
	c.wizlogInvis(obs.LogNormal, game.LevelGreaterGod, c.Character,
		"(GC) %s reset zone %d (%s)", c.Character.Name, target.Vnum, target.Name)
	return nil
}

// doAdvance is do_advance (act.wizard.c:1428).
func doAdvance(c *Context) error {
	name, level, _ := twoArguments(c.Arg)
	if name == "" {
		c.Send("Advance who?\r\n")
		return nil
	}
	victim := c.findAnywhere(name)
	if victim == nil {
		c.Send("That player is not here.\r\n")
		return nil
	}
	if levelOf(c.Character) <= levelOf(victim) {
		c.Send("Maybe that's not such a great idea.\r\n")
		return nil
	}
	if victim.IsNPC() {
		c.Send("NO!  Not on NPC's.\r\n")
		return nil
	}

	newLevel := atoi(level)
	switch {
	case level == "" || newLevel <= 0:
		c.Send("That's not a level!\r\n")
		return nil
	case newLevel > game.LevelImplementor:
		c.Send("%d is the highest possible level.\r\n", game.LevelImplementor)
		return nil
	case newLevel > levelOf(c.Character):
		c.Send("Yeah, right.\r\n")
		return nil
	case newLevel == levelOf(victim):
		c.Send("They are already at that level.\r\n")
		return nil
	}

	rec := victim.Record
	oldLevel := rec.Level

	if newLevel < oldLevel {
		// Demotion runs do_start *first* and then sets the level, so the
		// character is rebuilt as a level-one and then stamped with the new
		// number — which is why being demoted costs you your hit points as
		// well as your level.
		game.Start(rec, c.RNG)
		rec.Level = newLevel
		victim.Tell("You are momentarily enveloped by darkness!\r\n" +
			"You feel somewhat diminished.\r\n")
	} else {
		victim.Tell("%s makes some strange gestures.\r\n"+
			"A strange feeling comes upon you,\r\n"+
			"Like a giant hand, light comes down\r\n"+
			"from above, grabbing your body, that\r\n"+
			"begins to pulse with colored lights\r\n"+
			"from inside.\r\n\r\n"+
			"Your head seems to be filled with demons\r\n"+
			"from another plane as your body dissolves\r\n"+
			"to the elements of time and space itself.\r\n"+
			"Suddenly a silent explosion of light\r\n"+
			"snaps you back to reality.\r\n\r\n"+
			"You feel slightly different.\r\n", c.Character.Name)
	}

	c.Send("Okay.\r\n")

	// Somebody dropped out of immortality loses the flags that only make
	// sense up there.
	if newLevel < game.LevelImmortal {
		rec.Preferences = rec.Preferences.Without(
			game.PrefLog1, game.PrefLog2, game.PrefNoHassle, game.PrefHolylight)
	}

	// The level is set by the experience, not the other way round: the C
	// hands gain_exp_regardless the *difference* between this level's
	// threshold and what they have, and lets the levelling code do the rest.
	levels := game.GainExperienceRegardless(rec, game.LevelExperience(rec.Class, newLevel)-rec.Points.Exp, c.RNG)
	if levels > 0 {
		// gain_exp_regardless' copy of the same line (limits.c:351-357).
		// `advance` demoting somebody sets the level itself and gains no
		// levels here, so this fires on the way up only — which is the C's
		// behaviour too, `is_altered` being set by the loop alone.
		c.wizlogInvis(obs.LogBrief, game.LevelImmortal, victim,
			"%s advanced %d level%s to level %d.", victim.Name,
			levels, plural(int(levels)), rec.Level)
	}
	// The rest of that block: "You rise N levels!" to the victim and the
	// `<DoC>` whisper to everybody. The *Regardless* form, because that is
	// the copy `advance` stands in — and its whisper has no trailing newline,
	// which is the C's own difference between the two and not a slip here.
	c.World.AnnounceLevelGainRegardless(victim, levels)
	if c.Save != nil {
		c.Save(victim)
	}
	return nil
}

// --- do_wizutil: seven commands with one preamble ---------------------

// wizutilTarget is the shared front of do_wizutil (act.wizard.c:2017): find
// the player, and refuse if they outrank you.
func (c *Context) wizutilTarget() *game.Character {
	name, _ := oneArgument(c.Arg)
	switch {
	case name == "":
		c.Send("Yes, but for whom?!?\r\n")
	case c.findAnywhere(name) == nil:
		c.Send("There is no such player.\r\n")
	default:
		victim := c.findAnywhere(name)
		switch {
		case victim.IsNPC():
			c.Send("You can't do that to a mob!\r\n")
		case levelOf(victim) > levelOf(c.Character):
			// Note `>` and not `>=`: two gods of the same level may freeze
			// and reroll each other.
			c.Send("Hmmm...you'd better not.\r\n")
		default:
			return victim
		}
	}
	return nil
}

// doReroll is SCMD_REROLL.
func doReroll(c *Context) error {
	victim := c.wizutilTarget()
	if victim == nil {
		return nil
	}
	rec := victim.Record

	c.Send("Rerolled...\r\n")
	rec.RealAbilities = game.RollAbilities(rec.Class, c.RNG)
	game.RecomputeAffects(rec)

	a := rec.Abilities
	c.Send("New stats: Str %d/%d, Int %d, Wis %d, Dex %d, Con %d, Cha %d\r\n",
		a.Strength, a.StrengthPercentile, a.Intelligence, a.Wisdom,
		a.Dexterity, a.Constitution, a.Charisma)
	c.saveVictim(victim)
	return nil
}

// doPardon is SCMD_PARDON: clear the killer and thief flags.
func doPardon(c *Context) error {
	victim := c.wizutilTarget()
	if victim == nil {
		return nil
	}
	rec := victim.Record
	if !rec.PlayerFlags.HasAny(game.PlayerThief, game.PlayerKiller) {
		c.Send("Your victim is not flagged.\r\n")
		return nil
	}
	rec.PlayerFlags = rec.PlayerFlags.Without(game.PlayerThief, game.PlayerKiller)
	c.Send("Pardoned.\r\n")
	victim.Tell("You have been pardoned by the Gods!\r\n")
	// mudlog(buf, BRF, MAX(LVL_GOD, GET_INVIS_LEV(ch)), TRUE)
	// (act.wizard.c:2051-2052). Note the order of the two names: the C
	// puts the pardoned player first and the god second, the opposite way
	// round from most of do_wizutil's other lines.
	c.wizlogInvis(obs.LogBrief, game.LevelGod, c.Character,
		"(GC) %s pardoned by %s", victim.Name, c.Character.Name)
	c.saveVictim(victim)
	return nil
}

// doNoTitle and doSquelch are SCMD_NOTITLE and SCMD_SQUELCH, which are the
// same command over different bits.
func doNoTitle(c *Context) error {
	return c.togglePlayerFlag(game.PlayerNoTitle, "Notitle", obs.LogNormal)
}

func doSquelch(c *Context) error {
	return c.togglePlayerFlag(game.PlayerNoShout, "Squelch", obs.LogBrief)
}

// togglePlayerFlag carries the syslog verbosity as a parameter because the
// two commands do not share one: notitle is NRM and squelch is BRF
// (act.wizard.c:2074, 2082). Everything else about them is identical, down
// to the C building one string and using it for both the log line and the
// reply — which is why the reply here starts with "(GC) " too.
func (c *Context) togglePlayerFlag(flag game.PlayerFlag, label string, typ int) error {
	victim := c.wizutilTarget()
	if victim == nil {
		return nil
	}
	rec := victim.Record
	rec.PlayerFlags = rec.PlayerFlags.Toggle(flag)
	line := fmt.Sprintf("(GC) %s %s for %s by %s.",
		label, onOff(rec.PlayerFlags.Has(flag)), victim.Name, c.Character.Name)
	// mudlog(buf, ..., MAX(LVL_GOD, GET_INVIS_LEV(ch)), TRUE) and then
	// `strcat(buf, "\r\n"); send_to_char(buf, ch)` — the log line first,
	// the same text to the god second.
	c.wizlogInvis(typ, game.LevelGod, c.Character, "%s", line)
	c.Send("%s\r\n", line)
	c.saveVictim(victim)
	return nil
}

// doFreeze is SCMD_FREEZE: a frozen character cannot do anything at all.
func doFreeze(c *Context) error {
	victim := c.wizutilTarget()
	if victim == nil {
		return nil
	}
	if victim == c.Character {
		c.Send("Oh, yeah, THAT'S real smart...\r\n")
		return nil
	}
	rec := victim.Record
	if rec.PlayerFlags.Has(game.PlayerFrozen) {
		c.Send("Your victim is already pretty cold.\r\n")
		return nil
	}

	rec.PlayerFlags = rec.PlayerFlags.With(game.PlayerFrozen)
	// The level of whoever did it, so a lesser god cannot undo it.
	rec.FreezeLevel = levelOf(c.Character)

	victim.Tell("A bitter wind suddenly rises and drains every erg of heat " +
		"from your body!\r\nYou feel frozen!\r\n")
	c.Send("Frozen.\r\n")
	announce(c.World, victim.Room, victim,
		"A sudden cold wind conjured from nowhere freezes %s!\r\n", victim.Name)
	// mudlog(buf, BRF, MAX(LVL_GOD, GET_INVIS_LEV(ch)), TRUE)
	// (act.wizard.c:2100-2101).
	c.wizlogInvis(obs.LogBrief, game.LevelGod, c.Character,
		"(GC) %s frozen by %s.", victim.Name, c.Character.Name)
	c.saveVictim(victim)
	return nil
}

// doThaw is SCMD_THAW.
func doThaw(c *Context) error {
	victim := c.wizutilTarget()
	if victim == nil {
		return nil
	}
	rec := victim.Record
	if !rec.PlayerFlags.Has(game.PlayerFrozen) {
		c.Send("Sorry, your victim is not morbidly encased in ice at the moment.\r\n")
		return nil
	}
	if rec.FreezeLevel > levelOf(c.Character) {
		c.Send("Sorry, a level %d God froze %s... you can't unfreeze %s.\r\n",
			rec.FreezeLevel, victim.Name, victim.Objective())
		return nil
	}

	// mudlog(buf, BRF, MAX(LVL_GOD, GET_INVIS_LEV(ch)), TRUE)
	// (act.wizard.c:2114-2115), which the C does *before* clearing the bit
	// — the one place in do_wizutil it logs ahead of acting.
	c.wizlogInvis(obs.LogBrief, game.LevelGod, c.Character,
		"(GC) %s un-frozen by %s.", victim.Name, c.Character.Name)
	rec.PlayerFlags = rec.PlayerFlags.Without(game.PlayerFrozen)
	victim.Tell("A fireball suddenly explodes in front of you, melting the ice!\r\n" +
		"You feel thawed.\r\n")
	c.Send("Thawed.\r\n")
	announce(c.World, victim.Room, victim,
		"A sudden fireball conjured from nowhere thaws %s!\r\n", victim.Name)
	c.saveVictim(victim)
	return nil
}

// doUnaffect is SCMD_UNAFFECT: strip every spell.
func doUnaffect(c *Context) error {
	victim := c.wizutilTarget()
	if victim == nil {
		return nil
	}
	rec := victim.Record
	if len(rec.Affects) == 0 {
		c.Send("Your victim does not have any affections!\r\n")
		return nil
	}

	rec.Affects = nil
	game.RecomputeAffects(rec)
	victim.Tell("There is a brief flash of light!\r\nYou feel slightly different.\r\n")
	c.Send("All spells removed.\r\n")
	c.saveVictim(victim)
	return nil
}

// saveVictim is the `save_char(vict)` every branch of do_wizutil ends with.
func (c *Context) saveVictim(victim *game.Character) {
	if c.Save != nil {
		c.Save(victim)
	}
}

func onOff(on bool) string {
	if on {
		return "ON"
	}
	return "OFF"
}
