// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

//go:build play

package play

import (
	"strings"
	"testing"
	"time"
)

// The boot sequence, which is the half of the server no test outside this
// package touches: reading the world off disk, resetting the zones into it,
// attaching special procedures by vnum, and loading the text files.

// find returns the first log record with this message, and whether there was
// one.
func (m *mud) find(msg string) (logLine, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, l := range m.logs {
		if l.Msg == msg {
			return l, true
		}
	}
	return logLine{}, false
}

// The world the server actually boots on: it loads, the zones reset into
// it, and the specials attach.
//
// This used to be TestTheWorldLoadsTheSameInBothFormats, booting a server
// on examples/mini/binary and another on examples/mini/yaml and comparing
// the two "world loaded" log lines. That question is answered far better
// one level down now: `dlctl verify --against` compares every field of
// every record of every subsystem across all three corpora, on every push
// (docs/proposals/yaml-only.md §5.4), where this compared five counts
// across two boots and only at release time.
//
// What is left is the half that suite alone can answer: that a server
// booted on the shipping directory comes up with a world in it. That is
// not a comparison at all, and internal/server's tests cannot ask it —
// they build their world in Go and never read a file.
func TestTheWorldBootsWithItsZonesAndSpecials(t *testing.T) {
	m := start(t, mini)

	if _, ok := m.find("world loaded"); !ok {
		t.Fatalf("the server never logged what it loaded. Its log was:\n%s", m.logText())
	}

	// The zone reset is what puts anything *in* the rooms: without
	// it the mini world is sixteen empty rooms.
	populated, ok := m.find("world populated")
	if !ok {
		t.Fatalf("the zones were never reset. The log was:\n%s", m.logText())
	}
	if !strings.Contains(populated.raw, `"problems":0`) {
		t.Errorf("the zone reset reported problems: %s", populated.raw)
	}

	// And the specials, which are attached by vnum from a fixed
	// table (internal/game/specassign.go) rather than declared by
	// the world files. The mini world reuses the real table's vnums
	// on purpose so that its guildmaster, postmaster, receptionist,
	// bank and board are real ones -- examples/mini/README.md's
	// "Vnums" section is about exactly this, and it is the sort of
	// thing that silently stops being true.
	specials, ok := m.find("special procedures assigned")
	if !ok {
		t.Fatal("no specials were assigned")
	}
	contains(t, "the specials", specials.raw, `"attached":5`, `"shopkeepers":1`)

	m.noServerErrors()
}

// TestAZoneResetPutsEverythingBack.
//
// The reset is what makes the tutorial repeatable -- kill the dummy, take the
// loot, and twenty minutes later it is all there again -- and it is a whole
// subsystem (the zone commands in examples/mini/binary/world/zon/1.zon) that
// runs once at boot and then never again inside a test's lifetime. `zreset`
// is the immortal command that runs it on demand, which is the only way to
// see it happen in a test that is not twenty minutes long.
func TestAZoneResetPutsEverythingBack(t *testing.T) {
	m := start(t, mini)

	tourist := m.dial()
	tourist.create("Tourist", "tourpass", "m", "w")

	// Armed on the way through the Armory: barehanded, a level 1 warrior
	// against 2d4+4 hit points turns this into the slowest test in the
	// suite for no reason connected to what it is testing.
	tourist.toRoom(roomArmory)
	tourist.do("get sword")
	tourist.do("wield sword")
	tourist.north(roomSparringRing - roomArmory)

	// Strip the room bare: the loot into a pack, the dummy into a corpse.
	tourist.do("get all")
	tourist.do("kill dummy")
	tourist.expect("R.I.P.")
	before := tourist.do("look")
	missing(t, "the emptied Sparring Ring", before,
		"A slender wand rests here", "A straw-stuffed training dummy stands here")

	god := m.god()
	contains(t, "zreset", god.do("zreset 1"), "Reset zone 1")

	after := tourist.do("look")
	contains(t, "the Sparring Ring after a reset", after,
		"A straw-stuffed training dummy stands here, patiently waiting to be hit.")

	// The loot does *not* come back, and that is the reset doing its job
	// rather than failing it. Every `O` command in the zone file carries a
	// max-existing of 1 (examples/mini/binary/world/zon/1.zon), so an object
	// still in somebody's pack counts and is not loaded a second time. A
	// reset that ignored that would let a zone be farmed for infinite
	// copies of everything in it, which is the bug the field exists to
	// prevent.
	missing(t, "the loot the player is still carrying", after,
		"A slender wand rests here, humming faintly.",
		"A single scroll lies here, its edges curling.")
	contains(t, "and it is still in the pack", tourist.do("inventory"), "a wand", "a scroll")

	m.noServerErrors()
}

// TestTheHelpDatabaseIsReadable. `help` is loaded off disk at boot from a
// real indexed database, and `help circlemud` in particular is a licence
// obligation (docs/proposals/go-port-plan.md §12) that only means anything if
// a player can actually reach it.
func TestTheHelpDatabaseIsReadable(t *testing.T) {
	m := start(t, mini)
	c := m.dial()
	c.create("Tourist", "tourpass", "m", "w")

	contains(t, "help", c.doUntil("help", promptMarker),
		"Further information available by HELP <keyword>")

	// The licence entry, and the credits command beside it.
	credits := c.doUntil("help circlemud", "Return to continue")
	contains(t, "help circlemud", credits, "CircleMUD")
	c.doUntil("q", promptMarker)

	contains(t, "an unknown keyword", c.do("help nosuchhelptopic"),
		"There is no help on that word.")

	m.noServerErrors()
}

// TestTheClockKeepsTimeAcrossARestart.
//
// The mud clock's epoch is persisted (etc/time under classic, state/clock
// under yaml) and reloaded on boot, so that a restart does not send the game
// back to the beginning of time. save_mud_time runs on the way down, which
// makes this a shutdown test as much as a clock one.
func TestTheClockKeepsTimeAcrossARestart(t *testing.T) {
	m := start(t, mini)
	c := m.dial()
	c.create("Tourist", "tourpass", "m", "w")

	before := c.do("time")
	contains(t, "the clock", before, "o'clock")
	c.quit()
	c.close()

	m2 := m.restart(mini)
	back := m2.dial()
	back.login("Tourist", "tourpass")

	after := back.do("time")
	contains(t, "the clock after a restart", after, "o'clock")

	// The day of the month is the coarsest thing the clock prints and the
	// one least likely to tick over between the two reads, so it is what is
	// compared: an epoch that failed to persist restarts the calendar
	// entirely rather than moving it on by an hour.
	if day(before) != day(after) {
		t.Errorf("the calendar moved across a restart:\nbefore: %s\nafter:  %s", before, after)
	}

	m2.noServerErrors()
}

// day pulls the "It is the Nth Day of the ..." line out of what `time`
// printed.
func day(out string) string {
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "Day of the") {
			return strings.TrimSpace(line)
		}
	}
	return ""
}

// TestTheServerSurvivesNonsense. Every command in the table, given the sort
// of argument a player types by accident.
//
// This asserts almost nothing about the replies on purpose: what it is
// looking for is the server still being there afterwards, and no ERROR in its
// log -- which is where a panic contained by the world goroutine's recover
// shows up (internal/engine/engine.go:227). A new command that dereferences
// something it did not check fails here without anybody having had to think
// of the case.
func TestTheServerSurvivesNonsense(t *testing.T) {
	m := start(t, mini)
	c := m.dial()
	c.create("Tourist", "tourpass", "m", "w")

	for _, command := range []string{
		"", " ", "look at", "look in", "get", "get from", "put", "put in",
		"give", "drop all", "wear", "remove", "wield", "hold",
		"open", "close", "lock", "unlock", "pick",
		"kill", "hit", "flee", "rescue", "backstab", "kick", "bash",
		"cast", "cast ''", "cast 'magic missile'", "recite", "quaff", "use",
		"buy", "sell", "value", "list", "offer", "rent", "receive", "check",
		"deposit", "withdraw", "balance", "mail",
		"tell", "say", "whisper", "ask", "emote", "group", "follow", "split",
		"score", "inventory", "equipment", "exits", "time", "weather", "who",
		"practice", "skills", "spells", "title", "prompt", "display", "toggle",
		"north", "up", "down", "enter", "leave", "sleep", "wake", "stand", "rest",
		"consider", "diagnose", "eat", "drink", "fill", "pour", "sip", "taste",
		"alias", "alias x", "unalias x", "read", "write",
		"@@@@", "look 999999", "get -1", "buy 0", "purge", "goto",
	} {
		if got := c.do(command); strings.Contains(got, "panic") {
			t.Errorf("%q printed a panic:\n%s", command, got)
		}
	}

	// Still alive and still playing. Not asserted by room -- `north` is in
	// the list above and does what it says -- but by the character still
	// being there to answer at all.
	contains(t, "after all that", c.do("score"), "This ranks you as Tourist")
	contains(t, "and still in a room", c.do("look"), "[ Exits:")
	m.noServerErrors()
}

// TestARestartKeepsWhatWasSaved is the operator's question rather than the
// player's: a server that has been stopped and started again is the same
// game, with the same characters, their things and their money.
func TestARestartKeepsWhatWasSaved(t *testing.T) {
	m := start(t, mini)
	c := m.dial()
	c.create("Tourist", "tourpass", "m", "w")
	c.toRoom(roomSparringRing)
	c.do("get pouch")
	contains(t, "before the restart", c.do("score"), "75 gold coins")
	c.quit()
	c.close()

	if !eventually(10*time.Second, func() bool { return m.rosterHas("Tourist") }) {
		t.Fatal("the character never reached the roster index")
	}

	m2 := m.restart(mini)
	back := m2.dial()
	back.login("Tourist", "tourpass")

	contains(t, "after the restart", back.do("score"), "75 gold coins")
	contains(t, "what came back", back.do("inventory"), "a rusty key")

	m2.noServerErrors()
}
