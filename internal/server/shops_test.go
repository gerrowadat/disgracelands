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

// The shopkeeper, end to end.
//
// `buy`, `sell`, `value` and `list` are all `do_not_here` in the command
// table, so every one of these tests is really a test that the special
// procedure took the command away first.

// inShop puts a freshly created character in the shop room with a keeper, and
// returns them some money.
func inShop(t *testing.T, srv *Server, name string, gold int32) {
	t.Helper()
	inWorld(t, srv, func(w *game.Live) {
		who := w.Find(name)
		if who == nil {
			t.Error("the character is not in the world")
			return
		}
		if who.Record != nil {
			who.Record.Points.Gold = gold
			// A mortal, so the god shortcut in is_ok_char and shopping_buy
			// does not fire — the first character on an empty roster is an
			// implementor, and an implementor shops for free.
			who.Record.Level = 10
		}
		if err := w.Enter(who, ShopRoom); err != nil {
			t.Errorf("moving into the shop: %v", err)
			return
		}
		keeper := w.SpawnMobile(testShopkeeperVnum, ShopRoom, srv.rng)
		if keeper == nil {
			t.Error("could not put a shopkeeper in the shop")
			return
		}
		// Stock the shop, as a zone reset would. A producing shop is not
		// magic: it still needs one of the item on the shelf, and what
		// `shop_producing` buys it is that the one on the shelf is never
		// sold — a copy is made instead.
		if sword := w.NewObject(testSwordVnum); sword != nil {
			w.ObjectToChar(sword, keeper)
		}
	})
}

func goldOf(t *testing.T, srv *Server, name string) int32 {
	t.Helper()
	var n int32
	inWorld(t, srv, func(w *game.Live) {
		if who := w.Find(name); who != nil && who.Record != nil {
			n = who.Record.Points.Gold
		}
	})
	return n
}

func TestBuyingSomethingFromAShop(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Custom", "spendthrift", "m", "m")
	inShop(t, srv, "Custom", 1000)

	// The sword costs 100 and the shop's markup is 1.15, so it is 114 — see
	// docs/weirdnumbers.md on why it is not 115.
	c.send("buy sword")
	c.expect("That'll be 114 coins, please.")
	c.expect("You now have a long sword.")

	if got := goldOf(t, srv, "Custom"); got != 1000-114 {
		t.Errorf("after buying a 114-coin sword the buyer has %d, want %d", got, 1000-114)
	}

	var carrying int
	inWorld(t, srv, func(w *game.Live) {
		if who := w.Find("Custom"); who != nil {
			carrying = len(who.Carrying)
		}
	})
	if carrying != 1 {
		t.Errorf("the buyer is carrying %d things, want 1", carrying)
	}
}

// A shop that produces an item never runs out, so buying one leaves the
// keeper's own copy where it was.
func TestAProducingShopNeverRunsOut(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Regular", "everyday", "m", "m")
	inShop(t, srv, "Regular", 10_000)

	for i := 0; i < 3; i++ {
		c.send("buy sword")
		c.expectCount("You now have a long sword.", i+1)
	}

	var stock int
	inWorld(t, srv, func(w *game.Live) {
		if keeper := w.FindInRoom(nil, ShopRoom, "shopkeeper"); keeper != nil {
			stock = len(keeper.Carrying)
		}
	})
	if stock != 1 {
		t.Errorf("after three sales the keeper has %d swords, want 1 — it produces them", stock)
	}
}

func TestSellingSomethingToAShop(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Vendor", "cashinhand", "m", "m")
	inShop(t, srv, "Vendor", 0)

	inWorld(t, srv, func(w *game.Live) {
		who, sword := w.Find("Vendor"), w.NewObject(testSwordVnum)
		if who == nil || sword == nil {
			t.Error("could not give the seller a sword")
			return
		}
		w.ObjectToChar(sword, who)
	})

	// 100 at a 0.15 markdown is 15.
	c.send("value sword")
	c.expect("I'll give you 15 gold coins for that!")

	c.send("sell sword")
	c.expect("You'll get 15 coins for it!")

	if got := goldOf(t, srv, "Vendor"); got != 15 {
		t.Errorf("after selling a sword the seller has %d gold, want 15", got)
	}
}

// The shop buys weapons and wands and nothing else, and has a different
// refusal for each reason.
func TestAShopRefusesWhatItDoesNotBuy(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Hawker", "junkdealer", "m", "m")
	inShop(t, srv, "Hawker", 0)

	inWorld(t, srv, func(w *game.Live) {
		who := w.Find("Hawker")
		if who == nil {
			t.Error("the character is not in the world")
			return
		}
		// A ring: not a type this shop deals in.
		if ring := w.NewObject(testRingVnum); ring != nil {
			w.ObjectToChar(ring, who)
		}
		// A wand with no charges: the right type, refused anyway.
		if wand := w.NewObject(testWandVnum); wand != nil {
			wand.Values[2] = 0
			w.ObjectToChar(wand, who)
		}
	})

	c.send("sell ring")
	c.expect("I don't buy such items.")

	c.send("sell wand")
	c.expect("I don't buy used up wands or staves!")
}

func TestYouCannotBuyWhatYouCannotAfford(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Pauper", "notacoin", "m", "m")
	inShop(t, srv, "Pauper", 5)

	c.send("buy sword")
	c.expect("You can't afford it!")
	// Temper 0 is the shop's opinion of somebody who wastes its time.
	c.expect("pukes on you")

	if got := goldOf(t, srv, "Pauper"); got != 5 {
		t.Errorf("a failed purchase cost %d gold", 5-got)
	}
}

// `list` groups identical items and numbers them, and the number is what
// `buy #n` refers to.
func TestListingAShopsStock(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Browser", "justlooking", "m", "m")
	inShop(t, srv, "Browser", 10_000)

	inWorld(t, srv, func(w *game.Live) {
		keeper := w.FindInRoom(nil, ShopRoom, "shopkeeper")
		if keeper == nil {
			t.Error("no shopkeeper")
			return
		}
		// Two identical rings, which should group into one line with a count
		// of 2, and one produced sword, which should say Unlimited.
		for _, vnum := range []game.ObjVnum{testRingVnum, testRingVnum} {
			if obj := w.NewObject(vnum); obj != nil {
				w.ObjectToChar(obj, keeper)
			}
		}
		w.SortShopObjects(w.ShopFor(keeper), keeper)
	})

	c.send("list")
	c.expect("Available")
	c.settle()

	transcript := c.transcript()
	if !strings.Contains(transcript, "Unlimited") {
		t.Error("the produced sword is not listed as Unlimited")
	}
	if !strings.Contains(transcript, "    2       ") {
		t.Errorf("the two rings did not group into a count of 2:\n%s", transcript)
	}

	// The sword is the produced item and is item 1 or 2; buying by number
	// has to work either way, so check the one the listing actually gives.
	c.send("buy #1")
	c.expect("You now have")
}

// Outside the shop the same words are `do_not_here` and say so.
func TestShopCommandsDoNothingOutsideAShop(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Wanderer", "nowhereman", "m", "m")

	for _, command := range []string{"buy sword", "sell sword", "value sword", "list"} {
		c.send(command)
		c.expect("Sorry, but you cannot do that here!")
	}
}

// A shut shop says so, to the whole room, and serves nobody.
func TestAShutShopServesNobody(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Earlybird", "toosoon", "m", "m")
	inShop(t, srv, "Earlybird", 1000)

	inWorld(t, srv, func(w *game.Live) {
		// Shut for the whole day: hour 0 is before Open1, whatever the clock
		// says.
		for _, shop := range w.Shops() {
			shop.Open1, shop.Close1 = 25, 26
			shop.Open2, shop.Close2 = 25, 26
		}
	})

	c.send("buy sword")
	c.expect("Come back later!")

	if got := goldOf(t, srv, "Earlybird"); got != 1000 {
		t.Errorf("a shut shop took %d gold", 1000-got)
	}
}

// ok_damage_shopkeeper: a shop without WILL_FIGHT cannot be hurt, and the
// check is in damage() rather than in `kill`, so every route is covered.
func TestAShopkeeperWithoutWillFightCannotBeHurt(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Bruiser", "fistfight", "m", "w")
	inShop(t, srv, "Bruiser", 0)

	var before int32
	inWorld(t, srv, func(w *game.Live) {
		if keeper := w.FindInRoom(nil, ShopRoom, "shopkeeper"); keeper != nil && keeper.Record != nil {
			before = keeper.Record.Points.Hit
		}
	})

	c.send("hit shopkeeper")
	c.expect("Get out of here before I call the guards!")
	c.settle()

	var after int32
	inWorld(t, srv, func(w *game.Live) {
		if keeper := w.FindInRoom(nil, ShopRoom, "shopkeeper"); keeper != nil && keeper.Record != nil {
			after = keeper.Record.Points.Hit
		}
	})
	if after != before {
		t.Errorf("the shopkeeper lost %d hit points; a shop without WILL_FIGHT is untouchable",
			before-after)
	}
}

// With WILL_FIGHT set they are an ordinary mobile again.
func TestAShopkeeperWithWillFightCanBeHurt(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Brawler", "haveago", "m", "w")
	inShop(t, srv, "Brawler", 0)

	inWorld(t, srv, func(w *game.Live) {
		for _, shop := range w.Shops() {
			shop.Flags = shop.Flags.With(game.ShopWillFight)
		}
	})

	c.send("hit shopkeeper")
	c.expect("the shopkeeper") // present in every damage tier's text, hit or miss
}

// A shop message is routed through do_tell, so the customer's own name — the
// leading `%s` every shop message in the data files starts with — is consumed
// as the addressee and never appears in what they hear. Issue #191: the port
// used to send the formatted string straight out, so a customer called Custom
// was told "the shopkeeper tells you, 'Custom What do you want to buy??'".
//
// act() also capitalises the line, which is where the capital in "The
// shopkeeper" comes from: the mobile's own name is lower case.
func TestAShopMessageDoesNotEchoTheCustomersName(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Custom", "spendthrift", "m", "m")
	inShop(t, srv, "Custom", 1000)

	c.send("buy")
	c.expect("What do you want to buy??")

	line := "The shopkeeper tells you, 'What do you want to buy??'"
	if !c.seen(line) {
		t.Errorf("the shop said something other than %q:\n%s", line, c.transcript())
	}
	if strings.Contains(c.transcript(), "Custom What do you want") {
		t.Errorf("the customer's name leaked into the shop's message:\n%s", c.transcript())
	}
}
