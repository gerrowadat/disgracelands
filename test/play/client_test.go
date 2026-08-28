// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

//go:build play

package play

import (
	"errors"
	"io"
	"net"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/gerrowadat/disgracelands/internal/telnet"
)

// client is one player at the other end of a socket.
//
// The parser and the transcript live for the length of the connection rather
// than the length of a read, for the reason internal/server's own client
// documents: a telnet sequence split across two reads is lost by a helper
// that parses each read alone, and the tail of the chunk a match landed in is
// lost by one that throws it away. Both were real, and both looked like the
// server not sending things it had sent.
type client struct {
	t    *testing.T
	conn net.Conn

	parser telnet.Parser
	// text is what the player would have seen: negotiation removed, colour
	// removed. Colour goes too because this suite asserts on sentences, and
	// a room name arrives wrapped in an escape sequence -- matching "The
	// Armory" should not depend on whether the player has ANSI on.
	text strings.Builder
}

// timeout bounds every read. Generous, because the server may be race-
// instrumented and this may be a loaded CI machine; still finite, because a
// wedged read should fail as a test rather than as a 10-minute package
// timeout with no indication of which test hung.
const timeout = 20 * time.Second

// ansi is the colour the server writes. See colour.
var ansi = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// dial connects a new player to the server.
func (m *mud) dial() *client {
	m.t.Helper()

	conn, err := net.Dial("tcp", m.addr)
	if err != nil {
		m.t.Fatalf("dialling %s: %v", m.addr, err)
	}
	m.t.Cleanup(func() { _ = conn.Close() })
	return &client{t: m.t, conn: conn}
}

// read pulls once from the socket into the transcript.
func (c *client) read() error {
	_ = c.conn.SetReadDeadline(time.Now().Add(timeout))
	buf := make([]byte, 8192)
	n, err := c.conn.Read(buf)
	if n > 0 {
		c.text.WriteString(ansi.ReplaceAllString(string(c.parser.Feed(nil, buf[:n])), ""))
		// Drained rather than kept: nothing in this suite asserts on
		// negotiation, which is internal/server's TestTelnetNegotiation.
		c.parser.Events()
	}
	return err
}

// expect reads until want has appeared in the transcript.
//
// Like internal/server's, it returns at once if the transcript already
// contains want -- which is right for a marker that appears once, and wrong
// for anything a later command repeats. Prefer do() and assert on what it
// returns; that has the ambiguity designed out of it rather than documented.
func (c *client) expect(want string) string {
	c.t.Helper()

	for !strings.Contains(c.text.String(), want) {
		if err := c.read(); err != nil {
			c.t.Fatalf("waiting for %q, the transcript was:\n%s\n(%v)", want, c.text.String(), err)
		}
	}
	return c.text.String()
}

// promptMarker is the one substring every prompt carries whatever the
// character's hit points, mana and movement are -- make_prompt
// (interpreter.c), ported in internal/session.
const promptMarker = "V > "

// do types a command and returns everything printed before the next prompt.
//
// This is the primitive the rest of the suite is written in, and it is worth
// being clear about why it is not `send` + `expect`. The prompt is written
// after every command completes, whatever the command itself said
// (command_interpreter, interpreter.c) -- so waiting for a *new* prompt is a
// real barrier for "this command has finished", including for the commands
// that print nothing at all and for the ones that impose a wait state, where
// nothing else works. And slicing the transcript at that barrier means an
// assertion is about the command just typed rather than about anything the
// server said earlier: the single most persistent trap in internal/server's
// suite, where an `expect` written after a second command matches the first
// command's reply, has recurred at least nine times. Here it cannot.
func (c *client) do(command string) string {
	c.t.Helper()

	mark := c.text.Len()
	want := strings.Count(c.text.String(), promptMarker) + 1
	c.send(command)
	c.expectCount(promptMarker, want)

	return c.text.String()[mark:]
}

// expectCount reads until want has appeared n times in the whole transcript.
func (c *client) expectCount(want string, n int) string {
	c.t.Helper()

	for strings.Count(c.text.String(), want) < n {
		if err := c.read(); err != nil {
			c.t.Fatalf("waiting for %d occurrences of %q, the transcript was:\n%s\n(%v)",
				n, want, c.text.String(), err)
		}
	}
	return c.text.String()
}

// doUntil types a command and reads until marker appears, returning what was
// printed in between.
//
// For the commands that do not return to a prompt: anything that opens the
// editor, anything that opens the pager, and the menu. do() would block on
// those until its read deadline.
func (c *client) doUntil(command, marker string) string {
	c.t.Helper()

	mark := c.text.Len()
	want := strings.Count(c.text.String(), marker) + 1
	c.send(command)
	c.expectCount(marker, want)

	return c.text.String()[mark:]
}

// send types a line.
func (c *client) send(line string) {
	c.t.Helper()
	_ = c.conn.SetWriteDeadline(time.Now().Add(timeout))
	if _, err := c.conn.Write([]byte(line + "\r\n")); err != nil {
		c.t.Fatalf("typing %q: %v", line, err)
	}
}

// transcript is everything the player has seen.
func (c *client) transcript() string { return c.text.String() }

func (c *client) close() { _ = c.conn.Close() }

// expectEOF reads until the server hangs up, and fails if it does not.
//
// The assertion for anything that ends in a disconnect: the message on its
// own proves nothing, because saying it and closing the socket are two
// separate things.
func (c *client) expectEOF() string {
	c.t.Helper()

	for {
		err := c.read()
		if errors.Is(err, io.EOF) {
			return c.text.String()
		}
		if err != nil {
			c.t.Fatalf("waiting for the server to hang up, the transcript was:\n%s\n(%v)",
				c.text.String(), err)
		}
	}
}

// create walks the whole creation sequence and enters the game, leaving the
// character standing in the mortal start room.
func (c *client) create(name, password, sex, class string) {
	c.t.Helper()

	c.expect("By what name do you wish to be known?")
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
	c.enterGame()
}

// login takes an existing character in.
func (c *client) login(name, password string) {
	c.t.Helper()

	c.expect("By what name do you wish to be known?")
	c.send(name)
	c.expect("Password:")
	c.send(password)
	c.expect("PRESS RETURN")
	c.send("")
	c.enterGame()
}

// enterGame answers the main menu with "1) Enter the game."
func (c *client) enterGame() {
	c.t.Helper()
	c.expect("Make your choice:")
	c.doUntil("1", promptMarker)
}

// quit leaves the game the plain way, which in CircleMUD closes the
// connection rather than returning to the menu (do_quit, act.other.c).
//
// The "Goodbye, friend.. Come back soon!" is printed by the command itself;
// the save, the crash-save and taking the character out of the world happen
// afterwards, in the connection's teardown. So seeing the goodbye is a
// barrier for the command and not for the save -- anything that then reads
// the file the quit was supposed to write has to wait for it separately,
// which is what eventually is for.
func (c *client) quit() {
	c.t.Helper()
	c.doUntil("quit", "Come back soon!")
}

// walk types a sequence of movement commands and returns what the last one
// printed. The tour is sixteen rooms in a line, so nearly every test starts
// by walking some of it.
func (c *client) walk(dirs ...string) string {
	c.t.Helper()

	var last string
	for _, d := range dirs {
		last = c.do(d)
	}
	return last
}

// north walks n rooms up the corridor, which is how the tour is addressed:
// the start room is 3001 and each feature room is one further north.
func (c *client) north(n int) string {
	c.t.Helper()

	dirs := make([]string, n)
	for i := range dirs {
		dirs[i] = "north"
	}
	return c.walk(dirs...)
}

// contains fails the test unless out holds every one of want.
//
// Taking a list rather than one string on purpose: a command's whole output
// is usually several things at once -- the room name, the exits, what is on
// the floor -- and asserting them one call at a time buries the transcript
// that would explain the failure under repetition.
func contains(t *testing.T, what, out string, want ...string) {
	t.Helper()

	for _, w := range want {
		if !strings.Contains(out, w) {
			t.Errorf("%s: expected %q in the output, which was:\n%s", what, w, out)
		}
	}
}

// missing fails the test if out holds any of unwanted.
func missing(t *testing.T, what, out string, unwanted ...string) {
	t.Helper()

	for _, w := range unwanted {
		if strings.Contains(out, w) {
			t.Errorf("%s: did not expect %q in the output, which was:\n%s", what, w, out)
		}
	}
}

// containsAny reports whether out holds at least one of want.
func containsAny(out string, want ...string) bool {
	for _, w := range want {
		if strings.Contains(out, w) {
			return true
		}
	}
	return false
}
