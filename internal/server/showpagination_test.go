// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"strconv"
	"strings"
	"testing"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// The pager's remaining call sites: a board's message list and a message's
// own body (boards.c:281,338), a shop's stock (shop.c:874), the `practice`
// skill list (spec_procs.c:193), and three of `show`'s fields — zones,
// and the errors/death/godrooms trio that share one helper (act.wizard.c's
// do_show, case 1 and cases 5-7, all page_string). `show player`, `show
// stats` and `show snoop` are checked *not* to page, matching send_to_char
// at their own call sites.

// TestBoardListingPaginates: enough messages on a board that `look board`
// (Board_show_board, boards.c:233) pages rather than sending the whole
// list at once.
func TestBoardListingPaginates(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Reader", "somanyposts", "m", "m")
	atBoard(t, srv, "Reader")

	inWorld(t, srv, func(w *game.Live) {
		b := w.BoardInRoom(BoardRoom)
		if b == nil {
			t.Error("no board in the room")
			return
		}
		for i := 0; i < 25; i++ {
			b.Messages = append(b.Messages, game.BoardMessage{
				Heading: "Aug 20 2026 (Someone)     :: message " + strconv.Itoa(i),
				Level:   1, Body: "Body.\r\n",
			})
		}
	})

	c.send("look board")
	c.expect("There are 25 messages on the board.")
	c.expect("Return to continue")
	if c.seen("message 24") {
		t.Error("the first page already shows content past PAGE_LENGTH")
	}

	c.send("")
	c.expect("message 24")
	c.expect("V > ")
}

// TestBoardMessagePaginates: one long message body (`read N`,
// Board_display_msg, boards.c:286) pages too, not just the list.
func TestBoardMessagePaginates(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Reader", "onelongpost", "m", "m")
	atBoard(t, srv, "Reader")

	var lines []string
	for i := 1; i <= 30; i++ {
		lines = append(lines, "Line "+strconv.Itoa(i)+".")
	}
	body := strings.Join(lines, "\r\n") + "\r\n"

	inWorld(t, srv, func(w *game.Live) {
		b := w.BoardInRoom(BoardRoom)
		if b == nil {
			t.Error("no board in the room")
			return
		}
		b.Messages = append(b.Messages, game.BoardMessage{
			Heading: "Aug 20 2026 (Author)      :: a long one", Level: 1, Body: body,
		})
	})

	c.send("read 1")
	c.expect("Message 1 : ")
	c.expect("Line 1.")
	c.expect("Return to continue")
	if c.seen("Line 22.") {
		t.Error("the first page already shows content past PAGE_LENGTH")
	}

	c.send("")
	c.expect("Line 22.")
	c.expect("V > ")
}

// TestShopListingPaginates: enough distinct items on the shelf that `list`
// (shop.c:874) pages. Identical items group into one line (same_obj), so
// this needs distinct prototypes, not just a big pile — testFillerVnumBase.
func TestShopListingPaginates(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Browser", "lotsofstock", "m", "m")
	inShop(t, srv, "Browser", 10_000)

	inWorld(t, srv, func(w *game.Live) {
		keeper := w.FindInRoom(nil, ShopRoom, "shopkeeper")
		if keeper == nil {
			t.Error("no shopkeeper")
			return
		}
		for i := 0; i < testFillerVnumCount; i++ {
			if obj := w.NewObject(testFillerVnumBase + game.ObjVnum(i)); obj != nil {
				w.ObjectToChar(obj, keeper)
			}
		}
		w.SortShopObjects(w.ShopFor(keeper), keeper)
	})

	// Asserted on the entry *number* rather than on a named item, because
	// which item lands last is a property of the list's insertion end and
	// this test is about PAGE_LENGTH. It named "filler item 19" until #193,
	// when obj_to_char started putting new objects at the head the way the
	// C's does (handler.c:418-419): the twenty fillers now list newest-first
	// ahead of the sword the keeper was already holding, so the line that
	// falls onto page two is the twenty-first either way, and it is no
	// longer a filler at all.
	before := len(c.transcript())
	c.send("list")
	c.expect("Available")
	c.expect("Return to continue")
	if strings.Contains(c.transcript()[before:], " 21)") {
		t.Error("the first page already shows content past PAGE_LENGTH")
	}

	c.send("")
	c.expect(" 21)")
	c.expect("V > ")
}

// TestPracticeListPaginates: the first character on an empty roster is an
// implementor (newTestServer) and, per KnowsSpell's remort-vector walk,
// knows every class's spells — comfortably enough for list_skills's single
// page_string call (spec_procs.c:193) to page.
func TestPracticeListPaginates(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Scholar", "everyspell", "m", "c")

	c.send("practice")
	c.expect("You know of the following")
	c.expect("Return to continue")

	c.send("q")
	c.expect("V > ")
}

// TestShowZonesPaginates: `show zones`'s bare-listing branch, with enough
// zones (including the filler ones) to actually need the pager act.wizard.c
// funnels all three of do_show's zone branches through.
func TestShowZonesPaginates(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Cartographer", "allthezones", "m", "w")

	c.send("show zones")
	c.expect("Midgaard")
	c.expect("Return to continue")

	c.send("q")
	c.expect("V > ")
}

// TestShowDeathPaginates: `show death` and `show godrooms` share one Go
// helper (showRooms) with `show errors`, all three ported from the same
// page_string call in act.wizard.c — proving one of them proves the
// mechanism for all three.
func TestShowDeathPaginates(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Auditor", "toomanytraps", "m", "w")

	inWorld(t, srv, func(w *game.Live) {
		for i := 0; i < testFillerRoomVnumCount; i++ {
			room := w.Room(testFillerRoomVnumBase + game.RoomVnum(i))
			if room == nil {
				t.Errorf("filler room %d missing", i)
				continue
			}
			room.Flags = room.Flags.Set(game.RoomDeathTrap)
		}
	})

	c.send("show death")
	c.expect("Death Traps")
	c.expect("Return to continue")

	c.send("q")
	c.expect("V > ")
}

// TestShowStatsDoesNotPaginate: `show stats` is send_to_char in the C
// (act.wizard.c's do_show, case STAT) and stays that way here — a short,
// fixed-size report has nothing to page regardless of world size.
func TestShowStatsDoesNotPaginate(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Counter", "nopagerhere", "m", "w")

	c.send("show stats")
	c.expect("Current stats:")
	c.settle()
	if c.seen("Return to continue") {
		t.Error("show stats triggered the pager")
	}
}

// TestShowPlayerDoesNotPaginate: `show player` is send_to_char too.
func TestShowPlayerDoesNotPaginate(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Registrar", "nopagereither", "m", "w")

	c.send("show player registrar")
	c.expect("Player: Registrar")
	c.settle()
	if c.seen("Return to continue") {
		t.Error("show player triggered the pager")
	}
}

// TestShowSnoopDoesNotPaginate: `show snoop` uses send_to_char for its
// headers and its body alike — no page_string anywhere in that branch.
func TestShowSnoopDoesNotPaginate(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	god := dialClient(t, addr)
	god.create("Overseer", "nopagerforspies", "m", "w")

	victim := dialClient(t, addr)
	victim.create("Observed", "beingwatchedtoo", "m", "w")
	setLevel(t, srv, "Observed", 10)

	god.send("snoop observed")
	god.expect("Okay.")

	god.send("show snoop")
	god.expect("People currently snooping:")
	god.settle()
	if god.seen("Return to continue") {
		t.Error("show snoop triggered the pager")
	}
}
