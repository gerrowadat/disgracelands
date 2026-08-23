// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

// Package help reads and writes the do_help table: text/help/index plus
// the .hlp files it lists (db.c's index_boot/load_help) for classic, and
// docs/design/data-format.md §7's one-file-per-entry
// text/help/help.yaml for yaml.
//
// Like internal/persist/messages/names/socials, this is read-only game
// data at runtime — nothing in the C ever writes the help database while
// playing — so there is no Store interface with a runtime-mutation
// method to design, only a list to load and (for `dlctl help
// import`/`fmt`) to write back out.
//
// Classic and yaml share the same directory, unlike messages/socials'
// file-vs-directory split: classic is already multi-file (index plus
// several .hlp files) and the doc's own yaml path
// (data/text/help/help.yaml) sits beside them. The two formats never
// read each other's files — classic only opens index and what it lists,
// yaml only opens help.yaml and what it lists — so they can coexist in
// one directory without conflict.
package help

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/goccy/go-yaml"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// IndexFile is text/help/index under whatever directory Load/Save's dir
// names, for classic.
const IndexFile = "index"

// YamlFile is help.yaml under the same directory, for yaml.
const YamlFile = "help.yaml"

// helpSchema is the document's schema tag (data-format.md §10.1).
const helpSchema = "dl/help@1"

type doc struct {
	Schema  string     `yaml:"schema"`
	Entries []entryDoc `yaml:"entries,omitempty"`
}

type entryDoc struct {
	// Keywords is declared independently of the entry's own file, not
	// derived from it — the same decoupling data-format.md §3 gives
	// world/zones.yaml relative to the zone files it lists ("filenames
	// are conveniences... nothing resolves a record by filename"), here
	// applied to an entry's *content* rather than its filename.
	Keywords []string `yaml:"keywords"`
	// File is the entry's body, relative to dir — plain UTF-8 text (§7),
	// one entry, one file. Its first line is sent to the player verbatim
	// by do_help (act.informative.c's page_string call on the whole
	// entry), so it is stored and restored exactly as written, not
	// resynthesised from Keywords.
	File string `yaml:"file"`
}

// Load reads the help table in the given format ("classic" or "yaml",
// "" meaning classic). dir is text/help itself in both cases.
//
// A missing index (classic) or help.yaml (yaml) is not an error: a
// server with no help data is a poorer game, not a broken one — the
// same posture internal/server/text.go already took when it loaded
// these files directly, which this package now does on its behalf. A
// file the index/help.yaml *lists* but that does not exist is a hard
// error in both formats, matching load_help's own fatal C exit for a
// missing .hlp file.
func Load(format, dir string) ([]game.HelpEntry, error) {
	switch format {
	case "", "classic":
		return loadClassic(dir)
	case "yaml":
		return loadYaml(dir)
	default:
		return nil, fmt.Errorf("help: unknown format %q", format)
	}
}

// Save writes the help table in the given format. Only yaml is
// implemented, for the same reason internal/persist/messages.Save only
// writes yaml: the C never writes the help database at runtime, so a
// classic writer has nothing to serve except `dlctl`, which only ever
// needs to go classic to yaml, never back.
func Save(format, dir string, entries []game.HelpEntry) error {
	if format != "yaml" {
		return fmt.Errorf("help: writing %q is not supported (only yaml)", format)
	}
	return saveYaml(dir, entries)
}

func loadClassic(dir string) ([]game.HelpEntry, error) {
	indexPath := filepath.Join(dir, IndexFile)
	f, err := os.Open(indexPath) //nolint:gosec // operator-configured path
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", indexPath, err)
	}
	files, err := game.ParseHelpIndex(f)
	_ = f.Close()
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", indexPath, err)
	}

	var entries []game.HelpEntry
	for _, name := range files {
		path := filepath.Join(dir, name)
		hf, err := os.Open(path) //nolint:gosec // as above
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", path, err)
		}
		fileEntries, err := game.ParseHelpFile(hf)
		_ = hf.Close()
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", path, err)
		}
		entries = append(entries, fileEntries...)
	}
	return entries, nil
}

func loadYaml(dir string) ([]game.HelpEntry, error) {
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

	entries := make([]game.HelpEntry, 0, len(d.Entries))
	for _, ed := range d.Entries {
		entryPath := filepath.Join(dir, ed.File)
		body, err := os.ReadFile(entryPath) //nolint:gosec // as above
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", entryPath, err)
		}
		entries = append(entries, game.HelpEntry{
			Keywords: ed.Keywords,
			Body:     lfToBody(string(body)),
		})
	}
	return entries, nil
}

func saveYaml(dir string, entries []game.HelpEntry) error {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("writing %s: %w", dir, err)
	}

	d := doc{Schema: helpSchema, Entries: make([]entryDoc, 0, len(entries))}
	used := map[string]int{}
	for _, e := range entries {
		slug := game.HelpSlug(e.Keywords)
		if slug == "" {
			slug = fmt.Sprintf("entry-%d", len(d.Entries)+1)
		}
		// Belt on top of braces: the real archive needs no
		// disambiguation (checked, not assumed — see
		// TestHelpSlugsAreUniqueAgainstTheRealArchive), but a writer
		// that cannot survive two entries slugging to the same line
		// is not a format.
		used[slug]++
		file := slug + ".txt"
		if n := used[slug]; n > 1 {
			file = fmt.Sprintf("%s-%d.txt", slug, n)
		}

		if err := os.WriteFile(filepath.Join(dir, file), []byte(bodyToLF(e.Body)), 0o600); err != nil {
			return fmt.Errorf("writing %s: %w", file, err)
		}
		d.Entries = append(d.Entries, entryDoc{Keywords: e.Keywords, File: file})
	}

	out, err := yaml.MarshalWithOptions(d, yaml.Indent(2))
	if err != nil {
		return fmt.Errorf("writing %s: %w", filepath.Join(dir, YamlFile), err)
	}
	path := filepath.Join(dir, YamlFile)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, out, 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

// bodyToLF turns a game.HelpEntry.Body (\r\n-joined, matching
// ParseHelpFile's own construction) into the plain LF text every other
// file under data/text/ is stored as (§7: "prose stays prose") — \r\n is
// a wire concern applied when do_help's page_string sends the entry, not
// a storage concern.
func bodyToLF(body string) string { return strings.ReplaceAll(body, "\r\n", "\n") }

// lfToBody is bodyToLF's inverse. Any stray \r already in the file
// (a foreign line ending introduced by hand-editing) is normalised away
// first, so the \n-to-\r\n expansion that follows cannot double up.
func lfToBody(text string) string {
	return strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\n", "\r\n")
}
