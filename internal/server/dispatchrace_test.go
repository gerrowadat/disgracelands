// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// Issue #210: the dispatcher read Record.Level on the session's own
// goroutine, before it entered engine.DoSync, so anything that wrote a level
// on the world goroutine at the same moment raced it.
//
// This is the issue's own reproduction — one connection advancing another
// while that other types — and it is a `-race` test rather than an assertion
// test: there is nothing to assert afterwards, because a torn read of an
// int32 on amd64 produces a plausible level rather than a wrong one. The
// detector is the oracle. It found the original within a run or two of this
// shape, which is why the volume below is what it is: enough command
// dispatches against enough concurrent writes that the window is hit rather
// than hoped for.
//
// Everything it exercises is ordinary: two players, one of them an immortal
// advancing the other. The level read decides which command the typed word
// matches (interpreter.c:623), so it is on the path of every line anybody
// types.
func TestAdvancingSomebodyWhileTheyTypeIsNotARace(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	god, mortal := twoInARoom(t, srv, addr)

	typed := make(chan struct{})
	go func() {
		defer close(typed)
		// `look` rather than anything cheaper: it is a real command, so the
		// dispatch does the whole lookup-then-DoSync round every time.
		for range 200 {
			mortal.send("look")
		}
	}()

	// gain_exp_regardless writes GET_LEVEL on the world goroutine
	// (limits.c:357), which is the other half of the race.
	//
	// `advance` says only "Okay." to whoever ran it — the "advanced %d
	// levels" line is a mudlog, so it goes to immortals watching syslog and
	// not to the god who typed it. Hence expectCount: `expect` returns at
	// once for a marker already in the transcript, so waiting for a bare
	// "Okay." would match the first iteration's reply every time round.
	for level, n := 2, 1; level <= 30; level, n = level+1, n+1 {
		god.send(fmt.Sprintf("advance Bystander %d", level))
		god.expectCount("Okay.", n)
	}
	<-typed

	// Not an assertion about the race — see above — but the pair should still
	// be alive and talking at the end of it, which catches a fix that
	// deadlocks the dispatch instead of ordering it.
	mortal.settle()
	god.settle()
}

// Issue #251, in the shape it was reported: one connection finishing
// character creation while another player types `users`.
//
// Session.character was a plain field, written on the session's own
// goroutine at the end of login and creation and read on the *world*
// goroutine by any command that walks the descriptor list. It is an
// atomic.Pointer now — the third field of this shape to need one, after
// Session.state (#134) and the level read in Dispatcher.Do (#210).
//
// **The regression test for it lives in internal/session**
// (characterrace_test.go), and this one is the scenario rather than the
// assertion, because the window here is genuinely narrow: the write lands
// just after the DoSync that made the character, and the wizlog a few
// lines later queues a task to the world goroutine, which is a channel
// send and so an ordering edge that closes the window again within
// microseconds. That is why the original report did not reproduce on the
// next run either. This finds it some runs and not others — worth having
// for the end-to-end shape, not worth trusting as the only check.
func TestListingUsersWhileSomebodyLogsInIsNotARace(t *testing.T) {
	srv, _ := newTestServer(t)

	// Its own listener, with room for the whole crowd: the shared helper
	// allows eight connections from one address and every client here comes
	// from the loopback, so the default would refuse most of them.
	ln, err := ListenTelnet("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	accepted := make(chan struct{})
	go func() {
		defer close(accepted)
		_ = srv.Accept(ctx, ln, Limits{MaxPerHost: 64, LoginGrace: time.Minute})
	}()
	t.Cleanup(func() {
		cancel()
		<-accepted
	})
	addr := ln.Addr().String()

	god := dialClient(t, addr)
	god.create("Zod", "swordfish", "m", "w")

	// Six goroutines making thirty characters between them, all of it
	// against a `users` loop: enough arrivals at the write to be worth
	// running, cheap enough to sit in the ordinary suite.
	var joiners sync.WaitGroup
	for _, prefix := range []string{"Ana", "Bo", "Cyd", "Dee", "Eve", "Fay"} {
		joiners.Add(1)
		go func() {
			// Letters only: an all-letter name is the only kind character
			// creation accepts, so the run is spelled out rather than
			// numbered.
			defer joiners.Done()
			for _, suffix := range []string{"a", "b", "c", "d", "e"} {
				c := dialClient(t, addr)
				// Creation stopped at the MOTD rather than c.create's whole
				// sequence: pressing return enters the world, which is
				// another trip through the world goroutine and one more
				// edge that did not need to be there.
				c.expect("By what name")
				c.send(prefix + suffix)
				c.expect("Did I get that right")
				c.send("y")
				c.expect("Give me a password")
				c.send("swordfish")
				c.expect("retype password")
				c.send("swordfish")
				c.expect("What is your sex")
				c.send("m")
				c.expect("Class:")
				c.send("w")
				c.expect("PRESS RETURN")
				// Hanging up rather than pressing return leaves the
				// teardown to read the same field on the connection
				// goroutine, which is the other unordered read.
				c.close()
			}
		}()
	}

	joined := make(chan struct{})
	go func() {
		joiners.Wait()
		close(joined)
	}()

	// `users` walks every session and reads each one's character — do_users'
	// own loop over descriptor_list (users.go, `who = s.Character()`) — on
	// the world goroutine. expectCount, because expect returns at once for a
	// marker already in the transcript.
	for n := 1; ; n++ {
		god.send("users")
		god.expectCount("visible sockets connected.", n)
		select {
		case <-joined:
			return
		default:
		}
	}
}
