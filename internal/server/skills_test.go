// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"context"
	"testing"
	"time"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// TestASkillYouDoNotHave is refused, in the C's two different wordings.
func TestASkillYouDoNotHave(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	// A mortal warrior: the Implementor knows everything.
	first := dialClient(t, addr)
	first.create("Zod", "swordfish", "m", "w")
	first.send("quit")
	first.expect("Goodbye")
	first.close()

	c := dialClient(t, addr)
	c.create("Welmar", "hunter2!", "m", "w")

	for _, tc := range []struct{ command, expect string }{
		{"kick someone", "You have no idea how."},
		{"bash someone", "You have no idea how."},
		// backstab and rescue say it differently.
		{"backstab someone", "You have no idea how to do that."},
		{"rescue someone", "You have no idea how to do that."},
	} {
		c.send(tc.command)
		c.expect(tc.expect)
	}
}

// TestKickLands, and costs three rounds of lag.
func TestKickLands(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	spawnDog(t, srv, ImmortStartRoom)

	// Whether the kick's own message is a hit or a miss, wording and all,
	// is now a property of misc/messages — internal/game/fightmessages_test.go
	// and internal/server/damagemessages_test.go already cover the
	// dispatch itself, and a test server with none configured (the
	// default — see TestDeathBlowAgainstTheRealArchive's own doc comment)
	// sends nothing at all for a miss with nothing registered. `settle()`
	// itself does not work as the barrier here: kick's own wait state
	// would hold `settle()`'s own probe command right along with anything
	// else typed next, so this waits on the next occurrence of the
	// prompt instead — sent after every command completes, regardless of
	// what the command itself said.
	beforePrompts := waitPromptCount(c)
	c.send("kick dog")
	waitForPrompt(c, beforePrompts+1)

	if err := srv.engine.DoSync(context.Background(), func(w *game.Live) {
		if remaining := w.Find("Zod").WaitRemaining(); remaining < 4*time.Second {
			t.Errorf("kick left %s of lag, want three combat rounds", remaining)
		}
	}); err != nil {
		t.Fatal(err)
	}
}

// TestBashNeedsAWeapon, and knocks the basher over on a miss.
func TestBashNeedsAWeapon(t *testing.T) {
	srv, _ := newTestServer(t)
	// data/misc/messages' one registered bash entry (skill 132) names the
	// dog in all three of its die/miss/hit attacker texts — real data,
	// loaded so the swing actually says something to wait on.
	loadRealFightMessages(t, srv)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")
	spawnDog(t, srv, ImmortStartRoom)

	c.send("bash dog")
	c.expect("You need to wield a weapon to make it a success.")

	// With a weapon it works.
	drop(t, srv, testSwordVnum, ImmortStartRoom)
	c.send("get sword")
	c.expect("You get a long sword.")
	c.send("wield sword")
	c.expect("You wield a long sword.")

	c.send("bash dog")
	c.expect("large dog")
}

// TestBackstabNeedsAPiercingWeapon. A long sword slashes; only a piercing
// weapon will do.
func TestBackstabNeedsAPiercingWeapon(t *testing.T) {
	srv, _ := newTestServer(t)
	// data/misc/messages' one registered backstab entry (skill 131) names
	// the dog in all three of its die/miss/hit attacker texts.
	loadRealFightMessages(t, srv)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")
	spawnDog(t, srv, ImmortStartRoom)

	c.send("backstab dog")
	c.expect("You need to wield a weapon to make it a success.")

	// The test sword's fourth value is 3 — a slashing weapon.
	drop(t, srv, testSwordVnum, ImmortStartRoom)
	c.send("get sword")
	c.expect("You get a long sword.")
	c.send("wield sword")
	c.expect("You wield a long sword.")

	c.send("backstab dog")
	c.expect("Only piercing weapons can be used for backstabbing.")

	// Make it a piercing weapon and it works.
	if err := srv.engine.DoSync(context.Background(), func(w *game.Live) {
		w.Find("Zod").Equipment[game.WearWield].Values[3] = game.AttackPierce
	}); err != nil {
		t.Fatal(err)
	}
	c.send("backstab dog")
	c.expect("large dog")
}

// TestYouCannotBackstabSomebodyAlreadyFighting.
func TestYouCannotBackstabSomebodyAlreadyFighting(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	dog := spawnDog(t, srv, ImmortStartRoom)
	other, _ := place(t, srv, fighterRecord("Welmar", 10, 200), ImmortStartRoom)

	if err := srv.engine.DoSync(context.Background(), func(w *game.Live) {
		w.SetFighting(dog, other)
		sword := w.NewObject(testSwordVnum)
		sword.Values[3] = game.AttackPierce
		zod := w.Find("Zod")
		w.ObjectToChar(sword, zod)
		w.Equip(sword, zod, game.WearWield)
	}); err != nil {
		t.Fatal(err)
	}

	c.send("backstab dog")
	c.expect("You can't backstab a fighting person -- they're too alert!")
}

// TestRescueTakesTheFight.
func TestRescueTakesTheFight(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	dog := spawnDog(t, srv, ImmortStartRoom)
	victim, _ := place(t, srv, fighterRecord("Welmar", 10, 200), ImmortStartRoom)

	// Nobody is fighting them yet.
	c.send("rescue welmar")
	c.expect("But nobody is fighting Welmar!")

	if err := srv.engine.DoSync(context.Background(), func(w *game.Live) {
		w.SetFighting(dog, victim)
		w.SetFighting(victim, dog)
	}); err != nil {
		t.Fatal(err)
	}

	c.send("rescue welmar")
	c.expectAny("Banzai!  To the rescue...", "You fail the rescue!")

	if c.seen("Banzai") {
		// Read on the world goroutine. Everything here is mutated by the
		// violence pulse, and reading it from the test goroutine is a data
		// race the detector will and did find.
		if err := srv.engine.DoSync(context.Background(), func(w *game.Live) {
			if dog.Fighting == victim {
				t.Error("the dog is still fighting the rescued character")
			}
			if victim.Fighting != nil {
				t.Error("the rescued character is still fighting")
			}
			if remaining := victim.WaitRemaining(); remaining <= 0 {
				t.Error("the rescued character got no confusion lag")
			}
		}); err != nil {
			t.Fatal(err)
		}
	}
}

// TestRescuingYourself and other refusals.
func TestRescueRefusals(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")
	dog := spawnDog(t, srv, ImmortStartRoom)

	c.send("rescue zod")
	c.expect("What about fleeing instead?")

	if err := srv.engine.DoSync(context.Background(), func(w *game.Live) {
		w.SetFighting(w.Find("Zod"), dog)
	}); err != nil {
		t.Fatal(err)
	}
	c.send("rescue dog")
	c.expect("How can you rescue someone you are trying to kill?")
}

// TestAWaitStateDelaysTheNextCommand rather than refusing it, which is what
// the C does by not reading the descriptor.
func TestAWaitStateDelaysTheNextCommand(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")
	spawnDog(t, srv, ImmortStartRoom)

	// Waits for the next prompt rather than the kick's own reply or
	// settle()'s usual probe command — see TestKickLands's own comment
	// for why: kick's own wait state would hold settle()'s "time" right
	// along with anything else typed next.
	beforePrompts := waitPromptCount(c)
	c.send("kick dog")
	waitForPrompt(c, beforePrompts+1)

	// The next command is held for the wait, not rejected. Shorten it so the
	// test does not take six seconds.
	if err := srv.engine.DoSync(context.Background(), func(w *game.Live) {
		w.Find("Zod").BusyUntil = time.Now().Add(300 * time.Millisecond)
	}); err != nil {
		t.Fatal(err)
	}

	// Count the occurrences already in the transcript: the room name has been
	// printed once on entry, and expect would match that instantly.
	start := time.Now()
	c.send("look")
	c.expectCount("The Immortal Board Room", 2)
	if elapsed := time.Since(start); elapsed < 250*time.Millisecond {
		t.Errorf("the next command ran after %s, want it held for the wait", elapsed)
	}
}

// TestBackstabMultiplierJumpsForImmortals — 20, not a continuation of the
// curve.
func TestBackstabMultiplierJumpsForImmortals(t *testing.T) {
	for _, tc := range []struct{ level, want int32 }{
		{0, 1}, {1, 2}, {7, 2}, {8, 3}, {13, 3}, {14, 4}, {20, 4},
		{21, 5}, {28, 5}, {29, 6}, {30, 6}, {31, 20}, {34, 20},
	} {
		if got := game.BackstabMultiplier(tc.level); got != tc.want {
			t.Errorf("BackstabMultiplier(%d) = %d, want %d", tc.level, got, tc.want)
		}
	}
}
