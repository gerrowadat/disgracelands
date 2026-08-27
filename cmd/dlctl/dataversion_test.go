// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gerrowadat/disgracelands/internal/persist/dataversion"
)

// Every test here runs in a binary linked without the Makefile's -ldflags,
// so dataversion.Current() has no release version to report (the package's
// own TestCurrentHasNoVersionUnderGoTest pins that). That is the case
// being asserted, not a limitation being worked around: an unreleased
// dlctl stamps nothing and enforces nothing, and these are the four places
// an operator meets that.

func TestDataVersionBareReportsThisBuild(t *testing.T) {
	if err := run([]string{"data", "version"}); err != nil {
		t.Fatalf("run([data version]): %v", err)
	}
}

func TestDataVersionWriteRefusesFromAnUnreleasedBuild(t *testing.T) {
	dir := t.TempDir()
	if err := run([]string{"data", "version", "--dir", dir, "--write"}); err == nil {
		t.Fatal("run([data version --write]) from an unreleased build succeeded, want a refusal")
	}
	if _, err := os.Stat(filepath.Join(dir, dataversion.FileName)); !os.IsNotExist(err) {
		t.Errorf("a stamp was written anyway: %v", err)
	}
}

// An unreleased build has nothing to compare against, so even a stamp it
// could never satisfy is reported rather than refused — `data version`
// exits 0 and says so. A released build is where the refusal lives, and
// dataversion's own tests cover both directions of it.
func TestDataVersionReportsAMajorMismatchWithoutRefusing(t *testing.T) {
	dir := t.TempDir()
	if err := dataversion.Write(dir, dataversion.Version{Major: 99}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := run([]string{"data", "version", "--dir", dir}); err != nil {
		t.Errorf("run([data version --dir <major 99>]) from an unreleased build: %v", err)
	}
}

func TestDataVersionOnAnUnstampedDirectoryIsFine(t *testing.T) {
	if err := run([]string{"data", "version", "--dir", t.TempDir()}); err != nil {
		t.Errorf("run([data version --dir <unstamped>]): %v", err)
	}
}

func TestDataVersionWriteWithoutDirIsRejected(t *testing.T) {
	if err := run([]string{"data", "version", "--write"}); err == nil {
		t.Error("run([data version --write]) with no --dir succeeded, want an error")
	}
}
