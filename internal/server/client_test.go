// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"bytes"
	"context"
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

// inWorld runs a function on the world goroutine.
//
// Every test that inspects a character's hit points, position, fight or wait
// state must go through this. Those fields are written by the violence pulse,
// the mobile-activity pulse and the mud-hour tick, all of which run on the
// world goroutine — reading them from the test goroutine is a data race, and
// the detector finds it intermittently rather than reliably, which is worse.
func inWorld(t *testing.T, srv *Server, f func(w *game.Live)) {
	t.Helper()
	if err := srv.engine.DoSync(context.Background(), f); err != nil {
		t.Fatalf("running on the world goroutine: %v", err)
	}
}
