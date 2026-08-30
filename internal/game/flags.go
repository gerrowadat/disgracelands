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

// The C's bitvector_t encoding, and nothing else.
//
// There was a `Flags uint64` here — one type for eleven unrelated flag
// domains, which is what docs/design/idiomatic-go.md §3.1 was written
// about. Step 1 gave each domain its own `Set[T]` and step 2's Class
// retired the last user, the remort vector, by making it a `Set[Class]`.
// What is left is the *encoding*: `asciiflag_conv`'s letters in and
// `sprintbits`' letters out, over a raw bit vector, because a bit vector
// is the one thing thirteen unrelated domains have in common. See
// FlagLetters for why this layer speaks `uint64`.

// CFlagLimit is the number of bits the C server can actually represent.
//
// asciiflag_conv() computes `1 << (26 + (c - 'A'))` for uppercase letters,
// where the 1 is an `int`. Bit 31 is the sign bit and anything above it is
// undefined behaviour, so 'F' (bit 31) is the last letter the C server handles
// at all and everything from 'G' on is broken there. Data using those bits
// cannot round-trip to the C server whatever we do here.
const CFlagLimit = 32

// ParseFlagLetters decodes the world files' bitfield encoding.
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

// FlagLetters renders a raw bit vector in the letter encoding the C writer
// (sprintbits) produces: one letter per set bit in bit order, or the literal
// "0" when no bits are set, since an empty field would break the reader.
//
// It takes a `uint64` rather than any flag type, and that is the shape the
// whole name-and-letter layer takes as of
// docs/design/idiomatic-go.md's step 1. Once every domain has its own
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
