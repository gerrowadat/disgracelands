// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

// Package yaml implements the "yaml" world-data format:
// docs/design/data-format.md. It registers as world.Source and
// world.Sink alongside classic, and reads/writes the YAML-over-JSON zone
// files described there.
package yaml

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"

	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/persist/atomicfile"
	"github.com/gerrowadat/disgracelands/internal/persist/world"
)

// FormatName is the name this format registers under.
const FormatName = "yaml"

func init() {
	world.Register(FormatName, func(cfg world.Config) (world.Source, error) {
		return New(cfg)
	})
}

// Source reads and writes a yaml world directory.
type Source struct {
	dir string
	// mini restricts the load to sets.yaml's "mini" subset — world.Config's
	// own Mini, which is --mini-mud, which is the C's -m.
	//
	// This field not existing is issue #274: New dropped cfg.Mini on the
	// floor, classic was the only source that read it, and cmd/dlmud stopped
	// linking classic when yaml-only landed. The flag stayed valid and the
	// field stayed plumbed, so nothing failed and nothing was printed.
	mini bool
}

// New opens a yaml world source. It does not touch the filesystem; a
// missing directory surfaces from Load, alongside everything else that can
// go wrong with the data.
func New(cfg world.Config) (*Source, error) {
	if cfg.Dir == "" {
		return nil, fmt.Errorf("yaml: no world directory configured")
	}
	return &Source{dir: cfg.Dir, mini: cfg.Mini}, nil
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
	l := &loader{dir: s.dir, mini: s.mini}
	w, err := l.load(ctx)
	if err != nil {
		return w, l.warnings, err
	}
	// The same cross-reference pass the classic loader runs, over the same
	// game.World, so that a converted directory reports the dangling vnums
	// it still has rather than reporting nothing at all (#286). Before this
	// the archived lib/ produced twenty findings as classic and none as
	// yaml, four of which were still true of the converted data -- so `lint`
	// went quiet exactly at the cutover, on the format we actually run on.
	l.warnings = append(l.warnings, world.CheckReferences(w)...)
	return w, l.warnings, nil
}

type loader struct {
	dir      string
	mini     bool
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
		return nil, fmt.Errorf("yaml: %w", err)
	}

	files, findings, err := zoneFiles(l.dir)
	if err != nil {
		return nil, fmt.Errorf("yaml: %w", err)
	}
	for _, f := range findings {
		l.warnf("%s", f)
	}

	// --mini-mud: restrict to sets.yaml's "mini" subset. Resolved before
	// anything is loaded so that asking for a subset a directory does not
	// have fails at boot rather than half way through a world.
	var wanted map[int32]bool
	if l.mini {
		sets, serr := readSets(l.dir)
		if serr != nil {
			return nil, fmt.Errorf("yaml: %w", serr)
		}
		vnums, serr := selectSet(sets, MiniSet)
		if serr != nil {
			return nil, fmt.Errorf("yaml: --mini-mud: %w", serr)
		}
		wanted = make(map[int32]bool, len(vnums))
		for _, v := range vnums {
			wanted[v] = true
		}
	}

	seen := make(map[int32]bool, len(manifest.Zones))
	w := &game.World{}

	entries := make([]ManifestEntry, len(manifest.Zones))
	copy(entries, manifest.Zones)
	sort.Slice(entries, func(i, j int) bool { return entries[i].Vnum < entries[j].Vnum })

	for _, entry := range entries {
		seen[entry.Vnum] = true
		if wanted != nil && !wanted[entry.Vnum] {
			// Not a warning: being outside the subset is the whole point of
			// asking for one, and thirty of these on every --mini-mud boot
			// would be noise around the three lines that matter.
			continue
		}
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

	// A subset naming a zone the manifest does not is a real mistake and an
	// easy one — sets.yaml and zones.yaml are edited separately — so it is
	// an error rather than a silently smaller world.
	for vnum := range wanted {
		if !seen[vnum] {
			l.errorf("%s: the %q set names zone %d, which is not in %s", SetsFile, MiniSet, vnum, ManifestFile)
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
//
// This is where the pattern was written correctly first, with a *unique*
// temp name from os.CreateTemp. Every other format in the tree had its own
// copy using a fixed `path + ".tmp"` instead, which raced with itself; the
// shared implementation in internal/persist/atomicfile is this one,
// generalised, and this is now a thin call into it rather than a
// seventeenth variant.
func atomicWrite(path string, data []byte) error {
	return atomicfile.Write(path, data, 0o600)
}
