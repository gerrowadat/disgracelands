// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

//go:build play && race

package play

// raceEnabled is true when this test binary was built with -race, and is what
// makes TestMain build the server the same way. Split across two files
// because the race detector's own build tag is the only honest way to ask:
// there is no runtime flag for "was I instrumented".
const raceEnabled = true
