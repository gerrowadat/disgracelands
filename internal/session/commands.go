// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package session

import (
	"context"
	"fmt"
	"runtime/debug"
	"strings"
	"time"

	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/rng"
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
	// RNG is the game's generator, for commands that roll — fleeing picks a
	// random direction.
	RNG *rng.Rand
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
		{Name: "kick", Help: "Kick someone.", Run: doKick},
		// backstab before bash, which is the C's order (interpreter.c:235 and
		// :238) and therefore what `ba` means.
		{Name: "backstab", Help: "Stab someone who is not looking.", Run: doBackstab},
		{Name: "bash", Help: "Knock someone over.", Run: doBash},
		{Name: "cast", Help: "Cast a spell: cast 'magic missile' <target>.", Run: doCast},
		{Name: "flee", Help: "Run away from a fight.", Run: doFlee},
		// `fill` is at interpreter.c:296, ahead of flee — but a mortal's `f`
		// reaches neither in the C: the lookup skips commands above their
		// level, so `force` (:294) is passed over and `fart` (:295) wins.
		// Socials are not ported, so no ordering here reproduces that. Flee
		// keeps `f` until they are.
		{Name: "fill", Help: "Fill a container from a fountain.", Run: doFill},
		// After fill (interpreter.c:296 and :300), so `fo` is follow and `fi`
		// is fill.
		{Name: "follow", Help: "Follow somebody, or `follow self` to stop.", Run: doFollow},

		{Name: "stand", Help: "Get to your feet.", Run: doStand},
		// sip before sit is the C's order (interpreter.c:467 and :468), so
		// `si` takes a drink and sitting down needs the whole word.
		{Name: "sip", Help: "Take a small drink.", Run: doSip},
		{Name: "sit", Help: "Sit down.", Run: doSit},
		{Name: "rest", Help: "Rest, to recover faster.", Run: doRest},
		{Name: "sleep", Help: "Sleep, to recover faster still.", Run: doSleep},
		// After rest, as in the C (interpreter.c:426 and :441), so `res` is
		// still rest and only `resc` reaches rescue.
		{Name: "rescue", Help: "Take somebody else's fight onto yourself.", Run: doRescue},
		{Name: "wake", Help: "Wake up, or wake somebody else.", Run: doWake},

		// Before `pick`, `pour` and `practice`, which is the C's order
		// (interpreter.c:396, :401, :408 and :411) and therefore what a bare
		// `p` means: put.
		{Name: "put", Help: "Put something into a container.", Run: doPut},

		{Name: "practice", Help: "Practise a spell or skill, or list what you know.", Run: doPractice},
		{Name: "score", Help: "Show your own statistics.", Run: doScore},
		{Name: "exits", Help: "List the ways out.", Run: doExits},

		{Name: "open", Help: "Open a door.", Run: doOpen},
		{Name: "close", Help: "Close a door.", Run: doClose},
		{Name: "lock", Help: "Lock a door.", Run: doLock},
		{Name: "unlock", Help: "Unlock a door.", Run: doUnlock},
		{Name: "pick", Help: "Pick a lock.", Run: doPick},

		{Name: "get", Help: "Pick something up.", Run: doGet},
		// After get, as in the C (interpreter.c:307 and :310), so `g` is get
		// and giving needs `gi`.
		{Name: "give", Help: "Give something to somebody.", Run: doGive},
		// drink before drop, which is the C's order (interpreter.c:279 and
		// :280) — so `dr` is drink and only `dro` is drop.
		{Name: "drink", Help: "Drink from something.", Run: doDrink},
		{Name: "drop", Help: "Put something down.", Run: doDrop},
		{Name: "eat", Help: "Eat something.", Run: doEat},
		{Name: "inventory", Help: "List what you are carrying.", Run: doInventory},
		{Name: "equipment", Help: "List what you are wearing.", Run: doEquipment},
		{Name: "wear", Help: "Put something on.", Run: doWear},
		{Name: "wield", Help: "Take a weapon in hand.", Run: doWield},
		{Name: "remove", Help: "Take something off.", Run: doRemove},
		// group before grab, which is the C's order (interpreter.c:316 and
		// :317) and therefore what `gr` means.
		{Name: "group", Help: "List your group, or enrol somebody in it.", Run: doGroup},
		{Name: "grab", Help: "Take something in your hands.", Run: doGrab},

		// After `exits`, `close` and `wear`, which is the C's order — so `ex`
		// is exits, `co` is close and `wea` is wear, and only the longer
		// forms reach these.
		{Name: "examine", Help: "Look closely at something.", Run: doExamine},
		{Name: "consider", Help: "Size somebody up.", Run: doConsider},
		{Name: "time", Help: "Ask what time it is.", Run: doTime},
		{Name: "weather", Help: "Ask what the weather is doing.", Run: doWeather},
		{Name: "pour", Help: "Empty a container.", Run: doPour},
		{Name: "taste", Help: "Take a small bite.", Run: doTaste},
		{Name: "who", Help: "List who is playing.", Run: doWho},
		{Name: "credits", Help: "Show the CircleMUD and DikuMUD credits.", Run: doCredits},
		{Name: "help", Help: "Show this list, or help on a topic.", Run: doHelp},
		// The other spelling of `grab`, and it goes after `help` because the
		// C has it there (interpreter.c:328 and :334) — so `h` is help and
		// holding something needs `ho`.
		{Name: "hold", Help: "Take something in your hands.", Run: doGrab},
		{Name: "quit", Help: "Leave the game.", Run: doQuit},
		// Nowhere near the others: the C has it among the u's, after unlock
		// (interpreter.c:522 and :523), so `un` is unlock and ungrouping
		// needs `ung`.
		{Name: "ungroup", Help: "Disband your group, or expel somebody.", Run: doUngroup},
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
	// RNG is the game's generator.
	RNG *rng.Rand
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

	// A character with a wait state does not act yet. The C stops reading
	// that descriptor's input until the wait runs down, so the command is
	// *delayed* rather than refused — a player who types `kick` twice gets
	// two kicks, slowly. Sleeping here has the same effect, since this
	// goroutine is the one reading their input.
	if c := s.Character(); c != nil {
		if remaining := c.WaitRemaining(); remaining > 0 {
			select {
			case <-time.After(remaining):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}

	// The command itself runs on the world goroutine; everything it touches
	// is world state.
	return d.Run(ctx, func(w *game.Live) {
		c := &Context{
			Ctx: ctx, Session: s, Character: s.Character(),
			World: w, Text: d.Text, RNG: d.RNG, Arg: arg,
		}

		// A command that panics must not leave the player staring at a dead
		// terminal. The engine contains the panic and logs it, but it does so
		// by abandoning the rest of this function — including the prompt — so
		// the player sees nothing at all and cannot tell the difference
		// between a broken command and a hung server. Recovering here means
		// they get told, and get their prompt back.
		func() {
			defer func() {
				if r := recover(); r != nil {
					s.logger.Error("command panicked",
						"command", cmd.Name, "panic", r, "stack", string(debug.Stack()))
					s.Send("Something went wrong doing that.\r\n")
				}
			}()
			if err := cmd.Run(c); err != nil {
				s.logger.Error("command failed", "command", cmd.Name, "error", err)
				s.Send("Something went wrong doing that.\r\n")
			}
		}()
		if !s.Closed() {
			// The same numbers as the prompt, out of band, so a client does
			// not have to parse them back out of it.
			s.SendVitals(s.Character())
			s.Send("%s", prompt(s))
		}
	})
}

// Lookup finds the first command the word is a prefix of, which is what the C
// interpreter does — so the order of the table is player-visible muscle
// memory and worth asserting from outside this package.
func Lookup(word string) *Command { return lookup(word) }

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
	// `look <something>` describes one thing; bare `look` describes the room.
	if arg := strings.TrimSpace(c.Arg); arg != "" {
		// `look in <container>` and `look at <thing>` are both the C's, and
		// both mean "describe that".
		arg = strings.TrimPrefix(arg, "at ")
		arg = strings.TrimPrefix(arg, "in ")
		return c.lookAtTarget(arg)
	}

	room := c.World.Room(c.Character.Room)
	if room == nil {
		c.Send("You are nowhere at all. That should not be possible.\r\n")
		return nil
	}

	sendRoomInfo(c.Session, room)
	c.Send("%s", roomDescription(c.World, room, c.Character))
	return nil
}

// roomDescription is look_at_room's text: the name, the description, the way
// out, what is lying about and who is here.
//
// It takes the viewer so it can leave them out of the list of people, and it
// returns a string rather than sending, because a spell can move somebody
// else into a room and has to show it to them rather than to the caster.
func roomDescription(w *game.Live, room *game.RoomDef, viewer *game.Character) string {
	var b strings.Builder

	fmt.Fprintf(&b, "%s\r\n", room.Name)
	if room.Description != "" {
		b.WriteString(ensureNewline(room.Description))
	}
	if exits := exitList(room); exits != "" {
		fmt.Fprintf(&b, "[ Exits: %s ]\r\n", exits)
	}

	for _, obj := range w.RoomObjects(room.Vnum) {
		if obj.Description != "" {
			fmt.Fprintf(&b, "%s\r\n", obj.Description)
			continue
		}
		fmt.Fprintf(&b, "%s is lying here.\r\n", capitaliseFirst(obj.Name()))
	}

	for _, other := range w.Occupants(room.Vnum) {
		if other == viewer {
			continue
		}
		// A mobile has a long description written for exactly this line; a
		// player does not, so they get the generic one.
		if other.MobDef != nil && other.MobDef.LongDesc != "" {
			b.WriteString(ensureNewline(other.MobDef.LongDesc))
			continue
		}
		fmt.Fprintf(&b, "%s is standing here.\r\n", other.Name)
	}
	return b.String()
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
		c.moveCharacter(c.Character, dir)
		return nil
	}
}

// moveCharacter walks one character one step, porting perform_move and
// do_simple_move.
//
// It takes the character rather than working on the session's own, because
// the last thing it does is move everybody who was following them — and each
// of those has to see the room they arrive in, told to them rather than to
// whoever gave the order. The recursion is the C's, and it terminates because
// following in loops is refused when the link is made.
func (c *Context) moveCharacter(who *game.Character, dir game.Direction) bool {
	exit := c.World.Exit(who.Room, dir)
	if exit == nil || exit.ToRoom == game.NoRoom {
		who.Tell("Alas, you cannot go that way...\r\n")
		return false
	}
	// A closed door stops a player, as it already stopped a mobile. The C
	// names the door if it has a keyword, which is how a player knows what to
	// open.
	if exit.State.Has(game.ExitClosed) {
		if name := doorName(exit); name != "door" {
			who.Tell("The %s seems to be closed.\r\n", name)
		} else {
			who.Tell("It seems to be closed.\r\n")
		}
		return false
	}
	if c.World.Room(exit.ToRoom) == nil {
		// The loader reports these as warnings rather than refusing to start,
		// so a player can still walk into one.
		who.Tell("The way is blocked by something you cannot describe.\r\n")
		return false
	}

	leaving := who.Room
	if err := c.World.Enter(who, exit.ToRoom); err != nil {
		who.Tell("The way is blocked by something you cannot describe.\r\n")
		return false
	}
	announce(c.World, leaving, who, "%s leaves %s.\r\n", who.Name, dir)
	announce(c.World, exit.ToRoom, who, "%s has arrived.\r\n", who.Name)

	if room := c.World.Room(exit.ToRoom); room != nil {
		if who == c.Character {
			sendRoomInfo(c.Session, room)
		}
		who.Tell("%s", roomDescription(c.World, room, who))
	}

	c.moveFollowers(who, leaving, dir)
	return true
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
