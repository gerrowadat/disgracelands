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

// worldFlags declares the options shared by the world subcommands.
func worldFlags(fs *flag.FlagSet) (dir, format *string, mini *bool) {
	dir = fs.String("world-dir", "data/world", "World data directory")
	format = fs.String("world-format", classic.FormatName,
		fmt.Sprintf("World format: %v", world.Formats()))
	mini = fs.Bool("mini-mud", false, "Use the reduced index.mini file list")
	return
}

// loadWorld opens the configured source and loads it, returning findings
// where the source can produce them.
func loadWorld(ctx context.Context, dir, format string, mini bool, opts world.Options) (*world.Dump, []classic.Warning, error) {
	src, err := world.Open(format, world.Config{Dir: dir, Mini: mini})
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = src.Close() }()

	// Findings are format-specific by nature — they name that format's files
	// and quirks — so only a source that produces them offers them, and this
	// is a type assertion rather than a method on the Source interface.
	if cs, ok := src.(*classic.Source); ok {
		w, warnings, err := cs.LoadWithWarnings(ctx)
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

// cmdWorldLint checks the world files and reports what it finds. It replaces
// the C tree's src/util/scheck and its -c mode, and unlike either it can run
// in CI without starting a server.
func cmdWorldLint(args []string) error {
	fs := flag.NewFlagSet("world lint", flag.ContinueOnError)
	dir, format, mini := worldFlags(fs)
	quiet := fs.Bool("quiet", false, "Suppress informational findings")
	strict := fs.Bool("strict", false, "Exit non-zero on warnings as well as errors")
	if err := fs.Parse(args); err != nil {
		return err
	}

	dump, findings, err := loadWorld(context.Background(), *dir, *format, *mini, world.Options{})
	if err != nil {
		// A load failure is itself the finding, and the most important one.
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return errQuiet
	}

	out := bufio.NewWriter(os.Stdout)
	defer func() { _ = out.Flush() }()

	// Report in severity order, worst first: a boot-blocking error should not
	// be buried under fifty informational lines.
	sorted := make([]classic.Warning, len(findings))
	copy(sorted, findings)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Severity > sorted[j].Severity
	})

	var counts [3]int
	for _, f := range sorted {
		if f.Severity >= 0 && int(f.Severity) < len(counts) {
			counts[f.Severity]++
		}
		if *quiet && f.Severity == classic.Info {
			continue
		}
		_, _ = fmt.Fprintf(out, "%s: %s\n", f.Severity, f.Message)
	}

	_, _ = fmt.Fprintf(out, "\n%d rooms, %d mobiles, %d objects, %d zones\n",
		dump.Counts.Rooms, dump.Counts.Mobiles, dump.Counts.Objects, dump.Counts.Zones)
	_, _ = fmt.Fprintf(out, "%d error(s), %d warning(s), %d note(s)\n",
		counts[classic.Error], counts[classic.Warn], counts[classic.Info])
	_ = out.Flush()

	if counts[classic.Error] > 0 {
		return errQuiet
	}
	if *strict && counts[classic.Warn] > 0 {
		return errQuiet
	}
	return nil
}

// cmdWorldDump writes the loaded world as canonical JSON, for diffing against
// the same dump produced by the C loader.
func cmdWorldDump(args []string) error {
	fs := flag.NewFlagSet("world dump", flag.ContinueOnError)
	dir, format, mini := worldFlags(fs)
	outPath := fs.String("out", "-", "Output file, or - for stdout")
	parity := fs.Bool("parity", false,
		"Omit fields the C server does not retain, for diffing against its dump")
	if err := fs.Parse(args); err != nil {
		return err
	}

	dump, _, err := loadWorld(context.Background(), *dir, *format, *mini,
		world.Options{Parity: *parity})
	if err != nil {
		return err
	}

	w := os.Stdout
	if *outPath != "-" {
		f, err := os.Create(*outPath) //nolint:gosec // operator-supplied path
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		w = f
	}

	bw := bufio.NewWriter(w)
	if err := world.WriteDump(bw, dump); err != nil {
		return err
	}
	return bw.Flush()
}
