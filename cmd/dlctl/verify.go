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

	"github.com/gerrowadat/disgracelands/internal/persist/player"
	"github.com/gerrowadat/disgracelands/internal/persist/player/binary"
)

// verifyTypes is who `dlctl verify` supports today: just pfile.
var verifyTypes = []dirType{typePfile}

// cmdVerify checks a directory and reports what it found. For --type=pfile
// it is the tool for answering "is this file what you think it is" before
// trusting a migration.
func cmdVerify(args []string) error {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	typeRaw := fs.String("type", "", "Subsystem to verify: "+joinTypes(verifyTypes))
	base := fs.String("dir", "data", "Data directory (base)")
	format := fs.String("format", binary.FormatName, "Format")
	if err := fs.Parse(args); err != nil {
		return err
	}
	t, err := parseType(*typeRaw, verifyTypes)
	if err != nil {
		return err
	}
	if *format != binary.FormatName {
		return fmt.Errorf("verify --type=pfile currently understands only the %q format", binary.FormatName)
	}
	dir, err := resolveDir(t, *base, *format)
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
