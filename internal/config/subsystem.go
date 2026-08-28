// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package config

import "path/filepath"

// Subsystem is one directory or file that both the live server (cmd/dlmud,
// via LibDir) and the offline tooling (cmd/dlctl, via its own --from-dir/
// --to-dir/--dir base directories) need to find under a lib directory.
// Before this existed, both derived the same answer independently — dlmud
// inline in main.go, dlctl once per import/fmt/lint/dump/verify/passwd
// subcommand — which is how PlayerPath (below) went years answering "where
// are the players" with the ascii location even when asked about yaml.
//
// State's three classic-side homes are three separate values: the C
// spreads clock/boards/mail/bans/the house control file under etc/, house
// objects under house/, and the bug/idea/typo reports under misc/, even
// though yaml collects all three under one state/ directory — matching
// dlctl's own single --type=state, which resolves whichever of these it
// needs for the format actually in play.
type Subsystem int

const (
	SubsystemWorld Subsystem = iota
	SubsystemPlayers
	// SubsystemState covers the mud clock, the boards, the mail file, the
	// site ban list and the house control file: classic etc/, yaml state/.
	SubsystemState
	// SubsystemHouseObjects covers the per-house object files: classic
	// house/, yaml state/ (folded in alongside everything else
	// SubsystemState covers there).
	SubsystemHouseObjects
	// SubsystemReports covers the bug/idea/typo log: classic misc/, yaml
	// state/.
	SubsystemReports
	// SubsystemNames covers the disallowed-name list: classic misc/, yaml
	// config/.
	SubsystemNames
	// SubsystemMessages covers the skill_message/dam_message table:
	// classic misc/, yaml config/.
	SubsystemMessages
	// SubsystemSocials covers the do_action table: classic misc/, yaml
	// config/.
	SubsystemSocials
	// SubsystemHelp covers the help database: text/help/ under either
	// format, distinguished by which files are present rather than by
	// directory.
	SubsystemHelp
)

// Dir resolves a subsystem's directory under a lib directory base, given
// the format governing it. This is the one place that knows a classic lib/
// and a yaml one lay a lib directory out differently; cmd/dlmud and
// cmd/dlctl both call it rather than each hand-deriving the same join.
//
// format is "classic" or "yaml" for every subsystem except SubsystemPlayers,
// which also accepts "binary" (the original playerfile format, whose
// records live in etc/ — the only subsystem where the pre-yaml answer
// itself has two different homes, because ascii is this port's own
// addition and never shared etc/ with anything else).
func Dir(libDir string, s Subsystem, format string) string {
	yaml := format == "yaml"
	switch s {
	case SubsystemWorld:
		// Same subpath either way: examples/stock/binary/world and
		// examples/stock/yaml/world sit at the same relative place under
		// their own base, and nothing about the world format changes that.
		return filepath.Join(libDir, "world")
	case SubsystemPlayers:
		switch format {
		case "yaml":
			return filepath.Join(libDir, "players")
		case "binary":
			return filepath.Join(libDir, "etc")
		default: // "ascii", and anything else classic-shaped
			return filepath.Join(libDir, "pfiles")
		}
	case SubsystemState:
		if yaml {
			return filepath.Join(libDir, "state")
		}
		return filepath.Join(libDir, "etc")
	case SubsystemHouseObjects:
		if yaml {
			return filepath.Join(libDir, "state")
		}
		return filepath.Join(libDir, "house")
	case SubsystemReports:
		if yaml {
			return filepath.Join(libDir, "state")
		}
		return filepath.Join(libDir, "misc")
	case SubsystemNames, SubsystemMessages, SubsystemSocials:
		if yaml {
			return filepath.Join(libDir, "config")
		}
		return filepath.Join(libDir, "misc")
	case SubsystemHelp:
		return filepath.Join(libDir, "text", "help")
	default:
		return libDir
	}
}
