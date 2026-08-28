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

	"github.com/gerrowadat/disgracelands/internal/persist/convert"
	"github.com/gerrowadat/disgracelands/internal/persist/player"
	"github.com/gerrowadat/disgracelands/internal/persist/player/binary"
)

// verifyTypes is who `dlctl verify` supports. Every type, now — the
// --against comparison below works for all seven, and the older
// "does this decode" report is pfile-only and says so when asked for
// anything else.
var verifyTypes = allTypes

// cmdVerify checks a directory and reports what it found.
//
// Two modes, which answer two different questions:
//
//   - On its own, "is this file what you think it is" — the report
//     binary's own Verify produces, for deciding whether to trust an
//     archived roster before migrating it. pfile only, because it is the
//     only format whose file is a fixed-size record array with a size
//     that can be checked against arithmetic.
//   - With --against, "did the conversion lose anything" — load both
//     directories through their own drivers and compare the loaded
//     states, subsystem by subsystem, reporting every difference rather
//     than the first.
//
// The second is the operator-facing form of the compatibility testing
// docs/proposals/yaml-only.md §5 describes: the thing you run against
// *your* archive, which this repository's own fixtures cannot cover
// because the real data is private. It is also what makes §5.2's tests
// cheap — they are this command, invoked from a test.
//
// It compares loaded state rather than bytes on purpose. §4.1 sets out
// why at length; the short version is that the claim worth making is "a
// server running on the converted data behaves identically", bytes are a
// lossy proxy for it in both directions, and building classic/ascii/binary
// *writers* to get a byte comparison would be about 3,000 lines whose only
// consumer is a test.
func cmdVerify(args []string) error {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	typeRaw := fs.String("type", "", "Subsystem to verify: "+joinTypes(verifyTypes)+
		" (omit with --against to verify every subsystem at once)")
	base := fs.String("dir", "data", "Data directory (base)")
	format := fs.String("format", "", "Format of --dir (default: binary for pfile, classic for everything else)")
	against := fs.String("against", "", "Second data directory (base) to compare --dir against")
	againstFormat := fs.String("against-format", "yaml", "Format of --against")
	encName := fs.String("encoding", convert.DefaultEncoding,
		fmt.Sprintf("Text encoding of whichever side is not yaml: %v", encodingNames()))
	mini := fs.Bool("mini-mud", false, "Use the reduced index.mini file list (--type=world only)")
	fromObjsDir := fs.String("objs-dir", "", "plrobjs/ directory for --dir (default: beside or inside it)")
	fromAliasDir := fs.String("alias-dir", "", "plralias/ directory for --dir (default: beside or inside it)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *against == "" {
		t, err := parseType(*typeRaw, []dirType{typePfile})
		if err != nil {
			return err
		}
		return verifyDecodes(t, *base, defaultFormat(t, *format))
	}

	enc, ok := convert.Encodings[*encName]
	if !ok {
		return fmt.Errorf("unknown encoding %q (have: %v)", *encName, encodingNames())
	}

	types := allTypes
	if *typeRaw != "" {
		t, err := parseType(*typeRaw, verifyTypes)
		if err != nil {
			return err
		}
		types = []dirType{t}
	}

	left := loadOptions{
		base: *base, format: *format, enc: enc, mini: *mini,
		objsDir: *fromObjsDir, aliasDir: *fromAliasDir,
	}
	right := loadOptions{base: *against, format: *againstFormat, enc: enc, mini: *mini}
	return verifyAgainst(types, left, right)
}

// defaultFormat fills in an unset --format per type, the same way
// cmdImportAll leaves fromFormat blank and lets each importer pick its
// own: a legacy archive is `binary` for the roster and `classic` for
// everything else, and one --format cannot be both. Getting this wrong is
// not subtle — `--format=classic` for a whole directory reports "unknown
// player format" for pfile and nothing at all for the six that worked.
func defaultFormat(t dirType, explicit string) string {
	if explicit != "" {
		return explicit
	}
	if t == typePfile {
		return binary.FormatName
	}
	return "classic"
}

// verifyAgainst compares two directories subsystem by subsystem.
func verifyAgainst(types []dirType, left, right loadOptions) error {
	out := bufio.NewWriter(os.Stdout)
	defer func() { _ = out.Flush() }()

	var failed []string
	for _, t := range types {
		l, r := left, right
		l.format = defaultFormat(t, left.format)
		r.format = defaultFormat(t, right.format)
		diffs, err := compareSubsystem(t, l, r)
		switch {
		case err != nil:
			_, _ = fmt.Fprintf(out, "%-8s error: %v\n", t, err)
			failed = append(failed, string(t))
		case len(diffs) == 0:
			_, _ = fmt.Fprintf(out, "%-8s identical\n", t)
		default:
			_, _ = fmt.Fprintf(out, "%-8s %s\n", t, summarise(diffs))
			failed = append(failed, string(t))
		}
	}

	if len(failed) > 0 {
		_, _ = fmt.Fprintf(out, "\n%s %s and %s do not load to the same state\n",
			joinTypes(toTypes(failed)), pluralVerb(len(failed)), left.base)
		if err := out.Flush(); err != nil {
			return err
		}
		return errQuiet
	}
	_, _ = fmt.Fprintf(out, "\n%s and %s load to the same state\n", left.base, right.base)
	return out.Flush()
}

// compareSubsystem loads one subsystem from both sides and diffs them.
//
// It is a separate function from verifyAgainst so that §5.2's Go tests can
// call it directly and assert on the difference list rather than on
// stdout — one implementation, two callers, which was the argument for
// building this as a command first.
func compareSubsystem(t dirType, left, right loadOptions) ([]string, error) {
	a, err := loadSubsystem(t, left)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", left.base, err)
	}
	b, err := loadSubsystem(t, right)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", right.base, err)
	}
	return diffValues(a, b), nil
}

func toTypes(names []string) []dirType {
	out := make([]dirType, len(names))
	for i, n := range names {
		out[i] = dirType(n)
	}
	return out
}

func pluralVerb(n int) string {
	if n == 1 {
		return "differs:"
	}
	return "differ:"
}

// verifyDecodes is the original `verify --type=pfile`: a report on one
// roster file, for answering "is this what you think it is" before
// trusting a migration.
func verifyDecodes(t dirType, base, format string) error {
	if format != binary.FormatName {
		return fmt.Errorf("verify --type=%s with no --against understands only the %q format; "+
			"use --against=<dir> --against-format=<fmt> to compare two directories", t, binary.FormatName)
	}
	dir, err := resolveDir(t, base, format)
	if err != nil {
		return err
	}

	store, err := binary.New(player.Config{Dir: dir, ReadOnly: true})
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	report, err := store.Verify(context.Background())
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return errQuiet
	}

	out := bufio.NewWriter(os.Stdout)
	defer func() { _ = out.Flush() }()

	_, _ = fmt.Fprintf(out, "%d bytes, %d records of %d\n", report.Bytes, report.Records, report.RecordSize)
	_, _ = fmt.Fprintf(out, "%d named character(s), %d empty slot(s)\n", report.Named, report.Empty)
	if report.LegacyPasswords > 0 {
		_, _ = fmt.Fprintf(out, "%d character(s) still on legacy crypt(3) passwords\n", report.LegacyPasswords)
	}
	for _, p := range report.Problems {
		_, _ = fmt.Fprintf(out, "problem: %s\n", p)
	}
	if err := out.Flush(); err != nil {
		return err
	}
	if len(report.Problems) > 0 {
		return errQuiet
	}
	return nil
}
