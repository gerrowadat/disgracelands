// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

// Command torture writes examples/torture/binary: a deliberately hostile
// legacy CircleMUD lib/ directory, built to break the conversion into
// yaml.
//
//	go run ./internal/fixtures/torture --out=examples/torture/binary
//
// Why this exists, from docs/design/yaml-only.md §5.1: there is no
// player file, no rent file, no board, no mail file and no ban list in any
// other checked-in fixture in this repository, and stock CircleMUD's text
// is pure ASCII throughout. So the entire binary/ascii-to-yaml player and
// state path — the part the yaml-only release makes the *only* path — was
// tested only against fixtures each test built for itself, which by
// construction contain what the test author thought to include. This
// directory is what the test author did not think to include.
//
// # Why a Go generator and not a C one
//
// The plan this implements says to generate the fixture "by the C tools
// where a C tool can generate it", and reference/tools/pfilegen.c is
// exactly that shape of program. It is not what happened, for a reason
// worth stating rather than leaving to look like a shortcut.
//
// The generator has to run in day-to-day CI. What makes a checked-in
// fixture trustworthy is a test that regenerates it and diffs
// (fixture_test.go, the same standard cmd/dlctl/import_test.go already
// holds examples/stock and examples/mini to), and that test is only worth
// anything if it runs on every push. A C generator for the ILP32 struct
// dumps needs gcc-multilib, which by CLAUDE.md's own day-to-day/release
// split is installed in release.yml and nowhere else — so a C generator
// buys a fixture nobody checks until a release.
//
// The layout knowledge that would have justified the C is already
// C-anchored anyway: reference/tools/{pfile,board,mail,house}layout.c
// print the offsets gcc chooses for each of these structs and a test in
// each package requires the Go codec to reproduce them, under both data
// models. Writing the fixture through those codecs uses that verified
// knowledge rather than duplicating it unverified.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	out := flag.String("out", "examples/torture/binary", "directory to write the corpus into")
	flag.Parse()

	if err := generate(*out); err != nil {
		fmt.Fprintf(os.Stderr, "torture: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("wrote %s\n", *out)
}

// generate writes the whole corpus into dir, replacing whatever is there.
//
// Replacing rather than merging matters: several of these files are
// append-only through their own stores (bans, mail, reports), so writing
// over a previous run without clearing it first would double every record
// and the result would still look plausible.
func generate(dir string) error {
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("clearing %s: %w", dir, err)
	}
	if err := mkdirAll(dir); err != nil {
		return err
	}

	for rel, body := range worldFiles() {
		if err := writeFile(worldPath(dir, rel), body); err != nil {
			return err
		}
	}
	for rel, body := range textFiles() {
		if err := writeFile(filepath.Join(dir, filepath.FromSlash(rel)), body); err != nil {
			return err
		}
	}
	if err := writeFile(filepath.Join(dir, "config", "game.yaml"), gameConfig); err != nil {
		return err
	}
	if err := writeRoster(dir); err != nil {
		return err
	}
	return writeState(dir)
}

// gameConfig is the game tuning, which `dlctl import` copies rather than
// converts (it is this project's own yaml in either directory). Two values
// are set away from config.c's defaults on purpose: a conversion that
// silently dropped this file would leave a server back on the defaults,
// and a fixture whose tuning *is* the defaults could not tell the
// difference.
const gameConfig = `# examples/torture: the game tuning, deliberately not at its defaults.
free_rent: false
max_bad_pws: 3
`

// mkdirAll creates a directory with the same 0o750 the importers use for
// their own output: data, not secrets, and nothing here has to be
// world-readable to be read.
func mkdirAll(dir string) error {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}
	return nil
}

func writeFile(path, body string) error {
	if err := mkdirAll(filepath.Dir(path)); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}
