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

// Talking as a god, and making other people act, ported from act.wizard.c.
//
// `echo` and `emote` are the same function; `send` writes to one person;
// `gecho` writes to everybody; `wiznet` is the gods' channel; `force` makes
// somebody type something.

// doEcho is do_echo with SCMD_ECHO (act.wizard.c:102): text with no
// attribution at all, which is how a god narrates.
func doEcho(c *Context) error { return c.echo(false) }

// doEmote is the same function with SCMD_EMOTE, and it is a *mortal* command
// — level 1 in the C's table. The only difference is the "$n " in front.
func doEmote(c *Context) error { return c.echo(true) }

func (c *Context) echo(attributed bool) error {
	text := strings.TrimSpace(c.Arg)
	if text == "" {
		c.Send("Yes.. but what?\r\n")
		return nil
	}

	line := text
	if attributed {
		line = c.Character.Name + " " + text
	}
	c.announce("%s\r\n", line)

	// NOREPEAT swaps the echo back to you for a bare "Okay.", which is the
	// same bargain every channel in the game offers.
	if c.noRepeat() {
		c.Send("Okay.\r\n")
	} else {
		c.Send("%s\r\n", line)
	}
	return nil
}

// doSend is do_send (act.wizard.c:122): one line, to one person, from
// nowhere.
func doSend(c *Context) error {
	name, message := halfChop(c.Arg)
	if name == "" {
		c.Send("Send what to who?\r\n")
		return nil
	}
	victim := c.World.FindAnywhere(name)
	if victim == nil {
		c.Send("%s", noPerson)
		return nil
	}

	victim.Tell("%s\r\n", message)
	if c.noRepeat() {
		c.Send("Sent.\r\n")
	} else {
		c.Send("You send '%s' to %s.\r\n", message, victim.Name)
	}
	return nil
}

// doGecho is do_gecho (act.wizard.c:1616): to everybody in the game.
func doGecho(c *Context) error {
	text := strings.TrimSpace(c.Arg)
	// delete_doubledollar, because this goes out as a format string in the C
	// and a lone `$` would eat the next character.
	text = strings.ReplaceAll(text, "$$", "$")
	if text == "" {
		c.Send("That must be a mistake...\r\n")
		return nil
	}

	for _, who := range c.World.Players() {
		if who != c.Character {
			who.Tell("%s\r\n", text)
		}
	}
	if c.noRepeat() {
		c.Send("Okay.\r\n")
	} else {
		c.Send("%s\r\n", text)
	}
	return nil
}

// doForce is do_force (act.wizard.c:1812): make somebody type something.
func doForce(c *Context) error {
	name, command := halfChop(c.Arg)
	if name == "" || command == "" {
		c.Send("Whom do you wish to force do what?\r\n")
		return nil
	}
	told := fmt.Sprintf("%s has forced you to '%s'.\r\n", c.Character.Name, command)

	// `force all` and `force room` are a greater god's — and note how the C
	// spells that: the level test is folded into the same condition as the
	// name test, so a *lesser* god typing `force all` looks for somebody
	// actually called "all" rather than being refused.
	group := strings.ToLower(name)
	if levelOf(c.Character) < game.LevelGreaterGod || (group != "all" && group != "room") {
		victim := c.World.FindAnywhere(name)
		switch {
		case victim == nil:
			c.Send("%s", noPerson)
		case !victim.IsNPC() && levelOf(c.Character) <= levelOf(victim):
			c.Send("No, no, no!\r\n")
		default:
			c.Send("Okay.\r\n")
			victim.Tell("%s", told)
			c.runAs(victim, command)
		}
		return nil
	}

	c.Send("Okay.\r\n")
	targets := c.World.Occupants(c.Character.Room)
	if group == "all" {
		targets = c.World.Players()
	}
	for _, victim := range append([]*game.Character(nil), targets...) {
		if !victim.IsNPC() && levelOf(victim) >= levelOf(c.Character) {
			continue
		}
		victim.Tell("%s", told)
		c.runAs(victim, command)
	}
	return nil
}

// --- wiznet ------------------------------------------------------------

// logTypes are logtypes[] (act.wizard.c:1398), and they double as the
// syslog levels: the two PRF_LOG bits together make a number 0-3 which
// indexes this.
var logTypes = []string{"off", "brief", "normal", "complete"}

// doSyslog is do_syslog (act.wizard.c:1402).
func doSyslog(c *Context) error {
	rec := c.Character.Record
	if rec == nil {
		return nil
	}
	arg, _ := oneArgument(c.Arg)

	if arg == "" {
		c.Send("Your syslog is currently %s.\r\n", logTypes[syslogLevel(rec)])
		return nil
	}

	level := -1
	for i, name := range logTypes {
		if isPrefixOf(arg, name) {
			level = i
			break
		}
	}
	if level < 0 {
		c.Send("Usage: syslog { Off | Brief | Normal | Complete }\r\n")
		return nil
	}

	// Two bits making one number, which is why the C's assignment looks like
	// arithmetic: `PRF_LOG1 * (tp & 1) | PRF_LOG2 * (tp & 2) >> 1`.
	rec.Preferences = rec.Preferences.Clear(game.PrefLog1 | game.PrefLog2)
	if level&1 != 0 {
		rec.Preferences = rec.Preferences.Set(game.PrefLog1)
	}
	if level&2 != 0 {
		rec.Preferences = rec.Preferences.Set(game.PrefLog2)
	}
	c.Send("Your syslog is now %s.\r\n", logTypes[level])
	return nil
}

func syslogLevel(rec *game.PlayerRecord) int {
	level := 0
	if rec.Preferences.Has(game.PrefLog1) {
		level++
	}
	if rec.Preferences.Has(game.PrefLog2) {
		level += 2
	}
	return level
}

// doWiznet is do_wiznet (act.wizard.c:1867): the gods' channel.
//
// Four prefixes, and the parsing of them is the fiddliest thing in the file:
// `#<level>` restricts who hears it, `*` makes it an emote, `@` lists who is
// listening, and `\` escapes a line that starts with one of the others.
func doWiznet(c *Context) error {
	rec := c.Character.Record
	if rec == nil {
		return nil
	}

	text := strings.TrimSpace(c.Arg)
	text = strings.ReplaceAll(text, "$$", "$")
	if text == "" {
		c.Send("Usage: wiznet <text> | #<level> <text> | *<emotetext> |\r\n" +
			"       wiznet @<level> *<emotetext> | wiz @\r\n")
		return nil
	}

	emote := false
	level := game.LevelImmortal

	switch text[0] {
	case '*', '#':
		emote = text[0] == '*'
		// The C falls through from '*' into '#', so a `*` line may *also*
		// carry a level: `wiznet *32 waves` is an emote at level 32.
		rest := text[1:]
		if first, _ := oneArgument(rest); isNumber(first) {
			word, remainder := halfChop(rest)
			level = max(atoi(word), game.LevelImmortal)
			if level > levelOf(c.Character) {
				c.Send("You can't wizline above your own level.\r\n")
				return nil
			}
			text = remainder
		} else if emote {
			text = rest
		}
	case '@':
		c.listGods()
		return nil
	case '\\':
		text = text[1:]
	}

	if rec.Preferences.Has(game.PrefNoWiz) {
		c.Send("You are offline!\r\n")
		return nil
	}
	text = strings.TrimSpace(text)
	if text == "" {
		c.Send("Don't bother the gods like that!\r\n")
		return nil
	}

	prefix := ""
	if emote {
		prefix = "<--- "
	}
	line := fmt.Sprintf("%s: %s%s\r\n", c.Character.Name, prefix, text)
	if level > game.LevelImmortal {
		line = fmt.Sprintf("%s: <%d> %s%s\r\n", c.Character.Name, level, prefix, text)
	}

	for _, who := range c.World.Players() {
		if levelOf(who) < level || who.Record == nil {
			continue
		}
		if who.Record.Preferences.Has(game.PrefNoWiz) {
			continue
		}
		if who == c.Character && c.noRepeat() {
			continue
		}
		who.Tell("%s", line)
	}

	if c.noRepeat() {
		c.Send("Okay.\r\n")
	}
	return nil
}

// listGods is `wiznet @`: who is on the channel and who has turned it off.
func (c *Context) listGods() {
	var online, offline strings.Builder

	for _, who := range c.World.Players() {
		if levelOf(who) < game.LevelImmortal || who.Record == nil {
			continue
		}
		if who.Record.Preferences.Has(game.PrefNoWiz) {
			fmt.Fprintf(&offline, "  %s\r\n", who.Name)
			continue
		}
		fmt.Fprintf(&online, "  %s\r\n", who.Name)
	}

	var b strings.Builder
	if online.Len() > 0 {
		b.WriteString("Gods online:\r\n")
		b.WriteString(online.String())
	}
	if offline.Len() > 0 {
		b.WriteString("Gods offline:\r\n")
		b.WriteString(offline.String())
	}
	c.Send("%s", b.String())
}

// noRepeat is PRF_NOREPEAT: the preference that swaps your own echo for a
// bare acknowledgement.
func (c *Context) noRepeat() bool {
	return c.Character.Record != nil && c.Character.Record.Preferences.Has(game.PrefNoRepeat)
}
