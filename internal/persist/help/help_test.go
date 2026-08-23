// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package help

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/gerrowadat/disgracelands/internal/game"
)

func TestLoadClassicMissingIndexIsEmptyNotError(t *testing.T) {
	got, err := Load("classic", t.TempDir())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got != nil {
		t.Errorf("Load(missing index) = %v, want nil", got)
	}
}

func TestLoadYamlMissingHelpYAMLIsEmptyNotError(t *testing.T) {
	got, err := Load("yaml", t.TempDir())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got != nil {
		t.Errorf("Load(missing help.yaml) = %v, want nil", got)
	}
}

func TestYamlRoundTrips(t *testing.T) {
	dir := t.TempDir()
	want := []game.HelpEntry{
		{
			Keywords: []string{"ac", "armor class"},
			Body:     "AC ARMOR CLASS\r\n\r\nLower is better.\r\n\r\nSee also: EQUIP\r\n",
		},
		{
			// Punctuation-only keywords: HelpSlug returns "" for this
			// one, exercising Save's positional fallback.
			Keywords: []string{"!", "^"},
			Body:     "! ^\r\n\r\nUse ! to repeat the last command.\r\n",
		},
	}

	if err := Save("yaml", dir, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load("yaml", dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("round-trip = %+v, want %+v", got, want)
	}
}

func TestYamlDisambiguatesASlugCollision(t *testing.T) {
	dir := t.TempDir()
	// Two different entries whose keyword lines happen to slug the same
	// way — synthetic, since the real archive has none (see
	// internal/game's TestHelpSlugsAreUniqueAgainstTheRealArchive) — to
	// prove the writer's own belt-and-braces disambiguation actually
	// works rather than just being unreachable code.
	want := []game.HelpEntry{
		{Keywords: []string{"a-b"}, Body: "A-B\r\n\r\nFirst.\r\n"},
		{Keywords: []string{"a", "b"}, Body: "A B\r\n\r\nSecond.\r\n"},
	}
	if err := Save("yaml", dir, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load("yaml", dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("round-trip = %+v, want %+v", got, want)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	txtFiles := 0
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".txt" {
			txtFiles++
		}
	}
	if txtFiles != 2 {
		t.Errorf("got %d .txt files, want 2 (one per entry, disambiguated)", txtFiles)
	}
}

func TestYamlMissingEntryFileIsAnError(t *testing.T) {
	dir := t.TempDir()
	fixture := "schema: dl/help@1\nentries:\n- keywords: [ac]\n  file: ac.txt\n"
	if err := os.WriteFile(filepath.Join(dir, YamlFile), []byte(fixture), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	if _, err := Load("yaml", dir); err == nil {
		t.Error("Load with a listed-but-missing entry file succeeded, want an error")
	}
}

func TestClassicMissingListedFileIsAnError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, IndexFile), []byte("nonexistent.hlp\n$\n"), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	if _, err := Load("classic", dir); err == nil {
		t.Error("Load with a listed-but-missing .hlp file succeeded, want an error")
	}
}

func TestSaveClassicIsRefused(t *testing.T) {
	if err := Save("classic", t.TempDir(), nil); err == nil {
		t.Error("Save(classic) succeeded, want a refusal")
	}
}

func TestUnknownFormatIsRefused(t *testing.T) {
	if _, err := Load("nonsense", "x"); err == nil {
		t.Error("Load(nonsense) succeeded, want a refusal")
	}
	if err := Save("nonsense", t.TempDir(), nil); err == nil {
		t.Error("Save(nonsense) succeeded, want a refusal")
	}
}

// Against the real archive: classic parses it (already covered by
// game.ParseHelpFile's own tests), and importing it into yaml and
// reading it back produces byte-identical records.
func TestClassicToYamlImportAgainstTheRealArchive(t *testing.T) {
	classic, err := Load("classic", "../../../data/text/help")
	if err != nil {
		t.Fatalf("Load(classic): %v", err)
	}
	if len(classic) != 216 {
		t.Fatalf("got %d entries from the real archive, want 216", len(classic))
	}

	dir := t.TempDir()
	if err := Save("yaml", dir, classic); err != nil {
		t.Fatalf("Save(yaml): %v", err)
	}
	yaml, err := Load("yaml", dir)
	if err != nil {
		t.Fatalf("Load(yaml): %v", err)
	}
	if !reflect.DeepEqual(yaml, classic) {
		t.Fatalf("yaml round-trip does not match the classic parse")
	}
}

// dlctl help fmt should be idempotent: running Save on what Load(yaml)
// just produced writes byte-identical files.
func TestFmtIsIdempotent(t *testing.T) {
	classic, err := Load("classic", "../../../data/text/help")
	if err != nil {
		t.Fatalf("Load(classic): %v", err)
	}

	dir := t.TempDir()
	if err := Save("yaml", dir, classic); err != nil {
		t.Fatalf("Save(yaml) first pass: %v", err)
	}
	first, err := os.ReadFile(filepath.Join(dir, YamlFile))
	if err != nil {
		t.Fatal(err)
	}

	yaml, err := Load("yaml", dir)
	if err != nil {
		t.Fatalf("Load(yaml): %v", err)
	}
	if err := Save("yaml", dir, yaml); err != nil {
		t.Fatalf("Save(yaml) second pass: %v", err)
	}
	second, err := os.ReadFile(filepath.Join(dir, YamlFile))
	if err != nil {
		t.Fatal(err)
	}

	if string(first) != string(second) {
		t.Error("Save is not idempotent: help.yaml differs between passes")
	}
}
