// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import "testing"

// The encoding of the announcement level, which is the part with a trap in
// it: the two bits count *suppression*, so that a record written before the
// setting existed — every record ever written, on the day it shipped — reads
// as AnnounceAll rather than as silence.

// The property everything else rests on.
func TestAZeroedRecordHearsEverything(t *testing.T) {
	if got := AnnounceLevelOf(&PlayerRecord{}); got != AnnounceAll {
		t.Errorf("a record with no preference bits set is %v, want %v", got, AnnounceAll)
	}
	if got := AnnounceLevelOf(nil); got != AnnounceAll {
		t.Errorf("a nil record is %v, want %v", got, AnnounceAll)
	}
}

func TestAnnounceLevelRoundTrips(t *testing.T) {
	for _, level := range []AnnounceLevel{AnnounceOff, AnnounceBrief, AnnounceAll} {
		rec := &PlayerRecord{}
		SetAnnounceLevel(rec, level)
		if got := AnnounceLevelOf(rec); got != level {
			t.Errorf("set %v, read back %v", level, got)
		}
		// Nothing outside the two bits is touched, because this shares a
		// word with twenty-three other settings.
		if !rec.Preferences.Without(PrefNoAnnounce1, PrefNoAnnounce2).Empty() {
			t.Errorf("setting %v disturbed other preferences: %#x", level, rec.Preferences.Raw())
		}
	}
}

// Three is unreachable through `announce` — the command only ever writes 0, 1
// or 2 — but a hand-edited pfile can hold it, and it should answer quietly
// rather than with a fourth state nothing names.
func TestBothSuppressionBitsIsOff(t *testing.T) {
	rec := &PlayerRecord{Preferences: NewSet(PrefNoAnnounce1, PrefNoAnnounce2)}
	if got := AnnounceLevelOf(rec); got != AnnounceOff {
		t.Errorf("both bits set reads as %v, want %v", got, AnnounceOff)
	}
}

func TestWhichStreamsEachLevelHears(t *testing.T) {
	for _, tc := range []struct {
		level         AnnounceLevel
		rare, routine bool
	}{
		{AnnounceAll, true, true},
		{AnnounceBrief, true, false},
		{AnnounceOff, false, false},
	} {
		rec := &PlayerRecord{}
		SetAnnounceLevel(rec, tc.level)
		if got := AnnouncementRare.Hears(rec); got != tc.rare {
			t.Errorf("%v hears rare = %v, want %v", tc.level, got, tc.rare)
		}
		if got := AnnouncementRoutine.Hears(rec); got != tc.routine {
			t.Errorf("%v hears routine = %v, want %v", tc.level, got, tc.routine)
		}
	}
}

// Prefix matching, as `color` and `syslog` do it.
func TestParseAnnounceLevel(t *testing.T) {
	for word, want := range map[string]AnnounceLevel{
		"o": AnnounceOff, "off": AnnounceOff,
		"b": AnnounceBrief, "brief": AnnounceBrief,
		"a": AnnounceAll, "all": AnnounceAll,
		"ALL": AnnounceAll,
	} {
		got, ok := ParseAnnounceLevel(word)
		if !ok || got != want {
			t.Errorf("%q is (%v, %v), want (%v, true)", word, got, ok, want)
		}
	}
	for _, word := range []string{"", "x", "sideways", "offside"} {
		if _, ok := ParseAnnounceLevel(word); ok {
			t.Errorf("%q matched a level and should not have", word)
		}
	}
}
