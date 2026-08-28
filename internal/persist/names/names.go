// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

// Package names reads and writes the disallowed-name list, porting
// Read_Invalid_List (ban.c:301) and its file, misc/xnames (db.h:91's
// XNAME_FILE).
//
// Unlike boards/mail/houses/bans this is read-only game data: nothing in
// the C ever writes xnames at runtime, only Valid_Name (ban.c:255)
// consults it. So there is no Store interface with Add/Remove methods to
// design here, only a list to load — and, for `dlctl import --type=names`/`fmt`,
// to write back out in whichever format was asked for. Small enough that
// classic and yaml both live in this one package rather than getting
// their own subpackages and a Register/Open registry the way
// boards/mail/houses/bans did: there is exactly one shape of data (a list
// of strings) and no runtime mutation to plug in behind an interface.
package names

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/goccy/go-yaml"

	"github.com/gerrowadat/disgracelands/internal/persist/atomicfile"
	"github.com/gerrowadat/disgracelands/internal/persist/yamlenc"
)

// ClassicFile is misc/xnames under whatever directory holds it.
const ClassicFile = "xnames"

// YamlFile is config/names.yaml under whatever directory Load/Save's
// path names, per docs/design/data-format.md §9.
const YamlFile = "names.yaml"

// namesSchema is the document's schema tag (data-format.md §10.1).
const namesSchema = "dl/names@1"

type doc struct {
	Schema     string   `yaml:"schema"`
	Disallowed []string `yaml:"disallowed,omitempty"`
}

// Load reads the disallowed-name list in the given format ("classic" or
// "yaml", "" meaning classic). For classic, path is the file itself
// (.../misc/xnames); for yaml, path is the directory config/ lives
// under — the same classic-is-a-file/yaml-is-a-directory asymmetry
// bans/boards/mail/houses already have, because a yaml document always
// carries a schema tag and a yaml "file" is really "a directory holding
// one or more named documents".
//
// A missing file is not an error: nobody has configured a list, so
// nothing is disallowed — matching Read_Invalid_List's own posture
// (ban.c:301-318: a failed fopen logs and returns, leaving num_invalid at
// 0, which Valid_Name treats as "everything is fine", ban.c:263-264).
func Load(format, path string) ([]string, error) {
	switch format {
	case "", "classic":
		return loadClassic(path)
	case "yaml":
		return loadYaml(path)
	default:
		return nil, fmt.Errorf("names: unknown format %q", format)
	}
}

// Save writes the disallowed-name list in the given format. Only yaml
// is implemented: the C never writes xnames at runtime, and building a
// classic writer nothing needs yet would be exactly the "format before
// the feature" mistake this whole step's plan is careful to avoid — this
// exists for `dlctl import --type=names` (classic to yaml) and
// `fmt --type=names` (yaml, canonicalised), neither of which writes
// classic.
func Save(format, path string, list []string) error {
	if format != "yaml" {
		return fmt.Errorf("names: writing %q is not supported (only yaml)", format)
	}
	return saveYaml(path, list)
}

func loadClassic(path string) ([]string, error) {
	f, err := os.Open(path) //nolint:gosec // operator-configured path
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	var list []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		// get_line (utils.c:496-514) skips blank lines and lines starting
		// with '*' — comments — before returning one.
		if line == "" || strings.HasPrefix(line, "*") {
			continue
		}
		list = append(list, line)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return list, nil
}

func loadYaml(dir string) ([]string, error) {
	path := filepath.Join(dir, YamlFile)
	b, err := os.ReadFile(path) //nolint:gosec // operator-configured path
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var d doc
	if err := yaml.UnmarshalWithOptions(b, &d, yaml.Strict()); err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return d.Disallowed, nil
}

func saveYaml(dir string, list []string) error {
	path := filepath.Join(dir, YamlFile)
	d := doc{Schema: namesSchema, Disallowed: list}
	out, err := yaml.MarshalWithOptions(d, yamlenc.Options()...)
	if err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	if err := atomicfile.Write(path, out, 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}
