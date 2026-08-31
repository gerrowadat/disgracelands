// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"context"
	"testing"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// Wimpiness: the two calls to do_flee that damage() makes on the victim's
// behalf (fight.c:898-912), neither of which this port had until #375.
//
// The player half is the issue: `wimpy` set a threshold, `toggle` displayed
// it and the record saved it across a logout, and nothing in the game ever
// read the number back — so a player who had asked to run away at thirty hit
// points stood there and died. The mobile half (MOB_WIMPY) sits three lines
// above it in the same branch of the same switch and was missing for the
// same reason.

// hitFor lands one blow of a given size, on the world goroutine.
func hitFor(t *testing.T, srv *Server, attacker, victim *game.Character, amount int32) {
	t.Helper()
	if err := srv.engine.DoSync(context.Background(), func(w *game.Live) {
		srv.Damage(w, attacker, victim, amount)
	}); err != nil {
		t.Fatal(err)
	}
}

// whereIs reads where somebody is, by pointer rather than by name: the
// existing roomOf takes a name and goes through w.Find, which is no use for
// a mobile whose name is its short description.
func whereIs(t *testing.T, srv *Server, who *game.Character) game.RoomVnum {
	t.Helper()
	var room game.RoomVnum
	inWorld(t, srv, func(w *game.Live) { room = who.Room })
	return room
}

// hitUntilTheyFlee swings until the victim leaves the room, and reports
// whether they ever did.
//
// **For no damage, deliberately, and it is not a trick to keep them alive.**
// damage() runs its whole tail for a miss — the C calls
// `damage(ch, victim, 0, w_type)` for one (fight.c:1067-1068) rather than
// taking a separate path — so a *missed* swing at somebody already below
// their wimp level makes them run, on the real server and now here. That is
// what makes an unbounded number of attempts safe: nobody's hit points move.
//
// Attempts are needed at all because do_flee rolls. It picks a random
// direction up to six times and gives up if none of them is a way out
// (act.offensive.c), and the test world's temple has exactly one exit, so a
// single attempt misses it about a third of the time. Twenty is what
// fleeUntilItWorks already allows the `flee` command for the same reason.
func hitUntilTheyFlee(t *testing.T, srv *Server, attacker, victim *game.Character) bool {
	t.Helper()

	from := whereIs(t, srv, victim)
	for attempt := 1; attempt <= 20; attempt++ {
		hitFor(t, srv, attacker, victim, 0)
		if whereIs(t, srv, victim) != from {
			return true
		}
	}
	return false
}

// TestAPlayerBelowTheirWimpLevelFleesWhenHit is #375 itself.
func TestAPlayerBelowTheirWimpLevelFleesWhenHit(t *testing.T) {
	srv, _ := newTestServer(t)

	attacker, _ := place(t, srv, fighterRecord("Zod", 30, 500), MortalStartRoom)
	victim, victimClient := place(t, srv, fighterRecord("Welmar", 5, 200), MortalStartRoom)

	// Thirty hit points left and a wimp level of fifty: the state a player
	// sets this up for.
	inWorld(t, srv, func(_ *game.Live) {
		victim.Record.Points.Hit = 30
		victim.Record.WimpLevel = 50
	})

	if !hitUntilTheyFlee(t, srv, attacker, victim) {
		t.Fatal("a player below their wimp level never left the room")
	}

	if !victimClient.said("You wimp out, and attempt to flee!") {
		t.Error("the player was never told they were wimping out")
	}
	// The flee itself, not merely a walk: do_flee's own messages.
	if !victimClient.said("You flee head over heels.") {
		t.Error("the player left the room without do_flee's message")
	}

	var stillFighting bool
	inWorld(t, srv, func(_ *game.Live) {
		stillFighting = victim.Fighting != nil || attacker.Fighting != nil
	})
	if stillFighting {
		t.Error("fleeing did not end the fight")
	}
}

// TestNoWimpLevelMeansNoFleeing: the feature is off by default and stays off.
// `GET_WIMP_LEV(victim)` is the C's own first clause, and without it every
// player in the game would flee at zero hit points — which is to say, never,
// since the position switch has excluded them by then. The clause matters
// because it is what `wimpy 0` sets.
func TestNoWimpLevelMeansNoFleeing(t *testing.T) {
	srv, _ := newTestServer(t)

	attacker, _ := place(t, srv, fighterRecord("Zod", 30, 500), MortalStartRoom)
	victim, victimClient := place(t, srv, fighterRecord("Welmar", 5, 200), MortalStartRoom)

	inWorld(t, srv, func(_ *game.Live) {
		victim.Record.Points.Hit = 30
		victim.Record.WimpLevel = 0
	})

	if hitUntilTheyFlee(t, srv, attacker, victim) {
		t.Error("a player with no wimp level fled anyway")
	}
	if victimClient.said("wimp out") {
		t.Error("a player with no wimp level was told they were wimping out")
	}
}

// TestAboveTheWimpLevelNobodyRuns: the threshold is a threshold. `<`, not
// `<=`, so a player sitting exactly on their wimp level stands and fights —
// which is the off-by-one worth pinning, since it is invisible at every
// other number.
func TestAboveTheWimpLevelNobodyRuns(t *testing.T) {
	srv, _ := newTestServer(t)

	attacker, _ := place(t, srv, fighterRecord("Zod", 30, 500), MortalStartRoom)
	victim, victimClient := place(t, srv, fighterRecord("Welmar", 5, 200), MortalStartRoom)

	inWorld(t, srv, func(_ *game.Live) {
		victim.Record.Points.Hit = 50
		victim.Record.WimpLevel = 50
	})

	if hitUntilTheyFlee(t, srv, attacker, victim) {
		t.Error("a player exactly on their wimp level fled; the C's test is `<`")
	}
	if victimClient.said("wimp out") {
		t.Error("a player exactly on their wimp level was told they were wimping out")
	}
}

// TestBleedingToDeathNeverMakesYouWimpOut is the `victim != ch` clause.
//
// point_update's poison and bleeding are damage(ch, ch, n, TYPE_SUFFERING)
// in the C, so the victim is their own attacker and the wimpy branch cannot
// fire. Without that, a wimpy character who was poisoned would run out of
// the room every time the tick came round, with nobody chasing them.
func TestBleedingToDeathNeverMakesYouWimpOut(t *testing.T) {
	srv, _ := newTestServer(t)

	victim, victimClient := place(t, srv, fighterRecord("Welmar", 5, 200), MortalStartRoom)
	inWorld(t, srv, func(_ *game.Live) {
		victim.Record.Points.Hit = 40
		victim.Record.WimpLevel = 50
	})
	before := whereIs(t, srv, victim)

	for i := 0; i < 5; i++ {
		if err := srv.engine.DoSync(context.Background(), func(w *game.Live) {
			srv.suffer(w, victim, 1)
		}); err != nil {
			t.Fatal(err)
		}
	}

	if victimClient.said("wimp out") {
		t.Error("bleeding out told the character they were wimping out")
	}
	if got := whereIs(t, srv, victim); got != before {
		t.Errorf("bleeding out moved the character from %d to %d", before, got)
	}
}

// TestADyingPlayerDoesNotWimpOut: the flees are in the `default` branch of
// the position switch, so a blow that leaves somebody stunned or worse takes
// one of the four cases above it and never reaches them. A player cannot
// wimp out of the blow that put them on the floor.
func TestADyingPlayerDoesNotWimpOut(t *testing.T) {
	srv, _ := newTestServer(t)

	attacker, _ := place(t, srv, fighterRecord("Zod", 30, 500), MortalStartRoom)
	victim, victimClient := place(t, srv, fighterRecord("Welmar", 5, 200), MortalStartRoom)

	inWorld(t, srv, func(_ *game.Live) {
		victim.Record.Points.Hit = 30
		victim.Record.WimpLevel = 50
	})
	before := whereIs(t, srv, victim)

	// Enough to put them well under, not enough to kill: mortally wounded.
	hitFor(t, srv, attacker, victim, 37)

	var position game.Position
	inWorld(t, srv, func(_ *game.Live) { position = victim.Position })
	if position > game.PosStunned {
		t.Fatalf("the test did not put the victim on the floor; position is %v", position)
	}
	if victimClient.said("wimp out") {
		t.Error("a character knocked out of the fight was still told they wimped out")
	}
	if got := whereIs(t, srv, victim); got != before {
		t.Errorf("a character on the floor left the room, from %d to %d", before, got)
	}
}

// TestAWimpyMobileFleesWhenBadlyHurt is the other half of the same block,
// three lines above the player's (fight.c:903-904).
//
// Its threshold is not a threshold of its own: MOB_WIMPY runs at the same
// quarter-of-max-hit mark that prints "your wounds would stop BLEEDING",
// because the C nests the flee inside that `if` rather than beside it.
func TestAWimpyMobileFleesWhenBadlyHurt(t *testing.T) {
	srv, _ := newTestServer(t)

	attacker, _ := place(t, srv, fighterRecord("Zod", 30, 500), MortalStartRoom)
	mob := spawnDog(t, srv, MortalStartRoom)
	inWorld(t, srv, func(_ *game.Live) {
		mob.MobDef.ActionFlags = game.NewSet(game.MobWimpy)
		// Under a quarter of five hundred.
		mob.Record.Points.Hit = 100
	})

	if !hitUntilTheyFlee(t, srv, attacker, mob) {
		t.Error("a badly hurt MOB_WIMPY mobile stood its ground")
	}
}

// TestAMobileWithoutTheWimpyFlagStandsItsGround, at the same hit points, so
// the difference is the flag and nothing else.
func TestAMobileWithoutTheWimpyFlagStandsItsGround(t *testing.T) {
	srv, _ := newTestServer(t)

	attacker, _ := place(t, srv, fighterRecord("Zod", 30, 500), MortalStartRoom)
	mob := spawnDog(t, srv, MortalStartRoom)
	inWorld(t, srv, func(_ *game.Live) {
		mob.MobDef.ActionFlags = game.MobFlags{}
		mob.Record.Points.Hit = 100
	})

	if hitUntilTheyFlee(t, srv, attacker, mob) {
		t.Error("a mobile with no MOB_WIMPY flag fled")
	}
}

// TestAHealthyWimpyMobileStandsItsGround: MOB_WIMPY is not "always runs".
// Above the quarter mark the C never reaches the flee at all, because it is
// inside the bleeding warning's own `if`.
func TestAHealthyWimpyMobileStandsItsGround(t *testing.T) {
	srv, _ := newTestServer(t)

	attacker, _ := place(t, srv, fighterRecord("Zod", 30, 500), MortalStartRoom)
	mob := spawnDog(t, srv, MortalStartRoom)
	inWorld(t, srv, func(_ *game.Live) {
		mob.MobDef.ActionFlags = game.NewSet(game.MobWimpy)
		// Comfortably over a quarter of five hundred.
		mob.Record.Points.Hit = 400
	})

	if hitUntilTheyFlee(t, srv, attacker, mob) {
		t.Error("a healthy MOB_WIMPY mobile fled")
	}
}
