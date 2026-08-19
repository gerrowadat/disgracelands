// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package session

import (
	"fmt"
	"math"
	"time"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// The spell routines that are not a table lookup: the ones that happen to
// objects, the one that hits a whole room, and the manual spells, which are
// the ones with a C function apiece because they do not fit any of the
// others.

// medusaVnum is the mobile in mag_areas that an area spell cannot be cast
// near without consequences. Local: the C carries the number inline with a
// `/*Yoss - medusa*/` comment and no constant.
const medusaVnum game.MobVnum = 14205

// medusaRefuge is where she sends you.
const medusaRefuge game.RoomVnum = 14284

// pkAllowed is config.c's pk_allowed. Off, as it is there — which is what
// makes an area spell skip other players entirely.
const pkAllowed = false

// spellAlterObject applies an object-altering spell, porting mag_alter_objs.
//
// The C sends the same line to the caster and to the room, and prints
// NOEFFECT to the caster when nothing changed — so a curse that fails because
// the object is already cursed is indistinguishable from a curse that failed.
func (c *Context) spellAlterObject(number int32, obj *game.Object) {
	message := game.AlterObject(number, obj, c.Character.Level())
	if message == "" {
		c.Send("%s", game.NoEffect)
		return
	}
	c.Send("%s\r\n", message)
	c.announce("%s\r\n", message)
}

// spellCreation makes an object out of nothing, porting mag_creations.
func (c *Context) spellCreation(number int32) {
	if number != game.SpellCreateFood {
		c.Send("Spell unimplemented, it would seem.\r\n")
		return
	}

	obj := c.World.NewObject(game.CreateFoodVnum)
	if obj == nil {
		// The C logs a SYSERR here and tells the player the same thing.
		c.Send("I seem to have goofed.\r\n")
		return
	}

	c.World.ObjectToChar(obj, c.Character)
	c.Send("You create %s.\r\n", obj.Name())
	c.announce("%s creates %s.\r\n", c.Character.Name, obj.Name())
}

// spellArea hits everyone in the room, porting mag_areas.
//
// The four skips are the C's, in its order: the caster, immortals, other
// players when player-killing is off, and charmed mobiles — somebody's pet.
// The fifth is local, and it is the medusa: cast an area spell where she can
// see you and you are the one who leaves.
func (c *Context) spellArea(info game.SpellInfo, number int32, level int32) {
	if number == game.SpellEarthquake {
		c.Send("You gesture and the earth begins to shake all around you!\r\n")
		c.announce("%s gracefully gestures and the earth begins to shake violently!\r\n",
			c.Character.Name)
	}

	// A snapshot: the loop moves people and kills them.
	for _, victim := range append([]*game.Character(nil), c.World.Occupants(c.Character.Room)...) {
		switch {
		case victim == c.Character:
			continue
		case !victim.IsNPC() && victim.Level() >= game.LevelImmortal:
			continue
		case !pkAllowed && !c.Character.IsNPC() && !victim.IsNPC():
			continue
		case !c.Character.IsNPC() && victim.IsNPC() &&
			victim.Record != nil && victim.Record.AffectFlags.Has(game.AffectCharm):
			continue
		}

		if c.medusaLooksAt(victim) {
			continue
		}

		c.spellDamage(info, number, victim, level)
	}
}

// medusaLooksAt is the local rule in mag_areas: a mortal who is not blind and
// casts an area spell in front of the medusa is thrown out of the room
// instead. The mobile itself takes no damage, because the caster is gone
// before the loop reaches it.
func (c *Context) medusaLooksAt(victim *game.Character) bool {
	if victim.MobDef == nil || victim.MobDef.Vnum != medusaVnum {
		return false
	}
	if c.Character.Level() >= game.LevelImmortal {
		return false
	}
	if c.Character.Record != nil && c.Character.Record.AffectFlags.Has(game.AffectBlind) {
		return false
	}

	c.Send("Medusa turns her eyes to you!\r\nYou flee head over heals.\r\n")
	c.moveTo(c.Character, medusaRefuge, "", "%s has arrived.\r\n")
	return true
}

// castManual runs the spells that have a function of their own, porting the
// MANUAL_SPELL switch at the end of call_magic.
//
// It returns false for a spell that is not written yet, so the caller can say
// so rather than charging for nothing.
func (c *Context) castManual(number int32, victim *game.Character, obj *game.Object, level int32) bool {
	switch number {
	case game.SpellCreateWater:
		if obj == nil {
			return true
		}
		if message := game.FillWithWater(obj); message != "" {
			c.Send("%s\r\n", message)
		}
		return true

	case game.SpellDetectPoison:
		c.detectPoison(victim, obj)
		return true

	case game.SpellEnchantWeapon:
		if obj == nil {
			return true
		}
		if message := game.EnchantWeapon(obj, c.Character.Record, level); message != "" {
			c.Send("%s\r\n", message)
		}
		return true

	case game.SpellIdentify:
		switch {
		case obj != nil:
			c.Send("%s", game.IdentifyObject(obj))
		case victim != nil:
			c.Send("%s", game.IdentifyCharacter(victim, time.Now()))
		}
		return true

	case game.SpellLocateObject:
		c.locateObject(obj, level)
		return true

	case game.SpellDispelMagic:
		// Local, and marked `<DoC>` in the C: it strips every affect off the
		// victim, with no saving throw and no exceptions. Cast on yourself it
		// removes your own blessings too.
		if victim != nil && victim.Record != nil {
			game.RemoveAllAffects(victim.Record)
		}
		return true

	case game.SpellWordOfRecall:
		// Mobiles are not recalled: the C returns early on IS_NPC, so a
		// charmed pet cast on stays where it is.
		if victim == nil || victim.IsNPC() {
			return true
		}
		c.moveTo(victim, game.MortalStartRoom,
			"%s disappears.\r\n", "%s appears in the middle of the room.\r\n")
		return true

	case game.SpellTeleport:
		if victim == nil || victim.IsNPC() {
			return true
		}
		c.moveTo(victim, c.randomRoom(),
			"%s slowly fades out of existence and is gone.\r\n",
			"%s slowly fades into existence.\r\n")
		return true
	}
	return false
}

// detectPoison, porting spell_detect_poison. It reads a person or a thing,
// and says something different about yourself than about somebody else.
func (c *Context) detectPoison(victim *game.Character, obj *game.Object) {
	if victim != nil && victim.Record != nil {
		poisoned := victim.Record.AffectFlags.Has(game.AffectPoison)
		switch {
		case victim == c.Character && poisoned:
			c.Send("You can sense poison in your blood.\r\n")
		case victim == c.Character:
			c.Send("You feel healthy.\r\n")
		case poisoned:
			c.Send("You sense that %s is poisoned.\r\n", victim.Subject())
		default:
			c.Send("You sense that %s is healthy.\r\n", victim.Subject())
		}
	}
	if obj != nil {
		c.Send("%s", game.PoisonReport(obj))
	}
}

// locateObject finds copies of a thing anywhere in the world, porting
// spell_locate_object.
//
// The C's own comment calls this broken, and it is: the spell parser resolved
// the argument to *an object* before the spell ran, so all the spell has left
// to search by is that object's first keyword. Cast it on your own sword and
// it finds every sword in the game. Level/2 results, and no more.
func (c *Context) locateObject(target *game.Object, level int32) {
	if target == nil {
		return
	}

	name := firstWord(target.Keywords)
	remaining := level / 2
	found := remaining

	for _, obj := range c.World.Objects() {
		if remaining <= 0 {
			break
		}
		if !obj.Matches(name) {
			continue
		}
		// Local: an object flagged NO_LOCATE is skipped. The C tests the
		// flag on the *target* rather than on the object it is looking at
		// (spells.c:198), so one no-locate object hides every object that
		// shares its name. Reproduced.
		if target.ExtraFlags.Has(game.ItemNoLocate) {
			continue
		}

		c.Send("%s", capitaliseFirst(c.whereIs(obj)))
		remaining--
	}

	if remaining == found {
		c.Send("You sense nothing.\r\n")
	}
}

// whereIs describes where an object is, in the order spell_locate_object
// tests: carried, on the floor, inside something, worn, and finally the
// shrug for an object that is nowhere at all.
func (c *Context) whereIs(obj *game.Object) string {
	switch obj.Location {
	case game.CarriedBy:
		return fmt.Sprintf("%s is being carried by %s.\r\n", obj.Name(), obj.Holder.Name)
	case game.InRoom:
		room := c.World.Room(obj.Room)
		name := "somewhere"
		if room != nil {
			name = room.Name
		}
		return fmt.Sprintf("%s is in %s.\r\n", obj.Name(), name)
	case game.InObject:
		return fmt.Sprintf("%s is in %s.\r\n", obj.Name(), obj.Container.Name())
	case game.WornBy:
		return fmt.Sprintf("%s is being worn by %s.\r\n", obj.Name(), obj.Holder.Name)
	}
	return fmt.Sprintf("%s's location is uncertain.\r\n", obj.Name())
}

// randomRoom picks somewhere to be teleported to, porting the do/while in
// spell_teleport.
//
// The C rolls `number(0, top_of_world)` and rerolls until it finds a room
// that is not private, a death trap or a god room — with no bound on the
// number of tries, so a world made entirely of those rooms hangs the server.
// The bound here is the only difference.
func (c *Context) randomRoom() game.RoomVnum {
	const tries = 500

	top := c.World.RoomCount() - 1
	if top < 0 {
		return game.MortalStartRoom
	}
	if top > math.MaxInt32 {
		top = math.MaxInt32
	}

	for i := 0; i < tries; i++ {
		room := c.World.RoomAt(int(c.RNG.Number(0, int32(top)))) //nolint:gosec // clamped above
		if room == nil {
			continue
		}
		if room.Flags.HasAny(game.RoomPrivate | game.RoomDeathTrap | game.RoomGodRoom) {
			continue
		}
		return room.Vnum
	}
	return game.MortalStartRoom
}

// moveTo puts somebody in a room, tells both rooms, and shows them where they
// have arrived. The two formats take the traveller's name; either may be
// empty for a move nobody sees.
func (c *Context) moveTo(who *game.Character, to game.RoomVnum, leaving, arriving string) {
	from := who.Room
	if leaving != "" {
		announce(c.World, from, who, leaving, who.Name)
	}
	if err := c.World.Enter(who, to); err != nil {
		return
	}
	if arriving != "" {
		announce(c.World, to, who, arriving, who.Name)
	}

	if room := c.World.Room(to); room != nil {
		who.Tell("%s", roomDescription(c.World, room, who))
	}
}
