// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package session

import (
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

	if c.castSpellFor(info, number, victim, object) && mana > 0 {
		rec.Points.Mana = max(0, min(rec.Points.MaxMana, rec.Points.Mana-mana))
	}
	return nil
}

// castSpellFor is cast_spell (spell_parser.c:473), the step the C puts
// between do_cast and call_magic.
//
// It exists for the three refusals in the middle of it, which had no port at
// all: TargetSelfOnly and TargetNotSelf were being set correctly on thirteen
// spells from the C's own table and then read by nothing, so `cast 'detect
// magic' dog` put detect magic on the dog, and a group spell cast while
// ungrouped did nothing, said nothing and charged the full mana anyway. See
// docs/deviations.md.
//
// Where it sits is load-bearing. The C makes these checks *after* the mana
// check and *after* the skill roll, and returns 0 — and do_cast only spends
// the mana `if (cast_spell(...) && (mana > 0))`. So a refused spell costs
// nothing, but the roll before it has already happened and a lost
// concentration still costs half. Moving the checks up beside do_cast's own
// `tch == ch && violent` test would be tidier and would change both.
//
// Only the object magic path skips this: mag_objectmagic calls call_magic
// directly (spell_parser.c:352-455), so a scroll of detect magic read at
// somebody else is not refused — which is the C's arrangement and not an
// oversight here.
//
// **Three of cast_spell's six parts are still missing** and are #306: the
// per-spell MinPosition check with its five position-specific messages, the
// charmed-caster's "You are afraid you might hurt your master!", and the
// `OK` plus say_spell that announce the casting to the room.
func (c *Context) castSpellFor(info game.SpellInfo, number int32,
	victim *game.Character, object *game.Object,
) bool {
	// `(tch != ch) && IS_SET(SINFO.targets, TAR_SELF_ONLY)`
	// (spell_parser.c:506). Written as a comparison against the caster
	// rather than "there is a victim and it is somebody else" because the
	// C's is: tch is NULL for an object-targeted spell, and NULL != ch. No
	// spell in the table is both self-only and object-targeted, so the two
	// spellings cannot currently disagree — but the C's is the one that
	// stays right if one ever is.
	if victim != c.Character && info.Targets.Has(game.TargetSelfOnly) {
		c.Send("You can only cast this spell upon yourself!\r\n")
		return false
	}
	if victim == c.Character && info.Targets.Has(game.TargetNotSelf) {
		c.Send("You cannot cast this spell upon yourself!\r\n")
		return false
	}
	// mag_groups returns early for an ungrouped caster too (magic.c:855),
	// and the C keeps both: this is what says so and what stops the mana
	// being spent, and that one is the guard on the loop itself.
	if info.Routines.Has(game.MagGroups) && !c.Character.Grouped() {
		c.Send("You can't cast this spell if you're not in a group!\r\n")
		return false
	}

	return c.castSpell(info, number, victim, object, game.SaveSpell)
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
func (c *Context) castSpell(info game.SpellInfo, number int32, victim *game.Character, object *game.Object, save game.SaveType) bool {
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
func (c *Context) applyPoints(number int32, victim *game.Character, level int32) {
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
func (c *Context) spellAffect(number int32, victim *game.Character, save game.SaveType) {
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

// spellDamage applies a damage spell, including the two dispels that can turn
// on the caster.
func (c *Context) spellDamage(info game.SpellInfo, number int32, victim *game.Character, level int32) {
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
