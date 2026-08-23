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
	"unicode/utf8"

	"github.com/gerrowadat/disgracelands/internal/persist/convert"
	"github.com/gerrowadat/disgracelands/internal/persist/names"
)

// cmdNamesImport converts misc/xnames into config/names.yaml, step 6b of
// docs/design/data-format.md §9 — its own command, separate from `state
// import`, because it lives in a different directory (config/, not
// state/) and moves independently.
func cmdNamesImport(args []string) error {
	fs := flag.NewFlagSet("names import", flag.ContinueOnError)
	fromPath := fs.String("from-path", "data/misc/xnames", "Source (classic) xnames file")
	toDir := fs.String("to-dir", "data/config", "Destination (yaml) directory")
	encName := fs.String("encoding", convert.DefaultEncoding,
		fmt.Sprintf("Source text encoding: %v", encodingNames()))
	if err := fs.Parse(args); err != nil {
		return err
	}

	enc, ok := convert.Encodings[*encName]
	if !ok {
		return fmt.Errorf("unknown encoding %q (have: %v)", *encName, encodingNames())
	}

	list, err := names.Load("classic", *fromPath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", *fromPath, err)
	}
	transcoded := 0
	for i := range list {
		if utf8.ValidString(list[i]) {
			continue
		}
		if out, err := enc.NewDecoder().String(list[i]); err == nil {
			list[i] = out
			transcoded++
		}
	}
	if err := names.Save("yaml", *toDir, list); err != nil {
		return fmt.Errorf("writing %s: %w", filepath.Join(*toDir, names.YamlFile), err)
	}

	out := bufio.NewWriter(os.Stdout)
	_, _ = fmt.Fprintf(out, "names: imported %d\n", len(list))
	if transcoded > 0 {
		_, _ = fmt.Fprintf(out, "transcoded %d string(s) from %s to UTF-8\n", transcoded, *encName)
	}
	return out.Flush()
}

// cmdNamesFmt canonicalises a yaml names directory in place.
func cmdNamesFmt(args []string) error {
	fs := flag.NewFlagSet("names fmt", flag.ContinueOnError)
	dir := fs.String("names-dir", "data/config", "Yaml config directory")
	if err := fs.Parse(args); err != nil {
		return err
	}

	list, err := names.Load("yaml", *dir)
	if err != nil {
		return fmt.Errorf("reading %s: %w", filepath.Join(*dir, names.YamlFile), err)
	}
	if err := names.Save("yaml", *dir, list); err != nil {
		return fmt.Errorf("writing %s: %w", filepath.Join(*dir, names.YamlFile), err)
	}
	return nil
}
