// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

//go:build parity

package parity

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/gerrowadat/disgracelands/internal/parity"
)

// scenario is one script, played at both servers.
//
// The scripts themselves are in testdata/parity/, in the same format
// scripts/session-parity.sh plays, so a scenario can be run on its own by
// hand while it is being written. What lives here rather than in the file is
// everything the comparison needs to know: what the scenario is evidence
// about, how long to wait for an answer, and which of its differences have
// already been triaged.
type scenario struct {
	// name is both the subtest name and testdata/parity/<name>.session.
	name string
	// about is one line on what this scenario is evidence of. It is printed
	// with a failure, because "shops disagree" is a more useful thing to
	// read at the top of a diff than the name of a file.
	about string
	// quiet is how long a server must be silent before its answer is taken
	// to be complete. Zero takes the default. A scenario that waits for
	// something on a timer — a combat round, a mobile's own act — needs
	// longer than one that only ever answers immediately.
	quiet time.Duration
	// colourGap makes the scenario about the ANSI rather than about the
	// text: the two servers must agree once the colour is stripped, and
	// must *disagree* with it left in. Exactly one scenario sets it; see
	// its own comment for what it is pinning down and why every other
	// scenario compares with the colour stripped.
	colourGap bool
	// known are the differences already triaged. See known's own comment:
	// this is a list of decisions, not a list of excuses.
	known []known
}

// known is one triaged difference: a pattern for lines the two servers are
// allowed to disagree about, and the reason they do.
//
// Precondition 2 of the plan's Phase 7 is not "make the two servers agree" —
// it is that every difference is either fixed or *decided*. So an entry here
// has to say what the difference is and where the decision is written down,
// and a differing line no entry matches fails the suite.
//
// Matched against the *line*, not against the command, because one difference
// is rarely one command. A character whose hit points are rolled differently
// differs in the prompt after every command in the script; twenty entries
// naming twenty commands would say that badly, and would go stale the moment
// a line moved. One pattern says it once.
//
// It cuts both ways: an entry that matches nothing also fails, with "delete
// it". A triage list nobody prunes becomes a list of things that used to be
// true, which is worse than no list at all.
type known struct {
	// command narrows the entry to the answer to one typed line, matched
	// exactly. Empty means every block: a difference that shows up
	// everywhere — the prompt, the menu after `quit` — is one entry rather
	// than one per command.
	command string
	// match is a regular expression, matched against each differing line.
	match string
	// why says what the difference is and where it is written down.
	why string
}

// scenarios are the scripts, in the order they are declared.
//
// Each scenario gets its own pair of freshly booted servers (startPair), so
// the order they are declared in is presentation rather than dependency:
// nothing one scenario does is visible to the next. What a scenario needs to
// exist, its own script makes, which is what !reconnect is for — see
// mail.session, which has four connections and two characters in it.
// Two differences show up in every scenario, so they are named once and
// composed into the tables below rather than written out eleven times. Both
// are in docs/deviations.md, under "What the session-parity suite found".
var (
	// `quit` leaves the C server at the main menu — extract_char() puts the
	// descriptor back into CON_MENU — and disconnects here
	// (internal/session/commands.go's doQuit, Session.Close).
	quitReturnsToTheMenu = known{
		match: `^(Welcome to CircleMUD!|[0-5]\) .*|   Make your choice:)$`,
		why:   "`quit` returns to the menu in the C and disconnects here; docs/deviations.md",
	}
	// The prompt. It differs for two reasons at once and they cannot be
	// told apart line by line: the C sends one where this port does not
	// (after `quit`, on the way to the menu above), and a mortal's vitals
	// themselves differ — hit points are rolled differently at creation,
	// and this port never charges movement points for walking.
	theVitalsPrompt = known{
		match: `^\d+H \d+M \d+V >$`,
		why:   "the prompt: the C sends one this port does not, and a mortal's rolled HP and unspent movement differ; docs/deviations.md",
	}
)

// theShopkeepersTell is the same finding wherever a shop or a postmaster
// speaks, so it is written once too.
//
// The C builds its shop messages as "%s %s" of the *player's name* and the
// message (shop.c's MSG_* macros) and hands the result to do_tell, which
// eats the first word as the addressee — so the name never reaches the
// player. This port keeps it in the text. The capital is the same call: the
// C's do_tell capitalises the speaker's short description.
var theShopkeepersTell = known{
	match: `tells you, '`,
	why:   "the C's shop and mail messages go through do_tell, which eats the leading name and capitalises the speaker; docs/deviations.md",
}

var scenarios = []scenario{
	{
		name:  "login-and-look",
		about: "creating a character, and the wording of what a player reads first",
		known: []known{
			quitReturnsToTheMenu,
			theVitalsPrompt,
			{
				command: "who",
				match:   `^(\[.*\] Parityone the Implementor|One lonely character displayed\.|1 character playing\.)$`,
				why:     "who: the C prints the class abbreviation for immortals too, and counts in words; docs/deviations.md",
			},
			{
				command: "commands",
				match:   `.`,
				why:     "the C lists `'` and `:` among the commands and `hop` among the socials; this port has them the other way round, which moves every column; docs/deviations.md",
			},
			{
				command: "socials",
				match:   `.`,
				why:     "as `commands`: `hop` is a social in the C and a command here; docs/deviations.md",
			},
			{
				command: "exits",
				match:   `^(North|East|South|West|Up|Down) +- `,
				why:     "exits: the C shows room vnums to an immortal and this port does not; docs/deviations.md",
			},
		},
	},
	{
		// The largest single difference between the two servers, pinned
		// down in one place so it stops being noise on top of every other
		// finding.
		//
		// The C's new-character defaults turn colour on for everybody
		// (interpreter.c:1616, a <DoC> local change this port has as
		// game.ApplyNewCharacterDefaults), and the C then colours what it
		// prints through the CCCYN()/CCYEL() family — room names, exits,
		// the fight. This port renders the &-codes embedded in text
		// (internal/colour, Session.SendAt) and has none of those call-site
		// wrappers, so it sends the same words with no escape sequences at
		// all. docs/deviations.md has it.
		//
		// Stated as "they agree about the text and disagree about the
		// colour", which is exactly what is true and is self-pruning: when
		// the port grows the C's call-site colour, this scenario fails with
		// "they now agree" rather than passing quietly on a stale note.
		name:      "colour",
		about:     "the ANSI a new character is meant to be seeing",
		colourGap: true,
		known: []known{
			quitReturnsToTheMenu,
			theVitalsPrompt,
			{
				command: "who",
				match:   `^(\[.*\] Parityone the Implementor|One lonely character displayed\.|1 character playing\.)$`,
				why:     "as login-and-look's `who`; docs/deviations.md",
			},
			{
				command: "exits",
				match:   `^(North|East|South|West|Up|Down) +- `,
				why:     "as login-and-look's `exits`; docs/deviations.md",
			},
		},
	},
	{
		name:  "objects",
		about: "getting, wearing, wielding and putting things, and the refusals",
		known: []known{
			quitReturnsToTheMenu,
			theVitalsPrompt,
			{
				command: "remove all",
				match:   `.`,
				why:     "`remove all` is not the keyword `all` in the C; this port looks for an object called that; docs/deviations.md",
			},
			{
				command: "inventory",
				match:   `^(a shield|a chain mail shirt|a small sword|a bag)$`,
				why:     "what `remove all` above left on rather than in hand, and the order the rest are listed in; docs/deviations.md",
			},
			{
				command: "look",
				match:   `^An automatic teller machine has been installed in the wall here\.$`,
				why:     "the order a room's objects are listed in; docs/deviations.md",
			},
			{
				command: "get 2.sword",
				match:   `^You don't see a (2\.)?sword here\.$`,
				why:     "the C strips the count out of the name before saying it back; docs/deviations.md",
			},
		},
	},
	{
		name:  "fountains",
		about: "look_in_obj's drink-container branch: fullness[] and color_liquid[]",
		known: []known{
			quitReturnsToTheMenu,
			theVitalsPrompt,
		},
	},
	{
		name:  "combat",
		about: "a fight, round by round, and what a death says",
		// A round is two seconds (PULSE_VIOLENCE), so a command that starts
		// a fight is not finished answering for several of them.
		quiet: 3 * time.Second,
		known: []known{
			quitReturnsToTheMenu,
			theVitalsPrompt,
			{
				command: "kill fido",
				match:   `^(Your blood freezes as you hear the beastly fido's death cry\.|the beastly fido is dead!  R\.I\.P\.)$`,
				why:     "death: the C sends the death cry to the room and the killer's own line once; this port sends the room's line to the killer twice and no cry; docs/deviations.md",
			},
			{
				command: "look",
				match:   `^(A small sword lies here\.|An automatic teller machine has been installed in the wall here\.|The corpse of the beastly fido is lying here\.)$`,
				why:     "the order a room's objects are listed in, and the corpse's own contents; docs/deviations.md",
			},
			{
				command: "kill self",
				match:   `.`,
				why:     "the C answers `kill self` with its own refusal; this port does not resolve `self` as a target at all; docs/deviations.md",
			},
			{
				command: "flee",
				match:   `.`,
				why:     "fleeing picks a random exit and the two servers pick different ones; docs/deviations.md",
			},
		},
	},
	{
		name:  "shops",
		about: "buying, selling, listing and valuing, and a shopkeeper's refusals",
		// The keeper answers on its own pulse rather than in the reply to
		// the command, which is why this one waits longer than most.
		quiet: time.Second,
		known: []known{
			quitReturnsToTheMenu,
			theVitalsPrompt,
			theShopkeepersTell,
			{
				command: "list",
				match:   `^ +\d+\) `,
				why:     "the keeper's stock is listed in the opposite order, and the prices differ by one: this C is built 64-bit and the archived server was not (CLAUDE.md's shop-price oracle, -m32 -mfpmath=387); docs/deviations.md",
			},
			{
				command: "buy sword",
				match:   `^You now have a (small|long) sword\.$`,
				why:     "which sword `sword` matches follows the listing order above; docs/deviations.md",
			},
			{
				command: "inventory",
				match:   `^a (small|long) sword$`,
				why:     "as `buy sword` above; docs/deviations.md",
			},
		},
	},
	{
		name:  "banking-and-the-inn",
		about: "the ATM and the receptionist: balance, deposit, withdraw, offer",
		quiet: time.Second,
		known: []known{
			quitReturnsToTheMenu,
			theVitalsPrompt,
			theShopkeepersTell,
		},
	},
	{
		name:  "boards",
		about: "reading, writing and removing on a bulletin board, through the line editor",
		known: []known{
			quitReturnsToTheMenu,
			theVitalsPrompt,
			{
				match: `^\]$`,
				why:   "the improved editor prompts for each line with `]` in the C and silently here; docs/deviations.md",
			},
		},
	},
	{
		name:  "mail",
		about: "the postmaster: check, send, collect, and the block-allocated mail file",
		quiet: time.Second,
		known: []known{
			quitReturnsToTheMenu,
			theVitalsPrompt,
			theShopkeepersTell,
			{
				match: `^\]$`,
				why:   "as the boards scenario: the editor's per-line prompt; docs/deviations.md",
			},
			{
				command: "/s",
				match:   `^Message sent!$`,
				why:     "the C confirms a sent letter and this port does not; docs/deviations.md",
			},
			{
				command: "receive",
				match:   `.`,
				why:     "this C build cannot store mail at all — store_mail asserts its block size (mail.c:313) and a 64-bit build fails it; docs/deviations.md",
			},
			{
				command: "inventory",
				match:   `^( Nothing\.|a piece of mail)$`,
				why:     "as `receive` above; docs/deviations.md",
			},
			{
				command: "read letter",
				match:   `.`,
				why:     "as `receive` above; docs/deviations.md",
			},
			{
				command: "quit",
				match:   `^Saving \d+ items\.$`,
				why:     "as `receive` above: the letter this port delivered and the C did not; docs/deviations.md",
			},
		},
	},
	{
		name:  "housing",
		about: "hcontrol: building a house, showing the control file, destroying it",
		known: []known{
			quitReturnsToTheMenu,
			theVitalsPrompt,
		},
	},
	{
		name:  "mortal-refusals",
		about: "what a level 1 mortal is told they cannot do",
		known: []known{
			quitReturnsToTheMenu,
			theVitalsPrompt,
			{
				command: "score",
				match:   `^You have \d+\(\d+\) hit, \d+\(\d+\) mana and \d+\(\d+\) movement points\.$`,
				why:     "a mortal's hit points are rolled differently at creation; docs/deviations.md",
			},
		},
	},
}

// TestSessionParity plays every scenario at both servers and compares what
// they said.
//
// There is no expected output anywhere in this file. The C server is the
// expectation.
func TestSessionParity(t *testing.T) {
	requireCServer(t)

	for _, s := range scenarios {
		t.Run(s.name, func(t *testing.T) { s.run(t) })
	}
}

func (s scenario) run(t *testing.T) {
	t.Helper()

	script := s.script(t)
	servers := startPair(t)

	// One at a time, not both at once: two servers sharing a machine, each
	// framing its answers by silence, are quieter and steadier played one
	// after the other, and a scenario that creates a character would
	// otherwise be racing itself on two rosters.
	cText, err := parity.Run(script, parity.Options{Addr: servers.c.addr, Quiet: s.quiet, Deadline: 10 * time.Minute})
	if err != nil {
		t.Fatalf("playing %s against the C server: %v\n%s", s.name, err, servers.c.logText())
	}
	goText, err := parity.Run(script, parity.Options{Addr: servers.g.addr, Quiet: s.quiet, Deadline: 10 * time.Minute})
	if err != nil {
		t.Fatalf("playing %s against the Go server: %v\n%s", s.name, err, servers.g.logText())
	}

	// Compared with the colour stripped, always. It is the one systematic
	// difference between the two servers, it is on almost every line, and
	// the colour scenario below is where it is compared instead of set
	// aside.
	cNorm := parity.StripColour(parity.Normalise(cText))
	goNorm := parity.StripColour(parity.Normalise(goText))

	if s.colourGap {
		if len(parity.Compare(parity.Normalise(cText), parity.Normalise(goText))) == 0 {
			t.Errorf("this scenario says the port does not emit the colour the C does, and it now emits " +
				"the same colour the C does. Drop colourGap, and drop the entry in docs/deviations.md.")
		}
	}

	diffs := parity.Compare(cNorm, goNorm)
	explains := s.compile(t)
	used := make([]bool, len(s.known))
	where := ""

	for _, d := range diffs {
		var unexplained []parity.DiffLine
		for _, line := range d.Lines() {
			if i, ok := explains(d.Command, line.Text); ok {
				used[i] = true
				continue
			}
			unexplained = append(unexplained, line)
		}
		if len(unexplained) == 0 {
			t.Logf("%q: differs only in ways already triaged", d.Command)
			continue
		}
		if where == "" {
			// Written once, on the first real difference: the lines below
			// say what differed, and the whole transcript is what says
			// whether the difference started earlier.
			where = transcripts(t, s.name, cText, goText)
		}
		t.Errorf("%s: the two servers answered differently.\n    line %d: %q\n%s%s",
			s.about, d.N, d.Command, indent(parity.Render(unexplained)), where)
	}

	for i, k := range s.known {
		if !used[i] {
			t.Errorf("nothing in this scenario differs in the way %q is there to allow (%s). "+
				"Either it is fixed — delete the entry — or the script stopped exercising it.", k.match, k.why)
		}
	}
}

// compile turns the scenario's triage list into a lookup, once.
func (s scenario) compile(t *testing.T) func(command, line string) (int, bool) {
	t.Helper()

	patterns := make([]*regexp.Regexp, len(s.known))
	for i, k := range s.known {
		re, err := regexp.Compile(k.match)
		if err != nil {
			t.Fatalf("known difference %d has a bad pattern %q: %v", i, k.match, err)
		}
		patterns[i] = re
	}
	return func(command, line string) (int, bool) {
		for i, re := range patterns {
			if s.known[i].command != "" && s.known[i].command != command {
				continue
			}
			if re.MatchString(line) {
				return i, true
			}
		}
		return 0, false
	}
}

func indent(s string) string {
	var b strings.Builder
	for _, line := range strings.Split(strings.TrimSuffix(s, "\n"), "\n") {
		b.WriteString("      " + line + "\n")
	}
	return b.String()
}

func (s scenario) script(t *testing.T) parity.Script {
	t.Helper()

	path := filepath.Join(root, "testdata", "parity", s.name+".session")
	body, err := os.ReadFile(path) //nolint:gosec // a path this suite composed
	if err != nil {
		t.Fatalf("reading the script: %v", err)
	}
	script := parity.ParseScript(string(body))
	if len(script) == 0 {
		t.Fatalf("%s has nothing in it to type", path)
	}
	return script
}

// transcripts writes both sides out and returns where they went.
func transcripts(t *testing.T, name, cText, goText string) string {
	t.Helper()

	dir, err := os.MkdirTemp("", "parity-"+name+"-")
	if err != nil {
		return ""
	}
	for name, text := range map[string]string{"c.txt": cText, "go.txt": goText} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o600); err != nil {
			return ""
		}
	}
	return "    both transcripts in " + dir + "\n"
}

// scriptNames is what the release workflow and docs/developer.md check
// against: every .session file in testdata/parity is played by a scenario
// above, so a script added to the directory and forgotten about here is a
// test failure rather than a file nobody runs.
func TestEveryScriptIsPlayed(t *testing.T) {
	entries, err := os.ReadDir(filepath.Join(root, "testdata", "parity"))
	if err != nil {
		t.Fatal(err)
	}
	played := map[string]bool{}
	for _, s := range scenarios {
		played[s.name] = true
	}
	for _, e := range entries {
		name, ok := strings.CutSuffix(e.Name(), ".session")
		if !ok {
			continue
		}
		if !played[name] {
			t.Errorf("testdata/parity/%s.session is not in the scenarios table, so nothing plays it", name)
		}
	}
}
