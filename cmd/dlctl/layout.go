// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gerrowadat/disgracelands/internal/config"
	"github.com/gerrowadat/disgracelands/internal/persist/player/ascii"
	"github.com/gerrowadat/disgracelands/internal/persist/player/binary"
)

// dirType is one of dlctl's own --type values: which subsystem a verb
// operates on. Every verb below that takes --type shares this one set,
// rather than each subcommand group inventing its own vocabulary.
type dirType string

const (
	typeWorld    dirType = "world"
	typePfile    dirType = "pfile"
	typeState    dirType = "state"
	typeNames    dirType = "names"
	typeMessages dirType = "messages"
	typeSocials  dirType = "socials"
	typeHelp     dirType = "help"
	// typeCopied is not a subsystem and has no importer: it is the files
	// `dlctl import` copies rather than converts (text/'s prose,
	// config/game.yaml, text/help/screen), which `verify --against`
	// compares as bytes. See copied.go. It is deliberately not in
	// allTypes — nothing imports it, because copying is what import does
	// with it — and verify adds it to its own list.
	typeCopied dirType = "copied"
)

// allTypes lists every --type value, in the order usage/error text shows
// them — the same order internal/config/subsystem.go declares them.
var allTypes = []dirType{typeWorld, typePfile, typeState, typeNames, typeMessages, typeSocials, typeHelp}

func joinTypes(types []dirType) string {
	names := make([]string, len(types))
	for i, t := range types {
		names[i] = string(t)
	}
	return strings.Join(names, ", ")
}

// parseType validates a --type flag's value against the verb's own allowed
// set — not every verb supports every type yet (lint/verify/passwd are
// pfile- or world-only today) — and requires it be set at all.
func parseType(raw string, allowed []dirType) (dirType, error) {
	if raw == "" {
		return "", fmt.Errorf("--type is required (have: %s)", joinTypes(allowed))
	}
	t := dirType(raw)
	for _, a := range allowed {
		if a == t {
			return t, nil
		}
	}
	return "", fmt.Errorf("--type: unsupported value %q (have: %s)", raw, joinTypes(allowed))
}

// subsystem maps a --type onto the internal/config.Subsystem its yaml-side
// directory resolves through.
func subsystem(t dirType) (config.Subsystem, error) {
	switch t {
	case typeWorld:
		return config.SubsystemWorld, nil
	case typePfile:
		return config.SubsystemPlayers, nil
	case typeState:
		return config.SubsystemState, nil
	case typeNames:
		return config.SubsystemNames, nil
	case typeMessages:
		return config.SubsystemMessages, nil
	case typeSocials:
		return config.SubsystemSocials, nil
	case typeHelp:
		return config.SubsystemHelp, nil
	default:
		return 0, fmt.Errorf("unknown --type %q", t)
	}
}

// classicDirs is where each subsystem lives in a *legacy* lib directory.
//
// internal/config.Dir used to carry both layouts and pick between them from
// a format name. It carries one now — the server reads one format
// (docs/design/yaml-only.md §1) — and this is the other half, moved to
// the one command that still has to find it. Reading the old layout is
// `dlctl`'s whole job; it is no longer anybody else's.
//
// Note state's three separate homes: the C spreads the clock, boards, mail,
// bans and the house control file under etc/, house objects under house/,
// and the bug/idea/typo reports under misc/, where yaml collects all three
// into state/. classicStateDirs is what resolves that three-way split.
var classicDirs = map[dirType]string{
	typeWorld:    "world",
	typeState:    "etc",
	typeNames:    "misc",
	typeMessages: "misc",
	typeSocials:  "misc",
	// Both formats keep the help database in text/help/ and are told
	// apart by which files are in it, not by which directory it is.
	typeHelp: filepath.Join("text", "help"),
}

// classicPlayerDirs is the roster's own two legacy homes, which is the one
// subsystem where the pre-yaml answer is itself two answers: binary is the
// C's own `etc/players`, and ascii is this port's own addition and never
// shared etc/ with anything else.
var classicPlayerDirs = map[string]string{
	"binary": "etc",
	"ascii":  "pfiles",
}

// classicPlayerMarkers is the file whose presence proves a roster of that
// format is really there: the C's own `etc/players` database, and this
// port's `pfiles/plr_index`. Used by detectPlayerFormat, which is what
// stops the one-pass import silently converting nobody (#314).
var classicPlayerMarkers = map[string]string{
	"binary": filepath.Join("etc", binary.FileName),
	"ascii":  filepath.Join("pfiles", ascii.IndexFile),
}

// detectPlayerFormat works out which legacy roster a source directory
// holds, for the one-pass import where no --from-format was given.
//
// The roster is the only subsystem with two legacy source formats, and the
// two live in different directories with differently-named index files, so
// the layout answers the question on its own — there is nothing to guess
// at. What there *was*, before #314, was a default of binary applied
// regardless: an ascii tree has no etc/players, so the importer opened an
// empty roster, converted nobody, reported "imported 0 character(s)" and
// exited 0, with the verify pass agreeing that an empty roster matched an
// empty roster.
//
// found is false when there is no roster of either kind, which is not an
// error and is a real, checked-in case: examples/stock/binary is a fresh
// stock install with nobody in it, and examples/mini/binary has no etc/ at
// all. The caller falls back to the historical default and imports
// nothing — but says out loud that it found no roster, which is the part
// that was missing. A conversion that quietly drops every player is worth
// one line of output.
//
// Both present is an error, because picking one would be picking which
// half of somebody's players to lose.
func detectPlayerFormat(base string) (format string, found bool, err error) {
	var seen []string
	for _, f := range sortedKeysOf(classicPlayerMarkers) {
		if _, err := os.Stat(filepath.Join(base, classicPlayerMarkers[f])); err == nil {
			seen = append(seen, f)
		}
	}

	switch len(seen) {
	case 1:
		return seen[0], true, nil
	case 0:
		return binary.FormatName, false, nil
	default:
		return "", false, fmt.Errorf(
			"%s holds both a %s and a %s roster; pass --from-format=binary or "+
				"--from-format=ascii to say which one to import",
			base, classicPlayerMarkers["binary"], classicPlayerMarkers["ascii"])
	}
}

// playerRosterMarkers is the two paths detectPlayerFormat looks for, for a
// caller that wants to name them in a message.
func playerRosterMarkers() (binaryPath, asciiPath string) {
	return classicPlayerMarkers["binary"], classicPlayerMarkers["ascii"]
}

// isPlayerFormat reports whether a format name is a legacy roster format.
func isPlayerFormat(format string) bool {
	_, ok := classicPlayerDirs[format]
	return ok
}

// formatForType decides which subsystem one --from-format/--format flag is
// actually about.
//
// There is a single flag and seven subsystems, and a format name says
// which of the two families it belongs to: `binary` and `ascii` are roster
// formats and mean nothing to the other six, `classic` is the other six's
// and means nothing to the roster. So the flag applies to whichever family
// it names, and is blank for the rest — leaving them to their own
// defaults, which for the roster means detectPlayerFormat reading the
// layout.
//
// Before #314 this rule existed nowhere. cmdImportAll blanked the flag for
// every type including the one it was for, so the documented
// `--from-format=ascii` was accepted and discarded; verifyImport did the
// opposite, handing "ascii" to every type including world, so the same
// command's verify pass then failed to read the world. One rule in one
// place, used by both.
func formatForType(t dirType, explicit string) string {
	// yaml is not in either family: it is every subsystem's format, and is
	// what the destination side of a verify is named with. Blanking it
	// sends the comparison off to detectPlayerFormat, which finds no
	// legacy roster in a yaml directory and falls back to binary — so the
	// converted players read as an empty binary roster and every one of
	// them is reported missing. Caught by TestImportConvertsEndToEnd on
	// examples/torture, the one fixture with a roster in it.
	if explicit == "" || explicit == "yaml" {
		return explicit
	}
	if isPlayerFormat(explicit) != (t == typePfile) {
		return ""
	}
	return explicit
}

// classicFilenames names the file inside the classic-side directory a type
// resolves to, for the three types whose classic source is one file rather
// than a directory (misc/xnames, misc/messages, misc/socials).
var classicFilenames = map[dirType]string{
	typeNames:    "xnames",
	typeMessages: "messages",
	typeSocials:  "socials",
}

// resolveDir resolves a --type's directory (or, for names/messages/socials
// read from a non-yaml source, file) under a base directory, for the given
// format. This is dlctl's own half of the base-dir model: given "point me
// at the archive root", it works out which subdirectory (or file) that
// type actually lives in, the same way internal/config.Dir already does
// for cmd/dlmud's --lib-dir.
func resolveDir(t dirType, base, format string) (string, error) {
	if format == "yaml" {
		s, err := subsystem(t)
		if err != nil {
			return "", err
		}
		return config.Dir(base, s), nil
	}

	if t == typePfile {
		sub, ok := classicPlayerDirs[format]
		if !ok {
			return "", fmt.Errorf("unknown roster format %q (have: %s, yaml)",
				format, strings.Join(sortedKeysOf(classicPlayerDirs), ", "))
		}
		return filepath.Join(base, sub), nil
	}

	sub, ok := classicDirs[t]
	if !ok {
		return "", fmt.Errorf("unknown --type %q", t)
	}
	dir := filepath.Join(base, sub)
	if name, ok := classicFilenames[t]; ok {
		return filepath.Join(dir, name), nil
	}
	return dir, nil
}

func sortedKeysOf(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// stateClassicDirs resolves state's three separate classic-side source
// directories (etc/, house/, misc/) under one base — the C spreads
// clock/boards/mail/bans/hcontrol, house objects and reports across three
// directories that yaml collects into one state/. --from-house-dir/
// --from-misc-dir (cmdImportState) override these for a rearranged
// archive; this is only their default.
func stateClassicDirs(base string) (etcDir, houseDir, miscDir string) {
	return filepath.Join(base, "etc"), filepath.Join(base, "house"), filepath.Join(base, "misc")
}
