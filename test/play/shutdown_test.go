// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

//go:build play

package play

import (
	"strings"
	"testing"
	"time"
)

// Shutting the server down, which is the one operation an operator performs
// on a live game more often than any other and the one nothing else in the
// tree can test.
//
// The reason it needs a real process: the whole of the graceful path is
// signal handling, context cancellation and goroutine ordering in
// cmd/dlmud's own run(). None of that exists when internal/server's tests
// build a Server by hand, and the ordering is exactly where it went wrong --
// see TestShutdownSavesEveryoneStillInTheWorld.

// TestShutdownSavesEveryoneStillInTheWorld.
//
// A player who is logged in and playing when the server is stopped must not
// lose what they were doing. Crash_save_all is what the C does on its way
// down (comm.c:428) and Server.SaveEverything is the port of it, and the
// whole of it runs through engine.DoSync -- which is a handshake with a
// running world goroutine.
//
// This is the test that found the world goroutine being cancelled first: the
// saves then had nobody to hand their work to, sat in DoSync's select until
// the 30-second shutdown deadline, and returned "context deadline exceeded".
// Every shutdown took half a minute and saved nothing, and the only sign of
// it was one ERROR line after the process had already given up. Nothing that
// stops short of sending a real SIGTERM to a real server and then looking at
// what is on disk can see that.
func TestShutdownSavesEveryoneStillInTheWorld(t *testing.T) {
	m := start(t, miniClassic)
	c := m.dial()
	c.create("Tourist", "tourpass", "m", "w")

	// Something worth losing: coins, and a pack of things.
	c.toRoom(roomSparringRing)
	c.do("get all")
	contains(t, "before the shutdown", c.do("score"), "75 gold coins")

	// Stopped with the player still connected and still standing there.
	started := time.Now()
	m.stop()

	// A shutdown that saves nothing is also a shutdown that takes the full
	// deadline to do it, because every DoSync waits out its own context.
	// The bound is deliberately far below cmd/dlmud's 30-second
	// shutdownTimeout and far above what a working save costs.
	if took := time.Since(started); took > 15*time.Second {
		t.Errorf("the shutdown took %s, which is the shape of a save that never reached the world", took)
	}
	for _, line := range m.errorLines() {
		t.Errorf("the shutdown logged an error: %s", line)
	}
	if _, ok := m.find("stopped"); !ok {
		t.Fatalf("the server never finished shutting down. Its log was:\n%s", m.logText())
	}

	// And the proof: a new server on the same data directory hands it all
	// back.
	m2 := startAt(t, miniClassic, m.dir, startOptions{noFounder: true})
	back := m2.dial()
	back.login("Tourist", "tourpass")

	contains(t, "the gold survived", back.do("score"), "75 gold coins")
	contains(t, "the pack survived", back.do("inventory"), "a wand", "a staff", "a potion")

	m2.noServerErrors()
}

// TestShutdownFromInsideTheGame. `shutdown` typed by an implementor runs the
// same path a signal does -- deliberately, because a shutdown that skipped
// the saves would be worse than no shutdown command at all (cmd/dlmud's own
// comment on it).
func TestShutdownFromInsideTheGame(t *testing.T) {
	m := start(t, miniClassic)
	god := m.god()
	god.do("set Founder gold 250")

	god.send("shutdown")

	// The process going away is the assertion: the listener closes, the
	// saves run, and run() returns.
	if !eventually(30*time.Second, func() bool {
		_, ok := m.find("stopped")
		return ok
	}) {
		t.Fatalf("the server did not stop when told to from inside the game. Its log was:\n%s", m.logText())
	}
	if _, ok := m.find("shutdown requested from inside the game"); !ok {
		t.Errorf("the shutdown was not logged as coming from in-game:\n%s", m.logText())
	}
	for _, line := range m.errorLines() {
		t.Errorf("the shutdown logged an error: %s", line)
	}

	m2 := startAt(t, miniClassic, m.dir, startOptions{noFounder: true})
	back := m2.dial()
	back.login(founderName, founderPassword)
	contains(t, "what was saved on the way down", back.do("score"), "250 gold coins")

	m2.noServerErrors()
}

// TestTheServerRefusesToBootOnADirectoryItCannotRead is the other half of
// the operator's day: a misconfiguration should be a startup failure with
// something to read, not a server that comes up and serves half a game.
func TestTheServerRefusesToBootWithoutAWorld(t *testing.T) {
	dir := t.TempDir()

	out, err := runServer(t, "--lib-dir="+dir, "--listen-telnet=127.0.0.1:0",
		"--listen-telnets=", "--metrics-addr=")
	if err == nil {
		t.Fatalf("the server booted on an empty directory; it said:\n%s", out)
	}
	if !strings.Contains(out, "dlmud:") {
		t.Errorf("the failure was not reported on stderr in the usual shape:\n%s", out)
	}

	t.Logf("refused an empty --lib-dir with:\n%s", out)
}
