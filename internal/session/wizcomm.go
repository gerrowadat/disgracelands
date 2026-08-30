// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package session

import (
	"fmt"
	"strings"

	"github.com/gerrowadat/disgracelands/internal/colour"
	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/obs"
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
	victim := c.findAnywhere(name)
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
	// No delete_doubledollar. It is there in the C because the text goes
	// out as a format string there and every `$` in it was doubled on the
	// way in; here it goes out as an argument to %s, and nothing doubled.
	// See alias.go and docs/deviations.md.
	text := strings.TrimSpace(c.Arg)
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
		victim := c.findAnywhere(name)
		switch {
		case victim == nil:
			c.Send("%s", noPerson)
		case !victim.IsNPC() && levelOf(c.Character) <= levelOf(victim):
			c.Send("No, no, no!\r\n")
		default:
			c.Send("Okay.\r\n")
			victim.Tell("%s", told)
			// mudlog(buf, NRM, MAX(LVL_GOD, GET_INVIS_LEV(ch)), TRUE)
			// (act.wizard.c:1832-1833), before the forced command runs —
			// which matters, because that command may well be the last
			// thing either character does.
			c.wizlogInvis(obs.LogNormal, game.LevelGod, c.Character,
				"(GC) %s forced %s to %s", c.Character.Name, victim.Name, command)
			c.runAs(victim, command)
		}
		return nil
	}

	c.Send("Okay.\r\n")
	targets := c.World.Occupants(c.Character.Room)
	if group == "all" {
		// mudlog(buf, NRM, MAX(LVL_GOD, GET_INVIS_LEV(ch)), TRUE)
		// (act.wizard.c:1851-1852).
		c.wizlogInvis(obs.LogNormal, game.LevelGod, c.Character,
			"(GC) %s forced all to %s", c.Character.Name, command)
		targets = c.World.Players()
	} else {
		// (act.wizard.c:1839-1840). The C prints the room's vnum.
		c.wizlogInvis(obs.LogNormal, game.LevelGod, c.Character,
			"(GC) %s forced room %d to %s", c.Character.Name, c.Character.Room, command)
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
		c.Send("Your syslog is currently %s.\r\n", logTypes[SyslogLevel(rec)])
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
	rec.Preferences = rec.Preferences.Without(game.PrefLog1, game.PrefLog2)
	if level&1 != 0 {
		rec.Preferences = rec.Preferences.With(game.PrefLog1)
	}
	if level&2 != 0 {
		rec.Preferences = rec.Preferences.With(game.PrefLog2)
	}
	c.Send("Your syslog is now %s.\r\n", logTypes[level])
	return nil
}

// SyslogLevel is the two PRF_LOG bits read together as one number
// (act.wizard.c:1402's own arithmetic) — an online immortal's own syslog
// verbosity, and what mudlog()'s in-game echo (utils.c:250-253) compares a
// message's own type against. Exported for internal/server's echoWizVis,
// which applies the same comparison from outside this package.
func SyslogLevel(rec *game.PlayerRecord) int {
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

	// The delete_doubledollar beside skip_spaces in do_wiznet
	// (act.wizard.c:1721) is not ported, for the reason alias.go sets out:
	// nothing in this port doubles a `$` on the way in, so collapsing here
	// would only eat one an immortal typed on purpose.
	text := strings.TrimSpace(c.Arg)
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
		// `!PLR_FLAGGED(d->character, PLR_WRITING | PLR_MAILING)`
		// (act.wizard.c:1960): a god in the line editor is left alone,
		// the same courtesy do_gen_comm's channels extend. Live as of
		// #214 — nothing set either bit before that.
		if who.Record.PlayerFlags.HasAny(game.PlayerWriting, game.PlayerMailing) {
			continue
		}
		if who == c.Character && c.noRepeat() {
			continue
		}
		// Cyan, at C_NRM (act.wizard.c:1962-1967). Note it is resolved
		// against the *reader's* preference, as every CC macro is, and not
		// the wizline's author's.
		who.TellAt(colour.Normal, "{{cyan}}%s{{/}}", line)
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
		fmt.Fprintf(&online, "  %s%s\r\n", who.Name, writingSuffix(who.Record))
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

// writingSuffix is the annotation `wiznet @` hangs off a god's name
// (act.wizard.c:1907-1911):
//
//	if (PLR_FLAGGED(d->character, PLR_WRITING))
//	  strcat(buf1, " (Writing)\r\n");
//	else if (PLR_FLAGGED(d->character, PLR_MAILING))
//	  strcat(buf1, " (Writing mail)\r\n");
//
// **The second arm is unreachable, and it is reproduced anyway.** `mail`
// sets PLR_MAILING (mail.c:567) and then calls string_write, which sets
// PLR_WRITING (modify.c:100-101); the cleanup clears both together
// (:218-219) and a login clears both again (interpreter.c:1386). So
// nothing ever carries PLR_MAILING without PLR_WRITING, the first arm
// always wins, and a god writing a letter is reported as plain
// "(Writing)" — never "(Writing mail)", on the real server or here.
//
// do_who's own pair tests the two bits the other way round
// (act.informative.c:1174-1176) and so would say "(mailing)"; that
// annotation block is not ported. See docs/weirdnumbers.md.
func writingSuffix(rec *game.PlayerRecord) string {
	switch {
	case rec.PlayerFlags.Has(game.PlayerWriting):
		return " (Writing)"
	case rec.PlayerFlags.Has(game.PlayerMailing):
		return " (Writing mail)"
	}
	return ""
}
