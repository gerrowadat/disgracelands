// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

// The parts of dying that are pure world state, so that both ways into it can
// share them: the combat round's die() (internal/server/tick.go) and
// do_simple_move's death trap (internal/session, issue #209). A death trap is
// not a death in the C's sense — nothing calls die(), there is no corpse and
// no experience changes hands — but it does call death_cry() and it does
// scatter everything the victim owned, and those two are the same code.

// DeathCry is death_cry (fight.c:367): the room hears whose it was, and every
// room one step away hears that it was somebody.
//
// The neighbours are reached through CAN_GO, so a closed door muffles it —
// the same condition `exits` uses to decide what to list. Two exits leading
// to the same room send it there twice, which the C does not guard against
// and neither does this.
func (l *Live) DeathCry(c *Character) {
	for _, other := range l.Occupants(c.Room) {
		if other == c {
			continue
		}
		other.Tell("%s", l.Act("Your blood freezes as you hear $n's death cry.",
			ActArgs{Actor: c}, other))
	}

	room := l.Room(c.Room)
	if room == nil {
		return
	}
	for dir := Direction(0); dir < NumDirections; dir++ {
		exit := room.Exits[dir]
		if exit == nil || exit.ToRoom == NoRoom || exit.State.Has(ExitClosed) {
			continue
		}
		for _, other := range l.Occupants(exit.ToRoom) {
			other.Tell("Your blood freezes as you hear someone's death cry.\r\n")
		}
	}
}

// DropEverything empties a character's inventory and equipment onto the floor
// of the room they are standing in, porting the two loops in the middle of
// extract_char_final (handler.c:906-914).
//
// Inventory first, then equipment in wear-position order, which is the order
// the C does it in and therefore the order they lie in the room. Unlike
// MakeCorpse there is no container and no money object: the gold stays on the
// character's record, because the C only ever moves `ch->carrying` and
// `GET_EQ` here and never touches GET_GOLD.
//
// This is the difference between being killed and being extracted. A player
// who dies in a fight leaves a corpse with everything in it; a player who
// walks into a death trap leaves the things themselves, loose, in the room
// that killed them — and since the room is usually somewhere nobody can
// safely stand, that is effectively the game eating them.
func (l *Live) DropEverything(c *Character) {
	if c == nil {
		return
	}
	for _, o := range append([]*Object(nil), c.Carrying...) {
		l.ObjectToRoom(o, c.Room)
	}
	for pos := WearPosition(0); pos < NumWears; pos++ {
		if o := l.Unequip(c, pos); o != nil {
			l.ObjectToRoom(o, c.Room)
		}
	}
}
