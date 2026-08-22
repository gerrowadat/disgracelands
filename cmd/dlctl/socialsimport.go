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

	"github.com/gerrowadat/disgracelands/internal/persist/socials"
)

// cmdSocialsImport converts misc/socials into config/socials.yaml, step 6c
// of docs/proposals/data-format.md §7 — its own command, separate from
// `messages import` and `names import`, because --socials-format moves
// independently of both (see internal/config/config.go's own comment on
// why it is not folded into either).
func cmdSocialsImport(args []string) error {
	fs := flag.NewFlagSet("socials import", flag.ContinueOnError)
	fromPath := fs.String("from-path", "data/misc/socials", "Source (classic) socials file")
	toDir := fs.String("to-dir", "data/config", "Destination (native) directory")
	if err := fs.Parse(args); err != nil {
		return err
	}

	list, err := socials.Load("classic", *fromPath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", *fromPath, err)
	}
	if err := socials.Save("native", *toDir, list); err != nil {
		return fmt.Errorf("writing %s: %w", filepath.Join(*toDir, socials.NativeFile), err)
	}

	out := bufio.NewWriter(os.Stdout)
	_, _ = fmt.Fprintf(out, "socials: imported %d\n", len(list))
	return out.Flush()
}

// cmdSocialsFmt canonicalises a native socials directory in place.
func cmdSocialsFmt(args []string) error {
	fs := flag.NewFlagSet("socials fmt", flag.ContinueOnError)
	dir := fs.String("socials-dir", "data/config", "Native config directory")
	if err := fs.Parse(args); err != nil {
		return err
	}

	list, err := socials.Load("native", *dir)
	if err != nil {
		return fmt.Errorf("reading %s: %w", filepath.Join(*dir, socials.NativeFile), err)
	}
	if err := socials.Save("native", *dir, list); err != nil {
		return fmt.Errorf("writing %s: %w", filepath.Join(*dir, socials.NativeFile), err)
	}
	return nil
}
