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

// check_killer and the sanctioned-pkill half of set_fighting (fight.c:
// 219-233, :250-275, #213). game.Live.SetFighting's own doc comment carries
// the fidelity citations; these prove the port through the two things it
// promises — the flag/message pair and the mudlog line the caller is handed
// back to log.

// TestAttackingAnotherPlayerSetsThePlayerKillerFlag: the first unprovoked
// blow against another player marks the attacker for it, once, with the C's
// exact warning.
func TestAttackingAnotherPlayerSetsThePlayerKillerFlag(t *testing.T) {
	srv, _ := newTestServer(t)
	attacker, attackerClient := place(t, srv, fighterRecord("Zod", 20, 100), MortalStartRoom)
	victim, victimClient := place(t, srv, fighterRecord("Welmar", 20, 100), MortalStartRoom)

	var message string
	inWorld(t, srv, func(w *game.Live) {
		_, message = w.SetFighting(attacker, victim)
	})

	if !attacker.Record.PlayerFlags.Has(game.PlayerKiller) {
		t.Error("the attacker was not flagged a player killer")
	}
	if victim.Record.PlayerFlags.Has(game.PlayerKiller) {
		t.Error("the victim was flagged a player killer")
	}
	if !attackerClient.said("If you want to be a PLAYER KILLER, so be it...") {
		t.Error("the attacker was not warned")
	}
	if victimClient.said("PLAYER KILLER") {
		t.Error("the victim saw the attacker's own warning")
	}

	const want = "PC Killer bit set on Zod for initiating attack on Welmar at The Temple Of Midgaard."
	if message != want {
		t.Errorf("mudlog line = %q, want %q", message, want)
	}
}

// TestCheckKillerLeavesAnAlreadyFlaggedVictimAlone: fighting a player who is
// already a killer or a thief is fair game, not an infraction (fight.c:
// 223-224).
func TestCheckKillerLeavesAnAlreadyFlaggedVictimAlone(t *testing.T) {
	for _, tc := range []struct {
		name string
		flag game.Flags
	}{
		{"a player killer", game.PlayerKiller},
		{"a thief", game.PlayerThief},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := newTestServer(t)
			attacker, attackerClient := place(t, srv, fighterRecord("Zod", 20, 100), MortalStartRoom)
			victim, _ := place(t, srv, fighterRecord("Welmar", 20, 100), MortalStartRoom)
			victim.Record.PlayerFlags = victim.Record.PlayerFlags.Set(tc.flag)

			var message string
			inWorld(t, srv, func(w *game.Live) {
				_, message = w.SetFighting(attacker, victim)
			})

			if attacker.Record.PlayerFlags.Has(game.PlayerKiller) {
				t.Errorf("attacking %s flagged the attacker a player killer", tc.name)
			}
			if message != "" {
				t.Errorf("mudlog line = %q, want none", message)
			}
			if attackerClient.said("PLAYER KILLER") {
				t.Error("the attacker was warned anyway")
			}
		})
	}
}

// TestCheckKillerDoesNotRepeatOnceAlreadyAKiller: SET_BIT is idempotent in
// the C, but the message and the mudlog line are not free — check_killer's
// own guard (fight.c:225) stops both once the flag is already there.
func TestCheckKillerDoesNotRepeatOnceAlreadyAKiller(t *testing.T) {
	srv, _ := newTestServer(t)
	attacker, attackerClient := place(t, srv, fighterRecord("Zod", 20, 100), MortalStartRoom)
	attacker.Record.PlayerFlags = attacker.Record.PlayerFlags.Set(game.PlayerKiller)
	victim, _ := place(t, srv, fighterRecord("Welmar", 20, 100), MortalStartRoom)

	var message string
	inWorld(t, srv, func(w *game.Live) {
		_, message = w.SetFighting(attacker, victim)
	})

	if message != "" {
		t.Errorf("mudlog line = %q, want none", message)
	}
	if attackerClient.said("PLAYER KILLER") {
		t.Error("an already-flagged killer was warned again")
	}
}

// TestCheckKillerSkipsMobiles: IS_NPC(ch) || IS_NPC(vict) (fight.c:225) —
// neither a mobile's own attack nor an attack on one ever mints a player
// killer.
func TestCheckKillerSkipsMobiles(t *testing.T) {
	t.Run("NPC attacker", func(t *testing.T) {
		srv, _ := newTestServer(t)
		attacker, _ := place(t, srv, fighterRecord("a large dog", 5, 100), MortalStartRoom)
		attacker.NPC = true
		victim, _ := place(t, srv, fighterRecord("Welmar", 20, 100), MortalStartRoom)

		var message string
		inWorld(t, srv, func(w *game.Live) {
			_, message = w.SetFighting(attacker, victim)
		})

		if message != "" {
			t.Errorf("mudlog line = %q, want none", message)
		}
		if attacker.Record.PlayerFlags.Has(game.PlayerKiller) {
			t.Error("a mobile was flagged a player killer")
		}
	})

	t.Run("NPC victim", func(t *testing.T) {
		srv, _ := newTestServer(t)
		attacker, attackerClient := place(t, srv, fighterRecord("Zod", 20, 100), MortalStartRoom)
		victim, _ := place(t, srv, fighterRecord("a large dog", 5, 100), MortalStartRoom)
		victim.NPC = true

		var message string
		inWorld(t, srv, func(w *game.Live) {
			_, message = w.SetFighting(attacker, victim)
		})

		if message != "" {
			t.Errorf("mudlog line = %q, want none", message)
		}
		if attacker.Record.PlayerFlags.Has(game.PlayerKiller) {
			t.Error("attacking a mobile flagged the attacker a player killer")
		}
		if attackerClient.said("PLAYER KILLER") {
			t.Error("attacking a mobile warned the attacker")
		}
	})
}

// TestSanctionedPkillInAPKillRoom: ROOM_PKILL turns the same first blow into
// an announced brawl instead of an infraction (fight.c:262-273).
func TestSanctionedPkillInAPKillRoom(t *testing.T) {
	srv, _ := newTestServer(t)
	attacker, attackerClient := place(t, srv, fighterRecord("Zod", 20, 100), MortalStartRoom)
	victim, victimClient := place(t, srv, fighterRecord("Welmar", 20, 100), MortalStartRoom)

	var message string
	inWorld(t, srv, func(w *game.Live) {
		room := w.Room(MortalStartRoom)
		room.Flags = room.Flags.With(game.RoomPKill)
		_, message = w.SetFighting(attacker, victim)
	})

	if !attackerClient.said("Okay! Let's get it on!") {
		t.Error("the attacker was not told the fight was sanctioned")
	}
	if !victimClient.said("Looks like Zod wants a little...") {
		t.Error("the victim was not told who started it")
	}
	if attacker.Record.PlayerFlags.Has(game.PlayerKiller) {
		t.Error("a sanctioned pkill also flagged the attacker a player killer")
	}

	const want = "Zod started sanctioned pkill on Welmar at The Temple Of Midgaard."
	if message != want {
		t.Errorf("mudlog line = %q, want %q", message, want)
	}
}

// TestSanctionedPkillDoesNotMudlogAMobile: "Only mudlog if both
// protagonists are players" (fight.c:269) — the greeting still fires, since
// the C sends it unconditionally, but there is nothing to log.
func TestSanctionedPkillDoesNotMudlogAMobile(t *testing.T) {
	srv, _ := newTestServer(t)
	attacker, attackerClient := place(t, srv, fighterRecord("Zod", 20, 100), MortalStartRoom)
	victim, _ := place(t, srv, fighterRecord("a large dog", 5, 100), MortalStartRoom)
	victim.NPC = true

	var message string
	inWorld(t, srv, func(w *game.Live) {
		room := w.Room(MortalStartRoom)
		room.Flags = room.Flags.With(game.RoomPKill)
		_, message = w.SetFighting(attacker, victim)
	})

	if message != "" {
		t.Errorf("mudlog line = %q, want none", message)
	}
	if !attackerClient.said("Okay! Let's get it on!") {
		t.Error("the sanctioned-room greeting should fire regardless of NPC status")
	}
}

// TestMurderingAnotherPlayerMudlogsThePlayerKillerFlag is the end-to-end
// proof: through the real `murder` command, into startFighting, into
// game.Live.SetFighting, back out through Server.wizlog
// (internal/server/wizvis.go) and the same WizVis-tagged logging every
// other mudlog() call site uses (#134), and onto an online immortal's own
// socket — not just the unit-level guarantee above that SetFighting
// returns the right string.
func TestMurderingAnotherPlayerMudlogsThePlayerKillerFlag(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	// The first character on the roster is an implementor (twoInARoom's own
	// doc comment, visibility_test.go); syslog defaults off, so it has to be
	// turned up before the echo can reach it.
	god := dialClient(t, addr)
	god.create("Warden", "password123", "m", "w")
	god.send("syslog complete")
	god.expect("Your syslog is now complete.")

	killer := dialClient(t, addr)
	killer.create("Zod", "swordfish", "m", "w")

	grimm, _ := place(t, srv, fighterRecord("Grimm", 20, 100), MortalStartRoom)
	grimm.Record.Sex = game.SexMale

	killer.send("murder grimm")
	killer.expect("If you want to be a PLAYER KILLER, so be it...")

	god.expect("PC Killer bit set on Zod for initiating attack on Grimm at The Temple Of Midgaard.")
}
