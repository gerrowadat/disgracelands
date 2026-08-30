// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package main

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gerrowadat/disgracelands/internal/config"
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
