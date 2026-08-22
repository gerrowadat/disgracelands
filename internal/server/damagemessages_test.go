// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// damage()'s dispatch (fight.c:855-871), for the ordinary weapon swing:
// a miss or a death blow prefers a registered misc/messages entry over
// the compiled severity table; an ordinary non-fatal hit always uses the
// compiled table regardless of what is registered.
//
// Called through sendCombatMessage directly with dam fixed, rather than
// via a live swing: game.Attack's own hit/miss roll would otherwise have
// to be coaxed into a specific outcome, and the C's dispatch rule is a
// property of dam and victim.Position, not of how they got that way.

func TestOrdinaryHitAlwaysUsesTheCompiledTable(t *testing.T) {
	srv, _ := newTestServer(t)
	attacker, attackerClient := place(t, srv, fighterRecord("Zod", 30, 500), MortalStartRoom)
	victim, _ := place(t, srv, fighterRecord("Welmar", 5, 200), MortalStartRoom)

	// A registered entry for AttackHit exists, but a non-fatal hit must
	// ignore it — dam_message, not skill_message, for anything that is
	// neither a miss nor a death blow.
	srv.text.messages = game.NewFightMessages([]game.FightMessage{
		{AttackType: game.TypeHit + game.AttackHit, Hit: game.MsgSet{Attacker: "REGISTERED HIT MESSAGE"}},
	})

	inWorld(t, srv, func(w *game.Live) {
		victim.Position = game.PosFighting
		sendCombatMessageT(t, srv, w, attacker, victim, 5) // tier 3: "You #w $N."
	})

	if attackerClient.said("REGISTERED HIT MESSAGE") {
		t.Error("an ordinary hit used the registered message; dam_message should have won")
	}
	if !attackerClient.said("You hit Welmar.") {
		t.Errorf("attacker was not told the compiled tier-3 text; transcript: %q", attackerClient.lines)
	}
}

func TestMissPrefersARegisteredMessage(t *testing.T) {
	srv, _ := newTestServer(t)
	attacker, attackerClient := place(t, srv, fighterRecord("Zod", 30, 500), MortalStartRoom)
	victim, _ := place(t, srv, fighterRecord("Welmar", 5, 200), MortalStartRoom)

	srv.text.messages = game.NewFightMessages([]game.FightMessage{
		{AttackType: game.TypeHit + game.AttackHit, Miss: game.MsgSet{Attacker: "REGISTERED MISS MESSAGE"}},
	})

	inWorld(t, srv, func(w *game.Live) {
		victim.Position = game.PosFighting
		sendCombatMessageT(t, srv, w, attacker, victim, 0)
	})

	if !attackerClient.said("REGISTERED MISS MESSAGE") {
		t.Error("a miss with a registered message did not use it")
	}
}

func TestMissFallsBackWithNoRegisteredMessage(t *testing.T) {
	srv, _ := newTestServer(t)
	attacker, attackerClient := place(t, srv, fighterRecord("Zod", 30, 500), MortalStartRoom)
	victim, _ := place(t, srv, fighterRecord("Welmar", 5, 200), MortalStartRoom)

	inWorld(t, srv, func(w *game.Live) {
		victim.Position = game.PosFighting
		sendCombatMessageT(t, srv, w, attacker, victim, 0)
	})

	if !attackerClient.said("You try to hit Welmar, but miss.") {
		t.Errorf("attacker was not told the compiled tier-0 (miss) text; transcript: %q", attackerClient.lines)
	}
}

func TestDeathBlowPrefersARegisteredMessage(t *testing.T) {
	srv, _ := newTestServer(t)
	attacker, attackerClient := place(t, srv, fighterRecord("Zod", 30, 500), MortalStartRoom)
	victim, _ := place(t, srv, fighterRecord("Welmar", 5, 200), MortalStartRoom)

	srv.text.messages = game.NewFightMessages([]game.FightMessage{
		{AttackType: game.TypeHit + game.AttackHit, Die: game.MsgSet{Attacker: "REGISTERED DIE MESSAGE"}},
	})

	inWorld(t, srv, func(w *game.Live) {
		victim.Position = game.PosDead
		sendCombatMessageT(t, srv, w, attacker, victim, 40)
	})

	if !attackerClient.said("REGISTERED DIE MESSAGE") {
		t.Error("a death blow with a registered message did not use it")
	}
}

func TestDeathBlowFallsBackWithNoRegisteredMessage(t *testing.T) {
	srv, _ := newTestServer(t)
	attacker, attackerClient := place(t, srv, fighterRecord("Zod", 30, 500), MortalStartRoom)
	victim, _ := place(t, srv, fighterRecord("Welmar", 5, 200), MortalStartRoom)

	inWorld(t, srv, func(w *game.Live) {
		victim.Position = game.PosDead
		// 40 damage is tier 8 (24..50): "OBLITERATES", not a death-
		// specific message — dam_message has no die-branch at all.
		sendCombatMessageT(t, srv, w, attacker, victim, 40)
	})

	if !attackerClient.said("You OBLITERATE Welmar with your deadly hit!!") {
		t.Errorf("attacker was not told the compiled tier-8 text; transcript: %q", attackerClient.lines)
	}
}

// Damage (the shared entry point for kick, bash, spells and every other
// Violence caller) must stay silent about the hit itself — onDamaged is
// nil for it, so applyDamage never calls sendCombatMessage. Each of those
// commands prints its own message elsewhere; s.hit is the only caller
// that gets one from here.
func TestOtherDamageCallersStaySilentAboutTheHit(t *testing.T) {
	srv, _ := newTestServer(t)
	attacker, attackerClient := place(t, srv, fighterRecord("Zod", 30, 500), MortalStartRoom)
	victim, victimClient := place(t, srv, fighterRecord("Welmar", 5, 200), MortalStartRoom)

	// A registered message that would be unmistakable if it leaked in.
	srv.text.messages = game.NewFightMessages([]game.FightMessage{
		{AttackType: game.TypeHit + game.AttackHit, Hit: game.MsgSet{Attacker: "SHOULD NEVER APPEAR"}},
	})

	inWorld(t, srv, func(w *game.Live) {
		srv.Damage(w, attacker, victim, 10)
	})

	for _, c := range []*recorder{attackerClient, victimClient} {
		if len(c.lines) != 0 {
			t.Errorf("Damage sent %v, want silence — the caller prints its own message", c.lines)
		}
	}
}

// Against the real archive: a death blow's bare-hand entry (AttackHit,
// registered in data/misc/messages) is preferred over the compiled table
// — proving the live dispatch reaches real data, not just a synthetic
// fixture. newTestServer's own default has no messages loaded at all
// (unlike help/socials, deliberately: every other combat test in this
// package relies on the compiled table as the fallback default, and
// registering the real archive there would make bare-hand attack type
// 300 — which the real data does register — start winning unpredictably
// among its own variants), so this loads it just for this one test.
func TestDeathBlowAgainstTheRealArchive(t *testing.T) {
	srv, _ := newTestServer(t)
	attacker, attackerClient := place(t, srv, fighterRecord("Zod", 30, 500), MortalStartRoom)
	victim, _ := place(t, srv, fighterRecord("Welmar", 5, 200), MortalStartRoom)

	f, err := os.Open(filepath.Join(repoRoot(t), "data", messagesFile))
	if err != nil {
		t.Fatalf("opening the real archive: %v", err)
	}
	records, err := game.ParseMessagesFile(f)
	_ = f.Close()
	if err != nil {
		t.Fatalf("parsing the real archive: %v", err)
	}
	srv.text.messages = game.NewFightMessages(records)

	inWorld(t, srv, func(w *game.Live) {
		victim.Position = game.PosDead
		sendCombatMessageT(t, srv, w, attacker, victim, 40)
	})

	if len(attackerClient.lines) == 0 {
		t.Fatal("attacker was told nothing")
	}
	// Whichever of AttackHit's registered die-message variants was
	// picked, it is not the compiled table's — a real entry exists
	// (data/misc/messages has four records for attack type 300), so the
	// dispatch must have preferred it.
	if attackerClient.said("OBLITERATE") {
		t.Errorf("used the compiled table despite a real registered entry existing: %v", attackerClient.lines)
	}
}

// sendCombatMessageT calls sendCombatMessage with a bare-handed attack
// type, from within an inWorld closure — t.Fatal is never safe there
// (CLAUDE.md), so this only ever calls t.Helper.
func sendCombatMessageT(t *testing.T, srv *Server, w *game.Live, attacker, victim *game.Character, dam int32) {
	t.Helper()
	srv.sendCombatMessage(w, attacker, victim, nil, dam, game.TypeHit+game.AttackHit)
}
