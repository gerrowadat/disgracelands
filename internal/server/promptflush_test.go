// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// A prompt after output you did not ask for (#385), which is the C's
// process_output appending make_prompt to whatever it flushes
// (comm.c:1469) plus the loop that prompts anybody it missed
// (comm.c:865-869).
//
// The prompt is where your hit points are, so the thing these are really
// about is that the numbers move when the world hurts you rather than only
// when you type.

// vitalsPrompt matches a whole playing prompt, for the one assertion that
// reads the numbers out of one. Everything else counts promptMarker, which
// is the harness's own constant for the same thing.
var vitalsPrompt = regexp.MustCompile(`\d+H \d+M \d+V >`)

// promptCount is how many prompts this client has been sent so far.
//
// Counted rather than looked for, because every assertion in this file is
// about how *many* arrived and one is always already there.
func promptCount(c *client) int { return strings.Count(c.transcript(), promptMarker) }

// pulsePrompts runs one pulse of the sweep.
//
// Called by hand because this package's harness never installs the periodic
// work — engine.SetPeriodic is cmd/dlmud's job, so nothing ticks in these
// tests and every other tick test drives its pass the same way. That the
// sweep is *on* the pulse at all is asserted separately, in
// TestThePromptSweepIsOnEveryPulse, because these tests cannot see it.
func pulsePrompts(t *testing.T, srv *Server) {
	t.Helper()
	if err := srv.engine.DoSync(context.Background(), srv.flushPrompts); err != nil {
		t.Fatal(err)
	}
}

// settledPromptCount is promptCount taken at a moment when everything the
// server meant to send has been sent.
//
// The barrier is settle plus a wait for settle's *own* prompt, and both
// halves are needed. settle returns on the "o'clock" line, which the server
// writes before the prompt that follows it, so settle alone proves nothing
// about prompts — an earlier draft subtracted a constant for it and raced.
// Waiting for that prompt to land does prove it, and it proves it for
// anything queued earlier too: one connection's writes are ordered, so a
// stray prompt from a pulse before this one has arrived by the time this
// one has.
func settledPromptCount(t *testing.T, c *client) int {
	t.Helper()

	before := promptCount(c)
	c.settle()
	c.expectCount(promptMarker, before+1)
	return promptCount(c)
}

// TestSomebodySpeakingBringsAPrompt is the case that started #385.
func TestSomebodySpeakingBringsAPrompt(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	// The first character on an empty roster is an implementor and starts
	// in the board room; the second is a mortal in the temple. Only the
	// implementor can `goto`, so it is the listener that moves.
	listener := dialClient(t, addr)
	listener.create("Zod", "swordfish", "m", "w")
	talker := dialClient(t, addr)
	talker.create("Welmar", "swordfish", "m", "w")
	listener.send("goto 3001")
	listener.expect("The Temple Of Midgaard")

	before := settledPromptCount(t, listener)

	talker.send("say hello there")
	// The talker's own reply proves the say ran on the world goroutine, and
	// the listener's line was queued inside that same task — so the debt is
	// recorded by the time the sweep runs.
	talker.expect("You say")
	pulsePrompts(t, srv)

	// Blocks until the prompt arrives, and fails with the transcript if it
	// never does — which is what this whole issue was.
	listener.expectCount(promptMarker, before+1)
	if !strings.Contains(listener.transcript(), "Welmar says, 'hello there'") {
		t.Errorf("the listener never heard it:\n%s", listener.transcript())
	}
}

// TestABlowBringsAPromptWithTheNewNumbers is what the issue is actually
// about. A fight the player is not typing into still has to move the
// numbers, because those numbers are how you know to run.
func TestABlowBringsAPromptWithTheNewNumbers(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	before := settledPromptCount(t, c)
	mark := len(c.transcript())

	// Hurt them from the world's side, the way a combat round does, with
	// nothing typed.
	if err := srv.engine.DoSync(context.Background(), func(w *game.Live) {
		who := w.Find("Zod")
		attacker := &game.Character{
			Name: "a jackal", NPC: true, Position: game.PosStanding,
			MobDef: &game.MobDef{Vnum: 999, ShortDesc: "a jackal"},
			Record: &game.PlayerRecord{Name: "a jackal", Level: 1,
				Points: game.Points{Hit: 100, MaxHit: 100}},
		}
		if err := w.Enter(attacker, who.Room); err != nil {
			t.Errorf("placing the attacker: %v", err)
			return
		}
		// A real swing rather than Damage with a number: Damage alone
		// sends no message (its callers print their own), and a blow
		// nobody is told about owes no prompt — correctly. hit is what a
		// combat round does, and it always says something, hit or miss.
		srv.hit(w, attacker, who)
	}); err != nil {
		t.Fatal(err)
	}
	pulsePrompts(t, srv)
	c.expectCount(promptMarker, before+1)

	// And it carries the reduced figure, which is the whole point: a prompt
	// with stale numbers would pass the count above and still be useless.
	var hp int32
	inWorld(t, srv, func(w *game.Live) {
		if who := w.Find("Zod"); who != nil {
			hp = who.Record.Points.Hit
		}
	})
	shown := vitalsPrompt.FindAllString(c.transcript()[mark:], -1)
	if len(shown) == 0 {
		t.Fatalf("no prompt after being hit:\n%q", c.transcript()[mark:])
	}
	if want := strconv.Itoa(int(hp)) + "H"; !strings.Contains(shown[len(shown)-1], want) {
		t.Errorf("the prompt says %q, want it to show %s", shown[len(shown)-1], want)
	}
}

// TestAQuietConnectionIsNotPromptedRepeatedly.
//
// The sweep runs every pulse. It must prompt on *new* output and not on the
// mere passage of time, or an idle player's screen fills with prompts —
// which is the C's `has_prompt` staying set once written, spelled the other
// way round.
func TestAQuietConnectionIsNotPromptedRepeatedly(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	before := settledPromptCount(t, c)

	// Five pulses' worth of nothing happening.
	for i := 0; i < 5; i++ {
		pulsePrompts(t, srv)
	}

	// One more prompt, and it is settle's own.
	if got := settledPromptCount(t, c); got != before+1 {
		t.Errorf("an idle connection gained %d prompts across five pulses, want none but settle's own",
			got-before-1)
	}
}

// TestACommandIsNotPromptedTwice: the dispatcher already sends one, so the
// sweep must find the debt settled. Before the debt was cleared at both of
// the dispatcher's exits this produced a second prompt a pulse after every
// command.
func TestACommandIsNotPromptedTwice(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	// All three exits from Dispatcher.Do: a command that runs, a bare Enter,
	// and a word that means nothing. Each builds its prompt somewhere
	// different, and each has to settle the debt.
	for _, typed := range []string{"look", "", "notacommand"} {
		t.Run("after "+quoted(typed), func(t *testing.T) {
			before := settledPromptCount(t, c)

			c.send(typed)
			c.expectCount(promptMarker, before+1)
			pulsePrompts(t, srv)

			// One for the command, one for settle. A duplicate from the
			// sweep would have been written before settle's and so would
			// be counted here.
			if got := settledPromptCount(t, c); got != before+2 {
				t.Errorf("%d prompts for one command, want 1 (plus settle's)", got-before-1)
			}
		})
	}
}

func quoted(s string) string {
	if s == "" {
		return "an empty line"
	}
	return s
}

// TestThePromptSweepIsOnEveryPulse is the wiring the tests above cannot see.
//
// They call flushPrompts by hand, because this package's harness never
// installs the periodic work. So the thing that makes it happen on a real
// server — an entry on the schedule, every pulse — is asserted here, and it
// is `Every: 1` rather than any larger number because the C flushes output
// on every pass of game_loop and not on a subdivision of it.
func TestThePromptSweepIsOnEveryPulse(t *testing.T) {
	srv, _ := newTestServer(t)

	for _, p := range srv.Periodic() {
		if p.Name != "prompts" {
			continue
		}
		if p.Every != 1 {
			t.Errorf("the prompt sweep runs every %d pulses, want every one", p.Every)
		}
		return
	}
	t.Error("nothing on the schedule flushes prompts; unsolicited output will carry none")
}
