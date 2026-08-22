// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"path/filepath"
	"testing"

	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/persist/player"
	"github.com/gerrowadat/disgracelands/internal/persist/player/native"
)

// The other half of TestRentingEmptiesYourBags (rent_test.go): the same
// bag-with-a-ring-in-it fixture, quit and logged back in, but on
// --player-format=native — where the deliberate deviation the user chose
// when scoping this (docs/proposals/data-format.md §8, player.StoredObject
// .Contains) is supposed to actually turn on. ascii/binary still come back
// loose (proven by the existing test, unmodified); this proves native does
// not.
func TestRentingUnderNativeKeepsTheRingInTheBag(t *testing.T) {
	store, err := native.New(player.Config{Dir: filepath.Join(t.TempDir(), "players")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	srv, _ := newTestServerWith(t, store, store, nil, nil, nil, nil)
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

	if loose != 1 {
		t.Errorf("carrying %d items loose, want 1 (the bag) -- native should not have flattened it", loose)
	}
	if inBag != 1 {
		t.Errorf("%d item(s) still inside a container, want 1 (the ring, still in the bag)", inBag)
	}
}
