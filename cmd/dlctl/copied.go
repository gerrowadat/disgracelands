// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// The `copied` pseudo-subsystem: the files `dlctl import` copies rather
// than converts, and the one place in this comparison where *bytes* are
// the right question.
//
// docs/design/yaml-only.md §4.1 argues at length for comparing loaded
// state everywhere else, and that argument holds — a byte comparison is a
// lossy proxy for "a server running on the converted data behaves
// identically", in both directions. It does not apply here, because
// nothing converts these files: `import` copies them, so either the bytes
// arrived or they did not, and there is no loader to disagree through.
//
// They were compared by nothing at all until #241. Two of the prose files
// were covered by accident — `text/greetings` and `text/credits` are
// licence obligations and LoadText refuses to start without them, loudly —
// and the other nine were not, and neither was the tuning. That last one
// is the one that mattered: `copyGameConfig`'s own comment says why it
// goes out of its way to carry `config/game.yaml` across ("a lib/ that has
// been tuned must not silently lose its tuning on the way through a format
// conversion"), and a directory that came out the other side without it is
// a server quietly back on config.c's defaults, with `import --verify`
// having reported the conversion clean.

// gameConfigPath is the tuning file copyGameConfig carries across,
// relative to a directory's base.
var gameConfigPath = filepath.Join("config", "game.yaml")

// helpScreenPath is HELP_PAGE_FILE, which copyHelpScreen carries across as
// part of the help import. It lives inside text/help/ and is not a help
// entry, so the help comparison — which loads the help database — does not
// see it either.
var helpScreenPath = filepath.Join("text", "help", helpScreenName)

// copiedPaths lists every copied file present under base, relative to it,
// sorted.
//
// The text/ half mirrors copyTextFiles exactly, including what it leaves
// out: regular files directly inside text/ and nothing below it, so
// text/help/ — a converted subsystem with its own --type — is not swept up
// by accident.
func copiedPaths(base string) ([]string, error) {
	var out []string

	entries, err := os.ReadDir(filepath.Join(base, "text"))
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("reading %s: %w", filepath.Join(base, "text"), err)
	}
	for _, e := range entries {
		if !e.Type().IsRegular() {
			continue
		}
		out = append(out, filepath.Join("text", e.Name()))
	}

	for _, rel := range []string{gameConfigPath, helpScreenPath} {
		switch _, err := os.Stat(filepath.Join(base, rel)); {
		case err == nil:
			out = append(out, rel)
		case os.IsNotExist(err):
			// A missing motd, screen or game.yaml is not an error on
			// either side: the server treats absent canned text as a
			// poorer game and still a game (internal/server/text.go), and
			// an absent game.yaml as config.c's own defaults. What is
			// worth reporting is one side having it and the other not,
			// which the union below is what catches.
		default:
			return nil, fmt.Errorf("reading %s: %w", filepath.Join(base, rel), err)
		}
	}

	sort.Strings(out)
	return out, nil
}

// compareCopiedFiles byte-compares every copied file on either side.
//
// The union rather than one side's list, because the interesting failure
// is asymmetric: a file that arrived and one that did not are the same
// difference, and a comparison that walked only the source would call a
// converted directory with a truncated motd identical if the source had
// none.
func compareCopiedFiles(left, right loadOptions) ([]string, error) {
	want, err := copiedPaths(left.base)
	if err != nil {
		return nil, err
	}
	got, err := copiedPaths(right.base)
	if err != nil {
		return nil, err
	}

	union := map[string]bool{}
	for _, rel := range append(want, got...) {
		union[rel] = true
	}
	rels := make([]string, 0, len(union))
	for rel := range union {
		rels = append(rels, rel)
	}
	sort.Strings(rels)

	var diffs []string
	for _, rel := range rels {
		a, aerr := readCopied(left.base, rel)
		b, berr := readCopied(right.base, rel)
		switch {
		case aerr != nil:
			return nil, aerr
		case berr != nil:
			return nil, berr
		case a == nil && b != nil:
			diffs = append(diffs, fmt.Sprintf("%s is in %s and not in %s", rel, right.base, left.base))
		case a != nil && b == nil:
			diffs = append(diffs, fmt.Sprintf("%s is in %s and not in %s", rel, left.base, right.base))
		case !bytes.Equal(a, b):
			diffs = append(diffs, fmt.Sprintf("%s differs: %d bytes in %s, %d in %s",
				rel, len(a), left.base, len(b), right.base))
		}
	}
	return diffs, nil
}

// readCopied reads one copied file, returning nil bytes and no error when
// it is not there.
func readCopied(base, rel string) ([]byte, error) {
	body, err := os.ReadFile(filepath.Join(base, rel)) //nolint:gosec // an operator-named directory
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", filepath.Join(base, rel), err)
	}
	// A file that exists and is empty is not the same as a missing one —
	// `truncate -s 0 config/game.yaml` is a real way to lose the tuning —
	// so it must not read back as nil here.
	if body == nil {
		body = []byte{}
	}
	return body, nil
}
