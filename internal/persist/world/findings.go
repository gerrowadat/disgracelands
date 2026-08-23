// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package world

import (
	"context"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// Severity ranks a finding by what it means for someone maintaining the
// world, not by how hard it was to detect.
//
// This lived in package classic until the yaml format needed the same
// "report every problem, don't stop at the first" shape for its own
// findings — unknown flag names, flags_raw uses, unbalanced colour, a lossy
// typed-value fallback, a reset chain with no mobile. classic.Severity and
// classic.Warning are now aliases of these, so nothing that imported them
// there had to change.
type Severity int

const (
	// Info: the loader did something to the data that the source file does
	// not say, and someone reading the file would not predict. Not a defect.
	Info Severity = iota
	// Warn: the world is playable but something in it does not work — an
	// exit to nowhere, a reset command referring to a mob that was deleted.
	Warn
	// Error: the world cannot be loaded correctly on this, so it must be
	// fixed before the result means anything.
	Error
)

// String returns the severity's lowercase name.
func (s Severity) String() string {
	switch s {
	case Info:
		return "info"
	case Warn:
		return "warn"
	case Error:
		return "error"
	}
	return "?"
}

// Warning is one finding from a load.
type Warning struct {
	Severity Severity
	Message  string
}

func (w Warning) String() string { return w.Severity.String() + ": " + w.Message }

// FindingSource is implemented by a Source that can report what it found
// wrong with the data alongside a (possibly partial) world, rather than
// only succeeding or failing outright. cmd/dlctl's world lint and world dump
// use this instead of asserting to a specific format's concrete type, so a
// new format gets the same reporting for free by implementing it.
type FindingSource interface {
	LoadWithWarnings(ctx context.Context) (*game.World, []Warning, error)
}
