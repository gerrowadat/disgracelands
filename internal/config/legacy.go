// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// legacyMarkers are files that exist only in a CircleMUD `lib/` directory,
// listed with what each one is, so a refusal can say which one it found.
//
// Detection is on these rather than on the *absence* of yaml, which is the
// whole of the difference between a useful message and a confusing one: a
// genuinely empty directory, or one where somebody mistyped `--lib-dir`,
// gets the ordinary "no world data" error it has always got, and only a
// directory that really is an archive gets told to convert it
// (docs/design/yaml-only.md §3.3).
var legacyMarkers = []struct {
	path string
	what string
}{
	{filepath.Join("world", "zone.lst"), "the C server's zone list"},
	{filepath.Join("world", "wld", "index"), "a classic world index"},
	{filepath.Join("etc", "players"), "a binary player database"},
	{filepath.Join("etc", "plrmail"), "a classic mail file"},
	{filepath.Join("etc", "badsites"), "a classic ban list"},
	{filepath.Join("misc", "socials"), "a classic socials table"},
	{filepath.Join("misc", "messages"), "a classic damage-message table"},
	{"pfiles", "an ascii roster directory"},
}

// CheckNotLegacy refuses to boot on a directory that is a legacy CircleMUD
// lib/ rather than a converted one, and says what to run instead.
//
// The operator's archive is never written to and no conversion happens
// behind their back: the converted directory is somewhere they chose
// (§0's fourth decision). An in-place upgrade would be friendlier for
// about a minute and then be the thing that ate somebody's only copy of a
// 2008 roster.
//
// A directory that has *both* — yaml world files and legacy markers — is
// accepted, because that is what a conversion into an existing tree looks
// like halfway through, and because the yaml files are what the server
// will actually read. The check is for the case where there is nothing
// else to read.
func CheckNotLegacy(libDir string) error {
	if hasYamlWorld(libDir) {
		return nil
	}

	var found []string
	for _, m := range legacyMarkers {
		if _, err := os.Stat(filepath.Join(libDir, m.path)); err == nil {
			found = append(found, fmt.Sprintf("%s (%s)", m.path, m.what))
		}
	}
	if len(found) == 0 {
		return nil
	}

	return fmt.Errorf("%s is a CircleMUD lib/ directory, not a Disgracelands data directory: it has %s. "+
		"This server reads one on-disk format; convert it once (the source is not written to) and point "+
		"--lib-dir at the result — see docs/operations.md:\n"+
		"    dlctl import --from-dir=%s --to-dir=<somewhere>",
		libDir, strings.Join(found, ", "), libDir)
}

// hasYamlWorld reports whether libDir holds a converted world — the
// zones.yaml manifest, or any zone file beside it.
func hasYamlWorld(libDir string) bool {
	worldDir := Dir(libDir, SubsystemWorld)
	if _, err := os.Stat(filepath.Join(worldDir, "zones.yaml")); err == nil {
		return true
	}
	entries, err := os.ReadDir(worldDir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".yaml") {
			return true
		}
	}
	return false
}
