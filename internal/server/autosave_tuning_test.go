// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"testing"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// TestTickAutosaveOffNeverSaves is auto_save=NO (config.c:150): the sweep
// never runs, and the counter never moves, however many ticks pass.
func TestTickAutosaveOffNeverSaves(t *testing.T) {
	tuning := game.DefaultGameTuning()
	tuning.AutoSave = false
	tuning.AutosaveTime = 1

	var counter int32
	for i := 0; i < 10; i++ {
		if tickAutosave(tuning, &counter) {
			t.Fatalf("tick %d: saved with auto_save off", i)
		}
	}
	if counter != 0 {
		t.Errorf("counter = %d after auto_save-off ticks, want 0", counter)
	}
}

// TestTickAutosaveFiresEveryAutosaveTimeTicks is comm.c:928-929's own shape:
// autosave_time-1 ticks of nothing, then a save, then the counter starts
// over.
func TestTickAutosaveFiresEveryAutosaveTimeTicks(t *testing.T) {
	tuning := game.DefaultGameTuning()
	tuning.AutoSave = true
	tuning.AutosaveTime = 3

	var counter int32
	var saves int
	for i := 1; i <= 9; i++ {
		if tickAutosave(tuning, &counter) {
			saves++
			if i%3 != 0 {
				t.Errorf("tick %d saved, want only every 3rd tick", i)
			}
		}
	}
	if saves != 3 {
		t.Errorf("9 ticks at autosave_time=3 produced %d saves, want 3", saves)
	}
	if counter != 0 {
		t.Errorf("counter = %d after a multiple of autosave_time ticks, want 0", counter)
	}
}

// TestTickAutosaveTimeOneSavesEveryTick is the C's own default-adjacent
// edge: autosave_time=1 means every PULSE_AUTOSAVE tick saves.
func TestTickAutosaveTimeOneSavesEveryTick(t *testing.T) {
	tuning := game.DefaultGameTuning()
	tuning.AutoSave = true
	tuning.AutosaveTime = 1

	var counter int32
	for i := 0; i < 5; i++ {
		if !tickAutosave(tuning, &counter) {
			t.Fatalf("tick %d did not save at autosave_time=1", i)
		}
	}
}
