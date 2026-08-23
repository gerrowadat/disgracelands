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

	"github.com/gerrowadat/disgracelands/internal/persist/dataversion"
)

// stockBinaryDir is examples/stock/binary from cmd/dlctl's own package
// directory — the real, checked-in fixture `lib import`'s own README
// (examples/stock/README.md) documents producing examples/stock/yaml
// from, used here rather than a synthetic one so this test exercises the
// same seven subsystems and the same real archive data every other
// importer's own tests do individually.
const stockBinaryDir = "../../examples/stock/binary"

func TestLibImportRequiresBothDirs(t *testing.T) {
	if err := run([]string{"lib", "import"}); err == nil {
		t.Error("run([lib import]) with neither flag succeeded, want an error")
	}
	if err := run([]string{"lib", "import", "--from-dir", stockBinaryDir}); err == nil {
		t.Error("run([lib import --from-dir]) with no --to-dir succeeded, want an error")
	}
	if err := run([]string{"lib", "import", "--to-dir", t.TempDir()}); err == nil {
		t.Error("run([lib import --to-dir]) with no --from-dir succeeded, want an error")
	}
}

func TestLibImportConvertsStockEndToEnd(t *testing.T) {
	to := t.TempDir()
	if err := run([]string{"lib", "import", "--from-dir", stockBinaryDir, "--to-dir", to}); err != nil {
		t.Fatalf("run([lib import]): %v", err)
	}

	// One file or directory per subsystem `lib import` wraps, matching
	// what examples/stock/yaml itself already holds.
	for _, want := range []string{
		"world/zones.yaml",
		"state/clock.yaml",
		"state/houses.yaml",
		"config/names.yaml",
		"config/messages.yaml",
		"config/socials.yaml",
		"text/help/help.yaml",
		"text/credits",
		"text/motd",
	} {
		if _, err := os.Stat(filepath.Join(to, want)); err != nil {
			t.Errorf("expected %s to exist: %v", want, err)
		}
	}

	// players/ is not produced at all: examples/stock/binary's own roster
	// is empty (a fresh stock install has none), and pfile import does
	// not create an empty directory for zero characters — asserting this
	// the same way examples/stock/README.md's own prose does, rather
	// than assuming it.
	if _, err := os.Stat(filepath.Join(to, "players")); !os.IsNotExist(err) {
		t.Errorf("players/ exists with an empty roster to import: %v", err)
	}

	stamp, err := os.ReadFile(filepath.Join(to, dataversion.FileName))
	if err != nil {
		t.Fatalf("reading %s: %v", dataversion.FileName, err)
	}
	if got := string(stamp); !strings.Contains(got, dataversion.Current.String()) {
		t.Errorf("%s = %q, want it to contain %q", dataversion.FileName, got, dataversion.Current.String())
	}
}

func TestLibImportMatchesTheCheckedInExample(t *testing.T) {
	// examples/stock/yaml is examples/stock/binary converted exactly this
	// way (its own README says so) — regenerating it and diffing against
	// what is checked in is what proves that claim rather than assuming
	// it, and catches the fixture and the command drifting apart from
	// each other silently.
	to := t.TempDir()
	if err := run([]string{"lib", "import", "--from-dir", stockBinaryDir, "--to-dir", to}); err != nil {
		t.Fatalf("run([lib import]): %v", err)
	}

	const checkedIn = "../../examples/stock/yaml"
	var mismatches []string
	err := filepath.WalkDir(checkedIn, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(checkedIn, path)
		if err != nil {
			return err
		}
		if rel == dataversion.FileName {
			return nil // not produced by a fresh checkout of the fixture itself
		}
		want, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		got, err := os.ReadFile(filepath.Join(to, rel))
		if err != nil {
			mismatches = append(mismatches, rel+": "+err.Error())
			return nil
		}
		if string(got) != string(want) {
			mismatches = append(mismatches, rel+": content differs")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", checkedIn, err)
	}
	for _, m := range mismatches {
		t.Error(m)
	}
}
