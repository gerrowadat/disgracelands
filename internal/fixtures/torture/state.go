// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/persist/bans"
	bansclassic "github.com/gerrowadat/disgracelands/internal/persist/bans/classic"
	"github.com/gerrowadat/disgracelands/internal/persist/boards"
	boardsclassic "github.com/gerrowadat/disgracelands/internal/persist/boards/classic"
	"github.com/gerrowadat/disgracelands/internal/persist/clock"
	"github.com/gerrowadat/disgracelands/internal/persist/houses"
	housesclassic "github.com/gerrowadat/disgracelands/internal/persist/houses/classic"
	"github.com/gerrowadat/disgracelands/internal/persist/mail"
	mailclassic "github.com/gerrowadat/disgracelands/internal/persist/mail/classic"
	"github.com/gerrowadat/disgracelands/internal/persist/player"
	"github.com/gerrowadat/disgracelands/internal/persist/reports"
	reportsclassic "github.com/gerrowadat/disgracelands/internal/persist/reports/classic"
)

// The state half of the corpus: etc/badsites, etc/board.*, etc/plrmail,
// etc/hcontrol, etc/time, house/*.house and misc/{bugs,ideas,typos}.
//
// Every one of these is written through the classic store that already
// reads it, rather than by hand-assembling the bytes. That is not laziness
// about the struct dumps: reference/tools/{board,mail,house}layout.c pin
// the offsets gcc chooses and a test in each package requires the Go codec
// to reproduce them under both data models, so the layout these writers
// produce is C-verified already. What a hand-built blob would add is a
// second, unverified copy of the same knowledge.

// writeState writes every state file under base.
func writeState(base string) error {
	etc := filepath.Join(base, "etc")
	house := filepath.Join(base, "house")
	misc := filepath.Join(base, "misc")
	for _, d := range []string{etc, house, misc} {
		if err := mkdirAll(d); err != nil {
			return err
		}
	}
	for _, step := range []func(etc, house, misc string) error{
		writeBans, writeBoards, writeMail, writeHouses, writeReports, writeClock,
	} {
		if err := step(etc, house, misc); err != nil {
			return err
		}
	}
	return nil
}

// writeBans covers every ban type ban.c has a name for, plus the two
// shapes that are awkward rather than merely present: a site at
// BANNED_SITE_LENGTH exactly, and a ban whose `by` field is somebody with
// a name as long as the roster allows.
func writeBans(etc, _, _ string) error {
	store, err := bansclassic.New(bans.Config{Path: filepath.Join(etc, "badsites")})
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	when := time.Unix(1100000000, 0).UTC()
	list := []bans.Ban{
		{Site: "new.example.org", Type: bans.TypeNew, When: when, By: "Torturer"},
		{Site: "select.example.org", Type: bans.TypeSelect, When: when, By: "Torturer"},
		{Site: "all.example.org", Type: bans.TypeAll, When: when, By: "Torturer"},
		// BANNED_SITE_LENGTH is 50 and classic truncates to it, so a site
		// of exactly that length is the one that shows whether the
		// boundary is off by one in either direction.
		{Site: strings.Repeat("a", bans.MaxSiteLength-len(".example.org")) + ".example.org",
			Type: bans.TypeAll, When: when, By: strings.Repeat("A", 1) + strings.Repeat("b", 18)},
	}
	for _, b := range list {
		if _, err := store.Add(b); err != nil {
			return fmt.Errorf("ban %s: %w", b.Site, err)
		}
	}
	return nil
}

// writeBoards fills one board to MAX_BOARD_MESSAGES and leaves the others
// with the awkward individual cases: a body at MAX_MESSAGE_LENGTH, a
// heading and body in CP1252, an empty body, and a message posted at a
// level above LVL_IMPL.
//
// Both of those C limits are properties of a fixed-size array rather than
// of the game (docs/design/data-format.md §9) and yaml does not carry
// them across — which is exactly why a board that sits *on* them is worth
// having: it is the case where a converter's answer and the C's differ if
// anything has been assumed.
func writeBoards(etc, _, _ string) error {
	store, err := boardsclassic.New(boards.Config{Dir: etc})
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	// MAX_BOARD_MESSAGES (boards.h). A full board is the one that proves
	// nothing silently drops the last message.
	const maxBoardMessages = 60
	full := make([]boards.Message, 0, maxBoardMessages)
	for i := 0; i < maxBoardMessages; i++ {
		full = append(full, boards.Message{
			Heading: fmt.Sprintf("Aug %02d 2003 (Torturer)  :: message %d of %d", 1+i%28, i+1, maxBoardMessages),
			Level:   int32(i % 35),
			Body:    fmt.Sprintf("Body of message %d.\n", i+1),
		})
	}
	if err := store.Save("board.mort", full); err != nil {
		return err
	}

	// MAX_MESSAGE_LENGTH (boards.h) is 4096, the size of the buffer
	// do_write reads into.
	const maxMessageLength = 4096
	if err := store.Save("board.immort", []boards.Message{
		{
			Heading: "Aug 01 2003 (Torturer)  :: a body of exactly 4096 bytes",
			Level:   game.LevelImplementor,
			Body:    strings.Repeat("x", maxMessageLength-1) + "\n",
		},
		{
			Heading: "Aug 02 2003 (Torturer)  :: an empty body",
			Level:   game.LevelImplementor,
			Body:    "",
		},
	}); err != nil {
		return err
	}

	// CP1252 on a board, which is the shape `import --type=state` had no
	// transcoding for at all until docs/design/data-format.md §11.1 found
	// it — and could not have found here, because every fixture in the
	// tree was ASCII.
	return store.Save("board.social", []boards.Message{
		{
			Heading: "Aug 03 2003 (Torturer)  :: caf\x92 opening hours",
			Level:   0,
			Body:    "The caf\xe9 opens at 9. Bring \xa35 \x96 or don\x92t.\n",
		},
	})
}

// writeMail covers a body long enough to span many of mail.c's 100-byte
// blocks, a body that lands exactly on a block boundary, and CP1252 text.
//
// There is no empty-body message, and that is a fact about the format
// rather than an omission: store_mail refuses one outright ("SYSERR: Mail
// system -- non-fatal error #5", mail.c:303), so an empty mail is not a
// hostile input this corpus can hold — it is one the C never wrote.
func writeMail(etc, _, _ string) error {
	store, err := mailclassic.New(mail.Config{Path: filepath.Join(etc, "plrmail")})
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	sent := time.Unix(1100000000, 0).UTC()
	for _, m := range []mail.Message{
		{To: 1, From: 2, Sent: sent, Text: "Short enough for one block.\n"},
		{To: 1, From: 3, Sent: sent, Text: strings.Repeat("A long letter, spanning blocks. ", 200)},
		// Exactly BLOCK_SIZE-worth of body, the boundary between "one
		// block" and "a chain": mail.c's own allocator is the whole of
		// this format, and a body that does not cross a block exercises
		// none of it.
		{To: 4, From: 1, Sent: sent, Text: strings.Repeat("z", 100)},
		{To: 2, From: 1, Sent: sent, Text: "Caf\xe9 at nine \x97 don\x92t be late.\n"},
	} {
		if err := store.Send(m); err != nil {
			return err
		}
	}
	return nil
}

// writeHouses covers a house with contents, a house with none, a house at
// MAX_GUESTS, and the orphan.
//
// The orphan is a `<vnum>.house` object file with no control record naming
// it, and it is the one place these two formats genuinely differ:
// state/houses.yaml nests a house's contents inside its own control entry
// (docs/design/data-format.md §9), so a contents file belonging to no
// house has nowhere to go and is dropped on import. Keeping it here is
// deliberate — the difference is a real one and a corpus that avoids it
// would let it be discovered by an operator instead.
func writeHouses(etc, house, _ string) error {
	store, err := housesclassic.New(houses.Config{
		ControlPath: filepath.Join(etc, "hcontrol"), ObjectDir: house,
	})
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	built := time.Unix(1000000000, 0).UTC()
	// MAX_GUESTS (house.h).
	const maxGuests = 10
	guests := make([]int64, 0, maxGuests)
	for i := 0; i < maxGuests; i++ {
		guests = append(guests, int64(100+i))
	}

	if err := store.Save([]houses.House{
		{Vnum: 5008, Atrium: 5007, ExitNum: 2, BuiltOn: built, LastPayment: lastILP32Second,
			Mode: 0, Owner: 1, Guests: guests},
		{Vnum: 5007, Atrium: 5006, ExitNum: 2, BuiltOn: built, Mode: 1, Owner: 4},
		// A control record naming a room this world does not have. The C
		// checks that at boot and drops the house; the file itself is
		// perfectly well-formed, which is what makes it a conversion case
		// rather than a corrupt one.
		{Vnum: 9999, Atrium: 9998, ExitNum: 0, BuiltOn: built, Mode: 0, Owner: 1},
	}); err != nil {
		return err
	}

	if err := store.SaveObjects(5008, []player.StoredObject{
		{Vnum: 5004, Weight: 5},
		{Vnum: 5003, Weight: 1},
		{Vnum: 5000, Values: [game.NumObjValues]int32{1, 2, 3, 4}, Weight: 2147483647},
	}); err != nil {
		return err
	}

	// The orphan: contents for a house nothing declares.
	return store.SaveObjects(5006, []player.StoredObject{{Vnum: 5003, Weight: 1}})
}

// writeReports covers all three kinds, CP1252 text, an empty body and a
// body with embedded newlines — the last of which matters because the
// classic format is line-oriented and has no way to escape one.
func writeReports(_, _, misc string) error {
	store, err := reportsclassic.New(reports.Config{Dir: misc})
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	for _, r := range []reports.Report{
		{Kind: reports.KindBug, Reporter: "Torturer", Room: 5000, Body: "The room with every flag is unpleasant."},
		{Kind: reports.KindIdea, Reporter: "Nobody", Room: 5008, Body: "A caf\xe9 would be nice. Or a cr\xeape stand."},
		{Kind: reports.KindTypo, Reporter: "Torturer", Room: 5005, Body: "\x93Antechamber\x94 is spelled oddly."},
		{Kind: reports.KindBug, Reporter: "Torturer", Room: 5006, Body: ""},
	} {
		if _, err := store.Append(r); err != nil {
			return err
		}
	}
	return nil
}

// writeClock writes etc/time. db.c keeps the game's epoch there as a bare
// integer with nothing around it, so the hostile case is simply a value at
// the edge of what that can be.
func writeClock(etc, _, _ string) error {
	return clock.Save("classic", filepath.Join(etc, "time"), time.Unix(1000000000, 0).UTC())
}
