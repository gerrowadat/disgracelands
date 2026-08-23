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

func TestDataVersionBareReportsWhatThisBuildUnderstands(t *testing.T) {
	if err := run([]string{"data", "version"}); err != nil {
		t.Fatalf("run([data version]): %v", err)
	}
}

func TestDataVersionWriteThenCheckRoundTrips(t *testing.T) {
	dir := t.TempDir()
	if err := run([]string{"data", "version", "--dir", dir, "--write"}); err != nil {
		t.Fatalf("run([data version --dir %s --write]): %v", dir, err)
	}
	if _, err := os.Stat(filepath.Join(dir, dataversion.FileName)); err != nil {
		t.Fatalf("stamp not written: %v", err)
	}
	if err := run([]string{"data", "version", "--dir", dir}); err != nil {
		t.Errorf("run([data version --dir %s]) after --write: %v", dir, err)
	}
}

func TestDataVersionRefusesOnNewerMajor(t *testing.T) {
	dir := t.TempDir()
	newer := dataversion.Version{Major: dataversion.Current.Major + 1}
	if err := dataversion.Write(dir, newer); err != nil {
		t.Fatalf("Write: %v", err)
	}
	err := run([]string{"data", "version", "--dir", dir})
	if err == nil {
		t.Fatal("run([data version --dir <newer major>]) succeeded, want a refusal")
	}
}

func TestDataVersionWriteWithoutDirIsRejected(t *testing.T) {
	if err := run([]string{"data", "version", "--write"}); err == nil {
		t.Error("run([data version --write]) with no --dir succeeded, want an error")
	}
}
