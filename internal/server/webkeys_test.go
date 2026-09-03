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

// TestRecalledTextWaitsForItsEnter is the server-side assumption #369's
// up-arrow rests on, driven over a real WebSocket for the same reason the
// backspace tests are.
//
// Recall types the last command back at the server without the Enter that
// would run it, exactly as if the player had typed it again, and then lets
// them edit it. That only works if two things hold at this end: text with
// no line ending after it runs nothing, and the Enter that eventually
// arrives runs the whole accumulated line including whatever was added
// after the recall. Both are readLoop's own behaviour
// (internal/session/session.go) rather than anything the browser can
// arrange, and neither is obvious enough to leave unasserted — the old
// up-arrow sent its own '\r' precisely because it did not depend on this.
func TestRecalledTextWaitsForItsEnter(t *testing.T) {
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

	// The recall: characters, no Enter. Nothing should happen yet.
	if _, err := conn.Write([]byte("Newcome")); err != nil {
		t.Fatalf("sending recalled text: %v", err)
	}

	// Then the edit the recall exists to allow, and only then the Enter.
	if _, err := conn.Write([]byte("r\r\n")); err != nil {
		t.Fatalf("sending the edit and the Enter: %v", err)
	}
	got := readUntilString(t, conn, "(Y/N)", buf)
	if !strings.Contains(got, "Did I get that right, Newcomer (Y/N)?") {
		t.Errorf("recalled text plus an edit did not reach the server as one "+
			"line:\n%s", got)
	}
	// And the recall alone must not have run: a premature line would have
	// asked about "Newcome" first, and that answer would still be sitting
	// in the transcript ahead of the real one.
	if strings.Contains(got, "Newcome (Y/N)?") {
		t.Errorf("text with no line ending after it was run as a command:\n%s", got)
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
	// become the thing it recalls — lastCommand is only ever assigned
	// under the echo guard.
	if !strings.Contains(page, "if (arrow === 'up') recall();") {
		t.Error("/play does not wire up-arrow, and only up-arrow, to the recall")
	}
	if !strings.Contains(page, "if (echo && line !== '') lastCommand = line;") {
		t.Error("/play records a command for recall without checking local echo, " +
			"which would let up-arrow put a password back on the line in the clear")
	}
	// And a recall is only injected into an empty line: the text lands in
	// whatever the server is already holding, so with a half-typed line
	// there the two would run together. Since #233 an erase takes a
	// character back out of that buffer as well as off the screen, so
	// 'line' is exactly what the server is holding and there is nothing
	// else to count.
	if !strings.Contains(page, "if (!lastCommand || line !== '' || !localEcho) return;") {
		t.Error("/play does not gate the up-arrow recall on an empty line")
	}

	// #369: up-arrow restores the line for editing, it does not run it.
	// The recall sends the text and no Enter, which is the whole of the
	// difference — a `type(lastCommand + '\r')` here would be the old
	// behaviour spelled differently, so the assertion is on the call
	// itself rather than on the helper.
	if !strings.Contains(page, "function typeText(text) {") ||
		!strings.Contains(page, "\t\tws.send(text);\n\t\tconsume(text, localEcho);") {
		t.Error("/play does not have a helper that types text with no Enter after it")
	}
	if !strings.Contains(page, "\t\ttypeText(lastCommand);\n") {
		t.Error("/play does not recall the last command as editable text; " +
			"up-arrow must restore the line, not run it (#369)")
	}
	if strings.Contains(page, "lastCommand + '\\r'") {
		t.Error("/play appends an Enter to the recalled command, which runs it " +
			"instead of restoring it for editing (#369)")
	}
}

// TestALeadingBackspaceErasesNothing is the server half of "backspace does
// not eat the prompt".
//
// The page stops drawing the erase once the line is empty, but it still
// sends the byte -- it has no other way to stay in step with a server that
// might be holding something it is not. So the two only agree if a
// backspace against an empty buffer is a no-op at this end too. readLoop
// guards it with `len(line) > 0` and its comment says erasing nothing is
// not an error, which is `write_point > tmp` in the C (comm.c:1787); this
// asserts it rather than trusting the comment, because the page's decision
// to keep sending depends on it.
func TestALeadingBackspaceErasesNothing(t *testing.T) {
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

	// Backspace held down at a fresh prompt, then a perfectly good name.
	if _, err := conn.Write([]byte("\x7f\x7f\x7f\x7fNewcomer\r\n")); err != nil {
		t.Fatalf("sending backspaces at an empty line: %v", err)
	}
	got := readUntilString(t, conn, "(Y/N)", buf)
	if !strings.Contains(got, "Did I get that right, Newcomer (Y/N)?") {
		t.Errorf("backspaces against an empty buffer were not ignored:\n%s", got)
	}
}

// TestThePlayPageWillNotEraseThePrompt is the browser half of the same
// thing, and a contract check on the rendered page for the same reason the
// arrow-key test is one: there is no JavaScript engine here to run it.
//
// A backspace at an empty line has nothing of the player's to the left of
// the cursor, so a '\b \b' walks into the prompt instead -- hold the key
// down at "By what name do they call you?" and the question goes away a
// character at a time (#394), which is not something any terminal driver
// would do.
// The guard is on the echo, not on the send: the byte still goes out (see
// TestALeadingBackspaceErasesNothing).
func TestThePlayPageWillNotEraseThePrompt(t *testing.T) {
	srv, _ := newTestServer(t)
	ts := listeningWeb(t, srv, "", false)

	resp, err := http.Get(ts.URL + "/play")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	page := string(body)

	if !strings.Contains(page, `if (echo && line !== '') term.write('\b \b');`) {
		t.Error("/play erases a backspace off the screen without checking that " +
			"the player has anything on the line, which lets a backspace at " +
			"an empty prompt eat the prompt itself")
	}
}
