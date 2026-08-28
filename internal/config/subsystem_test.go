// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package config

import (
	"path/filepath"
	"testing"
)

// TestSubsystemDir pins every subsystem cmd/dlmud relies on Dir for, so a
// future edit that reshuffles one fails here rather than as a live-server
// boot failure.
//
// It used to pin nineteen subsystem/format *pairs*, because Dir answered
// differently depending on whether the answer was 2002 or now. There is
// one layout now (docs/proposals/yaml-only.md §1); the legacy half of the
// table moved to cmd/dlctl, along with the code, and is pinned there.
func TestSubsystemDir(t *testing.T) {
	base := filepath.FromSlash("/srv/dl")
	join := func(parts ...string) string { return filepath.Join(append([]string{base}, parts...)...) }

	cases := []struct {
		name string
		s    Subsystem
		want string
	}{
		{"world", SubsystemWorld, join("world")},
		{"players", SubsystemPlayers, join("players")},
		{"state", SubsystemState, join("state")},
		{"names", SubsystemNames, join("config")},
		{"messages", SubsystemMessages, join("config")},
		{"socials", SubsystemSocials, join("config")},
		{"help", SubsystemHelp, join("text", "help")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Dir(base, tc.s); got != tc.want {
				t.Errorf("Dir(%q, %v) = %q, want %q", base, tc.s, got, tc.want)
			}
		})
	}
}
