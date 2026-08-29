// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package session

import (
	"strings"
	"unicode/utf8"
)

// process_input's line stage (comm.c:1781-1846), which is everything it does
// to a finished line after the byte-by-byte copy in readLoop has built it.
//
// The C does six things to a line on its way in. readLoop does the two that
// belong in a byte loop — the backspace rub-out (#233) and the filter that
// drops anything unprintable — and this file does the other four, in the
// order the C does them, which is the order they have to be in:
//
//  1. Truncate an over-long line and say so.
//  2. Copy the line to whoever is snooping this connection.
//  3. `!`, `!<prefix>` and `^old^new`, against a five-deep history.
//  4. Hand the result on to be run.
//
// Snooping comes before the history substitution on purpose: a snooper sees
// the `!lo` a player typed, not the `look` it turned into.
//
// The one thing here that is not a port is the guard on echo. See
// recordable.

// maxInputLength is MAX_INPUT_LENGTH (structs.h:560).
const maxInputLength = 256

// maxInputChars is what a player can actually type, and it is not 255.
//
// process_input's copy loop is `for (ptr = read_point; (space_left > 1) &&
// (ptr < nl_pos); ptr++)` with `space_left = MAX_INPUT_LENGTH - 1`, so it
// stops with one byte of room still unspent — the comment above it says why,
// "The '> 1' reserves room for a '$ => $$' expansion" — and the last
// character it can write is the 254th. See docs/weirdnumbers.md.
const maxInputChars = maxInputLength - 2

// historySize is HISTORY_SIZE (structs.h:558), "Keep last 5 commands".
//
// Only four of them are ever reachable by `!<prefix>`; see recallHistory.
const historySize = 5

// inputHistory is one connection's command history — the C's `history`,
// `history_pos` and `last_input`, which live on `struct descriptor_data`
// (structs.h:1108-1112).
//
// No lock and no atomic, unlike Session.character next door: every field is
// written and read by readLoop alone, on the connection's own goroutine, and
// nothing else in the tree can reach them.
type inputHistory struct {
	// entries is the circular buffer, and pos is the slot the *next*
	// ordinary line goes into — so pos-1 is the most recent and pos itself
	// is the oldest.
	entries [historySize]string
	pos     int
	// last is `last_input`, which is a separate thing from the history: `!`
	// replays it, `^old^new` substitutes into it, and both of those write
	// it back without ever adding to the history.
	last string
}

// truncateInput applies MAX_INPUT_LENGTH, reporting whether it had to.
//
// Truncating at a rune boundary rather than a byte is this port's own
// business: the C could not have a part-formed character to cut through,
// because its isprint filter drops every byte above 127 before this. See
// readLoop's own note on the same divergence for the backspace.
func truncateInput(line string) (string, bool) {
	if utf8.RuneCountInString(line) <= maxInputChars {
		return line, false
	}
	n := 0
	for i := range line {
		if n == maxInputChars {
			return line[:i], true
		}
		n++
	}
	return line, false
}

// recordable reports whether this line may be remembered, relayed or
// recalled — and it is the one deliberate difference in this file.
//
// The C makes no distinction: process_input stores every line in the history
// and in last_input whatever state the descriptor is in, and copies every
// line to a snooper. So on the real server a password typed at the login
// prompt went into the history in the clear, where `!p` would find it, echo
// it back and run it as a command.
//
// This port does not do that. Nothing is recorded, recalled or relayed while
// the server has told the client to stop echoing, which is exactly the window
// a password is typed in (protocol.go's EchoOff/EchoOn, the C's
// echo_off_str). The precedent is in this repository already: the browser
// terminal's up-arrow refuses to replay anything typed with echo off, for the
// same reason and in the same words (#235, internal/server/web_templates.go).
// docs/deviations.md has the entry.
func (s *Session) recordable() bool {
	s.proto.mu.Lock()
	defer s.proto.mu.Unlock()
	return !s.proto.echo
}

// snoopInput copies a line to whoever is snooping this connection, as
// `% <line>` (comm.c:1813-1814).
//
// The port already relays a snooped session's *output* (Session.send), so
// without this half `snoop` showed one side of every conversation: the
// answers, never the questions.
func (s *Session) snoopInput(line string) {
	if !s.recordable() {
		return
	}
	if watcher := s.SnoopedBy(); watcher != nil {
		watcher.Send("%% %s\r\n", line)
	}
}

// recallInput is the `!` / `!<prefix>` / `^old^new` block (comm.c:1816-1846),
// returning the line to run and whether to run it at all.
//
// false is the C's `failed_subst`: a `^old^new` that named no second `^`, or
// whose old text is not in the last command, is answered and then *not* run.
// Nothing else here can refuse a line.
func (s *Session) recallInput(line string) (string, bool) {
	// None of it happens while echo is off, and that is not only about
	// keeping a password out of the history. A password beginning with `!`
	// or `^` would otherwise be read as a recall or a substitution and
	// never reach the prompt as itself: `^secret^` is a legal password
	// here (badNewPassword asks for six characters and nothing else) and
	// on the real server it was unusable, answered "Invalid substitution."
	// every time it was typed.
	if !s.recordable() {
		return line, true
	}

	switch {
	case line == "!":
		// Bare `!`: replay last_input, without echoing it. It is not added
		// to the history — the C's `strcpy(tmp, t->last_input)` and nothing
		// else — so `!` twice running replays the same line, and a history
		// walk still sees whatever came before it.
		return s.input.last, true

	case strings.HasPrefix(line, "!"):
		return s.recallHistory(line), true

	case strings.HasPrefix(line, "^"):
		substituted, ok := performSubst(s.input.last, line)
		if !ok {
			s.Send("Invalid substitution.\r\n")
			return "", false
		}
		s.input.last = substituted
		return substituted, true

	default:
		s.input.last = line
		s.input.entries[s.input.pos] = line
		s.input.pos++
		if s.input.pos >= historySize {
			s.input.pos = 0
		}
		return line, true
	}
}

// recallHistory is `!<prefix>`: the most recent command the prefix
// abbreviates, echoed back and run.
//
// **It can only ever see four of the five entries**, and that is the C's
// arithmetic rather than an economy here. The walk starts at `history_pos -
// 1` and runs while `cnt != starting_pos`, where starting_pos is
// `history_pos` — the slot the next line will overwrite, which is also the
// oldest one still stored. So the oldest of the five is passed over every
// time, and HISTORY_SIZE's own comment ("Keep last 5 commands") is true of
// what is kept and not of what `!` can find. See docs/weirdnumbers.md.
//
// A prefix that matches nothing is not an error: the line is returned
// unchanged, so `!zzz` reaches the interpreter as the command `!zzz` and gets
// "Huh?!?" — and, because it went through neither branch that records, is not
// added to the history either.
func (s *Session) recallHistory(line string) string {
	// skip_spaces, so `! lo` is `!lo`. Only spaces: a tab cannot get this
	// far, readLoop's filter having dropped it.
	prefix := strings.TrimLeft(line[1:], " ")

	start := s.input.pos
	cnt := start - 1
	if start == 0 {
		cnt = historySize - 1
	}
	for cnt != start {
		if entry := s.input.entries[cnt]; entry != "" && isAbbrevOf(prefix, entry) {
			s.input.last = entry
			// Echoed back, which bare `!` does not do: the player asked for
			// something by prefix and is being told what they got.
			s.Send("%s\r\n", entry)
			return entry
		}
		if cnt == 0 {
			cnt = historySize
		}
		cnt--
	}
	// Unchanged, spaces and all: the C leaves tmp alone when nothing
	// matched, so what reaches the interpreter is what was typed.
	return line
}

// isAbbrevOf is is_abbrev (interpreter.c:1145): a case-insensitive prefix
// match, false for an empty prefix.
//
// The C compares with LOWER(), which is ASCII-only. EqualFold is not, but
// the difference cannot bite here: it is applied to two byte spans of equal
// length, so a multi-byte prefix that lands mid-rune in the entry simply
// fails to match rather than matching something surprising.
func isAbbrevOf(prefix, full string) bool {
	if prefix == "" || len(prefix) > len(full) {
		return false
	}
	return strings.EqualFold(prefix, full[:len(prefix)])
}

// performSubst is perform_subst (comm.c:1874), the `^old^new` csh-ism: run
// the last command again with the first occurrence of `old` replaced.
//
// Two ways to fail, both of which the C answers "Invalid substitution." for:
// no closing `^`, and an `old` that does not appear in the last command.
//
// An empty `old` is not one of them. `^^sword` finds `strstr(orig, "")` at
// the very start of the last command and inserts there, so it prepends —
// which is a real, if accidental, feature of the C and comes out of Go's
// strings.Index the same way.
//
// And there is no trailing `^`, whatever the help file says. Stock
// CircleMUD's own `help !` — the first entry in commands.hlp, and shipped
// by this port along with everything else in that database — offers
// `^you^you doing^` as an example, which is the csh spelling. This
// implementation reads everything after the second `^` as the replacement,
// so that example substitutes "you doing^", stray caret and all. The code
// is what the players had for seven years; the help file has been wrong
// since 1993.
func performSubst(orig, subst string) (string, bool) {
	// subst[0] is the `^` that got us here.
	rest := subst[1:]
	end := strings.IndexByte(rest, '^')
	if end < 0 {
		return "", false
	}
	old, replacement := rest[:end], rest[end+1:]

	at := strings.Index(orig, old)
	if at < 0 {
		return "", false
	}
	// The C caps the result and says nothing about it
	// (`newsub[MAX_INPUT_LENGTH - 1] = '\0'`), which is the one place it
	// truncates silently on purpose. Its limit here is one character
	// longer than the one the copy loop enforces — 255 against 254, since
	// this one is not reserving room for a `$` — and the same limit is
	// used for both: a 255th character that survives a substitution but
	// not a keystroke is not a distinction worth keeping.
	out, _ := truncateInput(orig[:at] + replacement + orig[at+len(old):])
	return out, true
}
