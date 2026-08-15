// Command dlctl is the offline tooling for Disgracelands: format conversion,
// world linting, and the jobs the C tree spread across src/util/ and tools/.
//
// Phase 0 of docs/proposals/go-port-plan.md establishes the command structure. The
// subcommands that need the persistence layers report which phase implements
// them rather than pretending to work.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/gerrowadat/disgracelands/internal/buildinfo"
)

// errQuiet makes the process exit non-zero without printing anything further.
// A command that has already reported its findings in detail should not then
// append a one-line summary of the same failure.
var errQuiet = errors.New("")

// command is one dlctl subcommand. Groups are expressed by a space in the
// name ("world lint"), which keeps dispatch flat and the help text grouped.
type command struct {
	name    string
	summary string
	// phase is 0 for implemented commands, or the plan phase that will
	// implement it.
	phase int
	run   func(args []string) error
}

var commands = []command{
	{
		name:    "version",
		summary: "Print version information",
		run:     cmdVersion,
	},
	{
		name:    "world lint",
		summary: "Check world files for errors (replaces src/util/scheck and dlmud -c)",
		run:     cmdWorldLint,
	},
	{
		name:    "world dump",
		summary: "Dump the loaded world as canonical JSON, for parity diffing against the C loader",
		run:     cmdWorldDump,
	},
	{
		name:    "pfile convert",
		summary: "Convert player data between formats (replaces tools/bin2ascii.c, no 32-bit build needed)",
		phase:   2,
	},
	{
		name:    "pfile verify",
		summary: "Cross-check a converted player file against the binary original, field by field",
		phase:   2,
	},
	{
		name:    "pfile dump",
		summary: "Print a player file in any supported format (replaces tools/pfiledump.c)",
		phase:   2,
	},
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		if !errors.Is(err, errQuiet) {
			fmt.Fprintf(os.Stderr, "dlctl: %v\n", err)
		}
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		usage(os.Stdout)
		return nil
	}

	// Match the longest command name first so "world lint" wins over a
	// hypothetical "world".
	sorted := make([]command, len(commands))
	copy(sorted, commands)
	sort.Slice(sorted, func(i, j int) bool {
		return len(sorted[i].name) > len(sorted[j].name)
	})

	joined := strings.Join(args, " ")
	for _, c := range sorted {
		if joined != c.name && !strings.HasPrefix(joined, c.name+" ") {
			continue
		}
		rest := args[len(strings.Fields(c.name)):]
		if c.run == nil {
			return fmt.Errorf("%q is not implemented yet: it lands in Phase %d, see docs/proposals/go-port-plan.md §10", c.name, c.phase)
		}
		return c.run(rest)
	}

	usage(os.Stderr)
	return fmt.Errorf("unknown command %q", joined)
}

func usage(w io.Writer) {
	var b strings.Builder
	b.WriteString("dlctl - offline tooling for Disgracelands\n\n")
	b.WriteString("Usage: dlctl <command> [options]\n\nCommands:\n")

	width := 0
	for _, c := range commands {
		if len(c.name) > width {
			width = len(c.name)
		}
	}
	for _, c := range commands {
		status := ""
		if c.run == nil {
			status = fmt.Sprintf(" (Phase %d)", c.phase)
		}
		fmt.Fprintf(&b, "  %-*s  %s%s\n", width, c.name, c.summary, status)
	}

	_, _ = io.WriteString(w, b.String())
}

func cmdVersion(args []string) error {
	fs := flag.NewFlagSet("version", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	fmt.Println(strings.Replace(buildinfo.Get().String(), "dlmud", "dlctl", 1))
	return nil
}
