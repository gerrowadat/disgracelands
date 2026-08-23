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
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"

	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/persist/convert"
	"github.com/gerrowadat/disgracelands/internal/persist/world"
	"github.com/gerrowadat/disgracelands/internal/persist/world/classic"
	"github.com/gerrowadat/disgracelands/internal/persist/world/yaml"
)

// cmdWorldImport converts a classic world directory into yaml, per
// docs/design/data-format.md §11 step 2: `dlctl world import`.
//
// It replaces nothing the C server reads — classic stays the parity oracle
// — and produces a zones.yaml manifest listing every zone the source
// loaded, enabled, plus one YAML file per zone.
func cmdWorldImport(args []string) error {
	fs := flag.NewFlagSet("world import", flag.ContinueOnError)
	fromDir := fs.String("from-dir", "data/world", "Source (classic) world directory")
	toDir := fs.String("to-dir", "data/world", "Destination (yaml) world directory")
	encName := fs.String("encoding", convert.DefaultEncoding,
		fmt.Sprintf("Source text encoding: %v", encodingNames()))
	mini := fs.Bool("mini-mud", false, "Use the reduced index.mini file list")
	if err := fs.Parse(args); err != nil {
		return err
	}

	enc, ok := convert.Encodings[*encName]
	if !ok {
		return fmt.Errorf("unknown encoding %q (have: %v)", *encName, encodingNames())
	}

	src, err := classic.New(world.Config{Dir: *fromDir, Mini: *mini})
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()

	w, warnings, err := src.LoadWithWarnings(context.Background())
	if err != nil {
		return fmt.Errorf("loading %s: %w", *fromDir, err)
	}

	out := bufio.NewWriter(os.Stdout)
	defer func() { _ = out.Flush() }()

	for _, wr := range warnings {
		_, _ = fmt.Fprintf(out, "%s\n", wr)
	}

	transcoded := transcodeWorldStrings(w, enc)

	if err := os.MkdirAll(*toDir, 0o755); err != nil { //nolint:gosec // world data, not secrets
		return err
	}
	entries := make([]yaml.ManifestEntry, 0, len(w.Zones))
	for _, z := range w.Zones {
		entries = append(entries, yaml.ManifestEntry{Vnum: int32(z.Vnum), Enabled: true})
	}
	if err := yaml.WriteManifest(*toDir, entries); err != nil {
		return fmt.Errorf("writing zones.yaml: %w", err)
	}

	nsrc, err := yaml.New(world.Config{Dir: *toDir})
	if err != nil {
		return err
	}
	defer func() { _ = nsrc.Close() }()

	for _, z := range w.Zones {
		if err := nsrc.WriteZone(context.Background(), z, w); err != nil {
			return fmt.Errorf("writing zone %d: %w", z.Vnum, err)
		}
	}

	_, _ = fmt.Fprintf(out, "\nimported %d zone(s), %d room(s), %d mobile(s), %d object(s), %d shop(s)\n",
		len(w.Zones), len(w.Rooms), len(w.Mobiles), len(w.Objects), len(w.Shops))
	if transcoded > 0 {
		_, _ = fmt.Fprintf(out, "transcoded %d string(s) from %s to UTF-8\n", transcoded, *encName)
	}
	return out.Flush()
}

// cmdWorldFmt canonicalises a yaml world directory in place: §11 step 3's
// `dlctl world fmt`. Loading and immediately re-writing every zone is
// idempotent by construction, because yaml.Source.WriteZone is
// deterministic (§10.3) — running this twice in a row produces no further
// diff.
func cmdWorldFmt(args []string) error {
	fs := flag.NewFlagSet("world fmt", flag.ContinueOnError)
	dir := fs.String("world-dir", "data/world", "Yaml world data directory")
	if err := fs.Parse(args); err != nil {
		return err
	}

	nsrc, err := yaml.New(world.Config{Dir: *dir})
	if err != nil {
		return err
	}
	defer func() { _ = nsrc.Close() }()

	w, warnings, err := nsrc.LoadWithWarnings(context.Background())
	if err != nil {
		return fmt.Errorf("loading %s: %w", *dir, err)
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

// transcodeWorldStrings converts every text field in w from enc to UTF-8,
// in place, and returns how many fields actually needed it — mirroring
// internal/persist/convert's file-level transcode, applied here to a
// loaded world's fields instead of to file bytes, and only where the C
// loader treats the field as free text rather than a keyword or symbol.
func transcodeWorldStrings(w *game.World, enc *charmap.Charmap) int {
	n := 0
	fix := func(s *string) {
		if utf8.ValidString(*s) {
			return
		}
		if out, err := enc.NewDecoder().String(*s); err == nil {
			*s = out
			n++
		}
	}
	for _, r := range w.Rooms {
		fix(&r.Name)
		fix(&r.Description)
		for _, e := range r.Exits {
			if e != nil {
				fix(&e.Description)
			}
		}
		for i := range r.ExtraDescs {
			fix(&r.ExtraDescs[i].Description)
		}
	}
	for _, m := range w.Mobiles {
		fix(&m.ShortDesc)
		fix(&m.LongDesc)
		fix(&m.Description)
	}
	for _, o := range w.Objects {
		fix(&o.ShortDesc)
		fix(&o.Description)
		fix(&o.ActionDesc)
		for i := range o.ExtraDescs {
			fix(&o.ExtraDescs[i].Description)
		}
	}
	for _, sh := range w.Shops {
		for i := range sh.Messages {
			fix(&sh.Messages[i])
		}
	}
	return n
}
