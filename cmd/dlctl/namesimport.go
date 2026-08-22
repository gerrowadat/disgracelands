// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gerrowadat/disgracelands/internal/persist/names"
)

// cmdNamesImport converts misc/xnames into config/names.yaml, step 6b of
// docs/proposals/data-format.md §9 — its own command, separate from `state
// import`, because it lives in a different directory (config/, not
// state/) and moves independently.
func cmdNamesImport(args []string) error {
	fs := flag.NewFlagSet("names import", flag.ContinueOnError)
	fromPath := fs.String("from-path", "data/misc/xnames", "Source (classic) xnames file")
	toDir := fs.String("to-dir", "data/config", "Destination (native) directory")
	if err := fs.Parse(args); err != nil {
		return err
	}

	list, err := names.Load("classic", *fromPath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", *fromPath, err)
	}
	if err := names.Save("native", *toDir, list); err != nil {
		return fmt.Errorf("writing %s: %w", filepath.Join(*toDir, names.NativeFile), err)
	}

	out := bufio.NewWriter(os.Stdout)
	_, _ = fmt.Fprintf(out, "names: imported %d\n", len(list))
	return out.Flush()
}

// cmdNamesFmt canonicalises a native names directory in place.
func cmdNamesFmt(args []string) error {
	fs := flag.NewFlagSet("names fmt", flag.ContinueOnError)
	dir := fs.String("names-dir", "data/config", "Native config directory")
	if err := fs.Parse(args); err != nil {
		return err
	}

	list, err := names.Load("native", *dir)
	if err != nil {
		return fmt.Errorf("reading %s: %w", filepath.Join(*dir, names.NativeFile), err)
	}
	if err := names.Save("native", *dir, list); err != nil {
		return fmt.Errorf("writing %s: %w", filepath.Join(*dir, names.NativeFile), err)
	}
	return nil
}
