// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import (
	"testing"
	"time"
)

// Seconds is mud_time_to_secs' summation half: it reconstructs the real
// seconds a MudTime's own components represent, discarding whatever
// TimePassed itself already discarded — so an elapsed duration round-tripped
// through TimePassed and back through Seconds loses its remainder within
// the current mud hour, not any of the coarser components.
func TestMudTimeSecondsRoundTripsUpToTheCurrentHour(t *testing.T) {
	elapsed := 3*int64(SecondsPerMudYear) + 5*int64(SecondsPerMudMonth) +
		9*int64(SecondsPerMudDay) + 4*int64(SecondsPerMudHour) + 37

	mt := TimePassed(time.Duration(elapsed) * time.Second)
	got := mt.Seconds()
	want := elapsed - 37 // the 37 leftover seconds within the current hour are gone
	if got != want {
		t.Errorf("Seconds() = %d, want %d (elapsed %d minus its hour remainder)", got, want, elapsed)
	}
}

func TestMudTimeSecondsOfZeroIsZero(t *testing.T) {
	if got := (MudTime{}).Seconds(); got != 0 {
		t.Errorf("Seconds() of a zero MudTime = %d, want 0", got)
	}
}

// SavedEpoch is Live's half of save_mud_time/mud_time_to_secs: an epoch
// that, fed back into TimePassed(time.Since(epoch)) at the same instant,
// reproduces the current MudTime exactly.
func TestSavedEpochReproducesTheCurrentMudTime(t *testing.T) {
	l := &Live{}
	epoch := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	l.SetBooted(epoch)

	now := epoch.Add(time.Duration(SecondsPerMudHour*3+40) * time.Second)
	before := TimePassed(now.Sub(epoch))

	saved := l.SavedEpoch(now)
	after := TimePassed(now.Sub(saved))

	if after != before {
		t.Errorf("MudTime after round-tripping through SavedEpoch = %+v, want %+v", after, before)
	}
	// The saved epoch is later than the original: the 40 leftover seconds
	// within the current mud hour were dropped, which can only move the
	// epoch forward.
	if !saved.After(epoch) {
		t.Errorf("SavedEpoch = %v, want strictly after the original epoch %v", saved, epoch)
	}
	if drift := saved.Sub(epoch); drift <= 0 || drift >= time.Duration(SecondsPerMudHour)*time.Second {
		t.Errorf("drift = %v, want in (0, %ds]", drift, SecondsPerMudHour)
	}
}

func TestSetBootedChangesMudTime(t *testing.T) {
	l := &Live{}
	l.SetBooted(time.Now())
	if hours := l.MudTime().Hours; hours < 0 || hours > 23 {
		t.Fatalf("sanity: Hours out of range: %d", hours)
	}

	// Setting the epoch a whole mud year in the past should read back as
	// (at least) one mud year of age.
	l.SetBooted(time.Now().Add(-time.Duration(SecondsPerMudYear) * time.Second))
	if got := l.MudTime().Year; got < 1 {
		t.Errorf("MudTime().Year = %d after a year-old epoch, want >= 1", got)
	}
}
