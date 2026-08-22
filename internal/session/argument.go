// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package session

import (
	"strconv"
	"strings"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// Argument parsing, ported from one_argument/two_arguments/is_number
// (interpreter.c) and find_all_dots (handler.c).
//
// These four small functions are why `put sword in bag`, `put sword bag` and
// `put the sword in the bag` all do the same thing: one_argument silently
// drops a short list of filler words, so the preposition a player types is
// never actually read. That behaviour is invisible until you try to name an
// object "the" — and the C's own reserved word list exists because somebody
// did.

// fillWords are fill[] (interpreter.c:105). one_argument skips them, so they
// can never be arguments to anything.
var fillWords = map[string]bool{
	"in": true, "from": true, "with": true, "the": true,
	"on": true, "at": true, "to": true,
}

// oneArgument takes the next word, porting one_argument.
//
// The word is lower-cased — the C does it as it copies, which is why object
// keywords are matched case-insensitively everywhere without anyone asking —
// and filler words are skipped over entirely.
func oneArgument(argument string) (first, rest string) {
	for {
		argument = strings.TrimLeft(argument, " \t\r\n")
		if argument == "" {
			return "", ""
		}
		word := argument
		if i := strings.IndexAny(argument, " \t\r\n"); i >= 0 {
			word, rest = argument[:i], argument[i:]
		} else {
			rest = ""
		}
		word = strings.ToLower(word)
		if !fillWords[word] {
			return word, rest
		}
		argument = rest
	}
}

// twoArguments takes the next two words, porting two_arguments.
func twoArguments(argument string) (first, second, rest string) {
	first, rest = oneArgument(argument)
	second, rest = oneArgument(rest)
	return first, second, rest
}

// isNumber reports whether a word is all digits, porting is_number.
//
// The C returns true for the empty string, since its loop never runs. Every
// caller checks for emptiness first, so the same is true here and harmless;
// it is reproduced rather than fixed because a caller that stops checking
// should break the same way in both servers.
func isNumber(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// atoi is the C's atoi: a leading run of digits, and zero for anything else.
func atoi(s string) int32 {
	n, err := strconv.ParseInt(s, 10, 32)
	if err != nil {
		return 0
	}
	return int32(n)
}

// dotMode is what find_all_dots decided a word meant.
type dotMode int

const (
	// findIndiv: one named thing.
	findIndiv dotMode = iota
	// findAll: the bare word "all".
	findAll
	// findAllDot: "all.sword" — every sword.
	findAllDot
)

// findAllDots classifies an argument, porting find_all_dots.
//
// The C version rewrites its argument in place to strip the "all." prefix,
// which is why every caller passes a mutable buffer and why the same buffer is
// often reused afterwards. Here the stripped word is returned instead.
func findAllDots(arg string) (dotMode, string) {
	switch {
	case arg == "all":
		return findAll, arg
	case strings.HasPrefix(arg, "all."):
		return findAllDot, arg[len("all."):]
	default:
		return findIndiv, arg
	}
}

// Targeting helpers: the common shapes of get_char_room_vis and
// get_char_world_vis, with the viewer filled in.
//
// Every search in the C is filtered on CAN_SEE and honours a `2.` prefix.
// Both need to know who is looking, and for all but two callers that is the
// character who typed the command — so these exist rather than repeating
// `c.World.FindInRoom(c.Character, c.Character.Room, name)` forty times.

// findInRoom looks for somebody in the room the commanding character is in.
func (c *Context) findInRoom(word string) *game.Character {
	return c.World.FindInRoom(c.Character, c.Character.Room, word)
}

// findAnywhere looks for somebody anywhere in the world.
func (c *Context) findAnywhere(word string) *game.Character {
	return c.World.FindAnywhere(c.Character, word)
}
