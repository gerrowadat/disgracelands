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

// TestLoadGameTuningCommentsOnlyFileIsTheDefault is the shipped
// config/game.yaml as it stands: every key present but commented out. A
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
max_bad_pws: 5
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
		MaxFileSize: 100000, MaxBadPws: 5,
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

// shippedExamples is every annotated game.yaml this repo ships, each in the
// place it now lives: inside a data directory, not in a config/ directory of
// its own beside the binary (docs/design/data-format.md §6). One per example
// world per format, because a data directory is what carries this file —
// including examples/stock/binary, which is the server's own default
// --lib-dir, so a fresh clone has the template where it would be edited.
var shippedExamples = []string{
	filepath.Join("..", "..", "examples", "stock", "binary", "config", "game.yaml"),
	filepath.Join("..", "..", "examples", "stock", "yaml", "config", "game.yaml"),
	filepath.Join("..", "..", "examples", "mini", "binary", "config", "game.yaml"),
	filepath.Join("..", "..", "examples", "mini", "yaml", "config", "game.yaml"),
}

// TestLoadGameTuningShippedExampleFile loads those files, not copies of them
// — an edit that breaks the "every value commented out" promise in their own
// header comment fails here, not just in a hand-written fixture that could
// drift from the real file.
func TestLoadGameTuningShippedExampleFile(t *testing.T) {
	for _, path := range shippedExamples {
		got, err := LoadGameTuning(path)
		if err != nil {
			t.Errorf("LoadGameTuning(%s) = error %v, want success", path, err)
			continue
		}
		if want := game.DefaultGameTuning(); got != want {
			t.Errorf("LoadGameTuning(%s) = %+v, want the default %+v (every key should be commented out)", path, got, want)
		}
	}
}

// TestShippedExamplesAreTheSameFile. There is one template, copied into each
// example data directory — `dlctl import` is what puts the yaml ones
// there, byte for byte from their binary source. Editing one and not the
// others would leave three stale copies of a file whose entire job is to be
// read by a person, so this holds them together.
func TestShippedExamplesAreTheSameFile(t *testing.T) {
	first, err := os.ReadFile(shippedExamples[0])
	if err != nil {
		t.Fatalf("reading %s: %v", shippedExamples[0], err)
	}
	for _, path := range shippedExamples[1:] {
		got, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("reading %s: %v", path, err)
			continue
		}
		if string(got) != string(first) {
			t.Errorf("%s differs from %s; they are meant to be the same template", path, shippedExamples[0])
		}
	}
}

// TestGameConfigPath covers the resolution rule itself: the data directory
// by default, --config where one was given.
func TestGameConfigPath(t *testing.T) {
	cfg := Config{LibDir: "/srv/dl"}
	if got, want := cfg.GameConfigPath(), "/srv/dl/config/game.yaml"; got != want {
		t.Errorf("GameConfigPath() = %q, want %q", got, want)
	}

	cfg.GameConfigFile = "/etc/disgracelands/tuning.yaml"
	if got, want := cfg.GameConfigPath(), "/etc/disgracelands/tuning.yaml"; got != want {
		t.Errorf("GameConfigPath() with --config = %q, want the override %q", got, want)
	}
}

// TestLoadGameTuningForReadsTheDataDirectory is the whole point of the file
// living where it does: a server told only --lib-dir picks up that
// directory's own tuning, with no second flag naming it.
func TestLoadGameTuningForReadsTheDataDirectory(t *testing.T) {
	lib := t.TempDir()
	if err := os.MkdirAll(filepath.Join(lib, "config"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(lib, "config", "game.yaml"), "free_rent: false\n")

	cfg := Config{LibDir: lib}
	got, path, err := LoadGameTuningFor(&cfg)
	if err != nil {
		t.Fatalf("LoadGameTuningFor = error %v, want success", err)
	}
	if want := cfg.GameConfigPath(); path != want {
		t.Errorf("LoadGameTuningFor read %q, want %q", path, want)
	}
	want := game.DefaultGameTuning()
	want.FreeRent = false
	if got != want {
		t.Errorf("LoadGameTuningFor = %+v, want %+v", got, want)
	}
}

// TestLoadGameTuningForMissingFileIsFine: every stock and archived lib/ in
// existence has no config/game.yaml in it, and every one of them has to
// boot — on config.c's own values, and with no file reported as read.
func TestLoadGameTuningForMissingFileIsFine(t *testing.T) {
	cfg := Config{LibDir: t.TempDir()}
	got, path, err := LoadGameTuningFor(&cfg)
	if err != nil {
		t.Fatalf("LoadGameTuningFor with no file = error %v, want success", err)
	}
	if path != "" {
		t.Errorf("LoadGameTuningFor with no file read %q, want no file at all", path)
	}
	if want := game.DefaultGameTuning(); got != want {
		t.Errorf("LoadGameTuningFor with no file = %+v, want the default %+v", got, want)
	}
}

// TestLoadGameTuningForMissingConfigFlagFails is the other half of that
// rule: --config names a file an operator asked for, so a typo in it is a
// boot failure rather than a silent fallback to the defaults.
func TestLoadGameTuningForMissingConfigFlagFails(t *testing.T) {
	cfg := Config{
		LibDir:         t.TempDir(),
		GameConfigFile: filepath.Join(t.TempDir(), "does-not-exist.yaml"),
	}
	if _, _, err := LoadGameTuningFor(&cfg); err == nil {
		t.Error("LoadGameTuningFor with a missing --config = success, want an error")
	}
}

// TestLoadGameTuningForPrefersTheFlag: --config wins over a data directory
// that has its own file, so an operator who overrides gets what they asked
// for rather than a merge of the two.
func TestLoadGameTuningForPrefersTheFlag(t *testing.T) {
	lib := t.TempDir()
	if err := os.MkdirAll(filepath.Join(lib, "config"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(lib, "config", "game.yaml"), "min_rent_cost: 111\n")
	override := filepath.Join(t.TempDir(), "tuning.yaml")
	writeFile(t, override, "min_rent_cost: 222\n")

	got, path, err := LoadGameTuningFor(&Config{LibDir: lib, GameConfigFile: override})
	if err != nil {
		t.Fatalf("LoadGameTuningFor = error %v, want success", err)
	}
	if path != override {
		t.Errorf("LoadGameTuningFor read %q, want the override %q", path, override)
	}
	if got.MinRentCost != 222 {
		t.Errorf("MinRentCost = %d, want the override's 222", got.MinRentCost)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}
