// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package main

import (
	"sort"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"

	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/persist/convert"
)

func encodingNames() []string {
	names := make([]string, 0, len(convert.Encodings))
	for n := range convert.Encodings {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// transcodeString converts s from enc to UTF-8, leaving it alone if it is
// empty or already valid UTF-8 — shared by importBoards/importMail/
// importReports (stateio.go), the three state subsystems that carry free
// text. bans (a hostname substring and an admin's name) and houses
// (numeric fields and a StoredObject identified only by vnum, never by
// name or description) have nothing to transcode, so neither calls this.
func transcodeString(s *string, enc *charmap.Charmap) bool {
	if *s == "" || utf8.ValidString(*s) {
		return false
	}
	if out, err := enc.NewDecoder().String(*s); err == nil {
		*s = out
		return true
	}
	return false
}

// transcodeWorldStrings converts every text field in w from enc to UTF-8,
// in place, and returns how many fields actually needed it, only where the
// C loader treats the field as free text rather than a keyword or symbol.
func transcodeWorldStrings(w *game.World, enc *charmap.Charmap) int {
	n := 0
	fix := func(s *string) {
		if utf8.ValidString(*s) {
			return
		}
		if out, err := enc.NewDecoder().String(*s); err == nil {
			*s = out
			n++
		}
	}
	for _, r := range w.Rooms {
		fix(&r.Name)
		fix(&r.Description)
		for _, e := range r.Exits {
			if e != nil {
				fix(&e.Description)
			}
		}
		for i := range r.ExtraDescs {
			fix(&r.ExtraDescs[i].Description)
		}
	}
	for _, m := range w.Mobiles {
		fix(&m.ShortDesc)
		fix(&m.LongDesc)
		fix(&m.Description)
	}
	for _, o := range w.Objects {
		fix(&o.ShortDesc)
		fix(&o.Description)
		fix(&o.ActionDesc)
		for i := range o.ExtraDescs {
			fix(&o.ExtraDescs[i].Description)
		}
	}
	for _, sh := range w.Shops {
		for i := range sh.Messages {
			fix(&sh.Messages[i])
		}
	}
	return n
}

// transcodePlayerStrings converts a record's free-text fields from enc to
// UTF-8 in place. Name is not included: it is a filename in every format
// that stores one, and ascii's own pathFor already refuses anything
// outside [a-z].
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

// transcodeFightMessages converts every message template's free-text
// fields from enc to UTF-8 in place.
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

// transcodeSocials converts every social's free-text message fields from
// enc to UTF-8 in place. Name is not included: it is the command word, not
// prose.
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
