// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import "time"

// Player housing, ported from house.c.
//
// A house is a room with two flags on it and a record in a control file
// saying who owns it. The room's door must lead to an "atrium" and back
// again, which is the only structural rule: a house is a dead end with one
// two-way exit, so there is exactly one door to guard.

// MaxGuests is MAX_GUESTS (house.h:2). MAX_HOUSES is enforced by the store.
const MaxGuests = 10

// HouseModePrivate is HOUSE_PRIVATE, and the only mode. `House_can_enter`
// switches on the field and has one case; nothing ever set it to anything
// else.
const HouseModePrivate int32 = 0

// House is one entry of the control file, as the world holds it.
type House struct {
	Vnum    RoomVnum
	Atrium  RoomVnum
	ExitNum Direction

	BuiltOn     time.Time
	LastPayment time.Time

	Mode  int32
	Owner int64
	// Guests are the id numbers the owner has let in.
	Guests []int64
}

// SetHouses installs the loaded houses. Called once at boot.
func (l *Live) SetHouses(houses []*House) { l.houses = houses }

// Houses returns every house.
func (l *Live) Houses() []*House { return l.houses }

// FindHouse is find_house (house.c:212).
func (l *Live) FindHouse(vnum RoomVnum) *House {
	for _, h := range l.houses {
		if h.Vnum == vnum {
			return h
		}
	}
	return nil
}

// AddHouse appends a house and flags its rooms.
func (l *Live) AddHouse(h *House) {
	l.houses = append(l.houses, h)
	l.flagHouseRooms(h)
}

// RemoveHouse drops a house and clears its flags, porting the middle of
// hcontrol_destroy_house.
func (l *Live) RemoveHouse(h *House) {
	if room := l.Room(h.Atrium); room != nil {
		room.Flags = room.Flags.Without(RoomAtrium)
	}
	if room := l.Room(h.Vnum); room != nil {
		room.Flags = room.Flags.Without(RoomHouse, RoomPrivate, RoomHouseCrash)
	}
	for i, other := range l.houses {
		if other == h {
			l.houses = append(l.houses[:i], l.houses[i+1:]...)
			break
		}
	}
	// "Now, reset the ROOM_ATRIUM flag on all existing houses' atriums, just
	// in case the house we just deleted shared an atrium with another house."
	// The C's comment, dated 9/19/94, and the only reason this loop exists.
	for _, other := range l.houses {
		if room := l.Room(other.Atrium); room != nil {
			room.Flags = room.Flags.With(RoomAtrium)
		}
	}
}

// flagHouseRooms sets ROOM_HOUSE|ROOM_PRIVATE on the house and ROOM_ATRIUM on
// its atrium.
func (l *Live) flagHouseRooms(h *House) {
	if room := l.Room(h.Vnum); room != nil {
		room.Flags = room.Flags.With(RoomHouse, RoomPrivate)
	}
	if room := l.Room(h.Atrium); room != nil {
		room.Flags = room.Flags.With(RoomAtrium)
	}
}

// MarkHouseChanged sets ROOM_HOUSE_CRASH, which is what tells the periodic
// save that this house has something new in it.
func (l *Live) MarkHouseChanged(vnum RoomVnum) {
	if room := l.Room(vnum); room != nil && room.Flags.Has(RoomHouse) {
		room.Flags = room.Flags.With(RoomHouseCrash)
	}
}

// HouseCanEnter is House_can_enter (house.c:583).
//
// A greater god walks into anything, and a room that is not a house is not
// guarded at all — note the order, which means a *lesser* god is kept out of
// somebody's house like anybody else.
func (l *Live) HouseCanEnter(c *Character, vnum RoomVnum) bool {
	if c == nil || c.Record == nil {
		return true
	}
	if c.Record.Level >= LevelGreaterGod {
		return true
	}
	h := l.FindHouse(vnum)
	if h == nil {
		return true
	}
	if h.Mode != HouseModePrivate {
		return false
	}
	if c.Record.IDNum == h.Owner {
		return true
	}
	for _, guest := range h.Guests {
		if c.Record.IDNum == guest {
			return true
		}
	}
	return false
}

// AddGuest adds an id to the guest list, reporting whether there was room.
func (h *House) AddGuest(id int64) bool {
	if len(h.Guests) >= MaxGuests {
		return false
	}
	h.Guests = append(h.Guests, id)
	return true
}

// RemoveGuest drops an id from the guest list, reporting whether it was
// there.
//
// The C's version of this loop reads one past the end of the live entries
// (house.c:551) — `for (; j < num_of_guests; j++) guests[j] = guests[j+1];`
// touches guests[num_of_guests], which with a full list of ten is
// guests[10] and therefore the first bytes of last_payment. It has never
// mattered because the value is immediately overwritten by the decrement,
// but it is a genuine out-of-bounds read and there is no reason to reproduce
// it. See docs/deviations.md.
func (h *House) RemoveGuest(id int64) bool {
	for i, guest := range h.Guests {
		if guest == id {
			h.Guests = append(h.Guests[:i], h.Guests[i+1:]...)
			return true
		}
	}
	return false
}
