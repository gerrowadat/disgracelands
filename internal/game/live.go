// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Live is the running world: the loaded prototypes plus who is standing
// where.
//
// It is not safe for concurrent use, deliberately. One goroutine owns it and
// everything that touches it arrives as a command on that goroutine's queue —
// see internal/engine and docs/proposals/go-port-plan.md §3.1. That is what
// lets rooms, characters and objects reference each other directly without a
// lock on every field, and it is the reason a faithful port of the C code's
// structure is tractable at all.
type Live struct {
	defs *World

	rooms     map[RoomVnum]*RoomDef
	occupants map[RoomVnum][]*Character
	byName    map[string]*Character

	// objectDefs indexes the prototypes by vnum, so instantiating something
	// does not walk the list.
	objectDefs map[ObjVnum]*ObjDef
	// objects is every object that exists, by id. The C calls this
	// object_list and walks it every tick to decay corpses.
	objects map[uint64]*Object
	// roomObjects is what is lying on the floor of each room.
	roomObjects map[RoomVnum][]*Object
	// shops is the per-shop runtime state — the bank balance and how much of
	// the keeper's inventory is known to be sorted. The C keeps both in
	// shop_index beside the file data; kept apart here so the prototypes
	// stay read-only.
	shops map[ShopVnum]*shopState
	// boards is the bulletin boards, loaded once at boot. The C keeps them
	// in file-scope arrays in boards.c and loads them lazily, the first time
	// anybody looks at one.
	boards []*Board

	nextObjectID uint64

	// fighting is everyone currently in combat — the C's combat_list.
	fighting     map[*Character]bool
	nextFightSeq uint64

	// booted is when the world came up. Mud time is measured from it, as the
	// C measures from the boot time it writes to lib/etc/time.
	booted time.Time

	// mobileDefs indexes the mobile prototypes by vnum.
	mobileDefs map[MobVnum]*MobDef
	// mobiles is every mobile instance in the world, which is what the zone
	// population caps are counted against.
	mobiles map[*Character]bool
}

// MudTime is the current moment on the mud calendar.
func (l *Live) MudTime() MudTime { return TimePassed(time.Since(l.booted)) }

// ObjectDef returns an object prototype, or nil.
func (l *Live) ObjectDef(v ObjVnum) *ObjDef { return l.objectDefs[v] }

// MobileDef returns a mobile prototype, or nil.
func (l *Live) MobileDef(v MobVnum) *MobDef { return l.mobileDefs[v] }

// NewLive indexes a loaded world for play.
func NewLive(defs *World) *Live {
	l := &Live{
		defs:        defs,
		rooms:       make(map[RoomVnum]*RoomDef, len(defs.Rooms)),
		occupants:   make(map[RoomVnum][]*Character),
		byName:      make(map[string]*Character),
		objectDefs:  make(map[ObjVnum]*ObjDef, len(defs.Objects)),
		mobileDefs:  make(map[MobVnum]*MobDef, len(defs.Mobiles)),
		mobiles:     make(map[*Character]bool),
		booted:      time.Now(),
		objects:     make(map[uint64]*Object),
		roomObjects: make(map[RoomVnum][]*Object),
	}
	for _, r := range defs.Rooms {
		l.rooms[r.Vnum] = r
		// The C maps DoorFlag to exit_info at load; the loader here keeps the
		// raw value so a writer can round-trip it, so the mapping happens as
		// the world goes live instead.
		for _, e := range r.Exits {
			if e != nil {
				e.State = DoorState(e.DoorFlag)
			}
		}
	}
	for _, o := range defs.Objects {
		l.objectDefs[o.Vnum] = o
	}
	for _, m := range defs.Mobiles {
		l.mobileDefs[m.Vnum] = m
	}
	return l
}

// Room returns a room prototype, or nil.
func (l *Live) Room(v RoomVnum) *RoomDef { return l.rooms[v] }

// RoomAt returns the i'th room in load order, which is the C's *rnum* — the
// index `number(0, top_of_world)` picks from when teleport chooses somewhere
// at random. Nil if i is out of range.
func (l *Live) RoomAt(i int) *RoomDef {
	if l.defs == nil || i < 0 || i >= len(l.defs.Rooms) {
		return nil
	}
	return l.defs.Rooms[i]
}

// RoomCount is how many rooms the world has.
func (l *Live) RoomCount() int { return len(l.rooms) }

// Character is someone in the world.
//
// For now that means a logged-in player; mobiles arrive with Phase 4. The
// saved record is kept alongside so a save is a matter of writing it back
// rather than reconstructing it.
type Character struct {
	// Name is the character's name with its original capitalisation.
	Name string
	// Record is the saved character. Fields the game changes are written
	// through to it, so saving needs no gathering step.
	Record *PlayerRecord
	// Room is where they are.
	Room RoomVnum
	// Position is what they are doing: standing, fighting, sleeping, dying.
	// It lives here rather than in the record because it is not saved — the
	// C stores POS_STANDING for everyone on load and lets update_pos sort it
	// out.
	Position Position
	// NPC marks a mobile. The C tests MOB_ISNPC on the same flags field as
	// everything else; here it is a field of its own, because a player and a
	// mobile differ in enough places that a bit hidden in a flag word is a
	// trap.
	NPC bool
	// Carrying is their inventory, in the order things were picked up.
	Carrying []*Object
	// Equipment is what they are wearing, indexed by WearPosition.
	Equipment [NumWears]*Object
	// Keywords are what a player types to refer to a mobile. Empty for a
	// player, who is referred to by name.
	Keywords string
	// MobDef is the prototype a mobile was made from, or nil for a player.
	MobDef *MobDef
	// BusyUntil is when they may act again — the C's WAIT_STATE, which sets
	// ch->wait and stops game_loop reading that descriptor's input until it
	// runs down. Skills set it: kicking costs three combat rounds of lag,
	// bashing two.
	//
	// The C defers the queued command rather than refusing it, and so does
	// this: the dispatcher waits rather than sending "you are still
	// recovering", because a player who types `kick` twice expects two kicks,
	// slowly.
	BusyUntil time.Time
	// Fighting is who they are attacking, or nil.
	Fighting *Character

	// Master is who they are following, and Followers is who follows them.
	// Runtime only: the C keeps both on char_data and neither is saved, so a
	// group does not survive a reboot. See follow.go.
	Master    *Character
	Followers []*Character
	// fightSeq orders the combat round. Assigned when a fight starts, so a
	// round happens in the order people joined it rather than in map order.
	fightSeq uint64
	// Client is whoever is controlling this character, or nil for one that
	// nobody is — a mobile, or a player whose connection has dropped but
	// whose body is still standing there.
	//
	// An interface rather than a session, so the world never depends on the
	// network: a test drives a character with a buffer, and a mobile has no
	// client at all.
	Client Client
}

// Client is how the world speaks to whoever controls a character.
type Client interface {
	// Send delivers a message. It must not block: it is called from the
	// world goroutine, which everyone else is waiting on.
	Send(format string, args ...any)
}

// IsNPC reports whether this is a mobile, porting IS_NPC.
func (c *Character) IsNPC() bool { return c != nil && c.NPC }

// Tell sends to a character's client, if it has one.
func (c *Character) Tell(format string, args ...any) {
	if c.Client != nil {
		c.Client.Send(format, args...)
	}
}

// Level is the character's level.
func (c *Character) Level() int32 {
	if c.Record == nil {
		return 0
	}
	return c.Record.Level
}

// Sex is the character's sex, defaulting to neuter for anything without a
// record — which is what the C's pronoun macros assume of an unset field.
func (c *Character) Sex() int32 {
	if c == nil || c.Record == nil {
		return SexNeutral
	}
	return c.Record.Sex
}

// Subject, Possessive and Objective are the C's HSSH, HSHR and HMHR
// (utils.h:416), which is how act() fills in $E, $S and $M.
//
// There are exactly three sexes in the data file and the macros are written as
// nested ternaries against them, so anything that is not male or female is
// "it" — including every object-shaped mobile in the game, which is the point.
func (c *Character) Subject() string {
	switch c.Sex() {
	case SexMale:
		return "he"
	case SexFemale:
		return "she"
	}
	return "it"
}

// Possessive is HSHR: his, her, its.
func (c *Character) Possessive() string {
	switch c.Sex() {
	case SexMale:
		return "his"
	case SexFemale:
		return "her"
	}
	return "its"
}

// Objective is HMHR: him, her, it.
func (c *Character) Objective() string {
	switch c.Sex() {
	case SexMale:
		return "him"
	case SexFemale:
		return "her"
	}
	return "it"
}

// Title is what follows their name in the who-list.
func (c *Character) Title() string {
	if c.Record == nil {
		return ""
	}
	return c.Record.Title
}

// Enter puts a character into a room, removing them from wherever they were.
func (l *Live) Enter(c *Character, room RoomVnum) error {
	if _, ok := l.rooms[room]; !ok {
		return fmt.Errorf("room #%d does not exist", room)
	}
	l.Leave(c)
	c.Room = room
	l.occupants[room] = append(l.occupants[room], c)
	l.byName[strings.ToLower(c.Name)] = c

	// Entering the world is where a record and a body meet, so it is where
	// the record learns where its equipment is and what it is. Nothing is
	// recomputed here: the C does not total affects on entering a room
	// either, and doing it would rebuild a character from real values that a
	// caller may not have filled in yet.
	c.bindEquipment()
	if c.Record != nil {
		c.Record.Mobile = c.NPC
	}
	return nil
}

// Leave removes a character from their room. It is safe to call for someone
// who is not in one.
func (l *Live) Leave(c *Character) {
	here := l.occupants[c.Room]
	for i, other := range here {
		if other != c {
			continue
		}
		l.occupants[c.Room] = append(here[:i], here[i+1:]...)
		if len(l.occupants[c.Room]) == 0 {
			// Drop the empty slice rather than accumulate one per room the
			// world has ever had someone in.
			delete(l.occupants, c.Room)
		}
		break
	}
}

// Remove takes a character out of the world entirely.
func (l *Live) Remove(c *Character) {
	// Every following relationship goes with them, as extract_char's call to
	// die_follower does. A leader's follower list must never point at
	// somebody who has left the world.
	l.DieFollower(c)

	l.Leave(c)
	delete(l.byName, strings.ToLower(c.Name))
}

// Occupants returns who is in a room. The slice is the live one; callers must
// not retain or reorder it.
func (l *Live) Occupants(room RoomVnum) []*Character { return l.occupants[room] }

// FindInRoom finds a character in a room by a typed word, matching a prefix
// of their name as the C's get_char_room_vis does.
func (l *Live) FindInRoom(room RoomVnum, word string) *Character {
	word = strings.ToLower(strings.TrimSpace(word))
	if word == "" {
		return nil
	}
	for _, c := range l.occupants[room] {
		if strings.HasPrefix(strings.ToLower(c.Name), word) {
			return c
		}
		// A mobile is named by any of its keywords, as isname() does — which
		// is why `kill dragon` finds "the fractal dragon Puff".
		if c.Keywords != "" && matchesKeywords(c.Keywords, word) {
			return c
		}
	}
	return nil
}

// Find returns a character by name, case-insensitively.
func (l *Live) Find(name string) *Character { return l.byName[strings.ToLower(name)] }

// FindAnywhere returns the first character anywhere in the world a typed word
// names, porting get_char_world_vis.
//
// Rooms are walked in map order, so which of two identically named mobiles it
// finds is not defined — the C walks its character list in creation order and
// is equally arbitrary about it. Only the spells flagged TAR_CHAR_WORLD reach
// this: summon, and the two dispels.
func (l *Live) FindAnywhere(word string) *Character {
	word = strings.ToLower(strings.TrimSpace(word))
	if word == "" {
		return nil
	}
	// An exact name first, which is the common case and is not subject to
	// map order.
	if c := l.byName[word]; c != nil {
		return c
	}
	for room := range l.occupants {
		if c := l.FindInRoom(room, word); c != nil {
			return c
		}
	}
	return nil
}

// Players returns everyone in the world, sorted by level descending then name
// — the order the who-list wants.
func (l *Live) Players() []*Character {
	out := make([]*Character, 0, len(l.byName))
	for _, c := range l.byName {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Level() != out[j].Level() {
			return out[i].Level() > out[j].Level()
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// Exit returns the exit from a room in a direction, or nil.
func (l *Live) Exit(from RoomVnum, dir Direction) *ExitDef {
	room := l.rooms[from]
	if room == nil || !dir.Valid() {
		return nil
	}
	return room.Exits[dir]
}
