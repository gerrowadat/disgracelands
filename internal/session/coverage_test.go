// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package session

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// How much of the C's command table answers, derived rather than counted by
// hand.
//
// This exists because the figure had been wrong in four different ways across
// six documents inside a fortnight. It is prose in `README.md`, `BUILDING.md`,
// `TODO.md`, `docs/README.md`, `docs/deviations.md` and the plan, it changes
// every time anybody ports a command, and two branches touching it at once
// conflict in a way that looks textual and is actually factual — both sides
// wrong, on two separate occasions.
//
// So the *list* is the assertion and the numbers fall out of it. Port a
// command and this test fails until the name comes off `notPorted`, which is
// the reminder to fix the six documents in the same commit.

// notPorted is every row of `cmd_info[]` that nothing answers to yet.
//
// `RESERVED` is not typeable — `any_one_arg` lowercases the typed word
// (interpreter.c:1028) and the name is upper case — so it can never be
// matched and is not a gap. It is listed here because it *is* a row, and
// leaving it out would make the arithmetic look wrong.
var notPorted = map[string]string{
	"RESERVED": "a placeholder so a specproc can return command 0; unmatchable",

	// Phase 6, by design.
	"medit": "OasisOLC", "oedit": "OasisOLC", "redit": "OasisOLC",
	"sedit": "OasisOLC", "zedit": "OasisOLC", "olc": "OasisOLC",
	"edit": "OasisOLC", "tedit": "the in-game text-file editor",

	// Blocked on something.
	"color":     "the PRF_COLOR bits are stored and set color works, but nothing emits colour",
	"slowns":    "flips a server-wide global; needs somewhere to live that is not a package variable",
	"trackthru": "flips a server-wide global; game.TrackThroughDoors is read by the BFS already",

	// Simply not written yet.
	"skillset": "sets one skill on one character",
	"reload":   "re-reads the text files",
}

// TestEveryCommandIsPortedOrListed compares the C's table against ours.
func TestEveryCommandIsPortedOrListed(t *testing.T) {
	src, err := os.ReadFile("../../reference/moderncserver/src/interpreter.c")
	if err != nil {
		t.Fatalf("reading interpreter.c: %v", err)
	}

	entry := regexp.MustCompile(`^\s*\{ *"([^"]+)"\s*,`)
	inTable := map[string]bool{}
	for _, line := range strings.Split(string(src), "\n") {
		if m := entry.FindStringSubmatch(line); m != nil && m[1] != `\n` {
			inTable[m[1]] = true
		}
	}
	if len(inTable) == 0 {
		t.Fatal("no commands parsed out of interpreter.c")
	}

	// Commands holds only the static table in a unit test: the socials are
	// interleaved by RegisterSocials at boot, from socialLines. Both count.
	ported := map[string]bool{}
	for _, cmd := range Commands {
		ported[cmd.Name] = true
	}
	for name := range socialLines {
		ported[name] = true
	}

	var missing, unexpected, ghosts []string
	for name := range inTable {
		switch {
		case ported[name] && notPorted[name] != "":
			unexpected = append(unexpected, name)
		case !ported[name] && notPorted[name] == "":
			missing = append(missing, name)
		}
	}
	for name := range notPorted {
		if !inTable[name] {
			ghosts = append(ghosts, name)
		}
	}
	sort.Strings(missing)
	sort.Strings(unexpected)
	sort.Strings(ghosts)

	for _, name := range unexpected {
		t.Errorf("%q is ported and still listed in notPorted — take it out, and "+
			"update the command counts in README.md, BUILDING.md, TODO.md, "+
			"docs/README.md, docs/deviations.md and the plan's §10", name)
	}
	for _, name := range missing {
		t.Errorf("%q is in the C's table, is not ported, and is not in notPorted", name)
	}
	for _, name := range ghosts {
		t.Errorf("%q is in notPorted and not in the C's table at all", name)
	}

	// The documents count *typeable* commands, so RESERVED is out of both
	// halves: 319 rows, 318 of them reachable.
	t.Logf("%d of the C's %d typeable commands answer; %d rows listed as not ported",
		len(inTable)-len(notPorted), len(inTable)-1, len(notPorted)-1)
}

// TestTheCommandCountsInTheDocsAreRight reads the figure back out of the prose.
//
// Four documents state it in a sentence. They have been wrong four separate
// times, always because somebody ported a command in one branch while somebody
// else corrected the arithmetic in another. Reading it back is cheap and the
// alternative is doing this by hand again.
func TestTheCommandCountsInTheDocsAreRight(t *testing.T) {
	src, err := os.ReadFile("../../reference/moderncserver/src/interpreter.c")
	if err != nil {
		t.Fatalf("reading interpreter.c: %v", err)
	}
	entry := regexp.MustCompile(`^\s*\{ *"([^"]+)"\s*,`)
	rows := map[string]bool{}
	for _, line := range strings.Split(string(src), "\n") {
		if m := entry.FindStringSubmatch(line); m != nil && m[1] != `\n` {
			rows[m[1]] = true
		}
	}
	// RESERVED is a row and not a command, so it comes out of both halves:
	// the documents say "304 of 318", which is 319 rows less that one.
	typeable := len(rows) - 1
	answer := len(rows) - len(notPorted)

	stated := regexp.MustCompile(`(\d+) of the C's (\d+) commands`)
	checked := 0
	for _, path := range []string{
		"../../README.md", "../../BUILDING.md", "../../docs/README.md",
	} {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("reading %s: %v", path, err)
			continue
		}
		for _, m := range stated.FindAllStringSubmatch(string(body), -1) {
			checked++
			if m[1] != itoa(answer) || m[2] != itoa(typeable) {
				t.Errorf("%s says %q; the table says %d of %d",
					path, m[0], answer, typeable)
			}
		}
	}
	if checked == 0 {
		t.Error("no document states the command count; one of them should")
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}

// TestConnectedNamesMatchTheCSource re-parses `connected_types[]`.
//
// The C indexes it by CON_*, this port indexes by State, and the two orders
// differ — so the mapping is written out by hand and this is what stops a
// state being labelled as its neighbour. Checked by *set* rather than by
// index, since the orders are not comparable.
func TestConnectedNamesMatchTheCSource(t *testing.T) {
	src, err := os.ReadFile("../../reference/moderncserver/src/constants.c")
	if err != nil {
		t.Fatalf("reading constants.c: %v", err)
	}

	block := regexp.MustCompile(`(?s)connected_types\[\] = \{(.*?)\};`).FindSubmatch(src)
	if block == nil {
		t.Fatal("connected_types not found in constants.c")
	}

	inC := map[string]bool{}
	for _, m := range regexp.MustCompile(`"([^"\\]*)"`).FindAllStringSubmatch(string(block[1]), -1) {
		if m[1] != "" {
			inC[m[1]] = true
		}
	}
	if len(inC) == 0 {
		t.Fatal("no names parsed out of connected_types")
	}

	for state, name := range connectedNames {
		if !inC[name] {
			t.Errorf("state %v is called %q, which is not in the C's connected_types", state, name)
		}
	}

	// Every state the port has must have a name, or `users` prints "Unknown"
	// at somebody.
	for s := StateGetName; s <= StateClosed; s++ {
		if connectedNames[s] == "" {
			t.Errorf("state %v has no connected_types name", s)
		}
	}
}
