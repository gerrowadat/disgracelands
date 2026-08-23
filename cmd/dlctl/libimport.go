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
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/gerrowadat/disgracelands/internal/persist/convert"
	"github.com/gerrowadat/disgracelands/internal/persist/dataversion"
	"github.com/gerrowadat/disgracelands/internal/persist/player/binary"
)

// cmdLibImport converts a whole classic/binary lib/ directory into one
// fresh yaml directory in a single command, rather than the seven separate
// `world import`/`pfile import`/`state import`/`names import`/`messages
// import`/`socials import`/`helpdb import` calls examples/stock/
// README.md's own "How yaml/ was produced" section walks through by hand —
// this is that recipe, run in the same order and for the same reason
// (world first: it is independent of the rest, and a failure there is
// worth knowing about before the smaller conversions bother running).
//
// Two things beyond those seven: the eleven plain-text files directly
// under text/ (motd, credits, background, ...) are never a pluggable
// format (internal/server/text.go reads them from the same text/<name>
// path regardless of any --*-format flag), so they are copied unchanged
// rather than converted; and, once every step above has actually
// succeeded, --to-dir is stamped with this build's own format version
// (docs/design/data-format-versioning.md) — unlike the seven importers
// this wraps, which do not stamp on their own (a partial or repeated run
// against an existing directory is exactly the case a premature stamp
// would misrepresent), a --to-dir this command just finished writing from
// scratch has nothing ambiguous about what version it is.
func cmdLibImport(args []string) error {
	fs := flag.NewFlagSet("lib import", flag.ContinueOnError)
	fromDir := fs.String("from-dir", "", "Source lib/ directory (world/, etc/, misc/, house/, text/)")
	toDir := fs.String("to-dir", "", "Destination directory, written fresh in yaml throughout")
	playerFrom := fs.String("player-from", binary.FormatName, "Source player format")
	encName := fs.String("encoding", convert.DefaultEncoding,
		fmt.Sprintf("Source text encoding: %v", encodingNames()))
	mini := fs.Bool("mini-mud", false, "Use the reduced index.mini file list for the world")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *fromDir == "" || *toDir == "" {
		fs.Usage()
		return fmt.Errorf("both --from-dir and --to-dir are required")
	}

	worldArgs := []string{
		"--from-dir", filepath.Join(*fromDir, "world"),
		"--to-dir", filepath.Join(*toDir, "world"),
		"--encoding", *encName,
	}
	if *mini {
		worldArgs = append(worldArgs, "--mini-mud")
	}

	steps := []struct {
		label string
		args  []string
		run   func([]string) error
	}{
		{"world", worldArgs, cmdWorldImport},
		{"pfile", []string{
			"--from", *playerFrom,
			"--from-dir", filepath.Join(*fromDir, "etc"),
			"--to-dir", filepath.Join(*toDir, "players"),
			"--encoding", *encName,
		}, cmdPfileImport},
		{"state", []string{
			"--from-dir", filepath.Join(*fromDir, "etc"),
			"--from-house-dir", filepath.Join(*fromDir, "house"),
			"--from-misc-dir", filepath.Join(*fromDir, "misc"),
			"--to-dir", filepath.Join(*toDir, "state"),
		}, cmdStateImport},
		{"names", []string{
			"--from-path", filepath.Join(*fromDir, "misc", "xnames"),
			"--to-dir", filepath.Join(*toDir, "config"),
		}, cmdNamesImport},
		{"messages", []string{
			"--from-path", filepath.Join(*fromDir, "misc", "messages"),
			"--to-dir", filepath.Join(*toDir, "config"),
		}, cmdMessagesImport},
		{"socials", []string{
			"--from-path", filepath.Join(*fromDir, "misc", "socials"),
			"--to-dir", filepath.Join(*toDir, "config"),
		}, cmdSocialsImport},
		{"helpdb", []string{
			"--from-dir", filepath.Join(*fromDir, "text", "help"),
			"--to-dir", filepath.Join(*toDir, "text", "help"),
		}, cmdHelpImport},
	}

	var failed []string
	for _, s := range steps {
		fmt.Printf("== %s import ==\n", s.label)
		if err := s.run(s.args); err != nil {
			if !errors.Is(err, errQuiet) {
				fmt.Fprintf(os.Stderr, "%s import: %v\n", s.label, err)
			}
			failed = append(failed, s.label)
		}
		fmt.Println()
	}

	fmt.Println("== text ==")
	n, err := copyTextFiles(*fromDir, *toDir)
	switch {
	case err != nil:
		fmt.Fprintf(os.Stderr, "copying text/: %v\n", err)
		failed = append(failed, "text")
	default:
		fmt.Printf("copied %d plain text file(s) unchanged (not a pluggable format)\n", n)
	}
	fmt.Println()

	if len(failed) > 0 {
		fmt.Printf("Failed: %s. %s was not stamped with a format version — fix these and re-run.\n",
			strings.Join(failed, ", "), *toDir)
		return errQuiet
	}

	if err := dataversion.Write(*toDir, dataversion.Current); err != nil {
		return fmt.Errorf("stamping %s: %w", *toDir, err)
	}
	fmt.Printf("%s is a complete yaml directory, format version %s.\n", *toDir, dataversion.Current)
	return nil
}

// copyTextFiles copies every regular file directly inside fromDir/text —
// the plain-prose canned texts (motd, credits, background, ...) — into
// toDir/text, unchanged. text/help/ is a subdirectory, not a plain file,
// so os.ReadDir's own entries already exclude it without needing a name
// check; helpdb import is what converts it. A missing fromDir/text is not
// an error: the server treats missing canned text as "a poorer game, and
// still a game" (internal/server/text.go), and an archive with none is a
// legitimate source to import from.
func copyTextFiles(fromDir, toDir string) (int, error) {
	from := filepath.Join(fromDir, "text")
	entries, err := os.ReadDir(from)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("reading %s: %w", from, err)
	}

	to := filepath.Join(toDir, "text")
	if err := os.MkdirAll(to, 0o750); err != nil {
		return 0, fmt.Errorf("creating %s: %w", to, err)
	}

	var n int
	for _, e := range entries {
		if !e.Type().IsRegular() {
			continue
		}
		if err := copyFile(filepath.Join(from, e.Name()), filepath.Join(to, e.Name())); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

func copyFile(from, to string) error {
	src, err := os.Open(from) //nolint:gosec // operator-configured source directory
	if err != nil {
		return fmt.Errorf("reading %s: %w", from, err)
	}
	defer func() { _ = src.Close() }()

	dst, err := os.Create(to) //nolint:gosec // operator-configured destination directory
	if err != nil {
		return fmt.Errorf("writing %s: %w", to, err)
	}
	if _, err := io.Copy(dst, src); err != nil {
		_ = dst.Close()
		return fmt.Errorf("writing %s: %w", to, err)
	}
	return dst.Close()
}
