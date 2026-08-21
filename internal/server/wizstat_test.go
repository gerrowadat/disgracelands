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
)

// `stat`, `vstat` and `vnum`, end to end.
//
// These print, and printing is what they are for, so the assertions are on
// what appears — including the bitfield names, which are the only place
// several of these flags are described anywhere.

func TestStatRoom(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Builder", "showmeitall", "m", "w")

	inWorld(t, srv, func(w *game.Live) {
		room := w.Room(ImmortStartRoom)
		if room == nil {
			t.Error("no board room")
			return
		}
		room.Flags = room.Flags.Set(game.RoomIndoors | game.RoomPeaceful)
	})

	c.send("stat room")
	c.expect("Room name: The Immortal Board Room")
	c.expect("VNum: [ 1204]")
	c.expect("Flags: ")
	c.settle()

	transcript := c.transcript()
	for _, want := range []string{"INDOORS", "PEACEFUL", "Description:", "Chars present:"} {
		if !strings.Contains(transcript, want) {
			t.Errorf("stat room did not print %q:\n%s", want, transcript)
		}
	}
	// The temple is south of here, so there is an exit line to print.
	if !strings.Contains(transcript, "Exit south") {
		t.Errorf("stat room did not print the exit:\n%s", transcript)
	}
}

func TestStatObject(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Appraiser", "whatisthis", "m", "w")

	inWorld(t, srv, func(w *game.Live) {
		who, sword := w.Find("Appraiser"), w.NewObject(testSwordVnum)
		if who == nil || sword == nil {
			t.Error("could not make a sword")
			return
		}
		w.ObjectToChar(sword, who)
	})

	c.send("stat sword")
	c.expect("Name: 'a long sword'")
	c.expect("Type: WEAPON")
	c.expect("Weight: 10, Value: 100")
	// The type-dependent line: a weapon shows its damage dice.
	c.expect("Todam: 2d6, Message type: 3")
	c.expect("Affections: None")
}

// The values line is read differently for every kind of object, and getting
// it wrong is invisible until a builder is confused by it.
func TestStatObjectValuesByType(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Assessor", "typebytype", "m", "w")

	for _, tc := range []struct {
		vnum game.ObjVnum
		want string
	}{
		{testWandVnum, "charges remaining"},
		{testPotionVnum, "Spells: (Level"},
		{testBagVnum, "Weight capacity:"},
		{testFountainVnum, "Capacity:"},
	} {
		inWorld(t, srv, func(w *game.Live) {
			who, obj := w.Find("Assessor"), w.NewObject(tc.vnum)
			if who == nil || obj == nil {
				t.Errorf("could not make object %d", tc.vnum)
				return
			}
			// Everything else out of the way, so `stat` finds this one.
			for _, held := range append([]*game.Object(nil), who.Carrying...) {
				w.ExtractObject(held)
			}
			w.ObjectToChar(obj, who)
		})

		var keyword string
		inWorld(t, srv, func(w *game.Live) {
			if who := w.Find("Assessor"); who != nil && len(who.Carrying) > 0 {
				keyword = firstKeyword(who.Carrying[0].Keywords)
			}
		})

		c.send("stat " + keyword)
		c.expect(tc.want)
	}
}

func firstKeyword(keywords string) string {
	if i := strings.IndexAny(keywords, " \t"); i >= 0 {
		return keywords[:i]
	}
	return keywords
}

func TestStatCharacter(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Inspector", "letmelook", "m", "w")

	dog := aMobile(t, srv, "Inspector")
	if dog == nil {
		t.Fatal("no mobile to stat")
	}

	c.send("stat dog")
	c.expect("MOB 'a large dog'")
	c.expect("Monster Class:")
	c.expect("NPC flags:")
	c.expect("Mob Spec-Proc:")
	c.expect("AFF: ")

	// A player prints the player half instead.
	c.send("stat inspector")
	c.expect("PC 'Inspector'")
	c.expect("PLR: ")
	c.expect("PRF: ")
	c.expect("Hunger:")
	c.expect("Master is: <none>")
}

func TestStatFindsNothing(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Puzzled", "wheredidgo", "m", "w")

	c.send("stat")
	c.expect("Stats on who or what?")

	c.send("stat nothinglikethat")
	c.expect("Nothing around by that name.")

	c.send("stat mob nothinglikethat")
	c.expect("No such monster around.")

	c.send("stat player nothinglikethat")
	c.expect("No such player around.")
}

func TestVnum(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Lister", "givemenumbers", "m", "w")

	c.send("vnum")
	c.expect("Usage: vnum { obj | mob } <name>")

	c.send("vnum obj sword")
	c.expect("[  100] a long sword")

	c.send("vnum obj nothinglikethat")
	c.expect("No objects by that name.")

	c.send("vnum mob dog")
	c.expect("a large dog")

	c.send("vnum mob nothinglikethat")
	c.expect("No mobiles by that name.")
}

func TestVstat(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Prototyper", "beforeitexists", "m", "w")

	c.send("vstat")
	c.expect("Usage: vstat { obj | mob } <number>")

	c.send("vstat obj 100")
	c.expect("Name: 'a long sword'")

	c.send("vstat obj 99999")
	c.expect("There is no object with that number.")

	c.send("vstat mob 999")
	c.expect("a large dog")

	c.send("vstat mob 99999")
	c.expect("There is no monster with that number.")

	c.send("vstat thing 100")
	c.expect("That'll have to be either 'obj' or 'mob'.")

	// vstat must not leave the mobile it made standing about.
	c.settle()
	var here int
	inWorld(t, srv, func(w *game.Live) {
		here = len(w.Occupants(ImmortStartRoom))
	})
	if here != 1 {
		t.Errorf("%d characters in the room after vstat, want just the caller", here)
	}
}

// A mortal cannot see these commands at all.
func TestMortalsCannotStat(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	first := dialClient(t, addr)
	first.create("Impl", "thefirstone", "m", "w")

	c := dialClient(t, addr)
	c.create("Nosy", "cannotlook", "m", "w")
	setLevel(t, srv, "Nosy", 10)

	for _, command := range []string{"stat room", "vstat obj 100", "vnum obj sword"} {
		c.send(command)
		c.expect("Huh?!?")
	}
}
