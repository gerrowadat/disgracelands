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
// in place, and returns how many fields actually needed it.
//
// Every string field, including the keyword lists. That is a change: this
// used to skip them deliberately, "only where the C loader treats the
// field as free text rather than a keyword or symbol", on the reasoning
// that a keyword is matched byte-for-byte by isname rather than shown to
// anybody.
//
// The reasoning was sound and the outcome was not. A keyword left in
// CP1252 is not valid UTF-8, and the yaml writer has to put it in a
// document that says it is — so the encoder substitutes U+FFFD for each
// offending byte, and `caf<0x92> sign` becomes `caf<REPLACEMENT> sign`.
// That matches nothing a player can type, in either encoding, and it
// cannot be undone. Decoding it as CP1252 at least produces the keyword
// the builder meant, in the encoding the server actually speaks.
//
// Found by examples/torture, which is the first fixture in this
// repository with a non-ASCII keyword in it. docs/design/data-format.md
// §11.1 records the same shape of gap in five of the seven importers, and
// the same reason it sat inert: stock CircleMUD's text is pure ASCII, so
// nothing here ever had a byte to get wrong.
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
				fix(&e.Keywords)
			}
		}
		for i := range r.ExtraDescs {
			fix(&r.ExtraDescs[i].Keywords)
			fix(&r.ExtraDescs[i].Description)
		}
	}
	for _, m := range w.Mobiles {
		fix(&m.Keywords)
		fix(&m.ShortDesc)
		fix(&m.LongDesc)
		fix(&m.Description)
	}
	for _, o := range w.Objects {
		fix(&o.Keywords)
		fix(&o.ShortDesc)
		fix(&o.Description)
		fix(&o.ActionDesc)
		for i := range o.ExtraDescs {
			fix(&o.ExtraDescs[i].Keywords)
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
	// Host, the poof messages and the aliases too. None of these is prose
	// in the sense the world importer's own split meant, and none of them
	// *should* hold a byte outside ASCII — a hostname by definition does
	// not, and an alias is a command word. But "should not" is not a
	// property of archived data, and the alternative to decoding a stray
	// byte is not leaving it alone: it is the YAML encoder substituting
	// U+FFFD for it, silently and irreversibly. Found by
	// FuzzBinaryRecordRoundTrip on a host field with a 0xff in it.
	//
	// Name is not here, and that is deliberate rather than an oversight:
	// it is the filename every format stores the character under, the
	// stores refuse anything outside [a-z] for exactly that reason, and a
	// name that needs decoding is a record that needs a person to look at
	// it rather than a converter to guess.
	fix(&rec.Host)
	fix(&rec.PoofIn)
	fix(&rec.PoofOut)
	for i := range rec.Aliases {
		fix(&rec.Aliases[i].Name)
		fix(&rec.Aliases[i].Replacement)
	}
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
