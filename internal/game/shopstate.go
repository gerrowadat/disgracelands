// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import "strings"

// The rules half of shop.c: who a shop will deal with, what it will buy, what
// things cost, and when it is open.
//
// The messages and the commands are in internal/session/shops.go. This is the
// part with arithmetic in it, so this is the part with tests.

// Bank limits (shop.h:134). A keeper holding more than the maximum puts the
// excess in the bank; one holding less than the minimum draws on it.
//
// The asymmetry is the C's and it matters: the *trigger* is 5,000 but the
// *target* is 15,000, so a keeper who drops below 5,000 refills all the way
// to 15,000 rather than back to 5,000.
const (
	MinOutsideBank int32 = 5_000
	MaxOutsideBank int32 = 15_000
)

// What trade_with makes of an object (shop.h:58).
type TradeResult int

const (
	// TradeDead is a wand or staff with no charges left.
	TradeDead TradeResult = iota
	// TradeNotOK is something this shop does not deal in.
	TradeNotOK
	// TradeOK is a sale.
	TradeOK
	// TradeNoValue is something worth nothing.
	TradeNoValue
)

// Trade-with bits: a *set* bit means the shop refuses (shop.h).
const (
	tradeNoGood = 1 << iota
	tradeNoEvil
	tradeNoNeutral
	tradeNoMagicUser
	tradeNoCleric
	tradeNoThief
	tradeNoWarrior
)

// ShopFlag is one of the shop bitvector's bits, and ShopFlags is a shop's
// set of them. Bit indices, not masks: docs/proposals/idiomatic-go.md
// §4.1, and §4.1.1 for the trap. shop_bits[] (shop.c:110) is the name
// table, and the numbers are the file format's.
type ShopFlag int

// ShopFlags is a set of ShopFlag.
type ShopFlags = Set[ShopFlag]

const (
	// ShopWillFight — the keeper fights back, and can be killed.
	ShopWillFight ShopFlag = 0
	// ShopUsesBank — the keeper banks their takings.
	ShopUsesBank ShopFlag = 1
)

// Runtime state, kept per shop. In the C this lives in `shop_index` beside
// the prototype data, because there is exactly one of each shop — a shop is
// not instantiated the way a mobile is.
type shopState struct {
	// Bank is SHOP_BANK: takings above MaxOutsideBank.
	Bank int32
	// Sorted is SHOP_SORT: how many of the keeper's objects are known to be
	// in grouped order. Reset to zero whenever something arrives by a route
	// that is not a sale.
	Sorted int
}

// Shops returns every shop.
func (l *Live) Shops() []*ShopDef {
	if l.defs == nil {
		return nil
	}
	return l.defs.Shops
}

// Shop returns the shop with this vnum, or nil.
func (l *Live) Shop(vnum ShopVnum) *ShopDef {
	for _, shop := range l.Shops() {
		if shop.Vnum == vnum {
			return shop
		}
	}
	return nil
}

// ShopFor returns the shop a mobile keeps, or nil.
func (l *Live) ShopFor(keeper *Character) *ShopDef {
	if keeper == nil || keeper.MobDef == nil {
		return nil
	}
	for _, shop := range l.Shops() {
		if shop.Keeper == keeper.MobDef.Vnum {
			return shop
		}
	}
	return nil
}

// AssignShopkeepers is assign_the_shopkeepers (shop.c:1179): every shop's
// keeper gets the shop_keeper special, and whatever special it had before is
// kept as the shop's secondary function.
//
// Run after AssignSpecials, because it deliberately overwrites what that
// attached.
func (l *Live) AssignShopkeepers() int {
	assigned := 0
	for _, shop := range l.Shops() {
		if shop.Keeper == NoMob {
			continue
		}
		def, ok := l.mobileDefs[shop.Keeper]
		if !ok {
			continue
		}
		if def.Spec != "" && def.Spec != "shop_keeper" {
			shop.Secondary = def.Spec
		}
		def.Spec = "shop_keeper"
		assigned++
	}
	return assigned
}

// ReloadShop refreshes a shop's own configuration — prices, buy types,
// messages, temper, flags, trade-with, rooms, keeper — from a freshly
// loaded definition. New tooling, not a C port, the shop half of the
// reload family ReloadMobile/ReloadObject/ReloadZone started.
//
// Unlike a mobile or an object, a shop is never instantiated: shopState's
// own comment says it plainly — "there is exactly one of each shop" — so
// there is no shared-prototype-versus-live-instance question to answer at
// all. The one thing this does not touch is a shop's actual runtime
// state, its shopState (Bank, Sorted): that is the till, not the
// configuration, the same way ReloadMobile leaves a mobile's Room,
// Carrying and Position alone.
//
// If the keeper vnum changes, the new keeper's prototype needs the
// shop_keeper special the way AssignShopkeepers gives it at boot —
// ShopFor resolves the keeper by matching Keeper against a live mobile's
// vnum on every call, so no *Character needs to move, but nothing else
// would ever set the new keeper's Spec.
//
// Secondary, like a mobile's or an object's Spec, is preserved across the
// copy rather than taken from the fresh definition: no loader ever sets
// it (checked, not assumed — grepping every classic and yaml loader
// turns up nothing), it exists purely as what AssignShopkeepers computed
// at boot, so a freshly loaded ShopDef always has it blank. Only a real
// keeper change re-derives it, the same computation AssignShopkeepers
// itself does.
func (l *Live) ReloadShop(fresh *ShopDef) bool {
	if fresh == nil {
		return false
	}
	existing := l.Shop(fresh.Vnum)
	if existing == nil {
		return false
	}

	oldKeeper, secondary := existing.Keeper, existing.Secondary
	*existing = *fresh
	existing.Secondary = secondary

	if existing.Keeper != oldKeeper && existing.Keeper != NoMob {
		if def, ok := l.mobileDefs[existing.Keeper]; ok {
			// AssignShopkeepers' own condition, exactly: a blank or
			// already-shop_keeper Spec leaves Secondary as it was.
			if def.Spec != "" && def.Spec != "shop_keeper" {
				existing.Secondary = def.Spec
			}
			def.Spec = "shop_keeper"
		}
	}
	return true
}

// shopState returns the mutable state for a shop, creating it on first use.
func (l *Live) shopState(shop *ShopDef) *shopState {
	if l.shops == nil {
		l.shops = map[ShopVnum]*shopState{}
	}
	st, ok := l.shops[shop.Vnum]
	if !ok {
		st = &shopState{}
		l.shops[shop.Vnum] = st
	}
	return st
}

// ShopBank is what the shop has put away.
func (l *Live) ShopBank(shop *ShopDef) int32 { return l.shopState(shop).Bank }

// SettleShopBank moves money between the keeper's pocket and the shop's bank,
// porting the two blocks at the end of shopping_buy and shopping_sell.
//
// Only a shop with USES_BANK deposits; *every* shop withdraws. That is not
// symmetry the C bothered with, and a shop without the flag that somehow has
// a balance will still draw on it.
func (l *Live) SettleShopBank(shop *ShopDef, keeper *Character) {
	if keeper == nil || keeper.Record == nil {
		return
	}
	st := l.shopState(shop)
	gold := &keeper.Record.Points.Gold

	if shop.Flags.Has(ShopUsesBank) && *gold > MaxOutsideBank {
		st.Bank += *gold - MaxOutsideBank
		*gold = MaxOutsideBank
	}
	if *gold < MinOutsideBank {
		drawn := min(MaxOutsideBank-*gold, st.Bank)
		st.Bank -= drawn
		*gold += drawn
	}
}

// ShopIsOpen reports whether the shop is trading at this hour, porting
// is_open (shop.c:150). The second return is the message to say when it is
// not, which is empty when it is.
//
// The comparisons are the C's, and they are not the ones you would write: a
// shop with Open1 9 and Close1 17 is *shut* at 17 and open at 9, because the
// tests are `Open1 > hour` and `Close1 < hour` — so the closing hour is the
// last hour it trades, not the first it does not.
func ShopIsOpen(shop *ShopDef, hour int32) (bool, string) {
	switch {
	case shop.Open1 > hour:
		return false, "Come back later!"
	case shop.Close1 < hour:
		if shop.Open2 > hour {
			return false, "Sorry, we have closed, but come back later."
		}
		if shop.Close2 < hour {
			return false, "Sorry, come back tomorrow."
		}
	}
	return true, ""
}

// ShopServesRoom reports whether a shop trades in this room, porting
// ok_shop_room.
func ShopServesRoom(shop *ShopDef, room RoomVnum) bool {
	for _, r := range shop.Rooms {
		if r == room {
			return true
		}
	}
	return false
}

// ShopWillDealWith reports whether a shop deals with somebody, porting the
// alignment and class half of is_ok_char. The second return is why not.
//
// Gods are served by everybody, and the check is made before the alignment
// one — so an evil implementor shops at a NOTRADE_EVIL store. Mobiles skip
// only the *class* check, because a mobile has no class worth testing but
// does have an alignment.
func ShopWillDealWith(shop *ShopDef, c *Character) (bool, string) {
	if c == nil || c.Record == nil {
		return true, ""
	}
	if c.Record.Level >= LevelGod {
		return true, ""
	}

	align := c.Record.Alignment
	if (align >= 350 && shop.TradeWith&tradeNoGood != 0) ||
		(align <= -350 && shop.TradeWith&tradeNoEvil != 0) ||
		(align > -350 && align < 350 && shop.TradeWith&tradeNoNeutral != 0) {
		return false, "Get out of here before I call the guards!"
	}
	if c.IsNPC() {
		return true, ""
	}

	byClass := map[Class]int32{
		ClassMagicUser: tradeNoMagicUser,
		ClassCleric:    tradeNoCleric,
		ClassThief:     tradeNoThief,
		ClassWarrior:   tradeNoWarrior,
	}
	if bit, ok := byClass[c.Record.Class]; ok && shop.TradeWith&bit != 0 {
		return false, "We don't serve your kind here!"
	}
	return true, ""
}

// tradeLetters is trade_letters[] (shop.c:98), in the same order as the
// tradeNo* bits: alignment first, then class.
var tradeLetters = []string{"Good", "Evil", "Neutral", "Magic User", "Cleric", "Thief", "Warrior"}

// CustomerString is customer_string (shop.c:1198), used by `show shops`:
// a set tradeNo* bit means the shop *refuses* that category, so this lists
// what it will trade with. detailed spells the words out, comma-separated,
// for a single shop's own detail view; the summary form instead prints one
// letter per category, always in the same seven columns — an underscore
// where the shop refuses, so shops line up under `show shops`' own header
// regardless of which categories any one of them serves.
func CustomerString(shop *ShopDef, detailed bool) string {
	var out strings.Builder
	for i, name := range tradeLetters {
		bit := int32(1 << i)
		switch {
		case shop.TradeWith&bit == 0 && detailed:
			if out.Len() > 0 {
				out.WriteString(", ")
			}
			out.WriteString(name)
		case shop.TradeWith&bit == 0:
			out.WriteString(name[:1])
		case !detailed:
			out.WriteString("_")
		}
	}
	return out.String()
}

// TradeWith reports what a shop makes of an object somebody wants to sell,
// porting trade_with (shop.c:291).
func TradeWith(shop *ShopDef, obj *Object) TradeResult {
	if obj.Cost < 1 {
		return TradeNoValue
	}
	if obj.ExtraFlags.Has(ItemNoSell) {
		return TradeNotOK
	}
	for _, want := range shop.BuyTypes {
		if want.Type != obj.Type {
			continue
		}
		// A wand or staff with no charges is refused *whatever* the keyword
		// says, and the loop stops there rather than trying the next entry
		// for the same type.
		if charges, ok := obj.ChargesValues(); ok && charges.Remaining == 0 {
			return TradeDead
		}
		if EvaluateShopExpression(obj, want.Keyword) {
			return TradeOK
		}
	}
	return TradeNotOK
}

// SameObject reports whether two objects are interchangeable for a shop's
// purposes, porting same_obj (shop.c:314).
//
// Prototype, cost, extra flags and every affect slot. Note what is *not*
// compared: the charges left in a wand, the contents of a container, and the
// object's own weight — so two wands with different charges group together in
// a shop listing and sell for the same price.
func SameObject(a, b *Object) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.Def != b.Def {
		return false
	}
	if a.Cost != b.Cost || a.ExtraFlags != b.ExtraFlags {
		return false
	}
	for i := 0; i < MaxObjAffects; i++ {
		var x, y ObjAffect
		if i < len(a.Affects) {
			x = a.Affects[i]
		}
		if i < len(b.Affects) {
			y = b.Affects[i]
		}
		if x != y {
			return false
		}
	}
	return true
}

// ShopProduces reports whether the shop makes this item itself, and so has an
// endless supply, porting shop_producing.
//
// The comparison is against the *prototype*, so an item identical to one the
// shop produces counts as produced even if the player made it somewhere else
// — which is how selling a shop its own goods makes them vanish rather than
// stack up.
func (l *Live) ShopProduces(shop *ShopDef, obj *Object) bool {
	if obj == nil || obj.Def == nil {
		return false
	}
	for _, vnum := range shop.Producing {
		def := l.ObjectDef(vnum)
		if def == nil {
			continue
		}
		if SameObject(obj, prototypeObject(def)) {
			return true
		}
	}
	return false
}

// prototypeObject is `&obj_proto[rnum]`: the prototype seen as an object, for
// comparison only. The C compares against the prototype in place; this makes
// a throwaway with the same fields.
func prototypeObject(def *ObjDef) *Object { return NewObject(0, def) }

// BuyPrice is what the shop charges (shop.c:474).
//
// `return (GET_OBJ_COST(obj) * SHOP_BUYPROFIT(shop_nr));` — an int times a
// float, truncated back to an int. The multiplication is done at *double*
// width on purpose, and it changes the answer: 1.15 as a float32 is exactly
// 1.1499999761581420898437500, so a hundred-coin item costs
//
//   - 115 if the product is rounded to a float first (any SSE build), or
//   - 114 if it is kept wider (the x87, which is what the i386 build the
//     archive was written by used, FLT_EVAL_METHOD 2).
//
// Players paid 114. reference/tools/shopprice.c is the oracle and
// shopprice32_test.go compares against it; docs/weirdnumbers.md has the
// entry. Do not "simplify" this to float32.
func BuyPrice(shop *ShopDef, obj *Object) int32 {
	return int32(float64(obj.Cost) * float64(shop.ProfitBuy)) //nolint:gosec // truncation is the C's arithmetic
}

// SellPrice is what the shop pays (shop.c:632). Same arithmetic as BuyPrice,
// and the same reason for the width.
//
// The C's sell_price takes the character as an argument and never looks at
// them. It is a hook somebody meant to hang haggling on and never did.
func SellPrice(shop *ShopDef, obj *Object) int32 {
	return int32(float64(obj.Cost) * float64(shop.ProfitSell)) //nolint:gosec // truncation is the C's arithmetic
}

// --- the keyword expression evaluator ---------------------------------

// Operator characters, in precedence order (shop.c:87). Each operator has
// several spellings, which is why the buy-word expressions in the shop files
// are written with whatever punctuation was to hand.
var shopOperators = []string{
	"[({", // open paren
	"])}", // close paren
	"|+",  // or
	"&*",  // and
	"^'",  // not
}

const (
	operOpenParen = iota
	operCloseParen
	operOr
	operAnd
	operNot
	operNone = -1
)

// findOperator returns which operator a character is, or operNone.
func findOperator(r byte) int {
	for i, set := range shopOperators {
		if strings.IndexByte(set, r) >= 0 {
			return i
		}
	}
	return operNone
}

// EvaluateShopExpression evaluates a shop's buy-word expression against an
// object, porting evaluate_expression (shop.c:236).
//
// A bare word is true if it is one of the object's keywords, *or* if it names
// an ITEM_ extra flag the object has — the flag names are checked first, so a
// shop that will buy "GLOW" means the flag and not an object called glow.
//
// An empty expression is true, which the C's comment explains as "Allows
// opening ( first" — it is really so that a buy-type line with no keyword
// matches everything of that type, which is most of them.
func EvaluateShopExpression(obj *Object, expr string) bool {
	if expr == "" {
		return true
	}

	var ops []int
	var vals []bool

	// The C's stacks return NOTHING when empty and its top() is compared
	// with `>`, so an empty operator stack loses to every operator.
	topOp := func() int {
		if len(ops) == 0 {
			return operNone
		}
		return ops[len(ops)-1]
	}
	popOp := func() int {
		if len(ops) == 0 {
			return operNone
		}
		v := ops[len(ops)-1]
		ops = ops[:len(ops)-1]
		return v
	}

	i := 0
	for i < len(expr) {
		if expr[i] == ' ' || expr[i] == '\t' {
			i++
			continue
		}
		op := findOperator(expr[i])
		if op == operNone {
			start := i
			for i < len(expr) && expr[i] != ' ' && expr[i] != '\t' && findOperator(expr[i]) == operNone {
				i++
			}
			vals = append(vals, matchesShopWord(obj, expr[start:i]))
			continue
		}

		if op != operOpenParen {
			for topOp() > op {
				evaluateShopOperation(&ops, &vals)
			}
		}
		if op == operCloseParen {
			if popOp() != operOpenParen {
				// "SYSERR: Illegal parenthesis in shop keyword expression."
				return false
			}
		} else {
			ops = append(ops, op)
		}
		i++
	}

	for topOp() != operNone {
		evaluateShopOperation(&ops, &vals)
	}
	if len(vals) == 0 {
		return false
	}
	result := vals[len(vals)-1]
	if len(vals) > 1 {
		// "SYSERR: Extra operands left on shop keyword expression stack."
		return false
	}
	return result
}

// evaluateShopOperation is evaluate_operation (shop.c:206).
func evaluateShopOperation(ops *[]int, vals *[]bool) {
	popOp := func() int {
		if len(*ops) == 0 {
			return operNone
		}
		v := (*ops)[len(*ops)-1]
		*ops = (*ops)[:len(*ops)-1]
		return v
	}
	popVal := func() bool {
		if len(*vals) == 0 {
			return false
		}
		v := (*vals)[len(*vals)-1]
		*vals = (*vals)[:len(*vals)-1]
		return v
	}

	if oper := popOp(); oper == operNot {
		*vals = append(*vals, !popVal())
	} else {
		// Both popped before either is used — the C does this deliberately,
		// because short-circuiting would leave the value stack askew.
		v1, v2 := popVal(), popVal()
		switch oper {
		case operAnd:
			*vals = append(*vals, v1 && v2)
		case operOr:
			*vals = append(*vals, v1 || v2)
		}
	}
}

// matchesShopWord is the leaf of the expression: an extra-flag name, or one
// of the object's keywords.
func matchesShopWord(obj *Object, word string) bool {
	for i, name := range extraBitNames {
		if strings.EqualFold(word, name) {
			// ExtraFlag(i), not 1<<i: the table is indexed by bit
			// position and so is the domain, so the index *is* the flag.
			// This used to shift because the flags were masks, and the
			// shift kept compiling when they stopped being — silently
			// asking about bit 1 for the table's entry 0.
			return obj.ExtraFlags.Has(ExtraFlag(i))
		}
	}
	return obj.Matches(word)
}

// --- the keeper's inventory -------------------------------------------
//
// The C keeps a shopkeeper's carrying list *grouped*: identical items adjacent,
// so `list` can count them and `buy #3` can index them. slide_obj and
// sort_keeper_objs are what maintain that, and the C's own comment on the
// first calls it "a slight hack" that "involves knowing how the list is put
// together". A Go slice makes the same idea ordinary.

// ResetShopSort marks a keeper's inventory as needing a re-sort. The C does
// this whenever objects arrive by a route that is not a sale.
func (l *Live) ResetShopSort(shop *ShopDef) { l.shopState(shop).Sorted = 0 }

// DecrementShopSort records that one sorted object has left the keeper.
func (l *Live) DecrementShopSort(shop *ShopDef) {
	if st := l.shopState(shop); st.Sorted > 0 {
		st.Sorted--
	}
}

// SortShopObjects is sort_keeper_objs (shop.c:678): group everything the
// keeper is carrying that has arrived since the last sort.
//
// A produced item the keeper already has a copy of is *dropped*, not stacked
// — which is why selling a shop three of its own swords leaves it with one.
func (l *Live) SortShopObjects(shop *ShopDef, keeper *Character) {
	st := l.shopState(shop)
	if keeper == nil || st.Sorted >= len(keeper.Carrying) {
		return
	}

	// The C peels the unsorted objects off the *head* one at a time and
	// pushes each onto a second list (shop.c:684-689), which reverses them:
	// it walks them oldest-first below. The head is the newest end here as
	// it is there, so the unsorted set is the leading `n` and reversing it
	// gives the C's own walk order.
	//
	// This read `keeper.Carrying[st.Sorted:]` until #193 — the same set, at
	// the other end, because the list grew the other way round. The
	// arithmetic is the part that had to change with it, not just the
	// insertion.
	n := len(keeper.Carrying) - st.Sorted
	unsorted := make([]*Object, 0, n)
	for i := n - 1; i >= 0; i-- {
		unsorted = append(unsorted, keeper.Carrying[i])
	}
	keeper.Carrying = append([]*Object(nil), keeper.Carrying[n:]...)

	for _, obj := range unsorted {
		if l.ShopProduces(shop, obj) && !hasSameObject(keeper.Carrying, obj) {
			// obj_to_char, which is to say the head (shop.c:695).
			keeper.Carrying = prependObject(keeper.Carrying, obj)
			obj.Location, obj.Holder = CarriedBy, keeper
			st.Sorted++
			continue
		}
		l.SlideShopObject(shop, keeper, obj)
	}
}

// SlideShopObject is slide_obj (shop.c:638): put an object into the keeper's
// inventory next to any identical one already there.
//
// An object identical to one the shop *produces* is destroyed instead. The
// shop can make more whenever it likes, so keeping this one would be so much
// clutter — and it is why a shop's stock of its own goods never grows.
func (l *Live) SlideShopObject(shop *ShopDef, keeper *Character, obj *Object) {
	if keeper == nil || obj == nil {
		return
	}
	if l.ShopProduces(shop, obj) {
		l.ExtractObject(obj)
		return
	}

	st := l.shopState(shop)
	st.Sorted++
	l.detach(obj)
	obj.Location, obj.Holder = CarriedBy, keeper
	l.track(obj)

	for i, held := range keeper.Carrying {
		if SameObject(held, obj) {
			keeper.Carrying = append(keeper.Carrying, nil)
			copy(keeper.Carrying[i+2:], keeper.Carrying[i+1:])
			keeper.Carrying[i+1] = obj
			return
		}
	}
	// Nothing identical: the C leaves the object where obj_to_char put it,
	// by assigning it back over the head it had saved
	// (`keeper->carrying = obj`, shop.c:668). So an unmatched item goes to
	// the *front* of the stock, not the back — which is why a shop lists
	// what it most recently took in first. This appended until #193.
	keeper.Carrying = prependObject(keeper.Carrying, obj)
}

// ExtractFromChar takes an object out of somebody's hands without destroying
// it, porting obj_from_char.
func (l *Live) ExtractFromChar(obj *Object) { l.detach(obj) }

// hasSameObject reports whether the list already holds something identical.
func hasSameObject(list []*Object, obj *Object) bool {
	for _, held := range list {
		if SameObject(held, obj) {
			return true
		}
	}
	return false
}

// --- rent, the rules half ---------------------------------------------
//
// The receptionist's arithmetic. The file writing is in internal/server and
// the conversation is in internal/session; this is what a stay costs and what
// can be stored.

// The rent settings from config.c: FreeRent, MinRentCost and MaxObjSave now
// live in GameTuning (tuning.go), a runtime setting rather than a constant —
// a deliberate, named exception to "the archive wins" (docs/deviations.md).
// FreeRent defaults true (config.c:133), which was the single most
// consequential line in that file for as long as it was a constant:
// **nobody on this server ever paid rent.** The receptionist says "Rent is
// free here.  Just quit, and your objects will be saved!" and stops there.
// Everything Crash_offer_rent computes is dead code at that setting — ported
// anyway, because the path has to be right for an operator who turns it off.
//
// MaxObjSave (config.c:136): Crash_load logs a "hoarding check" against it
// but does not enforce it; only the receptionist refuses.

// Rent factors (objsave.c:22). A cryogenic stay costs four times a day's
// rent, once, instead of a daily charge.
const (
	RentFactor int32 = 1
	CryoFactor int32 = 4
)

// IsUnrentable reports whether an object cannot be stored at all, porting
// Crash_is_unrentable (objsave.c:699).
//
// Four ways to be unstorable, and the last is the surprising one: **every key
// is unrentable**, whatever its flags. Keys are how zones gate themselves, so
// letting one survive a reboot in somebody's pocket would let them keep a
// door open forever.
func IsUnrentable(obj *Object) bool {
	if obj == nil {
		return false
	}
	return obj.ExtraFlags.Has(ItemNoRent) ||
		obj.RentPerDay() < 0 ||
		obj.Def == nil ||
		obj.Type == ItemKey
}

// RentCostOf is Crash_calculate_rent (objsave.c:736): what one object and
// everything inside it costs to store per day.
//
// Negative rents count as zero rather than as a discount, which matters
// because an unrentable object is one with a negative rent and this is also
// called on lists that have not been filtered.
func RentCostOf(obj *Object) int32 {
	if obj == nil {
		return 0
	}
	total := max(0, obj.RentPerDay())
	for _, inside := range obj.Contents {
		total += RentCostOf(inside)
	}
	return total
}
