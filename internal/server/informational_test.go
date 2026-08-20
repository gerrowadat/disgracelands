// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"strings"
	"testing"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// TestGoldHasThreeAnswers.
func TestGoldHasThreeAnswers(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	set := func(n int32) {
		inWorld(t, srv, func(w *game.Live) { w.Find("Zod").Record.Points.Gold = n })
	}

	set(0)
	c.send("gold")
	c.expect("You're broke!")

	set(1)
	c.send("gold")
	c.expect("You have one miserable little gold coin.")

	set(250)
	c.send("gold")
	c.expect("You have 250 gold coins.")
}

// TestSplitDividesAmongTheGroupAndKeepsTheRemainder.
//
// Three people splitting ten coins get three each, and the odd coin stays
// with the splitter — who is charged for the two shares they hand over and
// nothing else.
func TestSplitDividesAmongTheGroupAndKeepsTheRemainder(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	bob, bobClient := place(t, srv, fighterRecord("Bob", 30, 100), ImmortStartRoom)
	cid, _ := place(t, srv, fighterRecord("Cid", 30, 100), ImmortStartRoom)

	inWorld(t, srv, func(w *game.Live) {
		zod := w.Find("Zod")
		zod.Record.Points.Gold = 100
		w.AddFollower(bob, zod)
		w.AddFollower(cid, zod)
		zod.SetGrouped(true)
		bob.SetGrouped(true)
		cid.SetGrouped(true)
		bob.Record.Points.Gold = 0
		cid.Record.Points.Gold = 0
	})

	c.send("split 10")
	c.expect("You split 10 coins among 3 members -- 3 coins each.")

	inWorld(t, srv, func(w *game.Live) {
		if bob.Record.Points.Gold != 3 || cid.Record.Points.Gold != 3 {
			t.Errorf("shares are %d and %d, want 3 each",
				bob.Record.Points.Gold, cid.Record.Points.Gold)
		}
		// 100, less the two shares handed over, *plus* the remainder — which
		// the C adds back to somebody who never gave it away. Ten coins split
		// three ways leaves 101 in the world. See docs/weirdnumbers.md.
		if got := w.Find("Zod").Record.Points.Gold; got != 95 {
			t.Errorf("the splitter has %d gold, want 95", got)
		}
	})
	if !bobClient.said("Zod splits 10 coins; you receive 3.") {
		t.Error("Bob was not told about his share")
	}

	c.send("split 1000")
	c.expect("You don't seem to have that much gold to split.")
}

// TestWimpyRefusesTheSillyValues.
func TestWimpyRefusesTheSillyValues(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	c.send("wimpy")
	c.expect("At the moment, you're not a wimp.  (sure, sure...)")

	c.send("wimpy 99999")
	c.expect("That doesn't make much sense, now does it?")

	// Half the maximum is the ceiling; a new implementor has 500.
	c.send("wimpy 400")
	c.expect("You can't set your wimp level above half your hit points.")

	c.send("wimpy 100")
	c.expect("Okay, you'll wimp out if you drop below 100 hit points.")

	c.send("wimpy")
	c.expect("Your current wimp level is 100 hit points.")

	c.send("wimpy 0")
	c.expect("Okay, you'll now tough out fights to the bitter end.")

	c.send("wimpy lots")
	c.expect("Specify at how many hit points you want to wimp out at.")
}

// TestPromptChoosesWhatTheNumbersAre.
func TestPromptChoosesWhatTheNumbersAre(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	c.send("prompt none")
	c.expect("Okay.")
	inWorld(t, srv, func(w *game.Live) {
		p := w.Find("Zod").Record.Preferences
		if p.HasAny(game.PrefDisplayHP | game.PrefDisplayMana | game.PrefDisplayMove) {
			t.Error("prompt none left something switched on")
		}
	})

	c.send("prompt hv")
	c.expectCount("Okay.", 2)
	inWorld(t, srv, func(w *game.Live) {
		p := w.Find("Zod").Record.Preferences
		if !p.Has(game.PrefDisplayHP) || !p.Has(game.PrefDisplayMove) {
			t.Error("prompt hv did not set hit points and movement")
		}
		if p.Has(game.PrefDisplayMana) {
			t.Error("prompt hv set mana as well")
		}
	})

	c.send("prompt x")
	c.expect("Usage: prompt { { H | M | V } | all | none }")
}

// TestToggleShowsEverySetting.
func TestToggleShowsEverySetting(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	c.send("wimpy 50")
	c.expect("Okay, you'll wimp out")

	c.send("toggle")
	got := c.expect("Gossip Channel")

	for _, want := range []string{
		"Hit Pnt Display", "Brief Mode", "Summon Protect",
		"Wimp Level: 50",
		// An implementor sees the immortal block as well.
		"No Hassle", "Holylight", "Room Flags",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("toggle is missing %q:\n%s", want, got)
		}
	}
}

// TestTitleRefusesBrackets, which is the C's one rule about it.
func TestTitleRefusesBrackets(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	c.send("title the Magnificent")
	c.expect("Okay, you're now Zod the Magnificent.")

	c.send("title (the Sneaky)")
	c.expect("Titles can't contain the ( or ) characters.")

	c.send("title " + strings.Repeat("x", 90))
	c.expect("Sorry, titles can't be longer than 80 characters.")
}

// TestReportTellsTheGroup.
func TestReportTellsTheGroup(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	c.send("report")
	c.expect("But you are not a member of any group!")

	member, listener := place(t, srv, fighterRecord("Bob", 30, 100), ImmortStartRoom)
	inWorld(t, srv, func(w *game.Live) {
		zod := w.Find("Zod")
		w.AddFollower(member, zod)
		zod.SetGrouped(true)
		member.SetGrouped(true)
	})

	c.send("report")
	c.expect("You report to the group.")
	if !listener.said("Zod reports: 500/500H") {
		t.Error("the group did not get the report")
	}
}

// TestWhereListsYourZone, and not the one next door.
func TestWhereListsYourZone(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	near, _ := place(t, srv, fighterRecord("Bob", 10, 100), ImmortStartRoom)
	far, _ := place(t, srv, fighterRecord("Cid", 10, 100), MortalStartRoom)
	_, _ = near, far

	c.send("where")
	c.settle()
	got := c.transcript()
	if !strings.Contains(got, "Bob") {
		t.Errorf("somebody in the same zone is missing:\n%s", got)
	}
	if strings.Contains(got[strings.LastIndex(got, "Players in your Zone"):], "Cid") {
		t.Errorf("somebody in another zone was listed:\n%s", got)
	}

	c.send("where nobody")
	c.expect("No-one around by that name.")
}

// TestLevelsAndDiagnose.
func TestLevelsAndDiagnose(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	c.send("levels")
	got := c.expect("Immortality")
	if !strings.Contains(got, "[ 1]") {
		t.Errorf("the levels table has no level one:\n%s", got)
	}

	victim, _ := place(t, srv, fighterRecord("Bob", 10, 100), ImmortStartRoom)
	inWorld(t, srv, func(_ *game.Live) { victim.Record.Points.Hit = 50 })

	c.send("diagnose bob")
	c.expect("Bob has quite a few wounds.")

	c.send("diagnose")
	c.expect("Diagnose who?")
}

// TestTheCannedTextCommands.
func TestTheCannedTextCommands(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	c.send("motd")
	c.expect("Mortal news.")

	// The test data directory has no news file, so the command says so
	// rather than printing nothing.
	c.send("news")
	c.expect("There is no news.")

	c.send("version")
	c.expect("A Go port of CircleMUD, version 3.00 beta patchlevel 19")

	c.send("whoami")
	c.expect("Zod")
}

// TestCommandsAndSocialsList.
func TestCommandsAndSocialsList(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	c.send("commands")
	c.settle()
	got := c.transcript()
	if !strings.Contains(got, "The following commands are available to you:") {
		t.Fatalf("no command list:\n%s", got)
	}
	for _, want := range []string{"north", "score", "gossip"} {
		if !strings.Contains(got, want) {
			t.Errorf("the command list is missing %q", want)
		}
	}
	if strings.Contains(got[strings.LastIndex(got, "The following commands"):], "smile") {
		t.Error("a social is listed among the commands")
	}

	c.send("socials")
	c.settle()
	got = c.transcript()
	if !strings.Contains(got, "The following socials are available to you:") {
		t.Fatalf("no socials list:\n%s", got)
	}
	if !strings.Contains(got, "smile") {
		t.Errorf("the socials list has no smile:\n%s", got)
	}
}
