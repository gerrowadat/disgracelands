// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package main

import (
	"fmt"
	"path/filepath"
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

// subsystem maps a --type onto the internal/config.Subsystem its single
// (non-state) or "primary" (state's yaml side) directory resolves through.
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
	s, err := subsystem(t)
	if err != nil {
		return "", err
	}
	dir := config.Dir(base, s, format)
	if format != "yaml" {
		if name, ok := classicFilenames[t]; ok {
			return filepath.Join(dir, name), nil
		}
	}
	return dir, nil
}

// stateClassicDirs resolves state's three separate classic-side source
// directories (etc/, house/, misc/) under one base — the C spreads
// clock/boards/mail/bans/hcontrol, house objects and reports across three
// directories that yaml collects into one state/. --from-house-dir/
// --from-misc-dir (cmdImportState) override these for a rearranged
// archive; this is only their default.
func stateClassicDirs(base string) (etcDir, houseDir, miscDir string) {
	return config.Dir(base, config.SubsystemState, "classic"),
		config.Dir(base, config.SubsystemHouseObjects, "classic"),
		config.Dir(base, config.SubsystemReports, "classic")
}
