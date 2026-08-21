// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"strings"
	"testing"

	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/persist/bans"
)

// `ban`, `unban` and `show`, end to end.

func TestBanning(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Doorman", "keepthemout", "m", "w")

	c.send("ban")
	c.expect("No sites are banned.")

	c.send("ban example.com")
	c.expect("Usage: ban {all | select | new} site_name")

	c.send("ban sideways example.com")
	c.expect("Flag must be ALL, SELECT, or NEW.")

	// `no` is a type in the file and not one you may set.
	c.send("ban no example.com")
	c.expectCount("Flag must be ALL, SELECT, or NEW.", 2)

	c.send("ban all example.com")
	c.expect("Site banned.")

	c.send("ban new example.com")
	c.expect("That site has already been banned -- unban it to change the ban type.")

	c.send("ban")
	c.expect("Banned Site Name")
	c.expect("example.com")
	c.expect("Doorman")

	if got := srv.banFor("mail.example.com"); got != bans.TypeAll {
		t.Errorf("the ban is %v, want all", got)
	}

	c.send("unban")
	c.expect("A site to unban might help.")

	c.send("unban nowhere.invalid")
	c.expect("That site is not currently banned.")

	c.send("unban example.com")
	c.expect("Site unbanned.")

	if got := srv.banFor("example.com"); got != bans.TypeNone {
		t.Errorf("the site is still banned: %v", got)
	}
}

// A banned site is refused at the name prompt, which is where the C checks.
func TestABannedSiteIsRefused(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	god := dialClient(t, addr)
	god.create("Warden", "nobodycomesin", "m", "w")

	// The test listener is on the loopback, so banning it bans everybody —
	// which is exactly what makes it testable.
	god.send("ban all 127.0.0.1")
	god.expect("Site banned.")

	turned := dialClient(t, addr)
	turned.expect("By what name do you wish to be known?")
	turned.send("Hopeful")
	turned.expect("You are not welcome here.")
}

// A `new` ban lets existing characters in and keeps new ones out.
func TestANewBanOnlyStopsNewCharacters(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	god := dialClient(t, addr)
	god.create("Selector", "someofyoumay", "m", "w")

	// Somebody who already exists.
	existing := dialClient(t, addr)
	existing.create("Regular", "beenherebefore", "m", "w")
	existing.send("quit")
	existing.expect("Goodbye")
	existing.close()
	waitForLogout(t, srv, "Regular")
	srv.WaitForWrites()

	god.send("ban new 127.0.0.1")
	god.expect("Site banned.")

	fresh := dialClient(t, addr)
	fresh.expect("By what name do you wish to be known?")
	fresh.send("Newcomer")
	fresh.expect("Sorry, new characters are not allowed from your site!")

	back := dialClient(t, addr)
	back.expect("By what name do you wish to be known?")
	back.send("Regular")
	back.expect("Password:")
}

func TestShowOptions(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Inspector", "showmeeverything", "m", "w")

	c.send("show")
	c.expect("Show options:")
	c.settle()
	for _, want := range []string{"zones", "player", "stats", "errors", "godrooms", "snoop"} {
		if !c.seen(want) {
			t.Errorf("the options list does not mention %q", want)
		}
	}

	c.send("show nonsense")
	c.expect("Sorry, I don't understand that.")
}

func TestShowZones(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Cartographer", "showmethezones", "m", "w")

	c.send("show zones")
	c.expect("Midgaard")
	c.expect("Range:")

	c.send("show zones 30")
	c.expect("Midgaard")

	c.send("show zones 999")
	c.expect("That is not a valid zone.")

	// `.` is the zone you are standing in, which is 12 for the board room.
	c.send("show zones .")
	c.expect("The Immortal Zone")
}

func TestShowStats(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Counter", "howbigisit", "m", "w")

	c.send("show stats")
	c.expect("Current stats:")
	c.expect("players in game")
	c.expect("prototypes")
	c.expect("rooms")
}

func TestShowPlayer(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	god := dialClient(t, addr)
	god.create("Registrar", "wholiveshere", "m", "w")

	other := dialClient(t, addr)
	other.create("Subject", "onthebooks", "m", "w")
	setLevel(t, srv, "Subject", 10)
	other.send("quit")
	other.expect("Goodbye")
	other.close()
	waitForLogout(t, srv, "Subject")
	srv.WaitForWrites()

	god.send("show player")
	god.expect("A name would help.")

	god.send("show player nobodyatall")
	god.expect("There is no such player.")

	god.send("show player subject")
	god.expect("Player: Subject")
	god.expect("Au: ")
	god.expect("Played:")
}

func TestShowRoomLists(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Auditor", "findtheproblems", "m", "w")

	inWorld(t, srv, func(w *game.Live) {
		if room := w.Room(MageGuildRoom); room != nil {
			room.Flags = room.Flags.Set(game.RoomDeathTrap | game.RoomGodRoom)
		}
	})

	c.send("show death")
	c.expect("Death Traps")
	c.expect("The Mage Guild")

	c.send("show godrooms")
	c.expect("Godrooms")
	c.expectCount("The Mage Guild", 2)
}

func TestShowSnoop(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	god := dialClient(t, addr)
	god.create("Overseer", "whoiswatching", "m", "w")

	victim := dialClient(t, addr)
	victim.create("Observed", "beingwatched", "m", "w")
	setLevel(t, srv, "Observed", 10)

	god.send("show snoop")
	god.expect("No one is currently snooping.")

	god.send("snoop observed")
	god.expect("Okay.")

	god.send("show snoop")
	god.expect("People currently snooping:")
	god.settle()
	if !strings.Contains(god.transcript(), "Observed   - snooped by Overseer.") {
		t.Errorf("the snoop was not listed:\n%s", god.transcript())
	}
}

// Each field has its own level, and being below it is a different message
// from not knowing the field at all.
func TestShowFieldsHaveTheirOwnLevels(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	first := dialClient(t, addr)
	first.create("Highest", "thefirstgod", "m", "w")

	c := dialClient(t, addr)
	c.create("Lowly", "justanimmortal", "m", "w")
	setLevel(t, srv, "Lowly", game.LevelImmortal)

	// `stats` is LVL_IMMORT and works.
	c.send("show stats")
	c.expect("Current stats:")

	// `errors` is LVL_IMPL and does not.
	c.send("show errors")
	c.expect("You are not godly enough for that!")
}

func TestMortalsCannotBanOrShow(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	first := dialClient(t, addr)
	first.create("Boss", "runstheshow", "m", "w")

	c := dialClient(t, addr)
	c.create("Nobody", "havenopower", "m", "w")
	setLevel(t, srv, "Nobody", 10)

	for _, command := range []string{"ban all example.com", "unban example.com", "show stats"} {
		c.send(command)
		c.expect("Huh?!?")
	}
}
