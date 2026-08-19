// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/persist/player"
)

// Crash_load and Crash_crashsave, end to end.
//
// The unit of behaviour worth testing here is not the file format — that is
// covered in the binary package — but the round trip: quit carrying
// something, come back, still have it.

// waitForLogout blocks until a character is out of the world.
//
// `quit` prints "Goodbye." from the command, but Leave — which is what
// crash-saves and then removes them — runs afterwards, in the connection
// goroutine's teardown. A test that writes a rent file the instant it sees
// "Goodbye" is racing a crash-save that will delete it.
func waitForLogout(t *testing.T, srv *Server, name string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var gone bool
		inWorld(t, srv, func(w *game.Live) { gone = w.Find(name) == nil })
		if gone {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("%s is still in the world five seconds after quitting", name)
}

// carriedNames returns what a character is carrying, read on the world
// goroutine.
func carriedNames(t *testing.T, srv *Server, name string) []string {
	t.Helper()
	var out []string
	inWorld(t, srv, func(w *game.Live) {
		c := w.Find(name)
		if c == nil {
			return
		}
		for _, obj := range c.Carrying {
			out = append(out, obj.Name())
		}
	})
	return out
}

func TestWhatYouCarryOutIsWhatYouCarryBackIn(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	c := dialClient(t, addr)
	c.create("Rentaghost", "hedgehog", "m", "m")

	inWorld(t, srv, func(w *game.Live) {
		who := w.Find("Rentaghost")
		if who == nil {
			t.Error("the character is not in the world")
			return
		}
		for _, vnum := range []game.ObjVnum{testSwordVnum, testRingVnum, testBagVnum} {
			if obj := w.NewObject(vnum); obj != nil {
				w.ObjectToChar(obj, who)
			}
		}
	})

	before := carriedNames(t, srv, "Rentaghost")
	if len(before) != 3 {
		t.Fatalf("set up with %d objects, want 3: %v", len(before), before)
	}

	c.send("quit")
	c.expect("Goodbye")
	c.close()
	waitForLogout(t, srv, "Rentaghost")

	// The file exists and says crash: quitting is free, and it is what brings
	// you back in the temple rather than where you left off.
	f, err := srv.objects.LoadObjects(context.Background(), "Rentaghost")
	if err != nil {
		t.Fatalf("reading the rent file after quitting: %v", err)
	}
	if f.Code != player.RentCrash {
		t.Errorf("quitting wrote a %s file, want a crash file", f.Code)
	}
	if len(f.Objects) != 3 {
		t.Errorf("the rent file holds %d objects, want 3", len(f.Objects))
	}

	back := dialClient(t, addr)
	back.login("Rentaghost", "hedgehog")

	after := carriedNames(t, srv, "Rentaghost")
	if len(after) != len(before) {
		t.Fatalf("came back with %v, want %v", after, before)
	}
	for i := range before {
		if after[i] != before[i] {
			t.Errorf("came back with %v, want %v", after, before)
			break
		}
	}
}

// A container's contents come back loose, because the file has nowhere to
// record that they were inside it — USE_AUTOEQ is 0 and obj_file_elem has no
// location member. This is the C's behaviour, and worth a test precisely
// because it looks like a bug in the port.
func TestRentingEmptiesYourBags(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	c := dialClient(t, addr)
	c.create("Baggins", "secondbreakfast", "m", "m")

	inWorld(t, srv, func(w *game.Live) {
		who := w.Find("Baggins")
		bag, ring := w.NewObject(testBagVnum), w.NewObject(testRingVnum)
		if who == nil || bag == nil || ring == nil {
			t.Error("could not set up a bag with a ring in it")
			return
		}
		w.ObjectToChar(bag, who)
		if !w.ObjectToObject(ring, bag) {
			t.Error("could not put the ring in the bag")
		}
	})

	c.send("quit")
	c.expect("Goodbye")
	c.close()
	waitForLogout(t, srv, "Baggins")

	back := dialClient(t, addr)
	back.login("Baggins", "secondbreakfast")

	var loose, inBag int
	inWorld(t, srv, func(w *game.Live) {
		who := w.Find("Baggins")
		if who == nil {
			return
		}
		loose = len(who.Carrying)
		for _, obj := range who.Carrying {
			inBag += len(obj.Contents)
		}
	})

	if loose != 2 {
		t.Errorf("came back carrying %d things, want 2 — the bag and the ring, both loose", loose)
	}
	if inBag != 0 {
		t.Errorf("came back with %d things still in the bag; renting empties them", inBag)
	}
}

// The prototype is the source of truth for everything the file does not
// store, and the file is the source of truth for the handful it does. A wand
// keeps its remaining charges.
func TestAWandComesBackWithTheChargesItHadLeft(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	c := dialClient(t, addr)
	c.create("Sparky", "abracadabra", "m", "m")

	inWorld(t, srv, func(w *game.Live) {
		who, wand := w.Find("Sparky"), w.NewObject(testWandVnum)
		if who == nil || wand == nil {
			t.Error("could not set up a wand")
			return
		}
		wand.Values[2] = 2 // two charges left of however many it holds
		w.ObjectToChar(wand, who)
	})

	c.send("quit")
	c.expect("Goodbye")
	c.close()
	waitForLogout(t, srv, "Sparky")

	back := dialClient(t, addr)
	back.login("Sparky", "abracadabra")

	var charges int32 = -1
	inWorld(t, srv, func(w *game.Live) {
		who := w.Find("Sparky")
		if who == nil || len(who.Carrying) == 0 {
			return
		}
		charges = who.Carrying[0].Values[2]
	})
	if charges != 2 {
		t.Errorf("the wand came back with %d charges, want 2", charges)
	}
}

// A character who owes more rent than they can pay loses everything, and is
// told so with the C's bell character.
func TestRentYouCannotAffordCostsYouEverything(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	c := dialClient(t, addr)
	c.create("Skint", "nomoney", "m", "m")
	c.send("quit")
	c.expect("Goodbye")
	c.close()
	waitForLogout(t, srv, "Skint")

	// Rewrite the file as a rent from a week ago at a price nobody could pay.
	ctx := context.Background()
	f, err := srv.objects.LoadObjects(ctx, "Skint")
	if err != nil {
		// Nothing was carried, so quitting deleted the file rather than
		// writing an empty one. Build the rented file from scratch.
		f = &player.RentFile{}
	}
	f.Code = player.RentRented
	f.Written = time.Now().Add(-7 * 24 * time.Hour)
	f.CostPerDay = 1_000_000
	f.Objects = []player.StoredObject{{
		Vnum:    testSwordVnum,
		Affects: make([]game.ObjAffect, game.MaxObjAffects),
	}}
	if err := srv.objects.SaveObjects(ctx, "Skint", f); err != nil {
		t.Fatalf("writing a rented file: %v", err)
	}

	back := dialClient(t, addr)
	back.login("Skint", "nomoney")
	back.expect("could not afford your rent")

	if !strings.Contains(back.transcript(), "Salvation Army") {
		t.Error("the rent-lost message is missing its second line")
	}

	var carrying int
	inWorld(t, srv, func(w *game.Live) {
		if who := w.Find("Skint"); who != nil {
			carrying = len(who.Carrying)
		}
	})
	if carrying != 0 {
		t.Errorf("came back carrying %d things after losing the lot", carrying)
	}
}

// Rent that *can* be paid is taken out of pocket first and the bank second,
// which is the order GET_BANK_GOLD -= MAX(cost - GET_GOLD, 0) produces.
func TestPayableRentComesOutOfPocketFirst(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	c := dialClient(t, addr)
	c.create("Solvent", "richasking", "m", "m")
	inWorld(t, srv, func(w *game.Live) {
		if who := w.Find("Solvent"); who != nil && who.Record != nil {
			who.Record.Points.Gold = 100
			who.Record.Points.BankGold = 1000
		}
	})
	c.send("quit")
	c.expect("Goodbye")
	c.close()
	waitForLogout(t, srv, "Solvent")

	ctx := context.Background()
	if err := srv.objects.SaveObjects(ctx, "Solvent", &player.RentFile{
		Code: player.RentRented,
		// Two days at 150 a day is 300: all 100 in pocket and 200 from the
		// bank.
		Written:    time.Now().Add(-48 * time.Hour),
		CostPerDay: 150,
		Objects: []player.StoredObject{{
			Vnum:    testSwordVnum,
			Affects: make([]game.ObjAffect, game.MaxObjAffects),
		}},
	}); err != nil {
		t.Fatalf("writing a rented file: %v", err)
	}

	back := dialClient(t, addr)
	back.login("Solvent", "richasking")

	var gold, bank int32
	inWorld(t, srv, func(w *game.Live) {
		if who := w.Find("Solvent"); who != nil && who.Record != nil {
			gold, bank = who.Record.Points.Gold, who.Record.Points.BankGold
		}
	})
	if gold != 0 {
		t.Errorf("came back with %d gold in pocket, want 0 — rent takes that first", gold)
	}
	if bank != 800 {
		t.Errorf("came back with %d in the bank, want 800", bank)
	}
}

// Coming back from a rented file un-rents it: the same stay cannot be
// charged, or collected, twice.
func TestUnrentingTurnsTheFileIntoACrashFile(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	ctx := context.Background()

	c := dialClient(t, addr)
	c.create("Encore", "onemoretime", "m", "m")
	c.send("quit")
	c.expect("Goodbye")
	c.close()
	waitForLogout(t, srv, "Encore")

	if err := srv.objects.SaveObjects(ctx, "Encore", &player.RentFile{
		Code:    player.RentRented,
		Written: time.Now(),
		Objects: []player.StoredObject{{
			Vnum:    testSwordVnum,
			Affects: make([]game.ObjAffect, game.MaxObjAffects),
		}},
	}); err != nil {
		t.Fatalf("writing a rented file: %v", err)
	}

	back := dialClient(t, addr)
	back.login("Encore", "onemoretime")

	f, err := srv.objects.LoadObjects(ctx, "Encore")
	if err != nil {
		t.Fatalf("reading the rent file after un-renting: %v", err)
	}
	if f.Code != player.RentCrash {
		t.Errorf("the file is still %s after un-renting, want a crash file", f.Code)
	}
	if len(f.Objects) != 1 {
		t.Errorf("un-renting left %d objects in the file, want 1 — only the header is rewritten",
			len(f.Objects))
	}
}

// An object whose prototype has been deleted from the world since the file
// was written is dropped rather than crashing the login.
func TestAnObjectWithNoPrototypeIsSkipped(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	ctx := context.Background()

	c := dialClient(t, addr)
	c.create("Ghosthand", "vanished", "m", "m")
	c.send("quit")
	c.expect("Goodbye")
	c.close()
	waitForLogout(t, srv, "Ghosthand")

	if err := srv.objects.SaveObjects(ctx, "Ghosthand", &player.RentFile{
		Code:    player.RentCrash,
		Written: time.Now(),
		Objects: []player.StoredObject{
			{Vnum: 31337, Affects: make([]game.ObjAffect, game.MaxObjAffects)},
			{Vnum: testSwordVnum, Affects: make([]game.ObjAffect, game.MaxObjAffects)},
		},
	}); err != nil {
		t.Fatalf("writing a crash file: %v", err)
	}

	back := dialClient(t, addr)
	back.login("Ghosthand", "vanished")

	var carrying int
	inWorld(t, srv, func(w *game.Live) {
		if who := w.Find("Ghosthand"); who != nil {
			carrying = len(who.Carrying)
		}
	})
	if carrying != 1 {
		t.Errorf("came back carrying %d things, want 1 — the other prototype is gone", carrying)
	}
}
