// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package session

import (
	"fmt"
	"strings"
	"time"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// do_stat and friends, ported from act.wizard.c:505.
//
// The builders' window on the world: every field of a room, an object or a
// character, printed as it is actually stored. Nearly all of it is bitfields
// run through sprintbit, which is why the name tables in
// internal/game/bitnames.go are now checked against constants.c — a name
// shifted by one renames every flag after it and nothing else would notice.

// doStat is do_stat (act.wizard.c:973).
func doStat(c *Context) error {
	what, name := halfChop(c.Arg)

	switch {
	case what == "":
		c.Send("Stats on who or what?\r\n")
		return nil
	case isPrefixOf(what, "room"):
		c.statRoom()
		return nil
	case isPrefixOf(what, "mob"):
		if victim := c.findInRoom(name); victim != nil {
			c.statCharacter(victim)
		} else {
			c.Send("No such monster around.\r\n")
		}
		return nil
	case isPrefixOf(what, "player"):
		if victim := c.World.Find(name); victim != nil {
			c.statCharacter(victim)
		} else {
			c.Send("No such player around.\r\n")
		}
		return nil
	case isPrefixOf(what, "object"):
		if obj := c.findObjectAnywhere(name); obj != nil {
			c.statObject(obj)
		} else {
			c.Send("No such object around.\r\n")
		}
		return nil
	}

	// No keyword: whatever answers to the word (act.wizard.c:871-889).
	//
	// This is not generic_find, and the difference is the point. It is a
	// hand-rolled `else if` ladder in its own order — **worn, carried,
	// somebody in this room, on the floor, somebody anywhere, an object
	// anywhere** — with objects and characters interleaved rather than all
	// the characters first. And it threads one `number` down the whole chain,
	// exactly as generic_find does, so `2.sword` is the second match across
	// that order.
	//
	// This port had the room's characters first and a fresh count for every
	// step, so `stat sword` with a sword in hand and a mobile answering to
	// "sword" in the room stated the mobile (#194).
	target := strings.TrimSpace(c.Arg)
	s := c.World.NewSearch(c.Character, target)
	if obj := s.VisibleEquippedObject(&c.Character.Equipment); obj != nil {
		c.statObject(obj)
		return nil
	}
	if obj := s.ObjectIn(c.Character.Carrying); obj != nil {
		c.statObject(obj)
		return nil
	}
	if victim := c.World.SearchInRoom(s, c.Character, c.Character.Room); victim != nil {
		c.statCharacter(victim)
		return nil
	}
	if obj := s.ObjectIn(c.World.RoomObjects(c.Character.Room)); obj != nil {
		c.statObject(obj)
		return nil
	}
	if victim := c.World.SearchAnywhere(s, c.Character); victim != nil {
		c.statCharacter(victim)
		return nil
	}
	if obj := s.ObjectIn(c.World.Objects()); obj != nil {
		c.statObject(obj)
		return nil
	}
	c.Send("Nothing around by that name.\r\n")
	return nil
}

// doVstat is do_vstat (act.wizard.c:1287): stat a prototype rather than a
// thing in the world.
//
// The C makes a real one, stats it, and extracts it again — which is why
// `vstat mob` on a mobile with a spec proc briefly puts one in room zero.
// Here the prototype is stated directly, so nothing is created; the output is
// the same because everything printed comes from the prototype anyway.
func doVstat(c *Context) error {
	what, number := halfChop(c.Arg)
	if what == "" || number == "" || !isNumber(number) {
		c.Send("Usage: vstat { obj | mob } <number>\r\n")
		return nil
	}

	switch {
	case isPrefixOf(what, "mob"):
		def := c.World.MobileDef(game.MobVnum(atoi(number)))
		if def == nil {
			c.Send("There is no monster with that number.\r\n")
			return nil
		}
		// The C reads a real one, puts it in room zero, stats it and
		// extracts it again (act.wizard.c:1305). Spawning it where the
		// caller is standing and removing it immediately is the same thing
		// without the trip through room zero — and it matters, because
		// `read_mobile` rolls hit points and `stat` prints them.
		mob := c.World.SpawnMobile(def.Vnum, c.Character.Room, c.RNG)
		if mob == nil {
			c.Send("There is no monster with that number.\r\n")
			return nil
		}
		c.statCharacter(mob)
		c.World.Remove(mob)
	case isPrefixOf(what, "obj"):
		obj := c.World.NewObject(game.ObjVnum(atoi(number)))
		if obj == nil {
			c.Send("There is no object with that number.\r\n")
			return nil
		}
		c.statObject(obj)
		c.World.ExtractObject(obj)
	default:
		c.Send("That'll have to be either 'obj' or 'mob'.\r\n")
	}
	return nil
}

// doVnum is do_vnum (act.wizard.c:485): what numbers answer to a name.
func doVnum(c *Context) error {
	what, name := halfChop(c.Arg)
	if what == "" || name == "" || (!isPrefixOf(what, "mob") && !isPrefixOf(what, "obj")) {
		c.Send("Usage: vnum { obj | mob } <name>\r\n")
		return nil
	}

	var b strings.Builder
	found := 0
	if isPrefixOf(what, "mob") {
		for _, def := range c.World.MobileDefs() {
			if game.MatchesAnyKeyword(def.Keywords, name) {
				found++
				fmt.Fprintf(&b, "%3d. [%5d] %s\r\n", found, def.Vnum, def.ShortDesc)
			}
		}
		if found == 0 {
			c.Send("No mobiles by that name.\r\n")
			return nil
		}
	} else {
		for _, def := range c.World.ObjectDefs() {
			if game.MatchesAnyKeyword(def.Keywords, name) {
				found++
				fmt.Fprintf(&b, "%3d. [%5d] %s\r\n", found, def.Vnum, def.ShortDesc)
			}
		}
		if found == 0 {
			c.Send("No objects by that name.\r\n")
			return nil
		}
	}
	c.Send("%s", b.String())
	return nil
}

// statRoom is do_stat_room (act.wizard.c:504).
func (c *Context) statRoom() {
	room := c.World.Room(c.Character.Room)
	if room == nil {
		c.Send("You are nowhere.\r\n")
		return
	}

	// The colour is do_stat_room's own, at C_NRM throughout: the name cyan,
	// the vnum green, the extra-description keywords cyan, the people
	// yellow, the contents green, and each exit's direction and destination
	// cyan (act.wizard.c:512-596).
	var b strings.Builder
	fmt.Fprintf(&b, "Room name: {{cyan}}%s{{/}}\r\n", room.Name)
	fmt.Fprintf(&b, "Zone: [%3d], VNum: [{{green}}%5d{{/}}], RNum: [%5d], Type: %s\r\n",
		zoneNumberOf(c.World, room.Vnum), room.Vnum, room.Vnum,
		game.SprintType(room.SectorType, game.SectorNames()))
	fmt.Fprintf(&b, "SpecProc: %s, Flags: %s\r\n",
		existsOrNone(room.Spec != ""), game.SprintBit(room.Flags.Raw(), game.RoomBitNames()))

	b.WriteString("Description:\r\n")
	if room.Description != "" {
		b.WriteString(ensureNewline(room.Description))
	} else {
		b.WriteString("  None.\r\n")
	}

	if len(room.ExtraDescs) > 0 {
		b.WriteString("Extra descs:{{cyan}}")
		for _, extra := range room.ExtraDescs {
			b.WriteString(" " + extra.Keywords)
		}
		b.WriteString("{{/}}\r\n")
	}

	b.WriteString("Chars present:{{yellow}}")
	names := make([]string, 0, len(c.World.Occupants(room.Vnum)))
	for _, who := range c.World.Occupants(room.Vnum) {
		names = append(names, fmt.Sprintf("%s(%s)", who.Name, kindOf(who)))
	}
	b.WriteString(joinWrapped(names))
	b.WriteString("{{/}}")

	if objects := c.World.RoomObjects(room.Vnum); len(objects) > 0 {
		b.WriteString("Contents:{{green}}")
		shorts := make([]string, 0, len(objects))
		for _, obj := range objects {
			shorts = append(shorts, obj.Name())
		}
		b.WriteString(joinWrapped(shorts))
		b.WriteString("{{/}}")
	}

	for dir := game.Direction(0); int(dir) < game.NumDirections; dir++ {
		exit := room.Exits[dir]
		if exit == nil {
			continue
		}
		to := " {{cyan}}NONE{{/}}"
		if exit.ToRoom != game.NoRoom {
			to = fmt.Sprintf("{{cyan}}%5d{{/}}", exit.ToRoom)
		}
		keyword := exit.Keywords
		if keyword == "" {
			keyword = "None"
		}
		fmt.Fprintf(&b, "Exit {{cyan}}%-5s{{/}}:  To: [%s], Key: [%5d], Keywrd: %s, Type: %s\r\n ",
			dir, to, exit.Key, keyword,
			game.SprintBit(exit.State.Raw(), game.ExitBitNames()))
		if exit.Description != "" {
			b.WriteString(ensureNewline(exit.Description))
		} else {
			b.WriteString("  No exit description.\r\n")
		}
	}

	c.Send("%s", b.String())
}

// zoneNumberOf is `zone_table[rm->zone].number`: which zone a room belongs
// to, or -1 for a room in none.
func zoneNumberOf(w *game.Live, vnum game.RoomVnum) int32 {
	if zone := w.ZoneOf(vnum); zone != nil {
		return int32(zone.Vnum)
	}
	return -1
}

// statObject is do_stat_object (act.wizard.c:605).
func (c *Context) statObject(obj *game.Object) {
	var b strings.Builder

	vnum := game.NoObject
	if obj.Def != nil {
		vnum = obj.Def.Vnum
	}
	// Yellow short description, green vnum, cyan extra-description keywords,
	// green contents — do_stat_object's own, all at C_NRM
	// (act.wizard.c:612-746).
	fmt.Fprintf(&b, "Name: '{{yellow}}%s{{/}}', Aliases: %s\r\n", orNone(obj.Name()), obj.Keywords)
	fmt.Fprintf(&b, "VNum: [{{green}}%5d{{/}}], RNum: [%5d], Type: %s, SpecProc: %s\r\n",
		vnum, vnum, game.SprintType(obj.Type, game.ItemTypeNames),
		existsOrNone(obj.ObjSpec() != ""))
	fmt.Fprintf(&b, "L-Des: %s\r\n", orNone(obj.Description))

	fmt.Fprintf(&b, "Can be worn on: %s\r\n", game.SprintBit(obj.WearFlags.Raw(), game.WearBitNames()))
	fmt.Fprintf(&b, "Set char bits : %s\r\n", game.SprintBit(obj.PermAffect.Raw(), game.AffectBitNames()))
	fmt.Fprintf(&b, "Extra flags   : %s\r\n", game.SprintBit(obj.ExtraFlags.Raw(), game.ExtraBitNames()))

	fmt.Fprintf(&b, "Weight: %d, Value: %d, Cost/day: %d, Timer: %d, Min Level: %d\r\n",
		obj.Weight, obj.Cost, obj.RentPerDay(), obj.Timer, obj.MinLevel())

	where := "Nowhere"
	if obj.Location == game.InRoom {
		where = fmt.Sprintf("%d", obj.Room)
	}
	fmt.Fprintf(&b, "In room: %s, In object: %s, Carried by: %s, Worn by: %s\r\n",
		where, nameOrNone(obj.Container), holderName(obj, game.CarriedBy),
		holderName(obj, game.WornBy))

	b.WriteString(objectValues(obj) + "\r\n")

	if len(obj.Contents) > 0 {
		b.WriteString("\r\nContents:{{green}}")
		shorts := make([]string, 0, len(obj.Contents))
		for _, inside := range obj.Contents {
			shorts = append(shorts, inside.Name())
		}
		b.WriteString(joinWrapped(shorts))
		b.WriteString("{{/}}")
	}

	b.WriteString("Affections:")
	found := 0
	for _, aff := range obj.Affects {
		if aff.Modifier == 0 {
			continue
		}
		if found > 0 {
			b.WriteString(",")
		}
		found++
		fmt.Fprintf(&b, " %+d to %s", aff.Modifier,
			game.SprintType(aff.Location, game.ApplyTypeNames()))
	}
	if found == 0 {
		b.WriteString(" None")
	}
	b.WriteString("\r\n")

	c.Send("%s", b.String())
}

// objectValues is the type-dependent line of do_stat_object: the four values
// read as whatever this kind of object uses them for.
func objectValues(obj *game.Object) string {
	v := obj.Values
	switch obj.Type {
	case game.ItemLight:
		if v[2] == -1 {
			return "Hours left: Infinite"
		}
		return fmt.Sprintf("Hours left: [%d]", v[2])
	case game.ItemScroll, game.ItemPotion:
		return fmt.Sprintf("Spells: (Level %d) %s, %s, %s", v[0],
			game.SpellName(v[1]), game.SpellName(v[2]), game.SpellName(v[3]))
	case game.ItemWand, game.ItemStaff:
		return fmt.Sprintf("Spell: %s at level %d, %d (of %d) charges remaining",
			game.SpellName(v[3]), v[0], v[2], v[1])
	case game.ItemWeapon:
		return fmt.Sprintf("Todam: %dd%d, Message type: %d", v[1], v[2], v[3])
	case game.ItemArmor:
		return fmt.Sprintf("AC-apply: [%d]", v[0])
	case game.ItemTrap:
		return fmt.Sprintf("Spell: %d, - Hitpoints: %d", v[0], v[1])
	case game.ItemContainer:
		return fmt.Sprintf("Weight capacity: %d, Lock Type: %s, Key Num: %d, Corpse: %s",
			v[0], game.SprintBit(uint64(v[1]), game.ContainerBitNames()), //nolint:gosec // a bitfield
			v[2], yesNo(v[3] != 0))
	case game.ItemDrinkCon, game.ItemFountain:
		return fmt.Sprintf("Capacity: %d, Contains: %d, Poisoned: %s, Liquid: %s",
			v[0], v[1], yesNo(v[3] != 0), game.DrinkName(v[2]))
	case game.ItemNote:
		return fmt.Sprintf("Tongue: %d", v[0])
	case game.ItemKey:
		return ""
	case game.ItemFood:
		return fmt.Sprintf("Makes full: %d, Poisoned: %s", v[0], yesNo(v[3] != 0))
	case game.ItemMoney:
		return fmt.Sprintf("Coins: %d", v[0])
	}
	return fmt.Sprintf("Values 0-3: [%d] [%d] [%d] [%d]", v[0], v[1], v[2], v[3])
}

// statCharacter is do_stat_character (act.wizard.c:779).
func (c *Context) statCharacter(k *game.Character) {
	rec := k.Record
	if rec == nil {
		c.Send("There is nothing to say about them.\r\n")
		return
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s %s '%s'  IDNum: [%5d], In room [%5d]\r\n",
		game.SprintType(rec.Sex.Number(), game.GenderNames()), kindOf(k), k.Name, rec.IDNum, k.Room)

	if k.IsNPC() && k.MobDef != nil {
		fmt.Fprintf(&b, "Alias: %s, VNum: [%5d], RNum: [%5d]\r\n",
			k.MobDef.Keywords, k.MobDef.Vnum, k.MobDef.Vnum)
	}
	fmt.Fprintf(&b, "Title: %s\r\n", orAngleNone(rec.Title))
	fmt.Fprintf(&b, "L-Des: %s", longDescOf(k))

	classNames := game.PcClassNames()
	label := "Class: "
	if k.IsNPC() {
		classNames, label = game.NpcClassNames(), "Monster Class: "
	}
	// do_stat_character's colour, all at C_NRM (act.wizard.c:812-954): the
	// level and experience yellow, the six abilities cyan, the three point
	// pools green, the flag words cyan and green and yellow, and each
	// affecting spell's name cyan.
	fmt.Fprintf(&b, "%s%s, Lev: [{{yellow}}%2d{{/}}], XP: [{{yellow}}%7d{{/}}], Align: [%4d]\r\n",
		label, game.SprintType(rec.Class.Number(), classNames),
		rec.Level, rec.Points.Exp, rec.Alignment)

	if !k.IsNPC() {
		hours := int64(rec.Played / time.Hour)
		minutes := int64(rec.Played/time.Minute) % 60
		fmt.Fprintf(&b, "Created: [%s], Last Logon: [%s], Played [%dh %dm], Age [%d]\r\n",
			statDate(rec.Birth), statDate(rec.LastLogon), hours, minutes,
			game.Age(rec, time.Now()))
		// Speaks is GET_TALK(k, 0..2), the three tongues. Nothing in this
		// tree ever sets them and `speak` was never ported, so they are
		// always zero — printed anyway, because `stat` prints what is stored.
		fmt.Fprintf(&b, "Hometown: [%d], Speaks: [0/0/0], (STL[%d]/per[%d]/NSTL[%d])",
			rec.Hometown, rec.SpellsToLearn,
			game.LearnPercent(rec.Abilities.Intelligence),
			game.Practices(rec.Abilities.Wisdom))
		if rec.Level >= game.LevelImmortal {
			fmt.Fprintf(&b, ", OLC[%d]", rec.OLCZone)
		}
		b.WriteString("\r\n")
	}

	a := rec.Abilities
	fmt.Fprintf(&b, "Str: [{{cyan}}%d/%d{{/}}]  Int: [{{cyan}}%d{{/}}]  Wis: [{{cyan}}%d{{/}}]  "+
		"Dex: [{{cyan}}%d{{/}}]  Con: [{{cyan}}%d{{/}}]  Cha: [{{cyan}}%d{{/}}]\r\n",
		a.Strength, a.StrengthPercentile, a.Intelligence, a.Wisdom,
		a.Dexterity, a.Constitution, a.Charisma)

	p := rec.Points
	regen := statRegen{who: k, room: c.World.Room(k.Room)}
	now := time.Now()
	fmt.Fprintf(&b, "Hit p.:[{{green}}%d/%d+%d{{/}}]  Mana p.:[{{green}}%d/%d+%d{{/}}]  "+
		"Move p.:[{{green}}%d/%d+%d{{/}}]\r\n",
		p.Hit, p.MaxHit, game.HitGain(rec, regen, now),
		p.Mana, p.MaxMana, game.ManaGain(rec, regen, now),
		p.Move, p.MaxMove, game.MoveGain(rec, regen, now))

	fmt.Fprintf(&b, "Coins: [%9d], Bank: [%9d] (Total: %d)\r\n",
		p.Gold, p.BankGold, p.Gold+p.BankGold)

	fmt.Fprintf(&b, "AC: [%d%+d/10], Hitroll: [%2d], Damroll: [%2d], Saving throws: [%d/%d/%d/%d/%d]\r\n",
		p.Armor, game.Dexterity(a.Dexterity).Defensive, p.HitRoll, p.DamRoll,
		rec.SavingThrows[0], rec.SavingThrows[1], rec.SavingThrows[2],
		rec.SavingThrows[3], rec.SavingThrows[4])

	fmt.Fprintf(&b, "Pos: %s, Fighting: %s",
		game.SprintType(int32(k.Position), game.PositionNames()), nameOrNobody(k.Fighting))
	if k.Client != nil {
		b.WriteString(", Connected: Playing")
	}
	b.WriteString("\r\n")

	fmt.Fprintf(&b, "Default position: %s, Idle Timer (in tics) [%d]\r\n",
		game.SprintType(defaultPositionOf(k), game.PositionNames()), 0)

	if k.IsNPC() {
		fmt.Fprintf(&b, "NPC flags: {{cyan}}%s{{/}}\r\n",
			game.SprintBit(mobFlagsOf(k).Raw(), game.ActionBitNames()))
	} else {
		fmt.Fprintf(&b, "PLR: {{cyan}}%s{{/}}\r\n",
			game.SprintBit(rec.PlayerFlags.Raw(), game.PlayerBitNames()))
		fmt.Fprintf(&b, "PRF: {{green}}%s{{/}}\r\n",
			game.SprintBit(rec.Preferences.Raw(), game.PreferenceBitNames()))
	}

	if k.IsNPC() && k.MobDef != nil {
		fmt.Fprintf(&b, "Mob Spec-Proc: %s, NPC Bare Hand Dam: %dd%d\r\n",
			existsOrNone(k.MobDef.Spec != ""),
			k.MobDef.DamageDice.Number, k.MobDef.DamageDice.Size)
	}

	worn := 0
	for _, obj := range k.Equipment {
		if obj != nil {
			worn++
		}
	}
	fmt.Fprintf(&b, "Carried: weight: %d, items: %d; Items in: inventory: %d, eq: %d\r\n",
		k.CarriedWeight(), len(k.Carrying), len(k.Carrying), worn)

	if !k.IsNPC() {
		fmt.Fprintf(&b, "Hunger: %d, Thirst: %d, Drunk: %d\r\n",
			rec.Conditions[game.CondFull], rec.Conditions[game.CondThirst],
			rec.Conditions[game.CondDrunk])
	}

	fmt.Fprintf(&b, "Master is: %s, Followers are:", nameOrAngleNone(k.Master))
	names := make([]string, 0, len(k.Followers))
	for _, follower := range k.Followers {
		names = append(names, follower.Name)
	}
	b.WriteString(joinWrapped(names))

	fmt.Fprintf(&b, "AFF: {{yellow}}%s{{/}}\r\n",
		game.SprintBit(rec.AffectFlags.Raw(), game.AffectBitNames()))

	for _, aff := range rec.Affects {
		line := fmt.Sprintf("SPL: (%3dhr) {{cyan}}%-21s{{/}} ",
			aff.Duration+1, game.SpellName(aff.Type))
		modifier := ""
		if aff.Modifier != 0 {
			modifier = fmt.Sprintf("%+d to %s", aff.Modifier,
				game.SprintType(aff.Location, game.ApplyTypeNames()))
			line += modifier
		}
		if !aff.Bits.Empty() {
			if modifier != "" {
				line += ", sets "
			} else {
				line += "sets "
			}
			line += game.SprintBit(aff.Bits.Raw(), game.AffectBitNames())
		}
		b.WriteString(line + "\r\n")
	}

	c.Send("%s", b.String())
}

// statRegen satisfies game.Regenerator so `stat` can print the per-tick gains
// the way hit_gain and friends compute them. The server's tick has its own
// copy of this; it is four one-line methods and not worth a shared package.
type statRegen struct {
	who  *game.Character
	room *game.RoomDef
}

func (r statRegen) IsNPC() bool             { return r.who.IsNPC() }
func (r statRegen) Position() game.Position { return r.who.Position }

func (r statRegen) Poisoned() bool {
	return r.who.Record != nil && r.who.Record.AffectFlags.Has(game.AffectPoison)
}

func (r statRegen) GoodRegen() bool {
	return r.room != nil && r.room.Flags.Has(game.RoomGoodRegen)
}

// mobFlagsOf is MOB_FLAGS: a mobile's action bits live on its prototype.
func mobFlagsOf(k *game.Character) game.MobFlags {
	if k.MobDef == nil {
		return game.MobFlags{}
	}
	return k.MobDef.ActionFlags
}

// --- the small formatting helpers -------------------------------------

// joinWrapped is the C's `if (strlen(buf) >= 62)` line-wrapping, which every
// list in do_stat does by hand and all of them do slightly differently.
//
// The shared shape: names separated by ", ", broken when the line reaches 62
// characters, and always ending in a newline — including the empty case,
// which prints a bare "Chars present:\r\n".
func joinWrapped(names []string) string {
	if len(names) == 0 {
		return "\r\n"
	}
	var b strings.Builder
	line := ""
	for i, name := range names {
		piece := " " + name
		if i > 0 {
			piece = ", " + name
		}
		if len(line)+len(piece) >= 62 {
			b.WriteString(line + ",\r\n")
			line = " " + name
			continue
		}
		line += piece
	}
	b.WriteString(line + "\r\n")
	return b.String()
}

// kindOf is the C's `(!IS_NPC(k) ? "PC" : (!IS_MOB(k) ? "NPC" : "MOB"))`.
//
// IS_MOB is IS_NPC *and* having a prototype, so "NPC" is reserved for a
// mobile with no prototype — which nothing in this tree creates. The branch
// is kept because the difference is real if anything ever does.
func kindOf(k *game.Character) string {
	switch {
	case !k.IsNPC():
		return "PC"
	case k.MobDef == nil:
		return "NPC"
	}
	return "MOB"
}

func statDate(t time.Time) string {
	if t.IsZero() {
		return "None"
	}
	stamp := t.Format("Mon Jan _2 15:04:05 2006")
	if len(stamp) > 10 {
		stamp = stamp[:10]
	}
	return stamp
}

func existsOrNone(exists bool) string {
	if exists {
		return "Exists"
	}
	return "None"
}

func yesNo(b bool) string {
	if b {
		return "Yes"
	}
	return "No"
}

func orNone(s string) string {
	if s == "" {
		return "None"
	}
	return s
}

func orAngleNone(s string) string {
	if s == "" {
		return "<None>"
	}
	return s
}

func longDescOf(k *game.Character) string {
	if k.MobDef != nil && k.MobDef.LongDesc != "" {
		return ensureNewline(k.MobDef.LongDesc)
	}
	return "<None>\r\n"
}

func nameOrNone(obj *game.Object) string {
	if obj == nil {
		return "None"
	}
	return obj.Name()
}

func nameOrNobody(k *game.Character) string {
	if k == nil {
		return "Nobody"
	}
	return k.Name
}

func nameOrAngleNone(k *game.Character) string {
	if k == nil {
		return "<none>"
	}
	return k.Name
}

// holderName answers "Carried by" and "Worn by", which are two different
// questions about the same pointer in the C.
func holderName(obj *game.Object, want game.Location) string {
	if obj.Holder != nil && obj.Location == want {
		return obj.Holder.Name
	}
	return "Nobody"
}

func defaultPositionOf(k *game.Character) int32 {
	if k.MobDef != nil {
		return k.MobDef.DefaultPosition
	}
	return int32(game.PosStanding)
}
