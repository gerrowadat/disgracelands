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

// TestSubsystemDir pins every subsystem/format pair cmd/dlmud and cmd/dlctl
// both rely on Dir for, so a future edit that reshuffles one of them fails
// here rather than as a live-server boot failure or a silent dlctl miss.
func TestSubsystemDir(t *testing.T) {
	base := filepath.FromSlash("/srv/dl")
	join := func(parts ...string) string { return filepath.Join(append([]string{base}, parts...)...) }

	cases := []struct {
		name   string
		s      Subsystem
		format string
		want   string
	}{
		{"world classic", SubsystemWorld, "classic", join("world")},
		{"world yaml", SubsystemWorld, "yaml", join("world")},
		{"players binary", SubsystemPlayers, "binary", join("etc")},
		{"players ascii", SubsystemPlayers, "ascii", join("pfiles")},
		{"players yaml", SubsystemPlayers, "yaml", join("players")},
		{"state classic", SubsystemState, "classic", join("etc")},
		{"state yaml", SubsystemState, "yaml", join("state")},
		{"house objects classic", SubsystemHouseObjects, "classic", join("house")},
		{"house objects yaml", SubsystemHouseObjects, "yaml", join("state")},
		{"reports classic", SubsystemReports, "classic", join("misc")},
		{"reports yaml", SubsystemReports, "yaml", join("state")},
		{"names classic", SubsystemNames, "classic", join("misc")},
		{"names yaml", SubsystemNames, "yaml", join("config")},
		{"messages classic", SubsystemMessages, "classic", join("misc")},
		{"messages yaml", SubsystemMessages, "yaml", join("config")},
		{"socials classic", SubsystemSocials, "classic", join("misc")},
		{"socials yaml", SubsystemSocials, "yaml", join("config")},
		{"help classic", SubsystemHelp, "classic", join("text", "help")},
		{"help yaml", SubsystemHelp, "yaml", join("text", "help")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Dir(base, tc.s, tc.format); got != tc.want {
				t.Errorf("Dir(%q, %v, %q) = %q, want %q", base, tc.s, tc.format, got, tc.want)
			}
		})
	}
}
