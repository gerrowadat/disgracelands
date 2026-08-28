// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

// Package dataversion reads and writes the yaml data format's own
// major.minor.patch stamp — docs/design/data-format-versioning.md, which
// this package is the implementation of.
//
// The number stamped is the *release semver of the build that wrote the
// directory*, taken from internal/buildinfo, not a version this package
// maintains by hand. A directory therefore records which dlctl made it,
// and dlmud compares that against its own release: the same major or it
// will not start, a differing minor and it says so and starts anyway.
//
// It is deliberately separate from any one subsystem's schema tag
// (data-format.md §10.1's "schema: dl/<kind>@<major>" on every individual
// file): that tag says what shape *one file* is in, checked the moment
// that file is read. This package says which release of the tooling last
// wrote a data/ directory as a whole — one number for every subsystem
// together, checked once, at boot.
package dataversion

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/goccy/go-yaml"

	"github.com/gerrowadat/disgracelands/internal/persist/yamlenc"

	"github.com/gerrowadat/disgracelands/internal/buildinfo"
)

// FileName is the stamp's own name, at the root of a data directory that
// has any yaml-format subsystem in it.
const FileName = ".dlversion"

// schema is this file's own schema tag, the same convention data-format.md
// §10.1 gives every other yaml document.
const schema = "dl/dataversion@1"

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

// Current is this build's own release version, and whether it has one at
// all. It is buildinfo's version string reduced to a semver: the Makefile
// stamps that from `git describe --tags --always --dirty`, so a build made
// from a tag reads "v1.2.3" and one made four commits past it reads
// "v1.2.3-4-gabc1234" — both of which are release 1.2.3 for compatibility
// purposes, because the format a build writes is whatever its source says,
// and the tag is the only name that source has.
//
// ok is false for a build with no release version to name — `go run`, `go
// test`, `go install`, a plain `go build` with no -ldflags, all of which
// leave buildinfo reporting "devel". Such a build stamps nothing and
// enforces nothing; see CheckBuild, and docs/design/data-format-
// versioning.md §6 for why that hole is deliberate rather than an
// oversight.
func Current() (v Version, ok bool) {
	return parseBuild(buildinfo.Get().Version)
}

// parseBuild reduces a `git describe` string to the release it names.
func parseBuild(s string) (Version, bool) {
	s = strings.TrimPrefix(s, "v")
	// "-4-gabc1234", "-dirty", and semver's own "+build" metadata all
	// describe *this* build, not the release it descends from.
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		s = s[:i]
	}
	v, err := Parse(s)
	if err != nil {
		return Version{}, false
	}
	return v, true
}

type doc struct {
	Schema  string `yaml:"schema"`
	Format  string `yaml:"format"`
	Version string `yaml:"version"`
}

// Read returns the version dir is stamped with, and whether it is stamped
// at all. A directory with no stamp is not an error: it is every directory
// that predates this mechanism, every one written by an unreleased build,
// and every one running only classic/ascii/binary, all of which are
// unversioned by design.
func Read(dir string) (v Version, ok bool, err error) {
	path := filepath.Join(dir, FileName)
	b, err := os.ReadFile(path) //nolint:gosec // operator-configured data directory
	if os.IsNotExist(err) {
		return Version{}, false, nil
	}
	if err != nil {
		return Version{}, false, fmt.Errorf("reading %s: %w", path, err)
	}
	var d doc
	if err := yaml.UnmarshalWithOptions(b, &d, yaml.Strict()); err != nil {
		return Version{}, false, fmt.Errorf("reading %s: %w", path, err)
	}
	got, err := Parse(d.Version)
	if err != nil {
		return Version{}, false, fmt.Errorf("%s: %w", path, err)
	}
	return got, true, nil
}

// Check reads dir's stamp, if it has one, and compares it against current.
// The three outcomes, per docs/design/data-format-versioning.md §2:
//
//   - No stamp, or the same major and minor: both return values are zero.
//     Nothing to say.
//   - Same major, a different minor in either direction: warning is
//     non-empty, err is nil. Something in the format moved additively —
//     boot anyway, but say so, because whichever side is behind may be
//     quietly dropping what the other side knows about.
//   - A different major in either direction: err is non-nil. The two
//     builds do not agree on what the files mean, and this one must not
//     start on data it may misread.
//
// Note that both comparisons are on *difference*, not on newer-ness: data
// written by an older major is refused exactly as firmly as data written
// by a newer one. A major bump is defined as a change an other-versioned
// reader gets wrong rather than merely fails to understand, and "wrong" is
// not a direction.
func Check(dir string, current Version) (warning string, err error) {
	got, ok, err := Read(dir)
	if err != nil || !ok {
		return "", err
	}
	path := filepath.Join(dir, FileName)
	switch {
	case got.Major != current.Major:
		return "", fmt.Errorf(
			"%s was written by release %s; this is release %s, and major version %d only loads data written by major version %d — see docs/design/data-format-versioning.md",
			path, got, current, current.Major, current.Major)
	case got.Minor != current.Minor:
		return fmt.Sprintf(
			"%s was written by release %s; this is release %s. Same major version, so it will load — but the two disagree on minor version, and whichever is behind may not understand everything the other wrote. Run `dlctl data version --dir=%s` for specifics.",
			path, got, current, dir), nil
	default:
		return "", nil
	}
}

// CheckBuild is Check against this build's own release version, and the
// entry point cmd/dlmud boots through.
//
// A build with no release version of its own (Current's ok is false) has
// nothing to compare against, so it enforces nothing. It says so, once,
// but only for a directory that actually carries a stamp — an unstamped
// directory read by an unversioned build is two halves of the same
// silence, and there is no finding to report.
func CheckBuild(dir string) (warning string, err error) {
	if current, ok := Current(); ok {
		return Check(dir, current)
	}
	got, ok, err := Read(dir)
	if err != nil || !ok {
		return "", err
	}
	return fmt.Sprintf(
		"%s was written by release %s, and this build (%s) has no release version of its own to check that against — no compatibility check was made. See docs/design/data-format-versioning.md.",
		filepath.Join(dir, FileName), got, buildinfo.Get().Version), nil
}

// Write stamps dir/.dlversion with v, atomically. `dlctl import` (no
// --type) calls it with Current at the end of a successful conversion; `dlctl
// data version --write` calls it by hand, which is the adoption path for
// a directory that predates the stamp or one an older release wrote.
func Write(dir string, v Version) error {
	path := filepath.Join(dir, FileName)
	d := doc{Schema: schema, Format: "yaml", Version: v.String()}
	out, err := yaml.MarshalWithOptions(d, yamlenc.Options()...)
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
