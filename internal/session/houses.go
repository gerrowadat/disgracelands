// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package session

import (
	"fmt"
	"strings"
	"time"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// do_hcontrol and do_house, ported from house.c:505 and :525.
//
// `hcontrol` is a greater god's command for building and destroying houses;
// `house` is the owner's, for letting people in. Between them they are the
// whole of the housing interface — there is no rent, no purchase and no
// eviction, because `hcontrol pay` records a payment and nothing ever checks
// it.

// HouseKeeper is what these commands need from the server: the control file,
// the per-house object files, and the name/id lookups.
type HouseKeeper interface {
	// SaveControl writes the control file back. The world is passed in
	// because these are called from commands, which already run on the world
	// goroutine — the same reason RentSaver takes it.
	SaveControl(w *game.Live)
	// SaveHouse writes one house's contents.
	SaveHouse(w *game.Live, vnum game.RoomVnum)
	// DeleteHouse removes a house's object file.
	DeleteHouse(vnum game.RoomVnum)
	// IDByName is get_id_by_name.
	IDByName(name string) (int64, bool)
	// NameByID is get_name_by_id, or "" for somebody who is gone.
	NameByID(id int64) string
}

// isPrefixOf is the C's is_abbrev: a non-empty prefix match, so `hcontrol b`
// builds and `hcontrol d` destroys.
func isPrefixOf(word, full string) bool {
	return word != "" && strings.HasPrefix(full, strings.ToLower(word))
}

const hcontrolFormat = "Usage: hcontrol build <house vnum> <exit direction> <player name>\r\n" +
	"       hcontrol destroy <house vnum>\r\n" +
	"       hcontrol pay <house vnum>\r\n" +
	"       hcontrol show\r\n"

func doHcontrol(c *Context) error {
	if c.Houses == nil {
		c.Send("Houses are not enabled on this server.\r\n")
		return nil
	}
	word, rest := halfChop(c.Arg)

	switch {
	case isPrefixOf(word, "build"):
		hcontrolBuild(c, rest)
	case isPrefixOf(word, "destroy"):
		hcontrolDestroy(c, rest)
	case isPrefixOf(word, "pay"):
		hcontrolPay(c, rest)
	case isPrefixOf(word, "show"):
		hcontrolShow(c)
	default:
		c.Send("%s", hcontrolFormat)
	}
	return nil
}

// hcontrolBuild is hcontrol_build_house (house.c:350).
func hcontrolBuild(c *Context, arg string) {
	if len(c.World.Houses()) >= maxHouses {
		c.Send("Max houses already defined.\r\n")
		return
	}

	vnumArg, rest := oneArgument(arg)
	if vnumArg == "" {
		c.Send("%s", hcontrolFormat)
		return
	}
	vnum := game.RoomVnum(atoi(vnumArg))
	if c.World.Room(vnum) == nil {
		c.Send("No such room exists.\r\n")
		return
	}
	if c.World.FindHouse(vnum) != nil {
		c.Send("House already exists.\r\n")
		return
	}

	dirArg, rest := oneArgument(rest)
	if dirArg == "" {
		c.Send("%s", hcontrolFormat)
		return
	}
	dir, ok := game.ParseDirection(dirArg)
	if !ok {
		c.Send("'%s' is not a valid direction.\r\n", dirArg)
		return
	}
	exit := c.World.Exit(vnum, dir)
	if exit == nil || c.World.Room(exit.ToRoom) == nil {
		c.Send("There is no exit %s from room %d.\r\n", dir.String(), vnum)
		return
	}
	atrium := exit.ToRoom

	// "A house's exit must be a two-way door." The structural rule, and the
	// only one: a house is a dead end with one door, so there is exactly one
	// way in to guard.
	back := c.World.Exit(atrium, dir.Reverse())
	if back == nil || back.ToRoom != vnum {
		c.Send("A house's exit must be a two-way door.\r\n")
		return
	}

	nameArg, _ := oneArgument(rest)
	if nameArg == "" {
		c.Send("%s", hcontrolFormat)
		return
	}
	owner, ok := c.Houses.IDByName(nameArg)
	if !ok || owner < 0 {
		c.Send("Unknown player '%s'.\r\n", nameArg)
		return
	}

	c.World.AddHouse(&game.House{
		Vnum: vnum, Atrium: atrium, ExitNum: dir,
		BuiltOn: time.Now(), Mode: game.HouseModePrivate, Owner: owner,
	})
	c.Houses.SaveHouse(c.World, vnum)
	c.Send("House built.  Mazel tov!\r\n")
	c.Houses.SaveControl(c.World)
}

// maxHouses is MAX_HOUSES (house.h:1).
const maxHouses = 100

// hcontrolDestroy is hcontrol_destroy_house (house.c:439).
func hcontrolDestroy(c *Context, arg string) {
	if strings.TrimSpace(arg) == "" {
		c.Send("%s", hcontrolFormat)
		return
	}
	h := c.World.FindHouse(game.RoomVnum(atoi(arg)))
	if h == nil {
		c.Send("Unknown house.\r\n")
		return
	}

	c.Houses.DeleteHouse(h.Vnum)
	c.World.RemoveHouse(h)
	c.Send("House deleted.\r\n")
	c.Houses.SaveControl(c.World)
}

// hcontrolPay is hcontrol_pay_house (house.c:485).
//
// It records a payment and nothing anywhere reads the field. Houses were
// never actually charged for; somebody meant to add it and the only trace is
// this command and a timestamp.
func hcontrolPay(c *Context, arg string) {
	if strings.TrimSpace(arg) == "" {
		c.Send("%s", hcontrolFormat)
		return
	}
	h := c.World.FindHouse(game.RoomVnum(atoi(arg)))
	if h == nil {
		c.Send("Unknown house.\r\n")
		return
	}
	h.LastPayment = time.Now()
	c.Houses.SaveControl(c.World)
	c.Send("Payment recorded.\r\n")
}

// hcontrolShow is hcontrol_list_houses (house.c:303). `show houses` is the
// same listing — the C's do_show calls straight into it (act.wizard.c:2321).
func hcontrolShow(c *Context) {
	houses := c.World.Houses()
	if len(houses) == 0 {
		c.Send("No houses have been defined.\r\n")
		return
	}

	var b strings.Builder
	b.WriteString("Address  Atrium  Build Date  Guests  Owner        Last Paymt\r\n")
	b.WriteString("-------  ------  ----------  ------  ------------ ----------\r\n")
	for _, h := range houses {
		// "Avoid seeing <UNDEF> entries from self-deleted people. -gg
		// 6/21/98" (house.c:318): an owner who has deleted their character
		// takes the whole house off the listing rather than showing it
		// ownerless. The house itself is still there — House_boot's sanity
		// pass is what removes those, at boot, not this.
		owner := c.Houses.NameByID(h.Owner)
		if owner == "" {
			continue
		}
		fmt.Fprintf(&b, "%7d %7d  %-10s    %2d    %-12s %s\r\n",
			h.Vnum, h.Atrium, houseDate(h.BuiltOn, "Unknown"), len(h.Guests),
			capitaliseFirst(owner), houseDate(h.LastPayment, "None"))
		writeGuests(c, &b, h, true)
	}
	c.Send("%s", b.String())
}

// houseDate is the C's `*(timestr + 10) = '\0'` on asctime output: the same
// weekday-and-day truncation the boards use, and for the same reason.
//
// `zero` is what the caller wants for a timestamp of 0, and the listing wants
// two different words out of the same line: a house with no build date reads
// "Unknown" (house.c:327) and one that has never been paid for reads "None"
// (house.c:334).
func houseDate(t time.Time, zero string) string {
	if t.IsZero() {
		return zero
	}
	stamp := t.Format("Mon Jan _2 15:04:05 2006")
	if len(stamp) > 10 {
		stamp = stamp[:10]
	}
	return stamp
}

// doHouse is do_house (house.c:525): the owner's guest list.
func doHouse(c *Context) error {
	if c.Houses == nil {
		c.Send("Houses are not enabled on this server.\r\n")
		return nil
	}
	name, _ := oneArgument(c.Arg)

	room := c.World.Room(c.Character.Room)
	switch {
	case room == nil || !room.Flags.Has(game.RoomHouse):
		c.Send("You must be in your house to set guests.\r\n")
		return nil
	}
	h := c.World.FindHouse(c.Character.Room)
	if h == nil {
		c.Send("Um.. this house seems to be screwed up.\r\n")
		return nil
	}
	if c.Character.Record == nil || c.Character.Record.IDNum != h.Owner {
		c.Send("Only the primary owner can set guests.\r\n")
		return nil
	}
	if name == "" {
		listGuests(c, h, false)
		return nil
	}

	id, ok := c.Houses.IDByName(name)
	if !ok || id < 0 {
		c.Send("No such player.\r\n")
		return nil
	}
	if id == c.Character.Record.IDNum {
		c.Send("It's your house!\r\n")
		return nil
	}

	// One command for both directions: naming somebody who is already a
	// guest removes them.
	if h.RemoveGuest(id) {
		c.Houses.SaveControl(c.World)
		c.Send("Guest deleted.\r\n")
		return nil
	}
	if !h.AddGuest(id) {
		c.Send("You have too many guests.\r\n")
		return nil
	}
	c.Houses.SaveControl(c.World)
	c.Send("Guest added.\r\n")
	return nil
}

// listGuests is House_list_guests (house.c:602), for `house` — which prints
// nothing else, so it sends its own line.
func listGuests(c *Context, h *game.House, quiet bool) {
	var b strings.Builder
	writeGuests(c, &b, h, quiet)
	if b.Len() > 0 {
		c.Send("%s", b.String())
	}
}

// writeGuests is the body of it. `hcontrol show` prints one of these after
// each house's line, interleaved, so it appends rather than sending.
//
// A guest whose character has been deleted is skipped silently, which is the
// C's fix for "Avoid <UNDEF>. -gg 6/21/98" (house.c:616) — so the count in
// `hcontrol show` and the names listed here can disagree, and a list of
// nothing but deleted characters is a bare "  Guests: ".
func writeGuests(c *Context, b *strings.Builder, h *game.House, quiet bool) {
	if len(h.Guests) == 0 {
		if !quiet {
			b.WriteString("  Guests: None\r\n")
		}
		return
	}

	b.WriteString("  Guests: ")
	for _, id := range h.Guests {
		name := c.Houses.NameByID(id)
		if name == "" {
			continue
		}
		b.WriteString(capitaliseFirst(name) + " ")
	}
	b.WriteString("\r\n")
}
