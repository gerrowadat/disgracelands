// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"testing"
	"time"

	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/persist/player"
)

// The bank and the inn, end to end. Both are specials over `do_not_here`
// commands, like the shop.

// withBanker puts a banker in the character's room and gives them money.
func withBanker(t *testing.T, srv *Server, name string, gold, bank int32) {
	t.Helper()
	inWorld(t, srv, func(w *game.Live) {
		who := w.Find(name)
		if who == nil {
			t.Error("the character is not in the world")
			return
		}
		if who.Record != nil {
			who.Record.Points.Gold, who.Record.Points.BankGold = gold, bank
		}
		def := w.MobileDef(testShopkeeperVnum)
		if def == nil {
			t.Error("no mobile to make a banker of")
			return
		}
		def.Spec = "bank"
		if w.SpawnMobile(testShopkeeperVnum, who.Room, srv.rng) == nil {
			t.Error("could not put a banker in the room")
		}
	})
}

func purseOf(t *testing.T, srv *Server, name string) (gold, bank int32) {
	t.Helper()
	inWorld(t, srv, func(w *game.Live) {
		if who := w.Find(name); who != nil && who.Record != nil {
			gold, bank = who.Record.Points.Gold, who.Record.Points.BankGold
		}
	})
	return gold, bank
}

func TestBanking(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Saver", "piggybank", "m", "m")
	withBanker(t, srv, "Saver", 500, 0)

	c.send("balance")
	c.expect("You currently have no money deposited.")

	c.send("deposit 200")
	c.expect("You deposit 200 coins.")

	c.send("balance")
	c.expect("Your current balance is 200 coins.")

	if gold, bank := purseOf(t, srv, "Saver"); gold != 300 || bank != 200 {
		t.Errorf("after depositing 200 of 500 the purse is %d/%d, want 300/200", gold, bank)
	}

	c.send("withdraw 50")
	c.expect("You withdraw 50 coins.")
	if gold, bank := purseOf(t, srv, "Saver"); gold != 350 || bank != 150 {
		t.Errorf("after withdrawing 50 the purse is %d/%d, want 350/150", gold, bank)
	}

	c.send("deposit 100000")
	c.expect("You don't have that many coins!")

	c.send("withdraw 100000")
	c.expect("You don't have that many coins deposited!")

	// atoi returns zero for anything that does not start with a digit, so
	// `deposit all` asks the question rather than depositing everything.
	c.send("deposit all")
	c.expect("How much do you want to deposit?")

	c.send("withdraw")
	c.expect("How much do you want to withdraw?")

	if gold, bank := purseOf(t, srv, "Saver"); gold != 350 || bank != 150 {
		t.Errorf("a refused transaction moved money: the purse is %d/%d, want 350/150", gold, bank)
	}
}

func TestBankCommandsDoNothingAwayFromABank(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Broke", "nobanker", "m", "m")

	for _, command := range []string{"balance", "deposit 10", "withdraw 10"} {
		c.send(command)
		c.expect("Sorry, but you cannot do that here!")
	}
}

// withReceptionist puts a receptionist in the character's room.
func withReceptionist(t *testing.T, srv *Server, name, spec string) {
	t.Helper()
	inWorld(t, srv, func(w *game.Live) {
		who := w.Find(name)
		if who == nil {
			t.Error("the character is not in the world")
			return
		}
		def := w.MobileDef(testShopkeeperVnum)
		if def == nil {
			t.Error("no mobile to make a receptionist of")
			return
		}
		def.Spec = spec
		if w.SpawnMobile(testShopkeeperVnum, who.Room, srv.rng) == nil {
			t.Error("could not put a receptionist in the room")
		}
	})
}

// free_rent is YES in config.c, so this is the whole of what the inn ever
// said on this server. The pricing behind it is ported and unreachable.
func TestRentIsFree(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Guest", "roomforone", "m", "m")
	withReceptionist(t, srv, "Guest", "receptionist")

	if !game.FreeRent {
		t.Skip("free_rent is off; this test describes the archived setting")
	}

	c.send("offer")
	c.expect("Rent is free here.  Just quit, and your objects will be saved!")

	c.send("rent")
	c.expect("Rent is free here.")

	// And they are still in the world: a free-rent inn never takes anybody.
	var present bool
	inWorld(t, srv, func(w *game.Live) { present = w.Find("Guest") != nil })
	if !present {
		t.Error("a free-rent receptionist removed the character from the world")
	}
}

func TestTheInnCommandsDoNothingAwayFromAnInn(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Homeless", "nobedhere", "m", "m")

	for _, command := range []string{"offer", "rent"} {
		c.send(command)
		c.expect("Sorry, but you cannot do that here!")
	}
}

// withPaidRent turns free_rent off for one test, as a server with the setting
// changed would have it. Everything below is unreachable on the archived
// configuration and is the bulk of gen_receptionist, so it is worth having
// covered.
func withPaidRent(t *testing.T) {
	t.Helper()
	was := game.FreeRent
	game.FreeRent = false
	t.Cleanup(func() { game.FreeRent = was })
}

func TestOfferPricesEveryItemAndAddsTheFee(t *testing.T) {
	srv, _ := newTestServer(t)
	withPaidRent(t)
	c := dialClient(t, listening(t, srv))
	c.create("Pricer", "howmuchis", "m", "m")
	withReceptionist(t, srv, "Pricer", "receptionist")

	inWorld(t, srv, func(w *game.Live) {
		who := w.Find("Pricer")
		if who == nil {
			t.Error("no character")
			return
		}
		who.Record.Points.Gold = 10_000
		// Two items with rents of 7 and 3.
		for _, r := range []int32{7, 3} {
			obj := w.NewObject(testSwordVnum)
			if obj == nil {
				continue
			}
			obj.Def = &game.ObjDef{
				Vnum: testSwordVnum, Keywords: obj.Keywords, ShortDesc: obj.ShortDesc,
				Type: obj.Type, Cost: obj.Cost, RentPerDay: r,
			}
			w.ObjectToChar(obj, who)
		}
	})

	c.send("offer")
	c.expect("coins for a long sword..")
	c.expect("Plus, my 100 coin fee..")
	// 100 + 7 + 3.
	c.expect("For a total of 110 coins per day.")
	// 10,000 in the purse at 110 a day.
	c.expect("You can rent for 90 days")
}

// Anything unrentable stops the whole transaction and is named. Every key is
// unrentable, whatever its flags — Crash_is_unrentable tests the type.
func TestAKeyStopsYouRenting(t *testing.T) {
	srv, _ := newTestServer(t)
	withPaidRent(t)
	c := dialClient(t, listening(t, srv))
	c.create("Locksmith", "keyholder", "m", "m")
	withReceptionist(t, srv, "Locksmith", "receptionist")

	inWorld(t, srv, func(w *game.Live) {
		who := w.Find("Locksmith")
		if who == nil {
			t.Error("no character")
			return
		}
		who.Record.Points.Gold = 10_000
		if key := w.NewObject(testKeyVnum); key != nil {
			w.ObjectToChar(key, who)
		}
	})

	c.send("rent")
	c.expect("You cannot store a small key.")

	var present bool
	inWorld(t, srv, func(w *game.Live) { present = w.Find("Locksmith") != nil })
	if !present {
		t.Error("a refused rent removed the character anyway")
	}
}

func TestYouCannotRentWithNothing(t *testing.T) {
	srv, _ := newTestServer(t)
	withPaidRent(t)
	c := dialClient(t, listening(t, srv))
	c.create("Emptyhand", "nothingatall", "m", "m")
	withReceptionist(t, srv, "Emptyhand", "receptionist")

	c.send("rent")
	c.expect("But you are not carrying anything!  Just quit!")
}

// Renting properly: the file says rented, the load room is the inn, and the
// character leaves the game.
func TestRentingStoresYourThingsAndTakesYouOut(t *testing.T) {
	srv, _ := newTestServer(t)
	withPaidRent(t)
	c := dialClient(t, listening(t, srv))
	c.create("Lodger", "bedandboard", "m", "m")
	withReceptionist(t, srv, "Lodger", "receptionist")

	var innRoom game.RoomVnum
	inWorld(t, srv, func(w *game.Live) {
		who := w.Find("Lodger")
		if who == nil {
			t.Error("no character")
			return
		}
		innRoom = who.Room
		who.Record.Points.Gold = 10_000
		if sword := w.NewObject(testSwordVnum); sword != nil {
			w.ObjectToChar(sword, who)
		}
	})

	c.send("rent")
	c.expect("stores your belongings and helps you into your private chamber.")
	// No c.close() here: renting is what ends the session, and closing the
	// socket first would race it.
	waitForLogout(t, srv, "Lodger")

	f, err := srv.objects.LoadObjects(t.Context(), "Lodger")
	if err != nil {
		t.Fatalf("reading the rent file: %v", err)
	}
	if f.Code != player.RentRented {
		t.Errorf("renting wrote a %s file, want a rented one", f.Code)
	}
	if len(f.Objects) != 1 {
		t.Errorf("the rent file holds %d objects, want 1", len(f.Objects))
	}
	// 100 fee plus a sword with no rent of its own.
	if f.CostPerDay != 100 {
		t.Errorf("the rent file says %d a day, want 100", f.CostPerDay)
	}

	// Renting is what sets the load room, and it is why you come back to the
	// inn rather than the temple. A plain quit leaves it alone.
	//
	// Polled rather than read once: RentCharacter takes the character out of
	// the world on the world goroutine and *then* saves, off it. So being
	// gone from the world does not mean the record has been written yet, and
	// reading it straight away is a race that a busy CI machine loses.
	var got game.RoomVnum
	if !eventually(5*time.Second, func() bool {
		rec, err := srv.players.Load(t.Context(), "Lodger")
		if err != nil {
			return false
		}
		got = rec.LoadRoom
		return got == innRoom
	}) {
		t.Errorf("after renting the load room is %d, want the inn at %d", got, innRoom)
	}
}
