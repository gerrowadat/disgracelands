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
	encName := fs.String("encoding", convert.DefaultEncoding,
		fmt.Sprintf("Source text encoding: %v", encodingNames()))
	if err := fs.Parse(args); err != nil {
		return err
	}

	enc, ok := convert.Encodings[*encName]
	if !ok {
		return fmt.Errorf("unknown encoding %q (have: %v)", *encName, encodingNames())
	}

	records, err := messages.Load("classic", *fromPath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", *fromPath, err)
	}
	transcoded := transcodeFightMessages(records, enc)
	if err := messages.Save("yaml", *toDir, records); err != nil {
		return fmt.Errorf("writing %s: %w", filepath.Join(*toDir, messages.YamlFile), err)
	}

	out := bufio.NewWriter(os.Stdout)
	_, _ = fmt.Fprintf(out, "messages: imported %d\n", len(records))
	if transcoded > 0 {
		_, _ = fmt.Fprintf(out, "transcoded %d string(s) from %s to UTF-8\n", transcoded, *encName)
	}
	return out.Flush()
}

// transcodeFightMessages converts every message template's free-text
// fields from enc to UTF-8 in place, mirroring worldimport.go's
// transcodeWorldStrings.
func transcodeFightMessages(records []game.FightMessage, enc *charmap.Charmap) int {
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
	for i := range records {
		for _, set := range []*game.MsgSet{&records[i].Die, &records[i].Miss, &records[i].Hit, &records[i].God} {
			fix(&set.Attacker)
			fix(&set.Victim)
			fix(&set.Room)
		}
	}
	return n
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
