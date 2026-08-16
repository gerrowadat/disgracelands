package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/persist/player"
	"github.com/gerrowadat/disgracelands/internal/persist/player/ascii"
	"github.com/gerrowadat/disgracelands/internal/persist/player/binary"
)

// cmdPfileConvert copies a roster from one format to another.
//
// This is what makes the binary format an input rather than something the
// server has to live with: convert an old data directory once, and the
// server runs on a format whose fields are not fixed-width and whose
// password field is not eleven bytes.
//
// It replaces reference/tools/bin2ascii.c and, unlike it, needs no 32-bit
// build — the 32-bit layout is a parameter of the decoder rather than a
// property of the binary doing the decoding.
func cmdPfileConvert(args []string) error {
	fs := flag.NewFlagSet("pfile convert", flag.ContinueOnError)
	from := fs.String("from", binary.FormatName, "Source format")
	fromDir := fs.String("from-dir", "data/etc", "Source directory")
	to := fs.String("to", ascii.FormatName, "Destination format")
	toDir := fs.String("to-dir", "data/pfiles", "Destination directory")
	dryRun := fs.Bool("dry-run", false, "Report what would be written without writing it")
	force := fs.Bool("force", false, "Overwrite characters that already exist in the destination")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *from == *to && *fromDir == *toDir {
		return fmt.Errorf("source and destination are the same")
	}

	src, err := player.Open(*from, player.Config{Dir: *fromDir, ReadOnly: true})
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()

	dst, err := player.Open(*to, player.Config{Dir: *toDir, ReadOnly: *dryRun})
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

		if !*force {
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

		if *dryRun {
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
	if *dryRun {
		verb = "would convert"
	}
	_, _ = fmt.Fprintf(out, "\n%s %d character(s), %d skipped, %d failed\n", verb, converted, skipped, failed)

	if caps.Supports(game.SchemeLegacyDES) && converted > 0 && !*dryRun {
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
