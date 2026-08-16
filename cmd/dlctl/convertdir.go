package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/gerrowadat/disgracelands/internal/persist/convert"
)

// cmdConvert turns an original CircleMUD data directory into one the server
// can run on: player database reformatted, text converted to UTF-8, and
// anything it does not understand left alone and reported.
func cmdConvert(args []string) error {
	fs := flag.NewFlagSet("convert", flag.ContinueOnError)
	from := fs.String("from", "", "The original CircleMUD data directory (its `lib`)")
	to := fs.String("to", "", "Where to write the converted directory")
	encoding := fs.String("encoding", convert.DefaultEncoding,
		"Text encoding of the source: "+strings.Join(encodingNames(), ", "))
	dryRun := fs.Bool("dry-run", false, "Report what would happen without writing anything")
	force := fs.Bool("force", false, "Write into a destination that is not empty")
	verbose := fs.Bool("verbose", false, "List every file, not just the ones that changed")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *from == "" || *to == "" {
		fs.Usage()
		return fmt.Errorf("both --from and --to are required")
	}

	enc, ok := convert.Encodings[*encoding]
	if !ok {
		return fmt.Errorf("--encoding: unknown encoding %q (have: %s)",
			*encoding, strings.Join(encodingNames(), ", "))
	}

	report, err := convert.Run(context.Background(), convert.Options{
		From: *from, To: *to, Encoding: enc,
		DryRun: *dryRun, Force: *force,
	})
	if err != nil {
		return err
	}

	out := bufio.NewWriter(os.Stdout)
	defer func() { _ = out.Flush() }()

	for _, e := range report.Entries {
		if e.Action == convert.Copied && !*verbose {
			continue
		}
		if e.Note != "" {
			_, _ = fmt.Fprintf(out, "%-12s %s\n             %s\n", e.Action, e.Path, e.Note)
			continue
		}
		_, _ = fmt.Fprintf(out, "%-12s %s\n", e.Action, e.Path)
	}

	verb := "Converted"
	if *dryRun {
		verb = "Would convert"
	}
	_, _ = fmt.Fprintf(out, "\n%s %s -> %s\n", verb, *from, *to)
	_, _ = fmt.Fprintf(out, "  %d file(s) copied unchanged\n", report.Count(convert.Copied))
	_, _ = fmt.Fprintf(out, "  %d transcoded to UTF-8\n", report.Count(convert.Transcoded))
	_, _ = fmt.Fprintf(out, "  %d reformatted\n", report.Count(convert.Reformatted))

	if n := report.Count(convert.Unsupported); n > 0 {
		_, _ = fmt.Fprintf(out, "  %d left untouched (see below)\n", n)
		_, _ = fmt.Fprintf(out, "\n%d file(s) are binary formats this cannot convert yet. They have been\n"+
			"copied exactly as they are, because a byte-level conversion would corrupt\n"+
			"them — they hold struct fields and length-prefixed text, not characters.\n"+
			"Each is converted by the phase that implements the subsystem reading it:\n\n", n)
		for _, e := range report.Entries {
			if e.Action == convert.Unsupported {
				_, _ = fmt.Fprintf(out, "  %s\n    %s\n", e.Path, e.Note)
			}
		}
	}

	for _, p := range report.Problems {
		_, _ = fmt.Fprintf(out, "\nproblem: %s\n", p)
	}

	if err := out.Flush(); err != nil {
		return err
	}
	if len(report.Problems) > 0 {
		return errQuiet
	}
	return nil
}

func encodingNames() []string {
	names := make([]string, 0, len(convert.Encodings))
	for n := range convert.Encodings {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
