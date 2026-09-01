// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"testing"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// MOB_MEMORY (#380), in the three places the C spends it: remember when
// somebody starts a fight (fight.c:817), forget when you kill them
// (fight.c:959), and attack on sight in between (mobact.c:163-181).

// identified is fighterRecord with an identity number on it.
//
// Not a detail to leave to the fixture's zero value: the memory holds
// GET_IDNUM and nothing else, so two players who both have 0 are the *same
// person* as far as a grudge is concerned. A test written without this
// passes "the mobile attacked a stranger" — which is what it did, because
// the stranger was indistinguishable from the one it remembered.
//
// The real server never produces a 0: Server.nextIDNum starts at one more
// than the highest on the roster, so the collision is an artefact of the
// fixture rather than a case the C guards against, and it is fixed here
// rather than by adding a guard the C does not have.
func identified(name string, id int64, level, hit int32) *game.PlayerRecord {
	rec := fighterRecord(name, level, hit)
	rec.IDNum = id
	return rec
}

// remembering places a MOB_MEMORY mobile and a player who has annoyed it,
// with the fight already over — the mobile remembers, and is free to act.
func remembering(t *testing.T, srv *Server) (mob, player *game.Character, client *recorder) {
	t.Helper()

	mob = mobile(t, srv, "a jackal", game.NewSet(game.MobMemory), MortalStartRoom)
	player, client = place(t, srv, identified("Zod", 1, 20, 500), MortalStartRoom)

	// One blow from the player is what makes the mobile fight back, and
	// fighting back is what makes it remember.
	hitFor(t, srv, player, mob, 1)
	inWorld(t, srv, func(w *game.Live) {
		w.StopFighting(mob)
		w.StopFighting(player)
	})
	return mob, player, client
}

func remembers(t *testing.T, srv *Server, mob, who *game.Character) bool {
	t.Helper()
	var got bool
	inWorld(t, srv, func(_ *game.Live) { got = mob.Remembers(who) })
	return got
}

// TestAMobileRemembersWhoStartedIt is the first of the three.
func TestAMobileRemembersWhoStartedIt(t *testing.T) {
	srv, _ := newTestServer(t)
	mob, player, _ := remembering(t, srv)

	if !remembers(t, srv, mob, player) {
		t.Error("a MOB_MEMORY mobile did not remember who attacked it")
	}
}

// TestAMobileWithoutTheFlagRemembersNothing, so the flag is what does it.
func TestAMobileWithoutTheFlagRemembersNothing(t *testing.T) {
	srv, _ := newTestServer(t)

	mob := mobile(t, srv, "a jackal", game.MobFlags{}, MortalStartRoom)
	player, _ := place(t, srv, identified("Zod", 1, 20, 500), MortalStartRoom)
	hitFor(t, srv, player, mob, 1)

	if remembers(t, srv, mob, player) {
		t.Error("a mobile with no MOB_MEMORY flag remembered an attacker")
	}
}

// TestAMobileAttacksSomebodyItRemembers is the second, and the one a player
// notices.
func TestAMobileAttacksSomebodyItRemembers(t *testing.T) {
	srv, _ := newTestServer(t)
	mob, player, client := remembering(t, srv)

	pulseMobiles(t, srv)

	if got := fightingOf(t, srv, mob); got != player {
		t.Errorf("the mobile is fighting %v, want the player it remembers", got)
	}
	if !client.said("'Hey!  You're the fiend that attacked me!!!', exclaims a jackal.") {
		t.Error("the room was not told why the mobile attacked")
	}
}

// TestAMobileLeavesStrangersAlone: the grudge is against one person, not
// everybody. A MOB_MEMORY mobile is not an aggressive one.
func TestAMobileLeavesStrangersAlone(t *testing.T) {
	srv, _ := newTestServer(t)
	mob, remembered, _ := remembering(t, srv)

	stranger, _ := place(t, srv, identified("Welmar", 2, 20, 500), MortalStartRoom)
	// The one it remembers leaves, so the only person left is a stranger.
	// Taken out inside the closure with a pointer fetched *outside* it: a
	// DoSync from the world goroutine never returns, which is how this
	// test first hung the whole package rather than failing.
	inWorld(t, srv, func(w *game.Live) { w.Remove(remembered) })

	pulseMobiles(t, srv)

	if got := fightingOf(t, srv, mob); got == stranger {
		t.Error("the mobile attacked somebody it had never met")
	}
}

// TestKillingSomebodySettlesIt is the third: forget on the kill
// (fight.c:959). Without it a mobile that has killed you once attacks on
// sight forever, which is a different game from the one the flag describes.
func TestKillingSomebodySettlesIt(t *testing.T) {
	srv, _ := newTestServer(t)
	mob, player, _ := remembering(t, srv)

	if !remembers(t, srv, mob, player) {
		t.Fatal("the setup did not leave a grudge to settle")
	}
	hitFor(t, srv, mob, player, 10000)

	if remembers(t, srv, mob, player) {
		t.Error("the mobile killed them and still holds a grudge")
	}
}

// TestNoHassleKeepsAGodOutOfIt, in both places the C tests it.
//
// Remember refuses to write a NOHASSLE player down at all, and the mobact
// pass refuses to act on one — the second matters because a god can turn
// nohassle on *after* being remembered, which is exactly when they would
// want to.
func TestNoHassleKeepsAGodOutOfIt(t *testing.T) {
	t.Run("never remembered", func(t *testing.T) {
		srv, _ := newTestServer(t)
		mob := mobile(t, srv, "a jackal", game.NewSet(game.MobMemory), MortalStartRoom)
		god, _ := place(t, srv, identified("Zod", 1, 34, 500), MortalStartRoom)
		inWorld(t, srv, func(_ *game.Live) {
			god.Record.Preferences = god.Record.Preferences.With(game.PrefNoHassle)
		})

		hitFor(t, srv, god, mob, 1)

		if remembers(t, srv, mob, god) {
			t.Error("a nohassle god was written into a mobile's memory")
		}
	})

	t.Run("remembered, then protected", func(t *testing.T) {
		srv, _ := newTestServer(t)
		mob, player, _ := remembering(t, srv)
		inWorld(t, srv, func(_ *game.Live) {
			player.Record.Preferences = player.Record.Preferences.With(game.PrefNoHassle)
		})

		pulseMobiles(t, srv)

		if got := fightingOf(t, srv, mob); got != nil {
			t.Errorf("a remembered player turned nohassle on and was attacked anyway, by %s", got.Name)
		}
	})
}

// TestAMobileCannotAvengeItselfOnSomebodyItCannotSee: CAN_SEE, which the
// aggressive pass tests too but for a different reason — here it means
// invisibility hides you from a grudge as well as from a wandering monster.
func TestAMobileCannotAvengeItselfOnSomebodyItCannotSee(t *testing.T) {
	srv, _ := newTestServer(t)
	mob, player, _ := remembering(t, srv)

	inWorld(t, srv, func(_ *game.Live) {
		player.Record.AffectFlags = player.Record.AffectFlags.With(game.AffectInvisible)
	})

	pulseMobiles(t, srv)

	if got := fightingOf(t, srv, mob); got != nil {
		t.Errorf("an invisible player was attacked from memory, by %s", got.Name)
	}
}

// TestMemoryOutlivesTheBody is why the C stores identity numbers rather than
// pointers, and it is the property that makes the feature worth anything: a
// player who runs away and logs back in is a different character struct with
// the same GET_IDNUM, and the mobile still knows them.
func TestMemoryOutlivesTheBody(t *testing.T) {
	srv, _ := newTestServer(t)
	mob, player, _ := remembering(t, srv)

	// The same player, freshly loaded: a different *game.Character with the
	// same identity number, which is exactly what a relogin produces.
	returned := &game.Character{
		Name:     player.Name,
		Position: game.PosStanding,
		Record:   &game.PlayerRecord{Name: player.Name, IDNum: player.Record.IDNum},
	}

	if !remembers(t, srv, mob, returned) {
		t.Error("the grudge did not survive the player getting a new body")
	}
}
