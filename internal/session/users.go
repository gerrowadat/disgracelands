// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package session

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// do_users (act.informative.c:1224): every connection, not every player.
//
// That is what makes it different from `who`, and it is the point of the
// command: somebody stuck at the password prompt, somebody linkdead, somebody
// a god has switched into, all show here and none of them shows in `who`.

const usersFormat = "format: users [-l minlevel[-maxlevel]] [-n name] " +
	"[-h host] [-c classlist] [-o] [-p]\r\n"

// userFilter is what the flags collect.
type userFilter struct {
	low, high  int32
	name, host string
	classes    game.Flags
	outlaws    bool
	// playing and deadweight are opposites and both can be set, in which case
	// nothing matches at all — the C does not stop you asking.
	playing, deadweight bool
}

// parseUserFlags walks the argument, porting the C's `while (*buf)` loop.
//
// Every flag but `-d` sets `playing`, including `-o`; so `users -d -o` asks for
// connections that are both playing and not, and gets nothing. That is the C's
// and it is not worth correcting.
func parseUserFlags(arg string) (userFilter, bool) {
	f := userFilter{low: 0, high: game.LevelImplementor}

	rest := strings.TrimSpace(arg)
	for rest != "" {
		var word string
		word, rest = halfChop(rest)
		if !strings.HasPrefix(word, "-") || len(word) < 2 {
			return f, false
		}

		switch word[1] {
		case 'o', 'k':
			f.outlaws, f.playing = true, true
		case 'p':
			f.playing = true
		case 'd':
			f.deadweight = true
		case 'l':
			f.playing = true
			var span string
			span, rest = halfChop(rest)
			f.low, f.high = parseLevelSpan(span, f.low, f.high)
		case 'n':
			f.playing = true
			f.name, rest = halfChop(rest)
		case 'h':
			f.playing = true
			f.host, rest = halfChop(rest)
		case 'c':
			f.playing = true
			var list string
			list, rest = halfChop(rest)
			// The C walks the argument byte by byte and lowercases each
			// one, so a multi-byte rune simply never matches a class letter.
			for i := 0; i < len(list); i++ {
				f.classes |= classBit(list[i])
			}
		default:
			return f, false
		}
	}
	return f, true
}

// parseLevelSpan reads `-l 5-10`, `-l 5` or nonsense.
//
// The C uses `sscanf(arg, "%d-%d", &low, &high)` and ignores how many fields
// it filled, so `-l 5` leaves `high` at whatever it was — LVL_IMPL — and `-l
// banana` leaves both alone. Reproduced: a partial parse keeps the old value
// rather than resetting to zero.
func parseLevelSpan(span string, low, high int32) (int32, int32) {
	first, second, _ := strings.Cut(span, "-")
	// Clamped rather than converted: the C reads into an int and compares it
	// against a level, so a player typing `-l 99999999999` gets nonsense
	// either way, but nonsense that cannot wrap.
	if n, err := strconv.Atoi(strings.TrimSpace(first)); err == nil {
		low = clampLevel(n)
	}
	if n, err := strconv.Atoi(strings.TrimSpace(second)); err == nil {
		high = clampLevel(n)
	}
	return low, high
}

func clampLevel(n int) int32 {
	switch {
	case n < 0:
		return 0
	case n > int(game.LevelImplementor):
		return game.LevelImplementor
	}
	return int32(n)
}

// classBit is find_class_bitvector (class.c:129).
func classBit(c byte) game.Flags {
	switch lowerByte(c) {
	case 'm':
		return 1 << game.ClassMagicUser
	case 'c':
		return 1 << game.ClassCleric
	case 't':
		return 1 << game.ClassThief
	case 'w':
		return 1 << game.ClassWarrior
	case 'p':
		return 1 << game.ClassPaladin
	}
	return 0
}

func lowerByte(b byte) byte {
	if b >= 'A' && b <= 'Z' {
		return b - 'A' + 'a'
	}
	return b
}

func doUsers(c *Context) error {
	if c.Operator == nil {
		return nil
	}
	f, ok := parseUserFlags(c.Arg)
	if !ok {
		c.Send("%s", usersFormat)
		return nil
	}

	c.Send("Num Class   Name         State          Idl Login@   Site\r\n")
	c.Send("--- ------- ------------ -------------- --- -------- ------------------------\r\n")

	shown := 0
	for i, s := range c.Operator.Sessions() {
		playing := s.State() == StatePlaying
		if !playing && f.playing {
			continue
		}
		if playing && f.deadweight {
			continue
		}

		// The character a god switched *out of* is the one being reported,
		// because that is who the connection belongs to.
		who := s.Original()
		if who == nil {
			who = s.Character()
		}

		class := "   -   "
		if playing {
			if who == nil || !c.matchesUserFilter(who, s, f) {
				continue
			}
			class = fmt.Sprintf("[%2d %s]", who.Level(), game.ClassAbbrevs[who.Record.Class])
		}

		name := "UNDEFINED"
		if who != nil {
			name = who.Name
		}

		state := s.ConnectedName()
		if playing && s.Original() != nil {
			state = "Switched"
		}

		// Idle is blank for a god and for anything not in the world. This
		// port has no per-character idle counter — see docs/deviations.md —
		// so it is blank for everybody.
		idle := ""

		host := s.Host()
		if host == "" {
			host = "Hostname unknown"
		}

		// The listing test is separate from the filter above and applies only
		// to connections that are playing: a descriptor at the login prompt is
		// always shown, whoever is asking.
		if playing && !c.World.CanSee(c.Character, s.Character()) {
			continue
		}

		c.Send("%3d %-7s %-12s %-14s %-3s %-8s [%s]\r\n",
			i+1, class, name, state, idle,
			s.LoginTime().Format("15:04:05"), host)
		shown++
	}

	c.Send("\r\n%d visible sockets connected.\r\n", shown)
	return nil
}

// matchesUserFilter is the C's run of `continue`s, for a connection that is
// playing. Everything here is skipped for one that is not.
func (c *Context) matchesUserFilter(who *game.Character, s *Session, f userFilter) bool {
	rec := who.Record
	if rec == nil {
		return false
	}
	if f.host != "" && !strings.Contains(s.Host(), f.host) {
		return false
	}
	if f.name != "" && !strings.EqualFold(who.Name, f.name) {
		return false
	}
	if !c.World.CanSee(c.Character, who) || rec.Level < f.low || rec.Level > f.high {
		return false
	}
	if f.outlaws && !rec.PlayerFlags.HasAny(game.PlayerKiller, game.PlayerThief) {
		return false
	}
	if f.classes != 0 && !f.classes.Has(1<<rec.Class) {
		return false
	}
	// The C's next line is `if (GET_INVIS_LEV(ch) > GET_LEVEL(ch)) continue;`
	// — `ch` on both sides, where the loop's subject is `tch`. It can never
	// fire, because `set invis` clamps the level to the character's own. Not
	// ported, because porting a dead check is porting nothing; recorded in
	// docs/weirdnumbers.md so it is not mistaken for an omission.
	return true
}
