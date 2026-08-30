// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package session

import (
	"fmt"
	"strings"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// `show shops`, ported from shop.c's own show_shops (:1350),
// list_all_shops (:1220) and list_detailed_shop (:1258).

// shopBitNames are shop_bits[] (shop.c:110), matching ShopWillFight/
// ShopUsesBank's bit order.
var shopBitNames = []string{"WILL_FIGHT", "USES_BANK"}

// showShops is show_shops: the summary table with no argument, or one
// shop's detail view given a number or `.` for the room stood in.
func (c *Context) showShops(value string, self bool) {
	shops := c.World.Shops()

	if value == "" {
		c.listAllShops(shops)
		return
	}

	index := -1
	switch {
	case self:
		for i, shop := range shops {
			if game.ShopServesRoom(shop, c.Character.Room) {
				index = i
				break
			}
		}
		if index < 0 {
			c.Send("This isn't a shop!\r\n")
			return
		}
	case isNumber(value):
		index = int(atoi(value)) - 1
	}

	if index < 0 || index >= len(shops) {
		c.Send("Illegal shop number.\r\n")
		return
	}
	c.listDetailedShop(shops[index], index)
}

// listAllShops is list_all_shops (shop.c:1220): one row per shop, a fresh
// header every PAGE_LENGTH-2 rows the same way the C repeats it down a
// page_string-driven listing.
func (c *Context) listAllShops(shops []*game.ShopDef) {
	var b strings.Builder
	for i, shop := range shops {
		if i%(pageLength-2) == 0 {
			b.WriteString(" ##   Virtual   Where    Keeper    Buy   Sell   Customers\r\n")
			b.WriteString("---------------------------------------------------------\r\n")
		}

		room := game.RoomVnum(0)
		if len(shop.Rooms) > 0 {
			room = shop.Rooms[0]
		}
		keeper := "<NONE>"
		if shop.Keeper != game.NoMob {
			keeper = fmt.Sprintf("%6d", shop.Keeper)
		}
		// The header reads "Buy   Sell", but the C's own sprintf plugs
		// ProfitSell under the first and ProfitBuy under the second
		// (shop.c:1236-1237) — a genuine swapped-column bug, not a
		// transcription error here. Reproduced rather than fixed; see
		// docs/weirdnumbers.md.
		fmt.Fprintf(&b, "%3d   %6d   %6d    %s   %3.2f   %3.2f    %s\r\n",
			i+1, shop.Vnum, room, keeper, shop.ProfitSell, shop.ProfitBuy,
			game.CustomerString(shop, false))
	}
	c.SendPaged("%s", b.String())
}

// listDetailedShop is list_detailed_shop (shop.c:1258).
//
// handle_detailed_list's own column-wrapping (breaking a long Rooms/
// Produces/Buys line at ~78 characters with a 12-space continuation
// indent) is not reproduced: every list this port's shops actually hold is
// short enough that the C would never wrap it either, and chasing an exact
// break column for lines nobody has is not worth the code. See
// docs/deviations.md.
func (c *Context) listDetailedShop(shop *game.ShopDef, index int) {
	c.Send("Vnum:       [%5d], Rnum: [%5d]\r\n", shop.Vnum, index+1)

	if len(shop.Rooms) == 0 {
		c.Send("Rooms:      None!\r\n")
	} else {
		items := make([]string, len(shop.Rooms))
		for i, vnum := range shop.Rooms {
			if room := c.World.Room(vnum); room != nil {
				items[i] = fmt.Sprintf("%s (#%d)", room.Name, vnum)
			} else {
				items[i] = fmt.Sprintf("<UNKNOWN> (#%d)", vnum)
			}
		}
		c.Send("Rooms:      %s\r\n", strings.Join(items, ", "))
	}

	if shop.Keeper == game.NoMob {
		c.Send("Shopkeeper: <NONE>\r\n")
	} else {
		name := fmt.Sprintf("#%d", shop.Keeper)
		if def := c.World.MobileDef(shop.Keeper); def != nil {
			name = fmt.Sprintf("%s (#%d)", def.ShortDesc, shop.Keeper)
		}
		c.Send("Shopkeeper: %s, Special Function: %s\r\n", name, capsYesNo(shop.Secondary != ""))

		if keeper := c.liveShopkeeper(shop); keeper != nil {
			bank := c.World.ShopBank(shop)
			c.Send("Coins:      [%9d], Bank: [%9d] (Total: %d)\r\n",
				gold(keeper), bank, gold(keeper)+bank)
		}
	}

	customers := game.CustomerString(shop, true)
	if customers == "" {
		customers = "None"
	}
	c.Send("Customers:  %s\r\n", customers)

	if len(shop.Producing) == 0 {
		c.Send("Produces:   Nothing!\r\n")
	} else {
		items := make([]string, len(shop.Producing))
		for i, vnum := range shop.Producing {
			desc := "<UNKNOWN>"
			if def := c.World.ObjectDef(vnum); def != nil {
				desc = def.ShortDesc
			}
			items[i] = fmt.Sprintf("%s (#%d)", desc, vnum)
		}
		c.Send("Produces:   %s\r\n", strings.Join(items, ", "))
	}

	if len(shop.BuyTypes) == 0 {
		c.Send("Buys:       Nothing!\r\n")
	} else {
		items := make([]string, len(shop.BuyTypes))
		for i, bt := range shop.BuyTypes {
			word := "[all]"
			if bt.Keyword != "" {
				word = fmt.Sprintf("[%s]", bt.Keyword)
			}
			items[i] = fmt.Sprintf("%s (#%d) %s", game.SprintType(bt.Type, game.ItemTypeNames), bt.Type, word)
		}
		c.Send("Buys:       %s\r\n", strings.Join(items, ", "))
	}

	// "Buy at:"/"Sell at:" carries the same swapped fields list_all_shops
	// does (ProfitSell first, ProfitBuy second) — the same C bug, at
	// shop.c:1338-1339 this time.
	c.Send("Buy at:     [%4.2f], Sell at: [%4.2f], Open: [%d-%d, %d-%d]\r\n",
		shop.ProfitSell, shop.ProfitBuy, shop.Open1, shop.Close1, shop.Open2, shop.Close2)

	c.Send("Bits:       %s\r\n", game.SprintBit(shop.Flags.Raw(), shopBitNames))
}

// capsYesNo is YESNO (utils.h:124) — all-caps, unlike this file's own
// yesNo (wizstat.go), which answers to a different macro at its own call
// site and would print the wrong case here.
func capsYesNo(b bool) string {
	if b {
		return "YES"
	}
	return "NO"
}

// liveShopkeeper finds the live mobile keeping a shop, if one is spawned —
// get_char_num's own lookup, done here by scanning the world's live
// mobiles for the keeper's vnum, since this port has no rnum-indexed table
// of live characters the way the C's own character_list traversal does.
func (c *Context) liveShopkeeper(shop *game.ShopDef) *game.Character {
	for _, m := range c.World.Mobiles() {
		if m.MobDef != nil && m.MobDef.Vnum == shop.Keeper {
			return m
		}
	}
	return nil
}
