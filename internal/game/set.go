// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import (
	"iter"
	"slices"
)

// Set is a set of flags drawn from one domain — room flags, player flags,
// affects, item extras, and the eight others the game keeps as bit
// vectors. docs/proposals/idiomatic-go.md §4.1.
//
// The C keeps every one of those in a single `bitvector_t` (structs.h:599),
// and so did this port: one `Flags` type for eleven unrelated domains,
// which meant they shared a namespace of *values* as well as a type.
// `PlayerKiller`, `PrefBrief` and `RoomDark` were all `1 << 0`, so
// `rec.PlayerFlags.Has(RoomDark)` compiled, ran, and was true of every
// killer in the game. Nothing in the toolchain diagnosed it — not `go vet`,
// not `staticcheck` — because at the type level nothing was wrong.
// `Set[RoomFlag]` and `Set[PlayerFlag]` are different types, so it stops
// compiling instead.
//
// **The bit positions do not move and never will.** A domain's constants
// are bit *indices* rather than masks, which is the only visible change to
// how they are declared; index 7 is still bit 7, is still what
// `asciiflag_conv`'s letters decode to, is still what `flags_raw` carries,
// and is still what bitnames_test.go proves against constants.c.
// docs/proposals/idiomatic-go.md §2.1.
//
// One generic type rather than eleven hand-written ones, settled in §4.1:
// eleven concrete types would read better at a call site and would be
// sixty-odd near-identical methods to keep in agreement, which is the
// duplication the same document complains about elsewhere, reinvented.
// This is the tree's first generic *type* (there is one generic function,
// dump.go's emptyIfNil), and that is a deliberate bet rather than an
// oversight.
//
// A Set is comparable, so two of them can be compared with ==, and its zero
// value is the empty set.
type Set[T ~int] struct{ bits uint64 }

// NewSet is the set holding exactly vs.
func NewSet[T ~int](vs ...T) Set[T] { return Set[T]{}.With(vs...) }

// SetFromRaw builds a set from a raw bit vector.
//
// This and Raw are the persistence boundary and nothing else: a file
// format stores bits, and the two conversions are where a stored bit
// pattern becomes a typed set and back. Everywhere else works in the
// domain's own constants.
func SetFromRaw[T ~int](bits uint64) Set[T] { return Set[T]{bits: bits} }

// Raw is the bit vector, for the persistence boundary. See SetFromRaw.
func (s Set[T]) Raw() uint64 { return s.bits }

// Empty reports whether nothing is set.
func (s Set[T]) Empty() bool { return s.bits == 0 }

// Has reports whether v is in the set.
func (s Set[T]) Has(v T) bool { return s.bits&bitOf(v) != 0 }

// HasAll reports whether every one of vs is in the set. The empty call is
// true, matching the C's HAS_BITS against a zero mask.
func (s Set[T]) HasAll(vs ...T) bool {
	m := maskOf(vs)
	return s.bits&m == m
}

// HasAny reports whether any of vs is in the set.
func (s Set[T]) HasAny(vs ...T) bool { return s.bits&maskOf(vs) != 0 }

// With returns the set with vs added.
func (s Set[T]) With(vs ...T) Set[T] { return Set[T]{bits: s.bits | maskOf(vs)} }

// Without returns the set with vs removed.
func (s Set[T]) Without(vs ...T) Set[T] { return Set[T]{bits: s.bits &^ maskOf(vs)} }

// Toggle returns the set with each of vs flipped, porting the C's TOG_BIT.
func (s Set[T]) Toggle(vs ...T) Set[T] { return Set[T]{bits: s.bits ^ maskOf(vs)} }

// Union, Intersect and Minus are the set operations against another set of
// the same domain, for the handful of places that hold a computed mask
// rather than a list of constants.
func (s Set[T]) Union(o Set[T]) Set[T] { return Set[T]{bits: s.bits | o.bits} }

// Intersect is the bits in both.
func (s Set[T]) Intersect(o Set[T]) Set[T] { return Set[T]{bits: s.bits & o.bits} }

// Minus is the bits in s and not in o.
func (s Set[T]) Minus(o Set[T]) Set[T] { return Set[T]{bits: s.bits &^ o.bits} }

// Overlaps reports whether the two sets share a bit.
func (s Set[T]) Overlaps(o Set[T]) bool { return s.bits&o.bits != 0 }

// Contains reports whether every member of o is also in s. The empty set is
// contained in everything, which is the C's IS_SET(x, 0) and is load-bearing
// where a required-flags mask can be "nothing in particular".
func (s Set[T]) Contains(o Set[T]) bool { return s.bits&o.bits == o.bits }

// All iterates the members in bit order, lowest first.
//
// Bit order rather than any other, deliberately: iteration order is
// player-visible wherever a set is printed, and
// docs/proposals/idiomatic-go.md §2.2 rules out anything with a
// non-deterministic order on a path that can roll dice. A map would have
// been the other obvious representation and is exactly the thing that rule
// forbids.
func (s Set[T]) All() iter.Seq[T] {
	return func(yield func(T) bool) {
		for i := 0; i < 64; i++ {
			if s.bits&(1<<uint(i)) == 0 {
				continue
			}
			if !yield(T(i)) {
				return
			}
		}
	}
}

// Members is All collected into a slice, in the same order.
func (s Set[T]) Members() []T { return slices.Collect(s.All()) }

// ExceedsCRange reports whether the set uses bits the C server cannot
// represent. See CFlagLimit; used by the linter to warn about world data
// the two servers would disagree about.
func (s Set[T]) ExceedsCRange() bool { return s.bits>>CFlagLimit != 0 }

// String renders the set in the letter encoding the C writer (sprintbits)
// produces, so a Set prints the way the field it came from is stored.
func (s Set[T]) String() string { return FlagLetters(s.bits) }

// bit is one member's mask. A value outside 0..63 has no bit — the C's
// `1 << n` would be undefined behaviour there, and a domain constant is
// never out of range, so silently contributing nothing is the honest
// answer for the one case that can produce it: a raw index read off disk.
func bitOf[T ~int](v T) uint64 {
	if v < 0 || v > 63 {
		return 0
	}
	return 1 << uint(v)
}

func maskOf[T ~int](vs []T) uint64 {
	var m uint64
	for _, v := range vs {
		m |= bitOf(v)
	}
	return m
}
