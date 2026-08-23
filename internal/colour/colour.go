// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

// Package colour turns markup into ANSI, or into nothing.
//
// The C interleaves escape sequences into its strings as it builds them —
// `CCCYN(ch, C_NRM)` expands to the escape or to "" depending on the reader's
// preference, so every message is assembled per reader. That works because
// send_to_char has the character in hand.
//
// Here the messages are written once and rendered at the socket, which is
// where the reader's preference is known. The markup is the one
// `docs/proposals/data-format.md` §5 already settled on for the native data
// format — `{{red}}` … `{{/}}` — so a room name coloured in a world file and a
// room name coloured by the engine go through the same renderer.
package colour

import "strings"

// Level is the C's C_OFF/C_SPR/C_NRM/C_CMP (screen.h:29), which is a
// *threshold* rather than a palette: a message declares how much colour it is,
// and a reader who has asked for less than that gets none of it.
type Level int

// The four levels, in the C's order — the numbers are the player's stored
// PRF_COLOR_1 and PRF_COLOR_2 bits read as a two-bit number, so they are the
// format as much as they are an enum.
const (
	Off Level = iota
	Sparse
	Normal
	Complete
)

// Names are `ctypes[]` (constants.c), which `color` matches against and
// prints back.
var Names = []string{"Off", "Sparse", "Normal", "Complete"}

// codes are the markup, mapping to the escapes screen.h defines.
//
// `{{/}}` is the reset, spelled short because it is the commonest token by a
// wide margin — every coloured run ends with one.
var codes = map[string]string{
	"/":       "\x1b[0m",
	"red":     "\x1b[31m",
	"green":   "\x1b[32m",
	"yellow":  "\x1b[33m",
	"blue":    "\x1b[34m",
	"magenta": "\x1b[35m",
	"cyan":    "\x1b[36m",
	"white":   "\x1b[37m",

	"bright-red":     "\x1b[1;31m",
	"bright-green":   "\x1b[1;32m",
	"bright-yellow":  "\x1b[1;33m",
	"bright-blue":    "\x1b[1;34m",
	"bright-magenta": "\x1b[1;35m",
	"bright-cyan":    "\x1b[1;36m",
	"bright-white":   "\x1b[1;37m",

	// A literal `{{`, for completeness. The proposal counted zero occurrences
	// of a doubled brace in the whole corpus, so nothing needs it yet.
	"lbrace": "{{",
}

// Render replaces the markup in s.
//
// want is how much colour the message is — the C's second argument to
// CCCYN and friends — and have is how much the reader has asked for. Escapes
// go in only when the reader has asked for at least as much as the message
// wants; otherwise the tokens come out and nothing takes their place, which is
// exactly what `KNUL` does in the C.
//
// An unknown token is left alone rather than dropped. It is far more likely to
// be a player typing braces at a bulletin board than a bug in a format string,
// and eating their text would be worse than showing it.
func Render(s string, want, have Level) string {
	if !strings.Contains(s, "{{") {
		return s
	}
	on := have >= want

	var b strings.Builder
	for {
		i := strings.Index(s, "{{")
		if i < 0 {
			b.WriteString(s)
			return b.String()
		}
		end := strings.Index(s[i:], "}}")
		if end < 0 {
			// An unterminated `{{` is text, not markup.
			b.WriteString(s)
			return b.String()
		}
		name := s[i+2 : i+end]
		b.WriteString(s[:i])

		if escape, ok := codes[name]; ok {
			if on {
				b.WriteString(escape)
			}
		} else {
			b.WriteString(s[i : i+end+2])
		}
		s = s[i+end+2:]
	}
}

// Strip removes the markup without rendering it, for anywhere the text is not
// going to a terminal — a log line, a board file, a saved description.
func Strip(s string) string { return Render(s, Complete, Off) }

// LevelOf reads the two stored preference bits as a number, porting
// `_clrlevel` (screen.h:32).
//
// The C writes it as arithmetic — `(PRF_COLOR_1 ? 1 : 0) + (PRF_COLOR_2 ? 2 :
// 0)` — because the two bits are not adjacent in the flags word and cannot
// simply be masked out.
func LevelOf(one, two bool) Level {
	var l Level
	if one {
		l++
	}
	if two {
		l += 2
	}
	return l
}

// Bits is LevelOf backwards: which of the two preference bits a level sets.
// `do_color` writes it as `PRF_COLOR_1 * (tp & 1) | PRF_COLOR_2 * (tp & 2) >>
// 1`, which is the same thing said in one line.
func Bits(l Level) (one, two bool) { return l&1 != 0, l&2 != 0 }

// ParseLevel matches a name the way `color` does — `search_block` with an
// exact-match flag of FALSE, which is a prefix match, so `co` is Complete and
// `s` is Sparse.
func ParseLevel(word string) (Level, bool) {
	word = strings.ToLower(strings.TrimSpace(word))
	if word == "" {
		return 0, false
	}
	for i, name := range Names {
		if strings.HasPrefix(strings.ToLower(name), word) {
			return Level(i), true
		}
	}
	return 0, false
}
