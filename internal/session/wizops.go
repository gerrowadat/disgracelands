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

// Running the place, ported from act.wizard.c.
//
// `snoop`, `switch` and `return` are the only commands in the game that reach
// past the character to the *connection* — one watches another's output, the
// other two put a session in charge of somebody else's body. Then `dc`,
// `wizlock`, `shutdown` and the two clocks.

// Operator is what these commands need from the server: the connections, and
// the switch that stops the world.
//
// A seam like the others. The session package knows what a Session is but not
// how they are kept, and the shutdown flag belongs to whoever owns main().
type Operator interface {
	// Sessions returns every live connection, in the order they were made —
	// which is the order `users` numbers them and `dc` addresses them.
	Sessions() []*Session
	// Restrict sets and reads the minimum level allowed in, which is the C's
	// `circle_restrict`.
	Restrict() int32
	SetRestrict(level int32)
	// Shutdown stops the server. reboot asks the supervisor to start it
	// again.
	Shutdown(reboot bool)
	// BootTime is when the server came up.
	BootTime() time.Time
	// LastLogin looks a character up in the roster without loading them into
	// the world, for `last`.
	LastLogin(name string) (LastLogin, bool)
	// ShowPlayer is the same lookup with more of the record, for
	// `show player`.
	ShowPlayer(name string) (PlayerSummary, bool)
	// ZoneAge is how many minutes since a zone last reset, which the C keeps
	// in the zone table and this port keeps beside the pulse.
	ZoneAge(vnum game.ZoneVnum) int32
}

// PlayerSummary is what `show player` reports.
type PlayerSummary struct {
	Name      string
	Sex       int32
	Level     int32
	Class     int32
	Gold      int32
	Bank      int32
	Exp       int32
	Alignment int32
	Lessons   int32
	Born      time.Time
	LastLogon time.Time
	Played    time.Duration
}

// LastLogin is what `last` reports.
type LastLogin struct {
	IDNum int64
	Level int32
	Class int32
	Name  string
	Host  string
	When  time.Time
}

// doSnoop is do_snoop (act.wizard.c:1128).
func doSnoop(c *Context) error {
	if c.Session == nil {
		return nil
	}
	name, _ := oneArgument(c.Arg)

	if name == "" {
		c.stopSnooping()
		return nil
	}
	victim := c.findAnywhere(name)
	switch victim {
	case nil:
		c.Send("No such person around.\r\n")
		return nil
	case c.Character:
		// Snooping yourself is how the C spells "stop", alongside a bare
		// `snoop`.
		c.stopSnooping()
		return nil
	}

	target, ok := victim.Client.(*Session)
	if !ok || target == nil {
		c.Send("There's no link.. nothing to snoop.\r\n")
		return nil
	}
	if target.SnoopedBy() != nil {
		c.Send("Busy already. \r\n")
		return nil
	}
	if target.Snooping() == c.Session {
		c.Send("Don't be stupid.\r\n")
		return nil
	}

	// Whose level counts is the *original* if they are switched: snooping a
	// god who is wearing a rat must still be refused.
	behind := victim
	if original := target.Original(); original != nil {
		behind = original
	}
	if levelOf(behind) >= levelOf(c.Character) {
		c.Send("You can't.\r\n")
		return nil
	}

	c.Session.StopSnooping()
	c.Session.SnoopOn(target)
	c.Send("Okay.\r\n")
	return nil
}

func (c *Context) stopSnooping() {
	if c.Session.Snooping() == nil {
		c.Send("You aren't snooping anyone.\r\n")
		return
	}
	c.Session.StopSnooping()
	c.Send("You stop snooping.\r\n")
}

// doSwitch is do_switch (act.wizard.c:1171): drive somebody else's body.
func doSwitch(c *Context) error {
	if c.Session == nil {
		return nil
	}
	name, _ := oneArgument(c.Arg)

	if c.Session.Original() != nil {
		c.Send("You're already switched.\r\n")
		return nil
	}
	if name == "" {
		c.Send("Switch with who?\r\n")
		return nil
	}
	victim := c.findAnywhere(name)
	switch {
	case victim == nil:
		c.Send("No such character.\r\n")
		return nil
	case victim == c.Character:
		c.Send("Hee hee... we are jolly funny today, eh?\r\n")
		return nil
	case victim.Client != nil:
		c.Send("You can't do that, the body is already in use!\r\n")
		return nil
	case levelOf(c.Character) < game.LevelImplementor && !victim.IsNPC():
		c.Send("You aren't holy enough to use a mortal's body.\r\n")
		return nil
	}

	// The two room checks, skipped for a greater god — the same pair
	// find_target_room applies, because switching is a way of being
	// somewhere without going there.
	if levelOf(c.Character) < game.LevelGreaterGod {
		room := c.World.Room(victim.Room)
		switch {
		case room != nil && room.Flags.Has(game.RoomGodRoom):
			c.Send("You are not godly enough to use that room!\r\n")
			return nil
		case room != nil && room.Flags.Has(game.RoomHouse) &&
			!c.World.HouseCanEnter(c.Character, victim.Room):
			c.Send("That's private property -- no trespassing!\r\n")
			return nil
		}
	}

	c.Send("Okay.\r\n")
	// The god's own body is left with no connection, which is what makes it
	// switchable-into by somebody else and what `return` has to undo.
	c.Character.Client = nil
	c.Session.SwitchInto(victim)
	return nil
}

// doReturn is do_return (act.wizard.c:1206).
//
// Not a wizard command in the table — it is level 0, because a *mortal*
// switched into by a god needs some way of saying so. In practice it does
// nothing at all for anybody who is not switched, which is the C's behaviour
// exactly: the whole body is inside `if (ch->desc && ch->desc->original)`.
func doReturn(c *Context) error {
	if c.Session == nil || c.Session.Original() == nil {
		return nil
	}
	c.Send("You return to your original body.\r\n")
	c.Session.SwitchBack()
	return nil
}

// doDisconnect is do_dc (act.wizard.c:1663): close somebody's connection by
// its number.
func doDisconnect(c *Context) error {
	if c.Operator == nil {
		return nil
	}
	arg, _ := oneArgument(c.Arg)
	number := int(atoi(arg))
	if number == 0 {
		c.Send("Usage: DC <user number> (type USERS for a list)\r\n")
		return nil
	}

	sessions := c.Operator.Sessions()
	if number < 1 || number > len(sessions) {
		c.Send("No such connection.\r\n")
		return nil
	}
	target := sessions[number-1]

	if who := target.Character(); who != nil && levelOf(who) >= levelOf(c.Character) {
		c.Send("Umm.. maybe that's not such a good idea...\r\n")
		return nil
	}
	if target.Closed() {
		c.Send("They're already being disconnected.\r\n")
		return nil
	}

	target.Close()
	c.Send("Connection #%d closed.\r\n", number)
	return nil
}

// doWizlock is do_wizlock (act.wizard.c:1723): who may log in.
func doWizlock(c *Context) error {
	if c.Operator == nil {
		return nil
	}
	arg, _ := oneArgument(c.Arg)

	when := "currently"
	if arg != "" {
		value := atoi(arg)
		if value < 0 || value > levelOf(c.Character) {
			c.Send("Invalid wizlock value.\r\n")
			return nil
		}
		c.Operator.SetRestrict(value)
		when = "now"
	}

	switch level := c.Operator.Restrict(); level {
	case 0:
		c.Send("The game is %s completely open.\r\n", when)
	case 1:
		c.Send("The game is %s closed to new players.\r\n", when)
	default:
		c.Send("Only level %d and above may enter the game %s.\r\n", level, when)
	}
	return nil
}

// doShutdown is do_shutdown (act.wizard.c:1057).
//
// Five spellings, and in the C they differ by which file gets touched on the
// way out: the shell script that wraps the server reads them and decides
// whether to start it again. This port has no wrapper — the container runtime
// restarts it, see docs/operations.md — so `reboot` and `now` ask to come
// back and `die` and `pause` ask not to.
func doShutdown(c *Context) error {
	if c.Operator == nil {
		return nil
	}
	arg, _ := oneArgument(c.Arg)

	reboot := false
	message := "Shutting down.\r\n"
	switch strings.ToLower(arg) {
	case "":
	case "now":
		reboot, message = true, "Rebooting.. come back in a minute or two.\r\n"
	case "reboot":
		reboot, message = true, "Rebooting.. come back in a minute or two.\r\n"
	case "die", "pause":
		message = "Shutting down for maintenance.\r\n"
	default:
		c.Send("Unknown shutdown option.\r\n")
		return nil
	}

	for _, who := range c.World.Players() {
		who.Tell("%s", message)
	}
	c.Operator.Shutdown(reboot)
	return nil
}

// doDate is do_date with SCMD_DATE (act.wizard.c:1756).
func doDate(c *Context) error {
	c.Send("Current machine time: %s\r\n", clockTime(time.Now()))
	return nil
}

// doUptime is the same function with SCMD_UPTIME.
func doUptime(c *Context) error {
	if c.Operator == nil {
		return nil
	}
	booted := c.Operator.BootTime()
	up := time.Since(booted)

	days := int(up.Hours()) / 24
	plural := "s"
	if days == 1 {
		plural = ""
	}
	c.Send("Up since %s: %d day%s, %d:%02d\r\n",
		clockTime(booted), days, plural, int(up.Hours())%24, int(up.Minutes())%60)
	return nil
}

// clockTime is asctime's format with the trailing newline stripped, which the
// C does by hand: `*(tmstr + strlen(tmstr) - 1) = '\0'`.
func clockTime(t time.Time) string { return t.Format("Mon Jan _2 15:04:05 2006") }

// doLast is do_last (act.wizard.c:1787): when somebody was last on, read out
// of the roster rather than the world.
func doLast(c *Context) error {
	if c.Operator == nil {
		return nil
	}
	name, _ := oneArgument(c.Arg)
	if name == "" {
		c.Send("For whom do you wish to search?\r\n")
		return nil
	}

	entry, ok := c.Operator.LastLogin(name)
	if !ok {
		c.Send("There is no such player.\r\n")
		return nil
	}
	// An implementor sees everybody; anybody else is refused a character
	// above their own level.
	if entry.Level > levelOf(c.Character) && levelOf(c.Character) < game.LevelImplementor {
		c.Send("You are not sufficiently godly for that!\r\n")
		return nil
	}

	c.Send("[%5d] [%2d %s] %-12s : %-18s : %-20s\r\n",
		entry.IDNum, entry.Level, game.ClassAbbrevs[entry.Class],
		entry.Name, entry.Host, ctimeFormat(entry.When))
	return nil
}

// ctimeFormat is ctime(3), which unlike the rest of this file keeps its
// trailing newline — the C passes it straight into the sprintf and the line
// ends up double-spaced. Reproduced.
func ctimeFormat(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return fmt.Sprintf("%s\n", t.Format("Mon Jan _2 15:04:05 2006"))
}
