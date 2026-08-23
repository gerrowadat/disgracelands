// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package dataversion

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseRoundTripsString(t *testing.T) {
	v := Version{Major: 1, Minor: 2, Patch: 3}
	got, err := Parse(v.String())
	if err != nil {
		t.Fatalf("Parse(%q): %v", v.String(), err)
	}
	if got != v {
		t.Errorf("Parse(%q) = %+v, want %+v", v.String(), got, v)
	}
}

func TestParseRejectsMalformedInput(t *testing.T) {
	for _, s := range []string{"", "1", "1.2", "1.2.3.4", "a.b.c", "1.2.-3", "1..3"} {
		if _, err := Parse(s); err == nil {
			t.Errorf("Parse(%q) succeeded, want an error", s)
		}
	}
}

func TestCheckNoStampFileIsSilentlyFine(t *testing.T) {
	warning, err := Check(t.TempDir(), Current)
	if err != nil || warning != "" {
		t.Errorf("Check(no stamp) = (%q, %v), want (\"\", nil)", warning, err)
	}
}

func TestCheckSameVersionIsSilentlyFine(t *testing.T) {
	dir := t.TempDir()
	if err := Write(dir, Current); err != nil {
		t.Fatalf("Write: %v", err)
	}
	warning, err := Check(dir, Current)
	if err != nil || warning != "" {
		t.Errorf("Check(same version) = (%q, %v), want (\"\", nil)", warning, err)
	}
}

func TestCheckOlderPatchIsSilentlyFine(t *testing.T) {
	dir := t.TempDir()
	// The data was written by a server one patch behind current — same
	// major.minor, so nothing a patch bump changed is anything a reader
	// needs to know about (docs/design/data-format-versioning.md).
	if err := Write(dir, Version{Major: Current.Major, Minor: Current.Minor, Patch: 0}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	newer := Version{Major: Current.Major, Minor: Current.Minor, Patch: Current.Patch + 5}
	warning, err := Check(dir, newer)
	if err != nil || warning != "" {
		t.Errorf("Check(older patch) = (%q, %v), want (\"\", nil)", warning, err)
	}
}

func TestCheckNewerMinorWarnsButDoesNotRefuse(t *testing.T) {
	dir := t.TempDir()
	newer := Version{Major: Current.Major, Minor: Current.Minor + 1, Patch: 0}
	if err := Write(dir, newer); err != nil {
		t.Fatalf("Write: %v", err)
	}
	warning, err := Check(dir, Current)
	if err != nil {
		t.Fatalf("Check(newer minor) returned an error, want a warning and nil: %v", err)
	}
	if warning == "" {
		t.Error("Check(newer minor) returned no warning, want one describing the mismatch")
	}
	if !strings.Contains(warning, newer.String()) || !strings.Contains(warning, Current.String()) {
		t.Errorf("Check(newer minor) warning %q does not name both versions", warning)
	}
}

func TestCheckNewerMajorRefusesToBoot(t *testing.T) {
	dir := t.TempDir()
	newer := Version{Major: Current.Major + 1, Minor: 0, Patch: 0}
	if err := Write(dir, newer); err != nil {
		t.Fatalf("Write: %v", err)
	}
	warning, err := Check(dir, Current)
	if err == nil {
		t.Fatal("Check(newer major) returned no error, want a refusal to boot")
	}
	if warning != "" {
		t.Errorf("Check(newer major) also returned a warning %q, want none — it should refuse instead", warning)
	}
	if !strings.Contains(err.Error(), newer.String()) {
		t.Errorf("Check(newer major) error %q does not name the data's version", err.Error())
	}
}

func TestCheckOlderMajorIsSilentlyFine(t *testing.T) {
	// A server newer than the data it is reading is the ordinary case —
	// every data/ directory starts at the version the server that last
	// wrote it understood, older than whatever ships tomorrow.
	dir := t.TempDir()
	if err := Write(dir, Version{Major: 1, Minor: 0, Patch: 0}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	newer := Version{Major: 2, Minor: 0, Patch: 0}
	warning, err := Check(dir, newer)
	if err != nil || warning != "" {
		t.Errorf("Check(older major) = (%q, %v), want (\"\", nil)", warning, err)
	}
}

func TestCheckMalformedVersionIsAnError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte("schema: dl/dataversion@1\nformat: yaml\nversion: garbage\n"), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	if _, err := Check(dir, Current); err == nil {
		t.Error("Check(malformed version) succeeded, want an error")
	}
}

func TestCheckUnknownFieldIsAnError(t *testing.T) {
	// Strict decoding, the same posture data-format.md §10.2 takes for
	// every other yaml document in this tree.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte("schema: dl/dataversion@1\nformat: yaml\nversion: 1.0.0\nbogus: true\n"), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	if _, err := Check(dir, Current); err == nil {
		t.Error("Check(unknown field) succeeded, want an error")
	}
}

func TestWriteIsAtomic(t *testing.T) {
	dir := t.TempDir()
	if err := Write(dir, Current); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, FileName+".tmp")); !os.IsNotExist(err) {
		t.Errorf("a .tmp file survived Write: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dir, FileName))
	if err != nil {
		t.Fatalf("reading the stamp: %v", err)
	}
	if !strings.Contains(string(b), Current.String()) {
		t.Errorf("stamp %q does not contain %q", b, Current.String())
	}
}
