// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// The browser terminal's key handling.
//
// Two halves, and only one of them is Go. The client-side logic lives in a
// <script> inside playTemplate and nothing here executes it — there is no
// JavaScript engine in this project's toolchain and adding one to run a
// forty-line handler would be a poor trade. So what is tested here is the
// half that *is* reachable: the server-side assumption the whole approach
// rests on, which is executable and which would silently invalidate the
// client if it ever changed, plus a contract check that the page still
// carries every sequence it has to.

// TestTheServerReadsAnEscapeSequenceAsPlainText is why arrow keys have to be
// swallowed in the browser rather than ignored at the far end.
//
// readLoop (internal/session/session.go) assembles a line by appending every
// byte that is not a line ending, a NUL or an erase. It has no notion of an
// escape sequence, so ESC [ A arrives as three perfectly ordinary characters
// of a command — which is exactly what a player saw before this: an arrow
// key at the name prompt answered "Names may only contain letters."
//
// If server-side escape handling is ever added, this test fails, and the
// browser's ARROW table should be revisited at the same time.
func TestTheServerReadsAnEscapeSequenceAsPlainText(t *testing.T) {
	srv, _ := newTestServer(t)
	ts := listeningWeb(t, srv, "", false)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, toWS(t, ts.URL)+"/ws", nil)
	if err != nil {
		t.Fatalf("dialing /ws: %v", err)
	}
	defer c.Close(websocket.StatusNormalClosure, "")

	conn := websocket.NetConn(ctx, c, websocket.MessageText)
	buf := make([]byte, 8192)
	if _, err := conn.Read(buf); err != nil {
		t.Fatalf("reading the greeting: %v", err)
	}

	// An up-arrow, as xterm.js would have sent it before this change.
	if _, err := conn.Write([]byte("\x1b[A\r\n")); err != nil {
		t.Fatalf("sending an escape sequence: %v", err)
	}
	got := readUntilString(t, conn, "By what name", buf)
	if !strings.Contains(got, "Names may only contain letters.") {
		t.Errorf("after an escape sequence the server said %q, want it "+
			"treated as ordinary (invalid) name text", got)
	}
}

// TestBackspaceOverTheWebSocketErases is the browser half of #233, driven
// over a real WebSocket because that is the client that always sends its
// keystrokes one at a time.
//
// The page erases a backspace off the screen itself (playTemplate's consume)
// and the byte still goes to the server, so what the player sees is only
// what the game got if the server erases too. Before the fix this exact
// sequence answered "Names may only contain letters." for a name that read
// "Newcomer" on screen.
func TestBackspaceOverTheWebSocketErases(t *testing.T) {
	srv, _ := newTestServer(t)
	ts := listeningWeb(t, srv, "", false)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, toWS(t, ts.URL)+"/ws", nil)
	if err != nil {
		t.Fatalf("dialing /ws: %v", err)
	}
	defer c.Close(websocket.StatusNormalClosure, "")

	conn := websocket.NetConn(ctx, c, websocket.MessageText)
	buf := make([]byte, 8192)
	readUntilString(t, conn, "By what name", buf)

	if _, err := conn.Write([]byte("Newcomerr\x7f\r\n")); err != nil {
		t.Fatalf("sending a name with a backspace in it: %v", err)
	}
	got := readUntilString(t, conn, "(Y/N)", buf)
	if !strings.Contains(got, "Did I get that right, Newcomer (Y/N)?") {
		t.Errorf("the server read something other than what the page shows "+
			"the player:\n%s", got)
	}
}

// TestBackspaceErasesAWholeRune: the erase is one *character*, not one
// byte, which is where this port has to part company with the C.
//
// process_input drops everything that is not `isascii && isprint`
// (comm.c:1796), so a multi-byte character could never reach the buffer it
// erases from — one byte was always one character there, and the question
// never came up. This port has no such filter and takes UTF-8 names:
// invalidName reads them with unicode.IsLetter. Erasing a byte left half a
// rune behind, so "Zoë", backspace, "e" reached the login as "Zo\xc3e" and
// answered "Names may only contain letters." for a name that read "Zoe" on
// screen — the same class of bug as #233 itself, one layer down.
func TestBackspaceErasesAWholeRune(t *testing.T) {
	srv, _ := newTestServer(t)
	ts := listeningWeb(t, srv, "", false)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, toWS(t, ts.URL)+"/ws", nil)
	if err != nil {
		t.Fatalf("dialing /ws: %v", err)
	}
	defer c.Close(websocket.StatusNormalClosure, "")

	conn := websocket.NetConn(ctx, c, websocket.MessageText)
	buf := make([]byte, 8192)
	readUntilString(t, conn, "By what name", buf)

	if _, err := conn.Write([]byte("Zoë\x7fe\r\n")); err != nil {
		t.Fatalf("sending a name with a multi-byte character in it: %v", err)
	}
	got := readUntilString(t, conn, "(Y/N)", buf)
	if !strings.Contains(got, "Did I get that right, Zoe (Y/N)?") {
		t.Errorf("erasing a multi-byte character left part of it behind:\n%s", got)
	}
}

// TestThePlayPageSwallowsEveryArrowKey is a contract check on the rendered
// page, not an execution of it: it asserts the eight sequences a cursor key
// can arrive as are all accounted for.
//
// Four keys but eight sequences, and the second four are the ones easily
// forgotten: a terminal in cursor-key mode (DECCKM) sends ESC O A rather
// than ESC [ A. Nothing in this game turns that mode on, so the second set
// would never be noticed missing until something did.
func TestThePlayPageSwallowsEveryArrowKey(t *testing.T) {
	srv, _ := newTestServer(t)
	ts := listeningWeb(t, srv, "", false)

	resp, err := http.Get(ts.URL + "/play")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	page := string(body)

	for _, seq := range []string{
		`\x1b[A`, `\x1b[B`, `\x1b[C`, `\x1b[D`,
		`\x1bOA`, `\x1bOB`, `\x1bOC`, `\x1bOD`,
	} {
		if !strings.Contains(page, "'"+seq+"'") {
			t.Errorf("/play does not handle the escape sequence %s", seq)
		}
	}

	// Up is the only one that does anything, and a password must never
	// become the thing it repeats — lastCommand is only ever assigned
	// under the echo guard.
	if !strings.Contains(page, "if (arrow === 'up') repeat();") {
		t.Error("/play does not wire up-arrow, and only up-arrow, to the repeat")
	}
	if !strings.Contains(page, "if (echo && line !== '') lastCommand = line;") {
		t.Error("/play records a command for repeat without checking local echo, " +
			"which would let up-arrow replay a password in the clear")
	}
	// And a repeat is only injected into an empty line: it is sent as
	// text plus an Enter, so with a half-typed line already in the
	// server's buffer the two would run together. Since #233 an erase
	// takes a character back out of that buffer as well as off the
	// screen, so 'line' is exactly what the server is holding and there
	// is nothing else to count.
	if !strings.Contains(page, "if (!lastCommand || line !== '' || !localEcho) return;") {
		t.Error("/play does not gate the up-arrow repeat on an empty line")
	}
}
