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

// The mudlog() audit (issue #134): bug/idea/typo was the one call site
// tagged for in-game echo, and every other command the C routes through
// mudlog() is wired up now. These are the end-to-end proofs for the
// interesting ones — the selection rules the C applies, exercised through
// producers other than `bug`, plus the three that are easy to get wrong.
//
// report_test.go already covers the plumbing itself (a log call reaching a
// live socket at all) and the two flat refusals, mortal and syslog-off.

// A NRM line reaches an immortal on syslog complete: `tp < type` with
// tp = 3 and type = NRM = 2 (utils.c:252-253).
func TestQuitEchoesToAnImmortalWatchingSyslog(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	god, mortal := twoInARoom(t, srv, addr)

	god.send("syslog complete")
	god.expect("Your syslog is now complete.")

	mortal.send("quit")
	mortal.expect("Goodbye, friend.. Come back soon!")

	// "%s has quit the game." (act.other.c:153).
	god.expect("Bystander has quit the game.")
}

// The same line at a *lower* verbosity than it needs: syslog brief is 1,
// the message's own type is NRM = 2, so `tp < type` skips it. This is the
// half of the selection nothing exercised before — `bug` is CMP, the
// loudest type there is, so it either arrives or does not at all.
func TestANormalLineDoesNotReachAnImmortalOnSyslogBrief(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	god, mortal := twoInARoom(t, srv, addr)

	god.send("syslog brief")
	god.expect("Your syslog is now brief.")

	mortal.send("quit")
	mortal.expect("Goodbye, friend.. Come back soon!")

	god.settle()
	if god.seen("Bystander has quit the game.") {
		t.Errorf("a NRM line reached an immortal on syslog brief:\n%s", god.transcript())
	}
}

// ...and a BRF line does, from the same setting: `ban` is BRF's
// neighbour NRM, so this uses the death line, which is BRF (fight.c:953).
// The point is that brief is not "off" — it is the C's own floor.
func TestABriefLineReachesAnImmortalOnSyslogBrief(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	god, _ := twoInARoom(t, srv, addr)

	god.send("syslog brief")
	god.expect("Your syslog is now brief.")

	// slay reaches RawKill, which does not go through damage() and so
	// logs nothing; `advance` does, through gain_exp_regardless
	// (limits.c:357), and is BRF.
	god.send("advance Bystander 3")
	god.expect("Bystander advanced 2 levels to level 3.")
}

// mudlog()'s level argument against the reader's own: `ban` is LVL_GOD
// (ban.c:204), so a level-31 immortal does not see it however high their
// syslog is turned up.
func TestAGodLevelLineDoesNotReachALesserImmortal(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	god, watcher := twoInARoom(t, srv, addr)

	// setLevel (wizmove_test.go) rather than typing `advance`: the
	// dispatcher reads Record.Level on the session's own goroutine to
	// match a command against its level (commands.go:799), so a level
	// written by somebody else's command at the same moment is a genuine
	// race, and -race finds it. inWorld's DoSync gives the ordering that
	// typing `advance` at another connection cannot.
	setLevel(t, srv, "Bystander", game.LevelImmortal)
	watcher.send("syslog complete")
	watcher.expect("Your syslog is now complete.")

	god.send("ban select badsite.example")
	god.expect("Site banned.")

	watcher.settle()
	if watcher.seen("has banned badsite.example") {
		t.Errorf("a LVL_IMMORT reader saw a LVL_GOD line:\n%s", watcher.transcript())
	}

	// The same line does reach a god, so the refusal above is the level
	// test and not the line failing to be produced at all.
	god.send("syslog complete")
	god.expect("Your syslog is now complete.")
	god.send("unban badsite.example")
	god.expect("Site unbanned.")
	god.expect("Zod removed the select-player ban on badsite.example.")
}

// `MAX(LVL_x, GET_INVIS_LEV(ch))`: a wizinvis god acting at level 34 is
// not reported to a god at 32, even though the line's own constant is
// LVL_GOD and that reader qualifies for it.
func TestWizinvisRaisesTheLevelALineIsReportedAt(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	god, watcher := twoInARoom(t, srv, addr)

	setLevel(t, srv, "Bystander", game.LevelGod)
	watcher.send("syslog complete")
	watcher.expect("Your syslog is now complete.")

	inWorld(t, srv, func(w *game.Live) {
		for _, c := range w.Players() {
			if c.Name == "Zod" && c.Record != nil {
				c.Record.InvisLevel = game.LevelImplementor
			}
		}
	})

	god.send("ban new hidden.example")
	god.expect("Site banned.")

	watcher.settle()
	if watcher.seen("has banned hidden.example") {
		t.Errorf("a level-32 god saw a line a level-34 wizinvis god produced:\n%s",
			watcher.transcript())
	}
}

// `if (level < 0) return;` (utils.c:238-239). do_skillset passes -1
// (modify.c:344), which means "log it and show nobody" — not "show
// everybody", which is what reading the level as a threshold gives.
func TestSkillsetEchoesToNobody(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	god, _ := twoInARoom(t, srv, addr)

	god.send("syslog complete")
	god.expect("Your syslog is now complete.")

	god.send("skillset Bystander 'sneak' 50")
	god.expect("You change Bystander's sneak to 50.")

	god.settle()
	if god.seen("changed Bystander's sneak to 50.") {
		t.Errorf("a mudlog at level -1 was echoed in game:\n%s", god.transcript())
	}
}

// A god watching their own doing: do_wizutil's freeze is BRF/LVL_GOD
// (act.wizard.c:2101), and the acting god is themselves a qualifying
// reader — mudlog has no "except the actor" rule.
func TestFreezeEchoesToTheGodWhoDidIt(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	god, _ := twoInARoom(t, srv, addr)

	god.send("syslog complete")
	god.expect("Your syslog is now complete.")

	god.send("freeze Bystander")
	god.expect("Frozen.")
	god.expect("(GC) Bystander frozen by Zod.")
}

// PLR_WRITING's stand-in (utils.c:248): a god in the line editor does not
// get a syslog line dumped into their buffer. The C tests the flag, which
// string_write sets (modify.c:100-101); nothing in this port sets it
// (#214), so sendWizVis tests the connection state instead.
func TestAnImmortalInTheEditorIsNotInterrupted(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	god, mortal := twoInARoom(t, srv, addr)

	// A third connection, purely as a barrier. The echo is queued onto
	// the world goroutine (Server.echoWizVis), so the god's own reply to
	// a command typed afterwards is *not* proof that the echo has been
	// decided yet — and the god is in the editor, where settle()'s `time`
	// would be taken as a line of text rather than a command. A bystander
	// typing a real command goes through the same task queue, in order,
	// so its reply is the barrier the god cannot be.
	barrier := dialClient(t, addr)
	barrier.create("Onlooker", "swordfish", "f", "w")
	god.settle()

	god.send("syslog complete")
	god.expect("Your syslog is now complete.")

	god.send("tedit motd")
	god.expect("Edit file below:")

	mortal.send("quit")
	mortal.expect("Goodbye, friend.. Come back soon!")
	barrier.settle()

	// Now drain the god's own socket: anything the echo wrote would have
	// been written before this reply, on the same output queue. /l on
	// `tedit motd` shows one line, the seeded file, not an empty buffer.
	god.send("/l")
	god.expect("1 line shown.")

	if god.seen("[ Bystander has quit the game. ]") {
		t.Errorf("a syslog line landed inside somebody's edit buffer:\n%s",
			god.transcript())
	}

	// The *room's* own "$n has left the game." does arrive mid-edit, and
	// should: the C guards mudlog with PLR_WRITING and guards act() with
	// nothing at all, so a busy room interrupts an editor there too. It
	// is also what proves the setup worked — without it this test would
	// pass just as well against a server that had dropped the quit.
	if !god.seen("Bystander has left the game.") {
		t.Errorf("the room announcement did not arrive either, so this test "+
			"is not proving anything about the syslog line:\n%s", god.transcript())
	}

	// And once the editor closes, a line does arrive — so the refusal
	// above is the state check and not the echo being broken.
	god.send("/a")
	god.expect("Edit aborted.")
	god.send("ban select afterwards.example")
	god.expect("[ Zod has banned afterwards.example for select players. ]")
}
