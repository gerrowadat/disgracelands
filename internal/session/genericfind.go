// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package session

import "github.com/gerrowadat/disgracelands/internal/game"

// generic_find (handler.c:1373), the C's one search that looks in several
// places at once.
//
// Every command that can mean "the thing in my hand, or the thing on the
// floor, or the person standing there" goes through it: `put`, `get X from
// Y`, `open`, `look`, `examine`, and a wand or staff aimed at something. The
// caller names which lists it will accept with a bitvector; the function
// answers which one it found the match in.
//
// Two things about it are the C's rather than a design choice here, and both
// are things a player can see.
//
// **The search order is fixed** — characters in the room, characters in the
// world, worn, carried, on the floor, anywhere at all — and does not depend
// on the order the bits are named at the call site. `look sword` shows the
// one in your hand rather than the one on the ground because of this, not
// because `look` asked for it that way round.
//
// **The count is shared across every list it walks.** The C threads one
// `int *number` down the whole chain (handler.c:1387), so `2.sword` means the
// second match across the *search order*, not the second in whichever list
// happens to hold it. With one sword worn and one carried, `2.sword` is the
// carried one. Searching each list with a fresh counter — which is what this
// port did everywhere except `look` — finds the worn one twice, and a player
// disambiguating by number gets a different answer to the same question
// depending only on which command they typed. That is issue #194.

// findBits is generic_find's bitvector (handler.c's FIND_* in structs.h).
type findBits uint

const (
	findCharRoom findBits = 1 << iota
	findCharWorld
	findObjInv
	findObjRoom
	findObjEquip
	findObjWorld
)

// findWhere is generic_find's return value: which list the match came from.
//
// look_in_obj is what needs the object half of it — a container's contents
// are headed with the container's own name and then "(carried)", "(here)" or
// "(used)", chosen by exactly this (act.informative.c:517-527).
type findWhere int

const (
	foundNowhere findWhere = iota
	foundCharInRoom
	foundCharInWorld
	foundInEquipment
	foundInInventory
	foundInRoom
	foundInWorld
)

// genericFind is generic_find itself.
func (c *Context) genericFind(arg string, bits findBits) (*game.Character, *game.Object, findWhere) {
	// one_argument first: generic_find searches for the *first word* of what
	// it is given and ignores the rest, which is why `open red door` looks
	// for "red" and not for "red door".
	word, _ := oneArgument(arg)
	if word == "" {
		return nil, nil, foundNowhere
	}

	s := c.World.NewSearch(c.Character, word)
	if s.Word == "" {
		return nil, nil, foundNowhere
	}
	// `if (!(number = get_number(&name))) return (0);` (handler.c:1387).
	// A leading `0.` stops the whole search here rather than meaning what it
	// means to a bare character lookup — where `0.name` is "the player of
	// that name". Nothing reachable through generic_find has that shortcut.
	if s.Number() == 0 {
		return nil, nil, foundNowhere
	}

	if bits&findCharRoom != 0 {
		if who := c.World.SearchInRoom(s, c.Character, c.Character.Room); who != nil {
			return who, nil, foundCharInRoom
		}
	}
	if bits&findCharWorld != 0 {
		if who := c.World.SearchAnywhere(s, c.Character); who != nil {
			return who, nil, foundCharInWorld
		}
	}
	if bits&findObjEquip != 0 {
		// Note this one is generic_find's own loop and not
		// get_obj_in_equip_vis: it does not check CAN_SEE_OBJ. See
		// Search.EquippedObject.
		if obj := s.EquippedObject(&c.Character.Equipment); obj != nil {
			return nil, obj, foundInEquipment
		}
	}
	if bits&findObjInv != 0 {
		if obj := s.ObjectIn(c.Character.Carrying); obj != nil {
			return nil, obj, foundInInventory
		}
	}
	if bits&findObjRoom != 0 {
		if obj := s.ObjectIn(c.World.RoomObjects(c.Character.Room)); obj != nil {
			return nil, obj, foundInRoom
		}
	}
	if bits&findObjWorld != 0 {
		if obj := s.ObjectIn(c.World.Objects()); obj != nil {
			return nil, obj, foundInWorld
		}
	}
	return nil, nil, foundNowhere
}
