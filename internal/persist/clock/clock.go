// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

// Package clock reads and writes the mud calendar's epoch, porting
// reset_time and save_mud_time (db.c:483-521, :534-544) and their file,
// lib/etc/time (db.h:97's TIME_FILE).
//
// Unlike misc/xnames this file *is* written at runtime — every
// PULSE_TIMESAVE (comm.c:921-922, thirty real minutes) and once more at
// shutdown (comm.c:441) — so, unlike internal/persist/names, both
// directions of both formats are implemented.
package clock

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/goccy/go-yaml"
)

// ClassicFile is etc/time under whatever directory holds it.
const ClassicFile = "time"

// YamlFile is state/clock.yaml under whatever directory Load/Save's path
// names, per docs/design/data-format.md §9.
const YamlFile = "clock.yaml"

// clockSchema is the document's schema tag (data-format.md §10.1).
const clockSchema = "dl/clock@1"

// DefaultEpoch is reset_time's fallback (db.c:495-496), used when the file
// is missing, unreadable, or its content is or parses to zero. See
// docs/weirdnumbers.md's "The clock's fallback epoch is a magic number
// with no explanation".
const DefaultEpoch = 650336715

type doc struct {
	Schema string `yaml:"schema"`
	// Epoch is RFC 3339 — the C's file has neither a name nor units on its
	// one integer, which is the whole reason state/clock.yaml exists
	// rather than just moving the same bare number (data-format.md §9).
	Epoch string `yaml:"epoch"`
}

// Load reads the epoch mud time is measured forward from, in the given
// format ("classic" or "yaml", "" meaning classic). A missing file, an
// unreadable one, or one that holds zero all resolve to DefaultEpoch —
// reset_time's own fallback, applied uniformly regardless of format since
// the fallback is a property of "no usable epoch was found", not of
// classic's file shape specifically.
func Load(format, path string) (time.Time, error) {
	var secs int64
	var err error
	switch format {
	case "", "classic":
		secs, err = loadClassic(path)
	case "yaml":
		secs, err = loadYaml(path)
	default:
		return time.Time{}, fmt.Errorf("clock: unknown format %q", format)
	}
	if err != nil {
		return time.Time{}, err
	}
	if secs == 0 {
		secs = DefaultEpoch
	}
	return time.Unix(secs, 0).UTC(), nil
}

// Save writes the epoch in the given format.
func Save(format, path string, epoch time.Time) error {
	switch format {
	case "", "classic":
		return saveClassic(path, epoch)
	case "yaml":
		return saveYaml(path, epoch)
	default:
		return fmt.Errorf("clock: unknown format %q", format)
	}
}

// loadClassic reads a bare integer, porting reset_time's fscanf
// (db.c:492). A missing file mirrors a failed fopen (db.c:488-489): logged
// there, silently absorbed here since this package has no logger of its
// own — the caller sees DefaultEpoch either way, which is what boot_db
// itself ends up with. Content that does not parse is treated the same
// way: fscanf on garbage leaves beginning_of_time at its initialised
// zero, exactly as if the file were empty.
func loadClassic(path string) (int64, error) {
	b, err := os.ReadFile(path) //nolint:gosec // operator-configured path
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("reading %s: %w", path, err)
	}
	secs, err := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64)
	if err != nil {
		return 0, nil //nolint:nilerr // garbage content is reset_time's "still zero" case, not a hard failure
	}
	return secs, nil
}

func saveClassic(path string, epoch time.Time) error {
	return atomicWrite(path, fmt.Appendf(nil, "%d\n", epoch.Unix()))
}

func loadYaml(dir string) (int64, error) {
	path := filepath.Join(dir, YamlFile)
	b, err := os.ReadFile(path) //nolint:gosec // operator-configured path
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("reading %s: %w", path, err)
	}
	var d doc
	if err := yaml.UnmarshalWithOptions(b, &d, yaml.Strict()); err != nil {
		return 0, fmt.Errorf("reading %s: %w", path, err)
	}
	if d.Epoch == "" {
		return 0, nil
	}
	t, err := time.Parse(time.RFC3339, d.Epoch)
	if err != nil {
		return 0, fmt.Errorf("%s: %q: %w", path, d.Epoch, err)
	}
	return t.Unix(), nil
}

func saveYaml(dir string, epoch time.Time) error {
	path := filepath.Join(dir, YamlFile)
	d := doc{Schema: clockSchema, Epoch: epoch.UTC().Format(time.RFC3339)}
	out, err := yaml.MarshalWithOptions(d, yaml.Indent(2))
	if err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return atomicWrite(path, out)
}

func atomicWrite(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}
