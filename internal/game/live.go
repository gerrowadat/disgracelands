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
}

// NewLive indexes a loaded world for play.
func NewLive(defs *World) *Live {
	l := &Live{
		defs:      defs,
		rooms:     make(map[RoomVnum]*RoomDef, len(defs.Rooms)),
		occupants: make(map[RoomVnum][]*Character),
		byName:    make(map[string]*Character),
	}
	for _, r := range defs.Rooms {
		l.rooms[r.Vnum] = r
	}
	return l
}

// Room returns a room prototype, or nil.
func (l *Live) Room(v RoomVnum) *RoomDef { return l.rooms[v] }

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
	l.Leave(c)
	delete(l.byName, strings.ToLower(c.Name))
}

// Occupants returns who is in a room. The slice is the live one; callers must
// not retain or reorder it.
func (l *Live) Occupants(room RoomVnum) []*Character { return l.occupants[room] }

// Find returns a character by name, case-insensitively.
func (l *Live) Find(name string) *Character { return l.byName[strings.ToLower(name)] }

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
