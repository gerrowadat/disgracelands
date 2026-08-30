// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import (
	"strconv"
	"strings"
)

// Moving objects about, porting handler.c's obj_to_*/obj_from_* family and
// equip_char/unequip_char.
//
// The C has eight functions that each unlink an object from one list and link
// it onto another, and the invariant that an object is on exactly one list is
// maintained by every caller remembering to call the matching pair. It mostly
// works. Where it does not is how items get duplicated.
//
// Here there is one unlink — detach — and every placement calls it first. An
// object cannot be in two places because putting it somewhere always takes it
// out of wherever it was.

// detach removes an object from wherever it currently is, leaving it nowhere.
func (l *Live) detach(o *Object) {
	if o == nil {
		return
	}

	switch p := o.placement.(type) {
	case InRoom:
		l.roomObjects[p.Room] = removeObject(l.roomObjects[p.Room], o)
		if len(l.roomObjects[p.Room]) == 0 {
			delete(l.roomObjects, p.Room)
		}
		// obj_from_room dirties a house too (handler.c:711): picking
		// something up out of one changes what has to be saved just as much
		// as dropping something in it.
		l.MarkHouseChanged(p.Room)

	case CarriedBy:
		if p.Holder != nil {
			p.Holder.Carrying = removeObject(p.Holder.Carrying, o)
		}

	case WornBy:
		// The range check is what the union removed: WornBy cannot exist
		// without a position, so only the bounds are left, and those are
		// still worth having because WearPosition comes off a world file.
		if p.Holder != nil && p.At >= 0 && p.At < NumWears {
			if p.Holder.Equipment[p.At] == o {
				p.Holder.Equipment[p.At] = nil
			}
		}

	case InContainer:
		if p.Container != nil {
			p.Container.Contents = removeObject(p.Container.Contents, o)
		}
	}

	o.placement = nil
}

// prependObject puts an object at the head of a list, which is what every one
// of the C's obj_to_* functions does:
//
//	object->next_content = ch->carrying;
//	ch->carrying = object;
//
// (handler.c:418-419, and the same two lines in obj_to_room at :685-686 and
// obj_to_obj at :737-738.) They are singly-linked lists with no tail pointer,
// so the head is the only place an insert can go cheaply — but the *order it
// produces* is not an implementation detail, because every reader walks from
// the head. What you dropped last is listed first, `get all` runs
// newest-first, and `2.sword` counts down a list in that order, so this
// decides which sword you get as well as how they are printed.
//
// This port appended for five phases, which reversed all of it. See
// docs/deviations.md and #193.
//
// The copy is deliberate rather than an in-place shift: `Occupants` aside,
// these slices are handed out by RoomObjects and read directly as
// `c.Carrying`, and a caller iterating one while a specproc drops something
// into it would otherwise see an element twice. The lists are a handful of
// items long and this runs when somebody picks something up.
func prependObject(list []*Object, o *Object) []*Object {
	out := make([]*Object, 0, len(list)+1)
	out = append(out, o)
	return append(out, list...)
}

// ObjectToRoom puts an object on the floor, porting obj_to_room.
func (l *Live) ObjectToRoom(o *Object, room RoomVnum) {
	if o == nil {
		return
	}
	l.detach(o)

	o.placement = InRoom{Room: room}
	if l.roomObjects == nil {
		l.roomObjects = map[RoomVnum][]*Object{}
	}
	l.roomObjects[room] = prependObject(l.roomObjects[room], o)
	l.track(o)

	// obj_to_room sets ROOM_HOUSE_CRASH on a house (handler.c:692), and
	// obj_from_room does the same (handler.c:711). That dirty bit is what
	// makes House_save_all cheap: a hundred houses do not get a hundred file
	// rewrites a minute just because somebody walked through one.
	l.MarkHouseChanged(room)
}

// ObjectToChar puts an object into somebody's inventory, porting
// obj_to_char.
func (l *Live) ObjectToChar(o *Object, c *Character) {
	if o == nil || c == nil {
		return
	}
	l.detach(o)

	o.placement = CarriedBy{Holder: c}
	c.Carrying = prependObject(c.Carrying, o)
	l.track(o)
}

// ObjectToObject puts an object inside a container, porting obj_to_obj.
//
// It refuses to put a container inside itself, directly or at any depth. The
// C does not check, and the result is an object graph with a cycle in it that
// hangs the next thing to walk it.
func (l *Live) ObjectToObject(o, container *Object) bool {
	if o == nil || container == nil || o == container {
		return false
	}
	for above := container; above != nil; above = above.ContainerOf() {
		if above == o {
			return false
		}
	}

	l.detach(o)
	o.placement = InContainer{Container: container}
	container.Contents = prependObject(container.Contents, o)
	l.track(o)
	return true
}

// Equip puts an object on a character, porting equip_char.
//
// It returns false if the slot is occupied, which is the caller's cue to say
// so. The C logs a SYSERR and drops the object on the floor, which is a way
// of losing equipment.
//
// Armour class is applied here and not in the recompute, because that is
// where the C applies it: equip_char subtracts apply_ac from the character's
// own armour figure. The `A` applies are the other mechanism and are
// recomputed from scratch. See equip.go.
func (l *Live) Equip(o *Object, c *Character, pos WearPosition) bool {
	if o == nil || c == nil || pos < 0 || pos >= NumWears {
		return false
	}
	if c.Equipment[pos] != nil {
		return false
	}

	l.detach(o)
	o.placement = WornBy{Holder: c, At: pos}
	c.Equipment[pos] = o
	l.track(o)

	if c.Record != nil {
		c.Record.RealArmor -= ArmorClassOf(o, pos)
		c.bindEquipment()
		RecomputeAffects(c.Record)
	}
	return true
}

// Unequip takes an object off and returns it, leaving it nowhere. The caller
// decides where it goes next — inventory, usually.
func (l *Live) Unequip(c *Character, pos WearPosition) *Object {
	if c == nil || pos < 0 || pos >= NumWears {
		return nil
	}
	o := c.Equipment[pos]
	if o == nil {
		return nil
	}
	l.detach(o)

	if c.Record != nil {
		c.Record.RealArmor += ArmorClassOf(o, pos)
		c.bindEquipment()
		RecomputeAffects(c.Record)
	}
	return o
}

// bindEquipment points a character's record at their equipment, so that
// anything recomputing from the record alone can see what they are wearing.
func (c *Character) bindEquipment() {
	if c != nil && c.Record != nil {
		c.Record.Worn = &c.Equipment
	}
}

// ExtractObject destroys an object and everything in it, porting
// extract_obj.
//
// The C moves a container's contents to wherever the container was, which is
// what stops a bag being destroyed from taking its contents with it. That is
// the behaviour here too, except when the object is being destroyed from
// nowhere, in which case there is nowhere for the contents to go and they are
// destroyed with it.
func (l *Live) ExtractObject(o *Object) {
	if o == nil {
		return
	}

	contents := o.Contents
	o.Contents = nil

	for _, inside := range contents {
		switch p := o.placement.(type) {
		case InRoom:
			l.ObjectToRoom(inside, p.Room)
		case CarriedBy:
			l.ObjectToChar(inside, p.Holder)
		case WornBy:
			l.ObjectToChar(inside, p.Holder)
		case InContainer:
			l.ObjectToObject(inside, p.Container)
		default:
			l.ExtractObject(inside)
		}
	}

	l.detach(o)
	delete(l.objects, o.ID)
}

// RoomObjects lists what is lying in a room.
func (l *Live) RoomObjects(room RoomVnum) []*Object { return l.roomObjects[room] }

// Objects returns every object in the world, for the decay pass.
func (l *Live) Objects() []*Object {
	out := make([]*Object, 0, len(l.objects))
	for _, o := range l.objects {
		out = append(out, o)
	}
	return out
}

// NewObject instantiates a prototype into the world.
func (l *Live) NewObject(vnum ObjVnum) *Object {
	def := l.ObjectDef(vnum)
	if def == nil {
		return nil
	}
	l.nextObjectID++
	return NewObject(l.nextObjectID, def)
}

// NewBareObject makes an object with no prototype, for corpses and coins.
func (l *Live) NewBareObject() *Object {
	l.nextObjectID++
	return NewObject(l.nextObjectID, nil)
}

// track records an object as existing in the world, so the decay pass and
// the shutdown sweep can find it.
func (l *Live) track(o *Object) {
	if l.objects == nil {
		l.objects = map[uint64]*Object{}
	}
	l.objects[o.ID] = o
}

// removeObject drops one object from a slice, preserving order.
//
// Order matters: it is the order `inventory` lists things in, and a player
// who put three things in a bag expects to see them in that order.
func removeObject(list []*Object, o *Object) []*Object {
	for i, candidate := range list {
		if candidate == o {
			return append(list[:i:i], list[i+1:]...)
		}
	}
	return list
}

// MatchesAnyKeyword is matchesKeywords for callers outside this package.
func MatchesAnyKeyword(keywords, word string) bool { return matchesKeywords(keywords, word) }

// matchesKeywords reports whether word names something with these keywords,
// porting isname() (handler.c:56).
//
// **It is a whole-word match, not a prefix one.** `get sword` picks up a long
// sword and `get swo` does not, because the C returns 1 only where the search
// string runs out *and* the keyword underneath it has ended too:
//
//	if (!*curstr && !isalpha(*curname))
//	  return (1);
//
// This port had it as a prefix match for four phases, with a comment claiming
// the C matched prefixes. It does not, and the oracle in
// `reference/tools/nameoracle.c` is what settled it — the loop reads like a
// prefix match and is not one. See docs/weirdnumbers.md.
//
// **A keyword ends at any non-alphabetic byte, not at whitespace**, which is
// what that `!isalpha(*curname)` says and what the second version of this
// function still got wrong. It was `strings.Fields` plus equality, which is
// the same answer for every namelist made only of letters and spaces — and
// every namelist in the oracle's own sweep was exactly that, so 168 pairings
// agreed with a C they were not really testing. A digit is not a letter, so
// the C matches "6" and "60" against a keyword of "606", and "a" against
// "a1b"; the archived world has "staircase stair 606 rs" and numbered
// keywords are an ordinary builder habit. Issue #277. The sweep now includes
// digits, punctuation, doubled and trailing spaces and a namelist wrapped
// across lines by fread_string, which is 1,482 pairings rather than 168.
//
// The body below is the C's own two loops rather than a rule derived from
// them, deliberately. The two loops disagree about what separates keywords —
// the inner one breaks on a literal space, the outer one skips over a run of
// *alphabetic* bytes and then steps one byte past it — so where a match may
// begin depends on where the previous attempt gave up. That is not something
// to restate in a sentence and hope; it is CLAUDE.md's "if you would have to
// simulate a function in your head to be sure of it" exactly. Transliterating
// it is safe here only because the oracle then checks every pairing.
//
// Byte-oriented, and ASCII-only in both `isalpha` and `LOWER`, because the C
// is: LOWER() (utils.h) tests 'A'..'Z' and nothing else, and the C locale's
// isalpha() is false for every byte above 127 — so a UTF-8 continuation byte
// ends a keyword there and must here too.
//
// The one deliberate difference: an empty word matches nothing here, where
// the C's isname matches it against anything (`!*curstr` is true on the first
// pass). Every caller checks for an empty argument first, so the difference is
// unreachable, and refusing is the safer shape.
func matchesKeywords(keywords, word string) bool {
	if strings.TrimSpace(word) == "" {
		return false
	}
	word = strings.TrimSpace(word)

	// curname and curstr are the C's two cursors, as indices.
	curname := 0
	for {
		curstr := 0
		for {
			// if (!*curstr && !isalpha(*curname)) return (1);
			if curstr == len(word) && !asciiAlpha(byteAt(keywords, curname)) {
				return true
			}
			// if (!*curname) return (0);
			if curname == len(keywords) {
				return false
			}
			// if (!*curstr || *curname == ' ') break;
			if curstr == len(word) || keywords[curname] == ' ' {
				break
			}
			// if (LOWER(*curstr) != LOWER(*curname)) break;
			if asciiLower(word[curstr]) != asciiLower(keywords[curname]) {
				break
			}
			curstr++
			curname++
		}

		// for (; isalpha(*curname); curname++);
		for curname < len(keywords) && asciiAlpha(keywords[curname]) {
			curname++
		}
		// if (!*curname) return (0);
		if curname == len(keywords) {
			return false
		}
		curname++ // first char of new name
	}
}

// byteAt is the C's `*p` where p may be at the terminating NUL: reading one
// past the last byte of a Go string is a panic, and in C it is the '\0' that
// every one of these tests is really asking about.
func byteAt(s string, i int) byte {
	if i >= len(s) {
		return 0
	}
	return s[i]
}

// asciiAlpha is the C locale's isalpha(): letters, and nothing else. Not
// unicode.IsLetter — a byte above 127 is not alphabetic to the C server, and
// treating one as a letter would run two keywords together.
func asciiAlpha(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// asciiLower is utils.h's LOWER(), which is 'A'..'Z' and nothing else.
func asciiLower(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + ('a' - 'A')
	}
	return c
}

// GetNumber splits a leading `2.` off a typed argument, porting get_number
// (handler.c:590): it returns which match the player asked for, and the rest
// of the word.
//
// The C rewrites the caller's buffer in place and returns the count, and three
// details of that are load-bearing:
//
//   - **The rewrite happens before the digits are checked.** `foo.sword`
//     returns 0 *and* leaves "sword" behind, so a caller looking for a
//     character searches for a *player* called sword, and one looking for an
//     object finds nothing at all. `.sword` does the same.
//   - **Zero is a value, not a failure.** `get_char_room_vis` reads it as
//     "player with this name" (handler.c:1068) and every object search reads
//     it as "give up". So 0 has two meanings depending on who asked.
//   - **atoi, with everything that implies.** `007.sword` is the seventh
//     sword; `2.3.sword` is the second `3.sword`, because only the first dot
//     is consumed; `-1.sword` is 0, because '-' is not a digit.
//
// Verified against the C over every one of those cases; see
// `reference/tools/nameoracle.c`.
func GetNumber(arg string) (int, string) {
	dot := strings.IndexByte(arg, '.')
	if dot < 0 {
		return 1, arg
	}
	prefix, rest := arg[:dot], arg[dot+1:]
	for _, r := range prefix {
		if r < '0' || r > '9' {
			return 0, rest
		}
	}
	// An empty prefix reaches atoi("") == 0, which is the C's answer for a
	// bare leading dot.
	n, err := strconv.Atoi(prefix)
	if err != nil {
		return 0, rest
	}
	return n, rest
}
