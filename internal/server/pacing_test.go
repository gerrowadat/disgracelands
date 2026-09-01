// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"strings"
	"testing"
	"time"
)

// One command per pulse (#386), porting game_loop's own `GET_WAIT_STATE(
// d->character) = 1` after every dequeue (comm.c:829).
//
// These are wall-clock tests and are therefore written to be one-sided: they
// assert a *floor* on how long a burst takes and never a ceiling, because a
// busy machine can always make something slower and nothing can make it
// faster than the pacing allows. The interval is a short one for the same
// reason testRoundLength is short — at the real 100ms a burst of six would
// spend most of the test asleep — and the floor is derived from whatever
// interval the test asked for rather than written out as a number.

// roomsSeen counts how many times the start room has already been named.
//
// Entering the world runs a look, so the room is in the transcript once
// before any of these tests types anything — and an expectCount that forgets
// it is satisfied one command early, which makes a burst of five look like a
// burst of four and a 160ms floor like 158ms. That is CLAUDE.md's
// expectCount trap arriving through the back door: the count is right and
// the baseline is not.
func roomsSeen(c *client) int {
	return strings.Count(c.transcript(), startRoomName)
}

// startRoomName is where these tests' characters are. The first character
// created on an empty roster is promoted to implementor, and an immortal
// starts in the board room rather than the temple — which is worth naming
// once rather than being surprised by in five assertions.
const startRoomName = "The Immortal Board Room"

// testCommandInterval is short enough to keep the suite quick and long
// enough that a burst of commands cannot finish inside timing noise.
const testCommandInterval = 40 * time.Millisecond

// TestCommandsArePacedAtOnePerInterval is the whole of #386.
func TestCommandsArePacedAtOnePerInterval(t *testing.T) {
	srv, _ := newTestServerPaced(t, testCommandInterval)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	// Written all at once, the way a paste or a client macro arrives. The
	// server gets them in one read; what stops them running together is the
	// pacing and nothing else.
	const burst = 5
	before := roomsSeen(c)
	start := time.Now()
	for i := 0; i < burst; i++ {
		c.send("look")
	}
	c.expectCount(startRoomName, before+burst)
	took := time.Since(start)

	// Four gaps between five commands. The first is free — an idle
	// connection acts at once, as one whose wait state is already zero does
	// in the C.
	floor := time.Duration(burst-1) * testCommandInterval
	if took < floor {
		t.Errorf("%d commands took %v, want at least %v: they are not being paced",
			burst, took, floor)
	}
}

// TestPacingDoesNotOutlastIdleness: the ration is one command per interval,
// not a queue that keeps filling. A player who waits between commands is
// never made to wait again.
func TestPacingDoesNotOutlastIdleness(t *testing.T) {
	srv, _ := newTestServerPaced(t, testCommandInterval)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	c.send("look")
	c.expectCount(startRoomName, roomsSeen(c)+1)
	time.Sleep(3 * testCommandInterval)

	before := roomsSeen(c)
	start := time.Now()
	c.send("look")
	c.expectCount(startRoomName, before+1)
	if took := time.Since(start); took > testCommandInterval {
		t.Errorf("a command after an idle spell waited %v; the ration does not accumulate", took)
	}
}

// TestPacingDoesNotStackWithASkillsOwnLag.
//
// The C sets its 1 *before* running the command, so `kick` overwrites it
// with three rounds and the larger wins rather than the two adding up. Here
// the same falls out of both being absolute moments: a pace inside a longer
// wait is absorbed by it.
//
// Asserted as a ceiling rather than a floor, unusually for this file,
// because the failure being guarded against is the pace being *added* to the
// wait — and that is a thing only a bug can do.
func TestPacingDoesNotStackWithASkillsOwnLag(t *testing.T) {
	srv, _ := newTestServerPaced(t, testCommandInterval)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	// testRoundLength is the wait a skill costs in this harness. Two
	// commands back to back: the second waits for the skill's lag, and must
	// not then wait for the pacing on top of it.
	c.send("look")
	c.expectCount(startRoomName, roomsSeen(c)+1)

	before := roomsSeen(c)
	start := time.Now()
	c.send("look")
	c.expectCount(startRoomName, before+1)
	if took := time.Since(start); took > testCommandInterval+testRoundLength {
		t.Errorf("a paced command took %v, more than the pace and a round together", took)
	}
}

// TestAConnectionWithNoCharacterIsNotPaced is the C's `if (d->character)`
// guard, and the one place the two servers disagree about who it covers.
//
// The C allocates a char_data at CON_GET_NAME, so it paces the name and
// password lines too. This port does not build one until the login succeeds,
// so the guard — the same guard — lets the whole login sequence through at
// once. Worth a test because it is the difference, not the similarity.
func TestAConnectionWithNoCharacterIsNotPaced(t *testing.T) {
	srv, _ := newTestServerPaced(t, 2*time.Second)
	c := dialClient(t, listening(t, srv))

	// Creating a character is eight lines. At a two-second pace that is
	// sixteen seconds if every one of them is paced, and about one interval
	// if only the tail is — the name, the confirmation, both passwords, the
	// sex and the class all arrive before there is a character to pace, and
	// the menu line arrives after. So the assertion is "nothing like eight
	// intervals" rather than "instant", which would be false.
	start := time.Now()
	c.create("Zod", "swordfish", "m", "w")
	if took := time.Since(start); took > 6*time.Second {
		t.Errorf("logging in took %v; the lines typed before a character exists are being paced", took)
	}
}

// TestPacingIsOffByDefaultInTests guards the harness itself rather than the
// server: every other test in this package sends commands back to back and
// would slow to a crawl if the default changed, which is a thing that would
// be noticed as "the suite got slower" rather than as a failure.
func TestPacingIsOffByDefaultInTests(t *testing.T) {
	srv, _ := newTestServer(t)
	if srv.commandEvery != 0 {
		t.Errorf("the test harness paces at %v; it should not pace at all", srv.commandEvery)
	}

	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	before := roomsSeen(c)
	start := time.Now()
	for i := 0; i < 5; i++ {
		c.send("look")
	}
	c.expectCount(startRoomName, before+5)
	if took := time.Since(start); took > time.Second {
		t.Errorf("five unpaced commands took %v", took)
	}
}
