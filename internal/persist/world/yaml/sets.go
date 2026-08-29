// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package yaml

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/goccy/go-yaml"

	"github.com/gerrowadat/disgracelands/internal/persist/yamlenc"
)

// SetsFile is sets.yaml's name within the world directory, and SetsSchema
// its schema tag. docs/design/data-format.md §4 has listed the file in this
// format's directory layout since the format was designed — "named subsets
// of zones (`mini`, for --mini-mud)" — and nothing built it, so `--mini-mud`
// was accepted, validated, plumbed all the way to world.Config.Mini and then
// silently ignored by the only source the server opens (issue #274).
const (
	SetsFile   = "sets.yaml"
	SetsSchema = "dl/sets@1"
)

// MiniSet is the set name --mini-mud selects. It is the yaml format's
// answer to classic's index.mini, and the name is the C's own: `-m`.
const MiniSet = "mini"

// setsDoc is sets.yaml: named subsets of the zones zones.yaml lists.
//
//	schema: dl/sets@1
//	sets:
//	  mini: [0, 12, 30]
//
// A map rather than a `mini:` field, because the file is called "sets" in
// the design and "named subsets" is what it says. Nothing but `mini` is
// consulted today; a second name costs nothing to store and is the obvious
// thing to want next (a builder's own working set, a test fixture).
//
// The subset is over *zones*, not over the five index files classic keeps,
// and that is exact rather than approximate: a yaml zone file holds the
// zone's rooms, mobiles, objects and shops together, and the classic mini
// indexes are themselves per-zone. Stock's are `0.wld 12.wld 30.wld`,
// `0.obj 30.obj`, `30.shp` — which is not three different subsets but one
// subset of three zones, two of which simply have no object or shop file to
// list. Checked against the data rather than assumed: there is no 12.obj or
// 12.shp in the stock tree at all.
type setsDoc struct {
	Schema string             `yaml:"schema"`
	Sets   map[string][]int32 `yaml:"sets"`
}

// readSets loads sets.yaml from dir. A missing file is not an error here —
// most directories have no subsets and want none; it is asking for a subset
// that does not exist that fails, in selectSet.
func readSets(dir string) (setsDoc, error) {
	data, err := os.ReadFile(filepath.Join(dir, SetsFile)) //nolint:gosec // operator-supplied directory
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return setsDoc{}, nil
		}
		return setsDoc{}, err
	}
	var doc setsDoc
	if err := yaml.UnmarshalWithOptions(data, &doc, yaml.Strict()); err != nil {
		return setsDoc{}, fmt.Errorf("%s: %w", SetsFile, err)
	}
	if doc.Schema != SetsSchema {
		return setsDoc{}, fmt.Errorf("%s: schema %q, want %q", SetsFile, doc.Schema, SetsSchema)
	}
	return doc, nil
}

// WriteSets writes sets.yaml, for tools that build a whole yaml directory
// from scratch. Writing no file at all when there are no sets is deliberate:
// an empty `sets: {}` is a file that says nothing, and `dlctl import` should
// not manufacture one for a source that had no index.mini.
func WriteSets(dir string, sets map[string][]int32) error {
	if len(sets) == 0 {
		return nil
	}
	normalised := make(map[string][]int32, len(sets))
	for name, vnums := range sets {
		if len(vnums) == 0 {
			continue
		}
		sorted := make([]int32, len(vnums))
		copy(sorted, vnums)
		sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
		normalised[name] = sorted
	}
	if len(normalised) == 0 {
		return nil
	}
	out, err := yaml.MarshalWithOptions(setsDoc{Schema: SetsSchema, Sets: normalised}, yamlenc.Options()...)
	if err != nil {
		return err
	}
	return atomicWrite(filepath.Join(dir, SetsFile), out)
}

// selectSet returns the vnums in the named set, and whether the caller
// should restrict to them at all.
//
// **A missing set is an error, not "load everything".** That is the C's own
// behaviour — `index_boot` exits when the index file it was told to open is
// not there (db.c) — and it is the specific mistake this whole issue is
// about: a flag that quietly does nothing looks exactly like a flag that
// worked. `dlmud --mini-mud` on a directory with no `mini` set now says so
// and stops, rather than booting all thirty zones and letting the operator
// believe they have three.
func selectSet(doc setsDoc, name string) ([]int32, error) {
	vnums, ok := doc.Sets[name]
	if !ok {
		return nil, fmt.Errorf("no %q set in %s: this directory has no %s subset defined, so there is nothing to load a subset of",
			name, SetsFile, name)
	}
	if len(vnums) == 0 {
		return nil, fmt.Errorf("the %q set in %s is empty: a world with no zones in it is not a world", name, SetsFile)
	}
	return vnums, nil
}
