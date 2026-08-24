// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"

	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/persist/convert"
	"github.com/gerrowadat/disgracelands/internal/persist/player"
	"github.com/gerrowadat/disgracelands/internal/persist/player/binary"
	"github.com/gerrowadat/disgracelands/internal/persist/player/yaml"
)

// cmdPfileImport converts a roster into yaml, per step 5's "getting
// there" — the players counterpart of `world import`. Rent/crash files are
// hardcoded to binary regardless of --from, same as cmd/dlmud/main.go's own
// wiring: they are not pluggable the way the roster is, since the C has one
// format for them.
func cmdPfileImport(args []string) error {
	fs := flag.NewFlagSet("pfile import", flag.ContinueOnError)
	fromFormat := fs.String("from", binary.FormatName, "Source player format")
	fromDir := fs.String("from-dir", "data/etc", "Source player directory")
	toDir := fs.String("to-dir", "data/players", "Destination (yaml) player directory")
	fromObjsDir := fs.String("from-objs-dir", "",
		"Source plrobjs/ directory (default: beside or inside --from-dir, whichever exists)")
	fromAliasDir := fs.String("from-alias-dir", "",
		"Source plralias/ directory (default: beside or inside --from-dir, whichever exists)")
	encName := fs.String("encoding", convert.DefaultEncoding,
		fmt.Sprintf("Source text encoding: %v", encodingNames()))
	if err := fs.Parse(args); err != nil {
		return err
	}

	enc, ok := convert.Encodings[*encName]
	if !ok {
		return fmt.Errorf("unknown encoding %q (have: %v)", *encName, encodingNames())
	}

	src, err := player.Open(*fromFormat, player.Config{Dir: *fromDir, ReadOnly: true})
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()

	objsDir, objsNote := resolveSubdir(*fromObjsDir, *fromDir, "plrobjs")
	objSrc, err := binary.NewObjectStore(player.Config{
		Dir: *fromDir, ObjectsDir: objsDir, ReadOnly: true,
	})
	if err != nil {
		return err
	}

	aliasDir, aliasNote := resolveSubdir(*fromAliasDir, *fromDir, "plralias")
	aliasSrc, err := binary.NewAliasStore(player.Config{
		Dir: *fromDir, AliasDir: aliasDir, ReadOnly: true,
	})
	if err != nil {
		return err
	}

	dst, err := yaml.New(player.Config{Dir: *toDir})
	if err != nil {
		return err
	}
	defer func() { _ = dst.Close() }()

	out := bufio.NewWriter(os.Stdout)
	defer func() { _ = out.Flush() }()
	for _, note := range []string{objsNote, aliasNote} {
		if note != "" {
			_, _ = fmt.Fprintln(out, note)
		}
	}

	ctx := context.Background()
	characters, withObjects, withAliases, transcoded := 0, 0, 0, 0
	for entry, err := range src.List(ctx) {
		if err != nil {
			_, _ = fmt.Fprintf(out, "listing: %v\n", err)
			continue
		}
		rec, err := src.Load(ctx, entry.Name)
		if err != nil {
			_, _ = fmt.Fprintf(out, "%s: %v\n", entry.Name, err)
			continue
		}
		// Aliases live in their own file per character, so they are folded
		// into the record before it is saved rather than arriving with it.
		// A character with none has no file at all (write_aliases removes
		// it), which is the ordinary case and not a failure.
		switch as, aerr := aliasSrc.LoadAliases(entry.Name); {
		case aerr == nil:
			rec.Aliases = as
			withAliases++
		case errors.Is(aerr, player.ErrNotFound):
		default:
			_, _ = fmt.Fprintf(out, "%s: aliases: %v\n", entry.Name, aerr)
		}

		transcoded += transcodePlayerStrings(rec, enc)
		if err := dst.Save(ctx, rec); err != nil {
			_, _ = fmt.Fprintf(out, "%s: writing: %v\n", entry.Name, err)
			continue
		}
		characters++

		f, err := objSrc.LoadObjects(ctx, entry.Name)
		switch {
		case errors.Is(err, player.ErrNotFound):
			// No rent/crash file: every character who has never left the
			// game carrying anything.
		case err != nil:
			_, _ = fmt.Fprintf(out, "%s: reading rent file: %v\n", entry.Name, err)
		default:
			if err := dst.SaveObjects(ctx, entry.Name, f); err != nil {
				_, _ = fmt.Fprintf(out, "%s: writing rent file: %v\n", entry.Name, err)
				continue
			}
			withObjects++
		}
	}

	_, _ = fmt.Fprintf(out, "\nimported %d character(s), %d with a rent/crash file, %d with aliases\n",
		characters, withObjects, withAliases)
	if transcoded > 0 {
		_, _ = fmt.Fprintf(out, "transcoded %d string(s) from %s to UTF-8\n", transcoded, *encName)
	}
	return out.Flush()
}

// cmdPfileFmt canonicalises a yaml player directory in place: load and
// immediately re-save every character. Store.Save's own read-merge-write
// (yaml.go) already preserves each file's rent/inventory section
// untouched, so this reformats the roster half only, idempotently.
func cmdPfileFmt(args []string) error {
	fs := flag.NewFlagSet("pfile fmt", flag.ContinueOnError)
	dir := fs.String("player-dir", "data/players", "Yaml player data directory")
	if err := fs.Parse(args); err != nil {
		return err
	}

	s, err := yaml.New(player.Config{Dir: *dir})
	if err != nil {
		return err
	}
	defer func() { _ = s.Close() }()

	out := bufio.NewWriter(os.Stdout)
	defer func() { _ = out.Flush() }()

	ctx := context.Background()
	n := 0
	for entry, err := range s.List(ctx) {
		if err != nil {
			_, _ = fmt.Fprintf(out, "listing: %v\n", err)
			continue
		}
		rec, err := s.Load(ctx, entry.Name)
		if err != nil {
			_, _ = fmt.Fprintf(out, "%s: %v\n", entry.Name, err)
			continue
		}
		if err := s.Save(ctx, rec); err != nil {
			_, _ = fmt.Fprintf(out, "%s: writing: %v\n", entry.Name, err)
			continue
		}
		n++
	}
	_, _ = fmt.Fprintf(out, "formatted %d character(s)\n", n)
	return out.Flush()
}

// transcodePlayerStrings converts a record's free-text fields from enc to
// UTF-8 in place, mirroring worldimport.go's transcodeWorldStrings. Name is
// not included: it is a filename in every format that stores one, and
// ascii's own pathFor already refuses anything outside [a-z].
func transcodePlayerStrings(rec *game.PlayerRecord, enc *charmap.Charmap) int {
	n := 0
	fix := func(s *string) {
		if *s == "" || utf8.ValidString(*s) {
			return
		}
		if out, err := enc.NewDecoder().String(*s); err == nil {
			*s = out
			n++
		}
	}
	fix(&rec.Title)
	fix(&rec.Description)
	return n
}

// resolveSubdir finds a per-character subdirectory (plrobjs/, plralias/)
// that goes with a roster directory, and says which one it picked.
//
// Two layouts are both real and neither is wrong. This port keeps a roster
// and the rent files that belong to it in one directory, so plrobjs/ is a
// child of --from-dir. The C keeps `etc/players` and `plrobjs/` as siblings
// under lib/, because it builds both paths from its own cwd (db.h's
// PLAYER_FILE and LIB_PLROBJS) — so an archived tree pointed at with
// `--from-dir=lib/etc` has its rent files one level up.
//
// Guessing between them beats the alternative, which is what this used to
// do: look only in the first place, find nothing, and report "0 with a
// rent/crash file" — a sentence that reads like a fact about the roster
// and was actually a fact about the path. A character with no rent file is
// completely ordinary, so there was nothing here to look wrong.
//
// --from-objs-dir / --from-alias-dir override, for a layout that is
// neither.
func resolveSubdir(explicit, fromDir, name string) (dir string, note string) {
	if explicit != "" {
		return explicit, ""
	}
	own := filepath.Join(fromDir, name)
	if isDir(own) {
		return own, ""
	}
	sibling := filepath.Join(filepath.Dir(filepath.Clean(fromDir)), name)
	if isDir(sibling) {
		return sibling, fmt.Sprintf("%s: reading %s (the C's lib/ layout, beside %s rather than inside it)",
			name, sibling, fromDir)
	}
	// Neither exists. Keep the port's own layout, so the message a caller
	// gets names the place they most likely meant.
	return own, ""
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
