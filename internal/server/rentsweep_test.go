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

// TestSweepLeavesARentFileWithNoTimestampAlone is the half of #294 that is
// a plain defect rather than a policy question.
//
// The yaml format's `written:` is omitempty, so a rent file that lacks one
// reads back as the zero Time. The sweep then computed now.Sub(zero) --
// about two thousand years -- and deleted a character's possessions on the
// strength of a timestamp it had failed to read. The C cannot reach this
// at all: `rent.time` is an int32 in a struct that is always fully
// present, so "missing" is not a state it has.
//
// The second case is what stops the fix being a much larger one by
// accident. A rent file that genuinely says 1970 reads back as
// time.Unix(0, 0), which is an ordinary instant and not the zero Time
// (whose year is 1) -- so it is still swept, exactly as the C sweeps it,
// and "we could not read the timestamp" stays distinct from "the timestamp
// is very old".
func TestSweepLeavesARentFileWithNoTimestampAlone(t *testing.T) {
	srv, _ := newTestServer(t)
	ctx := context.Background()
	chargeRent(t)

	for _, name := range []string{"Untimed", "Epoch"} {
		if err := srv.players.Save(ctx, &game.PlayerRecord{Name: name}); err != nil {
			t.Fatalf("seeding %s onto the roster: %v", name, err)
		}
	}

	// No timestamp at all: the writer omits `written:` for a zero time.
	if err := srv.objects.SaveObjects(ctx, "Untimed", &player.RentFile{Code: player.RentRented}); err != nil {
		t.Fatalf("writing Untimed's rent file: %v", err)
	}
	// A real timestamp that happens to be the epoch, which is far past
	// every timeout there is.
	if err := srv.objects.SaveObjects(ctx, "Epoch", &player.RentFile{
		Code: player.RentRented, Written: time.Unix(0, 0).UTC(),
	}); err != nil {
		t.Fatalf("writing Epoch's rent file: %v", err)
	}

	srv.SweepRentFiles(ctx)

	if _, err := srv.objects.LoadObjects(ctx, "Untimed"); err != nil {
		t.Errorf("a rent file with no timestamp was swept (%v); it should be left alone, "+
			"because the sweep does not know when it was written", err)
	}
	if _, err := srv.objects.LoadObjects(ctx, "Epoch"); err == nil {
		t.Error("a rent file written at the epoch survived: it has a timestamp, it is " +
			"thirty years past the timeout, and the C would delete it")
	}
}

// SweepRentFiles is update_obj_file (objsave.c:332): the boot-time pass
// that deletes rent files older than 30 real days and crash files older
// than 10, and leaves everything else — including cryo, which the C's own
// Crash_clean_file has no case for at all — alone.
func TestSweepRentFilesDeletesOnlyWhatIsPastItsTimeout(t *testing.T) {
	srv, _ := newTestServer(t)
	ctx := context.Background()
	chargeRent(t)

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

// chargeRent turns rent charging on for one test, which every test of the
// sweep needs: the sweep does nothing at all while rent is free, and rent is
// free by default because config.c:133 says so.
func chargeRent(t *testing.T) {
	t.Helper()
	orig := game.Tuning()
	t.Cleanup(func() { game.SetTuning(orig) })
	tuning := orig
	tuning.FreeRent = false
	game.SetTuning(tuning)
}

// TestNothingIsSweptWhileRentIsFree is the ruling #294 was filed for, and
// the deviation SweepRentFiles' own comment argues.
//
// The C runs update_obj_file whatever free_rent is set to. This does not,
// because the sweep is the enforcement half of a charge that is not being
// made: nobody on the archived server ever paid rent, so a rent file that
// "timed out" fell behind on a bill of nothing. Converting an archived lib/
// and booting on it deleted the stored possessions of every character who
// had not played in thirty days, which for an archive is all of them.
//
// The second half is what keeps this a setting rather than a decision:
// switch charging on and the C's behaviour comes back, timeouts and all.
func TestNothingIsSweptWhileRentIsFree(t *testing.T) {
	ctx := context.Background()

	seed := func(t *testing.T, srv *Server) {
		t.Helper()
		for _, name := range []string{"Ancient", "Crashed"} {
			if err := srv.players.Save(ctx, &game.PlayerRecord{Name: name}); err != nil {
				t.Fatalf("seeding %s: %v", name, err)
			}
		}
		// Both far past their own kind's timeout: an archive's worth of
		// age, which is the case that was losing everything.
		if err := srv.objects.SaveObjects(ctx, "Ancient", &player.RentFile{
			Code: player.RentRented, Written: time.Now().Add(-4000 * 24 * time.Hour),
		}); err != nil {
			t.Fatal(err)
		}
		if err := srv.objects.SaveObjects(ctx, "Crashed", &player.RentFile{
			Code: player.RentCrash, Written: time.Now().Add(-4000 * 24 * time.Hour),
		}); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("free rent keeps everything", func(t *testing.T) {
		srv, _ := newTestServer(t)
		seed(t, srv)

		srv.SweepRentFiles(ctx)

		for _, name := range []string{"Ancient", "Crashed"} {
			if _, err := srv.objects.LoadObjects(ctx, name); err != nil {
				t.Errorf("%s's rent file was swept while rent is free (%v); nothing "+
					"expires when nothing is charged", name, err)
			}
		}
	})

	t.Run("charging rent restores the C's behaviour", func(t *testing.T) {
		srv, _ := newTestServer(t)
		chargeRent(t)
		seed(t, srv)

		srv.SweepRentFiles(ctx)

		for _, name := range []string{"Ancient", "Crashed"} {
			if _, err := srv.objects.LoadObjects(ctx, name); err == nil {
				t.Errorf("%s's rent file survived a sweep with rent charging on: "+
					"update_obj_file deletes it and so should this", name)
			}
		}
	})
}
