// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCheckNotLegacyRefusesTheRealArchiveShape points the check at the
// checked-in stock lib/ — a real CircleMUD directory, not a synthetic one
// — and requires the refusal to name the offending files and the exact
// command to run.
func TestCheckNotLegacyRefusesTheRealArchiveShape(t *testing.T) {
	const stock = "../../examples/stock/binary"
	err := CheckNotLegacy(stock)
	if err == nil {
		t.Fatal("CheckNotLegacy accepted a classic lib/ directory")
	}
	for _, want := range []string{
		"world/zone.lst",
		"misc/socials",
		"dlctl import --from-dir=" + stock,
		"--to-dir=",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not mention %q:\n%v", want, err)
		}
	}
}

// TestCheckNotLegacyAcceptsAConvertedDirectory. The other half, and the
// one that would make this check worse than useless if it were wrong.
func TestCheckNotLegacyAcceptsAConvertedDirectory(t *testing.T) {
	if err := CheckNotLegacy("../../examples/stock/yaml"); err != nil {
		t.Errorf("CheckNotLegacy refused a converted directory: %v", err)
	}
}

// TestCheckNotLegacyIgnoresAnEmptyDirectory. Detection is on the legacy
// *marker files*, not on the absence of yaml, and this is why: somebody
// who mistypes --lib-dir, or points at a directory they have not created
// yet, should get the ordinary "no world data" error the server has
// always given rather than a confident instruction to convert an archive
// that is not there (docs/design/yaml-only.md §3.3).
func TestCheckNotLegacyIgnoresAnEmptyDirectory(t *testing.T) {
	if err := CheckNotLegacy(t.TempDir()); err != nil {
		t.Errorf("CheckNotLegacy refused an empty directory: %v", err)
	}
	if err := CheckNotLegacy(filepath.Join(t.TempDir(), "does-not-exist")); err != nil {
		t.Errorf("CheckNotLegacy refused a missing directory: %v", err)
	}
}

// TestCheckNotLegacyAcceptsAConversionInProgress. A directory with both
// yaml world files and legacy markers is what converting *into* an
// existing tree looks like halfway through, and the yaml files are what
// the server will read.
func TestCheckNotLegacyAcceptsAConversionInProgress(t *testing.T) {
	dir := t.TempDir()
	mkfile(t, filepath.Join(dir, "world", "zone.lst"), "Zone Vnum List\n")
	mkfile(t, filepath.Join(dir, "world", "zones.yaml"), "schema: dl/zones@1\n")

	if err := CheckNotLegacy(dir); err != nil {
		t.Errorf("CheckNotLegacy refused a directory that already has yaml in it: %v", err)
	}
}

// TestCheckNotLegacyNoticesEachMarkerOnItsOwn, so that a directory
// carrying only one of them — an archive whose world was converted and
// whose roster was not, say — is still caught.
func TestCheckNotLegacyNoticesEachMarkerOnItsOwn(t *testing.T) {
	for _, m := range legacyMarkers {
		t.Run(m.path, func(t *testing.T) {
			dir := t.TempDir()
			mkfile(t, filepath.Join(dir, m.path), "")
			err := CheckNotLegacy(dir)
			if err == nil {
				t.Fatalf("CheckNotLegacy accepted a directory holding %s", m.path)
			}
			if !strings.Contains(err.Error(), m.what) {
				t.Errorf("the refusal does not say what %s is (%q):\n%v", m.path, m.what, err)
			}
		})
	}
}

func mkfile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}
