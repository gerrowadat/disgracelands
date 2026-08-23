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

	"github.com/gerrowadat/disgracelands/internal/persist/help"
)

// cmdHelpImport converts text/help/index plus the .hlp files it lists
// into text/help/help.yaml plus one .txt file per entry, step 6c of
// docs/design/data-format.md §7 — its own command, separate from
// `messages import`/`socials import`/`names import`, because
// --help-format moves independently of them (see
// internal/config/config.go's own comment on why). Named "helpdb", not
// "help" (see this command's own entry in cmd/dlctl/main.go's table for
// why "help ..." is unreachable).
//
// Unlike those three, --to-dir defaults to the same directory as
// --from-dir, mirroring `world import`'s own default (data/world for
// both): classic and yaml share text/help/ itself, distinguished by
// which files are present rather than by directory
// (internal/persist/help's own package doc explains why), so converting
// in place leaves index/*.hlp inert beside the new files rather than
// requiring a separate tree.
func cmdHelpImport(args []string) error {
	fs := flag.NewFlagSet("helpdb import", flag.ContinueOnError)
	fromDir := fs.String("from-dir", "data/text/help", "Source (classic) help directory")
	toDir := fs.String("to-dir", "data/text/help", "Destination (yaml) help directory")
	if err := fs.Parse(args); err != nil {
		return err
	}

	entries, err := help.Load("classic", *fromDir)
	if err != nil {
		return fmt.Errorf("reading %s: %w", *fromDir, err)
	}
	if err := help.Save("yaml", *toDir, entries); err != nil {
		return fmt.Errorf("writing %s: %w", filepath.Join(*toDir, help.YamlFile), err)
	}

	out := bufio.NewWriter(os.Stdout)
	_, _ = fmt.Fprintf(out, "help: imported %d\n", len(entries))
	return out.Flush()
}

// cmdHelpFmt canonicalises a yaml help directory in place.
func cmdHelpFmt(args []string) error {
	fs := flag.NewFlagSet("helpdb fmt", flag.ContinueOnError)
	dir := fs.String("help-dir", "data/text/help", "Yaml help directory")
	if err := fs.Parse(args); err != nil {
		return err
	}

	entries, err := help.Load("yaml", *dir)
	if err != nil {
		return fmt.Errorf("reading %s: %w", filepath.Join(*dir, help.YamlFile), err)
	}
	if err := help.Save("yaml", *dir, entries); err != nil {
		return fmt.Errorf("writing %s: %w", filepath.Join(*dir, help.YamlFile), err)
	}
	return nil
}
