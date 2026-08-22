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

// Script is a list of lines to type, in order.
//
// A line beginning with '#' is a comment and is not sent; a blank line *is*
// sent, because pressing return at the MOTD prompt is a real thing a player
// does and the two servers must agree about it.
type Script []string

// ParseScript reads a script from text.
func ParseScript(body string) Script {
	var s Script
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		s = append(s, line)
	}
	// A trailing newline in the file is not a line the player typed.
	for len(s) > 0 && s[len(s)-1] == "" {
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
	if opt.Deadline <= 0 {
		opt.Deadline = 60 * time.Second
	}

	conn, err := net.DialTimeout("tcp", opt.Addr, 10*time.Second)
	if err != nil {
		return "", fmt.Errorf("dialling %s: %w", opt.Addr, err)
	}
	defer func() { _ = conn.Close() }()

	if err := conn.SetDeadline(time.Now().Add(opt.Deadline)); err != nil {
		return "", err
	}

	var out strings.Builder
	reader := bufio.NewReader(conn)

	// The greeting arrives unprompted, before anything is typed.
	drain(reader, conn, &out, opt.Quiet)

	for _, line := range script {
		if _, err := fmt.Fprintf(conn, "%s\r\n", line); err != nil {
			return out.String(), fmt.Errorf("sending %q: %w", line, err)
		}
		out.WriteString("\n>>> " + line + "\n")
		drain(reader, conn, &out, opt.Quiet)
	}
	return out.String(), nil
}

// drain reads until the server has been silent for the quiet period.
func drain(r *bufio.Reader, conn net.Conn, out *strings.Builder, quiet time.Duration) {
	buf := make([]byte, 4096)
	for {
		_ = conn.SetReadDeadline(time.Now().Add(quiet))
		n, err := r.Read(buf)
		if n > 0 {
			out.Write(buf[:n])
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
// It is **not** part of Normalise and is off by default, deliberately. The C
// emits colour and this port emits none — `color` is unported and nothing
// writes an escape sequence — and that is the largest single difference
// between the two servers. Normalising it away by default would be hiding the
// harness's loudest finding inside the harness.
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
		{regexp.MustCompile(`It is \d+ o'clock (am|pm)`), "It is <HOUR> o'clock", "mud clock"},
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

// Diff reports the lines that differ, with a little context.
//
// Deliberately not a full Myers diff: the two transcripts are the same script
// played twice, so they line up until they do not, and the first divergence is
// what anybody reading this wants. Anything cleverer would be hiding the
// answer inside a nicer presentation of it.
func Diff(want, got string) string {
	wantLines := strings.Split(want, "\n")
	gotLines := strings.Split(got, "\n")

	var b strings.Builder
	shown := 0
	for i := 0; i < len(wantLines) || i < len(gotLines); i++ {
		w, g := at(wantLines, i), at(gotLines, i)
		if w == g {
			continue
		}
		if shown >= 40 {
			fmt.Fprintf(&b, "    ... and more, from line %d\n", i+1)
			break
		}
		fmt.Fprintf(&b, "    line %d\n     C: %q\n    Go: %q\n", i+1, w, g)
		shown++
	}
	return b.String()
}

func at(lines []string, i int) string {
	if i < len(lines) {
		return lines[i]
	}
	return "<end of transcript>"
}
