// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import (
	"bufio"
	"fmt"
	"io"
	"sort"
	"strings"
)

// The help system, ported from load_help/index_boot (db.c) and do_help's
// lookup (act.informative.c:953-991).
//
// No I/O lives here — internal/server reads the files and hands this
// package readers, the same split game.ParseSocials already has, and the
// reason help.go does not import internal/persist.

// HelpEntry is one topic: every keyword that reaches it, and the text
// shown. A keyword line with several space-separated words
// ("CIRCLE CIRCLEMUD CREDITS") is one HelpEntry with several Keywords, not
// several entries — load_help gives them all the same entry text
// (db.c:1717-1724).
type HelpEntry struct {
	Keywords []string
	// Body is the displayed text, CRLF-joined. Its first line is the
	// keyword line itself: load_help builds the entry by copying the
	// keyword line in before appending anything else
	// (`strcpy(entry, strcat(key, "\r\n"))`, db.c:1710) — read this
	// wrong and every entry is missing its own heading.
	Body string
}

// ParseHelpFile parses one classic .hlp file, porting load_help
// (db.c:1701-1734). A keyword line is terminated by a following body, the
// body by a line starting with `#`, and the whole file by a keyword line
// starting with `$` — checked by first character only, matching the C's
// own `while (*key != '$')`/`while (*line != '#')`, not a full-string
// comparison.
func ParseHelpFile(r io.Reader) ([]HelpEntry, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20) // entries run long; keep headroom over the default 64KiB line limit

	var entries []HelpEntry
	for {
		key, ok, err := helpLine(sc)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("help: unterminated file (no $)")
		}
		if strings.HasPrefix(key, "$") {
			return entries, nil
		}

		var body strings.Builder
		body.WriteString(key)
		body.WriteString("\r\n")
		for {
			line, ok, err := helpLine(sc)
			if err != nil {
				return nil, err
			}
			if !ok {
				return nil, fmt.Errorf("help: unterminated entry (no #): %q", key)
			}
			if strings.HasPrefix(line, "#") {
				break
			}
			body.WriteString(line)
			body.WriteString("\r\n")
		}

		entries = append(entries, HelpEntry{Keywords: strings.Fields(key), Body: body.String()})
	}
}

// ParseHelpIndex parses text/help/index, porting index_boot's read loop
// (db.c:785-816): whitespace-delimited filenames, terminated by a lone
// `$` token.
func ParseHelpIndex(r io.Reader) ([]string, error) {
	sc := bufio.NewScanner(r)
	sc.Split(bufio.ScanWords)

	var files []string
	for sc.Scan() {
		tok := sc.Text()
		if tok == "$" {
			return files, nil
		}
		files = append(files, tok)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("help: index has no terminating $")
}

// helpLine reads one line, stripping a trailing \r so a CRLF source file
// works the same as an LF one — get_one_line (db.c:1670) only ever saw LF.
func helpLine(sc *bufio.Scanner) (line string, ok bool, err error) {
	if !sc.Scan() {
		return "", false, sc.Err()
	}
	return strings.TrimRight(sc.Text(), "\r"), true, nil
}

// HelpSlug names a help entry for the native format's one-file-per-entry
// layout (docs/proposals/data-format.md §7): every keyword joined by a
// space, lowercased, with every run of characters outside [a-z0-9]
// collapsed to one `-` and trimmed from both ends.
//
// Slugging the *whole* keyword line rather than just the first keyword —
// the doc's own illustrative example uses only the first ("ac.txt" for
// `[ac, "armor class"]") — is deliberate, not cosmetic: the real 216-entry
// archive's own quoted multi-word spell names are split into several
// keyword tokens apiece by strings.Fields (the same naive tokenisation
// the C's own keyword-line parsing does, see ParseHelpFile), so "CURE
// LIGHT WOUNDS" and "CURE BLIND" and "CURE CRITIC" all share the token
// `"CURE` — slugging only that shared first token collides three ways.
// Joining the whole line first keeps every entry's slug distinct: checked
// against the real archive, not assumed, and it holds for all 216 with no
// collision, one entry's whole line (`! ^`, both keywords pure
// punctuation) slugging to empty — the one case a caller needs its own
// fallback for, since a blank filename is not a filename.
func HelpSlug(keywords []string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(strings.Join(keywords, " ")) {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case !lastDash:
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

// helpRow is one keyword's row in the sorted table hsort (db.c:1739-1747)
// builds — one row per keyword, not per entry, since do_help's binary
// search walks keywords.
type helpRow struct {
	keyword string
	entry   HelpEntry
}

// HelpIndex is the loaded, sorted help table.
type HelpIndex struct {
	rows []helpRow
}

// NewHelpIndex flattens a set of parsed entries into one row per keyword
// and sorts them case-insensitively, porting hsort's `str_cmp` ordering.
func NewHelpIndex(entries []HelpEntry) *HelpIndex {
	var rows []helpRow
	for _, e := range entries {
		for _, k := range e.Keywords {
			rows = append(rows, helpRow{keyword: k, entry: e})
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		return strings.ToLower(rows[i].keyword) < strings.ToLower(rows[j].keyword)
	})
	return &HelpIndex{rows: rows}
}

// Lookup finds the entry do_help would show for a query, porting its
// binary search plus backward-walk (act.informative.c:966-988):
// `strn_cmp(argument, keyword, strlen(argument))` is a case-insensitive
// *prefix* match — the query's own length against each keyword — and a
// short, ambiguous query resolves to whichever matching keyword sorts
// first, which need not be the topic the player meant. That is the C's
// own behaviour and is reproduced deliberately, not fixed.
//
// Implemented as a sort.Search boundary rather than a literal binary
// search: any two correct binary searches over the same sorted data
// converge on the same result regardless of probe order, so finding "the
// first row whose full keyword is lexicographically >= query" and then
// checking whether *that* row's keyword has query as a prefix lands on
// exactly the row the C's walk-back would — the block of rows sharing a
// prefix is contiguous in sorted order, and this finds its start
// directly instead of probing into the middle and walking back out.
func (h *HelpIndex) Lookup(query string) (HelpEntry, bool) {
	if query == "" || len(h.rows) == 0 {
		return HelpEntry{}, false
	}
	lowerQuery := strings.ToLower(query)
	i := sort.Search(len(h.rows), func(i int) bool {
		return strings.ToLower(h.rows[i].keyword) >= lowerQuery
	})
	if i >= len(h.rows) || !strings.HasPrefix(strings.ToLower(h.rows[i].keyword), lowerQuery) {
		return HelpEntry{}, false
	}
	return h.rows[i].entry, true
}
