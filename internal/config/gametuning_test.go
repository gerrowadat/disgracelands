// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// TestLoadGameTuningEmptyPathIsTheDefault covers the no-'--config'-flag
// case: config.c's own values, exactly.
func TestLoadGameTuningEmptyPathIsTheDefault(t *testing.T) {
	got, err := LoadGameTuning("")
	if err != nil {
		t.Fatalf("LoadGameTuning(\"\") = error %v, want success", err)
	}
	if want := game.DefaultGameTuning(); got != want {
		t.Errorf("LoadGameTuning(\"\") = %+v, want the default %+v", got, want)
	}
}

// TestLoadGameTuningMissingKeysKeepTheirDefault is the "an empty file
// reproduces config.c exactly" guarantee go-port-plan.md §9.1 promises: a
// file naming only one key must not zero out every other field.
func TestLoadGameTuningMissingKeysKeepTheirDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "game.yaml")
	writeFile(t, path, "free_rent: false\n")

	got, err := LoadGameTuning(path)
	if err != nil {
		t.Fatalf("LoadGameTuning(%q) = error %v, want success", path, err)
	}

	want := game.DefaultGameTuning()
	want.FreeRent = false
	if got != want {
		t.Errorf("LoadGameTuning(%q) = %+v, want %+v", path, got, want)
	}
}

// TestLoadGameTuningEmptyFileIsTheDefault covers the literal empty file, not
// just an empty path.
func TestLoadGameTuningEmptyFileIsTheDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "game.yaml")
	writeFile(t, path, "")

	got, err := LoadGameTuning(path)
	if err != nil {
		t.Fatalf("LoadGameTuning(%q) = error %v, want success", path, err)
	}
	if want := game.DefaultGameTuning(); got != want {
		t.Errorf("LoadGameTuning(%q) = %+v, want the default %+v", path, got, want)
	}
}

// TestLoadGameTuningCommentsOnlyFileIsTheDefault is this repo's own
// config/game.yaml as shipped: every key present but commented out. A
// comments-only document parses as YAML null, which is the specific case
// that first exposed goccy/go-yaml zeroing the whole struct instead of
// leaving it alone (see LoadGameTuning's own comment) — this is the
// regression test for that.
func TestLoadGameTuningCommentsOnlyFileIsTheDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "game.yaml")
	writeFile(t, path, "# free_rent: true\n# min_rent_cost: 100\n")

	got, err := LoadGameTuning(path)
	if err != nil {
		t.Fatalf("LoadGameTuning(%q) = error %v, want success", path, err)
	}
	if want := game.DefaultGameTuning(); got != want {
		t.Errorf("LoadGameTuning(%q) = %+v, want the default %+v", path, got, want)
	}
}

// TestLoadGameTuningOverridesEveryField exercises every yaml key at once,
// against the field it is meant to set — a table re-parse against
// GameTuning's own yaml tags would catch a renamed field better than this,
// but there is exactly one file to check by hand here, not a table to walk.
func TestLoadGameTuningOverridesEveryField(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "game.yaml")
	writeFile(t, path, `
free_rent: false
min_rent_cost: 250
max_obj_save: 40
auto_save: false
autosave_time: 15
max_npc_corpse_time: 3
max_pc_corpse_time: 20
level_can_shout: 5
holler_move_cost: 30
max_filesize: 100000
`)

	got, err := LoadGameTuning(path)
	if err != nil {
		t.Fatalf("LoadGameTuning(%q) = error %v, want success", path, err)
	}
	want := game.GameTuning{
		FreeRent: false, MinRentCost: 250, MaxObjSave: 40,
		AutoSave: false, AutosaveTime: 15,
		NPCCorpseTime: 3, PlayerCorpseTime: 20,
		LevelCanShout: 5, HollerMoveCost: 30,
		MaxFileSize: 100000,
	}
	if got != want {
		t.Errorf("LoadGameTuning(%q) = %+v, want %+v", path, got, want)
	}
}

func TestLoadGameTuningMissingFile(t *testing.T) {
	if _, err := LoadGameTuning(filepath.Join(t.TempDir(), "does-not-exist.yaml")); err == nil {
		t.Error("LoadGameTuning on a missing file = success, want an error")
	}
}

func TestLoadGameTuningInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "game.yaml")
	writeFile(t, path, "free_rent: [this is not a bool\n")

	if _, err := LoadGameTuning(path); err == nil {
		t.Error("LoadGameTuning on invalid YAML = success, want an error")
	}
}

// TestLoadGameTuningRejectsInvalidValues covers SIGHUP's own failure mode: a
// syntactically valid file with a value that cannot work must be reported
// as an error, not silently accepted, so cmd/dlmud keeps the old tuning.
func TestLoadGameTuningRejectsInvalidValues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "game.yaml")
	writeFile(t, path, "autosave_time: 0\n")

	if _, err := LoadGameTuning(path); err == nil {
		t.Error("LoadGameTuning with autosave_time: 0 = success, want a validation error")
	}
}

// TestLoadGameTuningShippedExampleFile loads the repo's actual
// config/game.yaml, not a copy of it — an edit that breaks the "every value
// commented out" promise in its own header comment fails here, not just in
// a hand-written fixture that could drift from the real file.
func TestLoadGameTuningShippedExampleFile(t *testing.T) {
	got, err := LoadGameTuning(filepath.Join("..", "..", "config", "game.yaml"))
	if err != nil {
		t.Fatalf("LoadGameTuning(config/game.yaml) = error %v, want success", err)
	}
	if want := game.DefaultGameTuning(); got != want {
		t.Errorf("LoadGameTuning(config/game.yaml) = %+v, want the default %+v (every key should be commented out)", got, want)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}
