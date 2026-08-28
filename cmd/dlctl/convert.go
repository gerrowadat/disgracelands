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
	"strings"

	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/persist/convert"
	"github.com/gerrowadat/disgracelands/internal/persist/player"
	"github.com/gerrowadat/disgracelands/internal/persist/player/ascii"
	"github.com/gerrowadat/disgracelands/internal/persist/player/binary"
)

// convertTypes is who `dlctl convert --type=...` supports today: just
// pfile, the one subsystem with more than one runnable non-yaml format
// (binary and ascii) to reformat between.
var convertTypes = []dirType{typePfile}

// cmdConvert has two shapes, chosen by whether --type is given.
//
// With no --type, it turns a whole original CircleMUD data directory into
// one the server can run on: player database reformatted, text converted
// to UTF-8, and anything it does not understand left alone and reported —
// this is unchanged from the original `dlctl convert`.
//
// With --type=pfile, it copies a roster from one format to another,
// without going anywhere near yaml — this is what used to be the separate
// `dlctl convert --type=pfile` command. It is what makes the binary format an
// input rather than something the server has to live with: convert an old
// data directory once, and the server runs on a format whose fields are
// not fixed-width and whose password field is not eleven bytes. It
// replaces reference/tools/bin2ascii.c and, unlike it, needs no 32-bit
// build — the 32-bit layout is a parameter of the decoder rather than a
// property of the binary doing the decoding.
func cmdConvert(args []string) error {
	fs := flag.NewFlagSet("convert", flag.ContinueOnError)
	typeRaw := fs.String("type", "", "Subsystem to reformat: "+joinTypes(convertTypes)+
		" (omit to modernise a whole legacy lib directory in place, same formats, text re-encoded to UTF-8)")
	fromDir := fs.String("from-dir", "", "Source directory")
	toDir := fs.String("to-dir", "", "Destination directory")
	fromFormat := fs.String("from-format", binary.FormatName, "Source format (--type=pfile only)")
	toFormat := fs.String("to-format", ascii.FormatName, "Destination format (--type=pfile only)")
	encoding := fs.String("encoding", convert.DefaultEncoding,
		"Text encoding of the source (no --type only): "+strings.Join(encodingNames(), ", "))
	dryRun := fs.Bool("dry-run", false, "Report what would happen without writing anything")
	force := fs.Bool("force", false, "Overwrite an existing destination (no --type: a non-empty directory; "+
		"--type=pfile: characters that already exist there)")
	verbose := fs.Bool("verbose", false, "List every file, not just the ones that changed (no --type only)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *fromDir == "" || *toDir == "" {
		fs.Usage()
		return fmt.Errorf("both --from-dir and --to-dir are required")
	}

	if *typeRaw == "" {
		return cmdConvertLib(*fromDir, *toDir, *encoding, *dryRun, *force, *verbose)
	}
	t, err := parseType(*typeRaw, convertTypes)
	if err != nil {
		return err
	}
	return cmdConvertPfile(t, *fromDir, *fromFormat, *toDir, *toFormat, *dryRun, *force)
}

// cmdConvertLib turns an original CircleMUD data directory into one the
// server can run on: player database reformatted, text converted to
// UTF-8, and anything it does not understand left alone and reported.
func cmdConvertLib(from, to, encoding string, dryRun, force, verbose bool) error {
	enc, ok := convert.Encodings[encoding]
	if !ok {
		return fmt.Errorf("--encoding: unknown encoding %q (have: %s)",
			encoding, strings.Join(encodingNames(), ", "))
	}

	report, err := convert.Run(context.Background(), convert.Options{
		From: from, To: to, Encoding: enc,
		DryRun: dryRun, Force: force,
	})
	if err != nil {
		return err
	}

	out := bufio.NewWriter(os.Stdout)
	defer func() { _ = out.Flush() }()

	for _, e := range report.Entries {
		if e.Action == convert.Copied && !verbose {
			continue
		}
		if e.Note != "" {
			_, _ = fmt.Fprintf(out, "%-12s %s\n             %s\n", e.Action, e.Path, e.Note)
			continue
		}
		_, _ = fmt.Fprintf(out, "%-12s %s\n", e.Action, e.Path)
	}

	verb := "Converted"
	if dryRun {
		verb = "Would convert"
	}
	_, _ = fmt.Fprintf(out, "\n%s %s -> %s\n", verb, from, to)
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

// cmdConvertPfile copies a roster from one format to another under the
// given type's base directories.
func cmdConvertPfile(t dirType, fromBase, fromFormat, toBase, toFormat string, dryRun, force bool) error {
	fromDir, err := resolveDir(t, fromBase, fromFormat)
	if err != nil {
		return err
	}
	toDir, err := resolveDir(t, toBase, toFormat)
	if err != nil {
		return err
	}
	if fromFormat == toFormat && fromDir == toDir {
		return fmt.Errorf("source and destination are the same")
	}

	src, err := player.Open(fromFormat, player.Config{Dir: fromDir, ReadOnly: true})
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()

	dst, err := player.Open(toFormat, player.Config{Dir: toDir, ReadOnly: dryRun})
	if err != nil {
		return err
	}
	defer func() { _ = dst.Close() }()

	out := bufio.NewWriter(os.Stdout)
	defer func() { _ = out.Flush() }()

	ctx := context.Background()
	caps := dst.Capabilities()

	var converted, skipped, failed int
	for entry, err := range src.List(ctx) {
		if err != nil {
			return err
		}

		rec, err := src.Load(ctx, entry.Name)
		if err != nil {
			_, _ = fmt.Fprintf(out, "FAILED  %s: %v\n", entry.Name, err)
			failed++
			continue
		}

		// Refuse rather than truncate. A name that does not fit is a
		// different character, and finding that out after the conversion is
		// worse than not converting.
		if problems := checkFits(rec, caps); len(problems) > 0 {
			for _, p := range problems {
				_, _ = fmt.Fprintf(out, "FAILED  %s: %s\n", rec.Name, p)
			}
			failed++
			continue
		}

		if !force {
			exists, err := dst.Exists(ctx, rec.Name)
			if err != nil {
				return err
			}
			if exists {
				_, _ = fmt.Fprintf(out, "SKIP    %s: already exists in the destination (use --force to overwrite)\n", rec.Name)
				skipped++
				continue
			}
		}

		if dryRun {
			_, _ = fmt.Fprintf(out, "would convert %s\n", rec.Name)
			converted++
			continue
		}
		if err := dst.Save(ctx, rec); err != nil {
			_, _ = fmt.Fprintf(out, "FAILED  %s: %v\n", rec.Name, err)
			failed++
			continue
		}
		converted++
	}

	verb := "converted"
	if dryRun {
		verb = "would convert"
	}
	_, _ = fmt.Fprintf(out, "\n%s %d character(s), %d skipped, %d failed\n", verb, converted, skipped, failed)

	if caps.Supports(game.SchemeLegacyDES) && converted > 0 && !dryRun {
		_, _ = fmt.Fprintf(out, "\nPasswords were copied as-is; they are still legacy crypt(3) hashes.\n"+
			"They upgrade individually on each character's next successful login.\n")
	}

	if err := out.Flush(); err != nil {
		return err
	}
	if failed > 0 {
		return errQuiet
	}
	return nil
}

// checkFits reports what about a record the destination format cannot hold.
// A zero limit means the format imposes none.
func checkFits(rec *game.PlayerRecord, caps player.Capabilities) []string {
	var problems []string

	if caps.MaxNameLength > 0 && len(rec.Name) > caps.MaxNameLength {
		problems = append(problems, fmt.Sprintf("name is %d characters, destination holds %d",
			len(rec.Name), caps.MaxNameLength))
	}
	if caps.MaxTitleLength > 0 && len(rec.Title) > caps.MaxTitleLength {
		problems = append(problems, fmt.Sprintf("title is %d characters, destination holds %d",
			len(rec.Title), caps.MaxTitleLength))
	}
	if caps.MaxDescriptionLength > 0 && len(rec.Description) > caps.MaxDescriptionLength {
		problems = append(problems, fmt.Sprintf("description is %d characters, destination holds %d",
			len(rec.Description), caps.MaxDescriptionLength))
	}
	if caps.MaxAffects > 0 && len(rec.Affects) > caps.MaxAffects {
		problems = append(problems, fmt.Sprintf("has %d affects, destination holds %d",
			len(rec.Affects), caps.MaxAffects))
	}
	if caps.MaxSkillNumber > 0 {
		for num := range rec.Skills {
			if int(num) > caps.MaxSkillNumber {
				problems = append(problems, fmt.Sprintf("knows skill %d, destination holds up to %d",
					num, caps.MaxSkillNumber))
				break
			}
		}
	}
	if !caps.Supports(rec.Credential.Scheme) {
		problems = append(problems, fmt.Sprintf("password scheme %q is not storable in the destination",
			rec.Credential.Scheme))
	}
	return problems
}
