// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package session

import (
	"strings"

	"github.com/gerrowadat/disgracelands/internal/colour"
	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/obs"
)

// do_enter, do_leave and do_order, ported from act.movement.c:697, :731 and
// act.offensive.c:396.
//
// `enter` and `leave` are movement by *description* rather than by compass:
// walk in through the door, walk back out into the open. Neither takes a
// direction and both work by looking at the room flags of what is next door.

// doEnter is do_enter (act.movement.c:697).
func doEnter(c *Context) error {
	name, _ := oneArgument(c.Arg)

	if name != "" {
		// A named door. The C compares the *whole* keyword string with
		// str_cmp rather than matching one of its words, so a door whose
		// keyword is "gate portal" is entered by typing the whole of that and
		// not by typing "gate".
		for dir := game.Direction(0); int(dir) < game.NumDirections; dir++ {
			exit := c.World.Exit(c.Character.Room, dir)
			if exit != nil && exit.Keywords != "" && strings.EqualFold(exit.Keywords, name) {
				c.moveCharacterChecking(c.Character, dir, true)
				return nil
			}
		}
		c.Send("There is no %s here.\r\n", name)
		return nil
	}

	room := c.World.Room(c.Character.Room)
	if room != nil && room.Flags.Has(game.RoomIndoors) {
		c.Send("You are already indoors.\r\n")
		return nil
	}

	// The first open exit into somewhere indoors, in compass order — so
	// `enter` with two buildings next to you always picks the northerly one.
	for dir := game.Direction(0); int(dir) < game.NumDirections; dir++ {
		if dest := c.openExitTo(dir); dest != nil && dest.Flags.Has(game.RoomIndoors) {
			c.moveCharacterChecking(c.Character, dir, true)
			return nil
		}
	}
	c.Send("You can't seem to find anything to enter.\r\n")
	return nil
}

// doLeave is do_leave (act.movement.c:731): the mirror of enter with no
// argument.
func doLeave(c *Context) error {
	room := c.World.Room(c.Character.Room)
	if room == nil || !room.Flags.Has(game.RoomIndoors) {
		c.Send("You are outside.. where do you want to go?\r\n")
		return nil
	}

	for dir := game.Direction(0); int(dir) < game.NumDirections; dir++ {
		if dest := c.openExitTo(dir); dest != nil && !dest.Flags.Has(game.RoomIndoors) {
			c.moveCharacterChecking(c.Character, dir, true)
			return nil
		}
	}
	c.Send("I see no obvious exits to the outside.\r\n")
	return nil
}

// openExitTo returns the room through an exit, or nil when there is no exit,
// nowhere behind it, or the door is shut.
func (c *Context) openExitTo(dir game.Direction) *game.RoomDef {
	exit := c.World.Exit(c.Character.Room, dir)
	if exit == nil || exit.ToRoom == game.NoRoom || exit.State.Has(game.ExitClosed) {
		return nil
	}
	return c.World.Room(exit.ToRoom)
}

// doOrder is do_order (act.offensive.c:396).
//
// Telling a charmed follower what to do. Anybody can be *told*; only a
// charmed follower of yours actually does it, and everybody else in the room
// watches them look blank.
func doOrder(c *Context) error {
	name, message := halfChop(c.Arg)
	who := c.Character

	if name == "" || message == "" {
		c.Send("Order who to do what?\r\n")
		return nil
	}

	victim := c.World.FindInRoom(who, who.Room, name)
	toFollowers := isPrefixOf(name, "followers")
	if victim == nil && !toFollowers {
		c.Send("That person isn't here.\r\n")
		return nil
	}
	if victim == who {
		c.Send("You obviously suffer from skitzofrenia.\r\n")
		return nil
	}
	// Somebody else's puppet does not get to issue orders of their own.
	if who.Charmed() {
		c.Send("Your superior would not aprove of you giving orders.\r\n")
		return nil
	}

	if victim != nil {
		victim.Tell("%s orders you to '%s'\r\n", who.Name, message)
		c.announceExcept(victim, "%s gives %s an order.\r\n", who.Name, victim.Name)

		if victim.Master != who || !victim.Charmed() {
			// Note who this is sent about: the *victim*, to everyone
			// including the person who gave the order.
			for _, other := range c.World.Occupants(who.Room) {
				if other != victim {
					other.Tell("%s has an indifferent look.\r\n", victim.Name)
				}
			}
			return nil
		}
		c.Send("Okay.\r\n")
		c.runOrder(victim, message)
		return nil
	}

	// `order followers <something>`: everybody charmed and present.
	c.announce("%s issues the order '%s'.\r\n", who.Name, message)

	found := false
	for _, follower := range append([]*game.Character(nil), who.Followers...) {
		if follower.Room == who.Room && follower.Charmed() {
			found = true
			c.runOrder(follower, message)
		}
	}
	if found {
		c.Send("Okay.\r\n")
	} else {
		c.Send("Nobody here is a loyal subject of yours!\r\n")
	}
	return nil
}

// runOrder is command_interpreter(vict, message): the ordered character runs
// the command as if they had typed it.
//
// A mobile has no session, so the Context it gets has none either — which is
// exactly the case Context.Send already handles by writing to the character's
// client, and a mobile's client is nobody.
func (c *Context) runOrder(victim *game.Character, line string) {
	word, arg := split(line)
	cmd := lookup(word)
	if cmd == nil {
		victim.Tell("Huh?!?\r\n")
		return
	}
	ordered := *c
	ordered.Character = victim
	ordered.Session = nil
	ordered.Arg = arg
	ordered.Social = cmd.Social
	if err := cmd.Run(&ordered); err != nil {
		victim.Tell("Huh?!?\r\n")
	}
}

// hasBoat is has_boat (act.movement.c:52): whether a character may cross
// SECT_WATER_NOSWIM.
//
// Four ways to qualify, and two of them are easy to read wrongly.
//
// The level test is `GET_LEVEL(ch) > LVL_IMMORT` — **strictly greater**, so a
// plain level-31 immortal is *not* exempt and has to find a boat like anybody
// else, while a level-32 god walks on. Every other level gate in
// do_simple_move that lets gods past is `< LVL_IMMORT`, so this one is off by
// exactly one from its neighbours; see docs/weirdnumbers.md.
//
// The inventory test is `find_eq_pos(ch, obj, NULL) < 0`, which the C
// comments as "non-wearable boats in inventory". A boat that *can* be worn
// somewhere counts only when it is actually being worn: carrying it does
// nothing, and the following loop over the equipment is what picks it up
// again once it is on. So a wearable boat in a backpack leaves you standing
// at the water's edge holding the thing that would float you.
//
// Nothing here is guarded on IS_NPC, so a mobile needs a boat too — which is
// what keeps a wandering shopkeeper out of the sea.
func hasBoat(who *game.Character) bool {
	if who == nil {
		return false
	}
	if who.Level() > game.LevelImmortal {
		return true
	}
	if who.HasAffect(game.AffectWaterwalk) {
		return true
	}
	for _, obj := range who.Carrying {
		if obj != nil && obj.Type == game.ItemBoat && findWearPosition(obj) < 0 {
			return true
		}
	}
	for _, obj := range who.Equipment {
		if obj != nil && obj.Type == game.ItemBoat {
			return true
		}
	}
	return false
}

// deepWater is `SECT(room) == SECT_WATER_NOSWIM`, with the nil check the C
// does not need. A room this port could not find is not water: a broken exit
// is already refused a few lines earlier with its own message.
func deepWater(room *game.RoomDef) bool {
	return room != nil && room.SectorType == game.SectorWaterNoSwim
}

// Everything below this line moved here from commands.go in step 9 of
// docs/proposals/idiomatic-go.md — `move` and the checks a step goes
// through, which is what the rest of this file is already about. Code
// motion only: not a line changed.

// move returns the command for one direction.
func move(dir game.Direction) func(*Context) error {
	return func(c *Context) error {
		// do_move is the one caller that passes need_specials_check = 0
		// (act.movement.c:249). Everything else — enter, leave, following,
		// a mobile wandering — passes 1.
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
	return c.moveCharacterChecking(who, dir, false)
}

// moveCharacterChecking is moveCharacter with do_simple_move's
// need_specials_check argument, which reaches exactly one thing this port has
// ported: which of the two exhaustion messages a character gets. `do_move`
// passes 0 and everything else passes 1 (act.movement.c:249 against :233,
// :522, :535, :555), so somebody who walks into a wall of their own accord is
// "too exhausted" and somebody dragged after a leader is "too exhausted to
// follow" — and only if they have a master at all, which is the C's own
// second condition.
func (c *Context) moveCharacterChecking(who *game.Character, dir game.Direction, specialsCheck bool) bool {
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

	// The boat check (act.movement.c:112-119), which sits *before* the
	// movement cost is even computed — so being turned back at the water's
	// edge is free, and a character with no movement points left is told they
	// need a boat rather than that they are exhausted.
	//
	// The test is on *either* room being SECT_WATER_NOSWIM, not just the one
	// being entered: getting out of the water needs the boat you needed to
	// get in, which is what stops a boat being stolen from under someone
	// mid-crossing leaving them wading ashore.
	if deepWater(c.World.Room(who.Room)) || deepWater(c.World.Room(exit.ToRoom)) {
		if !hasBoat(who) {
			who.Tell("You need a boat to go there.\r\n")
			return false
		}
	}

	// need_movement (act.movement.c:127): the truncated average of the two
	// rooms' movement loss. Checked here, *before* the atrium check, because
	// that is the C's order — a player with no movement left standing in an
	// atrium is told they are exhausted rather than that the house is
	// private.
	//
	// The refusal is `!IS_NPC(ch)` only, so an immortal with no movement left
	// is refused too; what the level test guards is the *charge*
	// (act.movement.c:161), so an immortal never spends any and so never
	// reaches the refusal. A mobile is neither charged nor refused.
	cost := game.MovementCost(c.World.Room(who.Room), c.World.Room(exit.ToRoom))
	if !who.IsNPC() && who.Record != nil && who.Record.Points.Move < cost {
		if specialsCheck && who.Master != nil {
			who.Tell("You are too exhausted to follow.\r\n")
		} else {
			who.Tell("You are too exhausted.\r\n")
		}
		return false
	}

	// House_can_enter (act.movement.c:133). Note it is guarded by the room
	// you are *leaving* being an atrium, not by the room you are entering
	// being a house — so a house reachable any other way than through its
	// atrium is not guarded at all. That is why hcontrol insists the door be
	// two-way: it is the only door.
	if from := c.World.Room(who.Room); from != nil && from.Flags.Has(game.RoomAtrium) {
		if !c.World.HouseCanEnter(who, exit.ToRoom) {
			who.Tell("That's private property -- no trespassing!\r\n")
			return false
		}
	}

	// The tunnel cap (act.movement.c:139-146), which sits here: after the
	// atrium check and *before* the movement points are spent, so somebody
	// turned back at a full tunnel keeps the points the step would have
	// cost.
	//
	// Three things about it are easy to get wrong by reading it quickly:
	//
	//   - The flag is on the room being *entered*, not the one being left.
	//   - `num_pc_in_room` (utils.c:575) counts non-NPCs only, so a tunnel
	//     crowded with mobiles is empty as far as the cap is concerned.
	//   - There is no `IS_NPC` or level guard on the mover at all. A
	//     wandering mobile is refused, and so is an implementor — this is
	//     the one gate in do_simple_move that a god cannot simply walk
	//     through, and the only ways past it are `goto` and `teleport`,
	//     which do not come through here.
	//
	// The comparison is `>=`, so the cap is how many may be in the room, not
	// how many may already be there. Which of the two messages is used is
	// decided by the *setting* rather than by the room's occupancy, so a
	// server on the default 2 always says the first one.
	if to := c.World.Room(exit.ToRoom); to != nil && to.Flags.Has(game.RoomTunnel) {
		if size := game.Tuning().TunnelSize; c.World.PlayersInRoom(exit.ToRoom) >= size {
			if size > 1 {
				who.Tell("There isn't enough room for you to go there!\r\n")
			} else {
				who.Tell("There isn't enough room there for more than one person!\r\n")
			}
			return false
		}
	}

	// The godroom check (act.movement.c:147-151), the last gate before the
	// step is paid for.
	//
	// The level is `LVL_GRGOD`, not `LVL_IMMORT`, so a plain immortal is
	// refused as flatly as a mortal is — the flag is for the two top ranks.
	// And the wording is not the wording `goto` and `teleport` use for the
	// same flag: the C's movement message has the contraction
	// ("You aren't godly enough"), the two teleport sites spell it out
	// ("You are not godly enough"). Three call sites, two strings; the
	// difference is the C's, so the string here is deliberately not shared
	// with them.
	if to := c.World.Room(exit.ToRoom); to != nil &&
		to.Flags.Has(game.RoomGodRoom) && who.Level() < game.LevelGreaterGod {
		who.Tell("You aren't godly enough to use that room!\r\n")
		return false
	}

	// "Now we know we're allowed to go into the room." Mobiles and immortals
	// walk for nothing.
	if !who.IsNPC() && who.Record != nil && who.Level() < game.LevelImmortal {
		who.Record.Points.Move -= cost
	}

	leaving := who.Room
	if err := c.World.Enter(who, exit.ToRoom); err != nil {
		who.Tell("The way is blocked by something you cannot describe.\r\n")
		return false
	}
	// AFF_SNEAK suppresses both messages (act.movement.c:163-170). The C
	// tests the *mover's* flag and nothing else — there is no per-observer
	// roll, so `act()` is simply not called and the message is suppressed for
	// everyone in the room or for nobody. Sneaking past a watchful god works
	// exactly as well as sneaking past a sleeping rat.
	//
	// It conceals the movement, not the person: AFF_SNEAK is deliberately not
	// in INVIS_OK, so a sneaking character is still seen standing there by
	// anyone who looks.
	if !who.HasAffect(game.AffectSneak) {
		announce(c.World, leaving, who, "%s leaves %s.\r\n", who.Name, dir)
		announce(c.World, exit.ToRoom, who, "%s has arrived.\r\n", who.Name)
	}

	if room := c.World.Room(exit.ToRoom); room != nil {
		if who == c.Character {
			sendRoomInfo(c.Session, room)
		}
		who.Tell("%s", roomDescription(c.World, room, who, false))
	}

	// The death trap (act.movement.c:171-176), and note where it sits: after
	// the room description, so the last thing anybody sees is the room that
	// killed them. do_simple_move returns 0 from here, and perform_move takes
	// that as a failure and drags nobody after them (act.movement.c:202) — so
	// a group whose leader walks into a death trap loses exactly one member.
	if room := c.World.Room(who.Room); room != nil &&
		room.Flags.Has(game.RoomDeathTrap) && who.Level() < game.LevelImmortal {
		c.deathTrap(who, room)
		return false
	}

	c.moveFollowers(who, leaving, dir)
	return true
}

// deathTrap is log_death_trap (utils.c:164), death_cry() and extract_char()
// — the three lines do_simple_move ends on for a mortal standing in a
// ROOM_DEATH room (act.movement.c:172-175).
//
// It is deliberately *not* a death. Nothing here calls die() or raw_kill():
// there is no corpse, no experience is lost, the killer's alignment is not
// touched and no message says anybody died. The character is simply removed
// from the world with their things left lying in the room, saved, and put
// back at the menu — which on the real server is why a death trap costs
// everything you were carrying and nothing else.
func (c *Context) deathTrap(who *game.Character, room *game.RoomDef) {
	// mudlog(buf, BRF, LVL_IMMORT, TRUE) (utils.c:170). BRF, so it reaches
	// every immortal watching the syslog at all — a death trap is the one
	// event a god is most likely to be asked about afterwards. The vnum is
	// GET_ROOM_VNUM, printed with %d and so unpadded.
	c.wizlog(obs.LogBrief, game.LevelImmortal, "%s hit death trap #%d (%s)",
		who.Name, room.Vnum, room.Name)

	// The `<DoC>` half of log_death_trap (utils.c:172-173): a
	// send_to_all_color in cyan, so the whole game hears it and not just the
	// gods. Note the quoting, which is the C's verbatim — the room name is
	// wrapped in its own pair of single quotes *inside* the whisper's, and
	// the whisper's closing quote lands after the full stop.
	c.broadcastAt(game.AnnouncementRare, colour.Normal,
		"{{cyan}}A voice whispers in your ear, '%s has met their demise in the fatal death trap, '%s'.'{{/}}\r\n",
		who.Name, room.Name)

	c.World.DeathCry(who)

	// extract_char (handler.c:1007), whose player half this port already has
	// as the Extract seam — the same one `quit` uses, and for the same
	// reason: the connection stays open and the C's extract_char_final ends
	// at CON_MENU (handler.c:931).
	//
	// The belongings go on the floor *first*. That is the C's order
	// (handler.c:906-914 runs before the save at :938), and here it is also
	// what makes the crash file right: Extract snapshots what the character
	// is still holding, which by then is nothing, and writing an empty rent
	// file deletes it — Crash_delete_crashfile (handler.c:940) by another
	// name.
	c.World.DropEverything(who)

	if who.IsNPC() {
		// extract_char_final's mobile branch (handler.c:934): out of the
		// mobile list as well as the room, so the zone's population cap
		// frees up and the next reset can replace it. A wandering mobile
		// never walks into a death trap (mobact.c:104), but a charmed one
		// dragged in behind its master does.
		c.World.RemoveMobile(who)
		return
	}

	if c.Extract != nil {
		c.Extract(c.World, who)
	}
	// Reached through the Client interface rather than the session, because
	// the victim need not be whoever typed the direction: a follower dragged
	// into the trap is extracted with their own descriptor, and it is their
	// screen the menu belongs on. Same shape as `purge` (wizchange.go).
	if menu, ok := who.Client.(interface{ ReturnToMenu(string) }); ok {
		menu.ReturnToMenu(c.Text.Menu())
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
