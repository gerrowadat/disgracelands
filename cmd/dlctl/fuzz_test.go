// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/gerrowadat/disgracelands/internal/persist/convert"
	"github.com/gerrowadat/disgracelands/internal/persist/player"
	"github.com/gerrowadat/disgracelands/internal/persist/player/binary"
)

// The two cross-format fuzz targets docs/design/yaml-only.md §5.3 asks
// for, beside internal/persist/world/yaml's two text ones.
//
// The property is the same in both, and it is the one this release rests
// on: **anything the legacy reader accepts must convert to yaml and load
// back to the same state.** Not "must convert without erroring" — a
// conversion that succeeds and quietly drops a field is the failure mode
// this whole exercise exists to find, and every one found so far has been
// exactly that shape.
//
// Where the text targets fuzz a string, these fuzz a *file*, which is
// slower (a temporary directory and a real parse per execution) and
// reaches somewhere else entirely: parser divergences rather than encoder
// ones. They are seeded from the checked-in corpora, so the fuzzer starts
// from files that already parse and mutates outward from there, which is
// where the interesting inputs are — a random byte string is rejected by
// the first record header and tests nothing.

// maxFuzzInput bounds the file-shaped targets below. See the skip in
// FuzzClassicRecordRoundTrip for why.
const maxFuzzInput = 16 << 10

// FuzzClassicRecordRoundTrip pushes arbitrary bytes through the classic
// world reader, and, for anything it accepts, through the yaml writer and
// back.
func FuzzClassicRecordRoundTrip(f *testing.F) {
	for _, fx := range corpora {
		seedFromDir(f, filepath.Join(fx.binaryDir, "world", "wld"), ".wld")
	}
	f.Add("#1\nA Room~\nA description.\n~\n0 0 0\nS\n$\n")

	f.Fuzz(func(t *testing.T, body string) {
		// A cap on input size, because without one the fuzzer eventually
		// finds a way to declare tens of thousands of rooms and spends
		// minutes converting them — measured as a collapse from thousands
		// of executions per second to zero. Nothing about a hundredth
		// room exercises a parser path the second one did not, and an
		// engine that is not executing is not fuzzing.
		if len(body) > maxFuzzInput {
			t.Skip()
		}
		dir := t.TempDir()
		wld := filepath.Join(dir, "world", "wld")
		if err := os.MkdirAll(wld, 0o750); err != nil {
			t.Fatal(err)
		}
		for _, sub := range []string{"mob", "obj", "shp"} {
			if err := os.MkdirAll(filepath.Join(dir, "world", sub), 0o750); err != nil {
				t.Fatal(err)
			}
			// An index listing no files is a valid, empty subsystem. The
			// world under test is the one room file below.
			if err := os.WriteFile(filepath.Join(dir, "world", sub, "index"), []byte("$\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		// One zone spanning every vnum a room can have. Without it the
		// conversion has nowhere to put a room — the yaml format is one
		// file per zone — and every input would "fail" for that structural
		// reason rather than for anything about the bytes being fuzzed.
		// (importWorld refuses that case outright now, which is how this
		// harness's first version was found to be testing the wrong thing.)
		if err := os.MkdirAll(filepath.Join(dir, "world", "zon"), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "world", "zon", "index"), []byte("0.zon\n$\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		zone := "#0\nEverything~\n0 65535 30 2\nS\n$\n"
		if err := os.WriteFile(filepath.Join(dir, "world", "zon", "0.zon"), []byte(zone), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(wld, "index"), []byte("0.wld\n$\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(wld, "0.wld"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}

		// A file the classic reader refuses is not interesting: this is
		// about what happens *after* it says yes.
		enc := convert.Encodings[convert.DefaultEncoding]
		left := loadOptions{base: dir, format: "classic", enc: enc}
		if _, err := loadSubsystem(typeWorld, left); err != nil {
			t.Skip()
		}

		to := t.TempDir()
		if err := runImport(typeWorld, importOptions{fromDir: dir, toDir: to, encName: convert.DefaultEncoding}); err != nil {
			t.Fatalf("the classic reader accepted this world and the importer would not convert it: %v", err)
		}
		diffs, err := compareSubsystem(typeWorld, left, loadOptions{base: to, format: "yaml", enc: enc})
		if err != nil {
			t.Fatalf("reloading the converted world: %v", err)
		}
		if len(diffs) > 0 {
			t.Fatalf("converting this world lost something:\n%s\n--- source ---\n%s", summarise(diffs), body)
		}
	})
}

// FuzzBinaryRecordRoundTrip is the same property for char_file_u-shaped
// bytes: whatever the binary roster reader accepts must survive the trip
// through yaml.
//
// The input is padded or truncated to exactly one record before being
// written, rather than used as the file verbatim. That is what makes the
// target useful rather than merely present: the roster file's length has
// to be a whole number of records, so a fuzzer left to choose lengths
// spends essentially every execution being rejected by the size check —
// and the few that are not are megabyte files of thousands of records,
// each of which becomes a yaml file on disk. Measured, not assumed: the
// unbounded version managed 84 executions in ninety seconds and found
// nothing, because it was fuzzing the length rather than the record.
//
// The size check itself is not left untested by that decision; it has its
// own unit tests in the binary package, where it belongs.
func FuzzBinaryRecordRoundTrip(f *testing.F) {
	seed, err := os.ReadFile("../../examples/torture/binary/etc/players")
	if err != nil {
		f.Fatalf("reading the seed roster: %v", err)
	}
	size := recordSize(f)
	for off := 0; off+size <= len(seed); off += size {
		f.Add(seed[off : off+size])
	}

	f.Fuzz(func(t *testing.T, body []byte) {
		record := make([]byte, size)
		copy(record, body)

		dir := t.TempDir()
		etc := filepath.Join(dir, "etc")
		if err := os.MkdirAll(etc, 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(etc, "players"), record, 0o600); err != nil {
			t.Fatal(err)
		}

		enc := convert.Encodings[convert.DefaultEncoding]
		left := loadOptions{base: dir, format: "binary", enc: enc}
		state, err := loadSubsystem(typePfile, left)
		if err != nil {
			t.Skip()
		}
		if hasInvalidUTF8(state.(pfileState)) {
			// A YAML document is UTF-8 by definition, so a string that is
			// not cannot go in one — yamlenc.MarshalString refuses rather
			// than letting the encoder substitute U+FFFD, and `import`
			// fails with the --encoding advice. That refusal is the
			// designed behaviour for bytes a legacy file holds and a YAML
			// file cannot, not a conversion bug, so these inputs are out
			// of scope for a round-trip property.
			t.Skip()
		}
		if hasBareLF(state.(pfileState)) {
			// The one transform that is genuinely unavoidable and
			// genuinely settled: yaml stores LF and re-derives CRLF on
			// load, so a string holding a bare LF comes back with a CR in
			// front of it. docs/design/yaml-only.md §4.2 argues why
			// that is the same relationship classic's own bytes already
			// have to their own in-memory form rather than a loss, and
			// internal/persist/player/yaml has a test pinning the
			// behaviour so it cannot change without somebody noticing.
			//
			// Skipped here rather than tolerated in the comparison,
			// because "differences that are only \n versus \r\n are
			// fine" would also excuse a real one that happened to look
			// like that.
			t.Skip()
		}

		to := t.TempDir()
		if err := runImport(typePfile, importOptions{fromDir: dir, toDir: to, encName: convert.DefaultEncoding}); err != nil {
			t.Fatalf("the binary reader accepted this roster and the importer would not convert it: %v", err)
		}
		diffs, err := compareSubsystem(typePfile, left, loadOptions{base: to, format: "yaml", enc: enc})
		if err != nil {
			t.Fatalf("reloading the converted roster: %v", err)
		}
		if len(diffs) > 0 {
			t.Fatalf("converting this roster lost something:\n%s", summarise(diffs))
		}
	})
}

// recordSize is one char_file_u in the ILP32 layout the archive is in —
// asked of the store rather than hard-coded, because the whole point of
// internal/persist/player/binary/layout.go is that no offset or width in
// this format is ever written down by hand.
func recordSize(f *testing.F) int {
	f.Helper()
	store, err := binary.New(player.Config{Dir: f.TempDir(), ReadOnly: true})
	if err != nil {
		f.Fatalf("opening a store to ask its record size: %v", err)
	}
	defer func() { _ = store.Close() }()
	return store.RecordSize()
}

// hasInvalidUTF8 reports whether any string in a loaded roster holds
// bytes a YAML document cannot carry.
func hasInvalidUTF8(state pfileState) bool {
	for _, rec := range state.Characters {
		for _, s := range []string{
			rec.Name, rec.Title, rec.Description, rec.Host,
			rec.PoofIn, rec.PoofOut, rec.Credential.Hash,
		} {
			if !utf8.ValidString(s) {
				return true
			}
		}
		for _, a := range rec.Aliases {
			if !utf8.ValidString(a.Name) || !utf8.ValidString(a.Replacement) {
				return true
			}
		}
	}
	return false
}

// hasBareLF reports whether any free-text field in a loaded roster holds
// a line feed with no carriage return before it.
func hasBareLF(state pfileState) bool {
	bare := func(s string) bool {
		return strings.Contains(strings.ReplaceAll(s, "\r\n", ""), "\n")
	}
	for _, rec := range state.Characters {
		if bare(rec.Description) || bare(rec.Title) || bare(rec.Host) ||
			bare(rec.PoofIn) || bare(rec.PoofOut) {
			return true
		}
		for _, a := range rec.Aliases {
			if bare(a.Name) || bare(a.Replacement) {
				return true
			}
		}
	}
	return false
}

// seedFromDir adds every file with the given suffix under dir as a seed.
func seedFromDir(f *testing.F, dir, suffix string) {
	f.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != suffix {
			continue
		}
		body, err := os.ReadFile(filepath.Join(dir, e.Name())) //nolint:gosec // a fixture in this repository
		if err != nil {
			continue
		}
		f.Add(string(body))
	}
}
