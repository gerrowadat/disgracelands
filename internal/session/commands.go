// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package session

import (
	"context"
	"fmt"
	"strings"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// Command is one thing a player can type.
type Command struct {
	// Name is the full word. Players may type any unambiguous prefix of it.
	Name string
	// Help is the one-line description the command list shows.
	Help string
	// Run does the thing. It runs on the world goroutine, so it may touch
	// the world freely and must not block.
	Run func(cmd *Context) error
}

// Context is what a command is given.
type Context struct {
	Ctx       context.Context
	Session   *Session
	Character *game.Character
	World     *game.Live
	Text      TextFiles
	// Arg is everything after the command word, trimmed.
	Arg string
}

// Send writes to the player who typed the command.
func (c *Context) Send(format string, args ...any) { c.Session.Send(format, args...) }

// Commands is the command table.
//
// Order matters: the C server's interpreter matches the first entry whose
// name the typed word is a prefix of, so "n" is north and not "news" purely
// because of where they sit in the table. That behaviour is player-visible
// muscle memory — someone who has typed "n" for twenty years should not find
// it means something else — so the order here is deliberate and the movement
// commands come first.
// Populated in init() rather than as a literal: doHelp lists the table, so a
// literal would be an initialisation cycle.
var Commands []Command

func init() {
	Commands = []Command{
		{Name: "north", Help: "Move north.", Run: move(game.North)},
		{Name: "east", Help: "Move east.", Run: move(game.East)},
		{Name: "south", Help: "Move south.", Run: move(game.South)},
		{Name: "west", Help: "Move west.", Run: move(game.West)},
		{Name: "up", Help: "Move up.", Run: move(game.Up)},
		{Name: "down", Help: "Move down.", Run: move(game.Down)},

		{Name: "look", Help: "Look at the room around you.", Run: doLook},
		{Name: "kill", Help: "Attack someone.", Run: doKill},
		{Name: "who", Help: "List who is playing.", Run: doWho},
		{Name: "credits", Help: "Show the CircleMUD and DikuMUD credits.", Run: doCredits},
		{Name: "help", Help: "Show this list, or help on a topic.", Run: doHelp},
		{Name: "quit", Help: "Leave the game.", Run: doQuit},
	}
}

// Dispatcher runs commands against the world.
type Dispatcher struct {
	// Run submits a function to the world goroutine and waits for it. The
	// engine provides this; taking it as a function keeps this package from
	// importing the engine and the engine from importing this one.
	Run func(ctx context.Context, f func(*game.Live)) error
	// Text supplies the canned files.
	Text TextFiles
}

// Do implements CommandHandler.
func (d *Dispatcher) Do(ctx context.Context, s *Session, line string) error {
	word, arg := split(line)
	if word == "" {
		s.Send("%s", prompt(s))
		return nil
	}

	cmd := lookup(word)
	if cmd == nil {
		s.Send("Huh?!?\r\n%s", prompt(s))
		return nil
	}

	// The command itself runs on the world goroutine; everything it touches
	// is world state.
	return d.Run(ctx, func(w *game.Live) {
		c := &Context{
			Ctx: ctx, Session: s, Character: s.Character(),
			World: w, Text: d.Text, Arg: arg,
		}
		if err := cmd.Run(c); err != nil {
			s.logger.Error("command failed", "command", cmd.Name, "error", err)
			s.Send("Something went wrong doing that.\r\n")
		}
		if !s.Closed() {
			// The same numbers as the prompt, out of band, so a client does
			// not have to parse them back out of it.
			s.SendVitals(s.Character())
			s.Send("%s", prompt(s))
		}
	})
}

// lookup finds the first command the word is a prefix of, which is what the C
// interpreter does.
func lookup(word string) *Command {
	word = strings.ToLower(word)
	for i := range Commands {
		if strings.HasPrefix(Commands[i].Name, word) {
			return &Commands[i]
		}
	}
	return nil
}

// split separates the command word from its argument.
func split(line string) (word, arg string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return "", ""
	}
	word, arg, _ = strings.Cut(line, " ")
	return word, strings.TrimSpace(arg)
}

// prompt is what a player sees when the server is waiting for them, ported
// from make_prompt (comm.c:1043).
//
// Each part is shown only if the matching preference is set. A new character
// gets all three, because the local block at the class prompt turns them on;
// a converted one gets whatever they had turned on when they last played,
// which is the point of keeping the preference bits faithful.
func prompt(s *Session) string {
	c := s.Character()
	if c == nil || c.Record == nil {
		return "> "
	}
	rec := c.Record

	var b strings.Builder
	if rec.InvisLevel > 0 {
		fmt.Fprintf(&b, "i%d ", rec.InvisLevel)
	}
	if rec.Preferences.Has(game.PrefDisplayHP) {
		fmt.Fprintf(&b, "%dH ", rec.Points.Hit)
	}
	if rec.Preferences.Has(game.PrefDisplayMana) {
		fmt.Fprintf(&b, "%dM ", rec.Points.Mana)
	}
	if rec.Preferences.Has(game.PrefDisplayMove) {
		fmt.Fprintf(&b, "%dV ", rec.Points.Move)
	}
	b.WriteString("> ")
	return b.String()
}

func doLook(c *Context) error {
	room := c.World.Room(c.Character.Room)
	if room == nil {
		c.Send("You are nowhere at all. That should not be possible.\r\n")
		return nil
	}

	sendRoomInfo(c.Session, room)

	c.Send("%s\r\n", room.Name)
	if room.Description != "" {
		c.Send("%s", ensureNewline(room.Description))
	}

	if exits := exitList(room); exits != "" {
		c.Send("[ Exits: %s ]\r\n", exits)
	}

	for _, other := range c.World.Occupants(room.Vnum) {
		if other == c.Character {
			continue
		}
		c.Send("%s is standing here.\r\n", other.Name)
	}
	return nil
}

func exitList(room *game.RoomDef) string {
	return strings.Join(exitNames(room, 1), " ")
}

// exitNames lists the room's exits, truncated to width characters each. One
// character is what the `[ Exits: ]` line shows; GMCP wants the whole word.
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
	s.SendGMCP("Room.Info", RoomInfo{
		Vnum:  int32(room.Vnum),
		Name:  room.Name,
		Desc:  room.Description,
		Exits: exitNames(room, 0),
	})
}

// move returns the command for one direction.
func move(dir game.Direction) func(*Context) error {
	return func(c *Context) error {
		exit := c.World.Exit(c.Character.Room, dir)
		if exit == nil || exit.ToRoom == game.NoRoom {
			c.Send("Alas, you cannot go that way...\r\n")
			return nil
		}
		if c.World.Room(exit.ToRoom) == nil {
			// The loader reports these as warnings rather than refusing to
			// start, so a player can still walk into one.
			c.Send("The way is blocked by something you cannot describe.\r\n")
			return nil
		}

		leaving := c.Character.Room
		if err := c.World.Enter(c.Character, exit.ToRoom); err != nil {
			return err
		}
		announce(c.World, leaving, c.Character, "%s leaves %s.\r\n", c.Character.Name, dir)
		announce(c.World, exit.ToRoom, c.Character, "%s has arrived.\r\n", c.Character.Name)

		return doLook(c)
	}
}

// announce tells everyone in a room something, except the character it is
// about.
func announce(w *game.Live, room game.RoomVnum, except *game.Character, format string, args ...any) {
	for _, c := range w.Occupants(room) {
		if c != except {
			c.Tell(format, args...)
		}
	}
}

// doKill starts a fight, porting do_kill (act.offensive.c) as far as this
// phase goes: the immortal's instant-slay branch and the charmed-follower
// check arrive with the rest of act.offensive.
func doKill(c *Context) error {
	if c.Arg == "" {
		c.Send("Kill who?\r\n")
		return nil
	}

	victim := c.World.FindInRoom(c.Character.Room, c.Arg)
	if victim == nil {
		c.Send("They aren't here.\r\n")
		return nil
	}
	if victim == c.Character {
		c.Send("Your mother would be so sad... :(\r\n")
		return nil
	}

	room := c.World.Room(c.Character.Room)
	if room != nil && room.Flags.Has(game.RoomPeaceful) {
		c.Send("This room just has such a peaceful, easy feeling...\r\n")
		return nil
	}

	if c.Character.Fighting != nil {
		c.Send("You are already fighting %s.\r\n", c.Character.Fighting.Name)
		return nil
	}

	c.Send("You attack %s!\r\n", victim.Name)
	victim.Tell("%s attacks you!\r\n", c.Character.Name)
	for _, other := range c.World.Occupants(c.Character.Room) {
		if other != c.Character && other != victim {
			other.Tell("%s attacks %s!\r\n", c.Character.Name, victim.Name)
		}
	}

	c.World.SetFighting(c.Character, victim)
	return nil
}

func doWho(c *Context) error {
	players := c.World.Players()
	c.Send("Players\r\n-------\r\n")
	for _, p := range players {
		title := p.Title()
		if title != "" {
			title = " " + title
		}
		c.Send("[%3d] %s%s\r\n", p.Level(), p.Name, title)
	}
	c.Send("\r\n%d character%s playing.\r\n", len(players), plural(len(players)))
	return nil
}

// doCredits shows the credits file.
//
// This is licence compliance, not a feature. The CircleMUD licence requires
// that the text in the credits file be preserved and displayed when the
// `credits` command is used (docs/proposals/go-port-plan.md §12). It is in
// the first set of commands implemented for exactly that reason.
func doCredits(c *Context) error {
	c.Send("%s", ensureNewline(c.Text.Credits()))
	return nil
}

// doHelp lists the commands, and answers `help circlemud`.
//
// The CIRCLEMUD help entry is also a licence requirement and must be shown
// intact. Until the help database is loaded (a later phase), it is served
// from the credits file, which carries the same attribution and is required
// to be intact for the same reason.
func doHelp(c *Context) error {
	if strings.EqualFold(c.Arg, "circlemud") {
		c.Send("%s", ensureNewline(c.Text.Credits()))
		return nil
	}
	if c.Arg != "" {
		c.Send("There is no help on that yet.\r\n")
		return nil
	}

	c.Send("Commands\r\n--------\r\n")
	for _, cmd := range Commands {
		c.Send("  %-10s %s\r\n", cmd.Name, cmd.Help)
	}
	c.Send("\r\nType `help circlemud` for the credits.\r\n")
	return nil
}

func doQuit(c *Context) error {
	c.Send("Goodbye, friend.. Come back soon!\r\n")
	c.Session.MarkQuit()
	c.Session.Close()
	return nil
}

func ensureNewline(s string) string {
	if s == "" || strings.HasSuffix(s, "\n") {
		return s
	}
	return s + "\r\n"
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
