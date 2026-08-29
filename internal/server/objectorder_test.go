// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// The order objects are listed in, over a socket (#193).
//
// The C's obj_to_char, obj_to_room and obj_to_obj are the same two lines each
// — `object->next_content = list; list = object;` (handler.c:418-419, 685-686,
// 737-738) — so every one of them inserts at the head, and every reader walks
// from the head. Newest first, everywhere.
//
// This port appended for five phases, which reversed all of it. That is
// visible in a listing and it is not only cosmetic: the same order is what a
// numbered reference counts down, so it decided which of two matching items
// `2.filler` picked.

// orderOf returns the listed items, in the order they appeared, out of the
// transcript written since a marker position.
func orderOf(text string, names ...string) []string {
	type at struct {
		name string
		i    int
	}
	var found []at
	for _, n := range names {
		if i := strings.Index(text, n); i >= 0 {
			found = append(found, at{n, i})
		}
	}
	for i := 1; i < len(found); i++ {
		for j := i; j > 0 && found[j].i < found[j-1].i; j-- {
			found[j], found[j-1] = found[j-1], found[j]
		}
	}
	out := make([]string, 0, len(found))
	for _, f := range found {
		out = append(out, f.name)
	}
	return out
}

// getFillersInOrder drops filler items 0..n-1 into the room and picks each one
// up, oldest first.
//
// Each `get` is waited on before the next `drop`, and that is not tidiness:
// drop() places the object on the world goroutine while the client's previous
// command is still in flight, so without the wait the pickups interleave and
// the test builds an inventory in an order nobody chose. It failed that way
// first — "You get a filler item 1 / 2 / 0" — which reads exactly like the bug
// under test.
func getFillersInOrder(t *testing.T, srv *Server, c *client, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		drop(t, srv, testFillerVnumBase+game.ObjVnum(i), ImmortStartRoom)
		c.send("get filler")
		c.expect(fmt.Sprintf("You get a filler item %d.", i))
	}
}

// TestInventoryListsNewestFirst.
func TestInventoryListsNewestFirst(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	getFillersInOrder(t, srv, c, 3)

	before := len(c.transcript())
	c.send("inventory")
	c.settle()

	got := orderOf(c.transcript()[before:],
		"a filler item 2", "a filler item 1", "a filler item 0")
	want := []string{"a filler item 2", "a filler item 1", "a filler item 0"}
	if strings.Join(got, ", ") != strings.Join(want, ", ") {
		t.Errorf("inventory listed %v, want newest first %v", got, want)
	}
}

// TestTheFloorListsNewestFirst — obj_to_room is the same two lines as
// obj_to_char (handler.c:685-686), so a room's contents run the same way.
func TestTheFloorListsNewestFirst(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	for i := 0; i < 3; i++ {
		drop(t, srv, testFillerVnumBase+game.ObjVnum(i), ImmortStartRoom)
	}

	before := len(c.transcript())
	c.send("look")
	c.settle()

	got := orderOf(c.transcript()[before:],
		"A filler item 2", "A filler item 1", "A filler item 0")
	want := []string{"A filler item 2", "A filler item 1", "A filler item 0"}
	if strings.Join(got, ", ") != strings.Join(want, ", ") {
		t.Errorf("the floor listed %v, want newest first %v", got, want)
	}
}

// TestANumberedReferenceCountsFromTheNewest is the half of this that is not
// presentation. `2.filler` counts down the same list the listing prints, so
// the insertion end decides *which object you get*, not just the order they
// are named in — which is why docs/deviations.md flagged the difference as
// not purely cosmetic while it was still open.
func TestANumberedReferenceCountsFromTheNewest(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	getFillersInOrder(t, srv, c, 3)

	// 1. is the newest, so 2. is the middle one and 3. the first picked up.
	c.send("drop 2.filler")
	c.expect("You drop a filler item 1.")
	c.send("drop 2.filler")
	c.expect("You drop a filler item 0.")
}

// TestAContainerListsNewestFirst — obj_to_obj, the third of the three
// (handler.c:737-738).
func TestAContainerListsNewestFirst(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	drop(t, srv, testBagVnum, ImmortStartRoom)
	c.send("get bag")
	c.expect("You get a bag.")
	for i := 0; i < 3; i++ {
		drop(t, srv, testFillerVnumBase+game.ObjVnum(i), ImmortStartRoom)
		c.send("get filler")
		c.expect(fmt.Sprintf("You get a filler item %d.", i))
		c.send("put filler bag")
		c.expect(fmt.Sprintf("You put a filler item %d in a bag.", i))
	}

	before := len(c.transcript())
	c.send("look in bag")
	c.settle()

	got := orderOf(c.transcript()[before:],
		"a filler item 2", "a filler item 1", "a filler item 0")
	want := []string{"a filler item 2", "a filler item 1", "a filler item 0"}
	if strings.Join(got, ", ") != strings.Join(want, ", ") {
		t.Errorf("the bag listed %v, want newest first %v", got, want)
	}
}

// TestAHouseComesBackInTheOrderItWasLeft is the persistence half, and it is
// here rather than in houses_test.go because what it is really testing is the
// pairing: House_save recurses on next_content before writing the object it
// was handed (house.c:94-96), so the file holds the room back to front, and
// House_load reads it forwards into a list that grows at the head
// (house.c:73-81). Neither half preserves order alone.
//
// The existing house tests all store a single object, so nothing caught the
// loader still walking backwards after #193 changed the insertion end. The
// rent files' equivalent was caught, by TestWhatYouCarryOutIsWhatYouCarryBackIn
// — which is three objects rather than one, and that is the whole difference.
func TestAHouseComesBackInTheOrderItWasLeft(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Hoarder", "keepitsafe", "m", "m")
	c.send("hcontrol build 3020 north Hoarder")
	c.expect("House built.")

	moveTo(t, srv, "Hoarder", HouseRoom)
	srv.WaitForWrites()

	for i := 0; i < 3; i++ {
		vnum := testFillerVnumBase + game.ObjVnum(i)
		inWorld(t, srv, func(w *game.Live) {
			if obj := w.NewObject(vnum); obj != nil {
				w.ObjectToRoom(obj, HouseRoom)
			}
		})
	}

	var before []string
	inWorld(t, srv, func(w *game.Live) {
		for _, obj := range w.RoomObjects(HouseRoom) {
			before = append(before, obj.Name())
		}
	})

	srv.SaveChangedHouses(t.Context())

	// Empty the room, then load the file back into it.
	var after []string
	inWorld(t, srv, func(w *game.Live) {
		for _, obj := range append([]*game.Object(nil), w.RoomObjects(HouseRoom)...) {
			w.ExtractObject(obj)
		}
		srv.loadHouseObjects(w, HouseRoom)
		for _, obj := range w.RoomObjects(HouseRoom) {
			after = append(after, obj.Name())
		}
	})

	if strings.Join(after, ", ") != strings.Join(before, ", ") {
		t.Errorf("the house came back as %v, want the order it was left in %v", after, before)
	}
}
