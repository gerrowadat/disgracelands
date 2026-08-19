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

// spellGroup casts on everybody grouped with the caster who is in the room,
// porting mag_groups and perform_mag_groups.
//
// Only three spells are group spells, and each is a redirection to an
// ordinary one: group heal is heal, group armor is armor, group recall is
// word of recall. The caster is done *last* — which matters, because a group
// spell can move everybody out of the room.
func (c *Context) spellGroup(number int32, level int32) {
	if !c.Character.Grouped() {
		return
	}

	for _, member := range c.Character.GroupMembers(c.Character.Room) {
		switch number {
		case game.SpellGroupHeal:
			c.applyPoints(game.SpellHeal, member, level)
		case game.SpellGroupArmor:
			c.spellAffect(game.SpellArmor, member)
		case game.SpellGroupRecall:
			if !member.IsNPC() {
				c.moveTo(member, game.MortalStartRoom,
					"%s disappears.\r\n", "%s appears in the middle of the room.\r\n")
			}
		}
	}
}

// summonable are the two mobiles mag_summons can make (magic.c:790). The
// other six numbers in that block are for creatures the C says do not exist.
const (
	mobClone  game.MobVnum = 10
	mobZombie game.MobVnum = 11
)

// summonFailMessages are mag_summon_fail_msgs (magic.c:781). The failure is
// picked at random from entries 2 to 6, so a botched clone blames the
// elements or simply says "Gosh durnit!".
var summonFailMessages = [...]string{
	"\r\n",
	"There are no such creatures.\r\n",
	"Uh oh...\r\n",
	"Oh dear.\r\n",
	"Gosh durnit!\r\n",
	"The elements resist!\r\n",
	"You failed.\r\n",
	"There is no corpse!\r\n",
}

// spellSummon conjures something, porting mag_summons.
//
// Two of the spells work: clone, which fails half the time, and animate dead,
// which needs a corpse and fails one time in ten. The summoned creature is
// charmed rather than merely following, so it fights for you and cannot
// choose to leave.
func (c *Context) spellSummon(number int32, obj *game.Object, level int32) {
	var vnum game.MobVnum
	var failure int32
	var corpse *game.Object

	switch number {
	case game.SpellClone:
		vnum, failure = mobClone, 50
	case game.SpellAnimateDead:
		if obj == nil || !game.IsCorpse(obj) {
			c.Send("%s", summonFailMessages[7])
			return
		}
		corpse = obj
		vnum, failure = mobZombie, 10
	default:
		return
	}

	// A charmed caster cannot have followers of their own.
	if c.Character.Charmed() {
		c.Send("You are too giddy to have any followers!\r\n")
		return
	}
	if c.RNG.Number(0, 101) < failure {
		c.Send("%s", summonFailMessages[c.RNG.Number(2, 6)])
		return
	}

	mob := c.World.SpawnMobile(vnum, c.Character.Room, c.RNG)
	if mob == nil {
		c.Send("You don't quite remember how to make that creature.\r\n")
		return
	}

	// A clone wears the caster's name, and the C is careful to copy it rather
	// than point at the prototype's.
	if number == game.SpellClone {
		mob.Name = c.Character.Name
		mob.Keywords = c.Character.Name
	}
	if mob.Record != nil {
		mob.Record.BaseAffectFlags = mob.Record.BaseAffectFlags.Set(game.AffectCharm)
		game.RecomputeAffects(mob.Record)
	}

	if number == game.SpellClone {
		c.announce("%s magically divides!\r\n", c.Character.Name)
	} else {
		c.announce("%s animates a corpse!\r\n", c.Character.Name)
	}
	c.addFollower(mob, c.Character)

	// The corpse's contents go to the zombie, which is how the dead keep
	// their equipment for one more owner.
	if corpse != nil {
		for _, inside := range append([]*game.Object(nil), corpse.Contents...) {
			c.World.ObjectToChar(inside, mob)
		}
		c.World.ExtractObject(corpse)
	}
}

// spellCharm, porting spell_charm.
//
// The order of the refusals is the C's and each has its own message, so a
// player learns which is which. The duration arithmetic at the end is the
// interesting part: twenty-four hours doubled, multiplied by the caster's
// charisma and divided by the victim's intelligence — so a charismatic mage
// charming something stupid holds it for a very long time, and the guards
// against a zero divisor are the two `if`s rather than any clamping.
func (c *Context) spellCharm(victim *game.Character, level int32) {
	rec := c.Character.Record
	if victim == nil || rec == nil || victim.Record == nil {
		return
	}

	switch {
	case victim == c.Character:
		c.Send("You like yourself even better!\r\n")
	case !victim.IsNPC() && !victim.Record.Preferences.Has(game.PrefSummonable):
		c.Send("You fail because SUMMON protection is on!\r\n")
	case victim.Record.AffectFlags.Has(game.AffectSanctuary):
		c.Send("Your victim is protected by sanctuary!\r\n")
	case victim.HasMobFlag(game.MobNoCharm):
		c.Send("Your victim resists!\r\n")
	case c.Character.Charmed():
		c.Send("You can't have any followers of your own!\r\n")
	case victim.Charmed() || level < victim.Level():
		c.Send("You fail.\r\n")
	case !pkAllowed && !victim.IsNPC():
		c.Send("You fail - shouldn't be doing it anyway.\r\n")
	case game.CircleFollow(victim, c.Character):
		c.Send("Sorry, following in circles can not be allowed.\r\n")
	case game.MakesSavingThrow(victim.Record, victim.IsNPC(), game.SaveParalyse, 0, c.RNG):
		c.Send("Your victim resists!\r\n")
	default:
		if victim.Master != nil {
			c.stopFollowing(victim)
		}
		c.addFollower(victim, c.Character)

		duration := int32(24 * 2)
		if rec.Abilities.Charisma != 0 {
			duration *= rec.Abilities.Charisma
		}
		if victim.Record.Abilities.Intelligence != 0 {
			duration /= victim.Record.Abilities.Intelligence
		}
		game.AddAffect(victim.Record, game.Affect{
			Type:     game.SpellCharm,
			Duration: duration,
			Bits:     game.AffectCharm,
		})

		victim.Tell("Isn't %s just such a nice fellow?\r\n", c.Character.Name)
	}
}

// spellSummonPerson drags somebody to the caster, porting spell_summon.
//
// The local rule is the flat refusal to summon mobiles at all — the C carries
// it between `<DoC>` markers — which turns the spell from a way of moving
// monsters around into a way of moving players.
func (c *Context) spellSummonPerson(victim *game.Character, level int32) {
	if victim == nil {
		return
	}

	switch {
	case victim.Level() > min(game.LevelImmortal-1, level+3):
		c.Send("You failed.\r\n")
		return
	case victim.IsNPC():
		c.Send("Only players may be summoned.\r\n")
		return
	}

	if !pkAllowed && victim.Record != nil && !victim.Record.Preferences.Has(game.PrefSummonable) {
		room := c.World.Room(c.Character.Room)
		name := "somewhere"
		if room != nil {
			name = room.Name
		}
		victim.Tell("%s just tried to summon you to: %s.\r\n"+
			"%s failed because you have summon protection on.\r\n"+
			"Type NOSUMMON to allow other players to summon you.\r\n",
			c.Character.Name, name, capitaliseFirst(c.Character.Subject()))
		c.Send("You failed because %s has summon protection on.\r\n", victim.Name)
		return
	}

	c.moveTo(victim, c.Character.Room,
		"%s disappears suddenly.\r\n", "%s arrives suddenly.\r\n")
	victim.Tell("%s has summoned you!\r\n", c.Character.Name)
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

	case game.SpellCharm:
		c.spellCharm(victim, level)
		return true

	case game.SpellSummon:
		c.spellSummonPerson(victim, level)
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
