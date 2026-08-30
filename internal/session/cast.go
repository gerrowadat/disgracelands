// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package session

import (
	"fmt"

	"github.com/gerrowadat/disgracelands/internal/colour"
	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/obs"
)

// doCast, porting do_cast (spell_parser.c).
//
// The order of the checks is the C's throughout, because each one has its own
// message and a player learns which refusal means what. Silence first, then
// the paladin's standing, then the spell name, then whether they know it,
// then the target, then the mana, and only then the roll.
func doCast(c *Context) error {
	rec := c.Character.Record
	if c.Character.IsNPC() || rec == nil {
		return nil
	}

	if rec.AffectFlags.Has(game.AffectSilence) {
		c.Send("You try, but the words simply fail you.\r\n")
		return nil
	}

	// The paladin's standing. This can change the character — being cast out
	// is permanent — so it runs before anything else could refuse the spell
	// for a lesser reason.
	if verdict := game.JudgePaladin(rec); !verdict.Allowed || verdict.Message != "" {
		if verdict.Message != "" {
			c.Send("%s", verdict.Message)
		}
		if verdict.Broadcast != "" {
			c.broadcast("%s\r\n", verdict.Broadcast)
		}
		if !verdict.Allowed {
			return nil
		}
	}

	spellName, targetName, parseErr := game.ParseCastArgument(c.Arg)
	if parseErr != "" {
		c.Send("%s", parseErr)
		return nil
	}

	number, ok := game.SpellNumberByName(spellName)
	if !ok || number < 1 || number > game.MaxSpells {
		c.Send("Cast what?!?\r\n")
		return nil
	}
	info, ok := game.Spell(number)
	if !ok {
		c.Send("Cast what?!?\r\n")
		return nil
	}

	// Any class they have ever been, not just the one they are — the local
	// remort rule. See game.KnowsSpell.
	if !game.KnowsSpell(rec, info) {
		c.Send("You do not know that spell!\r\n")
		return nil
	}
	if rec.Skills[number] == 0 {
		c.Send("You are unfamiliar with that spell.\r\n")
		return nil
	}

	victim, object, found := c.findSpellTarget(info, targetName)
	if !found {
		if targetName != "" {
			c.Send("Cannot find the target of your spell!\r\n")
		} else {
			c.Send("%s", game.TargetQuestion(info))
		}
		return nil
	}
	if victim == c.Character && info.Violent {
		c.Send("You shouldn't cast that on yourself -- could be bad for your health!\r\n")
		return nil
	}

	mana := game.ManaCost(info, rec.Level)
	if mana > 0 && rec.Points.Mana < mana && rec.Level < game.LevelImmortal {
		c.Send("You haven't the energy to cast that spell!\r\n")
		return nil
	}

	// "You throws the dice and you takes your chances.. 101% is total
	// failure", says the C. A skill of 100 still fails one time in 102.
	if c.RNG.Number(0, 101) > rec.Skills[number] {
		c.Send("You lost your concentration!\r\n")
		if mana > 0 {
			// Half the mana is spent anyway, which is what makes a low skill
			// expensive rather than merely useless.
			rec.Points.Mana = max(0, min(rec.Points.MaxMana, rec.Points.Mana-mana/2))
		}
		// A botched violent spell provokes the mobile it was aimed at.
		if info.Violent && victim != nil && victim.IsNPC() {
			if _, message := c.World.SetFighting(victim, c.Character); message != "" {
				c.wizlog(obs.LogBrief, game.LevelImmortal, "%s", message)
			}
		}
		return nil
	}

	// cast_spell's own refusals (spell_parser.c:482-522), in the C's order.
	// The first two are #306; the three after them were #301.
	//
	// The per-spell minimum position, which is *on top of* the cast
	// command's own POS_SITTING from cmd_info[]. Without it a resting mage
	// could cast fireball and a fighting one could cast anything, rather
	// than only the POS_FIGHTING column. The message is chosen by the
	// caster's position rather than the spell's, and there are five of
	// them.
	if c.Character.Position < info.MinPosition {
		switch c.Character.Position {
		case game.PosSleeping:
			c.Send("You dream about great magical powers.\r\n")
		case game.PosResting:
			c.Send("You cannot concentrate while resting.\r\n")
		case game.PosSitting:
			c.Send("You can't do this sitting!\r\n")
		case game.PosFighting:
			c.Send("Impossible!  You can't concentrate enough!\r\n")
		default:
			c.Send("You can't do much of anything like this!\r\n")
		}
		return nil
	}

	// A charmed caster will not cast at whoever charmed them. Note it is
	// the *master* relationship and not the charm alone: a charmed pet can
	// still cast at anybody else in the room.
	if rec.AffectFlags.Has(game.AffectCharm) && victim != nil && c.Character.Master == victim {
		c.Send("You are afraid you might hurt your master!\r\n")
		return nil
	}

	// cast_spell's own three refusals (spell_parser.c:506-522). They are
	// *here*, after the concentration roll and before call_magic, because
	// that is where the C has them: do_cast rolls, and only then calls
	// cast_spell, which makes these checks and returns 0.
	//
	// Where they sit is not cosmetic. Two things follow from it:
	//
	//   - A refusal costs nothing at all. The full mana is subtracted only
	//     when cast_spell returns 1, and the half-mana penalty belongs to
	//     the failed roll, which this is not. `cast 'group heal'` while
	//     ungrouped used to charge all 60 for a spell that did nothing and
	//     said nothing -- spellGroup returns early when the caster is not
	//     grouped, but castSpell had already set did = true (#301).
	//   - A caster who fails the roll never sees them. Aim a self-only
	//     spell at somebody else and lose the concentration roll, and the
	//     C says "You lost your concentration!" and takes half the mana,
	//     exactly as it would for any other spell.
	//
	// They are not in castSpell, which is call_magic: a wand or a scroll
	// reaches call_magic without going through cast_spell, so
	// `mag_objectmagic` has never made these checks and neither does this.
	if victim != nil && victim != c.Character && info.Targets.Has(game.TargetSelfOnly) {
		c.Send("You can only cast this spell upon yourself!\r\n")
		return nil
	}
	if victim == c.Character && info.Targets.Has(game.TargetNotSelf) {
		c.Send("You cannot cast this spell upon yourself!\r\n")
		return nil
	}
	if info.Routines.Has(game.MagGroups) && !c.Character.Grouped() {
		c.Send("You can't cast this spell if you're not in a group!\r\n")
		return nil
	}

	// Everything above returns 0 in the C; from here the cast happens, and
	// these two lines are the last thing cast_spell does before call_magic
	// (spell_parser.c:523-524).
	// config.c:99: OK is "Okay.", not "Ok." -- same string do_gen_door
	// sends, for the same reason.
	c.Send("Okay.\r\n")
	c.saySpell(info, victim, object)

	if c.castSpell(info, number, victim, object, game.SaveSpell) && mana > 0 {
		rec.Points.Mana = max(0, min(rec.Points.MaxMana, rec.Points.Mana-mana))
	}
	return nil
}

// saySpell is say_spell (spell_parser.c:116-171): the line everybody else in
// the room gets when a spell is cast.
//
// It is not flavour. It is the only warning a player ever had that something
// was coming, and it is deliberately partial information: a bystander who
// shares the caster's class hears the spell's real name, and everybody else
// hears the syllable table's gibberish (game.ScrambleSpellName). Nobody in
// the room was told anything at all before this (#306).
//
// Four details worth keeping, each of which the C is specific about:
//
//   - The audience is the caster's room, minus the caster and the target.
//     The target gets their own sentence at the end, in the second person.
//   - Anyone asleep or without a descriptor is skipped -- so mobiles hear
//     nothing, which is why a charmed mob is not told what its master is
//     casting.
//   - The object branch requires the object to be in the caster's room or
//     carried by them. An object found anywhere in the world (locate
//     object) leaves the caster staring at nothing, and the C says so by
//     falling through to the plain sentence.
//   - say_spell calls perform_act per audience rather than act(), so it
//     makes no visibility check of its own: an invisible caster's words are
//     still heard, and $n resolves per audience through the ordinary CAN_SEE
//     that game.Act already does.
func (c *Context) saySpell(info game.SpellInfo, victim *game.Character, obj *game.Object) {
	caster := c.Character
	sameRoom := victim != nil && victim.Room == caster.Room
	atObj := obj != nil && !sameRoom && c.objectIsHere(obj)

	format := game.SaySpellFormat(sameRoom && victim == caster, sameRoom && victim != caster, atObj)
	real := fmt.Sprintf(format, info.Name)
	scrambled := fmt.Sprintf(format, game.ScrambleSpellName(info.Name))

	args := game.ActArgs{Actor: caster, Obj: obj, Victim: victim}
	for _, other := range c.World.Occupants(caster.Room) {
		if other == caster || other == victim || other.IsNPC() || other.Position <= game.PosSleeping {
			continue
		}
		said := scrambled
		if other.Record != nil && caster.Record != nil && other.Record.Class == caster.Record.Class {
			said = real
		}
		other.Tell("%s", c.World.Act(said, args, other))
	}

	if sameRoom && victim != caster {
		name := game.ScrambleSpellName(info.Name)
		if victim.Record != nil && caster.Record != nil && victim.Record.Class == caster.Record.Class {
			name = info.Name
		}
		victim.Tell("%s", c.World.Act(
			fmt.Sprintf("$n stares at you and utters the words, '%s'.", name), args, victim))
	}
}

// objectIsHere is say_spell's own test for whether the caster can be said to
// be staring at an object: `IN_ROOM(tobj) == IN_ROOM(ch) || tobj->carried_by
// == ch` (spell_parser.c:148-149). Equipment is not included, which is the
// C's omission and not this one's.
func (c *Context) objectIsHere(obj *game.Object) bool {
	for _, held := range c.Character.Carrying {
		if held == obj {
			return true
		}
	}
	for _, here := range c.World.RoomObjects(c.Character.Room) {
		if here == obj {
			return true
		}
	}
	return false
}

// findSpellTarget resolves what the spell is aimed at, porting the target
// block of do_cast.
//
// The order the C tries things in is the order here: a named target is looked
// for in the room, then the world, then the caster's inventory, then their
// equipment, then the room's floor. With no name given it falls back to the
// current fight and finally to the caster themselves — but only for a spell
// that is not violent, which is why `cast 'armor'` works and `cast 'harm'`
// asks who.
func (c *Context) findSpellTarget(info game.SpellInfo, name string) (*game.Character, *game.Object, bool) {
	if info.Targets.Has(game.TargetIgnore) {
		return nil, nil, true
	}

	if name != "" {
		if info.Targets.Has(game.TargetCharRoom) {
			if victim := c.findInRoom(name); victim != nil {
				return victim, nil, true
			}
		}
		if info.Targets.Has(game.TargetCharWorld) {
			if victim := c.findAnywhere(name); victim != nil {
				return victim, nil, true
			}
		}
		if info.Targets.Has(game.TargetObjInv) {
			if obj := c.findObject(c.Character.Carrying, name); obj != nil {
				return nil, obj, true
			}
		}
		if info.Targets.Has(game.TargetObjEquip) {
			for _, obj := range c.Character.Equipment {
				if obj != nil && obj.Matches(name) {
					return nil, obj, true
				}
			}
		}
		if info.Targets.Has(game.TargetObjRoom) {
			if obj := c.findObject(c.World.RoomObjects(c.Character.Room), name); obj != nil {
				return nil, obj, true
			}
		}
		if info.Targets.Has(game.TargetObjWorld) {
			// Anywhere at all, which only locate object asks for — and which
			// is why locate object can only search by the first keyword of
			// whatever the search happened to land on first.
			if obj := c.findObject(c.World.Objects(), name); obj != nil {
				return nil, obj, true
			}
		}
		return nil, nil, false
	}

	if info.Targets.Has(game.TargetFightSelf) && c.Character.Fighting != nil {
		return c.Character, nil, true
	}
	if info.Targets.Has(game.TargetFightVict) && c.Character.Fighting != nil {
		return c.Character.Fighting, nil, true
	}
	if info.Targets.Has(game.TargetCharRoom) && !info.Violent {
		return c.Character, nil, true
	}
	return nil, nil, false
}

// broadcast tells the whole game, which is what send_to_all (comm.c:2245)
// does: no colour, and — unlike its coloured sibling — no exclusions either.
func (c *Context) broadcast(format string, args ...any) {
	for _, other := range c.World.Players() {
		other.Tell(format, args...)
	}
}

// broadcastAt is send_to_all_color (comm.c:2256), which is a different
// function in the C and not merely broadcast with an escape in front: it
// applies the reader's own COLOR_LEV threshold, and it skips anybody carrying
// PLR_WRITING. game.Live.Announce is both, so that the four `<DoC>` callers
// share one implementation rather than four loops.
func (c *Context) broadcastAt(tier game.Announcement, want colour.Level, format string, args ...any) {
	c.World.Announce(tier, want, format, args...)
}

// castSpell runs the spell's routines, porting call_magic.
//
// All ten routines are implemented. A spell whose routines all decline to do
// anything says so rather than silently doing nothing and charging for it — a
// player who cannot tell "this spell has no effect" from "this spell is not
// written yet" cannot report a bug.
func (c *Context) castSpell(info game.SpellInfo, number game.SpellID, victim *game.Character, object *game.Object, save game.SaveType) bool {
	rec := c.Character.Record
	level := rec.Level

	// The two room checks at the top of call_magic. A no-magic room stops
	// everything; a peaceful one stops anything violent, which includes every
	// damage spell whether or not it is flagged violent.
	if room := c.World.Room(c.Character.Room); room != nil {
		if room.Flags.Has(game.RoomNoMagic) {
			c.Send("Your magic fizzles out and dies.\r\n")
			c.announce("%s's magic fizzles out and dies.\r\n", c.Character.Name)
			return false
		}
		if room.Flags.Has(game.RoomPeaceful) &&
			(info.Violent || info.Routines.Has(game.MagDamage)) {
			c.Send("A flash of white light fills the room, dispelling your violent magic!\r\n")
			c.announce("White light from no particular source suddenly fills the room, then vanishes.\r\n")
			return false
		}
	}

	var did bool

	if info.Routines.Has(game.MagDamage) && victim != nil {
		did = true
		c.spellDamage(info, number, victim, level)
	}

	if info.Routines.Has(game.MagPoints) && victim != nil {
		did = true
		c.applyPoints(number, victim, level)
	}

	if info.Routines.Has(game.MagAffects) && victim != nil && victim.Record != nil {
		did = true
		c.spellAffect(number, victim, save)
	}

	if info.Routines.Has(game.MagUnaffects) && victim != nil && victim.Record != nil {
		did = true
		c.spellUnaffect(number, victim)
	}

	if info.Routines.Has(game.MagAlterObjs) && object != nil {
		did = true
		c.spellAlterObject(number, object)
	}

	if info.Routines.Has(game.MagAreas) {
		did = true
		c.spellArea(info, number, level)
	}

	if info.Routines.Has(game.MagGroups) {
		did = true
		c.spellGroup(number, level)
	}

	if info.Routines.Has(game.MagSummons) {
		did = true
		c.spellSummon(number, object, level)
	}

	// MAG_MASSES is in the C's switch and its switch is empty: no spell in
	// stock CircleMUD is a mass spell. Counted as done so that a spell
	// flagged with it and nothing else does not claim to be unimplemented.
	if info.Routines.Has(game.MagMasses) {
		did = true
	}

	if info.Routines.Has(game.MagCreations) {
		did = true
		c.spellCreation(number)
	}

	if info.Routines.Has(game.MagManual) && c.castManual(number, victim, object, level) {
		did = true
	}

	if !did {
		// Named so the player knows the spell exists and this server has not
		// finished it, rather than wondering whether they missed.
		c.Send("Nothing seems to happen. (%s is not implemented yet.)\r\n", info.Name)
		return false
	}
	return true
}

// applyPoints heals or drains, porting mag_points.
func (c *Context) applyPoints(number game.SpellID, victim *game.Character, level int32) {
	healing := game.SpellHealing(number, victim.Record, level, c.RNG)
	if victim.Record != nil {
		victim.Record.Points.Hit = min(
			victim.Record.Points.MaxHit,
			victim.Record.Points.Hit+healing.Amount)
		victim.Position = game.UpdatePosition(victim.Record, victim.Position)
	}
	if healing.Message != "" {
		victim.Tell("%s", healing.Message)
	}
}

// spellAffect applies an affect spell, porting the tail of mag_affects.
func (c *Context) spellAffect(number game.SpellID, victim *game.Character, save game.SaveType) {
	rec := c.Character.Record

	// One saving throw per casting, rolled here so every spell that consults
	// it sees the same answer. Which of the five it is depends on where the
	// magic came from: a spell cast by a person is SAVING_SPELL, and anything
	// out of a wand, staff, scroll or potion is SAVING_ROD — which is the
	// column with the *better* numbers, so a scroll is easier to resist than
	// the same spell cast at you.
	saved := game.MakesSavingThrow(victim.Record, victim.IsNPC(), save, 0, c.RNG)

	result := game.AffectsOfSpell(number, rec, victim.Record,
		victim.IsNPC(), victim.MobFlags(), rec.Level, saved, c.RNG)

	if result.Refused {
		if result.RefusalToCaster != "" {
			c.Send("%s", result.RefusalToCaster)
		}
		return
	}
	if !game.CanAffect(result, victim.Record, victim.IsNPC(), number) {
		c.Send("%s", game.NoEffect)
		return
	}

	game.ApplyAffectSpell(result, victim.Record)

	if result.SleepsVictim && victim.Position > game.PosSleeping {
		victim.Tell("You feel very sleepy...  Zzzz......\r\n")
		for _, other := range c.World.Occupants(victim.Room) {
			if other != victim {
				other.Tell("%s goes to sleep.\r\n", victim.Name)
			}
		}
		victim.Position = game.PosSleeping
	}

	if result.ToVictim != "" {
		victim.Tell("%s\r\n", result.ToVictim)
	}
	if result.ToRoom != "" {
		for _, other := range c.World.Occupants(victim.Room) {
			if other != victim {
				other.Tell(result.ToRoom+"\r\n", victim.Name)
			}
		}
	}
}

// spellUnaffect is mag_unaffects (magic.c:901).
//
// The whole of it is the first line: the spell being cast is the *cure*, and
// what comes off is the affliction it cures. This used to hand
// RemoveAffectsOf the cure's own number, so `cure blind` removed affects of
// type 14, `remove poison` type 43 and `remove curse` type 35 -- numbers
// nothing ever applies -- and blindness, poison and curses were never
// touched by the three spells named after them (#299).
//
// The messages come with the mapping because in the C they are part of the
// same switch: "Your vision returns!" and a room line, not one "You feel
// better." for all three.
func (c *Context) spellUnaffect(number game.SpellID, victim *game.Character) {
	cure, ok := game.UnaffectionOf(number)
	if !ok {
		// The C logs a SYSERR and returns. `full heal` reaches this on
		// every cast; see game.UnaffectionOf.
		return
	}

	if !game.RemoveAffectsOf(victim.Record, cure.Affliction) {
		if !cure.Silent {
			c.Send("%s", game.NoEffect)
		}
		return
	}

	if cure.ToVictim != "" {
		victim.Tell("%s\r\n", cure.ToVictim)
	}
	if cure.ToRoom != "" {
		for _, other := range c.World.Occupants(victim.Room) {
			if other != victim {
				other.Tell(cure.ToRoom+"\r\n", victim.Name)
			}
		}
	}
}

// spellDamage applies a damage spell, including the two dispels that can turn
// on the caster.
func (c *Context) spellDamage(info game.SpellInfo, number game.SpellID, victim *game.Character, level int32) {
	rec := c.Character.Record

	target := victim
	var damage int32

	switch number {
	case game.SpellDispelEvil, game.SpellDispelGood:
		result := game.Dispel(number, rec, victim.Record, c.RNG)
		if result.Protected {
			c.Send("The gods protect %s.\r\n", victim.Name)
			return
		}
		if result.Backfired {
			target = c.Character
		}
		damage = result.Damage
	default:
		damage = game.SpellDamage(number, rec, victim.Record, level, c.RNG)
	}

	if target.Record == nil {
		return
	}

	// Through the same path as a swing: a spell that kills leaves a corpse and
	// pays out. damage() starts the fight whatever the spell's violent flag
	// says — the flag decides whether it may be cast at all in a peaceful
	// room, not whether being blasted is provocation.
	//
	// SkillDamage, not Damage: mag_damage's own C ends with
	// `return (damage(ch, victim, dam, spellnum))` (magic.c:294) — the same
	// damage() dispatch do_kick/do_bash/do_backstab already go through, with
	// the spell number as the attack type. A spell number is always below
	// TypeHit, so damage()'s IS_WEAPON check sends it down the non-weapon
	// path: skill_message alone, no fallback to dam_message, silence for
	// anything unregistered — exactly what SkillDamage already does. There
	// is no message of this function's own left to print.
	c.Violence.SkillDamage(c.World, c.Character, target, damage, number)
}
