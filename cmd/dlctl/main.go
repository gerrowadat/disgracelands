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
		name:    "data version",
		summary: "Show or check the yaml data format's own version (docs/design/data-format-versioning.md)",
		run:     cmdDataVersion,
	},
	{
		name:    "parity session",
		summary: "Play a script against both servers and diff what they say",
		run:     cmdParitySession,
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
		name:    "world import",
		summary: "Convert a classic world directory into yaml (docs/design/data-format.md)",
		run:     cmdWorldImport,
	},
	{
		name:    "world fmt",
		summary: "Canonicalise a yaml world directory in place",
		run:     cmdWorldFmt,
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
	{
		name:    "pfile import",
		summary: "Convert a player roster into yaml, folding in rent/crash files (docs/design/data-format.md)",
		run:     cmdPfileImport,
	},
	{
		name:    "pfile fmt",
		summary: "Canonicalise a yaml player directory in place",
		run:     cmdPfileFmt,
	},
	{
		name:    "state import",
		summary: "Convert bans, boards, mail, houses, reports and the clock into yaml together (docs/design/data-format.md)",
		run:     cmdStateImport,
	},
	{
		name:    "state fmt",
		summary: "Canonicalise a yaml state directory in place",
		run:     cmdStateFmt,
	},
	{
		name:    "names import",
		summary: "Convert misc/xnames into config/names.yaml (docs/design/data-format.md)",
		run:     cmdNamesImport,
	},
	{
		name:    "names fmt",
		summary: "Canonicalise a yaml config/names.yaml in place",
		run:     cmdNamesFmt,
	},
	{
		name:    "messages import",
		summary: "Convert misc/messages into config/messages.yaml (docs/design/data-format.md)",
		run:     cmdMessagesImport,
	},
	{
		name:    "messages fmt",
		summary: "Canonicalise a yaml config/messages.yaml in place",
		run:     cmdMessagesFmt,
	},
	{
		name:    "socials import",
		summary: "Convert misc/socials into config/socials.yaml (docs/design/data-format.md)",
		run:     cmdSocialsImport,
	},
	{
		name:    "socials fmt",
		summary: "Canonicalise a yaml config/socials.yaml in place",
		run:     cmdSocialsFmt,
	},
	{
		// Named "helpdb", not "help": dlctl's own bare "help" is reserved
		// (run's own args[0] == "help" check, alongside -h/--help) for
		// printing this usage listing, before the command table is even
		// consulted — a subcommand group literally named "help ..."
		// would be unreachable. "helpdb" matches the term
		// internal/server/text.go's own Reload doc comment already uses
		// for this data ("xhelp is the help *database*").
		name:    "helpdb import",
		summary: "Convert text/help/index and its .hlp files into text/help/help.yaml (docs/design/data-format.md)",
		run:     cmdHelpImport,
	},
	{
		name:    "helpdb fmt",
		summary: "Canonicalise a yaml text/help/help.yaml in place",
		run:     cmdHelpFmt,
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
