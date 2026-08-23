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

// The pet shop, end to end — SPECIAL(pet_shops) (spec_procs.c:951), the
// last mobile/room special besides the mayor (docs/deviations.md).
//
// Unlike every other shop in the game it has no keeper: the room itself
// answers `list` and `buy`, and the animals for sale live in the room one
// vnum higher (PetShopBackRoom), found by arithmetic on the room the buyer
// is standing in rather than by any lookup — the same blunt way the C does
// it.

// atPetShop moves a character to the shop's counter and puts a dog in the
// back room for them to buy.
func atPetShop(t *testing.T, srv *Server, name string) {
	t.Helper()
	inWorld(t, srv, func(w *game.Live) {
		who := w.Find(name)
		if who == nil {
			t.Error("the character is not in the world")
			return
		}
		if err := w.Enter(who, PetShopRoom); err != nil {
			t.Errorf("moving to the pet shop: %v", err)
			return
		}
		if len(w.Occupants(PetShopBackRoom)) == 0 {
			if pet := w.SpawnMobile(testDogVnum, PetShopBackRoom, srv.rng); pet == nil {
				t.Error("could not stock the pet shop")
			}
		}
	})
}

func TestPetShopListsItsStock(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Browser", "lookingfordogs", "m", "m")
	atPetShop(t, srv, "Browser")

	c.send("list")
	c.expect("Available pets are:")
	// testDogVnum is level 5; PET_PRICE is level * 300.
	c.expect("1500 - a large dog")
}

func TestBuyingAPet(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Owner", "iwantapuppy", "m", "m")
	atPetShop(t, srv, "Owner")

	inWorld(t, srv, func(w *game.Live) {
		if who := w.Find("Owner"); who != nil && who.Record != nil {
			who.Record.Points.Gold = 2000
		}
	})

	c.send("buy dog Rex")
	c.expect("May you enjoy your pet.")

	var (
		gold                   int32
		followerName           string
		followerCharmed        bool
		followerFollowsOwner   bool
		followerKeywordsHasRex bool
		followerExp            int32 = -1
	)
	inWorld(t, srv, func(w *game.Live) {
		owner := w.Find("Owner")
		if owner == nil || owner.Record == nil {
			t.Error("the owner vanished")
			return
		}
		gold = owner.Record.Points.Gold
		if len(owner.Followers) != 1 {
			t.Errorf("owner has %d followers, want 1", len(owner.Followers))
			return
		}
		pet := owner.Followers[0]
		followerName = pet.Name
		followerFollowsOwner = pet.Master == owner
		followerCharmed = pet.Record != nil && pet.Record.AffectFlags.Has(game.AffectCharm)
		// twoArguments (one_argument, interpreter.c:977) lowercases every
		// word it reads, the pet's name along with everything else — so
		// "buy dog Rex" really does store "rex", not "Rex", in the C too.
		followerKeywordsHasRex = strings.Contains(pet.Keywords, "rex")
		if pet.Record != nil {
			followerExp = pet.Record.Points.Exp
		}
	})

	if gold != 500 {
		t.Errorf("gold after buying is %d, want 500 (2000 - 1500)", gold)
	}
	if followerName != "a large dog" {
		t.Errorf("the follower is %q, want the dog's short description", followerName)
	}
	if !followerFollowsOwner {
		t.Error("the pet is not following its owner")
	}
	if !followerCharmed {
		t.Error("the pet is not permanently charmed")
	}
	if !followerKeywordsHasRex {
		t.Error("naming the pet did not add the name to its keywords")
	}
	if followerExp != 0 {
		t.Errorf("the pet's experience is %d, want 0", followerExp)
	}
}

func TestBuyingAPetYouCannotAfford(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Poor", "notenoughgold", "m", "m")
	atPetShop(t, srv, "Poor")

	inWorld(t, srv, func(w *game.Live) {
		if who := w.Find("Poor"); who != nil && who.Record != nil {
			who.Record.Points.Gold = 10
		}
	})

	c.send("buy dog")
	c.expect("You don't have enough gold!")
}

func TestBuyingAPetThatIsNotThere(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Confused", "nosuchanimal", "m", "m")
	atPetShop(t, srv, "Confused")

	c.send("buy elephant")
	c.expect("There is no such pet!")
}

// The pet shop only answers `list`/`buy`; everything else goes back to the
// ordinary command, the same as a real shopkeeper's do_not_here commands do.
func TestPetShopDoesNotSwallowOtherCommands(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Passerby", "justwalkingby", "m", "m")
	atPetShop(t, srv, "Passerby")

	c.send("look")
	c.expect("The Pet Shop")
}
