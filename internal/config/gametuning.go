// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/goccy/go-yaml"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// LoadGameTuningFor resolves cfg's game-tuning file (Config.GameConfigPath)
// and loads it, returning the tuning and the file it actually came from —
// empty if there was none. cmd/dlmud calls this once at boot and again on
// every SIGHUP.
//
// What "missing" means depends on which path it is, and the difference is
// the point. --config names a file an operator asked for by name, so one
// that is not there is a mistake worth refusing to boot on. The default
// <lib-dir>/config/game.yaml is not asked for at all: a data directory that
// has never been tuned simply does not have one, and that is config.c's own
// behaviour rather than an error — every stock and archived lib/ in
// existence is in exactly that state.
func LoadGameTuningFor(cfg *Config) (game.GameTuning, string, error) {
	path := cfg.GameConfigPath()
	t, err := LoadGameTuning(path)
	if err != nil {
		if cfg.GameConfigFile == "" && errors.Is(err, fs.ErrNotExist) {
			return game.DefaultGameTuning(), "", nil
		}
		return game.GameTuning{}, "", err
	}
	return t, path, nil
}

// LoadGameTuning reads path into a game.GameTuning, starting from
// game.DefaultGameTuning so a key the file omits keeps its config.c
// default — an empty file, and an empty path, both reproduce config.c's own
// behaviour exactly (go-port-plan.md §9.1).
//
// A path that does not exist is an error here; LoadGameTuningFor is what
// knows when that is worth reporting and when a missing file just means an
// untuned data directory.
func LoadGameTuning(path string) (game.GameTuning, error) {
	t := game.DefaultGameTuning()
	if path == "" {
		return t, nil
	}
	data, err := os.ReadFile(path) //nolint:gosec // an operator-configured data directory
	if err != nil {
		return game.GameTuning{}, fmt.Errorf("reading %s: %w", path, err)
	}

	// An empty, whitespace-only, or comments-only document parses as YAML
	// null, and goccy/go-yaml's Unmarshal of null into a struct pointer
	// zeroes every field rather than leaving them alone — the opposite of
	// every other key this function treats as "absent, keep the default".
	// Probe with a generic target first so a null document (this repo's own
	// every examples/ directory's own game.yaml as shipped, entirely
	// commented out) short-circuits to
	// the defaults rather than reaching the real Unmarshal below.
	var probe any
	if err := yaml.Unmarshal(data, &probe); err != nil {
		return game.GameTuning{}, fmt.Errorf("parsing %s: %w", path, err)
	}
	if probe == nil {
		return t, nil
	}

	if err := yaml.Unmarshal(data, &t); err != nil {
		return game.GameTuning{}, fmt.Errorf("parsing %s: %w", path, err)
	}
	if err := t.Validate(); err != nil {
		return game.GameTuning{}, fmt.Errorf("%s: %w", path, err)
	}
	return t, nil
}
