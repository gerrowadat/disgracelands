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
	"sort"

	"github.com/gerrowadat/disgracelands/internal/persist/world"
	"github.com/gerrowadat/disgracelands/internal/persist/world/classic"
)

// lintTypes is who `dlctl lint` supports today — just world, the C tree's
// scheck/-c equivalent. --type is still required and validated against
// this narrower set (not allTypes), so a future `lint --type=state` fails
// clearly rather than needing dispatch logic added blind.
var lintTypes = []dirType{typeWorld}

// loadWorld opens the configured source and loads it, returning findings
// where the source can produce them.
func loadWorld(ctx context.Context, dir, format string, mini bool, opts world.Options) (*world.Dump, []world.Warning, error) {
	src, err := world.Open(format, world.Config{Dir: dir, Mini: mini})
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = src.Close() }()

	// Findings are format-specific by nature — they name that format's files
	// and quirks — so only a source that produces them offers them, via the
	// optional FindingSource interface rather than a method on Source itself.
	if fs, ok := src.(world.FindingSource); ok {
		w, warnings, err := fs.LoadWithWarnings(ctx)
		if err != nil {
			return nil, warnings, err
		}
		return world.BuildDumpWithOptions(w, opts), warnings, nil
	}

	w, err := src.Load(ctx)
	if err != nil {
		return nil, nil, err
	}
	return world.BuildDumpWithOptions(w, opts), nil, nil
}

// cmdLint checks a directory's files and reports what it finds. For
// --type=world it replaces the C tree's src/util/scheck and its -c mode,
// and unlike either can run in CI without starting a server.
func cmdLint(args []string) error {
	fs := flag.NewFlagSet("lint", flag.ContinueOnError)
	typeRaw := fs.String("type", "", "Subsystem to check: "+joinTypes(lintTypes))
	dir := fs.String("dir", "data", "Data directory (base)")
	format := fs.String("format", classic.FormatName, fmt.Sprintf("Format: %v", world.Formats()))
	mini := fs.Bool("mini-mud", false, "Use the reduced index.mini file list")
	quiet := fs.Bool("quiet", false, "Suppress informational findings")
	strict := fs.Bool("strict", false, "Exit non-zero on warnings as well as errors")
	if err := fs.Parse(args); err != nil {
		return err
	}
	t, err := parseType(*typeRaw, lintTypes)
	if err != nil {
		return err
	}
	target, err := resolveDir(t, *dir, *format)
	if err != nil {
		return err
	}

	dump, findings, err := loadWorld(context.Background(), target, *format, *mini, world.Options{})
	if err != nil {
		// A load failure is itself the finding, and the most important one.
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return errQuiet
	}

	out := bufio.NewWriter(os.Stdout)
	defer func() { _ = out.Flush() }()

	// Report in severity order, worst first: a boot-blocking error should not
	// be buried under fifty informational lines.
	sorted := make([]world.Warning, len(findings))
	copy(sorted, findings)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Severity > sorted[j].Severity
	})

	var counts [3]int
	for _, f := range sorted {
		if f.Severity >= 0 && int(f.Severity) < len(counts) {
			counts[f.Severity]++
		}
		if *quiet && f.Severity == world.Info {
			continue
		}
		_, _ = fmt.Fprintf(out, "%s: %s\n", f.Severity, f.Message)
	}

	_, _ = fmt.Fprintf(out, "\n%d rooms, %d mobiles, %d objects, %d zones\n",
		dump.Counts.Rooms, dump.Counts.Mobiles, dump.Counts.Objects, dump.Counts.Zones)
	_, _ = fmt.Fprintf(out, "%d error(s), %d warning(s), %d note(s)\n",
		counts[world.Error], counts[world.Warn], counts[world.Info])
	_ = out.Flush()

	if counts[world.Error] > 0 {
		return errQuiet
	}
	if *strict && counts[world.Warn] > 0 {
		return errQuiet
	}
	return nil
}
