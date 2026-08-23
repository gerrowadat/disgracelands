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
	"github.com/gerrowadat/disgracelands/internal/persist/player"
)

// SweepRentFiles is update_obj_file (objsave.c:332): the boot-time pass
// that deletes rent files older than 30 real days and crash files older
// than 10, and leaves everything else — including cryo, which the C's own
// Crash_clean_file has no case for at all — alone.
func TestSweepRentFilesDeletesOnlyWhatIsPastItsTimeout(t *testing.T) {
	srv, _ := newTestServer(t)
	ctx := context.Background()

	names := []string{"Oldcrash", "Freshcrash", "Oldrent", "Freshrent", "Oldcryo", "Norentfile"}
	for _, name := range names {
		if err := srv.players.Save(ctx, &game.PlayerRecord{Name: name}); err != nil {
			t.Fatalf("seeding %s onto the roster: %v", name, err)
		}
	}

	save := func(name string, code player.RentCode, age time.Duration) {
		t.Helper()
		f := &player.RentFile{Code: code, Written: time.Now().Add(-age)}
		if err := srv.objects.SaveObjects(ctx, name, f); err != nil {
			t.Fatalf("writing %s's rent file: %v", name, err)
		}
	}
	// One real day either side of each kind's own timeout (10 for a crash,
	// 30 for a rent), so the boundary itself is exercised and not just the
	// obviously-old and obviously-fresh cases.
	save("Oldcrash", player.RentCrash, 11*24*time.Hour)
	save("Freshcrash", player.RentCrash, 9*24*time.Hour)
	save("Oldrent", player.RentRented, 31*24*time.Hour)
	save("Freshrent", player.RentRented, 29*24*time.Hour)
	// A cryo file older than either timeout would ever reach: still never
	// swept, since RentCryo has no case in Crash_clean_file's own
	// if/else-if at all.
	save("Oldcryo", player.RentCryo, 400*24*time.Hour)
	// Norentfile has none at all — Crash_clean_file's own ENOENT branch,
	// and must not make the sweep fail for anybody listed after it.

	srv.SweepRentFiles(ctx)

	exists := func(name string) bool {
		_, err := srv.objects.LoadObjects(ctx, name)
		return err == nil
	}
	cases := []struct {
		name string
		want bool
	}{
		{"Oldcrash", false},
		{"Freshcrash", true},
		{"Oldrent", false},
		{"Freshrent", true},
		{"Oldcryo", true},
	}
	for _, c := range cases {
		if got := exists(c.name); got != c.want {
			verb := "survived"
			if !c.want {
				verb = "was swept"
			}
			t.Errorf("%s's rent file %s = %v, want %v", c.name, verb, got, c.want)
		}
	}
}

// TestSweepRentFilesToleratesAMissingObjectStore, the same defensiveness
// every other Operator-adjacent method in this tree already has for a
// server built without one (docs/configuration.md's *(inert)* flags are
// what a nil store would otherwise crash on).
func TestSweepRentFilesToleratesAMissingObjectStore(t *testing.T) {
	srv, _ := newTestServer(t)
	srv.objects = nil
	srv.SweepRentFiles(context.Background()) // must not panic
}
