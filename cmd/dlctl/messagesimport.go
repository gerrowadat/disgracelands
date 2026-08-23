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

	"github.com/gerrowadat/disgracelands/internal/persist/messages"
)

// cmdMessagesImport converts misc/messages into config/messages.yaml,
// step 6c of docs/design/data-format.md §9 — its own command, separate
// from `names import` and `state import`, because --messages-format moves
// independently of both (see internal/config/config.go's own comment on
// why it is not folded into either).
func cmdMessagesImport(args []string) error {
	fs := flag.NewFlagSet("messages import", flag.ContinueOnError)
	fromPath := fs.String("from-path", "data/misc/messages", "Source (classic) messages file")
	toDir := fs.String("to-dir", "data/config", "Destination (yaml) directory")
	if err := fs.Parse(args); err != nil {
		return err
	}

	records, err := messages.Load("classic", *fromPath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", *fromPath, err)
	}
	if err := messages.Save("yaml", *toDir, records); err != nil {
		return fmt.Errorf("writing %s: %w", filepath.Join(*toDir, messages.YamlFile), err)
	}

	out := bufio.NewWriter(os.Stdout)
	_, _ = fmt.Fprintf(out, "messages: imported %d\n", len(records))
	return out.Flush()
}

// cmdMessagesFmt canonicalises a yaml messages directory in place.
func cmdMessagesFmt(args []string) error {
	fs := flag.NewFlagSet("messages fmt", flag.ContinueOnError)
	dir := fs.String("messages-dir", "data/config", "Yaml config directory")
	if err := fs.Parse(args); err != nil {
		return err
	}

	records, err := messages.Load("yaml", *dir)
	if err != nil {
		return fmt.Errorf("reading %s: %w", filepath.Join(*dir, messages.YamlFile), err)
	}
	if err := messages.Save("yaml", *dir, records); err != nil {
		return fmt.Errorf("writing %s: %w", filepath.Join(*dir, messages.YamlFile), err)
	}
	return nil
}
