// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package yaml

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/goccy/go-yaml"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// loadZoneFile decodes one zone file and appends its records to w.
func (l *loader) loadZoneFile(path string, w *game.World) error {
	data, err := os.ReadFile(path) //nolint:gosec // path built from the world directory
	if err != nil {
		return err
	}
	var doc zoneDoc
	if err := yaml.UnmarshalWithOptions(data, &doc, yaml.Strict()); err != nil {
		return err
	}
	if doc.Schema != ZoneSchema {
		return fmt.Errorf("schema %q, want %q", doc.Schema, ZoneSchema)
	}

	zone := &game.ZoneDef{
		Vnum:     game.ZoneVnum(doc.Zone.Vnum),
		Name:     doc.Zone.Name,
		Bottom:   game.RoomVnum(doc.Zone.Range[0]),
		Top:      game.RoomVnum(doc.Zone.Range[1]),
		Lifespan: doc.Zone.Lifespan,
	}
	switch doc.Zone.Reset {
	case "never":
		zone.ResetMode = 0
	case "empty":
		zone.ResetMode = 1
	case "always":
		zone.ResetMode = 2
	default:
		l.errorf("%s: zone %d: reset mode %q is not never/empty/always", path, doc.Zone.Vnum, doc.Zone.Reset)
	}

	for _, rd := range doc.Rooms {
		room := l.roomFromDoc(path, rd)
		room.Zone = zone.Vnum
		w.Rooms = append(w.Rooms, room)
	}
	for _, md := range doc.Mobiles {
		w.Mobiles = append(w.Mobiles, l.mobFromDoc(path, md))
	}
	for _, od := range doc.Objects {
		w.Objects = append(w.Objects, l.objFromDoc(path, od))
	}
	for _, sd := range doc.Shops {
		w.Shops = append(w.Shops, l.shopFromDoc(path, sd))
	}
	zone.Commands = FlattenResets(l.resetsFromDoc(path, doc.Resets))
	w.Zones = append(w.Zones, zone)
	return nil
}

// flags resolves a list of yaml flag names against table and returns the
// raw bit vector, reporting any name the table does not carry.
//
// Raw bits rather than any flag type: this one method serves six domains
// with six different destination types, and docs/design/idiomatic-go.md
// step 1 gives each of them its own. Bits are the one thing they have in
// common, and the loader is the persistence boundary Set.Raw/SetFromRaw
// exist for (§4.1).
func (l *loader) flags(path string, names []string, table []string, kind string, vnum int32) uint64 {
	f, unknown := game.ParseBitNames(names, table)
	if len(unknown) > 0 {
		l.errorf("%s: %s #%d: unknown %s name(s): %s", path, kind, vnum, kind, strings.Join(unknown, ", "))
	}
	return f
}

func (l *loader) roomFromDoc(path string, rd roomDoc) *game.RoomDef {
	room := &game.RoomDef{
		Vnum:        game.RoomVnum(rd.Vnum),
		Name:        string(rd.Name),
		Description: FromStored(string(rd.Desc)),
	}
	sector, ok := game.ValueByNameOrNumber(rd.Sector, game.YamlSectorNames())
	if !ok {
		l.errorf("%s: room #%d: unknown sector %q", path, rd.Vnum, rd.Sector)
	}
	room.SectorType = game.Sector(sector)
	room.Flags = game.SetFromRaw[game.RoomFlag](l.flags(path, rd.Flags, game.YamlRoomFlagNames(), "room flag", rd.Vnum) | rd.FlagsRaw)

	if rd.Exits != nil {
		slots := rd.Exits.slots()
		for dir := game.Direction(0); int(dir) < game.NumDirections; dir++ {
			ed := *slots[dir]
			if ed == nil {
				continue
			}
			exit := &game.ExitDef{
				Description: FromStored(string(ed.Desc)),
				Keywords:    ed.Keywords,
				Key:         game.ObjVnum(ed.Key),
				ToRoom:      game.RoomVnum(ed.To),
			}
			switch ed.Door {
			case "":
				exit.DoorFlag = 0
			case "regular":
				exit.DoorFlag = 1
			case "pickproof":
				exit.DoorFlag = 2
			default:
				l.errorf("%s: room #%d: exit %s: unknown door kind %q", path, rd.Vnum, dir, ed.Door)
			}
			exit.State = game.DoorState(exit.DoorFlag)
			room.Exits[dir] = exit
		}
	}

	for _, ed := range rd.ExtraDescs {
		room.ExtraDescs = append(room.ExtraDescs, game.ExtraDesc{
			Keywords:    strings.Join(ed.Keywords, " "),
			Description: FromStored(string(ed.Desc)),
		})
	}
	return room
}

func (l *loader) mobFromDoc(path string, md mobDoc) *game.MobDef {
	mob := &game.MobDef{
		Vnum:        game.MobVnum(md.Vnum),
		Keywords:    strings.Join(md.Keywords, " "),
		ShortDesc:   string(md.Short),
		LongDesc:    FromStored(string(md.Long)),
		Description: FromStored(string(md.Desc)),
		Alignment:   md.Alignment,
		Level:       md.Level,
		Thac0:       md.Thac0,
		ArmorClass:  md.AC,
		Gold:        md.Gold,
		Exp:         md.Exp,
	}
	mob.ActionFlags = game.SetFromRaw[game.MobFlag](l.flags(path, md.Act, game.YamlMobActFlagNames(), "mob act flag", md.Vnum) | md.ActRaw).With(game.MobIsNPC)
	mob.AffectionFlags = game.SetFromRaw[game.AffectFlag](l.flags(path, md.Affected, game.YamlAffectFlagNames(), "affect flag", md.Vnum) | md.AffectedRaw)

	if hp, ok := parseDiceString(md.HP); ok {
		mob.HitDice = hp
	} else {
		l.errorf("%s: mob #%d: bad hp dice %q", path, md.Vnum, md.HP)
	}
	if dmg, ok := parseDiceString(md.Damage); ok {
		mob.DamageDice = dmg
	} else {
		l.errorf("%s: mob #%d: bad damage dice %q", path, md.Vnum, md.Damage)
	}

	if pos, ok := game.ValueByNameOrNumber(md.Position, game.YamlPositionNames()); ok {
		mob.Position = game.Position(pos)
	} else {
		l.errorf("%s: mob #%d: unknown position %q", path, md.Vnum, md.Position)
	}
	if pos, ok := game.ValueByNameOrNumber(md.DefaultPosition, game.YamlPositionNames()); ok {
		mob.DefaultPosition = game.Position(pos)
	} else {
		l.errorf("%s: mob #%d: unknown default_position %q", path, md.Vnum, md.DefaultPosition)
	}
	if sex, ok := game.ValueByNameOrNumber(md.Sex, game.YamlSexNames()); ok {
		mob.Sex = game.Sex(sex)
	} else {
		l.errorf("%s: mob #%d: unknown sex %q", path, md.Vnum, md.Sex)
	}

	if md.Abilities != nil {
		mob.Enhanced = true
		mob.Especs = EspecsFromAbilities(*md.Abilities)
	}
	return mob
}

func (l *loader) objFromDoc(path string, od objDoc) *game.ObjDef {
	obj := &game.ObjDef{
		Vnum:        game.ObjVnum(od.Vnum),
		Keywords:    strings.Join(od.Keywords, " "),
		ShortDesc:   string(od.Short),
		Description: FromStored(string(od.Desc)),
		ActionDesc:  FromStored(string(od.ActionDesc)),
		Weight:      od.Weight,
		Cost:        od.Cost,
		RentPerDay:  od.Rent,
		MinLevel:    od.MinLevel,
	}
	typ, ok := game.ValueByNameOrNumber(od.Type, game.YamlItemTypeNames())
	if !ok {
		l.errorf("%s: object #%d: unknown type %q", path, od.Vnum, od.Type)
	}
	obj.Type = game.ItemType(typ)
	obj.WearFlags = game.SetFromRaw[game.WearFlag](l.flags(path, od.Wear, game.YamlWearFlagNames(), "wear flag", od.Vnum) | od.WearRaw)
	obj.ExtraFlags = game.SetFromRaw[game.ExtraFlag](l.flags(path, od.Flags, game.YamlItemExtraFlagNames(), "item flag", od.Vnum) | od.FlagsRaw)
	permAffect, unknown := game.ParseBitNames(od.PermAffect, game.YamlAffectFlagNames())
	if len(unknown) > 0 {
		l.errorf("%s: object #%d: unknown perm_affect name(s): %s", path, od.Vnum, strings.Join(unknown, ", "))
	}
	obj.PermAffect = int32(permAffect | od.PermAffectRaw) //nolint:gosec // affect bits fit comfortably

	obj.Values = l.objValues(path, od)

	for _, ad := range od.Affects {
		loc, ok := game.ValueByNameOrNumber(ad.Location, game.YamlApplyTypeNames())
		if !ok {
			l.errorf("%s: object #%d: unknown affect location %q", path, od.Vnum, ad.Location)
		}
		obj.Affects = append(obj.Affects, game.ObjAffect{Location: game.Apply(loc), Modifier: ad.Modifier})
	}
	for _, ed := range od.ExtraDescs {
		obj.ExtraDescs = append(obj.ExtraDescs, game.ExtraDesc{
			Keywords:    strings.Join(ed.Keywords, " "),
			Description: FromStored(string(ed.Desc)),
		})
	}
	return obj
}

// objValues resolves whichever typed block (if any) the document carries
// back to the raw four values, or uses the raw values: form directly.
// Exactly one of these should be present; if more than one is, the first
// found in this order wins and the rest are ignored silently — a
// consequence of the loose "any of these may be set" document shape rather
// than a validated union, which dlctl lint --type=world's write path never
// produces since the writer only ever sets one.
func (l *loader) objValues(path string, od objDoc) [game.NumObjValues]int32 {
	switch {
	case od.Weapon != nil:
		v, ok := ValuesFromWeapon(*od.Weapon)
		if !ok {
			l.errorf("%s: object #%d: invalid weapon block %+v", path, od.Vnum, *od.Weapon)
		}
		return v
	case od.Armor != nil:
		return ValuesFromArmor(*od.Armor)
	case od.Container != nil:
		return ValuesFromContainer(*od.Container)
	case od.Drink != nil:
		v, ok := ValuesFromDrink(*od.Drink)
		if !ok {
			l.errorf("%s: object #%d: invalid drink block %+v", path, od.Vnum, *od.Drink)
		}
		return v
	case od.Light != nil:
		return ValuesFromLight(*od.Light)
	case od.Charges != nil:
		v, ok := ValuesFromCharges(*od.Charges)
		if !ok {
			l.errorf("%s: object #%d: invalid charges block %+v", path, od.Vnum, *od.Charges)
		}
		return v
	case od.Values != nil:
		return *od.Values
	}
	l.errorf("%s: object #%d: no values given (values:, or a typed block)", path, od.Vnum)
	return [game.NumObjValues]int32{}
}

func (l *loader) shopFromDoc(path string, sd shopDoc) *game.ShopDef {
	shop := &game.ShopDef{
		Vnum:       game.ShopVnum(sd.Vnum),
		Keeper:     game.MobVnum(sd.Keeper),
		ProfitBuy:  sd.Markup,
		ProfitSell: sd.Markdown,
		Temper:     sd.Temper,
	}
	for _, r := range sd.Rooms {
		shop.Rooms = append(shop.Rooms, game.RoomVnum(r))
	}
	for _, s := range sd.Sells {
		shop.Producing = append(shop.Producing, game.ObjVnum(s))
	}

	for _, b := range sd.Buys {
		typ, ok := game.ValueByNameOrNumber(b.Type, game.YamlItemTypeNames())
		if !ok {
			l.errorf("%s: shop #%d: unknown buy type %q", path, sd.Vnum, b.Type)
		}
		shop.BuyTypes = append(shop.BuyTypes, game.ShopBuyType{Type: game.ItemType(typ), Keyword: b.Keyword})
	}
	for i, h := range sd.Hours {
		switch i {
		case 0:
			shop.Open1, shop.Close1 = h[0], h[1]
		case 1:
			shop.Open2, shop.Close2 = h[0], h[1]
		}
	}
	shop.Flags = game.SetFromRaw[game.ShopFlag](l.flags(path, sd.Flags, game.YamlShopFlagNames(), "shop flag", sd.Vnum))
	trade, unknown := game.ParseBitNames(sd.Refuses, game.YamlShopTradeNames())
	if len(unknown) > 0 {
		l.errorf("%s: shop #%d: unknown refuses name(s): %s", path, sd.Vnum, strings.Join(unknown, ", "))
	}
	shop.TradeWith = int32(trade) //nolint:gosec // seven bits

	if sd.Messages != nil {
		shop.Messages = [game.NumShopMessages]string{
			sd.Messages.NoSuchItemKeeper, sd.Messages.NoSuchItemPlayer, sd.Messages.DoNotBuy,
			sd.Messages.KeeperBroke, sd.Messages.PlayerBroke, sd.Messages.Buy, sd.Messages.Sell,
		}
	}
	return shop
}

func (l *loader) resetsFromDoc(path string, docs []resetDoc) []ResetNode {
	var out []ResetNode
	for _, d := range docs {
		out = append(out, l.resetNodeFromDoc(path, d))
	}
	return out
}

func (l *loader) resetNodeFromDoc(path string, d resetDoc) ResetNode {
	cmd := game.ResetCommand{}
	switch {
	case d.Mob != nil:
		cmd.Command = game.ResetMobile
		cmd.Arg1, cmd.Arg3 = *d.Mob, valOr(d.Room, 0)
		cmd.Arg2 = valOr(d.Max, 0)
	case d.Object != nil && d.Room != nil:
		cmd.Command = game.ResetObject
		cmd.Arg1, cmd.Arg3 = *d.Object, *d.Room
		cmd.Arg2 = valOr(d.Max, 0)
	case d.Give != nil:
		cmd.Command = game.ResetGive
		cmd.Arg1 = *d.Give
		cmd.Arg2 = valOr(d.Max, 0)
	case d.Equip != nil:
		cmd.Command = game.ResetEquip
		cmd.Arg1 = *d.Equip
		cmd.Arg2 = valOr(d.Max, 0)
		if slot, ok := wearSlotByName(d.Slot); ok {
			cmd.Arg3 = slot
		} else {
			l.errorf("%s: reset equip %d: unknown slot %q", path, *d.Equip, d.Slot)
		}
	case d.Put != nil:
		cmd.Command = game.ResetPutInObj
		cmd.Arg1 = *d.Put
		cmd.Arg2 = valOr(d.Max, 0)
		cmd.Arg3 = valOr(d.Into, 0)
	case d.Object != nil:
		// object: with no room: means "count against the population cap
		// only", the NOWHERE case reset.go documents.
		cmd.Command = game.ResetObject
		cmd.Arg1 = *d.Object
		cmd.Arg2 = valOr(d.Max, 0)
		cmd.Arg3 = int32(game.NoRoom)
	case d.Door != nil:
		cmd.Command = game.ResetDoor
		cmd.Arg1 = *d.Door
		if dir, ok := game.ParseDirection(d.Dir); ok {
			cmd.Arg2 = int32(dir)
		} else {
			l.errorf("%s: reset door %d: unknown direction %q", path, *d.Door, d.Dir)
		}
		switch d.State {
		case "open":
			cmd.Arg3 = game.DoorOpen
		case "closed":
			cmd.Arg3 = game.DoorClosed
		case "locked":
			cmd.Arg3 = game.DoorLocked
		default:
			l.errorf("%s: reset door %d: unknown state %q", path, *d.Door, d.State)
		}
	case d.Remove != nil:
		cmd.Command = game.ResetRemove
		cmd.Arg1 = valOr(d.Room, 0)
		cmd.Arg2 = *d.Remove
	default:
		l.errorf("%s: reset entry has no recognised opcode: %+v", path, d)
	}

	node := ResetNode{Command: cmd}
	for _, child := range d.Then {
		node.Then = append(node.Then, l.resetNodeFromDoc(path, child))
	}
	return node
}

func valOr(p *int32, fallback int32) int32 {
	if p == nil {
		return fallback
	}
	return *p
}

func wearSlotByName(name string) (int32, bool) {
	return game.ValueByNameOrNumber(name, game.YamlWearPositionNames())
}

func parseDiceString(s string) (game.Dice, bool) {
	// "NdS+B" or "NdS".
	d := game.Dice{}
	rest := s
	i := strings.IndexByte(rest, 'd')
	if i < 0 {
		return d, false
	}
	n, err := strconv.Atoi(rest[:i])
	if err != nil {
		return d, false
	}
	rest = rest[i+1:]
	if j := strings.IndexByte(rest, '+'); j >= 0 {
		size, err := strconv.Atoi(rest[:j])
		if err != nil {
			return d, false
		}
		bonus, err := strconv.Atoi(rest[j+1:])
		if err != nil {
			return d, false
		}
		return game.Dice{Number: int32(n), Size: int32(size), Bonus: int32(bonus)}, true //nolint:gosec // world-data-scale dice values
	}
	size, err := strconv.Atoi(rest)
	if err != nil {
		return d, false
	}
	return game.Dice{Number: int32(n), Size: int32(size)}, true //nolint:gosec // world-data-scale dice values
}
