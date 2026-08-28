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
	// A local addition to a ported listing, and the only one: `announce` is
	// not in the C and so neither is this row. It is here because a setting
	// `toggle` does not list is a setting nobody finds — the command exists
	// to be the one place a player can see what they have switched on.
	// docs/deviations.md records both halves.
	c.Send("  Announcements: %-3s\r\n", game.AnnounceLevelOf(rec))
	return nil
}

// doAnnounce sets how much of the `<DoC>` broadcast stream a player hears.
//
// **Not a port. There is no such command in the C**, and no such setting: the
// four send_to_all_color call sites reach everybody who is playing and not
// mid-edit, and that is the whole of it. Recorded in docs/deviations.md.
//
// Written to look like `color` and `syslog` — the two graded settings the C
// does have — down to the prefix match and the "Usage:" line, so that it
// reads as part of the same family rather than as something bolted on. The
// difference is underneath: the bits count *suppression*, so that a record
// written before this existed hears everything. See game.PrefNoAnnounce1.
func doAnnounce(c *Context) error {
	rec := c.Character.Record
	if c.Character.IsNPC() || rec == nil {
		return nil
	}
	arg, _ := oneArgument(c.Arg)

	if arg == "" {
		c.Send("Your announcements are currently %s.\r\n", game.AnnounceLevelOf(rec))
		return nil
	}

	level, ok := game.ParseAnnounceLevel(arg)
	if !ok {
		c.Send("Usage: announce { Off | Brief | All }\r\n")
		return nil
	}
	game.SetAnnounceLevel(rec, level)

	// What each setting actually means, said once at the moment somebody
	// chooses it: "Brief" is not self-explanatory, and a player who cannot
	// tell which messages they have just lost will turn it back on.
	switch level {
	case game.AnnounceOff:
		c.Send("Your announcements are now Off. You will hear none of them.\r\n")
	case game.AnnounceBrief:
		c.Send("Your announcements are now Brief. " +
			"You will hear about newcomers, death traps and remorts, but not levels.\r\n")
	default:
		c.Send("Your announcements are now All. You will hear everything.\r\n")
	}
	return nil
}

// doCommands, porting do_commands. `commands` lists the ordinary ones and
// `socials` the socials, from the same function and the same table.
// listMode is which of do_commands' three subcommands is running.
type listMode int

const (
	// listCommands is SCMD_COMMANDS: everything a mortal can type that is not
	// a social.
	listCommands listMode = iota
	// listSocials is SCMD_SOCIALS.
	listSocials
	// listWizhelp is SCMD_WIZHELP: the immortal commands, and *only* those.
	listWizhelp
)

// doCommands is do_commands (act.informative.c:1770) under all three of its
// names.
//
// The filter is three tests and the middle one is the surprise:
//
//	if ((cmd_info[i].minimum_level >= LVL_IMMORT) != wizhelp)
//	  continue;
//
// So `wizhelp` shows the immortal commands and nothing else, and `commands`
// shows the mortal ones and nothing else — a god typing `commands` does not
// see their own. The two lists are disjoint rather than nested, which is not
// what either name suggests.
//
// The third test puts `insult` among the socials, because it is a social that
// happens to be written in C rather than in the socials file.
func doCommands(mode listMode) func(*Context) error {
	return func(c *Context) error {
		level := c.Character.Level()

		var names []string
		for _, cmd := range Commands {
			if cmd.Run == nil || level < cmd.MinLevel {
				continue
			}
			if (cmd.MinLevel >= game.LevelImmortal) != (mode == listWizhelp) {
				continue
			}
			if mode != listWizhelp {
				// The C's test is on the *function* a row points at:
				//
				//   socials != (cmd_info[i].command_pointer == do_action ||
				//               cmd_info[i].command_pointer == do_insult)
				//
				// (act.informative.c:1502.) `socialLines` is precisely the
				// set of do_action rows, which makes membership in it the
				// faithful test — and a better one than "has a Social
				// attached", which this used before. `hop` is the row that
				// tells them apart: it points at do_action but the shipped
				// socials file has no entry to attach, so it was being listed
				// as a command here and is a social in the C.
				_, isAction := socialLines[cmd.Name]
				isSocial := isAction || cmd.Name == "insult"
				if isSocial != (mode == listSocials) {
					continue
				}
			}
			names = append(names, cmd.Name)
		}
		sort.Strings(names)

		kind := "commands"
		if mode == listSocials {
			kind = "socials"
		}
		privileged := ""
		if mode == listWizhelp {
			privileged = "privileged "
		}
		c.Send("The following %s%s are available to you:\r\n", privileged, kind)

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

// cannedText returns the command for one of do_gen_ps's files. do_gen_ps
// pages every one of its own branches (act.informative.c:1375-1399) —
// SendPaged, not Send.
func cannedText(name string, text func(TextFiles) string) func(*Context) error {
	return func(c *Context) error {
		body := text(c.Text)
		if strings.TrimSpace(body) == "" {
			c.Send("There is no %s.\r\n", name)
			return nil
		}
		c.SendPaged("%s", ensureNewline(body))
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
	// The immortal branch is the whole first half of the C's function
	// (act.other.c:404): at or above LVL_IMMORT, `visible` means
	// perform_immort_vis and has nothing to do with the invisibility
	// *spell*. It was missing, and the consequence was not subtle — a
	// wizinvis god typing `visible` fell through to the mortal path below,
	// which tests AFF_INVISIBLE, a flag they do not have, and was told
	// "You are already visible." while staying invisible. Toggling `invis`
	// a second time was the only way back. Found by test/play.
	if levelOf(c.Character) >= game.LevelImmortal {
		c.becomeVisible()
		return nil
	}

	rec := c.Character.Record
	if rec == nil {
		return nil
	}
	if !rec.AffectFlags.Has(game.AffectInvisible) {
		c.Send("You are already visible.\r\n")
		return nil
	}

	// appear() and then the message, in that order, as the C has it. The
	// room's half used to be "snaps into visibility", which is in neither
	// C tree — appear is where the wording lives, and both halves of
	// do_visible go through it.
	c.appear()
	c.Send("You break the spell of invisibility.\r\n")
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

// doSave, porting do_save (act.other.c:166-193).
//
// The C's `if (cmd)` is always taken here: `save` is only ever reached from
// the command table, and the one thing the branch guards — telling the
// player anything at all — is what a typed `save` is for. Nothing in the C
// calls do_save internally either (grep act.other.c and interpreter.c: the
// only references are the prototype and the table entry).
//
// What the branch actually holds is the duplication guard, and it is easy to
// read past because it looks like a message-formatting choice:
//
//	if (auto_save && GET_LEVEL(ch) <= LVL_IMMORT) {
//	  send_to_char(ch, "Saving aliases.\r\n");
//	  write_aliases(ch);
//	  return;
//	}
//
// With the periodic sweep on, a mortal's `save` writes their *aliases* and
// nothing else. The C's comment (act.other.c:173-179) says why: two players
// with coordinated saves, or one player with a house, can otherwise duplicate
// items across a save and a crash, and the sweep is already authoritative, so
// there is nothing to be gained by letting anyone force a write. It assumes
// guest immortals are untrustworthy, hence `<=` rather than `<` — level 31
// itself is inside the guard.
//
// The aliases still get written because in the C nothing else ever writes
// them: write_aliases has exactly these two call sites, so a `save` that
// skipped it would lose an alias defined this session. The formats a server
// runs on (`ascii` and `yaml`) keep aliases on the player record rather than
// in a separate plralias file, so "write only the aliases" is SaveAliases'
// read-modify-write of that record rather than a second file — the effect on
// disk is the same one the C has.
func doSave(c *Context) error {
	if c.Character.IsNPC() || c.Session == nil {
		return nil
	}
	if game.Tuning().AutoSave && c.Character.Level() <= game.LevelImmortal {
		c.Send("Saving aliases.\r\n")
		if c.SaveAliases != nil {
			c.SaveAliases(c.Character)
		}
		return nil
	}
	c.Send("Saving %s and aliases.\r\n", c.Character.Name)
	if c.Save != nil {
		c.Save(c.Character)
	}
	return nil
}
