// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"fmt"
	"testing"
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
