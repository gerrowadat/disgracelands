// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import (
	"fmt"
	"sync/atomic"
)

// GameTuning is the subset of reference/moderncserver/src/config.c that this
// port makes a runtime setting rather than an archive fact — a deliberate,
// per-field exception to "the archive wins" (see docs/deviations.md), decided
// field by field rather than by reopening config.c wholesale. Everything in
// config.c not listed here (pk_allowed, the room vnums, autowiz, ...) stays a
// constant on purpose.
//
// Every field's zero-value-free default matches config.c exactly, so an
// unconfigured server, or a config file that omits a key, behaves exactly as
// the archive did (go-port-plan.md §9.1).
type GameTuning struct {
	// FreeRent is free_rent (config.c:133). See docs/deviations.md: on the
	// archive's own settings this is always true, so a receptionist never
	// actually prices a stay — but unlike when this was a hardcoded
	// constant, an operator can now turn it off.
	FreeRent bool `yaml:"free_rent"`
	// MinRentCost is the receptionist's fee, added to the item total
	// (config.c:139).
	MinRentCost int32 `yaml:"min_rent_cost"`
	// MaxObjSave is the most items a rent file will hold (config.c:136).
	MaxObjSave int32 `yaml:"max_obj_save"`

	// AutoSave is auto_save (config.c:150): whether the periodic save sweep
	// (comm.c:928) runs at all.
	AutoSave bool `yaml:"auto_save"`
	// AutosaveTime is autosave_time (config.c:157), in minutes: how often
	// AutoSave actually saves everyone, checked once per PULSE_AUTOSAVE
	// (60s) tick per comm.c:928-929's mins_since_crashsave counter.
	AutosaveTime int32 `yaml:"autosave_time"`

	// NPCCorpseTime and PlayerCorpseTime are config.c:76's corpse timers,
	// in mud hours: a player's corpse lasts about twelve and a half real
	// minutes, a mobile's about six.
	NPCCorpseTime    int32 `yaml:"max_npc_corpse_time"`
	PlayerCorpseTime int32 `yaml:"max_pc_corpse_time"`

	// LevelCanShout and HollerMoveCost are config.c:61 and :64. Shouting is
	// free at level one; hollering costs movement however loud you are.
	LevelCanShout  int32 `yaml:"level_can_shout"`
	HollerMoveCost int32 `yaml:"holler_move_cost"`

	// MaxFileSize is max_filesize (config.c:233): do_gen_write (bug/idea/
	// typo reports) refuses to append once a report file reaches this many
	// bytes.
	MaxFileSize int64 `yaml:"max_filesize"`

	// MaxBadPws is max_bad_pws (config.c:236): how many wrong passwords one
	// connection may type at the login prompt before it is disconnected.
	// The count is per *connection* (the C's `d->bad_pws`), not per
	// character, so reconnecting starts again from zero — the separate
	// per-character tally is what the "N LOGIN FAILURES SINCE LAST
	// SUCCESSFUL LOGIN" notice reports (interpreter.c:1466-1474).
	MaxBadPws int32 `yaml:"max_bad_pws"`
}

// DefaultGameTuning returns config.c's own values — the archive's settings,
// unaltered. Loading no config file at all reproduces this exactly.
func DefaultGameTuning() GameTuning {
	return GameTuning{
		FreeRent:         true,
		MinRentCost:      100,
		MaxObjSave:       30,
		AutoSave:         true,
		AutosaveTime:     5,
		NPCCorpseTime:    5,
		PlayerCorpseTime: 10,
		LevelCanShout:    1,
		HollerMoveCost:   20,
		MaxFileSize:      50000,
		MaxBadPws:        3,
	}
}

// Validate rejects tuning that cannot work. Called on boot and before every
// SIGHUP reload takes effect — a bad reload logs and keeps the previous
// values rather than wedging the game (see cmd/dlmud).
func (t GameTuning) Validate() error {
	switch {
	case t.MinRentCost < 0:
		return fmt.Errorf("min_rent_cost: must not be negative, got %d", t.MinRentCost)
	case t.MaxObjSave < 0:
		return fmt.Errorf("max_obj_save: must not be negative, got %d", t.MaxObjSave)
	case t.AutosaveTime < 1:
		return fmt.Errorf("autosave_time: must be at least 1 (minute), got %d", t.AutosaveTime)
	case t.NPCCorpseTime < 0:
		return fmt.Errorf("max_npc_corpse_time: must not be negative, got %d", t.NPCCorpseTime)
	case t.PlayerCorpseTime < 0:
		return fmt.Errorf("max_pc_corpse_time: must not be negative, got %d", t.PlayerCorpseTime)
	case t.LevelCanShout < 0:
		return fmt.Errorf("level_can_shout: must not be negative, got %d", t.LevelCanShout)
	case t.HollerMoveCost < 0:
		return fmt.Errorf("holler_move_cost: must not be negative, got %d", t.HollerMoveCost)
	case t.MaxFileSize < 0:
		return fmt.Errorf("max_filesize: must not be negative, got %d", t.MaxFileSize)
	case t.MaxBadPws < 1:
		// Zero would disconnect on the *first* keystroke at the password
		// prompt, before anything had been checked: the C's test is
		// `++(d->bad_pws) >= max_bad_pws`, so one is the smallest setting
		// that still lets a password be typed at all.
		return fmt.Errorf("max_bad_pws: must be at least 1, got %d", t.MaxBadPws)
	}
	return nil
}

// current holds the live tuning behind an atomic pointer rather than a plain
// package var: it is read from the world goroutine (rent, corpse decay,
// shout/holler), from Server.RunAutosave's own dedicated ticker goroutine,
// and potentially from a report append that (deliberately, see
// internal/server/reports.go) does not run through Server.background either
// — three different goroutines with no other synchronization between them.
// An atomic pointer swap publishes a whole new, internally-consistent
// GameTuning value with one store, so every reader sees either the old
// snapshot or the new one, never a partial mix of old and new fields.
var current atomic.Pointer[GameTuning]

func init() {
	d := DefaultGameTuning()
	current.Store(&d)
}

// Tuning returns the live game tuning.
func Tuning() GameTuning {
	return *current.Load()
}

// SetTuning replaces the live game tuning, atomically. cmd/dlmud calls this
// once at boot (after loading the data directory's own config/game.yaml, if
// it has one) and again on every SIGHUP that passes GameTuning.Validate.
func SetTuning(t GameTuning) {
	cp := t
	current.Store(&cp)
}
