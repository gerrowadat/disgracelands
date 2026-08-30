// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import (
	"fmt"
	"strconv"
	"strings"
)

// Flags is a bitfield. The C code calls this bitvector_t and defines it as
// `unsigned long` (structs.h:599), which was 32 bits on the platform this game
// was written for and is 64 on modern Linux — exactly the kind of silent width
// change docs/design/go-port-plan.md §4 exists to eliminate. Here it is
// always 64 bits, and the places that must round-trip through a 32-bit
// representation say so explicitly.
type Flags uint64

// CFlagLimit is the number of bits the C server can actually represent.
//
// asciiflag_conv() computes `1 << (26 + (c - 'A'))` for uppercase letters,
// where the 1 is an `int`. Bit 31 is the sign bit and anything above it is
// undefined behaviour, so 'F' (bit 31) is the last letter the C server handles
// at all and everything from 'G' on is broken there. Data using those bits
// cannot round-trip to the C server whatever we do here.
const CFlagLimit = 32

// Has reports whether every bit in mask is set.
func (f Flags) Has(mask Flags) bool { return f&mask == mask }

// HasAny reports whether any bit in mask is set.
func (f Flags) HasAny(mask Flags) bool { return f&mask != 0 }

// Set returns f with the bits in mask set.
func (f Flags) Set(mask Flags) Flags { return f | mask }

// Clear returns f with the bits in mask cleared.
func (f Flags) Clear(mask Flags) Flags { return f &^ mask }

// ExceedsCRange reports whether f uses bits the C server cannot represent.
// Used by the linter to warn about world data that the two servers would
// disagree about.
func (f Flags) ExceedsCRange() bool { return f>>CFlagLimit != 0 }

// ParseFlags decodes the world files' bitfield encoding.
//
// The encoding has two forms and the reader accepts both, matching
// asciiflag_conv() in db.c:
//
//   - A letter string, one letter per set bit in bit order: 'a'–'z' for bits
//     0–25 and 'A'–'Z' for bits 26–51. "ae" is bits 0 and 4.
//   - A plain decimal number, used when the field consists only of digits.
//
// The digit check is what disambiguates them, and it is checked over the
// *whole* string: "128" is decimal 128, not bits for '1','2','8' (which are
// not letters and would set nothing). This branch ordering is load-bearing —
// see docs/investigations/ascii-pfile-format.md, which documents the same
// encoding on the player-file side.
//
// A malformed string is not an error in the C code: characters that are
// neither letters nor digits are silently ignored. That behaviour is kept,
// because real world files rely on the reader being forgiving, but the
// unrecognised runes are reported so the linter can complain about them.
func ParseFlags(s string) (flags Flags, unknown []rune) {
	bits, unknown := ParseFlagLetters(s)
	return Flags(bits), unknown
}

// ParseFlagLetters is ParseFlags over a raw bit vector, and is the actual
// implementation — see FlagLetters for why the pair is shaped this way.
func ParseFlagLetters(s string) (bits uint64, unknown []rune) {
	if s == "" {
		return 0, nil
	}

	allDigits := true
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			bits |= 1 << uint(r-'a')
		case r >= 'A' && r <= 'Z':
			bits |= 1 << uint(26+(r-'A'))
		default:
			unknown = append(unknown, r)
		}
		if r < '0' || r > '9' {
			allDigits = false
		}
	}

	if allDigits {
		// atol() on overflow is implementation-defined; ParseUint saturating
		// at the maximum is the closest well-defined equivalent, and the
		// linter flags anything that gets near it.
		n, err := strconv.ParseUint(s, 10, 64)
		if err != nil {
			return ^uint64(0), nil
		}
		return n, nil
	}

	return bits, unknown
}

// String renders f in the letter encoding the C writer (sprintbits) produces.
func (f Flags) String() string { return FlagLetters(uint64(f)) }

// FlagLetters renders a raw bit vector in the letter encoding the C writer
// (sprintbits) produces: one letter per set bit in bit order, or the literal
// "0" when no bits are set, since an empty field would break the reader.
//
// It takes a `uint64` rather than any flag type, and that is the shape the
// whole name-and-letter layer takes as of
// docs/proposals/idiomatic-go.md's step 1. Once every domain has its own
// `Set[T]` there is no single Go type left to write these signatures in —
// `Set[RoomFlag]` and `Set[PlayerFlag]` are unrelated types — and the two
// alternatives are both worse than a raw bit vector: making each helper
// generic buys nothing, because none of them can do anything with T that
// it cannot do with the bits, and it would force the domain to be named at
// every call site for no benefit; keeping eleven copies is the duplication
// §3.5 already complains about. So the letter encoding and the name tables
// operate on bits, `Set.Raw`/`SetFromRaw` are the conversion, and the
// domain type is what everything *above* this layer is written in. That is
// also exactly where §4.1 says the remaining G115 suppressions belong.
func FlagLetters(bits uint64) string {
	if bits == 0 {
		return "0"
	}
	var b strings.Builder
	for bit := 0; bit < 64; bit++ {
		if bits&(1<<uint(bit)) == 0 {
			continue
		}
		switch {
		case bit < 26:
			b.WriteRune(rune('a' + bit))
		case bit < 52:
			b.WriteRune(rune('A' + bit - 26))
		default:
			// No letter exists for these. Nothing in the real data reaches
			// here; if something ever does, say so rather than lose a bit.
			fmt.Fprintf(&b, "<bit%d>", bit)
		}
	}
	return b.String()
}

// Toggle flips the given bits, porting the C's TOG_BIT.
func (f Flags) Toggle(bits Flags) Flags { return f ^ bits }
