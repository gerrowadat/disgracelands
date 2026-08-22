// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package session

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/gerrowadat/disgracelands/internal/buildinfo"
	"github.com/gerrowadat/disgracelands/internal/game"
)

// The rest of act.informative.c and the small half of act.other.c: the
// commands that tell you something, and the settings that decide how much.
//
// Most of them are three lines and a message. What makes them worth porting
// carefully is that they are the ones a player types most: `gold`, `report`,
// `prompt`, `toggle`.

// doGold, porting do_gold. Three answers for one number.
func doGold(c *Context) error {
	rec := c.Character.Record
	if rec == nil {
		return nil
	}
	switch gold := rec.Points.Gold; gold {
	case 0:
		c.Send("You're broke!\r\n")
	case 1:
		c.Send("You have one miserable little gold coin.\r\n")
	default:
		c.Send("You have %d gold coins.\r\n", gold)
	}
	return nil
}

// doLevels, porting do_levels: the whole experience table for your class,
// with the title you wear at each rung.
func doLevels(c *Context) error {
	rec := c.Character.Record
	if c.Character.IsNPC() || rec == nil {
		c.Send("You ain't nothin' but a hound-dog.\r\n")
		return nil
	}

	var b strings.Builder
	for level := int32(1); level < game.LevelImmortal; level++ {
		fmt.Fprintf(&b, "[%2d] %8d-%-8d : %s\r\n", level,
			game.LevelExperience(rec.Class, level),
			game.LevelExperience(rec.Class, level+1)-1,
			game.Title(rec.Class, level, rec.Sex))
	}
	fmt.Fprintf(&b, "[%2d] %8d          : Immortality\r\n",
		game.LevelImmortal, game.LevelExperience(rec.Class, game.LevelImmortal))

	c.Send("%s", b.String())
	return nil
}

// doDiagnose, porting do_diagnose: how hurt somebody looks, which is the same
// line `look` gives and the only way to check on whoever you are fighting
// without looking away.
func doDiagnose(c *Context) error {
	name, _ := oneArgument(c.Arg)

	if name == "" {
		if c.Character.Fighting == nil {
			c.Send("Diagnose who?\r\n")
			return nil
		}
		c.Send("%s", game.HealthDiagnosis(c.Character.Fighting.Name, c.Character.Fighting.Record))
		return nil
	}

	victim := c.findInRoom(name)
	if victim == nil {
		c.Send("No-one by that name here.\r\n")
		return nil
	}
	c.Send("%s", game.HealthDiagnosis(victim.Name, victim.Record))
	return nil
}

// doWhere, porting perform_mortal_where.
//
// A mortal sees only their own zone, and only players — which is the whole
// difference between this and the immortal version that arrives with
// act.wizard.c.
func doWhere(c *Context) error {
	name, _ := oneArgument(c.Arg)

	if name == "" {
		c.Send("Players in your Zone\r\n--------------------\r\n")
		for _, other := range c.World.Players() {
			if other == c.Character || other.Client == nil || !c.sameZone(other.Room) {
				continue
			}
			// CAN_SEE (act.informative.c:1429). An invis god is not in your
			// zone as far as you are concerned.
			if !c.World.CanSee(c.Character, other) {
				continue
			}
			c.Send("%-20s - %s\r\n", other.Name, c.roomName(other.Room))
		}
		return nil
	}

	for _, other := range c.World.Players() {
		if other == c.Character || !c.sameZone(other.Room) || !nameMatches(other, name) {
			continue
		}
		if !c.World.CanSee(c.Character, other) {
			continue
		}
		c.Send("%-25s - %s\r\n", other.Name, c.roomName(other.Room))
		return nil
	}
	c.Send("No-one around by that name.\r\n")
	return nil
}

// doReport, porting do_report: your own condition, to the group.
func doReport(c *Context) error {
	rec := c.Character.Record
	if rec == nil {
		return nil
	}
	if !c.Character.Grouped() {
		c.Send("But you are not a member of any group!\r\n")
		return nil
	}

	report := fmt.Sprintf("%s reports: %d/%dH, %d/%dM, %d/%dV\r\n",
		c.Character.Name,
		rec.Points.Hit, rec.Points.MaxHit,
		rec.Points.Mana, rec.Points.MaxMana,
		rec.Points.Move, rec.Points.MaxMove)

	leader := c.Character.GroupLeader()
	for _, f := range leader.Followers {
		if f.Grouped() && f != c.Character {
			f.Tell("%s", report)
		}
	}
	if leader != c.Character {
		leader.Tell("%s", report)
	}
	c.Send("You report to the group.\r\n")
	return nil
}

// doSplit, porting do_split: divide gold between everybody grouped with you
// who is in the room.
//
// The remainder stays with the splitter, and the message says so — "3 coins
// were not splitable, so Zod keeps the money." The arithmetic that follows is
// the part to read twice: the splitter is charged `share * (num - 1)`, so
// they keep their own share *and* the remainder without ever handing it over.
func doSplit(c *Context) error {
	rec := c.Character.Record
	if c.Character.IsNPC() || rec == nil {
		return nil
	}

	word, _ := oneArgument(c.Arg)
	if !isNumber(word) || word == "" {
		c.Send("How many coins do you wish to split with your group?\r\n")
		return nil
	}
	amount := atoi(word)

	switch {
	case amount <= 0:
		c.Send("Sorry, you can't do that.\r\n")
		return nil
	case amount > rec.Points.Gold:
		c.Send("You don't seem to have that much gold to split.\r\n")
		return nil
	}

	// Everybody grouped and present, players only — a charmed pet does not
	// get a cut.
	var members []*game.Character
	leader := c.Character.GroupLeader()
	if leader.Grouped() && leader.Room == c.Character.Room {
		members = append(members, leader)
	}
	for _, f := range leader.Followers {
		if f.Grouped() && !f.IsNPC() && f.Room == c.Character.Room && f != leader {
			members = append(members, f)
		}
	}

	if len(members) == 0 || !c.Character.Grouped() {
		c.Send("With whom do you wish to share your gold?\r\n")
		return nil
	}

	num := int32(len(members)) //nolint:gosec // a room's worth of people
	share := amount / num
	rest := amount % num

	rec.Points.Gold -= share * (num - 1)

	told := fmt.Sprintf("%s splits %d coins; you receive %d.\r\n",
		c.Character.Name, amount, share)
	if rest != 0 {
		told += fmt.Sprintf("%d coin%s %s not splitable, so %s keeps the money.\r\n",
			rest, plural(int(rest)), wasWere(rest), c.Character.Name)
	}

	for _, member := range members {
		if member == c.Character || member.Record == nil {
			continue
		}
		member.Record.Points.Gold += share
		member.Tell("%s", told)
	}

	c.Send("You split %d coins among %d members -- %d coins each.\r\n", amount, num, share)
	if rest != 0 {
		c.Send("%d coin%s %s not splitable, so you keep the money.\r\n",
			rest, plural(int(rest)), wasWere(rest))
		rec.Points.Gold += rest
	}
	return nil
}

// doWimpy, porting do_wimpy: flee automatically below a hit-point threshold.
//
// The ceiling is half your maximum, and the refusals are worth keeping: a
// negative is "Heh, heh, heh.. we are jolly funny today, eh?" and one above
// your maximum is "That doesn't make much sense, now does it?".
func doWimpy(c *Context) error {
	rec := c.Character.Record
	if c.Character.IsNPC() || rec == nil {
		return nil
	}

	arg, _ := oneArgument(c.Arg)
	if arg == "" {
		if rec.WimpLevel != 0 {
			c.Send("Your current wimp level is %d hit points.\r\n", rec.WimpLevel)
		} else {
			c.Send("At the moment, you're not a wimp.  (sure, sure...)\r\n")
		}
		return nil
	}
	if !isNumber(arg) {
		c.Send("Specify at how many hit points you want to wimp out at.  (0 to disable)\r\n")
		return nil
	}

	level := atoi(arg)
	switch {
	case level == 0:
		c.Send("Okay, you'll now tough out fights to the bitter end.\r\n")
		rec.WimpLevel = 0
	case level < 0:
		c.Send("Heh, heh, heh.. we are jolly funny today, eh?\r\n")
	case level > rec.Points.MaxHit:
		c.Send("That doesn't make much sense, now does it?\r\n")
	case level > rec.Points.MaxHit/2:
		c.Send("You can't set your wimp level above half your hit points.\r\n")
	default:
		c.Send("Okay, you'll wimp out if you drop below %d hit points.\r\n", level)
		rec.WimpLevel = level
	}
	return nil
}

// doDisplay, porting do_display: which numbers the prompt carries.
func doDisplay(c *Context) error {
	rec := c.Character.Record
	if c.Character.IsNPC() || rec == nil {
		c.Send("Mosters don't need displays.  Go away.\r\n")
		return nil
	}

	arg := strings.ToLower(strings.TrimSpace(c.Arg))
	if arg == "" {
		c.Send("Usage: prompt { { H | M | V } | all | none }\r\n")
		return nil
	}

	const all = game.PrefDisplayHP | game.PrefDisplayMana | game.PrefDisplayMove
	switch arg {
	case "on", "all":
		rec.Preferences = rec.Preferences.Set(all)
	case "off", "none":
		rec.Preferences = rec.Preferences.Clear(all)
	default:
		wanted := game.Flags(0)
		for _, ch := range arg {
			switch ch {
			case 'h':
				wanted |= game.PrefDisplayHP
			case 'm':
				wanted |= game.PrefDisplayMana
			case 'v':
				wanted |= game.PrefDisplayMove
			default:
				c.Send("Usage: prompt { { H | M | V } | all | none }\r\n")
				return nil
			}
		}
		rec.Preferences = rec.Preferences.Clear(all).Set(wanted)
	}

	c.Send("Okay.\r\n")
	return nil
}

// doTitle, porting do_title.
func doTitle(c *Context) error {
	rec := c.Character.Record
	title := strings.TrimSpace(c.Arg)

	switch {
	case c.Character.IsNPC() || rec == nil:
		c.Send("Your title is fine... go away.\r\n")
	case rec.PlayerFlags.Has(game.PlayerNoTitle):
		c.Send("You can't title yourself -- you shouldn't have abused it!\r\n")
	case strings.ContainsAny(title, "()"):
		c.Send("Titles can't contain the ( or ) characters.\r\n")
	case len(title) > game.MaxTitleLength:
		c.Send("Sorry, titles can't be longer than %d characters.\r\n", game.MaxTitleLength)
	default:
		rec.Title = title
		c.Send("Okay, you're now %s %s.\r\n", c.Character.Name, rec.Title)
	}
	return nil
}

// doToggle, porting do_toggle: every preference on one screen.
func doToggle(c *Context) error {
	rec := c.Character.Record
	if c.Character.IsNPC() || rec == nil {
		return nil
	}

	wimp := "OFF"
	if rec.WimpLevel != 0 {
		wimp = strconv.Itoa(int(rec.WimpLevel))
	}
	on := func(flag game.Flags) string {
		if rec.Preferences.Has(flag) {
			return "ON"
		}
		return "OFF"
	}

	// The immortal block comes first and only for immortals.
	if c.Character.Level() >= game.LevelImmortal {
		c.Send("      No Hassle: %-3s          Holylight: %-3s         Room Flags: %-3s\r\n",
			on(game.PrefNoHassle), on(game.PrefHolylight), on(game.PrefRoomFlags))
	}

	c.Send("Hit Pnt Display: %-3s         Brief Mode: %-3s     Summon Protect: %-3s\r\n",
		on(game.PrefDisplayHP), on(game.PrefBrief), on(game.PrefSummonable))
	c.Send("   Move Display: %-3s       Compact Mode: %-3s           On Quest: %-3s\r\n",
		on(game.PrefDisplayMove), on(game.PrefCompact), on(game.PrefQuest))
	c.Send("   Mana Display: %-3s             NoTell: %-3s       Repeat Comm.: %-3s\r\n",
		on(game.PrefDisplayMana), on(game.PrefNoTell), on(game.PrefNoRepeat))
	c.Send(" Auto Show Exit: %-3s               Deaf: %-3s         Wimp Level: %-3s\r\n",
		on(game.PrefAutoExit), on(game.PrefDeaf), wimp)
	c.Send(" Gossip Channel: %-3s    Auction Channel: %-3s      Grats Channel: %-3s\r\n",
		on(game.PrefNoGoss), on(game.PrefNoAuct), on(game.PrefNoGratz))
	return nil
}

// doCommands, porting do_commands. `commands` lists the ordinary ones and
// `socials` the socials, from the same function and the same table.
func doCommands(socialsOnly bool) func(*Context) error {
	return func(c *Context) error {
		var names []string
		for _, cmd := range Commands {
			if (cmd.Social != nil || cmd.Run == nil) != socialsOnly {
				continue
			}
			// A one-character command is an alias for a real one and is not
			// listed; the C's table has them and `commands` shows them, but
			// they read as noise in a list.
			if len(cmd.Name) < 2 {
				continue
			}
			names = append(names, cmd.Name)
		}
		sort.Strings(names)

		kind := "commands"
		if socialsOnly {
			kind = "socials"
		}
		c.Send("The following %s are available to you:\r\n", kind)

		// Seven to a line, eleven columns each, as the C does.
		var b strings.Builder
		for i, name := range names {
			fmt.Fprintf(&b, "%-11s", name)
			if (i+1)%7 == 0 {
				b.WriteString("\r\n")
			}
		}
		if len(names)%7 != 0 {
			b.WriteString("\r\n")
		}
		c.Send("%s", b.String())
		return nil
	}
}

// cannedText returns the command for one of do_gen_ps's files.
func cannedText(name string, text func(TextFiles) string) func(*Context) error {
	return func(c *Context) error {
		body := text(c.Text)
		if strings.TrimSpace(body) == "" {
			c.Send("There is no %s.\r\n", name)
			return nil
		}
		c.Send("%s", ensureNewline(body))
		return nil
	}
}

// doClearScreen is SCMD_CLEAR: the ANSI home-and-clear, sent raw.
func doClearScreen(c *Context) error {
	c.Send("\033[H\033[J")
	return nil
}

// doWhoAmI is SCMD_WHOAMI, which exists because `switch` and `return` can
// leave an immortal genuinely unsure.
func doWhoAmI(c *Context) error {
	c.Send("%s\r\n", c.Character.Name)
	return nil
}

// doVisible, porting do_visible: drop invisibility deliberately.
func doVisible(c *Context) error {
	rec := c.Character.Record
	if rec == nil {
		return nil
	}
	if !rec.AffectFlags.Has(game.AffectInvisible) {
		c.Send("You are already visible.\r\n")
		return nil
	}

	game.RemoveAffectsOf(rec, game.SpellInvisible)
	rec.BaseAffectFlags = rec.BaseAffectFlags.Clear(game.AffectInvisible)
	game.RecomputeAffects(rec)
	c.Send("You break the spell of invisibility.\r\n")
	c.announce("%s snaps into visibility.\r\n", c.Character.Name)
	return nil
}

// nameMatches reports whether a typed word names somebody, the way `where`
// searches — which is the same isname() every other search uses, so a
// whole word and not a prefix.
func nameMatches(who *game.Character, word string) bool { return who.NamedBy(word) }

// roomName is a room's name, or a placeholder for one that has gone.
func (c *Context) roomName(vnum game.RoomVnum) string {
	if room := c.World.Room(vnum); room != nil {
		return room.Name
	}
	return "somewhere"
}

func wasWere(n int32) string {
	if n == 1 {
		return "was"
	}
	return "were"
}

// doVersion is SCMD_VERSION.
//
// The C answers "CircleMUD, version 3.00 beta patchlevel 19" — the base it
// was built from, not this tree, which never updated the string. This answers
// with both: what this server is, and what it is a port of, because the
// second half is a licence obligation (plan §12) and the first is the useful
// part of the question.
func doVersion(c *Context) error {
	c.Send("%s\r\n", buildinfo.Get().String())
	c.Send("A Go port of %s\r\n", game.CircleMUDVersion)
	return nil
}

// doSave, porting do_save.
//
// The C's version is mostly about object and house saving, neither of which
// exists yet. What it does that matters is confirm to the player that their
// character is on disk — and the C is careful to say so only when they typed
// the command rather than when something called it internally.
func doSave(c *Context) error {
	if c.Character.IsNPC() || c.Session == nil {
		return nil
	}
	c.Send("Saving %s.\r\n", c.Character.Name)
	if c.Save != nil {
		c.Save(c.Character)
	}
	return nil
}
