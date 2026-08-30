// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/persist/bans"
	bansyaml "github.com/gerrowadat/disgracelands/internal/persist/bans/yaml"
	"github.com/gerrowadat/disgracelands/internal/persist/player"
	"github.com/gerrowadat/disgracelands/internal/persist/player/ascii"
	"github.com/gerrowadat/disgracelands/internal/persist/player/binary"
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

// The same fixture as TestABannedSiteIsRefused, but on yaml — proving the
// live path actually reaches bans/yaml's Store, not just its own
// isolated round-trip test.
func TestABannedSiteIsRefusedUnderYaml(t *testing.T) {
	banStore, err := bansyaml.New(bans.Config{Path: filepath.Join(t.TempDir(), "state")})
	if err != nil {
		t.Fatal(err)
	}
	store, err := ascii.New(player.Config{Dir: filepath.Join(t.TempDir(), "pfiles")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	objects, err := binary.NewObjectStore(player.Config{Dir: filepath.Join(t.TempDir(), "plrobjs-lib")})
	if err != nil {
		t.Fatal(err)
	}
	srv, _ := newTestServerWith(t, store, objects, banStore, nil, nil, nil)
	addr := listening(t, srv)

	god := dialClient(t, addr)
	god.create("Warden", "nobodycomesin", "m", "w")

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

// A `select` ban is checked after the password, not at the name prompt —
// reading interpreter.c:1482-1490 closely found it fires at CON_PASSWORD,
// against the loaded character's own PLR_SITEOK bit, later than
// docs/deviations.md used to say. A brand new character is never touched
// by it (creation grants the flag for free, game.ApplyNewCharacterDefaults)
// and `set <name> siteok` is what clears an existing one.
func TestASelectBanChecksSiteOKAfterThePassword(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	god := dialClient(t, addr)
	god.create("Bouncer", "pickyaboutwho", "m", "w")

	regular := dialClient(t, addr)
	regular.create("Regular", "beenherebefore", "m", "w")

	// Creation grants siteok to everybody; strip it back off — the only
	// way an existing record ends up without it — before the ban goes up.
	god.send("set regular siteok off")
	god.expect("Siteok OFF for Regular.")

	regular.send("quit")
	regular.expect("Goodbye")
	regular.close()
	waitForLogout(t, srv, "Regular")
	srv.WaitForWrites()

	god.send("ban select 127.0.0.1")
	god.expect("Site banned.")

	// The name prompt itself does not refuse anybody.
	blocked := dialClient(t, addr)
	blocked.expect("By what name do you wish to be known?")
	blocked.send("Regular")
	blocked.expect("Password:")
	blocked.send("beenherebefore")
	blocked.expect("Sorry, this char has not been cleared for login from your site!")

	// A brand new character is unaffected: creation grants siteok before
	// the check could ever apply, so the ban never touches it.
	fresh := dialClient(t, addr)
	fresh.create("Newcomer", "nevermetaban", "m", "w")

	// `set <name> siteok on` is what actually clears an existing
	// character, not merely something nothing enforces: lift the ban
	// briefly to let Regular in, flip the flag while they are online (the
	// only time `set` can reach them), and reinstate the ban to prove it.
	god.send("unban 127.0.0.1")
	god.expect("Site unbanned.")

	back := dialClient(t, addr)
	back.login("Regular", "beenherebefore")

	god.send("set regular siteok on")
	god.expect("Siteok ON for Regular.")

	back.send("quit")
	back.expect("Goodbye")
	back.close()
	waitForLogout(t, srv, "Regular")
	srv.WaitForWrites()

	god.send("ban select 127.0.0.1")
	god.expect("Site banned.")

	cleared := dialClient(t, addr)
	cleared.login("Regular", "beenherebefore")
	cleared.send("look")
	cleared.expect(">")
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

	// The full listing is long enough to page (act.wizard.c's do_show
	// pages all three of its zone branches through one page_string call,
	// and the test world carries enough zones — including filler ones,
	// see testFillerZoneVnumBase — to actually exercise that), so the
	// pager has to be closed before the next command.
	c.send("show zones")
	c.expect("Midgaard")
	c.expect("Range:")
	c.expect("Return to continue")
	c.send("q")

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

// `show rent`, ported from Crash_listrent (objsave.c:342): a header word
// for why the file was written, then one line per object still resolvable
// against a live prototype.
func TestShowRent(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	god := dialClient(t, addr)
	god.create("Landlord", "whoownsit", "m", "w")

	god.send("show rent")
	god.expect("A name would help.")

	god.send("show rent nobodyatall")
	god.expect("nobodyatall has no rent file.")

	c := dialClient(t, addr)
	c.create("Tenant", "leavingsoon", "m", "w")
	inWorld(t, srv, func(w *game.Live) {
		who := w.Find("Tenant")
		if who == nil {
			t.Error("the character is not in the world")
			return
		}
		if obj := w.NewObject(testSwordVnum); obj != nil {
			w.ObjectToChar(obj, who)
		}
	})

	c.send("quit")
	c.expect("Goodbye")
	c.close()
	waitForLogout(t, srv, "Tenant")

	// Quitting always writes a crash file, free and unpaid-for — the same
	// fixture TestWhatYouCarryOutIsWhatYouCarryBackIn checks by reading the
	// file directly; this checks the same file through the command that
	// reports it.
	god.send("show rent tenant")
	god.expect("Crash")
	god.expect("a long sword")
}

// `show shops`, ported from show_shops (shop.c:1350): the summary table
// with no argument, and one shop's detail view given its number.
func TestShowShops(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	god := dialClient(t, addr)
	god.create("Auctioneer", "whatssforsale", "m", "w")

	god.send("show shops")
	god.expect("##   Virtual   Where    Keeper    Buy   Sell   Customers")
	god.expect("9001")

	god.send("show shops 1")
	god.expect("Vnum:       [ 9001], Rnum: [    1]")
	god.expect("Shopkeeper:")
	god.expect("Produces:   a long sword")
	god.expect("Buys:")
	// Buy at:/Sell at: carries the C's own swapped-column bug (shop.c:1338):
	// ProfitSell (0.15) prints under "Buy at:" and ProfitBuy (1.15) under
	// "Sell at:", reproduced rather than fixed — see docs/weirdnumbers.md.
	god.expect("Buy at:     [0.15], Sell at: [1.15]")
	god.expect("Bits:")

	god.send("show shops 999")
	god.expect("Illegal shop number.")
}

func TestShowRoomLists(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Auditor", "findtheproblems", "m", "w")

	inWorld(t, srv, func(w *game.Live) {
		if room := w.Room(MageGuildRoom); room != nil {
			room.Flags = room.Flags.With(game.RoomDeathTrap, game.RoomGodRoom)
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
