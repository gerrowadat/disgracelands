package classic

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// Shop files are the odd one out. Every other record type is found by a
// "#vnum" line read with get_line; shops read *every* structural element with
// fread_string, so the record header is "#3000~" — tilde-terminated — and the
// file's first line is a version banner rather than a record.
//
// The format also has two variants. A file containing the string "v3.0"
// anywhere before its first record switches the loader into a mode where the
// produce, trade and room lists are terminated by -1; without it the lists
// are fixed-length (5, 5 and 1). Every shop file in data/world carries the
// banner, but the old form still has to work, because the flag is set by
// content rather than by filename.

const (
	// version3Tag matches the C VERSION3_TAG.
	version3Tag = "v3.0"

	// Fixed list lengths used when the version banner is absent, matching
	// MAX_PROD, MAX_TRADE and the literal 1 in boot_the_shops().
	maxProduceOld = 5
	maxTradeOld   = 5
	maxRoomsOld   = 1

	// maxShopObj is the C MAX_SHOP_OBJ: entries past it are dropped with a
	// complaint rather than growing the list.
	maxShopObj = 100
)

// parseShopFile reads a whole shop file, appending every shop it contains.
func (l *loader) parseShopFile(w *game.World, r *reader, path string) error {
	newFormat := false

	for {
		// The C loader reads the dispatch line with fread_string, so a record
		// header, the version banner and the '$' terminator are all
		// tilde-terminated strings rather than plain lines.
		head, err := r.readString("a shop record header")
		if err != nil {
			// Running out of input here is how a file that ends without '$'
			// presents. The C loader loops forever on it.
			return fmt.Errorf("%s: ended without a '$' terminator", path)
		}

		switch {
		case strings.HasPrefix(head, "#"):
			vnum, ok := scanInt(head[1:])
			if !ok {
				return fmt.Errorf("%s: shop header %q has no number", r.where(""), head)
			}
			shop, err := l.parseShop(r, game.ShopVnum(vnum), newFormat)
			if err != nil {
				return err
			}
			w.Shops = append(w.Shops, shop)

		case strings.HasPrefix(head, "$"):
			return nil

		default:
			// Anything else is the version banner, or a comment the loader
			// ignores. Testing for the tag by substring is what the C code
			// does, so a shop message mentioning "v3.0" before the first
			// record would switch formats — a real trap, but not one this
			// world falls into.
			if strings.Contains(head, version3Tag) {
				newFormat = true
			}
		}
	}
}

// parseShop reads one shop record, positioned just after its header.
func (l *loader) parseShop(r *reader, vnum game.ShopVnum, newFormat bool) (*game.ShopDef, error) {
	what := fmt.Sprintf("shop #%d", vnum)
	shop := &game.ShopDef{Vnum: vnum, Keeper: game.NoMob}

	produce, err := l.readVnumList(r, what, "produce list", newFormat, maxProduceOld)
	if err != nil {
		return nil, err
	}
	for _, v := range produce {
		shop.Producing = append(shop.Producing, game.ObjVnum(v))
	}

	if shop.ProfitBuy, err = l.readFloat(r, what, "buy profit"); err != nil {
		return nil, err
	}
	if shop.ProfitSell, err = l.readFloat(r, what, "sell profit"); err != nil {
		return nil, err
	}

	if shop.BuyTypes, err = l.readTradeList(r, what, newFormat); err != nil {
		return nil, err
	}

	for i := 0; i < game.NumShopMessages; i++ {
		msg, err := r.readString(fmt.Sprintf("%s message %d", what, i))
		if err != nil {
			return nil, err
		}
		shop.Messages[i] = l.validateShopMessage(r, what, i, msg)
	}

	if shop.Temper, err = l.readInt(r, what, "broke temper"); err != nil {
		return nil, err
	}

	flags, err := l.readInt(r, what, "shop flags")
	if err != nil {
		return nil, err
	}
	// The C field is bitvector_t, which is unsigned; a negative value in the
	// file therefore becomes a large positive bitmask rather than an error.
	// Reproduce that rather than sign-extending it into the high 32 bits.
	shop.Flags = game.Flags(uint32(flags)) //nolint:gosec // deliberate reinterpretation, see above

	keeper, err := l.readInt(r, what, "shopkeeper vnum")
	if err != nil {
		return nil, err
	}
	// The C loader reads this with "%hd" into an `int` field (shop.h's
	// mob_rnum keeper), so sscanf writes only the low two bytes and leaves
	// the rest whatever the allocation held — zero, as it happens, which is
	// why nobody has noticed. A keeper vnum above 32767 would be silently
	// truncated there. Read it at full width here and report the ones that
	// would differ.
	if keeper > 32767 || keeper < -32768 {
		l.warnf("%s: shopkeeper vnum %d does not fit in the 16 bits the C loader reads it with; the two servers will disagree",
			r.where(what), keeper)
	}
	shop.Keeper = game.MobVnum(keeper)

	if shop.TradeWith, err = l.readInt(r, what, "trade-with flags"); err != nil {
		return nil, err
	}

	rooms, err := l.readVnumList(r, what, "room list", newFormat, maxRoomsOld)
	if err != nil {
		return nil, err
	}
	for _, v := range rooms {
		shop.Rooms = append(shop.Rooms, game.RoomVnum(v))
	}

	for _, field := range []struct {
		name string
		dst  *int32
	}{
		{"first opening hour", &shop.Open1},
		{"first closing hour", &shop.Close1},
		{"second opening hour", &shop.Open2},
		{"second closing hour", &shop.Close2},
	} {
		if *field.dst, err = l.readInt(r, what, field.name); err != nil {
			return nil, err
		}
	}

	return shop, nil
}

// readInt reads one structural line and parses a number from it, mirroring
// the C read_line().
func (l *loader) readInt(r *reader, what, field string) (int32, error) {
	line, ok := r.getLine()
	if !ok {
		return 0, fmt.Errorf("%s: file ended before the %s", r.where(what), field)
	}
	v, ok := scanInt(line)
	if !ok {
		return 0, fmt.Errorf("%s: %s %q is not a number", r.where(what), field, line)
	}
	return v, nil
}

// readFloat reads a profit multiplier. The C struct stores these as float,
// not double, and the difference shows up in prices.
func (l *loader) readFloat(r *reader, what, field string) (float32, error) {
	line, ok := r.getLine()
	if !ok {
		return 0, fmt.Errorf("%s: file ended before the %s", r.where(what), field)
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(strings.Fields(line + " ")[0]), 32)
	if err != nil {
		return 0, fmt.Errorf("%s: %s %q is not a number", r.where(what), field, line)
	}
	return float32(v), nil
}

// readVnumList reads a produce or room list: -1 terminated in the v3.0
// format, fixed-length otherwise.
func (l *loader) readVnumList(r *reader, what, field string, newFormat bool, fixedLen int) ([]int32, error) {
	var out []int32

	add := func(v int32) {
		// A -1 entry is the "empty slot" filler in the old fixed-length
		// format and is dropped rather than stored.
		if v == -1 {
			return
		}
		if len(out) >= maxShopObj {
			l.warnf("%s: %s has more than %d entries; the rest are dropped, as in the C loader",
				r.where(what), field, maxShopObj)
			return
		}
		out = append(out, v)
	}

	if newFormat {
		for {
			v, err := l.readInt(r, what, field)
			if err != nil {
				return nil, err
			}
			if v < 0 {
				return out, nil
			}
			add(v)
		}
	}

	for i := 0; i < fixedLen; i++ {
		v, err := l.readInt(r, what, field)
		if err != nil {
			return nil, err
		}
		add(v)
	}
	return out, nil
}

// readTradeList reads the item types a shop will buy.
//
// This is the one part of the format read with a bare fgets rather than
// get_line, so blank lines and '*' comments are *not* skipped inside it, and
// a ';' introduces a trailing comment. Entries may be an item-type name or a
// number, optionally followed by a keyword.
func (l *loader) readTradeList(r *reader, what string, newFormat bool) ([]game.ShopBuyType, error) {
	if !newFormat {
		vals, err := l.readVnumList(r, what, "trade list", false, maxTradeOld)
		if err != nil {
			return nil, err
		}
		out := make([]game.ShopBuyType, 0, len(vals))
		for _, v := range vals {
			out = append(out, game.ShopBuyType{Type: v})
		}
		return out, nil
	}

	var out []game.ShopBuyType
	for {
		line, ok := r.rawLine()
		if !ok {
			return nil, fmt.Errorf("%s: file ended inside the trade list", r.where(what))
		}
		if i := strings.IndexByte(line, ';'); i >= 0 {
			line = line[:i]
		}

		rest := strings.TrimSpace(line)
		typ, ok := scanInt(rest)
		if !ok {
			// An item-type *name* rather than a number. The C loader matches
			// these against item_types[]; nothing in data/world uses the form,
			// so recognising it is deferred to the phase that owns the item
			// type table rather than guessed at here.
			l.warnf("%s: trade list entry %q is a type name rather than a number, which is not supported yet",
				r.where(what), rest)
			continue
		}
		if typ < 0 {
			return out, nil
		}

		// Anything after the number is a keyword restricting the entry.
		keyword := strings.TrimSpace(strings.TrimLeft(rest, "+-0123456789"))
		if len(out) >= maxShopObj {
			l.warnf("%s: trade list has more than %d entries; the rest are dropped", r.where(what), maxShopObj)
			continue
		}
		out = append(out, game.ShopBuyType{Type: typ, Keyword: keyword})
	}
}

// validateShopMessage reproduces read_shop_message()'s format checking. A
// message that fails is stored as NULL by the C loader, which is why the
// keeper says nothing at all in that situation rather than saying something
// wrong.
func (l *loader) validateShopMessage(r *reader, what string, index int, msg string) string {
	var percentS, percentD, errs int

	for i := 0; i < len(msg); i++ {
		if msg[i] != '%' || i+1 >= len(msg) {
			continue
		}
		switch next := msg[i+1]; {
		case next == 's':
			percentS++
		case next == 'd' && (index == game.MsgBuy || index == game.MsgSell):
			if percentS == 0 {
				l.warnf("%s: message %d has %%d before %%s", r.where(what), index)
				errs++
			}
			percentD++
		case next == '%':
			// An escaped literal percent; skip its second character.
			i++
		default:
			l.warnf("%s: message %d has an invalid format specifier %%%c", r.where(what), index, next)
			errs++
		}
	}

	if percentS > 1 || percentD > 1 {
		l.warnf("%s: message %d has too many format specifiers (%%s=%d, %%d=%d)",
			r.where(what), index, percentS, percentD)
		errs++
	}
	if errs > 0 {
		// Matches the C loader, which frees the string and stores NULL.
		return ""
	}
	return msg
}

// resolveShopReferences reports shop entries pointing at things that do not
// exist, and drops the produce entries the C loader drops.
//
// real_object() is applied to produce entries as they are read, and an
// unresolvable one is discarded silently. Doing that here rather than in the
// parser means the object list is fully loaded first, which it is not while
// the shop files are being read.
func (l *loader) resolveShopReferences(w *game.World) {
	objs := make(map[game.ObjVnum]bool, len(w.Objects))
	for _, o := range w.Objects {
		objs[o.Vnum] = true
	}
	mobs := make(map[game.MobVnum]bool, len(w.Mobiles))
	for _, m := range w.Mobiles {
		mobs[m.Vnum] = true
	}
	rooms := make(map[game.RoomVnum]bool, len(w.Rooms))
	for _, rm := range w.Rooms {
		rooms[rm.Vnum] = true
	}

	for _, shop := range w.Shops {
		kept := shop.Producing[:0]
		for _, v := range shop.Producing {
			if !objs[v] {
				l.warnf("shop #%d produces object #%d, which does not exist; the C loader drops it silently", shop.Vnum, v)
				continue
			}
			kept = append(kept, v)
		}
		shop.Producing = kept

		if shop.Keeper != game.NoMob && !mobs[shop.Keeper] {
			l.warnf("shop #%d is kept by mob #%d, which does not exist; the shop will have no keeper", shop.Vnum, shop.Keeper)
			shop.Keeper = game.NoMob
		}
		for _, rv := range shop.Rooms {
			if !rooms[rv] {
				l.warnf("shop #%d operates in room #%d, which does not exist", shop.Vnum, rv)
			}
		}
	}
}
