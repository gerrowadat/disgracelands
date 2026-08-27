// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/gerrowadat/disgracelands/internal/buildinfo"
	"github.com/gerrowadat/disgracelands/internal/persist/dataversion"
)

// cmdDataVersion is `dlctl data version`: docs/design/data-format-
// versioning.md's own linter. Bare, it just says what release this build
// is, which is the version it stamps and checks with. With --dir, it also
// reads that directory's own .dlversion stamp (if any) and reports whether
// the two are compatible — the same check dlmud makes at boot
// (dataversion.Check), but printed rather than acted on, so an operator
// can ask the question before it costs them a failed start. --write stamps
// --dir with this build's release, which is the adoption path for a
// directory that predates the mechanism or one an older release wrote.
func cmdDataVersion(args []string) error {
	fs := flag.NewFlagSet("data version", flag.ContinueOnError)
	dir := fs.String("dir", "", "A data directory to check .dlversion in, if any")
	write := fs.Bool("write", false, "Stamp --dir with this build's release version (requires --dir)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	// An unreleased build — `go run`, `go test`, a plain `go build` with
	// no -ldflags — has no release version, so it has nothing to stamp
	// with and nothing to compare against. Every path below has to say so
	// rather than print a made-up number.
	current, released := dataversion.Current()
	raw := buildinfo.Get().Version

	if *write {
		if *dir == "" {
			return errors.New("data version --write needs --dir")
		}
		if !released {
			fmt.Fprintf(os.Stderr, "this build (%s) has no release version to stamp %s with; build with `make build`, or use a released dlctl\n", raw, *dir)
			return errQuiet
		}
		if err := dataversion.Write(*dir, current); err != nil {
			return err
		}
		fmt.Printf("Stamped %s as written by release %s.\n", *dir, current)
		return nil
	}

	if released {
		fmt.Printf("This is release %s; it stamps and reads data directories as %s.\n", raw, current)
	} else {
		fmt.Printf("This build (%s) has no release version: it stamps nothing, and checks nothing.\n", raw)
	}
	if *dir == "" {
		return nil
	}

	stamped, hasStamp, err := dataversion.Read(*dir)
	if err != nil {
		return err
	}
	if !hasStamp {
		fmt.Printf("%s has no %s — nothing to check.\n", *dir, dataversion.FileName)
		return nil
	}
	fmt.Printf("%s was written by release %s.\n", *dir, stamped)

	warning, err := dataversion.CheckBuild(*dir)
	switch {
	case err != nil:
		// CheckBuild's own error already names both versions and the
		// path; nothing to add beyond making clear this is a report, not
		// a command that itself failed to run.
		fmt.Println(err.Error())
		return errQuiet
	case warning != "":
		fmt.Println(warning)
	default:
		fmt.Printf("%s is compatible with this build.\n", *dir)
	}
	return nil
}
