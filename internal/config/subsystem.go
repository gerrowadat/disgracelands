// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package config

import "path/filepath"

// Subsystem is one directory under a lib directory that the server needs to
// find.
//
// Before this existed, cmd/dlmud and cmd/dlctl each derived the same answers
// independently — dlmud inline in main.go, dlctl once per subcommand — which
// is how PlayerPath went years answering "where are the players" with the
// ascii location even when asked about yaml.
type Subsystem int

const (
	SubsystemWorld Subsystem = iota
	// SubsystemPlayers is one file per character.
	SubsystemPlayers
	// SubsystemState covers the mud clock, the boards, the mail, the site
	// ban list, the house control file and its contents, and the
	// bug/idea/typo reports — all of them in state/.
	SubsystemState
	// SubsystemNames, SubsystemMessages and SubsystemSocials are the three
	// files under config/: the disallowed-name list, the
	// skill_message/dam_message table and the do_action table.
	SubsystemNames
	SubsystemMessages
	SubsystemSocials
	// SubsystemHelp is the help database, under text/help/.
	SubsystemHelp
)

// Dir resolves a subsystem's directory under a lib directory base.
//
// It used to take a third parameter — the format governing the subsystem —
// and most of its body was the two layouts that answered to: players in
// `players/` or `etc/` or `pfiles/`, state in `state/` or `etc/`, house
// objects in `state/` or `house/`, reports in `state/` or `misc/`, names
// and messages and socials in `config/` or `misc/`. docs/proposals/
// yaml-only.md §1 names that function as the clearest single piece of
// evidence that the legacy formats were load-bearing where they had no
// business being: nine subsystems, each answering "where do I live"
// depending on whether the answer was 2002 or now.
//
// There is one layout now, so there is one answer per subsystem, and the
// three separate classic homes that state/ collected (etc/, house/, misc/)
// collapse into one constant rather than three.
//
// cmd/dlctl still has to find both layouts, because reading the old one is
// its whole job — its own resolveDir keeps that knowledge, where it
// belongs.
func Dir(libDir string, s Subsystem) string {
	switch s {
	case SubsystemWorld:
		return filepath.Join(libDir, "world")
	case SubsystemPlayers:
		return filepath.Join(libDir, "players")
	case SubsystemState:
		return filepath.Join(libDir, "state")
	case SubsystemNames, SubsystemMessages, SubsystemSocials:
		return filepath.Join(libDir, "config")
	case SubsystemHelp:
		return filepath.Join(libDir, "text", "help")
	default:
		return libDir
	}
}
