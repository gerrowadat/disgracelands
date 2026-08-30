// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import "testing"

// The shop keyword expression evaluator, the prices, and the opening hours.

func objectWithKeywords(words string) *Object {
	return &Object{Keywords: words, Cost: 100}
}

func TestShopKeywordExpressions(t *testing.T) {
	sword := objectWithKeywords("sword long steel")
	sword.ExtraFlags = ItemGlow

	for _, tc := range []struct {
		expr string
		want bool
		why  string
	}{
		// An empty expression matches everything of the type, which is what
		// almost every real shop file has.
		{"", true, "an empty expression is true"},

		{"sword", true, "a bare keyword"},
		{"dagger", false, "a keyword the object does not have"},

		// Each operator has several spellings, because the file format never
		// settled on one. All of these are the same expression.
		{"sword&steel", true, "and, spelled &"},
		{"sword*steel", true, "and, spelled *"},
		{"sword&dagger", false, "and, with one side false"},
		{"sword|dagger", true, "or, spelled |"},
		{"sword+dagger", true, "or, spelled +"},
		{"axe|dagger", false, "or, with both sides false"},
		{"^dagger", true, "not, spelled ^"},
		{"'dagger", true, "not, spelled '"},
		{"^sword", false, "not, on something true"},

		// Parentheses, likewise, in three flavours.
		{"(sword|axe)&steel", true, "parenthesised or, spelled ()"},
		{"[sword|axe]&steel", true, "parenthesised or, spelled []"},
		{"{axe|dagger}&steel", false, "parenthesised or that is false"},

		// An extra-flag name is checked before the keyword list, so a shop
		// that buys "GLOW" means the flag.
		{"GLOW", true, "an extra flag the object has"},
		{"HUM", false, "an extra flag it does not have"},
		{"glow", true, "flag names are matched without regard to case"},
	} {
		if got := EvaluateShopExpression(sword, tc.expr); got != tc.want {
			t.Errorf("EvaluateShopExpression(%q) = %v, want %v — %s", tc.expr, got, tc.want, tc.why)
		}
	}
}

// Unbalanced parentheses log a SYSERR in the C and evaluate to false. A shop
// file with a typo in it should refuse to buy, not buy everything.
func TestAMalformedShopExpressionIsFalse(t *testing.T) {
	sword := objectWithKeywords("sword")
	for _, expr := range []string{"sword)", "(sword", ")sword("} {
		if EvaluateShopExpression(sword, expr) {
			t.Errorf("EvaluateShopExpression(%q) is true, want false", expr)
		}
	}
}

// The prices are a float multiplier applied to an int cost and stored back in
// an int, so they truncate — and the width the multiplication happens at
// decides the answer. A 1.15 markup on a hundred-coin item is 114, not 115,
// because 1.15 as a float32 is a hair *under* 1.15 and the product is kept
// wide enough to notice. 0.15 as a float32 is a hair *over*, so the sell
// prices round the other way. See the note on BuyPrice, and shopprice32_test.go
// for the check against a 32-bit build of the C.
func TestShopPricesTruncate(t *testing.T) {
	shop := &ShopDef{ProfitBuy: 1.15, ProfitSell: 0.15}

	for _, tc := range []struct{ cost, buy, sell int32 }{
		{10, 11, 1},
		{100, 114, 15},
		{1000, 1149, 150},
		{1, 1, 0},
		{0, 0, 0},
	} {
		obj := &Object{Cost: tc.cost}
		if got := BuyPrice(shop, obj); got != tc.buy {
			t.Errorf("an item costing %d sells for %d, want %d", tc.cost, got, tc.buy)
		}
		if got := SellPrice(shop, obj); got != tc.sell {
			t.Errorf("an item costing %d is bought for %d, want %d", tc.cost, got, tc.sell)
		}
	}
}

// The hour comparisons are `>` and `<` rather than `>=` and `<=`, so a shop
// open 9 to 17 is still trading *at* 17.
func TestAShopIsOpenOnItsClosingHour(t *testing.T) {
	shop := &ShopDef{Open1: 9, Close1: 17, Open2: 0, Close2: 0}

	for _, tc := range []struct {
		hour int32
		want bool
	}{
		{8, false},
		{9, true},
		{12, true},
		{17, true},
		{18, false},
	} {
		got, _ := ShopIsOpen(shop, tc.hour)
		if got != tc.want {
			t.Errorf("at hour %d the shop is open=%v, want %v", tc.hour, got, tc.want)
		}
	}
}

// Which of the three "we're shut" messages you get depends on which period
// you fall between.
func TestTheClosedMessageSaysWhichKindOfClosed(t *testing.T) {
	shop := &ShopDef{Open1: 9, Close1: 12, Open2: 14, Close2: 17}

	for _, tc := range []struct {
		hour int32
		want string
	}{
		{8, "Come back later!"},
		{13, "Sorry, we have closed, but come back later."},
		{18, "Sorry, come back tomorrow."},
	} {
		open, message := ShopIsOpen(shop, tc.hour)
		if open {
			t.Errorf("at hour %d the shop is open, want shut", tc.hour)
			continue
		}
		if message != tc.want {
			t.Errorf("at hour %d the shop says %q, want %q", tc.hour, message, tc.want)
		}
	}
}

func TestTradeWith(t *testing.T) {
	shop := &ShopDef{BuyTypes: []ShopBuyType{{Type: ItemWeapon}, {Type: ItemWand}}}

	weapon := &Object{Type: ItemWeapon, Cost: 100}
	if got := TradeWith(shop, weapon); got != TradeOK {
		t.Errorf("a weapon this shop buys got %v, want TradeOK", got)
	}

	armour := &Object{Type: ItemArmor, Cost: 100}
	if got := TradeWith(shop, armour); got != TradeNotOK {
		t.Errorf("armour this shop does not buy got %v, want TradeNotOK", got)
	}

	// Worthlessness is checked before everything, including the buy list.
	worthless := &Object{Type: ItemWeapon, Cost: 0}
	if got := TradeWith(shop, worthless); got != TradeNoValue {
		t.Errorf("a worthless weapon got %v, want TradeNoValue", got)
	}

	noSell := &Object{Type: ItemWeapon, Cost: 100, ExtraFlags: ItemNoSell}
	if got := TradeWith(shop, noSell); got != TradeNotOK {
		t.Errorf("a NOSELL weapon got %v, want TradeNotOK", got)
	}

	// A spent wand is refused with its own message even though the shop deals
	// in wands.
	spent := &Object{Type: ItemWand, Cost: 100}
	spent.Values[2] = 0
	if got := TradeWith(shop, spent); got != TradeDead {
		t.Errorf("a spent wand got %v, want TradeDead", got)
	}
	charged := &Object{Type: ItemWand, Cost: 100}
	charged.Values[2] = 3
	if got := TradeWith(shop, charged); got != TradeOK {
		t.Errorf("a charged wand got %v, want TradeOK", got)
	}
}

// same_obj compares the prototype, the cost, the extra flags and the affects
// — and nothing else. Two wands with different charges are the same object as
// far as a shop is concerned, which is why they group in a listing.
func TestSameObjectIgnoresCharges(t *testing.T) {
	def := &ObjDef{Vnum: 3000, Type: ItemWand, Cost: 100}
	a, b := NewObject(1, def), NewObject(2, def)
	a.Values[2], b.Values[2] = 1, 9

	if !SameObject(a, b) {
		t.Error("two wands of the same kind with different charges are not the same object")
	}

	b.Cost = 101
	if SameObject(a, b) {
		t.Error("two objects with different costs are the same object")
	}
	b.Cost = 100
	b.Affects = []ObjAffect{{Location: 1, Modifier: 2}}
	if SameObject(a, b) {
		t.Error("two objects with different affects are the same object")
	}
}

// The bank asymmetry: the keeper tops up when they drop below 5,000, and they
// top up to 15,000 rather than to 5,000.
func TestAKeeperRefillsAllTheWayFromTheBank(t *testing.T) {
	l := &Live{}
	shop := &ShopDef{Vnum: 1, Flags: NewSet(ShopUsesBank)}
	keeper := &Character{Record: &PlayerRecord{}}

	// Over the maximum: the excess goes in.
	keeper.Record.Points.Gold = 20_000
	l.SettleShopBank(shop, keeper)
	if keeper.Record.Points.Gold != MaxOutsideBank {
		t.Errorf("a keeper with 20,000 kept %d, want %d", keeper.Record.Points.Gold, MaxOutsideBank)
	}
	if got := l.ShopBank(shop); got != 5_000 {
		t.Errorf("the bank holds %d, want 5,000", got)
	}

	// Under the minimum: they draw right back up to the *maximum*, not the
	// minimum — so 5,000 in the bank all comes out at once.
	keeper.Record.Points.Gold = 4_000
	l.SettleShopBank(shop, keeper)
	if keeper.Record.Points.Gold != 9_000 {
		t.Errorf("a keeper with 4,000 and 5,000 banked ended with %d, want 9,000",
			keeper.Record.Points.Gold)
	}
	if got := l.ShopBank(shop); got != 0 {
		t.Errorf("the bank holds %d after the refill, want 0", got)
	}
}

// A shop without USES_BANK never deposits, but still withdraws — the C only
// guards the deposit.
func TestAShopWithoutTheBankFlagStillWithdraws(t *testing.T) {
	l := &Live{}
	shop := &ShopDef{Vnum: 2}
	keeper := &Character{Record: &PlayerRecord{}}

	l.shopState(shop).Bank = 1_000
	keeper.Record.Points.Gold = 100_000
	l.SettleShopBank(shop, keeper)
	if keeper.Record.Points.Gold != 100_000 {
		t.Errorf("a shop without USES_BANK deposited: keeper has %d", keeper.Record.Points.Gold)
	}

	keeper.Record.Points.Gold = 100
	l.SettleShopBank(shop, keeper)
	if keeper.Record.Points.Gold != 1_100 {
		t.Errorf("a shop without USES_BANK did not withdraw: keeper has %d", keeper.Record.Points.Gold)
	}
}
