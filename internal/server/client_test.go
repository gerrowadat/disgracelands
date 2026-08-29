// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/telnet"
)

// client is a test player at the other end of a connection.
//
// It keeps one telnet parser and one accumulated transcript for the life of
// the connection, which matters more than it looks: a helper that parses each
// read in isolation loses any telnet sequence split across two reads, and a
// helper that throws away the tail of the chunk its match landed in loses
// whatever the server said next. Both were happening, and both showed up as
// the server apparently not sending things it had sent.
type client struct {
	t    *testing.T
	conn net.Conn

	parser telnet.Parser
	// text is everything the player would have seen, negotiation removed.
	text strings.Builder
	// raw is every byte the server sent, negotiation included.
	raw []byte
}

func dialClient(t *testing.T, addr string) *client {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dialling %s: %v", addr, err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return &client{t: t, conn: conn}
}

func wrapClient(t *testing.T, conn net.Conn) *client {
	t.Helper()
	return &client{t: t, conn: conn}
}

// expect reads until want has appeared in the transcript.
//
// It returns everything seen so far rather than only the new bytes, so an
// assertion about something the server said earlier in the exchange — a
// message of the day, say, which arrives attached to the prompt after it —
// does not depend on which read happened to contain it.
func (c *client) expect(want string) string {
	c.t.Helper()

	if strings.Contains(c.text.String(), want) {
		return c.text.String()
	}

	_ = c.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	buf := make([]byte, 4096)
	for {
		n, err := c.conn.Read(buf)
		if n > 0 {
			c.raw = append(c.raw, buf[:n]...)
			c.text.Write(c.parser.Feed(nil, buf[:n]))
			// Telnet events are drained rather than kept: the tests that care
			// about negotiation read c.raw.
			c.parser.Events()
		}
		if strings.Contains(c.text.String(), want) {
			return c.text.String()
		}
		if err != nil {
			c.t.Fatalf("waiting for %q, the transcript was:\n%s\n(%v)", want, c.text.String(), err)
		}
	}
}

// expectCount reads until want has appeared n times.
//
// expect returns at once if the transcript already contains what is being
// waited for, which is right for a marker that appears only once and wrong
// for anything a second command repeats — the prompt, or a file shown twice.
func (c *client) expectCount(want string, n int) string {
	c.t.Helper()

	if strings.Count(c.text.String(), want) >= n {
		return c.text.String()
	}

	_ = c.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	buf := make([]byte, 4096)
	for {
		read, err := c.conn.Read(buf)
		if read > 0 {
			c.raw = append(c.raw, buf[:read]...)
			c.text.Write(c.parser.Feed(nil, buf[:read]))
			c.parser.Events()
		}
		if strings.Count(c.text.String(), want) >= n {
			return c.text.String()
		}
		if err != nil {
			c.t.Fatalf("waiting for %d occurrences of %q, the transcript was:\n%s\n(%v)",
				n, want, c.text.String(), err)
		}
	}
}

// sendExpectNew sends a command and waits for one *more* occurrence of want
// than the transcript already held, rather than for the first.
//
// This is the barrier to reach for whenever the marker could plausibly
// already be there — and after twoInARoom (visibility_test.go) that is
// every room name in the world the two characters have walked through,
// which is exactly the two rooms these tests move between. `expect` returns
// immediately in that case, by design and for a good reason (see its own
// comment), so:
//
//	god.send("north")
//	god.expect("The Immortal Board Room")   // matches Zod's *creation*
//	mortal.send("north")                    // races the god's move
//
// waits for nothing at all. Nothing is wrong with the god's own assertion —
// he does end up there — but the *next* line runs before the world goroutine
// has processed the move, so a test about what the second character then
// sees is a coin toss. That is the mechanism behind two flaky failures seen
// within a day of each other (#275, and TestARefusedTunnelStepCostsNoMovement
// on a PR that touched none of this), and CLAUDE.md's note that this trap
// "has recurred at least nine times" is about the same shape.
//
// Counting before the send is the whole point and is not a stylistic
// choice: doing it afterwards would race the very output being counted.
func (c *client) sendExpectNew(line, want string) string {
	c.t.Helper()

	before := strings.Count(c.text.String(), want)
	c.send(line)
	return c.expectCount(want, before+1)
}

// settle waits for everything sent so far to have been processed, by sending
// a command that always prints something and waiting for one *more* copy of
// it than the transcript already holds.
//
// It exists for commands that print nothing at all — a spell that fails
// silently, which is most of mag_alter_objs — where waiting for the prompt
// would match one that arrived before the command was even sent.
func (c *client) settle() {
	c.t.Helper()

	const marker = "o'clock"
	n := strings.Count(c.text.String(), marker)
	c.send("time")
	c.expectCount(marker, n+1)
}

// promptMarker is the one substring every prompt carries regardless of a
// character's HP/mana/move — see prompt() (interpreter.c's make_prompt,
// internal/session/commands.go).
const promptMarker = "V > "

// waitPromptCount is how many prompts have arrived so far — call before
// sending a command, and pass the result +1 to waitForPrompt afterwards.
//
// settle() does not work as a "wait for this command to finish" barrier
// when the command just sent imposes a wait state (kick/bash/backstab):
// settle()'s own probe command would be held by that wait state right
// along with anything else typed next, and hang until its own 5-second
// deadline. The prompt is sent after every command completes regardless
// of what the command itself said (interpreter.c's command_interpreter),
// so waiting for the next one works even when nothing else does.
func waitPromptCount(c *client) int { return strings.Count(c.text.String(), promptMarker) }

// waitForPrompt waits for the n'th prompt to have arrived.
func waitForPrompt(c *client, n int) { c.expectCount(promptMarker, n) }

// expectAny reads until any one of the given strings appears.
func (c *client) expectAny(wants ...string) string {
	c.t.Helper()

	// Count what is already there, so a call waits for a *new*
	// occurrence rather than matching the previous one.
	before := map[string]int{}
	for _, want := range wants {
		before[want] = strings.Count(c.text.String(), want)
	}
	grew := func() bool {
		for _, want := range wants {
			if strings.Count(c.text.String(), want) > before[want] {
				return true
			}
		}
		return false
	}

	_ = c.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	buf := make([]byte, 4096)
	for {
		n, err := c.conn.Read(buf)
		if n > 0 {
			c.raw = append(c.raw, buf[:n]...)
			c.text.Write(c.parser.Feed(nil, buf[:n]))
			c.parser.Events()
		}
		if grew() {
			return c.text.String()
		}
		if err != nil {
			c.t.Fatalf("waiting for one of %v, the transcript was:\n%s\n(%v)",
				wants, c.text.String(), err)
		}
	}
}

// seen reports whether the transcript already contains s.
func (c *client) seen(s string) bool { return strings.Contains(c.text.String(), s) }

// transcript is everything the player would have seen so far.
func (c *client) transcript() string { return c.text.String() }

// wire is every byte the server has sent, for tests about negotiation.
func (c *client) wire() []byte { return c.raw }

// gmcp returns every GMCP message the server has sent so far.
func (c *client) gmcp() []telnet.GMCPMessage {
	c.t.Helper()

	var parser telnet.Parser
	parser.Feed(nil, c.raw)

	var out []telnet.GMCPMessage
	for _, ev := range parser.Events() {
		if ev.Kind != telnet.EventSubnegotiation || ev.Option != telnet.OptGMCP {
			continue
		}
		msg, err := telnet.ParseGMCP(ev.Payload)
		if err != nil {
			c.t.Errorf("unparseable GMCP from the server: %v", err)
			continue
		}
		out = append(out, msg)
	}
	return out
}

// send types a line.
func (c *client) send(line string) {
	c.t.Helper()
	_ = c.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if _, err := c.conn.Write([]byte(line + "\r\n")); err != nil {
		c.t.Fatalf("writing %q: %v", line, err)
	}
}

// sendRaw writes bytes without a line ending, for negotiation.
func (c *client) sendRaw(b []byte) {
	c.t.Helper()
	_ = c.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if _, err := c.conn.Write(b); err != nil {
		c.t.Fatalf("writing % x: %v", b, err)
	}
}

func (c *client) close() { _ = c.conn.Close() }

// expectEOF reads until the server hangs up, and fails if it does not.
//
// It is the assertion for anything that ends with a disconnect rather than a
// message: the message on its own proves nothing, because sending it and
// closing the socket are two separate things and the C's CON_CLOSE only does
// the second one on the next pass through the game loop.
func (c *client) expectEOF() string {
	c.t.Helper()

	_ = c.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	buf := make([]byte, 4096)
	for {
		n, err := c.conn.Read(buf)
		if n > 0 {
			c.raw = append(c.raw, buf[:n]...)
			c.text.Write(c.parser.Feed(nil, buf[:n]))
			c.parser.Events()
		}
		if errors.Is(err, io.EOF) {
			return c.text.String()
		}
		if err != nil {
			c.t.Fatalf("waiting for the server to hang up, the transcript was:\n%s\n(%v)", c.text.String(), err)
		}
	}
}

// echoRestored reports whether the bytes contain IAC WONT ECHO — the server
// telling the client it may echo again.
func echoRestored(b []byte) bool {
	return bytes.Contains(b, telnet.Negotiate(telnet.WONT, telnet.OptEcho))
}

// create walks the whole creation sequence, which every test that needs a
// character in the world has to do.
func (c *client) create(name, password string, sex, class string) {
	c.t.Helper()
	c.expect("By what name")
	c.send(name)
	c.expect("Did I get that right")
	c.send("y")
	c.expect("Give me a password")
	c.send(password)
	c.expect("retype password")
	c.send(password)
	c.expect("What is your sex")
	c.send(sex)
	c.expect("Class:")
	c.send(class)
	c.expect("PRESS RETURN")
	c.send("")
	c.menuEnter()
}

// login takes an existing character in.
func (c *client) login(name, password string) {
	c.t.Helper()
	c.expect("By what name")
	c.send(name)
	c.expect("Password:")
	c.send(password)
	c.expect("PRESS RETURN")
	c.send("")
	c.menuEnter()
}

// menuEnter answers the main menu with "enter the game".
func (c *client) menuEnter() {
	c.t.Helper()
	c.expect("Make your choice:")
	c.send("1")
	c.expect("> ")
}

// eventually polls until a condition holds or the deadline passes.
//
// For the things that happen *after* a command's reply and off the world
// goroutine — a record written to disk, a file appearing. `expect` is not a
// barrier for those; see the note on settle() in TestWhisperAndAsk.
func eventually(within time.Duration, ok func() bool) bool {
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if ok() {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return ok()
}

// inWorld runs a function on the world goroutine.
//
// Every test that inspects a character's hit points, position, fight or wait
// state must go through this. Those fields are written by the violence pulse,
// the mobile-activity pulse and the mud-hour tick, all of which run on the
// world goroutine — reading them from the test goroutine is a data race, and
// the detector finds it intermittently rather than reliably, which is worse.
func inWorld(t *testing.T, srv *Server, f func(w *game.Live)) {
	t.Helper()

	// **Never call t.Fatal, t.Skip or t.FailNow inside this closure.** They
	// call runtime.Goexit, and doing that here kills the *world* goroutine:
	// every later DoSync blocks forever and the test binary hangs until its
	// timeout, with no indication of which test did it. Use t.Error and
	// return, or read the value out and assert on it afterwards.
	//
	// The panic is caught here rather than by the engine. The engine's own
	// recover is there to keep one bad command from taking the world down,
	// and it would swallow a nil dereference in a test assertion — which
	// looks exactly like the test passing.
	var panicked any
	if err := srv.engine.DoSync(context.Background(), func(w *game.Live) {
		defer func() { panicked = recover() }()
		f(w)
	}); err != nil {
		t.Fatalf("running on the world goroutine: %v", err)
	}
	if panicked != nil {
		t.Fatalf("the world closure panicked: %v", panicked)
	}
}
