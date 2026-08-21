// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

// Command dlctl is the offline tooling for Disgracelands: format conversion,
// world linting, and the jobs the C tree spread across its src/util/ and
// reference/tools/.
//
// Every subcommand listed here is implemented. The structure keeps room for
// ones that are not: a command declared ahead of the layer it needs reports
// which phase of docs/proposals/go-port-plan.md implements it rather than
// pretending to work.
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
		name:    "convert",
		summary: "Convert an old CircleMUD data directory into one the server can run on",
		run:     cmdConvert,
	},
	{
		name:    "world lint",
		summary: "Check world files for errors (replaces the C tree's scheck, and dlmud -c)",
		run:     cmdWorldLint,
	},
	{
		name:    "world dump",
		summary: "Dump the loaded world as canonical JSON, for parity diffing against the C loader",
		run:     cmdWorldDump,
	},
	{
		name:    "pfile convert",
		summary: "Convert player data between formats (replaces bin2ascii, with no 32-bit build)",
		run:     cmdPfileConvert,
	},
	{
		name:    "pfile verify",
		summary: "Check a player database decodes, and report what is in it",
		run:     cmdPfileVerify,
	},
	{
		name:    "pfile dump",
		summary: "List a roster, or print one character (replaces pfiledump)",
		run:     cmdPfileDump,
	},
	{
		name:    "pfile passwd",
		summary: "Set a character's password (the game itself has no way to)",
		run:     cmdPfilePasswd,
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
