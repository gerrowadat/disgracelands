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
	"github.com/gerrowadat/disgracelands/internal/persist/player/ascii"
	"github.com/gerrowadat/disgracelands/internal/persist/world"
	"github.com/gerrowadat/disgracelands/internal/persist/world/classic"
)

// dumpTypes is who `dlctl dump` supports today: world and pfile.
var dumpTypes = []dirType{typeWorld, typePfile}

// cmdDump prints what is in a directory, for the --type given.
func cmdDump(args []string) error {
	fs := flag.NewFlagSet("dump", flag.ContinueOnError)
	typeRaw := fs.String("type", "", "Subsystem to dump: "+joinTypes(dumpTypes))
	dir := fs.String("dir", "data", "Data directory (base)")
	format := fs.String("format", "", "Format (default: classic for world, ascii for pfile)")
	outPath := fs.String("out", "-", "Output file, or - for stdout (--type=world only)")
	parity := fs.Bool("parity", false,
		"Omit fields the C server does not retain, for diffing against its dump (--type=world only)")
	mini := fs.Bool("mini-mud", false, "Use the reduced index.mini file list (--type=world only)")
	name := fs.String("name", "", "Print this character in full instead of listing the roster (--type=pfile only)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	t, err := parseType(*typeRaw, dumpTypes)
	if err != nil {
		return err
	}
	switch t {
	case typeWorld:
		f := *format
		if f == "" {
			f = classic.FormatName
		}
		return dumpWorld(*dir, f, *mini, *outPath, *parity)
	case typePfile:
		f := *format
		if f == "" {
			f = ascii.FormatName
		}
		return dumpPfile(*dir, f, *name)
	default:
		return fmt.Errorf("dump: unsupported --type %q", t)
	}
}

// dumpWorld writes the loaded world as canonical JSON, for diffing against
// the same dump produced by the C loader.
func dumpWorld(base, format string, mini bool, outPath string, parity bool) error {
	dir, err := resolveDir(typeWorld, base, format)
	if err != nil {
		return err
	}
	dump, _, err := loadWorld(context.Background(), dir, format, mini, world.Options{Parity: parity})
	if err != nil {
		return err
	}

	w := os.Stdout
	if outPath != "-" {
		f, err := os.Create(outPath) //nolint:gosec // operator-supplied path
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

// dumpPfile prints a roster, or one character in full. It replaces
// reference/tools/pfiledump.c, and unlike it reads the binary format as
// well as the ascii one — and needs no 32-bit build to do it.
func dumpPfile(base, format, name string) error {
	dir, err := resolveDir(typePfile, base, format)
	if err != nil {
		return err
	}
	store, err := player.Open(format, player.Config{Dir: dir, ReadOnly: true})
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	out := bufio.NewWriter(os.Stdout)
	defer func() { _ = out.Flush() }()

	ctx := context.Background()
	if name != "" {
		rec, err := store.Load(ctx, name)
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
	p("Remort vector", fmt.Sprintf("%d (%s)", r.RemortVector.Raw(), r.RemortVector))
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
