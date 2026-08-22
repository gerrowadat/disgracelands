// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"context"
	"testing"
	"time"

	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/persist/clock"
)

// saveClock is save_mud_time end to end: it reads the world's current
// epoch, and writes back one that reproduces the same mud time — not
// necessarily the original epoch, per SavedEpoch's own doc comment and
// docs/weirdnumbers.md's "Saving the clock loses up to an hour, on
// purpose".
func TestSaveClockPersistsTheEpoch(t *testing.T) {
	srv, _ := newTestServer(t)
	dir := t.TempDir()
	srv.clockFormat = "native"
	srv.clockPath = dir

	ctx := context.Background()
	// Whole seconds: both formats hold seconds resolution (a bare Unix
	// integer, or RFC 3339), so a sub-second epoch would make "loaded is
	// not before the original" fail on the truncation alone.
	epoch := time.Now().Add(-3 * 24 * time.Hour).Truncate(time.Second)
	if err := srv.engine.DoSync(ctx, func(w *game.Live) { w.SetBooted(epoch) }); err != nil {
		t.Fatalf("DoSync: %v", err)
	}

	srv.saveClock(ctx)
	srv.WaitForWrites()

	loaded, err := clock.Load("native", dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Before(epoch) {
		t.Errorf("loaded epoch %v is before the original %v", loaded, epoch)
	}
	if drift := loaded.Sub(epoch); drift < 0 || drift >= time.Duration(game.SecondsPerMudHour)*time.Second {
		t.Errorf("drift = %v, want in [0, %ds) per SavedEpoch's own bound", drift, game.SecondsPerMudHour)
	}
}

// A server with no clock path configured (the default a test world gets)
// does not try to write anywhere.
func TestSaveClockWithNoPathConfiguredIsANoOp(t *testing.T) {
	srv, _ := newTestServer(t)
	srv.saveClock(context.Background())
	srv.WaitForWrites()
	// Nothing to assert beyond "did not panic and did not block" — there is
	// no path for anything to have been written to.
}

// SaveEverything, the shutdown path, saves the clock too — comm.c:441
// calls save_mud_time right after save_all() on the way down.
func TestShutdownSavesTheClock(t *testing.T) {
	srv, _ := newTestServer(t)
	dir := t.TempDir()
	srv.clockFormat = "native"
	srv.clockPath = dir

	ctx := context.Background()
	epoch := time.Now().Add(-time.Hour)
	if err := srv.engine.DoSync(ctx, func(w *game.Live) { w.SetBooted(epoch) }); err != nil {
		t.Fatalf("DoSync: %v", err)
	}

	srv.SaveEverything(ctx)
	srv.WaitForWrites()

	loaded, err := clock.Load("native", dir)
	if err != nil {
		t.Fatalf("Load after shutdown save: %v", err)
	}
	// Not the fallback: a file was actually written, distinct from "nothing
	// was there so Load made something up".
	if loaded.Unix() == clock.DefaultEpoch {
		t.Error("the shutdown save left no usable file behind (Load fell back to DefaultEpoch)")
	}
}
