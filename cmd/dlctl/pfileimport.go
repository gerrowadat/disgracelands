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

	objSrc, err := binary.NewObjectStore(player.Config{Dir: *fromDir, ReadOnly: true})
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

	ctx := context.Background()
	characters, withObjects, transcoded := 0, 0, 0
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

	_, _ = fmt.Fprintf(out, "\nimported %d character(s), %d with a rent/crash file\n", characters, withObjects)
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
