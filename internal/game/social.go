// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Socials, ported from boot_social_messages (act.social.c:216) and the file
// format it reads.
//
// A social is nine strings and two numbers, and there are 106 of them in
// `data/misc/socials`. They are a third of the command table and none of them
// is code: `smile`, `grin`, `hug`, `poke`, `puke`. What makes them worth
// getting exactly right is that they were the game's whole social layer for
// seven years, and every one of them is a line somebody read a thousand times.

// Social is one entry: the C's `struct social_messg`.
type Social struct {
	// Name is the command word.
	Name string
	// Hide is the C's `hide` flag, passed to act() as its `hide_invisible`
	// argument — a social with it set is not shown to people who cannot see
	// the actor. Nothing reads it yet, because nothing computes visibility.
	Hide bool
	// MinVictimPosition is how upright the victim must be. Below it they get
	// "$N is not in a proper position for that."
	MinVictimPosition Position

	// With no argument.
	CharNoArg   string
	OthersNoArg string

	// With an argument, and somebody found. An empty CharFound means the
	// social takes no argument at all and the four fields after it were never
	// in the file.
	CharFound   string
	OthersFound string
	VictFound   string

	// With an argument and nobody of that name here.
	NotFound string

	// Aimed at yourself.
	CharAuto   string
	OthersAuto string
}

// TakesTarget reports whether the social can be aimed at somebody. The C
// tests `char_found` being non-NULL, and a social without it ignores its
// argument entirely.
func (s Social) TakesTarget() bool { return s.CharFound != "" }

// ParseSocials reads the socials file, porting boot_social_messages.
//
// The format is a header line — name, hide flag, minimum victim position —
// followed by up to eight message lines. A line consisting of `#` is a NULL
// string, and a `#` in the *third* slot means the social takes no argument
// and the file moves straight on to the next one. The list ends at a line
// beginning `$`.
//
// The C exits the process on a malformed file. Here it is an error, because a
// server that will not start is worse than one that starts without `smirk`.
func ParseSocials(r io.Reader) ([]Social, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	// next returns the next non-blank line, since the file separates entries
	// with them and fscanf's " %s " skips whitespace.
	next := func() (string, bool) {
		for scanner.Scan() {
			if line := strings.TrimRight(scanner.Text(), "\r\n"); strings.TrimSpace(line) != "" {
				return line, true
			}
		}
		return "", false
	}
	// message returns the next line as a message, or "" for the `#` that
	// means NULL. Blank lines are *not* skipped here: a social's message can
	// legitimately be empty, and the C's fgets would read it as one.
	message := func() (string, bool) {
		if !scanner.Scan() {
			return "", false
		}
		line := strings.TrimRight(scanner.Text(), "\r\n")
		if strings.HasPrefix(line, "#") {
			return "", true
		}
		return line, true
	}

	var out []Social
	for {
		header, ok := next()
		if !ok {
			return nil, fmt.Errorf("socials: unexpected end of file, no terminating $")
		}
		if strings.HasPrefix(strings.TrimSpace(header), "$") {
			return out, nil
		}

		fields := strings.Fields(header)
		if len(fields) != 3 {
			return nil, fmt.Errorf("socials: malformed header %q", header)
		}
		hide, err := strconv.Atoi(fields[1])
		if err != nil {
			return nil, fmt.Errorf("socials: %q: bad hide flag %q", fields[0], fields[1])
		}
		pos, err := strconv.Atoi(fields[2])
		if err != nil {
			return nil, fmt.Errorf("socials: %q: bad victim position %q", fields[0], fields[2])
		}

		s := Social{
			Name:              strings.ToLower(fields[0]),
			Hide:              hide != 0,
			MinVictimPosition: Position(pos), //nolint:gosec // a file value, bounded by the format
		}

		for _, field := range []*string{&s.CharNoArg, &s.OthersNoArg, &s.CharFound} {
			line, ok := message()
			if !ok {
				return nil, fmt.Errorf("socials: %q: unexpected end of file", s.Name)
			}
			*field = line
		}

		// No char_found means the rest is not in the file at all.
		if s.CharFound != "" {
			for _, field := range []*string{
				&s.OthersFound, &s.VictFound, &s.NotFound, &s.CharAuto, &s.OthersAuto,
			} {
				line, ok := message()
				if !ok {
					return nil, fmt.Errorf("socials: %q: unexpected end of file", s.Name)
				}
				*field = line
			}
		}

		out = append(out, s)
	}
}
