// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/persist/houses"
	housesnative "github.com/gerrowadat/disgracelands/internal/persist/houses/native"
	"github.com/gerrowadat/disgracelands/internal/persist/player"
	"github.com/gerrowadat/disgracelands/internal/persist/player/ascii"
	"github.com/gerrowadat/disgracelands/internal/persist/player/binary"
)

// Player housing, end to end.

// moveTo puts a character in a room without walking them there.
func moveTo(t *testing.T, srv *Server, name string, room game.RoomVnum) {
	t.Helper()
	inWorld(t, srv, func(w *game.Live) {
		who := w.Find(name)
		if who == nil {
			t.Error("the character is not in the world")
			return
		}
		if err := w.Enter(who, room); err != nil {
			t.Errorf("moving to %d: %v", room, err)
		}
	})
}

func TestBuildingAndDestroyingAHouse(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	// The first character on an empty roster is an implementor, which is
	// what `hcontrol` needs.
	c.create("Builder", "bricksandmortar", "m", "m")

	c.send("hcontrol show")
	c.expect("No houses have been defined.")

	c.send("hcontrol build 3020 north Builder")
	c.expect("House built.  Mazel tov!")

	c.send("hcontrol show")
	c.expect("Address  Atrium  Build Date")
	c.expect("   3020    3021")

	// The rooms are flagged, which is what actually guards the door.
	var houseFlagged, atriumFlagged bool
	inWorld(t, srv, func(w *game.Live) {
		if room := w.Room(HouseRoom); room != nil {
			houseFlagged = room.Flags.Has(game.RoomHouse) && room.Flags.Has(game.RoomPrivate)
		}
		if room := w.Room(AtriumRoom); room != nil {
			atriumFlagged = room.Flags.Has(game.RoomAtrium)
		}
	})
	if !houseFlagged {
		t.Error("the house room is not flagged ROOM_HOUSE|ROOM_PRIVATE")
	}
	if !atriumFlagged {
		t.Error("the atrium is not flagged ROOM_ATRIUM")
	}

	// And it is on disk.
	if !eventually(5*time.Second, func() bool {
		got, err := srv.houses.Load()
		return err == nil && len(got) == 1 && got[0].Vnum == int32(HouseRoom)
	}) {
		t.Error("the house never reached the control file")
	}

	c.send("hcontrol destroy 3020")
	c.expect("House deleted.")

	inWorld(t, srv, func(w *game.Live) {
		if room := w.Room(HouseRoom); room != nil {
			houseFlagged = room.Flags.Has(game.RoomHouse)
		}
		if room := w.Room(AtriumRoom); room != nil {
			atriumFlagged = room.Flags.Has(game.RoomAtrium)
		}
	})
	if houseFlagged || atriumFlagged {
		t.Error("destroying the house left its flags behind")
	}
}

func TestHcontrolRefusesNonsense(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Surveyor", "measuretwice", "m", "m")

	c.send("hcontrol")
	c.expect("Usage: hcontrol build")

	c.send("hcontrol build 99999 north Surveyor")
	c.expect("No such room exists.")

	c.send("hcontrol build 3020 sideways Surveyor")
	c.expect("'sideways' is not a valid direction.")

	c.send("hcontrol build 3020 east Surveyor")
	c.expect("There is no exit east from room 3020.")

	// The mage guild's south exit leads to the temple, and the temple's
	// north leads somewhere else — so that door is one-way and cannot be a
	// house's.
	c.send("hcontrol build 3017 south Surveyor")
	c.expect("A house's exit must be a two-way door.")

	// Lower case: one_argument lowercases every word it pulls off
	// (interpreter.c:977), so the C echoes it back that way too.
	c.send("hcontrol build 3020 north Nobodyatall")
	c.expect("Unknown player 'nobodyatall'.")

	c.send("hcontrol destroy 3020")
	c.expect("Unknown house.")

	c.send("hcontrol pay 3020")
	c.expectCount("Unknown house.", 2)

	// Only after all of that does a good one work — and then it is a
	// duplicate.
	c.send("hcontrol build 3020 north Surveyor")
	c.expect("House built.")
	c.send("hcontrol build 3020 north Surveyor")
	c.expect("House already exists.")
}

// The whole point of a house: nobody else gets in through the atrium.
func TestTrespassing(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	owner := dialClient(t, addr)
	owner.create("Owner", "myveryownhouse", "m", "m")
	owner.send("hcontrol build 3020 north Owner")
	owner.expect("House built.")

	stranger := dialClient(t, addr)
	stranger.create("Stranger", "letmeinplease", "m", "m")
	// A mortal: a greater god walks into anything.
	inWorld(t, srv, func(w *game.Live) {
		if who := w.Find("Stranger"); who != nil && who.Record != nil {
			who.Record.Level = 10
		}
	})

	moveTo(t, srv, "Stranger", AtriumRoom)
	stranger.send("south")
	stranger.expect("That's private property -- no trespassing!")

	// The owner walks straight in.
	moveTo(t, srv, "Owner", AtriumRoom)
	owner.send("south")
	owner.expect("A Small House")

	// And a guest does too, once invited.
	owner.send("house Stranger")
	owner.expect("Guest added.")

	stranger.send("south")
	stranger.expect("A Small House")

	// Uninvited again, and back out they go.
	moveTo(t, srv, "Stranger", AtriumRoom)
	owner.send("house Stranger")
	owner.expect("Guest deleted.")

	stranger.send("south")
	stranger.expectCount("That's private property -- no trespassing!", 2)
}

func TestTheGuestList(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	owner := dialClient(t, addr)
	owner.create("Host", "comeonover", "m", "m")
	owner.send("hcontrol build 3020 north Host")
	owner.expect("House built.")

	// Away from the house, `house` refuses.
	owner.send("house")
	owner.expect("You must be in your house to set guests.")

	moveTo(t, srv, "Host", HouseRoom)

	owner.send("house")
	owner.expect("  Guests: None")

	owner.send("house Host")
	owner.expect("It's your house!")

	owner.send("house Nobodyatall")
	owner.expect("No such player.")

	// Somebody who is not the owner cannot set guests, even standing in it.
	visitor := dialClient(t, addr)
	visitor.create("Visitor", "justpassing", "m", "m")
	inWorld(t, srv, func(w *game.Live) {
		if who := w.Find("Visitor"); who != nil && who.Record != nil {
			who.Record.Level = 10
		}
	})
	moveTo(t, srv, "Visitor", HouseRoom)
	visitor.send("house Host")
	visitor.expect("Only the primary owner can set guests.")
}

// Things left in a house are still there after a reboot, which is the reason
// houses exist at all.
func TestWhatYouLeaveInYourHouseStaysThere(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Hoarder", "keepitsafe", "m", "m")
	c.send("hcontrol build 3020 north Hoarder")
	c.expect("House built.")

	moveTo(t, srv, "Hoarder", HouseRoom)
	inWorld(t, srv, func(w *game.Live) {
		who := w.Find("Hoarder")
		sword := w.NewObject(testSwordVnum)
		if who == nil || sword == nil {
			t.Error("could not set up a sword")
			return
		}
		w.ObjectToChar(sword, who)
	})

	c.send("drop sword")
	c.expect("You drop a long sword.")

	// The dirty bit is what makes House_save_all write it.
	var dirty bool
	inWorld(t, srv, func(w *game.Live) {
		if room := w.Room(HouseRoom); room != nil {
			dirty = room.Flags.Has(game.RoomHouseCrash)
		}
	})
	if !dirty {
		t.Error("dropping something in a house did not mark it for saving")
	}

	srv.SaveChangedHouses(t.Context())

	b, err := srv.houses.LoadObjects(int32(HouseRoom))
	if err != nil {
		t.Fatalf("reading the house file: %v", err)
	}
	if len(b) == 0 {
		t.Fatal("the house file is empty")
	}

	// And the flag is cleared, so the next sweep does not rewrite it.
	inWorld(t, srv, func(w *game.Live) {
		if room := w.Room(HouseRoom); room != nil {
			dirty = room.Flags.Has(game.RoomHouseCrash)
		}
	})
	if dirty {
		t.Error("saving the house left it marked as changed")
	}
}

// The same fixture as TestWhatYouLeaveInYourHouseStaysThere, on native --
// proving the live build/drop/crash-save path actually reaches
// houses/native's Store.
func TestWhatYouLeaveInYourHouseStaysThereUnderNative(t *testing.T) {
	houseStore, err := housesnative.New(houses.Config{ObjectDir: filepath.Join(t.TempDir(), "state")})
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
	srv, _ := newTestServerWith(t, store, objects, nil, nil, nil, houseStore)
	c := dialClient(t, listening(t, srv))
	c.create("Hoarder", "keepitsafe", "m", "m")
	c.send("hcontrol build 3020 north Hoarder")
	c.expect("House built.")

	moveTo(t, srv, "Hoarder", HouseRoom)
	inWorld(t, srv, func(w *game.Live) {
		who := w.Find("Hoarder")
		sword := w.NewObject(testSwordVnum)
		if who == nil || sword == nil {
			t.Error("could not set up a sword")
			return
		}
		w.ObjectToChar(sword, who)
	})

	c.send("drop sword")
	c.expect("You drop a long sword.")

	srv.SaveChangedHouses(t.Context())

	objs, err := srv.houses.LoadObjects(int32(HouseRoom))
	if err != nil {
		t.Fatalf("reading the house file: %v", err)
	}
	if len(objs) == 0 {
		t.Fatal("the house file is empty")
	}
	if objs[0].Vnum != game.ObjVnum(testSwordVnum) {
		t.Errorf("the house holds vnum %d, want the sword (%d)", objs[0].Vnum, testSwordVnum)
	}
}

// House_boot's sanity checks: a record whose room has gone, or whose door no
// longer leads to its atrium, is dropped rather than becoming a room nobody
// can enter and nobody can destroy.
func TestHouseBootDropsRecordsThatNoLongerMakeSense(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Founder", "firstsettler", "m", "m")
	ownerID := idOf(t, srv, "Founder")

	// One good house and four broken ones, written straight to the control
	// file as an older server would have left them.
	records, err := srv.houses.Load()
	if err != nil {
		t.Fatalf("loading: %v", err)
	}
	good := housesRecord(int32(HouseRoom), int32(AtriumRoom), int32(game.North), ownerID)
	records = append(records,
		good,
		housesRecord(99999, int32(AtriumRoom), int32(game.North), ownerID),           // no such room
		housesRecord(int32(HouseRoom), 99999, int32(game.North), ownerID),            // no such atrium
		housesRecord(int32(HouseRoom), int32(AtriumRoom), 99, ownerID),               // bad direction
		housesRecord(int32(HouseRoom), int32(BoardRoom), int32(game.North), ownerID), // exit mismatch
		housesRecord(int32(HouseRoom), int32(AtriumRoom), int32(game.North), 9999),   // no such owner
	)
	if err := srv.houses.Save(records); err != nil {
		t.Fatalf("saving: %v", err)
	}

	inWorld(t, srv, func(w *game.Live) {
		w.SetHouses(nil)
		srv.loadHouses(w)
	})

	var count int
	inWorld(t, srv, func(w *game.Live) { count = len(w.Houses()) })
	if count != 1 {
		t.Errorf("loaded %d houses from six records, want 1 — the rest are broken", count)
	}
}

// housesRecord builds a control record for the boot tests.
func housesRecord(vnum, atrium, exit int32, owner int64) houses.House {
	return houses.House{
		Vnum: vnum, Atrium: atrium, ExitNum: exit,
		BuiltOn: time.Unix(1_000_000_000, 0).UTC(),
		Mode:    houses.ModePrivate, Owner: owner,
	}
}
