// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// checkedIn is examples/torture/binary from this package's own directory.
const checkedIn = "../../../examples/torture/binary"

// TestTheCheckedInCorpusMatchesThisGenerator regenerates the corpus and
// diffs it against what is committed, byte for byte and file for file.
//
// This is the same standard cmd/dlctl/import_test.go already holds
// examples/stock and examples/mini to, applied one level earlier: that
// test proves `yaml/` is `binary/` converted, and this one proves
// `binary/` is what the generator beside it says it is. Without it the
// hostile cases documented in world.go and the bytes actually committed
// could drift apart silently — which is exactly the failure mode a
// fixture built to be subtle invites.
//
// It also means the generator runs under `go test -race ./...` on every
// push, which is the reason it is written in Go at all (see this
// package's doc comment).
func TestTheCheckedInCorpusMatchesThisGenerator(t *testing.T) {
	fresh := t.TempDir()
	if err := generate(fresh); err != nil {
		t.Fatalf("generate: %v", err)
	}

	want := walk(t, checkedIn)
	got := walk(t, fresh)

	for _, rel := range sortedKeys(want) {
		g, ok := got[rel]
		if !ok {
			t.Errorf("%s is committed but the generator does not write it", rel)
			continue
		}
		if !bytes.Equal(g, want[rel]) {
			t.Errorf("%s differs: generated %d bytes, committed %d "+
				"(re-run `go run ./internal/fixtures/torture --out=examples/torture/binary`)",
				rel, len(g), len(want[rel]))
		}
	}
	for _, rel := range sortedKeys(got) {
		if _, ok := want[rel]; !ok {
			t.Errorf("the generator writes %s and it is not committed "+
				"(re-run `go run ./internal/fixtures/torture --out=examples/torture/binary`)", rel)
		}
	}
}

// TestTheReportDatesAreFixedRatherThanToday is a guard on the clock, not on
// the format.
//
// reportsclassic.Append substitutes time.Now() for a zero When
// (classic.go:114) and writes asctime's month-and-day slice into the file.
// The generator did leave it zero, so the corpus it produced was correct on
// the day it was committed and different the next morning: the test above
// failed on a tree nobody had touched, naming three files whose byte counts
// matched exactly. Checking for the fixed dates fails at once instead of at
// the next midnight, which is the difference between a bug found by reading
// it and one found by waiting.
func TestTheReportDatesAreFixedRatherThanToday(t *testing.T) {
	for file, want := range map[string]string{
		"bugs":  "(Aug  1)",
		"ideas": "(Sep 12)",
		"typos": "(Dec 25)",
	} {
		body, err := os.ReadFile(filepath.Join(checkedIn, "misc", file))
		if err != nil {
			t.Fatalf("reading the committed %s: %v", file, err)
		}
		if !bytes.Contains(body, []byte(want)) {
			t.Errorf("misc/%s does not carry the fixed date %s, so it was "+
				"written with the clock and will differ from a fresh "+
				"generation tomorrow:\n%s", file, want, body)
		}
	}
}

// TestTheHostileCasesAreActuallyInTheBytes asserts the three string shapes
// internal/persist/world/yaml/text.go's needsQuoting exists for are really
// in the committed world file.
//
// Checking the bytes rather than trusting the room text is the point.
// These three are invisible — a blank line before a tilde, a bare carriage
// return, a trailing space — and every one of them is the kind of thing an
// editor, a linter or a well-meaning reformat silently removes. If that
// happens the corpus goes on passing every other test in the tree while
// having quietly stopped testing the thing it was built for.
func TestTheHostileCasesAreActuallyInTheBytes(t *testing.T) {
	body, err := os.ReadFile(filepath.Join(checkedIn, "world", "wld", "50.wld"))
	if err != nil {
		t.Fatalf("reading the committed world file: %v", err)
	}

	for _, tc := range []struct {
		what string
		want []byte
	}{
		{"a blank line before a closing tilde (room #5002)", []byte("\n\n~\n")},
		{"a bare carriage return mid-description (room #5003)", []byte("after it:\r\n")},
		{"trailing spaces on an unterminated final line (room #5004)", []byte("instead.   ~\n")},
		{"a '#' at the start of a line inside a description (room #5005)", []byte("\n#5005 is prose")},
		{"a '*' at the start of a line inside a description (room #5007)", []byte("\n* this line begins")},
		{"a CP1252 byte (room #5001)", []byte{0x92}},
	} {
		if !bytes.Contains(body, tc.want) {
			t.Errorf("the committed world file no longer contains %s", tc.what)
		}
	}
}

// walk reads every regular file under dir, keyed by slash-separated path
// relative to it.
func walk(t *testing.T, dir string) map[string][]byte {
	t.Helper()
	out := map[string][]byte{}
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		body, err := os.ReadFile(path) //nolint:gosec // a fixture directory in this repository
		if err != nil {
			return err
		}
		out[filepath.ToSlash(rel)] = body
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", dir, err)
	}
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
