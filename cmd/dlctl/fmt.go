// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gerrowadat/disgracelands/internal/persist/help"
	"github.com/gerrowadat/disgracelands/internal/persist/messages"
	"github.com/gerrowadat/disgracelands/internal/persist/names"
	"github.com/gerrowadat/disgracelands/internal/persist/player"
	playeryaml "github.com/gerrowadat/disgracelands/internal/persist/player/yaml"
	"github.com/gerrowadat/disgracelands/internal/persist/socials"
	"github.com/gerrowadat/disgracelands/internal/persist/world"
	worldyaml "github.com/gerrowadat/disgracelands/internal/persist/world/yaml"
)

// cmdFmt canonicalises a yaml directory in place, for the --type given —
// loading and immediately re-writing is idempotent by construction for
// every one of these, since every yaml writer is deterministic (running
// this twice in a row produces no further diff).
func cmdFmt(args []string) error {
	fs := flag.NewFlagSet("fmt", flag.ContinueOnError)
	typeRaw := fs.String("type", "", "Subsystem to canonicalise: "+joinTypes(allTypes))
	dir := fs.String("dir", "data", "Yaml directory (base)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	t, err := parseType(*typeRaw, allTypes)
	if err != nil {
		return err
	}
	base, err := resolveDir(t, *dir, "yaml")
	if err != nil {
		return err
	}
	switch t {
	case typeWorld:
		return fmtWorld(base)
	case typePfile:
		return fmtPfile(base)
	case typeState:
		return fmtState(base)
	case typeNames:
		return fmtNames(base)
	case typeMessages:
		return fmtMessages(base)
	case typeSocials:
		return fmtSocials(base)
	case typeHelp:
		return fmtHelp(base)
	default:
		return fmt.Errorf("fmt: unsupported --type %q", t)
	}
}

// fmtWorld canonicalises a yaml world directory in place: §11 step 3's
// `dlctl fmt --type=world`.
func fmtWorld(dir string) error {
	nsrc, err := worldyaml.New(world.Config{Dir: dir})
	if err != nil {
		return err
	}
	defer func() { _ = nsrc.Close() }()

	w, warnings, err := nsrc.LoadWithWarnings(context.Background())
	if err != nil {
		return fmt.Errorf("loading %s: %w", dir, err)
	}

	out := bufio.NewWriter(os.Stdout)
	defer func() { _ = out.Flush() }()
	for _, wr := range warnings {
		if wr.Severity >= world.Warn {
			_, _ = fmt.Fprintf(out, "%s\n", wr)
		}
	}

	for _, z := range w.Zones {
		if err := nsrc.WriteZone(context.Background(), z, w); err != nil {
			return fmt.Errorf("writing zone %d: %w", z.Vnum, err)
		}
	}
	_, _ = fmt.Fprintf(out, "formatted %d zone(s)\n", len(w.Zones))
	return out.Flush()
}

// fmtPfile canonicalises a yaml player directory in place: load and
// immediately re-save every character. Store.Save's own read-merge-write
// already preserves each file's rent/inventory section untouched, so this
// reformats the roster half only, idempotently.
func fmtPfile(dir string) error {
	s, err := playeryaml.New(player.Config{Dir: dir})
	if err != nil {
		return err
	}
	defer func() { _ = s.Close() }()

	out := bufio.NewWriter(os.Stdout)
	defer func() { _ = out.Flush() }()

	ctx := context.Background()

	// Drain the listing before touching anything, rather than saving
	// inside the range loop.
	//
	// player/yaml's List holds the store's read lock for the whole
	// iteration, and Save takes the write lock — and a Go RWMutex is
	// neither reentrant nor writer-starving, so a writer that arrives
	// while a reader is still iterating blocks forever, and the reader
	// never finishes because it is the same goroutine. `dlctl fmt
	// --type=pfile` therefore hung, permanently, on any directory with a
	// character in it.
	//
	// Nothing found it because until examples/torture there was no
	// checked-in fixture in this repository with a roster in it at all
	// (docs/proposals/yaml-only.md §5.1) — a hang needs a character to
	// hang on, and every corpus had zero.
	var names []string
	for entry, err := range s.List(ctx) {
		if err != nil {
			_, _ = fmt.Fprintf(out, "listing: %v\n", err)
			continue
		}
		names = append(names, entry.Name)
	}

	n := 0
	for _, name := range names {
		rec, err := s.Load(ctx, name)
		if err != nil {
			_, _ = fmt.Fprintf(out, "%s: %v\n", name, err)
			continue
		}
		if err := s.Save(ctx, rec); err != nil {
			_, _ = fmt.Fprintf(out, "%s: writing: %v\n", name, err)
			continue
		}
		n++
	}
	_, _ = fmt.Fprintf(out, "formatted %d character(s)\n", n)
	return out.Flush()
}

func fmtNames(dir string) error {
	list, err := names.Load("yaml", dir)
	if err != nil {
		return fmt.Errorf("reading %s: %w", filepath.Join(dir, names.YamlFile), err)
	}
	return names.Save("yaml", dir, list)
}

func fmtMessages(dir string) error {
	records, err := messages.Load("yaml", dir)
	if err != nil {
		return fmt.Errorf("reading %s: %w", filepath.Join(dir, messages.YamlFile), err)
	}
	return messages.Save("yaml", dir, records)
}

func fmtSocials(dir string) error {
	list, err := socials.Load("yaml", dir)
	if err != nil {
		return fmt.Errorf("reading %s: %w", filepath.Join(dir, socials.YamlFile), err)
	}
	return socials.Save("yaml", dir, list)
}

func fmtHelp(dir string) error {
	entries, err := help.Load("yaml", dir)
	if err != nil {
		return fmt.Errorf("reading %s: %w", filepath.Join(dir, help.YamlFile), err)
	}
	return help.Save("yaml", dir, entries)
}
