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
	"github.com/gerrowadat/disgracelands/internal/persist/messages"
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
	// An NPC victim, so Damage's own startFighting does not also run
	// check_killer (fight.c:219-233, #213) — a real message on a genuine
	// player-vs-player hit, but unrelated to what this test checks.
	victim.NPC = true

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
// registered in examples/stock/binary/misc/messages) is preferred over the compiled table
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

	loadRealFightMessages(t, srv)

	inWorld(t, srv, func(w *game.Live) {
		victim.Position = game.PosDead
		sendCombatMessageT(t, srv, w, attacker, victim, 40)
	})

	if len(attackerClient.lines) == 0 {
		t.Fatal("attacker was told nothing")
	}
	// Whichever of AttackHit's registered die-message variants was
	// picked, it is not the compiled table's — a real entry exists
	// (examples/stock/binary/misc/messages has four records for attack type 300), so the
	// dispatch must have preferred it.
	if attackerClient.said("OBLITERATE") {
		t.Errorf("used the compiled table despite a real registered entry existing: %v", attackerClient.lines)
	}
}

// loadRealFightMessages loads the real stock misc/messages table into
// srv, for a test that specifically wants real registered entries rather
// than newTestServer's default (no messages configured at all — see
// TestDeathBlowAgainstTheRealArchive's own doc comment for why that stays
// the default).
func loadRealFightMessages(t *testing.T, srv *Server) {
	t.Helper()
	f, err := os.Open(filepath.Join(repoRoot(t), "examples", "stock", "binary", messagesFile))
	if err != nil {
		t.Fatalf("opening the real archive: %v", err)
	}
	records, err := game.ParseMessagesFile(f)
	_ = f.Close()
	if err != nil {
		t.Fatalf("parsing the real archive: %v", err)
	}
	srv.text.messages = game.NewFightMessages(records)
}

// sendCombatMessageT calls sendCombatMessage with a bare-handed attack
// type, from within an inWorld closure — t.Fatal is never safe there
// (CLAUDE.md), so this only ever calls t.Helper.
func sendCombatMessageT(t *testing.T, srv *Server, w *game.Live, attacker, victim *game.Character, dam int32) {
	t.Helper()
	srv.sendCombatMessage(w, attacker, victim, nil, dam, game.TypeHit+game.AttackHit)
}

// SkillDamage (kick/bash/backstab) always prefers a registered message —
// there is no dam_message fallback for a non-weapon attack at all
// (fight.c:854's `!IS_WEAPON` branch), unlike the weapon swing.

func TestSkillDamageUsesARegisteredHitMessage(t *testing.T) {
	srv, _ := newTestServer(t)
	attacker, attackerClient := place(t, srv, fighterRecord("Zod", 30, 500), MortalStartRoom)
	victim, _ := place(t, srv, fighterRecord("Welmar", 5, 200), MortalStartRoom)

	srv.text.messages = game.NewFightMessages([]game.FightMessage{
		{AttackType: game.SkillKick, Hit: game.MsgSet{Attacker: "REGISTERED KICK HIT"}},
	})

	inWorld(t, srv, func(w *game.Live) {
		victim.Position = game.PosFighting
		srv.SkillDamage(w, attacker, victim, 5, game.SkillKick)
	})

	if !attackerClient.said("REGISTERED KICK HIT") {
		t.Errorf("attacker was not told the registered hit message: %v", attackerClient.lines)
	}
}

func TestSkillDamageUsesARegisteredMissMessage(t *testing.T) {
	srv, _ := newTestServer(t)
	attacker, attackerClient := place(t, srv, fighterRecord("Zod", 30, 500), MortalStartRoom)
	victim, _ := place(t, srv, fighterRecord("Welmar", 5, 200), MortalStartRoom)

	srv.text.messages = game.NewFightMessages([]game.FightMessage{
		{AttackType: game.SkillBash, Miss: game.MsgSet{Attacker: "REGISTERED BASH MISS"}},
	})

	inWorld(t, srv, func(w *game.Live) {
		victim.Position = game.PosFighting
		srv.SkillDamage(w, attacker, victim, 0, game.SkillBash)
	})

	if !attackerClient.said("REGISTERED BASH MISS") {
		t.Errorf("attacker was not told the registered miss message: %v", attackerClient.lines)
	}
}

func TestSkillDamageUsesARegisteredDieMessage(t *testing.T) {
	srv, _ := newTestServer(t)
	attacker, attackerClient := place(t, srv, fighterRecord("Zod", 30, 500), MortalStartRoom)
	victim, _ := place(t, srv, fighterRecord("Welmar", 5, 1), MortalStartRoom)

	srv.text.messages = game.NewFightMessages([]game.FightMessage{
		{AttackType: game.SkillBackstab, Die: game.MsgSet{Attacker: "REGISTERED BACKSTAB DIE"}},
	})

	inWorld(t, srv, func(w *game.Live) {
		srv.SkillDamage(w, attacker, victim, 40, game.SkillBackstab)
	})

	if !attackerClient.said("REGISTERED BACKSTAB DIE") {
		t.Errorf("attacker was not told the registered die message: %v", attackerClient.lines)
	}
	// Not asserting victim.Position here: a player who dies is
	// resurrected at 1 HP standing (internal/server/tick.go's die, "A
	// dead player wakes up at the temple with one hit point") as part of
	// the same applyDamage call, after the die message has already gone
	// out — which is exactly what the check above already proved.
}

// Against the real archive: kick (skill 134) has two registered variants
// whose hit-text disagrees on whether it names the victim at all — real
// data, not synthetic, is what surfaces that kind of thing — so this
// checks only that *something* was sent, not its exact wording. Bash and
// backstab's own real-archive coverage lives in
// internal/server/skills_test.go (TestBashNeedsAWeapon,
// TestBackstabNeedsAPiercingWeapon), which can wait on the victim's name
// because each has only one registered variant.
func TestSkillDamageAgainstTheRealArchive(t *testing.T) {
	srv, _ := newTestServer(t)
	attacker, attackerClient := place(t, srv, fighterRecord("Zod", 30, 500), MortalStartRoom)
	victim, _ := place(t, srv, fighterRecord("Welmar", 5, 200), MortalStartRoom)
	loadRealFightMessages(t, srv)

	inWorld(t, srv, func(w *game.Live) {
		victim.Position = game.PosFighting
		srv.SkillDamage(w, attacker, victim, 5, game.SkillKick)
	})

	if len(attackerClient.lines) == 0 {
		t.Error("attacker was told nothing for a registered kick hit")
	}
}

// A spell number is just another skillType to SkillDamage — mag_damage's
// own C ends with `return (damage(ch, victim, dam, spellnum))`
// (magic.c:294), the identical dispatch a skill's number goes through.
// Magic Missile has a real examples/stock/binary/misc/messages entry; Ouchie, one of the
// two local joke spells (spell.go's own naming), does not — proving the
// registered/unregistered split holds for spell numbers exactly as it
// does for SkillKick/SkillBash/SkillBackstab, not by inspection but by
// running both through the real archive.
func TestSkillDamageTreatsSpellNumbersLikeSkillNumbers(t *testing.T) {
	srv, _ := newTestServer(t)
	attacker, attackerClient := place(t, srv, fighterRecord("Zod", 30, 500), MortalStartRoom)
	victim, _ := place(t, srv, fighterRecord("Welmar", 5, 200), MortalStartRoom)
	loadRealFightMessages(t, srv)

	inWorld(t, srv, func(w *game.Live) {
		victim.Position = game.PosFighting
		srv.SkillDamage(w, attacker, victim, 5, game.SpellMagicMissile)
	})
	if len(attackerClient.lines) == 0 {
		t.Error("SkillDamage(SpellMagicMissile) with a real registered entry said nothing")
	}

	attackerClient.lines = nil
	inWorld(t, srv, func(w *game.Live) {
		victim.Position = game.PosFighting
		srv.SkillDamage(w, attacker, victim, 5, game.SpellOuchie)
	})
	if len(attackerClient.lines) != 0 {
		t.Errorf("SkillDamage(SpellOuchie), unregistered, said %v, want silence", attackerClient.lines)
	}
}

// No dam_message fallback exists for a non-weapon attack: nothing
// registered means genuine silence, not compiled text.
func TestSkillDamageIsSilentWithNothingRegistered(t *testing.T) {
	srv, _ := newTestServer(t)
	attacker, attackerClient := place(t, srv, fighterRecord("Zod", 30, 500), MortalStartRoom)
	victim, victimClient := place(t, srv, fighterRecord("Welmar", 5, 200), MortalStartRoom)
	// An NPC victim, so SkillDamage's own startFighting does not also run
	// check_killer (fight.c:219-233, #213) — a real message on a genuine
	// player-vs-player hit, but unrelated to what this test checks.
	victim.NPC = true

	inWorld(t, srv, func(w *game.Live) {
		victim.Position = game.PosFighting
		srv.SkillDamage(w, attacker, victim, 5, game.SkillKick)
	})

	for _, c := range []*recorder{attackerClient, victimClient} {
		if len(c.lines) != 0 {
			t.Errorf("SkillDamage with nothing registered said %v, want silence", c.lines)
		}
	}
}

// End to end: LoadText(dir, "yaml") reads config/messages.yaml, not
// misc/messages, and a kick still resolves a real registered message
// through it — proving the wiring (internal/server/text.go's own
// messages.Load call, the --messages-format flag it is fed from in
// cmd/dlmud/main.go), not just internal/persist/messages' own codec
// (already covered by its own real-archive round-trip test).
func TestYamlMessagesFormatEndToEnd(t *testing.T) {
	classic, err := messages.Load("classic", "../../examples/stock/binary/misc/messages")
	if err != nil {
		t.Fatalf("Load(classic): %v", err)
	}

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "text"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		greetingFile: testGreeting,
		creditsFile:  testCredits,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := messages.Save("yaml", filepath.Join(dir, "config"), classic); err != nil {
		t.Fatalf("Save(yaml): %v", err)
	}

	text, err := LoadText(dir, "yaml", "classic", "classic")
	if err != nil {
		t.Fatalf("LoadText: %v", err)
	}

	if _, ok := text.FightMessages().Pick(game.SkillKick, testRNG()); !ok {
		t.Error("LoadText(dir, \"yaml\") found no registered kick message, want the real archive's")
	}
}
