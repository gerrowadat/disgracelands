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

	"golang.org/x/text/encoding/charmap"

	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/persist/convert"
	"github.com/gerrowadat/disgracelands/internal/persist/socials"
)

// cmdSocialsImport converts misc/socials into config/socials.yaml, step 6c
// of docs/design/data-format.md §7 — its own command, separate from
// `messages import` and `names import`, because --socials-format moves
// independently of both (see internal/config/config.go's own comment on
// why it is not folded into either).
func cmdSocialsImport(args []string) error {
	fs := flag.NewFlagSet("socials import", flag.ContinueOnError)
	fromPath := fs.String("from-path", "data/misc/socials", "Source (classic) socials file")
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

	list, err := socials.Load("classic", *fromPath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", *fromPath, err)
	}
	transcoded := transcodeSocials(list, enc)
	if err := socials.Save("yaml", *toDir, list); err != nil {
		return fmt.Errorf("writing %s: %w", filepath.Join(*toDir, socials.YamlFile), err)
	}

	out := bufio.NewWriter(os.Stdout)
	_, _ = fmt.Fprintf(out, "socials: imported %d\n", len(list))
	if transcoded > 0 {
		_, _ = fmt.Fprintf(out, "transcoded %d string(s) from %s to UTF-8\n", transcoded, *encName)
	}
	return out.Flush()
}

// transcodeSocials converts every social's free-text message fields from
// enc to UTF-8 in place, mirroring worldimport.go's transcodeWorldStrings.
// Name is not included: it is the command word, not prose.
func transcodeSocials(list []game.Social, enc *charmap.Charmap) int {
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
	for i := range list {
		fix(&list[i].CharNoArg)
		fix(&list[i].OthersNoArg)
		fix(&list[i].CharFound)
		fix(&list[i].OthersFound)
		fix(&list[i].VictFound)
		fix(&list[i].NotFound)
		fix(&list[i].CharAuto)
		fix(&list[i].OthersAuto)
	}
	return n
}

// cmdSocialsFmt canonicalises a yaml socials directory in place.
func cmdSocialsFmt(args []string) error {
	fs := flag.NewFlagSet("socials fmt", flag.ContinueOnError)
	dir := fs.String("socials-dir", "data/config", "Yaml config directory")
	if err := fs.Parse(args); err != nil {
		return err
	}

	list, err := socials.Load("yaml", *dir)
	if err != nil {
		return fmt.Errorf("reading %s: %w", filepath.Join(*dir, socials.YamlFile), err)
	}
	if err := socials.Save("yaml", *dir, list); err != nil {
		return fmt.Errorf("writing %s: %w", filepath.Join(*dir, socials.YamlFile), err)
	}
	return nil
}
