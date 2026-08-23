// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

// Package dataversion reads and writes the yaml data format's own
// major.minor.patch stamp — docs/design/data-format-versioning.md, which
// this package is the implementation of.
//
// It is deliberately separate from any one subsystem's schema tag
// (data-format.md §10.1's "schema: dl/<kind>@<major>" on every individual
// file): that tag says what shape *one file* is in, checked the moment
// that file is read. This package says what release of the yaml format
// packages as a whole a data/ directory was last written by — one number
// for every subsystem together, checked once, at boot.
package dataversion

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/goccy/go-yaml"
)

// FileName is the stamp's own name, at the root of a data directory that
// has any yaml-format subsystem in it.
const FileName = ".dlversion"

// schema is this file's own schema tag, the same convention data-format.md
// §10.1 gives every other yaml document.
const schema = "dl/dataversion@1"

// Current is the version this build's yaml packages implement. Bump it —
// and only it — when a change to internal/persist/*/yaml earns one, per
// docs/design/data-format-versioning.md's three tiers. There has been
// exactly one version so far: this is also the first one.
var Current = Version{Major: 1, Minor: 0, Patch: 0}

// Version is a data/ directory's format-version stamp.
type Version struct {
	Major, Minor, Patch int
}

// String renders "major.minor.patch".
func (v Version) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// Parse reads a "major.minor.patch" string, the only shape Write ever
// produces and Check ever accepts.
func Parse(s string) (Version, error) {
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return Version{}, fmt.Errorf("dataversion: %q is not major.minor.patch", s)
	}
	var nums [3]int
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return Version{}, fmt.Errorf("dataversion: %q is not major.minor.patch", s)
		}
		nums[i] = n
	}
	return Version{Major: nums[0], Minor: nums[1], Patch: nums[2]}, nil
}

type doc struct {
	Schema  string `yaml:"schema"`
	Format  string `yaml:"format"`
	Version string `yaml:"version"`
}

// Check reads dir/.dlversion, if it exists, and compares it against
// current. The three tiers, per docs/design/data-format-versioning.md:
//
//   - No stamp, or the stamp is at or below what current supports: both
//     return values are zero. There is nothing for a caller to say — this
//     covers every data/ directory that predates the stamp, and every one
//     already caught up.
//   - The stamp's minor is newer than current's, same major: warning is
//     non-empty, err is nil. "Own risk" — boot anyway, but say so.
//   - The stamp's major is newer than current's: err is non-nil. This
//     build may misread the data outright, and must not start on it.
func Check(dir string, current Version) (warning string, err error) {
	path := filepath.Join(dir, FileName)
	b, err := os.ReadFile(path) //nolint:gosec // operator-configured data directory
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", path, err)
	}
	var d doc
	if err := yaml.UnmarshalWithOptions(b, &d, yaml.Strict()); err != nil {
		return "", fmt.Errorf("reading %s: %w", path, err)
	}
	got, err := Parse(d.Version)
	if err != nil {
		return "", fmt.Errorf("%s: %w", path, err)
	}
	switch {
	case got.Major > current.Major:
		return "", fmt.Errorf(
			"%s is format version %s; this server understands up to major version %d and will not start on newer data — see docs/design/data-format-versioning.md",
			path, got, current.Major)
	case got.Major == current.Major && got.Minor > current.Minor:
		return fmt.Sprintf(
			"%s is format version %s; this server only knows %s — starting anyway, at your own risk (docs/design/data-format-versioning.md). Run `dlctl data version --dir=%s` for specifics.",
			path, got, current, dir), nil
	default:
		return "", nil
	}
}

// Write stamps dir/.dlversion with v, atomically. This is the adoption
// path for a data directory that predates the stamp — `dlctl data version
// --write`'s own implementation — and what a future format-changing
// `fmt`/`import` tool would call once one has a reason to bump v.
func Write(dir string, v Version) error {
	path := filepath.Join(dir, FileName)
	d := doc{Schema: schema, Format: "yaml", Version: v.String()}
	out, err := yaml.MarshalWithOptions(d, yaml.Indent(2))
	if err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
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
