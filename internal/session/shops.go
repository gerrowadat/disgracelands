// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package session

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// shop_keeper, ported from shop.c.
//
// Shops are not commands. `buy`, `sell`, `value` and `list` are all
// `do_not_here` in the command table — they answer "Sorry, but you cannot do
// that here!" and mean it — and what makes them work is a mobile standing in
// the room with the `shop_keeper` special attached. Walk out and the same
// word stops working.
//
// Everything the keeper says goes through `tell`, addressed by name, which is
// why every shop message in the data files begins with a `%s`.

// specShopKeeper is SPECIAL(shop_keeper) (shop.c:889).
func specShopKeeper(sc *SpecialCall) bool {
	keeper := sc.Mob
	shop := sc.World.ShopFor(keeper)
	if shop == nil {
		return false
	}

	// The keeper's own special, if it had one before it became a shopkeeper.
	// It gets first refusal, and can take the command away from the shop.
	if shop.Secondary != "" {
		if fn, ok := specials[shop.Secondary]; ok && fn(sc) {
			return true
		}
	}

	// A keeper acting on themselves — "drop all", say — invalidates the
	// sorted order and handles nothing. The C's comment is "Safety in case
	// 'drop all'".
	if keeper == sc.Actor {
		if !sc.Pulse() {
			sc.World.ResetShopSort(shop)
		}
		return false
	}
	if !game.ShopServesRoom(shop, sc.Actor.Room) {
		return false
	}
	if !keeper.Position.Awake() {
		return false
	}

	switch {
	case sc.Is("buy"):
		shoppingBuy(sc, shop, keeper)
	case sc.Is("sell"):
		shoppingSell(sc, shop, keeper)
	case sc.Is("value"):
		shoppingValue(sc, shop, keeper)
	case sc.Is("list"):
		shoppingList(sc, shop, keeper)
	default:
		return false
	}
	return true
}

// keeperTells is do_tell(keeper, ...): the keeper addresses one customer by
// name. Every shop message in the data files is formatted this way.
func keeperTells(sc *SpecialCall, keeper *game.Character, format string, args ...any) {
	sc.Actor.Tell("%s tells you, '%s'\r\n", keeper.Name, fmt.Sprintf(format, args...))
}

// keeperSays is do_say: to the whole room. `is_open` uses this for the
// closing messages, which is why everybody hears a shop turn somebody away.
func keeperSays(sc *SpecialCall, keeper *game.Character, message string) {
	for _, who := range sc.World.Occupants(keeper.Room) {
		if who != keeper {
			who.Tell("%s says, '%s'\r\n", keeper.Name, message)
		}
	}
}

// shopIsOK is is_ok (shop.c:171): open, and willing to deal with you.
//
// Order matters and is the C's: closed is checked first, so a shop that will
// not serve your class still tells you it is shut rather than insulting you.
func shopIsOK(sc *SpecialCall, shop *game.ShopDef, keeper *game.Character) bool {
	if open, message := game.ShopIsOpen(shop, sc.World.MudTime().Hours); !open {
		keeperSays(sc, keeper, message)
		return false
	}
	if ok, message := game.ShopWillDealWith(shop, sc.Actor); !ok {
		keeperTells(sc, keeper, "%s", message)
		return false
	}
	return true
}

// transactionAmount is transaction_amt (shop.c:353): pulls a leading count
// off the argument, leaving the rest.
//
// "buy 5 3" is five of item three; "buy 5" is item five. The C only treats a
// leading number as a count when something follows it, which is the whole
// distinction.
func transactionAmount(arg string) (int, string) {
	first, rest := oneArgument(arg)
	if rest != "" && first != "" && isNumber(first) {
		n, err := strconv.Atoi(first)
		if err != nil {
			return 1, arg
		}
		return n, rest
	}
	return 1, arg
}

// timesMessage is times_message (shop.c:373): the phrase naming what changed
// hands, with a count if there was more than one.
func timesMessage(obj *game.Object, name string, num int) string {
	var out string
	if obj != nil {
		out = obj.Name()
	} else {
		// A name typed as "3.sword" names the third sword; only the word
		// after the dot is worth repeating back.
		word := name
		if i := strings.IndexByte(name, '.'); i >= 0 {
			word = name[i+1:]
		}
		out = article(word) + " " + word
	}
	if num > 1 {
		out += fmt.Sprintf(" (x %d)", num)
	}
	return out
}

// purchaseObject is get_purchase_obj (shop.c:445): what the keeper has that
// answers to this name, by keyword or by "#3" list position.
//
// The C loops, extracting anything it finds that is worth nothing, because a
// zero-cost item in a keeper's inventory would otherwise be unbuyable and
// permanently in the way. This does the same.
func purchaseObject(sc *SpecialCall, shop *game.ShopDef, keeper *game.Character,
	arg string, complain bool,
) *game.Object {
	name, _ := oneArgument(arg)
	for {
		var obj *game.Object
		if strings.HasPrefix(name, "#") || isNumber(name) {
			obj = shopObjectByIndex(sc, shop, keeper, strings.TrimPrefix(name, "#"))
		} else {
			obj = sc.World.NewSearch(sc.Actor, name).ObjectIn(keeper.Carrying)
		}
		if obj == nil {
			if complain {
				keeperTells(sc, keeper, "%s", shopMessage(shop, game.MsgNoSuchItem1, sc.Actor.Name))
			}
			return nil
		}
		if obj.Cost > 0 {
			return obj
		}
		sc.World.ExtractObject(obj)
	}
}

// shopObjectByIndex is get_hash_obj_vis: the n'th *distinct* item in the
// keeper's list, which is the number the listing prints.
func shopObjectByIndex(sc *SpecialCall, shop *game.ShopDef, keeper *game.Character, digits string) *game.Object {
	want, err := strconv.Atoi(digits)
	if err != nil || want < 1 {
		return nil
	}
	var last *game.Object
	seen := 0
	for _, obj := range keeper.Carrying {
		if obj.Cost <= 0 {
			continue
		}
		if last == nil || !game.SameObject(last, obj) {
			seen++
			last = obj
		}
		if seen == want {
			return obj
		}
	}
	_ = shop
	return nil
}

// shopMessage returns one of the shop's canned lines with the customer's
// name (and, for the two money ones, an amount) filled in.
//
// The C sprintfs the file's string directly, so a shop file with the wrong
// number of `%s` in a message crashes the server. Here a message the shop
// does not have falls back to the C's own default text rather than to
// nothing, because a silent shopkeeper is indistinguishable from a broken
// one.
func shopMessage(shop *game.ShopDef, which int, args ...any) string {
	format := shop.Messages[which]
	if format == "" {
		format = defaultShopMessages[which]
	}
	return safeSprintf(format, args...)
}

// defaultShopMessages are what boot_the_shops stores when a file's message
// fails validation (shop.c:1068).
var defaultShopMessages = [game.NumShopMessages]string{
	game.MsgNoSuchItem1:  "%s Sorry, I don't stock that item.",
	game.MsgNoSuchItem2:  "%s You don't seem to have that.",
	game.MsgDoNotBuy:     "%s I don't buy that sort of thing.",
	game.MsgMissingCash1: "%s I can't afford that!",
	game.MsgMissingCash2: "%s You can't afford that!",
	game.MsgBuy:          "%s That'll be %d coins, thanks.",
	game.MsgSell:         "%s Here's your %d coins.",
}

// safeSprintf formats without producing Go's %!s(MISSING) noise when a shop
// file's message has fewer verbs than we have arguments.
func safeSprintf(format string, args ...any) string {
	out := fmt.Sprintf(format, args...)
	if i := strings.Index(out, "%!"); i >= 0 {
		return strings.TrimRight(out[:i], " ")
	}
	return out
}

// shoppingBuy is shopping_buy (shop.c:480).
func shoppingBuy(sc *SpecialCall, shop *game.ShopDef, keeper *game.Character) {
	if !shopIsOK(sc, shop, keeper) {
		return
	}
	sc.World.SortShopObjects(shop, keeper)

	buynum, arg := transactionAmount(sc.Arg)
	if buynum < 0 {
		keeperTells(sc, keeper, "%s A negative amount?  Try selling me something.", sc.Actor.Name)
		return
	}
	if arg == "" || buynum == 0 {
		keeperTells(sc, keeper, "%s What do you want to buy??", sc.Actor.Name)
		return
	}

	obj := purchaseObject(sc, shop, keeper, arg, true)
	if obj == nil {
		return
	}

	god := isGod(sc.Actor)
	if game.BuyPrice(shop, obj) > gold(sc.Actor) && !god {
		keeperTells(sc, keeper, "%s", shopMessage(shop, game.MsgMissingCash2, sc.Actor.Name))
		// The keeper's opinion of somebody who cannot pay. Temper 0 is a
		// social; temper 1 is an emote that has been in CircleMUD since 1993
		// and says what it says.
		switch shop.Temper {
		case 0:
			sc.ToRoom("%s pukes on %s.\r\n", keeper.Name, sc.Actor.Name)
			sc.Tell("%s pukes on you.\r\n", keeper.Name)
		case 1:
			sc.ToRoom("%s smokes on his joint.\r\n", keeper.Name)
			sc.Tell("%s smokes on his joint.\r\n", keeper.Name)
		}
		return
	}
	if len(sc.Actor.Carrying)+1 > carryCount(sc.Actor) {
		sc.Tell("%s: You can't carry any more items.\r\n", firstWord(obj.Keywords))
		return
	}
	if sc.Actor.CarriedWeight()+obj.Weight > carryWeight(sc.Actor) {
		sc.Tell("%s: You can't carry that much weight.\r\n", firstWord(obj.Keywords))
		return
	}

	var goldamt int32
	bought := 0
	var last *game.Object
	for obj != nil && (gold(sc.Actor) >= game.BuyPrice(shop, obj) || god) &&
		len(sc.Actor.Carrying) < carryCount(sc.Actor) && bought < buynum &&
		sc.Actor.CarriedWeight()+obj.Weight <= carryWeight(sc.Actor) {
		bought++

		if sc.World.ShopProduces(shop, obj) {
			// A shop that makes the thing never runs out: a fresh copy is
			// created and the keeper's own stays put.
			obj = sc.World.NewObject(obj.Def.Vnum)
			if obj == nil {
				break
			}
		} else {
			sc.World.ExtractFromChar(obj)
			sc.World.DecrementShopSort(shop)
		}
		sc.World.ObjectToChar(obj, sc.Actor)

		price := game.BuyPrice(shop, obj)
		goldamt += price
		if !god {
			addGold(sc.Actor, -price)
		}

		last = obj
		obj = purchaseObject(sc, shop, keeper, arg, false)
		if !game.SameObject(obj, last) {
			break
		}
	}

	if bought < buynum {
		switch {
		case obj == nil || !game.SameObject(last, obj):
			keeperTells(sc, keeper, "%s I only have %d to sell you.", sc.Actor.Name, bought)
		case gold(sc.Actor) < game.BuyPrice(shop, obj):
			keeperTells(sc, keeper, "%s You can only afford %d.", sc.Actor.Name, bought)
		case len(sc.Actor.Carrying) >= carryCount(sc.Actor):
			keeperTells(sc, keeper, "%s You can only hold %d.", sc.Actor.Name, bought)
		case sc.Actor.CarriedWeight()+obj.Weight > carryWeight(sc.Actor):
			keeperTells(sc, keeper, "%s You can only carry %d.", sc.Actor.Name, bought)
		default:
			keeperTells(sc, keeper, "%s Something screwy only gave you %d.", sc.Actor.Name, bought)
		}
	}
	if !god {
		addGold(keeper, goldamt)
	}

	// times_message(ch->carrying, 0, bought) — the *first thing in the
	// buyer's inventory*, not the thing bought. In the C those are the same
	// because obj_to_char prepends; here they are not, so the last thing
	// bought is named explicitly.
	what := timesMessage(last, "", bought)
	sc.ToRoom("%s buys %s.\r\n", sc.Actor.Name, what)
	keeperTells(sc, keeper, "%s", shopMessage(shop, game.MsgBuy, sc.Actor.Name, goldamt))
	sc.Tell("You now have %s.\r\n", what)

	sc.World.SettleShopBank(shop, keeper)
}

// sellingObject is get_selling_obj (shop.c:592): what in the customer's hands
// answers to this name, and whether the shop will take it.
func sellingObject(sc *SpecialCall, shop *game.ShopDef, keeper *game.Character,
	name string, complain bool,
) *game.Object {
	obj := sc.World.NewSearch(sc.Actor, name).ObjectIn(sc.Actor.Carrying)
	if obj == nil {
		if complain {
			keeperTells(sc, keeper, "%s", shopMessage(shop, game.MsgNoSuchItem2, sc.Actor.Name))
		}
		return nil
	}
	if game.TradeWith(shop, obj) == game.TradeOK {
		return obj
	}
	if !complain {
		return nil
	}

	switch game.TradeWith(shop, obj) {
	case game.TradeNoValue:
		keeperTells(sc, keeper, "%s You've got to be kidding, that thing is worthless!", sc.Actor.Name)
	case game.TradeNotOK:
		keeperTells(sc, keeper, "%s", shopMessage(shop, game.MsgDoNotBuy, sc.Actor.Name))
	case game.TradeDead:
		keeperTells(sc, keeper, "%s I don't buy used up wands or staves!", sc.Actor.Name)
	}
	return nil
}

// shoppingSell is shopping_sell (shop.c:702).
func shoppingSell(sc *SpecialCall, shop *game.ShopDef, keeper *game.Character) {
	if !shopIsOK(sc, shop, keeper) {
		return
	}

	sellnum, arg := transactionAmount(sc.Arg)
	if sellnum < 0 {
		keeperTells(sc, keeper, "%s A negative amount?  Try buying something.", sc.Actor.Name)
		return
	}
	if arg == "" || sellnum == 0 {
		keeperTells(sc, keeper, "%s What do you want to sell??", sc.Actor.Name)
		return
	}

	name, _ := oneArgument(arg)
	obj := sellingObject(sc, shop, keeper, name, true)
	if obj == nil {
		return
	}

	purse := gold(keeper) + sc.World.ShopBank(shop)
	if purse < game.SellPrice(shop, obj) {
		keeperTells(sc, keeper, "%s", shopMessage(shop, game.MsgMissingCash1, sc.Actor.Name))
		return
	}

	var goldamt int32
	sold := 0
	for obj != nil && gold(keeper)+sc.World.ShopBank(shop) >= game.SellPrice(shop, obj) && sold < sellnum {
		sold++
		price := game.SellPrice(shop, obj)
		goldamt += price
		addGold(keeper, -price)

		sc.World.ExtractFromChar(obj)
		sc.World.SlideShopObject(shop, keeper, obj)
		obj = sellingObject(sc, shop, keeper, name, false)
	}

	if sold < sellnum {
		switch {
		case obj == nil:
			keeperTells(sc, keeper, "%s You only have %d of those.", sc.Actor.Name, sold)
		case gold(keeper)+sc.World.ShopBank(shop) < game.SellPrice(shop, obj):
			keeperTells(sc, keeper, "%s I can only afford to buy %d of those.", sc.Actor.Name, sold)
		default:
			keeperTells(sc, keeper, "%s Something really screwy made me buy %d.", sc.Actor.Name, sold)
		}
	}
	addGold(sc.Actor, goldamt)

	what := timesMessage(nil, name, sold)
	sc.ToRoom("%s sells %s.\r\n", sc.Actor.Name, what)
	keeperTells(sc, keeper, "%s", shopMessage(shop, game.MsgSell, sc.Actor.Name, goldamt))
	sc.Tell("The shopkeeper now has %s.\r\n", what)

	sc.World.SettleShopBank(shop, keeper)
}

// shoppingValue is shopping_value (shop.c:776): what would you give me for
// this.
func shoppingValue(sc *SpecialCall, shop *game.ShopDef, keeper *game.Character) {
	if !shopIsOK(sc, shop, keeper) {
		return
	}
	if strings.TrimSpace(sc.Arg) == "" {
		keeperTells(sc, keeper, "%s What do you want me to evaluate??", sc.Actor.Name)
		return
	}
	name, _ := oneArgument(sc.Arg)
	obj := sellingObject(sc, shop, keeper, name, true)
	if obj == nil {
		return
	}
	keeperTells(sc, keeper, "%s I'll give you %d gold coins for that!",
		sc.Actor.Name, game.SellPrice(shop, obj))
}

// shoppingList is shopping_list (shop.c:831).
//
// Identical items are grouped, and the number printed against each is the
// number a `buy #n` refers to. A shop that produces an item shows "Unlimited"
// rather than a count.
func shoppingList(sc *SpecialCall, shop *game.ShopDef, keeper *game.Character) {
	if !shopIsOK(sc, shop, keeper) {
		return
	}
	sc.World.SortShopObjects(shop, keeper)

	name, _ := oneArgument(sc.Arg)

	var b strings.Builder
	b.WriteString(" ##   Available   Item                                               Cost\r\n")
	b.WriteString("-------------------------------------------------------------------------\r\n")

	var last *game.Object
	count, index, found := 0, 0, false
	emit := func(obj *game.Object, cnt, idx int) {
		if name != "" && !obj.Matches(name) {
			return
		}
		found = true
		b.WriteString(shopListLine(sc, shop, obj, cnt, idx))
	}

	for _, obj := range keeper.Carrying {
		if obj.Cost <= 0 {
			continue
		}
		switch {
		case last == nil:
			last, count = obj, 1
		case game.SameObject(last, obj):
			count++
		default:
			index++
			emit(last, count, index)
			last, count = obj, 1
		}
	}
	index++

	switch {
	case last == nil:
		sc.Tell("Currently, there is nothing for sale.\r\n")
		return
	case name == "" || last.Matches(name):
		emit(last, count, index)
	case !found:
		sc.Tell("Presently, none of those are for sale.\r\n")
		return
	}
	sc.Tell("%s", b.String())
}

// shopListLine is list_object (shop.c:802).
func shopListLine(sc *SpecialCall, shop *game.ShopDef, obj *game.Object, cnt, index int) string {
	available := fmt.Sprintf("%5d       ", cnt)
	if sc.World.ShopProduces(shop, obj) {
		available = "Unlimited   "
	}

	desc := obj.Name()
	if obj.Type == game.ItemDrinkCon && obj.Values[1] != 0 {
		desc += " of " + game.DrinkName(obj.Values[2])
	}
	if (obj.Type == game.ItemWand || obj.Type == game.ItemStaff) && obj.Values[2] < obj.Values[1] {
		desc += " (partially used)"
	}

	return fmt.Sprintf(" %2d)  %s%s\r\n", index, available,
		capitaliseFirst(fmt.Sprintf("%-48s %6d", desc, game.BuyPrice(shop, obj))))
}

// --- small helpers ----------------------------------------------------

func gold(c *game.Character) int32 {
	if c == nil || c.Record == nil {
		return 0
	}
	return c.Record.Points.Gold
}

func addGold(c *game.Character, n int32) {
	if c == nil || c.Record == nil {
		return
	}
	c.Record.Points.Gold += n
}

func isGod(c *game.Character) bool {
	return c != nil && c.Record != nil && c.Record.Level >= game.LevelGod
}

// carryCount is CAN_CARRY_N (utils.h): 5 + half your dexterity + half your
// level, both halved with `>>` rather than `/`.
func carryCount(c *game.Character) int {
	if c == nil || c.Record == nil {
		return 0
	}
	return int(5 + (c.Record.Abilities.Dexterity >> 1) + (c.Record.Level >> 1))
}

// doNotHere is ACMD(do_not_here) (act.other.c:207).
//
// The whole of the answer for `buy` outside a shop, `mail` outside a post
// office, `rent` outside an inn. Every one of these commands is in the table
// pointing here, and every one of them only works because something in the
// room takes it away first.
func doNotHere(c *Context) error {
	c.Send("Sorry, but you cannot do that here!\r\n")
	return nil
}
