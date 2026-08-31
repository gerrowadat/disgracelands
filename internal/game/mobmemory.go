// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

// A mobile's grudges, porting the memory_rec list and the three routines
// that maintain it (mobact.c:281-340).
//
// **It holds identity numbers, not characters**, which is the C's own
// choice and is the whole reason the feature is worth having: a player who
// runs away, logs out and comes back an hour later is a different
// char_data with the same GET_IDNUM, and the mobile still knows them. A
// list of pointers would forget at the door and would also keep a dead
// player's body alive in the heap.
//
// Runtime only. The C keeps this on char_data, never writes it to a file,
// and frees it in extract_char_final (handler.c:937) — which is a free()
// rather than a rule, and needs no counterpart here: a mobile that leaves
// the world takes its slice with it.

// Remember adds somebody to a mobile's grudge list, porting remember()
// (mobact.c:284).
//
// The three refusals are the C's, in its order, and the middle one is the
// reason a mobile brawl does not turn into a vendetta: a mobile never
// remembers another mobile. PRF_NOHASSLE is checked here *and* at the
// point of acting on the list, because a god can turn it on after being
// remembered and the C tests it in both places.
func (c *Character) Remember(victim *Character) {
	if c == nil || victim == nil {
		return
	}
	if !c.IsNPC() || victim.IsNPC() || victim.Record == nil {
		return
	}
	if victim.Record.Preferences.Has(PrefNoHassle) {
		return
	}
	if c.Remembers(victim) {
		return
	}
	c.Memory = append(c.Memory, victim.Record.IDNum)
}

// Forget drops somebody from the list, porting forget() (mobact.c:307).
// Forgetting somebody who was never on it is not an error — the C walks off
// the end and returns.
func (c *Character) Forget(victim *Character) {
	if c == nil || victim == nil || victim.Record == nil {
		return
	}
	id := victim.Record.IDNum
	for i, remembered := range c.Memory {
		if remembered == id {
			c.Memory = append(c.Memory[:i], c.Memory[i+1:]...)
			return
		}
	}
}

// Remembers reports whether a mobile is holding a grudge against somebody.
func (c *Character) Remembers(victim *Character) bool {
	if c == nil || victim == nil || victim.Record == nil {
		return false
	}
	for _, remembered := range c.Memory {
		if remembered == victim.Record.IDNum {
			return true
		}
	}
	return false
}
