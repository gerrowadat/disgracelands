// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"testing"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// The wizard commands that change things.

func countHere(t *testing.T, srv *Server, room game.RoomVnum) (people, objects int) {
	t.Helper()
	inWorld(t, srv, func(w *game.Live) {
		people = len(w.Occupants(room))
		objects = len(w.RoomObjects(room))
	})
	return people, objects
}

func TestLoad(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Maker", "outofnothing", "m", "w")

	c.send("load")
	c.expect("Usage: load { obj | mob } <number>")

	c.send("load thing 100")
	c.expect("That'll have to be either 'obj' or 'mob'.")

	c.send("load obj 99999")
	c.expect("There is no object with that number.")

	c.send("load obj 100")
	c.expect("You create a long sword.")

	// load_into_inventory is YES, so it is in their hands, not on the floor.
	var carrying int
	inWorld(t, srv, func(w *game.Live) {
		if who := w.Find("Maker"); who != nil {
			carrying = len(who.Carrying)
		}
	})
	if carrying != 1 {
		t.Errorf("a loaded object left the loader carrying %d things, want 1", carrying)
	}

	c.send("load mob 999")
	c.expect("You create a large dog.")

	people, _ := countHere(t, srv, ImmortStartRoom)
	if people != 2 {
		t.Errorf("%d characters in the room after loading a mobile, want 2", people)
	}
}

func TestPurgeOneThing(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Cleaner", "scorchedearth", "m", "w")

	c.send("purge nothinglikethat")
	c.expect("Nothing here by that name.")

	aMobile(t, srv, "Cleaner")
	c.send("purge dog")
	c.expect("Okay.")

	people, _ := countHere(t, srv, ImmortStartRoom)
	if people != 1 {
		t.Errorf("%d characters after purging the dog, want just the caller", people)
	}

	inWorld(t, srv, func(w *game.Live) {
		if sword := w.NewObject(testSwordVnum); sword != nil {
			w.ObjectToRoom(sword, ImmortStartRoom)
		}
	})
	c.send("purge sword")
	c.expectCount("Okay.", 2)

	_, objects := countHere(t, srv, ImmortStartRoom)
	if objects != 0 {
		t.Errorf("%d objects after purging the sword, want none", objects)
	}
}

func TestPurgeTheWholeRoom(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Sweeper", "cleanitall", "m", "w")

	aMobile(t, srv, "Sweeper")
	inWorld(t, srv, func(w *game.Live) {
		for _, vnum := range []game.ObjVnum{testSwordVnum, testRingVnum} {
			if obj := w.NewObject(vnum); obj != nil {
				w.ObjectToRoom(obj, ImmortStartRoom)
			}
		}
	})

	c.send("purge")
	c.expect("The world seems a little cleaner.")
	c.settle()

	people, objects := countHere(t, srv, ImmortStartRoom)
	if people != 1 {
		t.Errorf("%d characters after purging the room, want just the caller", people)
	}
	if objects != 0 {
		t.Errorf("%d objects after purging the room, want none", objects)
	}
}

// You cannot purge somebody at or above your own level — and the test is
// `<=`, so not even an equal.
func TestYouCannotPurgeYourEquals(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	first := dialClient(t, addr)
	first.create("Alpha", "firstgodhere", "m", "w")

	second := dialClient(t, addr)
	second.create("Beta", "secondgodhere", "m", "w")
	setLevel(t, srv, "Beta", game.LevelImplementor)
	moveTo(t, srv, "Beta", ImmortStartRoom)

	first.send("purge beta")
	first.expect("Fuuuuuuuuu!")
}

func TestRestore(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Healer", "makethemwell", "m", "w")

	dog := aMobile(t, srv, "Healer")
	inWorld(t, srv, func(_ *game.Live) {
		if dog != nil && dog.Record != nil {
			dog.Record.Points.Hit = 1
		}
	})

	c.send("restore")
	c.expect("Whom do you wish to restore?")

	c.send("restore nobodyatall")
	c.expect("No-one by that name here.")

	c.send("restore dog")
	c.expect("Okay.")

	var hit, maxHit int32
	inWorld(t, srv, func(_ *game.Live) {
		if dog != nil && dog.Record != nil {
			hit, maxHit = dog.Record.Points.Hit, dog.Record.Points.MaxHit
		}
	})
	if hit != maxHit {
		t.Errorf("after restoring, the dog has %d of %d hit points", hit, maxHit)
	}
}

func TestAdvance(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	god := dialClient(t, addr)
	god.create("Promoter", "riseupnow", "m", "w")

	victim := dialClient(t, addr)
	victim.create("Climber", "goingupward", "m", "w")
	setLevel(t, srv, "Climber", 5)

	god.send("advance")
	god.expect("Advance who?")

	god.send("advance nobodyatall 10")
	god.expect("That player is not here.")

	god.send("advance climber")
	god.expect("That's not a level!")

	god.send("advance climber 99")
	god.expect("34 is the highest possible level.")

	god.send("advance climber 5")
	god.expect("They are already at that level.")

	god.send("advance climber 10")
	god.expect("Okay.")
	victim.expect("You feel slightly different.")

	var level int32
	inWorld(t, srv, func(w *game.Live) {
		if who := w.Find("Climber"); who != nil && who.Record != nil {
			level = who.Record.Level
		}
	})
	if level != 10 {
		t.Errorf("after advancing they are level %d, want 10", level)
	}

	// And back down, which runs do_start first and so costs them everything.
	god.send("advance climber 3")
	victim.expect("You feel somewhat diminished.")
	inWorld(t, srv, func(w *game.Live) {
		if who := w.Find("Climber"); who != nil && who.Record != nil {
			level = who.Record.Level
		}
	})
	if level != 3 {
		t.Errorf("after demoting they are level %d, want 3", level)
	}
}

func TestFreezeAndThaw(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	god := dialClient(t, addr)
	god.create("Winter", "coldashell", "m", "w")

	victim := dialClient(t, addr)
	victim.create("Chilly", "brrrrritscold", "m", "w")
	setLevel(t, srv, "Chilly", 10)

	god.send("thaw chilly")
	god.expect("Sorry, your victim is not morbidly encased in ice at the moment.")

	god.send("freeze winter")
	god.expect("Oh, yeah, THAT'S real smart...")

	god.send("freeze chilly")
	god.expect("Frozen.")
	victim.expect("You feel frozen!")

	var frozen bool
	inWorld(t, srv, func(w *game.Live) {
		if who := w.Find("Chilly"); who != nil && who.Record != nil {
			frozen = who.Record.PlayerFlags.Has(game.PlayerFrozen)
		}
	})
	if !frozen {
		t.Error("the victim is not flagged frozen")
	}

	god.send("freeze chilly")
	god.expect("Your victim is already pretty cold.")

	god.send("thaw chilly")
	god.expect("Thawed.")
	victim.expect("You feel thawed.")
}

// A lesser god cannot undo a greater one's freeze: the level of whoever did
// it is remembered.
func TestYouCannotThawAboveYourLevel(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	first := dialClient(t, addr)
	first.create("Bigger", "toppower", "m", "w")

	victim := dialClient(t, addr)
	victim.create("Frosty", "onthereceiving", "m", "w")
	setLevel(t, srv, "Frosty", 10)

	first.send("freeze frosty")
	first.expect("Frozen.")

	// Now drop the god below the freeze level and try to undo it.
	setLevel(t, srv, "Bigger", game.LevelGreaterGod)
	first.send("thaw frosty")
	first.expect("you can't unfreeze")
}

func TestPardonAndMute(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	god := dialClient(t, addr)
	god.create("Judge", "lawandorder", "m", "w")

	victim := dialClient(t, addr)
	victim.create("Rascal", "upofnogood", "m", "w")
	setLevel(t, srv, "Rascal", 10)

	god.send("pardon rascal")
	god.expect("Your victim is not flagged.")

	inWorld(t, srv, func(w *game.Live) {
		if who := w.Find("Rascal"); who != nil && who.Record != nil {
			who.Record.PlayerFlags = who.Record.PlayerFlags.Set(game.PlayerThief)
		}
	})

	god.send("pardon rascal")
	god.expect("Pardoned.")
	victim.expect("You have been pardoned by the Gods!")

	// `mute`, not `squelch` — the subcommand is SCMD_SQUELCH but the word is
	// mute.
	god.send("mute rascal")
	god.expect("Squelch ON for Rascal")
	god.send("mute rascal")
	god.expect("Squelch OFF for Rascal")
}

func TestUnaffect(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	god := dialClient(t, addr)
	god.create("Dispeller", "beginagain", "m", "w")

	victim := dialClient(t, addr)
	victim.create("Enchanted", "fullofmagic", "m", "w")
	setLevel(t, srv, "Enchanted", 10)

	god.send("unaffect enchanted")
	god.expect("Your victim does not have any affections!")

	inWorld(t, srv, func(w *game.Live) {
		who := w.Find("Enchanted")
		if who == nil || who.Record == nil {
			t.Error("no victim")
			return
		}
		who.Record.Affects = append(who.Record.Affects, game.Affect{
			Type: game.SpellSanctuary, Duration: 10, Bits: game.AffectSanctuary,
		})
		game.RecomputeAffects(who.Record)
	})

	god.send("unaffect enchanted")
	god.expect("All spells removed.")
	victim.expect("You feel slightly different.")

	var affects int
	inWorld(t, srv, func(w *game.Live) {
		if who := w.Find("Enchanted"); who != nil && who.Record != nil {
			affects = len(who.Record.Affects)
		}
	})
	if affects != 0 {
		t.Errorf("%d affects left after unaffect", affects)
	}
}

func TestWizutilRefusals(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Official", "byotherule", "m", "w")

	aMobile(t, srv, "Official")

	c.send("pardon")
	c.expect("Yes, but for whom?!?")

	c.send("pardon nobodyatall")
	c.expect("There is no such player.")

	c.send("pardon dog")
	c.expect("You can't do that to a mob!")
}

func TestZreset(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Rewinder", "startagain", "m", "w")

	c.send("zreset")
	c.expect("You must specify a zone.")

	c.send("zreset 999")
	c.expect("Invalid zone number.")

	// The board room is in zone 12.
	c.send("zreset 12")
	c.expect("Reset zone 12")

	c.send("zreset .")
	c.expectCount("Reset zone 12", 2)

	c.send("zreset *")
	c.expect("Reset world.")
}

// The levels are part of matching, so a mortal sees none of these.
func TestMortalsCannotChangeThings(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	first := dialClient(t, addr)
	first.create("Almighty", "thefirstgod", "m", "w")

	c := dialClient(t, addr)
	c.create("Meek", "nopowerhere", "m", "w")
	setLevel(t, srv, "Meek", 10)

	for _, command := range []string{
		"load obj 100", "purge", "restore meek", "advance meek 30",
		"freeze meek", "reroll meek", "zreset 12",
	} {
		c.send(command)
		c.expect("Huh?!?")
	}
}
