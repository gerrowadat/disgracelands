// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"context"
	"strings"
	"testing"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// timesSaid counts how often a line was sent, which "said" cannot: the
// bound the C's `found` puts on the helper pass is *one per helper*, and
// one is only distinguishable from two by counting.
func (r *recorder) timesSaid(s string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return strings.Count(strings.Join(r.lines, ""), s)
}

// MOB_HELPER (#379), the last pass in mobile_activity (mobact.c:259-272).
//
// Most of these are about what the flag does *not* do. Its three skips are
// what make it "guards gang up on you" rather than "flagged mobiles join
// every fight in the room", and a port that dropped any one of them would
// still look right in the obvious test.

// helperSetup: a flagged mobile, a second mobile, and a player for the
// second mobile to be fighting.
func helperSetup(t *testing.T, srv *Server, flags game.MobFlags) (helper, victim *game.Character, player *game.Character) {
	t.Helper()

	helper = mobile(t, srv, "a temple guard", flags, MortalStartRoom)
	victim = mobile(t, srv, "a shopkeeper", game.MobFlags{}, MortalStartRoom)
	player, _ = place(t, srv, fighterRecord("Zod", 20, 500), MortalStartRoom)
	return helper, victim, player
}

// fight puts a and b into a fight, on the world goroutine.
func fight(t *testing.T, srv *Server, a, b *game.Character) {
	t.Helper()
	if err := srv.engine.DoSync(context.Background(), func(w *game.Live) {
		w.SetFighting(a, b)
	}); err != nil {
		t.Fatal(err)
	}
}

// pulseMobiles runs one mobile-activity pass.
func pulseMobiles(t *testing.T, srv *Server) {
	t.Helper()
	if err := srv.engine.DoSync(context.Background(), srv.mobileActivity); err != nil {
		t.Fatal(err)
	}
}

func fightingOf(t *testing.T, srv *Server, who *game.Character) *game.Character {
	t.Helper()
	var target *game.Character
	inWorld(t, srv, func(_ *game.Live) { target = who.Fighting })
	return target
}

// TestAHelperJoinsAFightAgainstAPlayer is #379.
func TestAHelperJoinsAFightAgainstAPlayer(t *testing.T) {
	srv, _ := newTestServer(t)

	helper, victim, player := helperSetup(t, srv, game.NewSet(game.MobHelper))
	fight(t, srv, victim, player)

	pulseMobiles(t, srv)

	if got := fightingOf(t, srv, helper); got != player {
		t.Errorf("the helper is fighting %v, want the player", got)
	}
}

// TestAHelperSaysSo: act("$n jumps to the aid of $N!", ...) goes to the
// room, the one being helped included and the helper excluded (TO_ROOM).
func TestAHelperSaysSo(t *testing.T) {
	srv, _ := newTestServer(t)

	helper, victim, player := helperSetup(t, srv, game.NewSet(game.MobHelper))
	_, playerClient := place(t, srv, fighterRecord("Bystander", 5, 100), MortalStartRoom)
	fight(t, srv, victim, player)

	pulseMobiles(t, srv)

	// Capitalised, because game.Act capitalises the first letter of what it
	// renders — perform_act does, so every act() line in the game arrives
	// that way and this one is not special.
	if !playerClient.said("A temple guard jumps to the aid of a shopkeeper!") {
		t.Error("the room was not told the helper joined in")
	}
	_ = helper
}

// TestAHelperIgnoresAFightBetweenMobiles: `IS_NPC(FIGHTING(vict))` skips.
// Without this, two flagged mobiles that started on each other would pull
// the whole room in.
func TestAHelperIgnoresAFightBetweenMobiles(t *testing.T) {
	srv, _ := newTestServer(t)

	helper, victim, _ := helperSetup(t, srv, game.NewSet(game.MobHelper))
	other := mobile(t, srv, "a rat", game.MobFlags{}, MortalStartRoom)
	fight(t, srv, victim, other)

	pulseMobiles(t, srv)

	if got := fightingOf(t, srv, helper); got != nil {
		t.Errorf("the helper joined a fight between two mobiles, against %s", got.Name)
	}
}

// TestAHelperStaysOutOfAFightBetweenPlayers is `!IS_NPC(vict)`, and it took
// two attempts to test.
//
// The obvious version — a player fighting a mobile, the helper asked not to
// come to their aid — passes with that condition deleted, because the *next*
// one catches it instead: the player's opponent is an NPC, so
// `IS_NPC(FIGHTING(vict))` skips anyway. Two conditions covering one case
// means neither is pinned. Player against player is the case only
// `!IS_NPC(vict)` refuses, and with it removed the helper joins a duel and
// starts swinging at whoever the loser was fighting.
func TestAHelperStaysOutOfAFightBetweenPlayers(t *testing.T) {
	srv, _ := newTestServer(t)

	helper := mobile(t, srv, "a temple guard", game.NewSet(game.MobHelper), MortalStartRoom)
	one, _ := place(t, srv, fighterRecord("Zod", 20, 500), MortalStartRoom)
	two, _ := place(t, srv, fighterRecord("Welmar", 20, 500), MortalStartRoom)
	fight(t, srv, one, two)

	pulseMobiles(t, srv)

	if got := fightingOf(t, srv, helper); got != nil {
		t.Errorf("the helper joined a fight between two players, against %s", got.Name)
	}
}

// TestAHelperDoesNotJoinAFightAgainstItself: `ch == FIGHTING(vict)` skips.
// Reachable whenever a helper is already being attacked by somebody its
// neighbour is also attacking, and swinging at them again would be an extra
// attack out of nowhere.
func TestAHelperDoesNotJoinAFightAgainstItself(t *testing.T) {
	srv, _ := newTestServer(t)

	helper, victim, _ := helperSetup(t, srv, game.NewSet(game.MobHelper))
	// The neighbour is fighting the helper itself.
	fight(t, srv, victim, helper)
	inWorld(t, srv, func(w *game.Live) { w.StopFighting(helper) })

	pulseMobiles(t, srv)

	if got := fightingOf(t, srv, helper); got != nil {
		t.Errorf("the helper joined a fight aimed at itself, against %s", got.Name)
	}
}

// TestABlindOrCharmedHelperSitsItOut, both of the C's affect exclusions.
func TestABlindOrCharmedHelperSitsItOut(t *testing.T) {
	for _, tc := range []struct {
		name   string
		affect game.AffectFlag
	}{
		{"blind", game.AffectBlind},
		{"charmed", game.AffectCharm},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := newTestServer(t)

			helper, victim, player := helperSetup(t, srv, game.NewSet(game.MobHelper))
			inWorld(t, srv, func(_ *game.Live) {
				helper.Record.AffectFlags = helper.Record.AffectFlags.With(tc.affect)
			})
			fight(t, srv, victim, player)

			pulseMobiles(t, srv)

			if got := fightingOf(t, srv, helper); got != nil {
				t.Errorf("a %s helper joined in, against %s", tc.name, got.Name)
			}
		})
	}
}

// TestAMobileWithoutTheHelperFlagStaysOut, so the flag is what does it.
func TestAMobileWithoutTheHelperFlagStaysOut(t *testing.T) {
	srv, _ := newTestServer(t)

	helper, victim, player := helperSetup(t, srv, game.MobFlags{})
	fight(t, srv, victim, player)

	pulseMobiles(t, srv)

	if got := fightingOf(t, srv, helper); got != nil {
		t.Errorf("an unflagged mobile joined in, against %s", got.Name)
	}
}

// TestAHelperHelpsOneNeighbourPerPulse is the C's `found`, and it bounds
// less than it looks like it bounds.
//
// `found` is reset once per *mobile* (mobact.c:261, inside the loop over
// character_list), so it stops one helper joining two fights in the same
// pulse. It does not stop two helpers both joining in — each runs its own
// pass — and a room of guards really does all pile in at once, which is
// what the flag is for. Only the per-helper bound is asserted here, and it
// has to be counted rather than read off Fighting, which holds one opponent
// either way.
func TestAHelperHelpsOneNeighbourPerPulse(t *testing.T) {
	srv, _ := newTestServer(t)

	helper := mobile(t, srv, "a temple guard", game.NewSet(game.MobHelper), MortalStartRoom)
	_, watcher := place(t, srv, fighterRecord("Bystander", 5, 100), MortalStartRoom)

	// Two separate fights for it to choose between.
	for _, pair := range []struct{ mob, player string }{
		{"a shopkeeper", "Zod"},
		{"a butcher", "Welmar"},
	} {
		m := mobile(t, srv, pair.mob, game.MobFlags{}, MortalStartRoom)
		p, _ := place(t, srv, fighterRecord(pair.player, 20, 500), MortalStartRoom)
		fight(t, srv, m, p)
	}

	pulseMobiles(t, srv)

	if got := watcher.timesSaid("jumps to the aid of"); got != 1 {
		t.Errorf("the helper joined %d fights in one pulse, want exactly 1", got)
	}
	_ = helper
}
