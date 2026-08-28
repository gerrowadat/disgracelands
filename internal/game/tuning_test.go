// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import "testing"

// TestDefaultGameTuningMatchesConfigC pins every field against
// reference/moderncserver/src/config.c's own value, so a change here is a
// deliberate edit to that file, not a silent drift.
func TestDefaultGameTuningMatchesConfigC(t *testing.T) {
	d := DefaultGameTuning()
	want := GameTuning{
		FreeRent:         true,  // config.c:133
		MinRentCost:      100,   // config.c:139
		MaxObjSave:       30,    // config.c:136
		AutoSave:         true,  // config.c:150
		AutosaveTime:     5,     // config.c:157
		NPCCorpseTime:    5,     // config.c:76
		PlayerCorpseTime: 10,    // config.c:75
		LevelCanShout:    1,     // config.c:61
		HollerMoveCost:   20,    // config.c:64
		MaxFileSize:      50000, // config.c:233
		MaxBadPws:        3,     // config.c:236
	}
	if d != want {
		t.Errorf("DefaultGameTuning() = %+v, want %+v", d, want)
	}
	if err := d.Validate(); err != nil {
		t.Errorf("the default tuning does not validate: %v", err)
	}
}

// TestTuningStartsAtTheDefault covers the package's own init(): before
// anything calls SetTuning, Tuning() must already answer with config.c's
// values, since nothing in cmd/dlmud guarantees SetTuning runs before code
// that reads Tuning() in a test binary that never calls main.
func TestTuningStartsAtTheDefault(t *testing.T) {
	if got := Tuning(); got != DefaultGameTuning() {
		t.Errorf("Tuning() before any SetTuning = %+v, want the default %+v", got, DefaultGameTuning())
	}
}

// TestSetTuningRoundTrips is the whole contract SIGHUP reload depends on:
// whatever is stored is what the next Tuning() call returns.
func TestSetTuningRoundTrips(t *testing.T) {
	orig := Tuning()
	t.Cleanup(func() { SetTuning(orig) })

	custom := DefaultGameTuning()
	custom.FreeRent = false
	custom.MinRentCost = 250
	custom.LevelCanShout = 5
	SetTuning(custom)

	if got := Tuning(); got != custom {
		t.Errorf("Tuning() after SetTuning = %+v, want %+v", got, custom)
	}
}

func TestGameTuningValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*GameTuning)
		wantErr bool
	}{
		{"the default", func(*GameTuning) {}, false},
		{"negative min_rent_cost", func(g *GameTuning) { g.MinRentCost = -1 }, true},
		{"negative max_obj_save", func(g *GameTuning) { g.MaxObjSave = -1 }, true},
		{"zero autosave_time", func(g *GameTuning) { g.AutosaveTime = 0 }, true},
		{"negative autosave_time", func(g *GameTuning) { g.AutosaveTime = -1 }, true},
		{"negative max_npc_corpse_time", func(g *GameTuning) { g.NPCCorpseTime = -1 }, true},
		{"negative max_pc_corpse_time", func(g *GameTuning) { g.PlayerCorpseTime = -1 }, true},
		{"negative level_can_shout", func(g *GameTuning) { g.LevelCanShout = -1 }, true},
		{"negative holler_move_cost", func(g *GameTuning) { g.HollerMoveCost = -1 }, true},
		{"negative max_filesize", func(g *GameTuning) { g.MaxFileSize = -1 }, true},
		// One is the floor rather than zero: `++(d->bad_pws) >= max_bad_pws`
		// at zero would disconnect before a password could be typed.
		{"one max_bad_pws", func(g *GameTuning) { g.MaxBadPws = 1 }, false},
		{"zero max_bad_pws", func(g *GameTuning) { g.MaxBadPws = 0 }, true},
		{"negative max_bad_pws", func(g *GameTuning) { g.MaxBadPws = -1 }, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := DefaultGameTuning()
			tt.mutate(&g)
			err := g.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() = %v, want error: %v", err, tt.wantErr)
			}
		})
	}
}
