// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

// ShopVnum identifies a shop.
type ShopVnum int32

// NumShopMessages is the count of canned messages every shop carries, in the
// order boot_the_shops() reads them.
const NumShopMessages = 7

// Shop message indices, in file order.
const (
	MsgNoSuchItem1  = iota // keeper does not stock it
	MsgNoSuchItem2         // player does not have it
	MsgDoNotBuy            // keeper does not buy that kind of thing
	MsgMissingCash1        // keeper cannot afford it
	MsgMissingCash2        // player cannot afford it
	MsgBuy                 // successful purchase
	MsgSell                // successful sale
)

// ShopDef is a shop prototype.
type ShopDef struct {
	Vnum ShopVnum

	// Producing lists objects the shop creates on demand, so it never runs
	// out. The C loader resolves these to real numbers as it reads them and
	// silently drops any that name an object that does not exist; this keeps
	// vnums, and the loader records a finding for the dropped ones.
	Producing []ObjVnum

	// ProfitBuy multiplies an item's cost when the shop sells to a player;
	// ProfitSell when it buys from one. Stored as float32 because the C
	// struct does, and the difference is visible in prices.
	ProfitBuy  float32
	ProfitSell float32

	// BuyTypes are the item types the shop will buy, each optionally with a
	// keyword restricting it further.
	BuyTypes []ShopBuyType

	// Messages are the seven canned lines, indexed by the Msg* constants. An
	// empty string means the file's message failed validation and the C
	// loader stored NULL.
	Messages [NumShopMessages]string

	// Temper is how the keeper reacts to a player who cannot pay.
	Temper int32
	// Flags is the shop bitvector: WILL_FIGHT, USES_BANK.
	Flags ShopFlags

	// Keeper is the shopkeeper's mob vnum, or -1 if the file named a mob
	// that does not exist.
	Keeper MobVnum

	// Secondary is the special the keeper's prototype had *before*
	// assign_the_shopkeepers overwrote it with shop_keeper (shop.c:1179).
	// The C saves it in SHOP_FUNC and shop_keeper calls it first, which is
	// how a mobile can be both a shopkeeper and something else.
	Secondary string

	// TradeWith is a bitvector of who the shop refuses to deal with
	// (TRADE_NOGOOD, TRADE_NOEVIL, ...). Note the inversion: a set bit means
	// "will not trade with".
	TradeWith int32

	// Rooms are the rooms the shop operates in.
	Rooms []RoomVnum

	// Open1/Close1 and Open2/Close2 are two opening periods, in MUD hours.
	Open1, Close1 int32
	Open2, Close2 int32
}

// ShopBuyType is one entry in a shop's buy list: an item type, and optionally
// a keyword that further restricts what it will take.
type ShopBuyType struct {
	Type    ItemType
	Keyword string
}

// NoMob marks a shopkeeper the world does not define, matching the C NOBODY.
const NoMob MobVnum = -1
