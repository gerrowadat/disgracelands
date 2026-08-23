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

	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/goccy/go-yaml"
)

// WriteZone implements world.Sink: it writes one zone and everything whose
// vnum falls inside it to its own file, canonically (§10.3) and atomically
// (§3's "one writer per file").
func (s *Source) WriteZone(_ context.Context, zone *game.ZoneDef, w *game.World) error {
	doc := zoneDocFrom(zone, w)
	out, err := yaml.MarshalWithOptions(doc, yaml.Indent(2))
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
		if int32(m.Vnum) >= int32(zone.Bottom) && int32(m.Vnum) <= int32(zone.Top) {
			doc.Mobiles = append(doc.Mobiles, mobDocFrom(m))
		}
	}
	for _, o := range w.Objects {
		if int32(o.Vnum) >= int32(zone.Bottom) && int32(o.Vnum) <= int32(zone.Top) {
			doc.Objects = append(doc.Objects, objDocFrom(o))
		}
	}
	for _, sh := range w.Shops {
		if int32(sh.Vnum) >= int32(zone.Bottom) && int32(sh.Vnum) <= int32(zone.Top) {
			doc.Shops = append(doc.Shops, shopDocFrom(sh))
		}
	}
	doc.Resets = resetsToDoc(NestResets(zone.Commands))
	return doc
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
	names, raw := game.NameBits(r.Flags, game.YamlRoomFlagNames())
	rd := roomDoc{
		Vnum:     int32(r.Vnum),
		Name:     Text(r.Name),
		Sector:   sectorName(r.SectorType),
		Flags:    names,
		FlagsRaw: uint64(raw),
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

func sectorName(v int32) string {
	name, ok := game.NameByValue(v, game.YamlSectorNames())
	if !ok {
		return fmt.Sprintf("unknown-%d", v)
	}
	return name
}

func doorName(flag int32) string {
	name, _ := game.NameByValue(flag, game.YamlExitDoorNames())
	return name
}

func mobDocFrom(m *game.MobDef) mobDoc {
	actFlags := m.ActionFlags.Clear(game.MobIsNPC) // never written; the reader force-sets it
	actNames, actRaw := game.NameBits(actFlags, game.YamlMobActFlagNames())
	affNames, affRaw := game.NameBits(m.AffectionFlags, game.YamlAffectFlagNames())

	md := mobDoc{
		Vnum:            int32(m.Vnum),
		Keywords:        strings.Fields(m.Keywords),
		Short:           Text(m.ShortDesc),
		Long:            Text(ToStored(m.LongDesc)),
		Desc:            Text(ToStored(m.Description)),
		Act:             actNames,
		ActRaw:          uint64(actRaw),
		Affected:        affNames,
		AffectedRaw:     uint64(affRaw),
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

func valueName(v int32, table []string) string {
	name, ok := game.NameByValue(v, table)
	if !ok {
		return fmt.Sprintf("unknown-%d", v)
	}
	return name
}

func diceDocString(d game.Dice) string {
	if d.Bonus != 0 {
		return fmt.Sprintf("%dd%d+%d", d.Number, d.Size, d.Bonus)
	}
	return fmt.Sprintf("%dd%d", d.Number, d.Size)
}

func objDocFrom(o *game.ObjDef) objDoc {
	wearNames, wearRaw := game.NameBits(o.WearFlags, game.YamlWearFlagNames())
	flagNames, flagRaw := game.NameBits(o.ExtraFlags, game.YamlItemExtraFlagNames())
	permNames, _ := game.NameBits(game.Flags(o.PermAffect), game.YamlAffectFlagNames()) //nolint:gosec // affect bits fit comfortably

	od := objDoc{
		Vnum:       int32(o.Vnum),
		Keywords:   strings.Fields(o.Keywords),
		Short:      Text(o.ShortDesc),
		Desc:       Text(ToStored(o.Description)),
		ActionDesc: Text(ToStored(o.ActionDesc)),
		Type:       valueName(o.Type, game.YamlItemTypeNames()),
		Wear:       wearNames,
		WearRaw:    uint64(wearRaw),
		Flags:      flagNames,
		FlagsRaw:   uint64(flagRaw),
		PermAffect: permNames,
		Weight:     o.Weight,
		Cost:       o.Cost,
		Rent:       o.RentPerDay,
		MinLevel:   o.MinLevel,
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
	flagNames, _ := game.NameBits(sh.Flags, game.YamlShopFlagNames())
	sd.Flags = flagNames
	refuseNames, _ := game.NameBits(game.Flags(sh.TradeWith), game.YamlShopTradeNames()) //nolint:gosec // seven bits
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
