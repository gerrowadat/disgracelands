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
	"text/tabwriter"
	"time"

	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/persist/player"
	"github.com/gerrowadat/disgracelands/internal/persist/player/binary"
)

func pfileFlags(fs *flag.FlagSet) (dir, format *string) {
	dir = fs.String("player-dir", "data/etc", "Directory holding the player data")
	format = fs.String("player-format", binary.FormatName,
		fmt.Sprintf("Player-file format: %v", player.Formats()))
	return
}

// cmdPfileDump prints a roster, or one character in full. It replaces
// reference/tools/pfiledump.c, and unlike it reads the binary format as well
// as the ascii one — and needs no 32-bit build to do it.
func cmdPfileDump(args []string) error {
	fs := flag.NewFlagSet("pfile dump", flag.ContinueOnError)
	dir, format := pfileFlags(fs)
	name := fs.String("name", "", "Print this character in full instead of listing the roster")
	if err := fs.Parse(args); err != nil {
		return err
	}

	store, err := player.Open(*format, player.Config{Dir: *dir, ReadOnly: true})
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	out := bufio.NewWriter(os.Stdout)
	defer func() { _ = out.Flush() }()

	ctx := context.Background()
	if *name != "" {
		rec, err := store.Load(ctx, *name)
		if err != nil {
			return err
		}
		printRecord(out, rec)
		return out.Flush()
	}

	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "NAME\tIDNUM\tLEVEL\tFLAGS")
	n := 0
	for e, err := range store.List(ctx) {
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(tw, "%s\t%d\t%d\t%s\n", e.Name, e.IDNum, e.Level, e.Flags)
		n++
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "\n%d character(s)\n", n)
	return out.Flush()
}

func printRecord(w *bufio.Writer, r *game.PlayerRecord) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	p := func(k string, v any) { _, _ = fmt.Fprintf(tw, "%s\t%v\n", k, v) }

	p("Name", r.Name)
	p("Title", r.Title)
	p("IDNum", r.IDNum)
	p("Level", r.Level)
	p("Class", r.Class)
	p("Sex", r.Sex)
	p("Hometown", r.Hometown)
	p("Alignment", r.Alignment)
	p("Birth", formatTime(r.Birth))
	p("Last logon", formatTime(r.LastLogon))
	p("Played", r.Played.Round(time.Minute))
	p("Host", r.Host)
	p("Password", describeCredential(r.Credential))
	p("Player flags", r.PlayerFlags)
	p("Affect flags", r.AffectFlags)
	p("Preferences", r.Preferences)
	p("Remort vector", fmt.Sprintf("%d (%s)", r.RemortVector, remortFlags(r.RemortVector)))
	p("Gold / bank", fmt.Sprintf("%d / %d", r.Points.Gold, r.Points.BankGold))
	p("Exp", r.Points.Exp)
	p("HP / mana / move", fmt.Sprintf("%d/%d  %d/%d  %d/%d",
		r.Points.Hit, r.Points.MaxHit, r.Points.Mana, r.Points.MaxMana,
		r.Points.Move, r.Points.MaxMove))
	p("Abilities", fmt.Sprintf("str %d/%d int %d wis %d dex %d con %d cha %d",
		r.Abilities.Strength, r.Abilities.StrengthPercentile, r.Abilities.Intelligence,
		r.Abilities.Wisdom, r.Abilities.Dexterity, r.Abilities.Constitution,
		r.Abilities.Charisma))
	p("Skills known", len(r.Skills))
	p("Affects", len(r.Affects))
	_ = tw.Flush()

	if r.Description != "" {
		_, _ = fmt.Fprintf(w, "\nDescription:\n%s\n", r.Description)
	}
}

// remortFlags renders the multiclass bitmask in the same letter form the
// world files use for their flags.
func remortFlags(v int32) game.Flags { return game.Flags(uint32(v)) } //nolint:gosec // reinterpretation, not truncation

func formatTime(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	return t.Format("2006-01-02")
}

func describeCredential(c game.Credential) string {
	switch c.Scheme {
	case game.SchemeNone:
		return "(none set)"
	case game.SchemeLegacyDES:
		// The hash itself is not printed. It is a real person's credential,
		// it is DES with a public salt, and printing it into a terminal
		// scrollback or a CI log is how it ends up somewhere it should not
		// be. That it exists and needs upgrading is the useful part.
		return "legacy crypt(3) DES - verify only, upgrade on next login"
	default:
		return string(c.Scheme)
	}
}

// cmdPfileVerify checks a player database and reports what it found. It is
// the tool for answering "is this file what I think it is" before trusting a
// migration.
func cmdPfileVerify(args []string) error {
	fs := flag.NewFlagSet("pfile verify", flag.ContinueOnError)
	dir, format := pfileFlags(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *format != binary.FormatName {
		return fmt.Errorf("pfile verify currently understands only the %q format", binary.FormatName)
	}

	store, err := binary.New(player.Config{Dir: *dir, ReadOnly: true})
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
