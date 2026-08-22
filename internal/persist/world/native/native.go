// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

// Package native implements the "native" world-data format:
// docs/proposals/data-format.md. It registers as world.Source and
// world.Sink alongside classic, and reads/writes the YAML-over-JSON zone
// files described there.
package native

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/persist/world"
)

// FormatName is the name this format registers under.
const FormatName = "native"

func init() {
	world.Register(FormatName, func(cfg world.Config) (world.Source, error) {
		return New(cfg)
	})
}

// Source reads and writes a native world directory.
type Source struct {
	dir string
}

// New opens a native world source. It does not touch the filesystem; a
// missing directory surfaces from Load, alongside everything else that can
// go wrong with the data.
func New(cfg world.Config) (*Source, error) {
	if cfg.Dir == "" {
		return nil, fmt.Errorf("native: no world directory configured")
	}
	return &Source{dir: cfg.Dir}, nil
}

// Name implements world.Source.
func (s *Source) Name() string { return FormatName }

// Close implements world.Source. Nothing is held open between calls.
func (s *Source) Close() error { return nil }

// Load implements world.Source.
func (s *Source) Load(ctx context.Context) (*game.World, error) {
	w, _, err := s.LoadWithWarnings(ctx)
	return w, err
}

// LoadWithWarnings implements world.FindingSource.
func (s *Source) LoadWithWarnings(ctx context.Context) (*game.World, []world.Warning, error) {
	l := &loader{dir: s.dir}
	w, err := l.load(ctx)
	return w, l.warnings, err
}

type loader struct {
	dir      string
	warnings []world.Warning
}

func (l *loader) at(sev world.Severity, format string, args ...any) {
	l.warnings = append(l.warnings, world.Warning{Severity: sev, Message: fmt.Sprintf(format, args...)})
}

func (l *loader) infof(format string, args ...any)  { l.at(world.Info, format, args...) }
func (l *loader) warnf(format string, args ...any)  { l.at(world.Warn, format, args...) }
func (l *loader) errorf(format string, args ...any) { l.at(world.Error, format, args...) }

func (l *loader) load(_ context.Context) (*game.World, error) {
	manifest, err := readManifest(l.dir)
	if err != nil {
		return nil, fmt.Errorf("native: %w", err)
	}

	files, findings, err := zoneFiles(l.dir)
	if err != nil {
		return nil, fmt.Errorf("native: %w", err)
	}
	for _, f := range findings {
		l.warnf("%s", f)
	}

	seen := make(map[int32]bool, len(manifest.Zones))
	w := &game.World{}

	entries := make([]ManifestEntry, len(manifest.Zones))
	copy(entries, manifest.Zones)
	sort.Slice(entries, func(i, j int) bool { return entries[i].Vnum < entries[j].Vnum })

	for _, entry := range entries {
		seen[entry.Vnum] = true
		if !entry.Enabled {
			l.infof("zone %d: not loaded (%s)", entry.Vnum, noteOr(entry.Note, "disabled in zones.yaml"))
			continue
		}
		filename, ok := files[entry.Vnum]
		if !ok {
			l.errorf("zone %d: listed in zones.yaml but no file has that vnum", entry.Vnum)
			continue
		}
		if err := l.loadZoneFile(filepath.Join(l.dir, filename), w); err != nil {
			l.errorf("%s: %v", filename, err)
		}
	}

	for vnum, filename := range files {
		if !seen[vnum] {
			l.warnf("%s: zone %d exists on disk but is not listed in zones.yaml", filename, vnum)
		}
	}

	return w, nil
}

func noteOr(note, fallback string) string {
	if note != "" {
		return note
	}
	return fallback
}

// atomicWrite writes data to path via a temp file and rename, so a reader
// never sees a half-written file — §3's "one writer per file" rule.
func atomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tmp-*.yaml")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
