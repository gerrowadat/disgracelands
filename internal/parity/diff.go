// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package parity

import (
	"fmt"
	"strings"
)

// Block is one line the script typed and everything the server said in reply.
//
// The transcript is compared a block at a time rather than a line at a time,
// and that is not presentation. A line-by-line comparison of two whole
// transcripts is positional, so a *single* extra blank line early on — and
// there is one, in the login sequence — reports every line after it as
// differing too. The first run of this harness produced forty "differences"
// that were one difference and thirty-nine consequences of it, which is why
// its findings sat untriaged: nothing in the output distinguished them.
//
// Splitting at the typed command re-synchronises at every prompt. A command
// whose reply differs is one finding, and the next command starts clean.
type Block struct {
	// N is the position in the script, 1-based. Block 0 is the greeting,
	// which arrives before anything is typed.
	N int
	// Command is the line the script typed, or "" for the greeting.
	Command string
	// Text is what the server said in reply.
	Text string
}

// blockMarker is what Run writes into the transcript before each command. It
// has to be something no game output starts a line with.
const blockMarker = ">>>"

// Split cuts a transcript into blocks at the markers Run wrote.
func Split(transcript string) []Block {
	blocks := []Block{{N: 0}}
	var text strings.Builder

	flush := func() {
		blocks[len(blocks)-1].Text = text.String()
		text.Reset()
	}
	for _, line := range strings.Split(transcript, "\n") {
		if rest, ok := strings.CutPrefix(line, blockMarker); ok {
			flush()
			blocks = append(blocks, Block{N: len(blocks), Command: strings.TrimSpace(rest)})
			continue
		}
		text.WriteString(line)
		text.WriteString("\n")
	}
	flush()
	return blocks
}

// BlockDiff is one command the two servers answered differently.
type BlockDiff struct {
	// N and Command locate it in the script.
	N       int
	Command string
	// Want is what the C server said; Got is what the Go server said. The C
	// is the reference implementation: where the two differ, the Go server
	// is what is wrong.
	Want, Got string
}

// String renders one difference the way a person reads it: the command, then
// the lines that differ.
func (d BlockDiff) String() string {
	var b strings.Builder
	if d.Command == "" {
		b.WriteString("    on connecting (before anything was typed)\n")
	} else {
		fmt.Fprintf(&b, "    line %d: %q\n", d.N, d.Command)
	}
	b.WriteString(indent(LineDiff(d.Want, d.Got), "      "))
	return b.String()
}

// Compare plays the two transcripts against each other block by block.
//
// The two are the same script typed at two servers, so the blocks line up by
// construction; a transcript with fewer of them is a server that stopped
// answering, which is reported as the block it stopped at rather than as
// every block after it.
func Compare(want, got string) []BlockDiff {
	wantBlocks, gotBlocks := Split(want), Split(got)

	var diffs []BlockDiff
	for i := 0; i < len(wantBlocks) || i < len(gotBlocks); i++ {
		w, g := blockAt(wantBlocks, i), blockAt(gotBlocks, i)
		w.Text, g.Text = withoutBlankLines(w.Text), withoutBlankLines(g.Text)
		if w.Text == g.Text && w.Command == g.Command {
			continue
		}
		command := w.Command
		if command == "" {
			command = g.Command
		}
		diffs = append(diffs, BlockDiff{N: i, Command: command, Want: w.Text, Got: g.Text})
	}
	return diffs
}

// withoutBlankLines drops every blank line from a block's output.
//
// This is a decision to stop comparing something, so here is the something.
// The C prepends a CRLF to any output that interrupts a prompt —
// process_output sends its buffer from i rather than i+2 when has_prompt is
// set (comm.c:1459), and a new descriptor is born with has_prompt = 1,
// "prompt is part of greetings" (comm.c:1404). Every separate write the C
// makes while answering one command therefore arrives with a blank line in
// front of it, and how many separate writes a command makes is an
// implementation detail of each server rather than anything a player is
// reading. This port has no equivalent and adds newlines of its own in
// other places.
//
// Left in, that is the loudest thing in every transcript: three blank lines
// on `quit` alone, on every script, on top of every real finding. Taken out,
// what is left is words, which is what the suite is for.
//
// The cost, stated rather than discovered later: a difference that is *only*
// whitespace — a missing blank line between two paragraphs of a room
// description — is not caught here. Nothing else in the harness compares
// whitespace either, since Normalise already trims the end of every line.
// The underlying difference is real and is written down in
// docs/deviations.md rather than being fixed here; fixing it means porting
// the C's has_prompt semantics, which is a change to every line the server
// writes and not to this harness.
func withoutBlankLines(text string) string {
	var b strings.Builder
	for _, line := range splitLines(text) {
		if strings.TrimSpace(line) == "" {
			continue
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	return b.String()
}

func blockAt(blocks []Block, i int) Block {
	if i < len(blocks) {
		return blocks[i]
	}
	return Block{N: i, Text: "<the server said nothing more>\n"}
}

// Diff reports every block the two servers disagreed about.
func Diff(want, got string) string {
	var b strings.Builder
	for _, d := range Compare(want, got) {
		b.WriteString(d.String())
	}
	return b.String()
}

// DiffLine is one line that appears on one side and not the other.
type DiffLine struct {
	// FromC is true for a line the C server said and the Go server did not.
	FromC bool
	// Text is the line, without its newline.
	Text string
}

// Lines are the lines of this block that differ.
//
// Exposed as data rather than only as a string because triage is done on
// *what* differs rather than on which command differed: one difference in a
// character's hit points shows up in the prompt after every command in a
// script, and a caller that can see the lines can say "that one line, this is
// why" once instead of naming twenty commands.
func (d BlockDiff) Lines() []DiffLine { return lcsWalk(splitLines(d.Want), splitLines(d.Got)) }

// LineDiff is a longest-common-subsequence diff of two pieces of transcript,
// in the -/+ shape everybody already reads.
//
// A real LCS rather than a positional walk, for the same reason Block exists:
// an inserted blank line is one difference, and saying so is the difference
// between a finding and forty of them. The inputs are one command's output,
// so the quadratic table is small.
func LineDiff(want, got string) string {
	return Render(lcsWalk(splitLines(want), splitLines(got)))
}

// Render prints differing lines the way a person reads them.
func Render(lines []DiffLine) string {
	var b strings.Builder
	for i, line := range lines {
		if i >= maxDiffLines {
			fmt.Fprintf(&b, "    ... and %d more differing line(s)\n", len(lines)-maxDiffLines)
			break
		}
		side := "Go:"
		if line.FromC {
			side = " C:"
		}
		fmt.Fprintf(&b, "%s %s\n", side, quote(line.Text))
	}
	return b.String()
}

// lcsWalk is the diff itself: the lines of w and g that are not in the
// longest common subsequence of the two.
func lcsWalk(w, g []string) []DiffLine {
	// lcs[i][j] is the length of the longest common subsequence of w[i:] and
	// g[j:].
	lcs := make([][]int, len(w)+1)
	for i := range lcs {
		lcs[i] = make([]int, len(g)+1)
	}
	for i := len(w) - 1; i >= 0; i-- {
		for j := len(g) - 1; j >= 0; j-- {
			if w[i] == g[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}

	var out []DiffLine
	i, j := 0, 0
	for i < len(w) || j < len(g) {
		switch {
		case i < len(w) && j < len(g) && w[i] == g[j]:
			i, j = i+1, j+1
		case j < len(g) && (i == len(w) || lcs[i][j+1] >= lcs[i+1][j]):
			out = append(out, DiffLine{Text: g[j]})
			j++
		default:
			out = append(out, DiffLine{FromC: true, Text: w[i]})
			i++
		}
	}
	return out
}

// maxDiffLines bounds one block's report. A whole command's output differing
// is a real thing — a listing in a different order, a room a mobile is
// standing in — and printing all of it buries the next finding.
const maxDiffLines = 30

// quote renders a line so trailing whitespace and control characters are
// visible, because half the differences this harness finds are exactly that.
func quote(s string) string { return fmt.Sprintf("%q", s) }

// splitLines splits into lines without the empty trailing element a final
// newline produces, which would otherwise show up as a difference between a
// transcript that ends in a newline and one that does not.
func splitLines(s string) []string {
	s = strings.TrimSuffix(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

func indent(s, with string) string {
	var b strings.Builder
	for _, line := range splitLines(s) {
		b.WriteString(with)
		b.WriteString(line)
		b.WriteString("\n")
	}
	return b.String()
}
