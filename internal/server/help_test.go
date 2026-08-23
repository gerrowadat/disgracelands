// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import "testing"

// `help` alone shows the screen, `help <keyword>` a real entry, and an
// unknown word the C's own miss message — do_help (act.informative.c:
// 953-991) end to end, against the real archived data testText loads.
func TestHelp(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "password123", "m", "w")

	c.send("help")
	c.expect("Further information available by HELP <keyword>")

	// ALIAS's own real entry runs to 49 lines — comfortably over
	// PAGE_LENGTH, so it pages (the pager, step 6c-vii). "q" closes it
	// before the next query, or "help nonsensicalgibberish" would be
	// read as pager input rather than a command.
	c.send("help alias")
	c.expect("An alias is a single command used to represent")
	c.expect("Return to continue")
	c.send("q")

	c.send("help nonsensicalgibberish")
	c.expect("There is no help on that word.")
}

// A short, ambiguous query resolves to whichever matching keyword sorts
// first — do_help's own backward-walk (act.informative.c:975-978), not
// something a port should "fix". "battle" is a prefix of both BATTLECRY
// and (via WARRIOR's "See also") nothing shorter in the real data, so this
// uses the same synthetic ambiguity internal/game/help_test.go's
// TestHelpIndexLookup already proves the algorithm on — this test proves
// the live command path reaches the same lookup, not a second copy of it.
func TestHelpResolvesAmbiguousPrefixesToTheAlphabeticallyFirstMatch(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "password123", "m", "w")

	// "wh" matches both WHERE and WHO in commands.hlp; WHERE sorts first.
	c.send("help wh")
	c.expect("WHERE")
}
