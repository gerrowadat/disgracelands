// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package yaml

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/goccy/go-yaml"

	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/persist/yamlenc"
)

// WriteZone implements world.Sink: it writes one zone and everything whose
// vnum falls inside it to its own file, canonically (§10.3) and atomically
// (§3's "one writer per file").
func (s *Source) WriteZone(_ context.Context, zone *game.ZoneDef, w *game.World) error {
	doc := zoneDocFrom(zone, w)
	out, err := yaml.MarshalWithOptions(doc, yamlenc.Options()...)
	if err != nil {
		return fmt.Errorf("yaml: zone %d: %w", zone.Vnum, err)
	}
	path := filepath.Join(s.dir, zoneFileName(int32(zone.Vnum), zone.Name))
	return atomicWrite(path, out)
}

func zoneDocFrom(zone *game.ZoneDef, w *game.World) zoneDoc {
	doc := zoneDoc{
		Schema: ZoneSchema,
		Zone: zoneMeta{
			Vnum:     int32(zone.Vnum),
			Name:     zone.Name,
			Range:    [2]int32{int32(zone.Bottom), int32(zone.Top)},
			Lifespan: zone.Lifespan,
			Reset:    resetModeName(zone.ResetMode),
		},
	}
	for _, r := range w.Rooms {
		if r.Zone == zone.Vnum {
			doc.Rooms = append(doc.Rooms, roomDocFrom(r))
		}
	}
	for _, m := range w.Mobiles {
		if writtenUnder(w.Zones, zone, int32(m.Vnum)) {
			doc.Mobiles = append(doc.Mobiles, mobDocFrom(m))
		}
	}
	for _, o := range w.Objects {
		if writtenUnder(w.Zones, zone, int32(o.Vnum)) {
			doc.Objects = append(doc.Objects, objDocFrom(o))
		}
	}
	for _, sh := range w.Shops {
		if writtenUnder(w.Zones, zone, shopHomeVnum(w.Zones, sh)) {
			doc.Shops = append(doc.Shops, shopDocFrom(sh))
		}
	}
	doc.Resets = resetsToDoc(NestResets(zone.Commands))
	return doc
}

// writtenUnder reports whether a mobile, object or shop with this vnum
// belongs in zone's file.
//
// A room carries its own zone (db.c's boot_world stamps every room with the
// zone it was read under), so rooms need none of this. A mobile, object or
// shop carries nothing but its vnum, and the C never needs it to: db.c
// reads every file named in an index into one flat table keyed by vnum, and
// zone_table's bottom/top only ever decide which *resets* run and which
// builder may edit what. Which file a record physically sat in has no
// meaning to the loader at all.
//
// A per-zone format has to invent that meaning, and the range is the
// obvious way to. It is also not always true: a builder can put a record in
// a zone's file with a vnum outside that zone's declared range, nothing in
// the C stops them or notices, and it happens. Matching on range alone
// dropped those records — silently, because the writer works zone by zone
// and no zone ever claimed them, so nothing was there to report a loss.
//
// So an unclaimed vnum falls to the zone whose range *starts* nearest below
// it, and to the lowest zone if it is below every range. That is not a
// recovery of the record's original file — nothing in memory remembers it —
// but it lands the common case in exactly the file it came from, since a
// vnum that overshoots its zone's top usually overshoots by less than the
// gap to the next zone. What matters for fidelity is only that it lands
// somewhere: the reader rebuilds the same flat table the C does, and a
// record's file is not part of what the game sees.
func writtenUnder(zones []*game.ZoneDef, zone *game.ZoneDef, vnum int32) bool {
	if vnum >= int32(zone.Bottom) && vnum <= int32(zone.Top) {
		return true
	}
	if claimingZone(zones, vnum) != nil {
		return false // claimed by a zone, and not this one
	}
	return zone == fallbackZone(zones, vnum)
}

// shopHomeVnum picks the vnum that decides which file a shop is written
// under: its keeper's, when some zone's range claims that, and its own
// otherwise.
//
// A shop is the one record type whose vnum routinely has nothing to do with
// the zone it belongs to. Nothing derives a shop's number from anything —
// the C reads it only to print it and to answer OLC's `real_shop` — so a
// hand-written .shp file is free to number its shops from 1, and the
// archived Disgracelands lib/ has exactly that: 20.shp holds shops #1-#5,
// keepers #2021, #2020, #2015, #2014 and #2019, and 190.shp holds #190-#194
// with keepers in the #19000s. (OasisOLC's own save_shops, genshp.c:317,
// does write by shop vnum — it walks the zone's range and asks real_shop
// for each number — so a zone whose shop file it has ever rewritten *is*
// vnum-aligned. Both shapes are in the wild; only one of them is
// self-describing.)
//
// Writing those by shop vnum scattered them: #1-#5 to zone 0's file because
// 0-99 happens to claim them, #190-#194 to the fallback because no zone's
// range covers 100-199. The records all survived — the reader rebuilds one
// flat table either way — but the *order* did not, and shop order is not
// inert. It is `shop_index`'s order in the C, which decides which shop
// `shop_keeper` (shop.c) finds first when one mobile keeps two, and which
// row `show shops <n>` numbers as n. Zone files load in vnum order, so a
// shop written under the wrong zone comes back in the wrong place, and
// `dlctl import --verify` refused the whole archive over it.
//
// The keeper is the shop, in every way the game can see: the special is
// attached to the keeper mobile, and the shop does nothing anywhere the
// keeper is not. It is also, unlike the shop's own number, a vnum somebody
// had to allocate out of a zone's range — which is why it identifies the
// file the shop came from in all 77 of the archive's shops and all 46 of
// stock CircleMUD's, where it agrees with the shop vnum anyway.
func shopHomeVnum(zones []*game.ZoneDef, sh *game.ShopDef) int32 {
	if sh.Keeper != game.NoMob && claimingZone(zones, int32(sh.Keeper)) != nil {
		return int32(sh.Keeper)
	}
	return int32(sh.Vnum)
}

// claimingZone returns the zone whose declared range covers vnum, or nil if
// none does.
func claimingZone(zones []*game.ZoneDef, vnum int32) *game.ZoneDef {
	for _, z := range zones {
		if vnum >= int32(z.Bottom) && vnum <= int32(z.Top) {
			return z
		}
	}
	return nil
}

// fallbackZone picks the zone an unclaimed vnum is written under: the one
// whose range begins nearest below it, or the lowest-numbered zone if it
// begins below them all. Ties go to the earlier zone in the slice, which is
// index order, so the choice does not depend on map iteration.
func fallbackZone(zones []*game.ZoneDef, vnum int32) *game.ZoneDef {
	var best, lowest *game.ZoneDef
	for _, z := range zones {
		if lowest == nil || z.Bottom < lowest.Bottom {
			lowest = z
		}
		if int32(z.Bottom) <= vnum && (best == nil || z.Bottom > best.Bottom) {
			best = z
		}
	}
	if best != nil {
		return best
	}
	return lowest
}

func resetModeName(mode int32) string {
	switch mode {
	case 0:
		return "never"
	case 1:
		return "empty"
	default:
		return "always"
	}
}

func roomDocFrom(r *game.RoomDef) roomDoc {
	names, raw := game.NameBits(r.Flags.Raw(), game.YamlRoomFlagNames())
	rd := roomDoc{
		Vnum:     int32(r.Vnum),
		Name:     Text(r.Name),
		Sector:   sectorName(r.SectorType),
		Flags:    names,
		FlagsRaw: raw,
		Desc:     Text(ToStored(r.Description)),
	}

	var exits exitSetDoc
	any := false
	slots := exits.slots()
	for dir := game.Direction(0); int(dir) < game.NumDirections; dir++ {
		e := r.Exits[dir]
		if e == nil {
			continue
		}
		any = true
		ed := &exitDoc{
			To:       int32(e.ToRoom),
			Desc:     NestedText(ToStored(e.Description)),
			Keywords: e.Keywords,
			Key:      int32(e.Key),
			Door:     doorName(e.DoorFlag),
		}
		*slots[dir] = ed
	}
	if any {
		rd.Exits = &exits
	}

	for _, ed := range r.ExtraDescs {
		rd.ExtraDescs = append(rd.ExtraDescs, extraDescDoc{
			Keywords: strings.Fields(ed.Keywords),
			Desc:     NestedText(ToStored(ed.Description)),
		})
	}
	return rd
}

func sectorName(v int32) string { return game.NameOrNumber(v, game.YamlSectorNames()) }

// doorName has no numeric fallback and does not need one: the C's door
// flag is a two-bit field and YamlExitDoorNames covers all four values,
// so there is nothing outside the table for one to catch.
func doorName(flag int32) string {
	name, _ := game.NameByValue(flag, game.YamlExitDoorNames())
	return name
}

func mobDocFrom(m *game.MobDef) mobDoc {
	actFlags := m.ActionFlags.Clear(game.MobIsNPC) // never written; the reader force-sets it
	actNames, actRaw := game.NameBits(uint64(actFlags), game.YamlMobActFlagNames())
	affNames, affRaw := game.NameBits(uint64(m.AffectionFlags), game.YamlAffectFlagNames())

	md := mobDoc{
		Vnum:            int32(m.Vnum),
		Keywords:        strings.Fields(m.Keywords),
		Short:           Text(m.ShortDesc),
		Long:            Text(ToStored(m.LongDesc)),
		Desc:            Text(ToStored(m.Description)),
		Act:             actNames,
		ActRaw:          actRaw,
		Affected:        affNames,
		AffectedRaw:     affRaw,
		Alignment:       m.Alignment,
		Level:           m.Level,
		Thac0:           m.Thac0,
		AC:              m.ArmorClass,
		HP:              diceDocString(m.HitDice),
		Damage:          diceDocString(m.DamageDice),
		Gold:            m.Gold,
		Exp:             m.Exp,
		Position:        valueName(m.Position, game.YamlPositionNames()),
		DefaultPosition: valueName(m.DefaultPosition, game.YamlPositionNames()),
		Sex:             valueName(m.Sex, game.YamlSexNames()),
	}
	if m.Enhanced {
		abilities, _ := AbilitiesFromEspecs(m.Especs)
		if !abilities.IsZero() {
			md.Abilities = &abilities
		}
	}
	return md
}

// valueName names a value from a symbolic table, falling back to game's
// own "#N" convention for one the table has no name for.
//
// It used to write "unknown-%d", which is worse than useless: the reader
// looks the name up in the same table, does not find "unknown-104"
// either, and reports a load error — so a world with one out-of-range
// enum in it converted successfully into a directory that would not load.
// "#104" reads back as 104. See game.NameOrNumber.
func valueName(v int32, table []string) string {
	return game.NameOrNumber(v, table)
}

func diceDocString(d game.Dice) string {
	if d.Bonus != 0 {
		return fmt.Sprintf("%dd%d+%d", d.Number, d.Size, d.Bonus)
	}
	return fmt.Sprintf("%dd%d", d.Number, d.Size)
}

func objDocFrom(o *game.ObjDef) objDoc {
	wearNames, wearRaw := game.NameBits(uint64(o.WearFlags), game.YamlWearFlagNames())
	flagNames, flagRaw := game.NameBits(uint64(o.ExtraFlags), game.YamlItemExtraFlagNames())
	permNames, permRaw := game.NameBits(uint64(o.PermAffect), game.YamlAffectFlagNames()) //nolint:gosec // affect bits fit comfortably

	od := objDoc{
		Vnum:          int32(o.Vnum),
		Keywords:      strings.Fields(o.Keywords),
		Short:         Text(o.ShortDesc),
		Desc:          Text(ToStored(o.Description)),
		ActionDesc:    Text(ToStored(o.ActionDesc)),
		Type:          valueName(o.Type, game.YamlItemTypeNames()),
		Wear:          wearNames,
		WearRaw:       wearRaw,
		Flags:         flagNames,
		FlagsRaw:      flagRaw,
		PermAffect:    permNames,
		PermAffectRaw: permRaw,
		Weight:        o.Weight,
		Cost:          o.Cost,
		Rent:          o.RentPerDay,
		MinLevel:      o.MinLevel,
	}

	typed, unusedNonzero, ok := TypedValues(o.Type, o.Values)
	if ok && !unusedNonzero {
		switch v := typed.(type) {
		case WeaponValues:
			od.Weapon = &v
		case ArmorValues:
			od.Armor = &v
		case ContainerValues:
			od.Container = &v
		case DrinkValues:
			od.Drink = &v
		case LightValues:
			od.Light = &v
		case ChargesValues:
			od.Charges = &v
		}
	} else {
		values := RawValues(o.Values)
		od.Values = &values
	}

	for _, a := range o.Affects {
		od.Affects = append(od.Affects, objAffectDoc{
			Location: valueName(a.Location, game.YamlApplyTypeNames()),
			Modifier: a.Modifier,
		})
	}
	for _, ed := range o.ExtraDescs {
		od.ExtraDescs = append(od.ExtraDescs, extraDescDoc{
			Keywords: strings.Fields(ed.Keywords),
			Desc:     NestedText(ToStored(ed.Description)),
		})
	}
	return od
}

func shopDocFrom(sh *game.ShopDef) shopDoc {
	sd := shopDoc{
		Vnum:     int32(sh.Vnum),
		Keeper:   int32(sh.Keeper),
		Markup:   sh.ProfitBuy,
		Markdown: sh.ProfitSell,
		Temper:   sh.Temper,
	}
	for _, r := range sh.Rooms {
		sd.Rooms = append(sd.Rooms, int32(r))
	}
	for _, p := range sh.Producing {
		sd.Sells = append(sd.Sells, int32(p))
	}
	for _, b := range sh.BuyTypes {
		sd.Buys = append(sd.Buys, shopBuyDoc{
			Type:    valueName(b.Type, game.YamlItemTypeNames()),
			Keyword: b.Keyword,
		})
	}
	if sh.Open1 != 0 || sh.Close1 != 0 || sh.Open2 != 0 || sh.Close2 != 0 {
		sd.Hours = append(sd.Hours, [2]int32{sh.Open1, sh.Close1})
		if sh.Open2 != 0 || sh.Close2 != 0 {
			sd.Hours = append(sd.Hours, [2]int32{sh.Open2, sh.Close2})
		}
	}
	flagNames, _ := game.NameBits(uint64(sh.Flags), game.YamlShopFlagNames())
	sd.Flags = flagNames
	refuseNames, _ := game.NameBits(uint64(sh.TradeWith), game.YamlShopTradeNames()) //nolint:gosec // seven bits
	sd.Refuses = refuseNames

	if sh.Messages != [game.NumShopMessages]string{} {
		sd.Messages = &shopMessagesDoc{
			NoSuchItemKeeper: sh.Messages[game.MsgNoSuchItem1],
			NoSuchItemPlayer: sh.Messages[game.MsgNoSuchItem2],
			DoNotBuy:         sh.Messages[game.MsgDoNotBuy],
			KeeperBroke:      sh.Messages[game.MsgMissingCash1],
			PlayerBroke:      sh.Messages[game.MsgMissingCash2],
			Buy:              sh.Messages[game.MsgBuy],
			Sell:             sh.Messages[game.MsgSell],
		}
	}
	return sd
}

func resetsToDoc(nodes []ResetNode) []resetDoc {
	out := make([]resetDoc, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, resetDocFrom(n))
	}
	return out
}

func resetDocFrom(n ResetNode) resetDoc {
	cmd := n.Command
	d := resetDoc{}
	maxPtr := func(v int32) *int32 { return &v }

	switch cmd.Command {
	case game.ResetMobile:
		d.Mob = maxPtr(cmd.Arg1)
		room := cmd.Arg3
		d.Room = &room
		d.Max = maxPtr(cmd.Arg2)
	case game.ResetObject:
		obj := cmd.Arg1
		d.Object = &obj
		d.Max = maxPtr(cmd.Arg2)
		if game.RoomVnum(cmd.Arg3) != game.NoRoom {
			room := cmd.Arg3
			d.Room = &room
		}
	case game.ResetGive:
		d.Give = maxPtr(cmd.Arg1)
		d.Max = maxPtr(cmd.Arg2)
	case game.ResetEquip:
		d.Equip = maxPtr(cmd.Arg1)
		d.Max = maxPtr(cmd.Arg2)
		d.Slot = valueName(cmd.Arg3, game.YamlWearPositionNames())
	case game.ResetPutInObj:
		d.Put = maxPtr(cmd.Arg1)
		d.Max = maxPtr(cmd.Arg2)
		into := cmd.Arg3
		d.Into = &into
	case game.ResetDoor:
		d.Door = maxPtr(cmd.Arg1)
		if dir, ok := game.DirectionFromInt(cmd.Arg2); ok {
			d.Dir = dir.String()
		}
		switch cmd.Arg3 {
		case game.DoorOpen:
			d.State = "open"
		case game.DoorClosed:
			d.State = "closed"
		case game.DoorLocked:
			d.State = "locked"
		}
	case game.ResetRemove:
		room := cmd.Arg1
		d.Room = &room
		d.Remove = maxPtr(cmd.Arg2)
	}

	for _, child := range n.Then {
		d.Then = append(d.Then, resetDocFrom(child))
	}
	return d
}
