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

	"github.com/gerrowadat/disgracelands/internal/persist/dataversion"
)

// cmdDataVersion is `dlctl data version`: docs/design/data-format-
// versioning.md's own linter. Bare, it just says what version of the yaml
// format this build understands. With --dir, it also reads that
// directory's own .dlversion stamp (if any) and reports whether the two
// are compatible — the same check dlmud makes at boot
// (dataversion.Check), but printed rather than acted on, so an operator
// can ask the question before it costs them a failed start. --write
// stamps --dir with the version this build understands, for a directory
// that predates the mechanism.
func cmdDataVersion(args []string) error {
	fs := flag.NewFlagSet("data version", flag.ContinueOnError)
	dir := fs.String("dir", "", "A data directory to check .dlversion in, if any")
	write := fs.Bool("write", false, "Stamp --dir with this build's version (requires --dir)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *write {
		if *dir == "" {
			return errors.New("data version --write needs --dir")
		}
		if err := dataversion.Write(*dir, dataversion.Current); err != nil {
			return err
		}
		fmt.Printf("Stamped %s with format version %s.\n", *dir, dataversion.Current)
		return nil
	}

	fmt.Printf("This build understands yaml data format version %s.\n", dataversion.Current)
	if *dir == "" {
		return nil
	}

	stampPath := *dir + string(os.PathSeparator) + dataversion.FileName
	warning, err := dataversion.Check(*dir, dataversion.Current)
	switch {
	case err != nil:
		// Check's own error already names both versions and the path;
		// nothing to add beyond making clear this is a report, not a
		// command that itself failed to run.
		fmt.Println(err.Error())
		return errQuiet
	case warning != "":
		fmt.Println(warning)
	default:
		if _, statErr := os.Stat(stampPath); os.IsNotExist(statErr) {
			fmt.Printf("%s has no %s — nothing to check.\n", *dir, dataversion.FileName)
		} else {
			fmt.Printf("%s is compatible with this build.\n", *dir)
		}
	}
	return nil
}
