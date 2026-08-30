// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package session

import (
	"strconv"
	"strings"
	"time"

	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/obs"
)

// do_set and perform_set, ported from act.wizard.c:2773 and :2426.
//
// Fifty-two fields behind one command, each with its own level, its own idea
// of whether it applies to players or mobiles or both, and its own range. The
// C keeps them in a table and a switch on the index; this keeps them in a
// table with the handler attached, so the two cannot drift apart — and
// wizset_test.go re-parses the C's table and checks the names, levels and
// kinds line up.

// setWho is the C's PC/NPC/BOTH.
type setWho int

const (
	setPC   setWho = 1
	setNPC  setWho = 2
	setBoth setWho = setPC | setNPC
)

// setKind is the C's MISC/BINARY/NUMBER: how the value argument is read, and
// what the acknowledgement says.
type setKind int

const (
	setMisc setKind = iota
	setBinary
	setNumber
)

// setContext is what a field's handler is given.
type setContext struct {
	c      *Context
	victim *game.Character
	rec    *game.PlayerRecord
	// arg is the raw value; on and off are set for a BINARY field; value is
	// the parsed number for a NUMBER one.
	arg   string
	on    bool
	off   bool
	value int32
	// output is the acknowledgement, pre-filled by kind and overwritten by
	// the handlers that say something else.
	output string
	// failed stops the acknowledgement being sent, for a handler that has
	// already said what went wrong.
	failed bool
	// fieldName is the name being set, for the messages that mention it.
	fieldName string
}

// setOrRemove is the C's SET_OR_REMOVE macro: set on `on`, clear on `off`,
// and — worth noticing — do nothing at all if neither, which cannot happen
// for a BINARY field because the parse refuses anything else.
// A free function rather than a method on setContext because it serves two
// flag domains with two different set types, and Go has no generic methods.
// docs/design/idiomatic-go.md §4.1.
func setOrRemove[T ~int](s *setContext, flags *game.Set[T], bits ...T) {
	switch {
	case s.on:
		*flags = flags.With(bits...)
	case s.off:
		*flags = flags.Without(bits...)
	}
}

// rangeOf is the C's RANGE macro, which *assigns back* to `value` as well as
// returning it — several fields rely on that.
func (s *setContext) rangeOf(low, high int32) int32 {
	s.value = max(low, min(high, s.value))
	return s.value
}

// refuse says why and stops the acknowledgement.
func (s *setContext) refuse(format string, args ...any) {
	s.c.Send(format, args...)
	s.failed = true
}

// abilityRange is the ceiling on a rolled statistic: 18 for a mortal, 25 for
// a mobile or a greater god. The same three lines appear six times in the C.
func (s *setContext) abilityRange() int32 {
	if s.victim.IsNPC() || s.rec.Level >= game.LevelGreaterGod {
		return s.rangeOf(3, 25)
	}
	return s.rangeOf(3, 18)
}

type setField struct {
	name  string
	level int32
	who   setWho
	kind  setKind
	apply func(s *setContext)
}

// setFields is set_fields[] (act.wizard.c:2364), in the C's order — which is
// the order `set` matches abbreviations in, so it is the order that decides
// what `set <name> h` means.
var setFields = []setField{
	{"brief", game.LevelGod, setPC, setBinary, func(s *setContext) {
		setOrRemove(s, &s.rec.Preferences, game.PrefBrief)
	}},
	{"invstart", game.LevelGod, setPC, setBinary, func(s *setContext) {
		setOrRemove(s, &s.rec.PlayerFlags, game.PlayerInvisStart)
	}},
	{"title", game.LevelGod, setPC, setMisc, func(s *setContext) {
		s.rec.Title = s.arg
		s.output = s.victim.Name + "'s title is now: " + s.rec.Title
	}},
	{"nosummon", game.LevelGreaterGod, setPC, setBinary, func(s *setContext) {
		// The flag is SUMMONABLE and the field is NOSUMMON, so the sense is
		// inverted — and the C's acknowledgement says ONOFF(!on) to match.
		setOrRemove(s, &s.rec.Preferences, game.PrefSummonable)
		s.output = "Nosummon " + onOff(!s.on) + " for " + s.victim.Name + "."
	}},
	{"maxhit", game.LevelGreaterGod, setBoth, setNumber, func(s *setContext) {
		s.rec.Points.MaxHit = s.rangeOf(1, 5000)
	}},
	{"maxmana", game.LevelGreaterGod, setBoth, setNumber, func(s *setContext) {
		s.rec.Points.MaxMana = s.rangeOf(1, 5000)
	}},
	{"maxmove", game.LevelGreaterGod, setBoth, setNumber, func(s *setContext) {
		s.rec.Points.MaxMove = s.rangeOf(1, 5000)
	}},
	{"hit", game.LevelGreaterGod, setBoth, setNumber, func(s *setContext) {
		// The floor is -9, not 0: a god may set somebody *dying*.
		s.rec.Points.Hit = s.rangeOf(-9, s.rec.Points.MaxHit)
	}},
	{"mana", game.LevelGreaterGod, setBoth, setNumber, func(s *setContext) {
		s.rec.Points.Mana = s.rangeOf(0, s.rec.Points.MaxMana)
	}},
	{"move", game.LevelGreaterGod, setBoth, setNumber, func(s *setContext) {
		s.rec.Points.Move = s.rangeOf(0, s.rec.Points.MaxMove)
	}},
	{"align", game.LevelGod, setBoth, setNumber, func(s *setContext) {
		s.rec.Alignment = s.rangeOf(-1000, 1000)
	}},
	{"str", game.LevelGreaterGod, setBoth, setNumber, func(s *setContext) {
		s.rec.RealAbilities.Strength = s.abilityRange()
		// Setting strength clears the percentile, so `set x str 18` is 18/00
		// and not 18/00 plus whatever was there.
		s.rec.RealAbilities.StrengthPercentile = 0
	}},
	{"stradd", game.LevelGreaterGod, setBoth, setNumber, func(s *setContext) {
		s.rec.RealAbilities.StrengthPercentile = s.rangeOf(0, 100)
		if s.value > 0 {
			s.rec.RealAbilities.Strength = 18
		}
	}},
	{"int", game.LevelGreaterGod, setBoth, setNumber, func(s *setContext) {
		s.rec.RealAbilities.Intelligence = s.abilityRange()
	}},
	{"wis", game.LevelGreaterGod, setBoth, setNumber, func(s *setContext) {
		s.rec.RealAbilities.Wisdom = s.abilityRange()
	}},
	{"dex", game.LevelGreaterGod, setBoth, setNumber, func(s *setContext) {
		s.rec.RealAbilities.Dexterity = s.abilityRange()
	}},
	{"con", game.LevelGreaterGod, setBoth, setNumber, func(s *setContext) {
		s.rec.RealAbilities.Constitution = s.abilityRange()
	}},
	{"cha", game.LevelGreaterGod, setBoth, setNumber, func(s *setContext) {
		s.rec.RealAbilities.Charisma = s.abilityRange()
	}},
	{"ac", game.LevelGreaterGod, setBoth, setNumber, func(s *setContext) {
		s.rec.Points.Armor = s.rangeOf(-100, 100)
	}},
	{"gold", game.LevelGod, setBoth, setNumber, func(s *setContext) {
		s.rec.Points.Gold = s.rangeOf(0, 100000000)
	}},
	{"bank", game.LevelGod, setPC, setNumber, func(s *setContext) {
		s.rec.Points.BankGold = s.rangeOf(0, 100000000)
	}},
	{"exp", game.LevelGreaterGod, setBoth, setNumber, func(s *setContext) {
		s.rec.Points.Exp = s.rangeOf(0, 50000000)
	}},
	{"hitroll", game.LevelGreaterGod, setBoth, setNumber, func(s *setContext) {
		s.rec.Points.HitRoll = s.rangeOf(-20, 20)
	}},
	{"damroll", game.LevelGreaterGod, setBoth, setNumber, func(s *setContext) {
		s.rec.Points.DamRoll = s.rangeOf(-20, 20)
	}},
	{"invis", game.LevelImplementor, setPC, setNumber, func(s *setContext) {
		if !s.selfOrImplementor() {
			return
		}
		s.rec.InvisLevel = s.rangeOf(0, s.rec.Level)
	}},
	{"nohassle", game.LevelGreaterGod, setPC, setBinary, func(s *setContext) {
		if !s.selfOrImplementor() {
			return
		}
		setOrRemove(s, &s.rec.Preferences, game.PrefNoHassle)
	}},
	{"frozen", game.LevelGreaterGod, setPC, setBinary, func(s *setContext) {
		if s.victim == s.c.Character && s.on {
			s.refuse("Better not -- could be a long winter!\r\n")
			return
		}
		setOrRemove(s, &s.rec.PlayerFlags, game.PlayerFrozen)
	}},
	// `practices` and `lessons` are the same field under two names: the C
	// falls through from one case to the other.
	{"practices", game.LevelGreaterGod, setPC, setNumber, setPractices},
	{"lessons", game.LevelGreaterGod, setPC, setNumber, setPractices},
	{"drunk", game.LevelGreaterGod, setBoth, setMisc, setCondition(game.CondDrunk)},
	{"hunger", game.LevelGreaterGod, setBoth, setMisc, setCondition(game.CondFull)},
	{"thirst", game.LevelGreaterGod, setBoth, setMisc, setCondition(game.CondThirst)},
	{"killer", game.LevelGod, setPC, setBinary, func(s *setContext) {
		setOrRemove(s, &s.rec.PlayerFlags, game.PlayerKiller)
	}},
	{"thief", game.LevelGod, setPC, setBinary, func(s *setContext) {
		setOrRemove(s, &s.rec.PlayerFlags, game.PlayerThief)
	}},
	{"level", game.LevelImplementor, setBoth, setNumber, func(s *setContext) {
		if s.value > levelOf(s.c.Character) || s.value > game.LevelImplementor {
			s.refuse("You can't do that.\r\n")
			return
		}
		s.rec.Level = s.rangeOf(0, game.LevelImplementor)
	}},
	{"room", game.LevelImplementor, setBoth, setNumber, func(s *setContext) {
		room := game.RoomVnum(s.value)
		if s.c.World.Room(room) == nil {
			s.refuse("No room exists with that number.\r\n")
			return
		}
		if err := s.c.World.Enter(s.victim, room); err != nil {
			s.refuse("No room exists with that number.\r\n")
		}
	}},
	{"roomflag", game.LevelGreaterGod, setPC, setBinary, func(s *setContext) {
		setOrRemove(s, &s.rec.Preferences, game.PrefRoomFlags)
	}},
	{"siteok", game.LevelGreaterGod, setPC, setBinary, func(s *setContext) {
		setOrRemove(s, &s.rec.PlayerFlags, game.PlayerSiteOK)
	}},
	{"deleted", game.LevelImplementor, setPC, setBinary, func(s *setContext) {
		setOrRemove(s, &s.rec.PlayerFlags, game.PlayerDeleted)
	}},
	{"class", game.LevelGreaterGod, setBoth, setMisc, func(s *setContext) {
		// parse_class takes a *character*, not a word: `set x class m` is a
		// magic user and `set x class mage` is also a magic user, because
		// only the first letter is read.
		if s.arg == "" {
			s.refuse("That is not a class.\r\n")
			return
		}
		class := game.ParseClass(s.arg[0])
		if class == game.ClassUndefined {
			s.refuse("That is not a class.\r\n")
			return
		}
		s.rec.Class = class
	}},
	{"nowizlist", game.LevelGod, setPC, setBinary, func(s *setContext) {
		setOrRemove(s, &s.rec.PlayerFlags, game.PlayerNoWizList)
	}},
	{"quest", game.LevelGod, setPC, setBinary, func(s *setContext) {
		setOrRemove(s, &s.rec.Preferences, game.PrefQuest)
	}},
	{"loadroom", game.LevelGreaterGod, setPC, setMisc, func(s *setContext) {
		if strings.EqualFold(s.arg, "off") {
			s.rec.PlayerFlags = s.rec.PlayerFlags.Without(game.PlayerLoadRoom)
			return
		}
		if !isNumber(s.arg) {
			s.refuse("Must be 'off' or a room's virtual number.\r\n")
			return
		}
		room := game.RoomVnum(atoi(s.arg))
		if s.c.World.Room(room) == nil {
			s.refuse("That room does not exist!\r\n")
			return
		}
		s.rec.PlayerFlags = s.rec.PlayerFlags.With(game.PlayerLoadRoom)
		s.rec.LoadRoom = room
		s.output = s.victim.Name + " will enter at room #" + strconv.FormatInt(int64(room), 10) + "."
	}},
	{"color", game.LevelGod, setPC, setBinary, func(s *setContext) {
		setOrRemove(s, &s.rec.Preferences, game.PrefColour1, game.PrefColour2)
	}},
	{"idnum", game.LevelImplementor, setPC, setNumber, func(s *setContext) {
		// Character number one only, and only on a *mobile* — which, since
		// the field is marked PC, means never. See docs/weirdnumbers.md.
		if idOf(s.c.Character) != 1 || !s.victim.IsNPC() {
			s.failed = true
			return
		}
		s.rec.IDNum = int64(s.value)
	}},
	{"passwd", game.LevelGreaterGod, setPC, setMisc, func(s *setContext) {
		if s.rec.Level >= game.LevelGreaterGod {
			s.refuse("You cannot change that.\r\n")
			return
		}
		if s.c.SetPassword == nil {
			s.refuse("Passwords cannot be changed here.\r\n")
			return
		}
		if err := s.c.SetPassword(s.victim, s.arg); err != nil {
			s.refuse("That password could not be set.\r\n")
			return
		}
		// mudlog(buf, BRF, MAX(LVL_GOD, GET_INVIS_LEV(ch)), TRUE)
		// (act.wizard.c:2721-2722). The one field in the whole of do_set's
		// table that logs — the C added it to this case alone, and this is
		// obviously the one it would.
		s.c.wizlogInvis(obs.LogBrief, game.LevelGod, s.c.Character,
			"(GC) %s has set password for %s.", s.c.Character.Name, s.victim.Name)
		// The C echoes the new password back in clear. Not reproduced; see
		// docs/deviations.md.
		s.output = "Password changed."
	}},
	{"nodelete", game.LevelGod, setPC, setBinary, func(s *setContext) {
		setOrRemove(s, &s.rec.PlayerFlags, game.PlayerNoDelete)
	}},
	{"sex", game.LevelGreaterGod, setBoth, setMisc, func(s *setContext) {
		sex := -1
		for i, name := range game.GenderNames() {
			if strings.HasPrefix(strings.ToLower(name), strings.ToLower(s.arg)) && s.arg != "" {
				sex = i
				break
			}
		}
		if sex < 0 {
			s.refuse("Must be 'male', 'female', or 'neutral'.\r\n")
			return
		}
		s.rec.Sex = game.Sex(sex)
	}},
	{"age", game.LevelGreaterGod, setBoth, setNumber, func(s *setContext) {
		if s.value < 2 || s.value > 200 {
			s.refuse("Ages 2 to 200 accepted.\r\n")
			return
		}
		// Age is not stored: it is computed from the birthday, so setting it
		// moves the birthday backwards. The C's own comment warns that the
		// answer may not read back exactly, because the arithmetic that
		// computes an age divides.
		s.rec.Birth = time.Now().Add(-time.Duration(s.value-17) * game.SecondsPerMudYear * time.Second)
	}},
	{"height", game.LevelGod, setBoth, setNumber, func(s *setContext) {
		s.rec.Height = s.value
	}},
	{"weight", game.LevelGod, setBoth, setNumber, func(s *setContext) {
		s.rec.Weight = s.value
	}},
	{"olc", game.LevelImplementor, setPC, setNumber, func(s *setContext) {
		s.rec.OLCZone = s.value
	}},
}

// setPractices is cases 27 and 28, which the C runs through the same code.
func setPractices(s *setContext) { s.rec.SpellsToLearn = s.rangeOf(0, 100) }

// setCondition is cases 29 to 31: drunk, hunger and thirst, which take a
// number or the word "off".
func setCondition(which game.Condition) func(*setContext) {
	return func(s *setContext) {
		if strings.EqualFold(s.arg, "off") {
			s.rec.Conditions[which] = game.CondNotApplicable
			s.output = s.victim.Name + "'s " + s.fieldName + " now off."
			return
		}
		if !isNumber(s.arg) {
			s.refuse("Must be 'off' or a value from 0 to 24.\r\n")
			return
		}
		s.value = atoi(s.arg)
		s.rec.Conditions[which] = s.rangeOf(0, 24)
		s.output = s.victim.Name + "'s " + s.fieldName + " set to " + strconv.FormatInt(int64(s.value), 10) + "."
	}
}

// selfOrImplementor is the extra guard on `invis` and `nohassle`: only an
// implementor may set them on somebody else.
func (s *setContext) selfOrImplementor() bool {
	if levelOf(s.c.Character) < game.LevelImplementor && s.victim != s.c.Character {
		s.refuse("You aren't godly enough for that!\r\n")
		return false
	}
	return true
}

// doSet is do_set (act.wizard.c:2773).
func doSet(c *Context) error {
	name, rest := halfChop(c.Arg)

	// The three qualifiers. `file` edits somebody who is not logged in and is
	// not ported; `player` and `mob` narrow the search.
	onlyPlayers, onlyMobiles := false, false
	switch strings.ToLower(name) {
	case "file":
		c.Send("Setting a character who is not logged in is not supported here.\r\n")
		return nil
	case "player":
		onlyPlayers = true
		name, rest = halfChop(rest)
	case "mob":
		onlyMobiles = true
		name, rest = halfChop(rest)
	}

	field, value := halfChop(rest)
	if name == "" || field == "" {
		c.Send("Usage: set <victim> <field> <value>\r\n")
		return nil
	}

	victim := c.findAnywhere(name)
	switch {
	case victim == nil:
		c.Send("There is no such player.\r\n")
		return nil
	case onlyPlayers && victim.IsNPC():
		c.Send("There is no such player.\r\n")
		return nil
	case onlyMobiles && !victim.IsNPC():
		c.Send("There is no such mobile.\r\n")
		return nil
	}

	chosen := -1
	for i := range setFields {
		if strings.HasPrefix(setFields[i].name, strings.ToLower(field)) {
			chosen = i
			break
		}
	}
	if chosen < 0 {
		c.Send("Can't set that!\r\n")
		return nil
	}

	c.performSet(victim, &setFields[chosen], value)
	return nil
}

// performSet is perform_set (act.wizard.c:2426).
func (c *Context) performSet(victim *game.Character, field *setField, value string) {
	rec := victim.Record
	if rec == nil {
		c.Send("Can't set that!\r\n")
		return
	}

	// An implementor may set anybody, including somebody above them —
	// there is nobody above an implementor. Everybody else is refused a
	// player of their own level or higher, *unless* it is themselves.
	if levelOf(c.Character) != game.LevelImplementor {
		if !victim.IsNPC() && levelOf(c.Character) <= levelOf(victim) && victim != c.Character {
			c.Send("Maybe that's not such a great idea...\r\n")
			return
		}
	}
	if levelOf(c.Character) < field.level {
		c.Send("You are not godly enough for that!\r\n")
		return
	}
	if victim.IsNPC() && field.who&setNPC == 0 {
		c.Send("You can't do that to a beast!\r\n")
		return
	}
	if !victim.IsNPC() && field.who&setPC == 0 {
		c.Send("That can only be done to a beast!\r\n")
		return
	}

	s := &setContext{c: c, victim: victim, rec: rec, arg: value, fieldName: field.name}

	switch field.kind {
	case setBinary:
		switch strings.ToLower(value) {
		case "on", "yes":
			s.on = true
		case "off", "no":
			s.off = true
		default:
			c.Send("Value must be 'on' or 'off'.\r\n")
			return
		}
		s.output = field.name + " " + onOff(s.on) + " for " + victim.Name + "."
	case setNumber:
		s.value = atoi(value)
		s.output = victim.Name + "'s " + field.name + " set to " + strconv.FormatInt(int64(s.value), 10) + "."
	case setMisc:
		s.output = "Okay."
	}

	field.apply(s)
	if s.failed {
		return
	}

	// Every numeric change has to be re-applied through the equipment and
	// spell modifiers, which is what affect_total does after nearly every
	// case in the C.
	game.RecomputeAffects(rec)
	if c.Save != nil {
		c.Save(victim)
	}

	c.Send("%s\r\n", capitaliseFirst(s.output))
}

func idOf(who *game.Character) int64 {
	if who == nil || who.Record == nil {
		return 0
	}
	return who.Record.IDNum
}
