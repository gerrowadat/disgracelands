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
	encName := fs.String("encoding", convert.DefaultEncoding,
		fmt.Sprintf("Source text encoding: %v", encodingNames()))
	if err := fs.Parse(args); err != nil {
		return err
	}

	enc, ok := convert.Encodings[*encName]
	if !ok {
		return fmt.Errorf("unknown encoding %q (have: %v)", *encName, encodingNames())
	}

	entries, err := help.Load("classic", *fromDir)
	if err != nil {
		return fmt.Errorf("reading %s: %w", *fromDir, err)
	}
	transcoded := 0
	for i := range entries {
		if entries[i].Body == "" || utf8.ValidString(entries[i].Body) {
			continue
		}
		if out, err := enc.NewDecoder().String(entries[i].Body); err == nil {
			entries[i].Body = out
			transcoded++
		}
	}
	if err := help.Save("yaml", *toDir, entries); err != nil {
		return fmt.Errorf("writing %s: %w", filepath.Join(*toDir, help.YamlFile), err)
	}

	// text/help/screen is HELP_PAGE_FILE (db.h:78) -- what bare `help`
	// prints -- and it is not a help *entry*, so nothing above carries it.
	// That is right when converting in place, where it simply stays put,
	// and wrong when --to-dir is a different tree: `lib import` gives every
	// step its own destination, so a converted directory came out with no
	// screen in it at all, and bare `help` on a yaml server printed the
	// command list instead of the help screen. Copied rather than
	// converted, since internal/server/text.go reads it as plain prose from
	// the same path under either format.
	copied, err := copyHelpScreen(*fromDir, *toDir)
	if err != nil {
		return err
	}

	out := bufio.NewWriter(os.Stdout)
	_, _ = fmt.Fprintf(out, "help: imported %d\n", len(entries))
	if transcoded > 0 {
		_, _ = fmt.Fprintf(out, "transcoded %d help entries from %s to UTF-8\n", transcoded, *encName)
	}
	if copied {
		_, _ = fmt.Fprintf(out, "copied %s unchanged (not a help entry)\n", helpScreenName)
	}
	return out.Flush()
}

// helpScreenName is HELP_PAGE_FILE's own basename.
const helpScreenName = "screen"

// copyHelpScreen copies text/help/screen from one directory to another,
// reporting whether there was one to copy.
//
// A missing screen is not an error, for the same reason a missing motd is
// not: the server treats absent canned text as a poorer game and still a
// game (internal/server/text.go). Nothing happens when the two directories
// are the same, which is `helpdb import`'s own default.
func copyHelpScreen(fromDir, toDir string) (bool, error) {
	if fromDir == toDir {
		return false, nil
	}
	src := filepath.Join(fromDir, helpScreenName)
	if _, err := os.Stat(src); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("reading %s: %w", src, err)
	}
	// 0o750, matching copyTextFiles' own choice for the plain prose beside
	// this (cmd/dlctl/libimport.go): canned text is not a secret, but
	// nothing here has to be world-readable to be served.
	if err := os.MkdirAll(toDir, 0o750); err != nil {
		return false, fmt.Errorf("creating %s: %w", toDir, err)
	}
	// copyFile rather than ReadFile/WriteFile, for the same reason
	// `lib import`'s own text/ step uses it: one way of copying a file
	// across in this command, already the shape the linter is happy with.
	if err := copyFile(src, filepath.Join(toDir, helpScreenName)); err != nil {
		return false, err
	}
	return true, nil
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
