// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"strings"
	"testing"
)

// process_input, the rest of it (#238). #233 ported the backspace rub-out
// because it was actively corrupting commands from the browser; these are the
// other five things the C does to a line on its way in, none of which had a
// command-table entry to find them by — they live in comm.c, which is why
// grepping interpreter.c for `"!"` turns up nothing at all.

// TestBangRepeatsTheLastCommand is `!` (comm.c:1816-1817), which replays
// last_input and does not echo it.
func TestBangRepeatsTheLastCommand(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	c.send("gold")
	c.expect("You're broke!")

	c.send("!")
	if got := c.expectCount("You're broke!", 2); !strings.Contains(got, "You're broke!") {
		t.Errorf("`!` did not repeat the last command:\n%s", got)
	}

	// Twice running replays the same line rather than replaying the `!`:
	// the C's bare-`!` branch never writes last_input.
	c.send("!")
	c.expectCount("You're broke!", 3)
}

// TestBangPrefixSearchesTheHistory is `!<prefix>` (comm.c:1818-1834): the
// most recent command the prefix abbreviates, echoed back and then run.
func TestBangPrefixSearchesTheHistory(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	c.send("gold")
	c.expect("You're broke!")
	c.send("time")
	c.expect("o'clock")

	// `!go` reaches past `time` to `gold`, and says so on the way.
	c.send("!go")
	got := c.expectCount("You're broke!", 2)
	if !strings.Contains(got, "gold\r\n") {
		t.Errorf("`!go` did not echo the command it found:\n%s", got)
	}

	// A leading space is skipped, which is skip_spaces on the C's own
	// `commandln`.
	c.send("! go")
	c.expectCount("You're broke!", 3)

	// And a prefix that matches nothing is not an error: the line goes to
	// the interpreter as typed, which does not know it.
	c.send("!zzzz")
	if got := c.expect("Huh?!?"); !strings.Contains(got, "Huh?!?") {
		t.Errorf("an unmatched `!` prefix was not passed through:\n%s", got)
	}
}

// TestTheHistoryOnlyEverFindsFourOfItsFive is the C's own arithmetic, and it
// is the sort of thing only a test states out loud.
//
// HISTORY_SIZE is 5 and its comment says "Keep last 5 commands", which is
// true of what is stored. The search walks from history_pos-1 while `cnt !=
// starting_pos` where starting_pos is history_pos, so the slot the next line
// will overwrite — the oldest of the five — is passed over every time. Five
// commands in, and the first of them cannot be recalled.
func TestTheHistoryOnlyEverFindsFourOfItsFive(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	// `gold` first, then exactly four more, so gold is the oldest of five
	// and sitting in the slot the walk passes over.
	//
	// Four distinct `say`s rather than four different commands, and
	// deliberately not settle(): settle types `time`, which would put two
	// entries in the history per iteration and push gold out of the buffer
	// altogether — a test that passes for the wrong reason, and one this
	// was until the mutation said so.
	c.send("gold")
	c.expect("You're broke!")
	for _, word := range []string{"aaa", "bbb", "ccc", "ddd"} {
		c.send("say " + word)
		c.expect("You say, '" + word + "'")
	}

	before := strings.Count(c.transcript(), "You're broke!")
	c.send("!go")
	c.expect("Huh?!?")
	if after := strings.Count(c.transcript(), "You're broke!"); after != before {
		t.Errorf("the oldest of the five history entries was recalled "+
			"(`gold` ran %d more times); the C's walk skips it", after-before)
	}

	// One more line pushes gold out of the buffer altogether, which is the
	// same answer for a different reason.
	c.send("say eee")
	c.expect("You say, 'eee'")
	c.send("!go")
	c.expectCount("Huh?!?", 2)
	if after := strings.Count(c.transcript(), "You're broke!"); after != before {
		t.Errorf("`gold` ran again after being overwritten in the history")
	}
}

// TestCaretSubstitutionRerunsTheLastCommand is `^old^new` (perform_subst,
// comm.c:1874), the csh-ism.
func TestCaretSubstitutionRerunsTheLastCommand(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	c.send("say hello")
	c.expect("You say, 'hello'")

	c.send("^hello^goodbye")
	if got := c.expect("You say, 'goodbye'"); !strings.Contains(got, "You say, 'goodbye'") {
		t.Errorf("the substitution did not run:\n%s", got)
	}

	// It replaces the first occurrence only, and writes the result back as
	// the new last command — so a second substitution works on the first
	// one's output.
	c.send("^goodbye^hello")
	c.expectCount("You say, 'hello'", 2)

	// The stock help file's own example is wrong, and this is the code
	// rather than the documentation. `help !` offers `^you^you doing^`,
	// which is the csh spelling; perform_subst reads everything after the
	// second `^` as the replacement, so the third one is substituted in.
	c.send("say how are you")
	c.expect("You say, 'how are you'")
	c.send("^you^you doing^")
	if got := c.expect("you doing"); !strings.Contains(got, "You say, 'how are you doing^'") {
		t.Errorf("the help file's own example does not do what the help file says, "+
			"and this port follows the code:\n%s", got)
	}

	// Two ways to fail, both answered and neither run.
	c.send("^nosuchtext^whatever")
	c.expect("Invalid substitution.")
	c.send("^unterminated")
	c.expectCount("Invalid substitution.", 2)

	// And the player still has a prompt afterwards: the C's game loop
	// writes one whether a command ran or not.
	c.send("gold")
	c.expect("You're broke!")
}

// TestSnoopingShowsWhatTheVictimTypes is comm.c:1813-1814, the half of
// `snoop` this port did not have.
//
// The output half was already relayed, so a snooper saw every answer and
// none of the questions — which is most of the way to useless for the thing
// snoop exists for.
func TestSnoopingShowsWhatTheVictimTypes(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	god := dialClient(t, addr)
	god.create("Watcher", "overyourshoulder", "m", "w")
	victim := dialClient(t, addr)
	victim.create("Watched", "beingobserved", "m", "w")
	setLevel(t, srv, "Watched", 10)

	god.send("snoop watched")
	god.expect("Okay.")

	victim.send("gold")
	victim.expect("You're broke!")
	if got := god.expect("% gold"); !strings.Contains(got, "% gold") {
		t.Errorf("the snooper did not see what was typed:\n%s", got)
	}

	// What was typed, not what it became: the copy happens before the
	// history substitution, so `!` shows as `!`.
	victim.send("!")
	victim.expectCount("You're broke!", 2)
	if got := god.expect("% !"); !strings.Contains(got, "% !") {
		t.Errorf("the snooper saw the expansion rather than the keystrokes:\n%s", got)
	}
}

// TestControlCharactersNeverReachACommand is the isprint half of the C's
// input filter (comm.c:1796).
//
// Only the isprint half: the isascii half would throw away every byte above
// 127, and this port takes UTF-8 on purpose. See readLoop's own note.
func TestControlCharactersNeverReachACommand(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))

	c.expect("By what name")
	// ESC, and a tab, which isprint excludes too. What is left is a
	// perfectly ordinary name.
	c.sendRaw([]byte("\x1bZ\tod\r\n"))
	if got := c.expect("(Y/N)"); !strings.Contains(got, "Did I get that right, Zod (Y/N)?") {
		t.Errorf("a control character reached the name:\n%s", got)
	}
}

// TestALineTooLongIsTruncatedAndSaidSo is MAX_INPUT_LENGTH (structs.h:560).
//
// The port bounded a line at 64KB and cut it silently. The limit is the C's
// now, and the message is the C's words — see docs/deviations.md for the one
// thing about it that is not the C's behaviour, which is that the C's own
// message is unreachable for a line of ordinary text.
func TestALineTooLongIsTruncatedAndSaidSo(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	// `say` plus enough text to run past the limit. What comes back is the
	// warning, and then the truncated line actually running.
	said := strings.Repeat("x", 300)
	c.send("say " + said)
	got := c.expect("Line too long.  Truncated to:")
	if !strings.Contains(got, "Line too long.  Truncated to:") {
		t.Fatalf("an over-long line was truncated silently:\n%s", got)
	}

	spoken := c.expect("You say, '")
	// 254 characters survive, of which four are "say ".
	if !strings.Contains(spoken, "You say, '"+strings.Repeat("x", 250)+"'") {
		t.Errorf("the truncated line is not 254 characters:\n%s", spoken)
	}
}

// TestATypedDollarIsLeftAlone is the `$`-doubling half of #238, and the
// answer to it is that this port does not double and so must not halve.
//
// The C doubles every `$` in a line on the way in off the socket
// (comm.c:1806-1810) so that player text cannot later be read as an act()
// code, and halves it again at the far end — in act() itself, or in the
// handful of delete_doubledollar calls. This port had the halving and never
// the doubling, so a typed `$$` came out as `$`. Now it has neither.
func TestATypedDollarIsLeftAlone(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	god, mortal := twoInARoom(t, srv, addr)

	// A `$n` typed at somebody must reach them as `$n`, not as the sender's
	// name. This is what the C's doubling buys, and what passing the text
	// to act() as $T buys here instead.
	mortal.send("tell Zod costs $n gold")
	if got := god.expect("tells you"); !strings.Contains(got, "tells you, 'costs $n gold'") {
		t.Errorf("an act() code typed by a player was expanded:\n%s", got)
	}

	// And a doubled dollar stays doubled, all the way to both ends of the
	// conversation.
	mortal.send("say costs $$5")
	mortal.expect("You say, 'costs $$5'")
	if got := god.expect("says, 'costs"); !strings.Contains(got, "says, 'costs $$5'") {
		t.Errorf("a typed `$$` was collapsed:\n%s", got)
	}
}

// TestAPasswordIsNeverPutInTheHistory is the one thing in this port's
// process_input that is not the C's behaviour.
//
// The C records every line, whatever state the descriptor is in, so a
// password typed at the login prompt went into the history in the clear and
// `!s` would find it, echo it back and run it as a command. Nothing here is
// recorded, recalled or relayed while the server has told the client to stop
// echoing. The same rule the browser terminal's up-arrow already follows
// (#235). See docs/deviations.md and input.go's `recordable`.
func TestAPasswordIsNeverPutInTheHistory(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	first := dialClient(t, addr)
	first.create("Zod", "swordfish", "m", "w")
	first.send("quit")
	first.expect("Goodbye")
	first.close()
	waitForLogout(t, srv, "Zod")

	// A login, rather than the creation sequence: it types four lines, of
	// which the password is the third-most-recent by the time the game
	// starts, and so squarely inside the four the search can reach. (The
	// creation sequence types enough afterwards to push the password into
	// the one slot `!` skips over — see the test above — which would have
	// made this pass for the wrong reason.)
	c := dialClient(t, addr)
	c.login("Zod", "swordfish")

	// `swordfish` was typed two lines ago and must be unreachable.
	c.send("!sw")
	c.expect("Huh?!?")
	if c.seen("swordfish") {
		t.Errorf("the password was echoed back out of the history:\n%s", c.transcript())
	}

	// Nor as last_input, which `!` on its own would replay.
	c.send("!")
	c.settle()
	if c.seen("swordfish") {
		t.Errorf("the password was replayed by a bare `!`:\n%s", c.transcript())
	}
}

// TestAPasswordMayBeginWithABangOrACaret is the other half of not recalling
// while echo is off, and it is a usability trap the real server had.
//
// `^secret^` is a legal password here — badNewPassword asks for six
// characters and nothing else — and on the C server it was unusable:
// process_input runs for the password prompt too, so every attempt to type
// it was answered "Invalid substitution." and the password itself never
// reached the prompt.
func TestAPasswordMayBeginWithABangOrACaret(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	const password = "^secret^"
	first := dialClient(t, addr)
	first.create("Zod", password, "m", "w")
	first.send("quit")
	first.expect("Goodbye")
	first.close()
	waitForLogout(t, srv, "Zod")

	c := dialClient(t, addr)
	c.expect("By what name")
	c.send("Zod")
	c.expect("Password:")
	c.send(password)
	if got := c.expect("PRESS RETURN"); !strings.Contains(got, "PRESS RETURN") {
		t.Errorf("a password beginning with `^` was read as a substitution:\n%s", got)
	}
}
