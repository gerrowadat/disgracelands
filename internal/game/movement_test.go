// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import (
	"os"
	"regexp"
	"strconv"
	"testing"
)

// movement_loss[] is a table, so it gets re-parsed rather than read — the same
// argument as every `const char *` table in bitnames_test.go, and with more at
// stake, because these entries are numbers and a transposed pair reads as
// perfectly plausible terrain costs.

// cIntTableSource matches `int name[] = { 1, 2, ... };`.
var cIntTableSource = regexp.MustCompile(`(?s)\bint (\w+)\[\]\s*=?\s*\{(.*?)\n\};`)

var cInteger = regexp.MustCompile(`-?\d+`)

func TestMovementLossMatchesTheCSource(t *testing.T) {
	const path = "../../reference/moderncserver/src/constants.c"

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}

	var want []int32
	found := false
	for _, m := range cIntTableSource.FindAllStringSubmatch(string(b), -1) {
		if m[1] != "movement_loss" {
			continue
		}
		found = true
		// The comments name the sector and contain no digits, but strip them
		// anyway: a comment that ever grows a number would silently become an
		// entry.
		body := cLineComment.ReplaceAllString(cBlockComment.ReplaceAllString(m[2], ""), "")
		for _, lit := range cInteger.FindAllString(body, -1) {
			n, err := strconv.Atoi(lit)
			if err != nil {
				t.Fatalf("movement_loss has a non-integer entry %q", lit)
			}
			want = append(want, int32(n))
		}
	}
	if !found {
		t.Fatalf("no movement_loss table in %s", path)
	}

	if len(movementLoss) != len(want) {
		t.Fatalf("movement_loss has %d entries, the C has %d\n got: %v\nwant: %v",
			len(movementLoss), len(want), movementLoss, want)
	}
	for i := range want {
		if movementLoss[i] != want[i] {
			t.Errorf("movement_loss[%d] is %d, the C has %d (%s)",
				i, movementLoss[i], want[i], SectorNames()[i])
		}
	}

	// One entry per sector type, or the index means different things in the
	// two tables.
	if len(movementLoss) != len(SectorNames()) {
		t.Errorf("movement_loss has %d entries and there are %d sector types",
			len(movementLoss), len(SectorNames()))
	}
}

// The average truncates, which is where every surprising cost comes from.
func TestMovementCostIsTheTruncatedAverage(t *testing.T) {
	// The named constants rather than a second list of numbers: all ten
	// sectors have names now, so a local `city = 1` is a copy of one.
	const (
		inside     = SectorInside
		city       = SectorCity
		field      = SectorField
		forest     = SectorForest
		mountains  = SectorMountains
		underwater = SectorUnderwater
	)
	room := func(sector Sector) *RoomDef { return &RoomDef{SectorType: sector} }

	for _, tc := range []struct {
		name     string
		from, to Sector
		want     int32
	}{
		{"city to city", city, city, 1},
		// (1+2)/2 is 1.5 and the C truncates, so stepping off the pavement
		// into a field is as cheap as walking down the street.
		{"city to field", city, field, 1},
		{"field to forest", field, forest, 2},
		{"city to mountains", city, mountains, 3},
		{"mountains to city", mountains, city, 3},
		{"inside to inside", inside, inside, 1},
		{"underwater to mountains", underwater, mountains, 5},
	} {
		if got := MovementCost(room(tc.from), room(tc.to)); got != tc.want {
			t.Errorf("%s costs %d, want %d", tc.name, got, tc.want)
		}
	}

	// A sector the loader would have refused answers Inside's cost rather
	// than reading off the end of the table, which is what the C does.
	if got := MovementCost(room(99), room(city)); got != 1 {
		t.Errorf("an out-of-range sector costs %d, want 1", got)
	}
	if got := MovementCost(nil, nil); got != 1 {
		t.Errorf("a step between two rooms that do not exist costs %d, want 1", got)
	}
}
