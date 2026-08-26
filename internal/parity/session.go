// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

// Package parity drives a scripted session against a running MUD and returns
// what it said back.
//
// This is the other half of the world-parity harness: that one compares what
// the two servers *loaded*, and this one compares what they *say*. Plan §11
// calls it the missing piece, and it is — everything else in the suite compares
// the Go against a reading of the C or against an oracle written by hand, and
// a reading is exactly what has been wrong repeatedly.
package parity

import (
	"bufio"
	"fmt"
	"net"
	"regexp"
	"strings"
	"time"
)

// Step is one line of a script: a line to type, or the one directive the
// format has.
type Step struct {
	// Line is what to type.
	Line string
	// Reconnect closes the connection and opens a new one instead of typing
	// anything. It is how a scenario has more than one character: a script
	// is one connection, and the first character created on an empty roster
	// is an implementor (db.c's "if this is our first player --- he be
	// God"), so a scenario that needs both a god and a mortal has to be
	// able to hang up and dial again.
	//
	// Type `quit` before it rather than relying on it: dropping a socket
	// with a character still in the world is a different thing on the two
	// servers, and comparing *that* is a scenario of its own rather than an
	// accident of every scenario.
	Reconnect bool
}

// Script is a list of steps, in order.
//
// A line beginning with '#' is a comment and is not sent; a blank line *is*
// sent, because pressing return at the MOTD prompt is a real thing a player
// does and the two servers must agree about it. The single line `!reconnect`
// is the one directive.
type Script []Step

// Reconnect is the directive a script line has to be, exactly, to hang up
// and dial again.
const Reconnect = "!reconnect"

// reconnectPause is how long to wait between hanging up and dialling again.
const reconnectPause = time.Second

// ParseScript reads a script from text.
func ParseScript(body string) Script {
	var s Script
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		if strings.TrimSpace(line) == Reconnect {
			s = append(s, Step{Reconnect: true})
			continue
		}
		s = append(s, Step{Line: line})
	}
	// A trailing newline in the file is not a line the player typed.
	for len(s) > 0 && s[len(s)-1] == (Step{}) {
		s = s[:len(s)-1]
	}
	return s
}

// Options are the knobs a run needs.
type Options struct {
	// Addr is host:port of a plaintext telnet listener.
	Addr string
	// Quiet is how long the server must say nothing before the driver decides
	// a command's output is complete. Both servers get the same value, so a
	// slow one produces a short transcript rather than a wrong comparison.
	Quiet time.Duration
	// FirstByte is how long to wait for an answer to *begin*, which is a
	// different question from how long to wait for it to finish and wants a
	// much longer answer.
	//
	// Framing purely by silence conflates them, and the difference is not
	// academic: the C server queues its output and flushes it on its own
	// pulse, so its greeting can take longer to arrive than a whole command's
	// output takes to finish once it has started. Waited on as silence, that
	// read as "the C server said nothing when it connected", and every block
	// in the transcript after it lined up against the wrong one.
	FirstByte time.Duration
	// Deadline bounds the whole run.
	Deadline time.Duration
}

// Run connects, plays the script, and returns the transcript.
//
// Framing is by silence rather than by prompt, deliberately. Waiting for a
// prompt means knowing what a prompt looks like on both servers, and they
// differ — the whole point of the exercise is to find out where. Silence is a
// property of the connection and needs no agreement.
func Run(script Script, opt Options) (string, error) {
	if opt.Quiet <= 0 {
		opt.Quiet = 250 * time.Millisecond
	}
	if opt.FirstByte <= 0 {
		opt.FirstByte = 3 * time.Second
	}
	if opt.Deadline <= 0 {
		opt.Deadline = 60 * time.Second
	}
	until := time.Now().Add(opt.Deadline)

	var out strings.Builder
	conn, reader, err := connect(opt, until, &out)
	if err != nil {
		return "", err
	}
	defer func() { _ = conn.Close() }()

	for _, step := range script {
		if step.Reconnect {
			_ = conn.Close()
			out.WriteString("\n" + blockMarker + " " + Reconnect + "\n")
			// A pause before dialling again, the same on both sides. The
			// character that just quit is saved off the world goroutine
			// here (CLAUDE.md, "Saves are pushed off the world goroutine on
			// purpose"), and the roster index it has to appear in is
			// written after the record is: reconnecting the instant the
			// socket closes is a race with a save, and losing it looks like
			// the port having forgotten a character.
			time.Sleep(reconnectPause)
			if conn, reader, err = connect(opt, until, &out); err != nil {
				return out.String(), err
			}
			continue
		}
		if _, err := fmt.Fprintf(conn, "%s\r\n", step.Line); err != nil {
			return out.String(), fmt.Errorf("sending %q: %w", step.Line, err)
		}
		out.WriteString("\n" + blockMarker + " " + step.Line + "\n")
		drain(reader, conn, &out, opt)
	}
	return out.String(), nil
}

// connect dials and reads the greeting, which arrives unprompted before
// anything is typed.
func connect(opt Options, until time.Time, out *strings.Builder) (net.Conn, *bufio.Reader, error) {
	conn, err := net.DialTimeout("tcp", opt.Addr, 10*time.Second)
	if err != nil {
		return nil, nil, fmt.Errorf("dialling %s: %w", opt.Addr, err)
	}
	if err := conn.SetDeadline(until); err != nil {
		_ = conn.Close()
		return nil, nil, err
	}
	reader := bufio.NewReader(conn)
	drain(reader, conn, out, opt)
	return conn, reader, nil
}

// drain reads an answer: up to FirstByte for it to begin, then until the
// server has been silent for Quiet.
func drain(r *bufio.Reader, conn net.Conn, out *strings.Builder, opt Options) {
	buf := make([]byte, 4096)
	wait := opt.FirstByte
	for {
		_ = conn.SetReadDeadline(time.Now().Add(wait))
		n, err := r.Read(buf)
		if n > 0 {
			out.Write(buf[:n])
			wait = opt.Quiet
		}
		if err != nil {
			return
		}
	}
}

// ansi matches an SGR escape sequence.
var ansi = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// StripColour removes ANSI colour, for a run hunting differences other than
// the one big known one.
//
// It is **not** part of Normalise and is off by default, deliberately. Both
// servers turn colour on for a new character, but the C colours what it
// prints at the call site — CCCYN() around a room name, CCYEL() around the
// fight (screen.h:42) — and this port renders only the &-codes embedded in
// text, so the C's transcript has an escape sequence on lines this one
// leaves plain. It is the largest single difference between the two
// servers, and normalising it away by default would be hiding the harness's
// loudest finding inside the harness. test/parity strips it everywhere
// except in the one scenario that is *about* it.
func StripColour(s string) string { return ansi.ReplaceAllString(s, "") }

// Normalise removes what cannot agree between two servers and is not worth
// making agree.
//
// Every rule here is a decision to *stop* comparing something, so each one is
// named and justified. Anything not listed is compared byte for byte.
func Normalise(s string) string {
	// Telnet negotiation. The C sends IAC sequences around the password
	// prompt and this port negotiates properly, which is a deliberate and
	// documented difference (docs/deviations.md) rather than a bug to find.
	s = stripTelnet(s)

	for _, r := range []struct {
		re   *regexp.Regexp
		with string
		why  string
	}{
		// The clock. `time`, `date` and the login banner all print one, and
		// two servers started a second apart will differ.
		{regexp.MustCompile(`\b\d{1,2}:\d{2}:\d{2}\b`), "<TIME>", "wall clock"},
		{regexp.MustCompile(`\b(Mon|Tue|Wed|Thu|Fri|Sat|Sun) [A-Z][a-z]{2} [ \d]\d \d{2}:\d{2}:\d{2} \d{4}\b`), "<DATE>", "asctime"},
		// Mud time advances while the script runs, and the two servers boot
		// seconds apart, so the hour can differ across a long script.
		//
		// The weekday and the calendar date go with it, and that was found
		// the expensive way: the hour alone was normalised at first, on the
		// reasoning that an hour is the granularity that moves. A mud hour
		// is 75 real seconds and a mud day is 24 of them (utils.h:109-110),
		// so a day is 1800 — and the two transcripts are taken about fifteen
		// seconds apart, one server after the other, which puts a day
		// rollover between them roughly one run in a hundred and twenty. It
		// duly landed on one, a day after the suite first went green, and
		// reported the port's calendar as wrong when nothing was: both
		// servers compute the date from the same epoch with the same
		// formula, and one of them was simply asked later. The weekday moves
		// with it rather than independently — the C derives it, `weekday =
		// ((35 * month) + day) % 7` (act.informative.c:896) — so a rollover
		// changes both lines at once, which is what the failure looked like.
		//
		// The cost, stated plainly: this suite no longer compares the mud
		// calendar at all. That is the right trade anyway, and not only for
		// the flake — `mud_time_passed` is pure arithmetic over elapsed
		// seconds, which CLAUDE.md says to check with a C oracle rather than
		// by eye. Two transcripts sampled fifteen seconds apart were never
		// going to be evidence about it. Freezing the clock in both servers
		// (the shape `--freeze-mobiles` already has) is what would make it
		// comparable here, if it is ever worth comparing here.
		{regexp.MustCompile(`It is \d+ o'clock (am|pm), on .*`), "It is <HOUR> o'clock, on <WEEKDAY>", "mud clock"},
		{regexp.MustCompile(`The \d+(st|nd|rd|th) Day of the .*, Year \d+\.`), "The <DATE>, Year <YEAR>.", "mud calendar"},
		// Uptime, for the same reason.
		{regexp.MustCompile(`Up since .*`), "Up since <UPTIME>", "uptime"},
		// The port numbers differ because the two servers cannot share one.
		{regexp.MustCompile(`\b(port|Port) \d+\b`), "port <PORT>", "listening port"},
	} {
		s = r.re.ReplaceAllString(s, r.with)
	}

	// Trailing whitespace on a line is invisible and the two servers disagree
	// about it in places that do not matter — a column-padded listing whose
	// last column is empty.
	var b strings.Builder
	for _, line := range strings.Split(s, "\n") {
		b.WriteString(strings.TrimRight(line, " \t\r"))
		b.WriteString("\n")
	}
	return b.String()
}

// stripTelnet removes IAC sequences, which are protocol rather than text.
func stripTelnet(s string) string {
	const iac, sb, se = 255, 250, 240
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != iac {
			b.WriteByte(s[i])
			continue
		}
		if i+1 >= len(s) {
			return b.String()
		}
		if s[i+1] == sb {
			// Subnegotiation: skip to IAC SE.
			for i += 2; i+1 < len(s) && (s[i] != iac || s[i+1] != se); i++ {
			}
			i++
			continue
		}
		// IAC WILL/WONT/DO/DONT x, or IAC IAC.
		i += 2
	}
	return b.String()
}
