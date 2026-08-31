// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package session

import (
	"fmt"
	"strings"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// `look`, ported from act.informative.c.
//
// do_look is a dispatcher, not a command: four different functions answer to
// it depending on what follows the word, and this port had collapsed all four
// into one. `look in <container>` in particular reached look_at_target, which
// describes a thing, rather than look_in_obj, which opens it — so a corpse
// answered "You see nothing special about the corpse of a fido." and there
// was no way to find out what was in it.
//
// The session-parity suite is what has an opinion about all of this now
// (test/parity, objects.session and combat.session): the C server is the
// expectation, and the shapes below are what it actually printed.

// findObjectAndWhere is generic_find restricted to FIND_OBJ_EQUIP |
// FIND_OBJ_INV | FIND_OBJ_ROOM, which is what look_in_obj and look_at_target
// ask for. See genericFind, which is the whole of it.
func (c *Context) findObjectAndWhere(arg string) (*game.Object, findWhere) {
	_, obj, where := c.genericFind(arg, findObjEquip|findObjInv|findObjRoom)
	return obj, where
}

// lookInObject is look_in_obj (act.informative.c:500): `look in <thing>`.
//
// Three kinds of thing answer to it — a container, a drink container and a
// fountain — and everything else gets "There's nothing inside that!" whether
// or not it looks like it should have an inside.
func (c *Context) lookInObject(arg string) error {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		c.Send("Look in what?\r\n")
		return nil
	}

	obj, where := c.findObjectAndWhere(arg)
	if obj == nil {
		// AN() picks the article off the first letter alone (utils.h:133),
		// so it says "an hour" wrong and "a onion" wrong, and it says the
		// player's own typo back at them: `look in nothing` is "There
		// doesn't seem to be a nothing here."
		c.Send("There doesn't seem to be %s %s here.\r\n", article(arg), arg)
		return nil
	}

	switch obj.Type {
	case game.ItemContainer:
		if obj.ContainerClosed() {
			c.Send("It is closed.\r\n")
			return nil
		}
		// fname, not Name(): the *first keyword*, not the short description,
		// so a "leather bag" listed as "a small leather bag" heads its
		// contents with "bag". The trailing space before the newline is the
		// C's own (act.informative.c:519).
		c.Send("%s", fname(obj.Keywords))
		switch where {
		case foundInInventory:
			c.Send(" (carried): \r\n")
		case foundInRoom:
			c.Send(" (here): \r\n")
		case foundInEquipment:
			c.Send(" (used): \r\n")
		}
		c.listObjects(obj.Contents)
	case game.ItemDrinkCon, game.ItemFountain:
		c.showLiquid(obj)
	default:
		c.Send("There's nothing inside that!\r\n")
	}
	return nil
}

// showLiquid is look_in_obj's drink-container branch.
//
// The middle case is marked `/* BUG */` in the C itself, and it is left
// alone: a container whose contents exceed its capacity, or whose capacity is
// zero, reports "Its contents seem somewhat murky." rather than dividing by
// it. Reproduced because the guard is what stops the division, and because
// "murky" is what a player of the real game saw.
func (c *Context) showLiquid(obj *game.Object) {
	contents, _ := obj.DrinkValues()
	capacity, filled, liquid := contents.Capacity, contents.Filled, contents.Liquid
	switch {
	case filled <= 0:
		c.Send("It is empty.\r\n")
	case capacity <= 0 || filled > capacity:
		c.Send("Its contents seem somewhat murky.\r\n")
	default:
		c.Send("It's %sfull of a %s liquid.\r\n",
			game.Fullness(filled*3/capacity), game.LiquidColour(liquid))
	}
}

// listObjects is list_obj_to_char with SHOW_OBJ_SHORT and show set
// (act.informative.c:129), which is what every container's contents go
// through.
//
// The leading space on " Nothing." is the C's, and so is the fact that it
// appears at all only when `show` is true — a room's floor listing passes
// false and stays silent when there is nothing on it.
func (c *Context) listObjects(list []*game.Object) {
	var found bool
	for _, obj := range list {
		if !c.World.CanSeeObj(c.Character, obj) {
			continue
		}
		c.Send("%s%s\r\n", obj.Name(), objectModifiers(c.Character, obj))
		found = true
	}
	if !found {
		c.Send(" Nothing.\r\n")
	}
}

// objectModifiers is show_obj_modifiers (act.informative.c:104): the little
// tags an object trails when it is invisible, blessed, magical, glowing or
// humming.
//
// Two of the five are conditional on the *viewer* rather than the object —
// blue for blessed and yellow for magical need detect alignment and detect
// magic up — which is why this takes the character as well as the object.
func objectModifiers(viewer *game.Character, obj *game.Object) string {
	var b strings.Builder
	if obj.ExtraFlags.Has(game.ItemInvisible) {
		b.WriteString(" (invisible)")
	}
	if obj.ExtraFlags.Has(game.ItemBless) && viewer.HasAffect(game.AffectDetectAlign) {
		b.WriteString(" ..It glows blue!")
	}
	if obj.ExtraFlags.Has(game.ItemMagic) && viewer.HasAffect(game.AffectDetectMagic) {
		b.WriteString(" ..It glows yellow!")
	}
	if obj.ExtraFlags.Has(game.ItemGlow) {
		b.WriteString(" ..It has a soft glowing aura!")
	}
	if obj.ExtraFlags.Has(game.ItemHum) {
		b.WriteString(" ..It emits a faint humming sound!")
	}
	return b.String()
}

// lookInDirection is look_in_direction (act.informative.c:534).
//
// The door lines are an if/else rather than two ifs, so a closed door says
// only "The gate is closed." and never also that it is a door.
func (c *Context) lookInDirection(dir game.Direction) error {
	room := c.World.Room(c.Character.Room)
	if room == nil || room.Exits[dir] == nil {
		c.Send("Nothing special there...\r\n")
		return nil
	}
	exit := room.Exits[dir]
	if exit.Description != "" {
		c.Send("%s", ensureNewline(exit.Description))
	} else {
		c.Send("You see nothing special.\r\n")
	}

	if exit.Keywords == "" {
		return nil
	}
	switch {
	case exit.State.Has(game.ExitClosed):
		c.Send("The %s is closed.\r\n", fname(exit.Keywords))
	case exit.State.Has(game.ExitIsDoor):
		c.Send("The %s is open.\r\n", fname(exit.Keywords))
	}
	return nil
}

// fname is handler.c:698: the leading run of letters in a keyword list.
//
// It stops at the first non-alphabetic character rather than at whitespace,
// which is not the same thing — a keyword list beginning "bag2 leather"
// yields "bag", not "bag2". Nothing in the shipped world exercises the
// difference; it is reproduced because the C's loop is written that way and
// a world file is free to.
func fname(namelist string) string {
	end := 0
	for end < len(namelist) && isLetter(namelist[end]) {
		end++
	}
	return namelist[:end]
}

func isLetter(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

// Everything below this line moved here from commands.go in step 9 of
// docs/design/idiomatic-go.md — `look` and the room description it
// builds, which had been sitting in the dispatcher's file since the first
// commands were written. Code motion only: not a line changed.

func doLook(c *Context) error {
	// do_look has a gate of its own (act.informative.c:662), and it is *not*
	// look_at_room's: different words, and the two tests the other way round.
	// Typing `look` while blind says "you're blind"; walking into a dark room
	// while blind says it is pitch black, because there the darkness is asked
	// first. Both are reachable and they disagree on purpose — or at least
	// they disagree, and there is no sign anybody meant them to.
	//
	// The C's first branch, `GET_POS(ch) < POS_SLEEPING` → "You can't see
	// anything but stars!", is **unreachable**: `look` and `read` are both
	// POS_RESTING in the command table (interpreter.c:355, :427), so the
	// interpreter has already refused anything below that. Not ported, and
	// recorded in docs/weirdnumbers.md with the other four of its kind.
	if isBlind(c.Character) {
		c.Send("You can't see a damned thing, you're blind!\r\n")
		return nil
	}
	if c.World.RoomIsDark(c.Character.Room) && !game.CanSeeInDark(c.Character) {
		c.Send("It is pitch black...\r\n")
		// And the one thing you *can* see in the dark. The C's comment on this
		// line is just "glowing red eyes", which is the only clue that
		// list_char_to_char has a second branch at all.
		if room := c.World.Room(c.Character.Room); room != nil {
			c.Send("%s", listCharToChar(c.World, room, c.Character))
		}
		return nil
	}

	// From here do_look is a dispatcher rather than a command: four different
	// functions answer to it (act.informative.c:679-690), and the order they
	// are tried in is what decides what an ambiguous word means.
	arg, rest := halfChop(c.Arg)

	switch {
	case arg == "":
		// `look` typed on purpose ignores brief mode, which is what the C's
		// ignore_brief argument is for. Everywhere else in the whole tree
		// passes 0: act.informative.c:680 is the only caller that passes 1.
		return lookAtRoom(c, true)

	case isPrefixOf(arg, "in"):
		// Before the direction check, so `look i` is "look in what?" rather
		// than a direction — nothing abbreviates to a direction from "i", but
		// the ordering is the C's and is what makes that answerable.
		return c.lookInObject(rest)

	case direction(arg):
		// `look north`. Ahead of `at`, so a direction beats an extra
		// description of the same name.
		dir, _ := game.ParseDirection(arg)
		return c.lookInDirection(dir)

	case isPrefixOf(arg, "at"):
		return c.lookAtTarget(rest)

	default:
		// `look sword`, with no preposition at all.
		return c.lookAtTarget(arg)
	}
}

// direction reports whether a word names one, which do_look asks with
// search_block(arg, dirs, FALSE) — abbreviations allowed.
func direction(word string) bool {
	_, ok := game.ParseDirection(word)
	return ok
}

// lookAtRoom shows the character the room they are in.
func lookAtRoom(c *Context, ignoreBrief bool) error {
	room := c.World.Room(c.Character.Room)
	if room == nil {
		c.Send("You are nowhere at all. That should not be possible.\r\n")
		return nil
	}

	sendRoomInfo(c.Session, room)
	c.Send("%s", roomDescription(c.World, room, c.Character, ignoreBrief))
	return nil
}

// roomDescription is look_at_room (act.informative.c:413): the name, the
// description, the way out, what is lying about and who is here.
//
// It takes the viewer so it can leave them out of the list of people, consult
// their preferences and work out whether they can see at all, and it returns a
// string rather than sending, because a spell can move somebody else into a
// room and has to show it to them rather than to the caster.
//
// ignoreBrief is the C's argument of the same name: `look` typed by hand shows
// the description whatever PRF_BRIEF says, and the automatic look on arriving
// somewhere does not.
func roomDescription(w *game.Live, room *game.RoomDef, viewer *game.Character, ignoreBrief bool) string {
	// Darkness first, and blindness after it — the C's order, which decides
	// which message a blind character standing in the dark gets. They are told
	// it is pitch black, not that they are blind.
	//
	// Note this asks CAN_SEE_IN_DARK, so holylight counts directly here. It is
	// a different question from LIGHT_OK's, which takes infravision alone.
	if w.RoomIsDark(room.Vnum) && !game.CanSeeInDark(viewer) {
		return "It is pitch black...\r\n"
	}
	if isBlind(viewer) {
		return "You see nothing but infinite darkness...\r\n"
	}

	var b strings.Builder

	// An immortal with `roomflags` on gets the vnum and the flags in the
	// title line.
	// Cyan, as look_at_room does (act.informative.c:425). The colour markup
	// is resolved at the socket against the reader's preference — see
	// internal/colour — so this string is written once for everybody.
	if hasPref(viewer, game.PrefRoomFlags) {
		fmt.Fprintf(&b, "{{cyan}}[%5d] %s [ %s]{{/}}\r\n",
			room.Vnum, room.Name, game.SprintBit(room.Flags.Raw(), game.RoomBitNames()))
	} else {
		fmt.Fprintf(&b, "{{cyan}}%s{{/}}\r\n", room.Name)
	}

	// Brief mode drops the description — but never in a DEATH room, because
	// that description is the only warning you get.
	if room.Description != "" &&
		(ignoreBrief || !hasPref(viewer, game.PrefBrief) || room.Flags.Has(game.RoomDeathTrap)) {
		b.WriteString(ensureNewline(room.Description))
	}

	if hasPref(viewer, game.PrefAutoExit) {
		fmt.Fprintf(&b, "{{cyan}}[ Exits: %s]{{/}}\r\n", autoExits(room))
	}

	// The two local additions, both `<DoC>` (act.informative.c:444, :452),
	// and both coloured at C_NRM. The player-killer line changes colour
	// three times in one sentence — yellow, red for the bracketed words,
	// yellow again — because the C sends it as seven separate writes.
	if room.Flags.Has(game.RoomGoodRegen) {
		b.WriteString("{{blue}}You feel a soft, warm feeling in your bones.{{/}}\r\n")
	}
	if room.Flags.Has(game.RoomPKill) {
		b.WriteString("{{yellow}}You have entered a {{/}}{{red}}[Player Killer]{{/}}" +
			"{{yellow}} room. Beware!{{/}}\r\n")
	}

	// Green for what is lying about and yellow for who is here, which is the
	// C switching colour around each list rather than colouring the lines
	// themselves (act.informative.c:469-473). The reset goes after the whole
	// list, not after each line.
	// list_obj_to_char (act.informative.c:165). An object you cannot see is
	// simply not there, with no marker: the C's `show` argument produces
	// " Nothing." for an empty *inventory*, never for an empty floor.
	var objects strings.Builder
	for _, obj := range w.RoomObjects(room.Vnum) {
		if !w.CanSeeObj(viewer, obj) {
			continue
		}
		if obj.Description != "" {
			fmt.Fprintf(&objects, "%s\r\n", obj.Description)
			continue
		}
		fmt.Fprintf(&objects, "%s is lying here.\r\n", capitaliseFirst(obj.Name()))
	}
	// Unconditionally, and with one reset for both lists rather than one each.
	// The C sends the colour codes as bare writes around the two calls
	// (act.informative.c:469-473):
	//
	//	send_to_char(CCGRN(ch, C_NRM), ch);
	//	list_obj_to_char(...);
	//	send_to_char(CCYEL(ch, C_NRM), ch);
	//	list_char_to_char(...);
	//	send_to_char(CCNRM(ch, C_NRM), ch);
	//
	// So an empty room still gets a green, a yellow and a reset with nothing
	// between them — visible in a transcript, invisible on a terminal, and
	// reproduced because the session-parity harness compares transcripts.
	fmt.Fprintf(&b, "{{green}}%s{{yellow}}%s{{/}}",
		objects.String(), listCharToChar(w, room, viewer))
	return b.String()
}

// listCharToChar is list_char_to_char (act.informative.c:343).
//
// The `else if` is the interesting half and is easy to skip past: somebody you
// *cannot* see is not always silent. If the room is dark, you cannot see in the
// dark, and **they** have infravision, you get a pair of glowing red eyes
// instead of nothing. Note whose infravision it is — theirs, not yours — so it
// is the creature's own night vision that gives it away.
func listCharToChar(w *game.Live, room *game.RoomDef, viewer *game.Character) string {
	var b strings.Builder
	dark := w.RoomIsDark(room.Vnum) && !game.CanSeeInDark(viewer)

	for _, other := range w.Occupants(room.Vnum) {
		if other == viewer {
			continue
		}
		if w.CanSee(viewer, other) {
			b.WriteString(listOneChar(w, other, viewer))
			continue
		}
		if dark && other.HasAffect(game.AffectInfravision) {
			b.WriteString("You see a pair of glowing red eyes looking your way.\r\n")
		}
	}
	return b.String()
}

// charPositions are list_one_char's positions[] (act.informative.c:261),
// indexed by position. POS_FIGHTING's slot is never used — the code branches
// before reaching it — and the C's placeholder is left here for the same
// reason it is there: so the indices line up with the position constants.
var charPositions = [...]string{
	" is lying here, dead.",
	" is lying here, mortally wounded.",
	" is lying here, incapacitated.",
	" is lying here, stunned.",
	" is sleeping here.",
	" is resting here.",
	" is sitting here.",
	"!FIGHTING!",
	" is standing here.",
}

// listOneChar is list_one_char (act.informative.c:259): one line describing
// somebody the viewer can see.
//
// Two shapes, and which one you get is not about being a mobile. A mobile
// standing in its *default* position uses the long description the builder
// wrote for it; the same mobile sitting down, or fighting, or dead, falls
// through to the constructed line — which is why a corpse-to-be says "the
// cityguard is lying here, mortally wounded" rather than the long description
// that has it standing at attention.
func listOneChar(w *game.Live, who, viewer *game.Character) string {
	if who.MobDef != nil && who.MobDef.LongDesc != "" &&
		who.Position == who.MobDef.Position {
		var b strings.Builder
		// A `*` in front of a long description means invisible. You only ever
		// see it with detect invisible on, since otherwise the mobile is not
		// listed at all.
		if who.HasAffect(game.AffectInvisible) {
			b.WriteString("*")
		}
		b.WriteString(auraPrefix(who, viewer))
		b.WriteString(ensureNewline(who.MobDef.LongDesc))
		b.WriteString(glowLines(w, who, viewer))
		return b.String()
	}

	var b strings.Builder
	if who.IsNPC() {
		b.WriteString(capitaliseFirst(who.Name))
	} else {
		fmt.Fprintf(&b, "%s %s", who.Name, title(who))
	}

	if who.HasAffect(game.AffectInvisible) {
		b.WriteString(" (invisible)")
	}
	if who.HasAffect(game.AffectHide) {
		b.WriteString(" (hidden)")
	}
	if !who.IsNPC() && who.Client == nil {
		b.WriteString(" (linkless)")
	}
	if !who.IsNPC() && who.Record != nil && who.Record.PlayerFlags.Has(game.PlayerWriting) {
		b.WriteString(" (writing)")
	}

	if who.Position != game.PosFighting {
		b.WriteString(charPositions[who.Position])
	} else if who.Fighting == nil {
		// The C's comment is "NIL fighting pointer", and it happens: a
		// position of FIGHTING outlives the opponent by however long it takes
		// something to clear it.
		b.WriteString(" is here struggling with thin air.")
	} else {
		b.WriteString(" is here, fighting ")
		switch {
		case who.Fighting == viewer:
			b.WriteString("YOU!")
		case who.Fighting.Room != who.Room:
			b.WriteString("someone who has already left!")
		default:
			// PERS, so an invisible opponent is "someone" even here.
			fmt.Fprintf(&b, "%s!", w.Pers(who.Fighting, viewer))
		}
	}

	b.WriteString(auraSuffix(who, viewer))
	b.WriteString("\r\n")
	b.WriteString(glowLines(w, who, viewer))
	return b.String()
}

// title is the player's title, which follows their name in the room list. A
// mobile has none.
func title(who *game.Character) string {
	if who.Record == nil {
		return ""
	}
	return who.Record.Title
}

// auraPrefix and auraSuffix are the same two tests written twice in the C, in
// the two branches of list_one_char, and they differ: the long-description
// branch puts "(Red Aura) " *before* with a trailing space, the constructed one
// puts " (Red Aura)" *after*. Reproduced rather than unified, because the
// difference is visible.
func auraPrefix(who, viewer *game.Character) string {
	switch {
	case !viewer.HasAffect(game.AffectDetectAlign) || who.Record == nil:
		return ""
	case game.IsEvil(who.Record):
		return "(Red Aura) "
	case game.IsGood(who.Record):
		return "(Blue Aura) "
	}
	return ""
}

func auraSuffix(who, viewer *game.Character) string {
	switch {
	case !viewer.HasAffect(game.AffectDetectAlign) || who.Record == nil:
		return ""
	case game.IsEvil(who.Record):
		return " (Red Aura)"
	case game.IsGood(who.Record):
		return " (Blue Aura)"
	}
	return ""
}

// glowLines are the act() messages list_one_char sends after the line itself.
// Both branches send the sanctuary one; only the long-description branch sends
// the blindness one, which is the C's and looks like an oversight rather than
// a decision — a blind *player* is never reported as groping around.
func glowLines(w *game.Live, who, viewer *game.Character) string {
	var b strings.Builder
	if who.HasAffect(game.AffectSanctuary) {
		b.WriteString(w.Act("...$e glows with a bright light!", game.ActArgs{Actor: who}, viewer))
	}
	if who.MobDef != nil && who.MobDef.LongDesc != "" && who.HasAffect(game.AffectBlind) {
		b.WriteString(w.Act("...$e is groping around blindly!", game.ActArgs{Actor: who}, viewer))
	}
	return b.String()
}

// isBlind reports whether AFF_BLIND is set.
func isBlind(ch *game.Character) bool {
	return ch != nil && ch.Record != nil && ch.Record.AffectFlags.Has(game.AffectBlind)
}

// hasPref reports whether a player has a PRF_ bit set. A mobile has none: the
// C guards every one of these with !IS_NPC, because player_specials is not
// allocated for a mobile and reading it would be a null dereference.
func hasPref(ch *game.Character, flag game.PrefFlag) bool {
	return ch != nil && !ch.IsNPC() && ch.Record != nil && ch.Record.Preferences.Has(flag)
}

// autoExits is do_auto_exits' list (act.informative.c:358).
//
// Two details that are easy to miss and both player-visible: a **closed** exit
// is not listed, so a shut door hides the way it leads; and a room with no way
// out at all says "None! " rather than nothing. Each letter is written with a
// trailing space, which is where the space before the closing bracket comes
// from.
func autoExits(room *game.RoomDef) string {
	var b strings.Builder
	for dir, e := range room.Exits {
		if e == nil || e.ToRoom == game.NoRoom || e.State.Has(game.ExitClosed) {
			continue
		}
		fmt.Fprintf(&b, "%c ", game.Direction(dir).String()[0])
	}
	if b.Len() == 0 {
		return "None! "
	}
	return b.String()
}

// exitNames lists the room's exits, truncated to width characters each.
//
// This feeds GMCP rather than the `[ Exits: ]` line, and unlike that line it
// reports closed exits too: a client drawing a map wants to know the door is
// there.
func exitNames(room *game.RoomDef, width int) []string {
	var out []string
	for dir, e := range room.Exits {
		if e == nil || e.ToRoom == game.NoRoom {
			continue
		}
		name := game.Direction(dir).String()
		if width > 0 && width < len(name) {
			name = name[:width]
		}
		out = append(out, name)
	}
	return out
}

// sendRoomInfo publishes the room out of band, so a web client can draw a map
// instead of parsing the description.
func sendRoomInfo(s *Session, room *game.RoomDef) {
	// A character with no connection has no GMCP to send. Until #375 there
	// was no such caller — lookAtRoom was only ever reached from a command,
	// and a command has a session — but damage() flees a MOB_WIMPY mobile
	// that is badly hurt, and do_flee looks at the room it lands in. A
	// mobile has no session at all, so this would have been a nil
	// dereference on the world goroutine.
	if s == nil {
		return
	}
	s.SendGMCP("Room.Info", RoomInfo{
		Vnum:  int32(room.Vnum),
		Name:  room.Name,
		Desc:  room.Description,
		Exits: exitNames(room, 0),
	})
}
