// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package world

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// The dump is the parity format: a canonical rendering of a loaded world that
// two independent loaders can both produce and `diff` can compare. It exists
// so "the Go loader agrees with the C loader" is a check somebody runs rather
// than a claim somebody makes.
//
// Three properties matter more than elegance here:
//
//   - **Deterministic.** Records appear in load order, which is index order,
//     which both loaders share. Nothing is sorted into a different order and
//     no map is ranged over.
//   - **Post-load, not raw file content.** The values dumped are what the
//     loader produced, after the adjustments it makes on the way in — the
//     drink-container weight fix, the article-lowercasing of short
//     descriptions, the reversal of extra-description order. Dumping the file
//     text back out would compare the files to themselves and prove nothing.
//   - **Explicit about absence.** A nil exit is null, not omitted, so a
//     missing exit and an exit to nowhere cannot look the same in a diff.
//
// Strings are emitted as-is. They are not UTF-8 in general (data/world
// contains CP1252 bytes), so the JSON encoder escapes what it must and the
// bytes survive a round trip.

// DumpRoom is a room in the parity format.
type DumpRoom struct {
	Vnum       game.RoomVnum   `json:"vnum"`
	Zone       game.ZoneVnum   `json:"zone"`
	Name       Text            `json:"name"`
	Desc       Text            `json:"desc"`
	Flags      string          `json:"flags"`
	FlagBits   uint64          `json:"flag_bits"`
	Sector     int32           `json:"sector"`
	Exits      []*DumpExit     `json:"exits"`
	ExtraDescs []DumpExtraDesc `json:"extra_descs"`
}

// DumpExit is one exit; null in the exits array means no exit that way.
type DumpExit struct {
	Dir      string        `json:"dir"`
	Desc     Text          `json:"desc"`
	Keywords Text          `json:"keywords"`
	DoorFlag int32         `json:"door_flag"`
	Key      game.ObjVnum  `json:"key"`
	ToRoom   game.RoomVnum `json:"to_room"`
}

// DumpExtraDesc is a keyword-triggered description.
type DumpExtraDesc struct {
	Keywords Text `json:"keywords"`
	Desc     Text `json:"desc"`
}

// DumpMob is a mobile prototype in the parity format.
type DumpMob struct {
	Vnum        game.MobVnum `json:"vnum"`
	Keywords    Text         `json:"keywords"`
	ShortDesc   Text         `json:"short_desc"`
	LongDesc    Text         `json:"long_desc"`
	Desc        Text         `json:"desc"`
	ActFlags    string       `json:"act_flags"`
	ActBits     uint64       `json:"act_bits"`
	AffFlags    string       `json:"aff_flags"`
	AffBits     uint64       `json:"aff_bits"`
	Alignment   int32        `json:"alignment"`
	Enhanced    *bool        `json:"enhanced"`
	Level       int32        `json:"level"`
	Thac0       int32        `json:"thac0"`
	HitRoll     int32        `json:"hitroll"`
	ArmorClass  int32        `json:"ac"`
	ArmorScaled int32        `json:"ac_scaled"`
	HitDice     string       `json:"hit_dice"`
	DamageDice  string       `json:"damage_dice"`
	Gold        int32        `json:"gold"`
	Exp         int32        `json:"exp"`
	Position    int32        `json:"position"`
	DefaultPos  int32        `json:"default_position"`
	Sex         int32        `json:"sex"`
	Especs      []game.Espec `json:"especs"`
}

// DumpObj is an object prototype in the parity format.
type DumpObj struct {
	Vnum       game.ObjVnum     `json:"vnum"`
	Keywords   Text             `json:"keywords"`
	ShortDesc  Text             `json:"short_desc"`
	Desc       Text             `json:"desc"`
	ActionDesc Text             `json:"action_desc"`
	Type       int32            `json:"type"`
	ExtraFlags string           `json:"extra_flags"`
	ExtraBits  uint64           `json:"extra_bits"`
	WearFlags  string           `json:"wear_flags"`
	WearBits   uint64           `json:"wear_bits"`
	PermAffect int32            `json:"perm_affect"`
	Values     [4]int32         `json:"values"`
	Weight     int32            `json:"weight"`
	Cost       int32            `json:"cost"`
	RentPerDay int32            `json:"rent_per_day"`
	MinLevel   int32            `json:"min_level"`
	Affects    []game.ObjAffect `json:"affects"`
	ExtraDescs []DumpExtraDesc  `json:"extra_descs"`
}

// DumpZone is a zone in the parity format.
type DumpZone struct {
	Vnum      game.ZoneVnum `json:"vnum"`
	Name      Text          `json:"name"`
	Bottom    game.RoomVnum `json:"bottom"`
	Top       game.RoomVnum `json:"top"`
	Lifespan  int32         `json:"lifespan"`
	ResetMode int32         `json:"reset_mode"`
	Commands  []DumpReset   `json:"commands"`
}

// DumpReset is one reset command, as the server holds it after renumbering.
//
// Command is the opcode the server ended up with, so a command whose vnums
// did not resolve appears as "*" — disabled — exactly as renum_zone_table()
// leaves it. Disabled says the same thing more legibly for a human reading a
// diff.
//
// Arg3 is null for the two-argument opcodes rather than dumped as a zero,
// because the C loader leaves it uninitialised there and a zero would be a
// lie.
type DumpReset struct {
	Command  string `json:"command"`
	Disabled bool   `json:"disabled"`
	IfFlag   int32  `json:"if_flag"`
	Arg1     *int32 `json:"arg1"`
	Arg2     *int32 `json:"arg2"`
	Arg3     *int32 `json:"arg3"`
}

// DumpShop is a shop in the parity format.
type DumpShop struct {
	Vnum       game.ShopVnum   `json:"vnum"`
	Producing  []game.ObjVnum  `json:"producing"`
	ProfitBuy  string          `json:"profit_buy"`
	ProfitSell string          `json:"profit_sell"`
	BuyTypes   []DumpShopBuy   `json:"buy_types"`
	Messages   []Text          `json:"messages"`
	Temper     int32           `json:"temper"`
	Flags      string          `json:"flags"`
	FlagBits   uint64          `json:"flag_bits"`
	Keeper     game.MobVnum    `json:"keeper"`
	TradeWith  int32           `json:"trade_with"`
	Rooms      []game.RoomVnum `json:"rooms"`
	Open1      int32           `json:"open1"`
	Close1     int32           `json:"close1"`
	Open2      int32           `json:"open2"`
	Close2     int32           `json:"close2"`
}

// DumpShopBuy is one entry of a shop's buy list.
type DumpShopBuy struct {
	Type    int32 `json:"type"`
	Keyword Text  `json:"keyword"`
}

// Dump is the whole thing, with counts up front so a truncated comparison
// still catches a missing record.
type Dump struct {
	Counts  DumpCounts `json:"counts"`
	Zones   []DumpZone `json:"zones"`
	Rooms   []DumpRoom `json:"rooms"`
	Mobiles []DumpMob  `json:"mobiles"`
	Objects []DumpObj  `json:"objects"`
	Shops   []DumpShop `json:"shops"`
}

// DumpCounts is the record census.
type DumpCounts struct {
	Rooms   int `json:"rooms"`
	Mobiles int `json:"mobiles"`
	Objects int `json:"objects"`
	Zones   int `json:"zones"`
	Shops   int `json:"shops"`
}

// Options control what BuildDump emits.
type Options struct {
	// Parity omits the fields the C server does not retain after loading, so
	// the two dumps can be compared directly. Two mob fields fall into this
	// category: the simple/enhanced distinction, which parse_enhanced_mob()
	// consumes without recording, and the espec key/value lines, which
	// interpret_espec() folds into ordinary fields and then discards. The Go
	// loader keeps both because they are useful; the C loader cannot report
	// them at all, so comparing them would only ever produce noise.
	Parity bool
}

// BuildDump converts a loaded world into the parity format.
func BuildDump(w *game.World) *Dump { return BuildDumpWithOptions(w, Options{}) }

// BuildDumpWithOptions is BuildDump with explicit options.
func BuildDumpWithOptions(w *game.World, opts Options) *Dump {
	res := newResolver(w)

	d := &Dump{
		Counts: DumpCounts{
			Rooms: len(w.Rooms), Mobiles: len(w.Mobiles),
			Objects: len(w.Objects), Zones: len(w.Zones),
			Shops: len(w.Shops),
		},
		Zones:   make([]DumpZone, 0, len(w.Zones)),
		Rooms:   make([]DumpRoom, 0, len(w.Rooms)),
		Mobiles: make([]DumpMob, 0, len(w.Mobiles)),
		Objects: make([]DumpObj, 0, len(w.Objects)),
		Shops:   make([]DumpShop, 0, len(w.Shops)),
	}

	for _, z := range w.Zones {
		dz := DumpZone{
			Vnum: z.Vnum, Name: Text(z.Name), Bottom: z.Bottom, Top: z.Top,
			Lifespan: z.Lifespan, ResetMode: z.ResetMode,
			Commands: make([]DumpReset, 0, len(z.Commands)),
		}
		for _, c := range z.Commands {
			opcode, a1, a2, a3, disabled := res.resetCommand(c)
			dr := DumpReset{Command: string(opcode), Disabled: disabled, IfFlag: c.IfFlag}

			// A disabled command is dumped as its disabled self and nothing
			// more. renum_zone_table() rewrites the opcode to '*', which
			// destroys both which command it was - and so how many arguments
			// it took - and the arguments themselves, some of which it had
			// already overwritten with real numbers before noticing the
			// failure. Neither side can reconstruct them, so both dump nulls.
			// That the command is dead is the part worth comparing.
			if !disabled {
				dr.Arg1, dr.Arg2 = &a1, &a2
				if c.NumArgs() == 3 {
					dr.Arg3 = &a3
				}
			}
			dz.Commands = append(dz.Commands, dr)
		}
		d.Zones = append(d.Zones, dz)
	}

	for _, r := range w.Rooms {
		dr := DumpRoom{
			Vnum: r.Vnum, Zone: r.Zone, Name: Text(r.Name), Desc: Text(r.Description),
			Flags: r.Flags.String(), FlagBits: r.Flags.Raw(),
			Sector:     r.SectorType,
			Exits:      make([]*DumpExit, game.NumDirections),
			ExtraDescs: dumpExtras(r.ExtraDescs),
		}
		for i, e := range r.Exits {
			if e == nil {
				continue
			}
			dr.Exits[i] = &DumpExit{
				Dir: game.Direction(i).String(), Desc: Text(e.Description),
				Keywords: Text(e.Keywords), DoorFlag: e.DoorFlag,
				Key: e.Key,
				// Post-resolution: an exit to a room that does not exist is
				// NOWHERE in the running server, and the file's vnum is gone.
				ToRoom: res.room(e.ToRoom),
			}
		}
		d.Rooms = append(d.Rooms, dr)
	}

	for _, m := range w.Mobiles {
		especs := m.Especs
		if especs == nil {
			especs = []game.Espec{}
		}
		enhanced := &m.Enhanced
		if opts.Parity {
			enhanced, especs = nil, []game.Espec{}
		}
		d.Mobiles = append(d.Mobiles, DumpMob{
			Vnum: m.Vnum, Keywords: Text(m.Keywords), ShortDesc: Text(m.ShortDesc),
			LongDesc: Text(m.LongDesc), Desc: Text(m.Description),
			ActFlags: m.ActionFlags.String(), ActBits: uint64(m.ActionFlags),
			AffFlags: m.AffectionFlags.String(), AffBits: uint64(m.AffectionFlags),
			Alignment: m.Alignment, Enhanced: enhanced,
			Level: m.Level, Thac0: m.Thac0, HitRoll: m.HitRoll(),
			ArmorClass: m.ArmorClass, ArmorScaled: m.ArmorClassScaled(),
			HitDice: diceString(m.HitDice), DamageDice: diceString(m.DamageDice),
			Gold: m.Gold, Exp: m.Exp,
			Position: m.Position, DefaultPos: m.DefaultPosition, Sex: m.Sex,
			Especs: especs,
		})
	}

	for _, o := range w.Objects {
		affects := o.Affects
		if affects == nil {
			affects = []game.ObjAffect{}
		}
		d.Objects = append(d.Objects, DumpObj{
			Vnum: o.Vnum, Keywords: Text(o.Keywords), ShortDesc: Text(o.ShortDesc),
			Desc: Text(o.Description), ActionDesc: Text(o.ActionDesc), Type: o.Type,
			ExtraFlags: o.ExtraFlags.String(), ExtraBits: o.ExtraFlags.Raw(),
			WearFlags: o.WearFlags.String(), WearBits: o.WearFlags.Raw(),
			PermAffect: o.PermAffect, Values: o.Values,
			Weight: o.Weight, Cost: o.Cost, RentPerDay: o.RentPerDay,
			MinLevel: o.MinLevel, Affects: affects,
			ExtraDescs: dumpExtras(o.ExtraDescs),
		})
	}

	for _, s := range w.Shops {
		ds := DumpShop{
			Vnum:      s.Vnum,
			Producing: emptyIfNil(s.Producing),
			// Formatted rather than emitted as a JSON number: the C struct
			// holds these as float, and "1.15" round-tripped through two
			// languages' float printers is not reliably the same text. A
			// fixed format both sides use verbatim is.
			ProfitBuy:  formatProfit(s.ProfitBuy),
			ProfitSell: formatProfit(s.ProfitSell),
			BuyTypes:   make([]DumpShopBuy, 0, len(s.BuyTypes)),
			Messages:   make([]Text, game.NumShopMessages),
			Temper:     s.Temper,
			Flags:      s.Flags.String(),
			FlagBits:   s.Flags.Raw(),
			Keeper:     s.Keeper,
			TradeWith:  s.TradeWith,
			Rooms:      emptyIfNil(s.Rooms),
			Open1:      s.Open1,
			Close1:     s.Close1,
			Open2:      s.Open2,
			Close2:     s.Close2,
		}
		for _, b := range s.BuyTypes {
			ds.BuyTypes = append(ds.BuyTypes, DumpShopBuy{Type: b.Type, Keyword: Text(b.Keyword)})
		}
		for i, m := range s.Messages {
			ds.Messages[i] = Text(m)
		}
		d.Shops = append(d.Shops, ds)
	}

	return d
}

// emptyIfNil keeps a nil slice from dumping as null, so an empty list and a
// missing one cannot be confused in a diff.
func emptyIfNil[T any](in []T) []T {
	if in == nil {
		return []T{}
	}
	return in
}

// formatProfit renders a shop's profit multiplier. Six decimal places is what
// C's "%f" defaults to, and matching it exactly is the point.
func formatProfit(f float32) string {
	return fmt.Sprintf("%.6f", f)
}

func dumpExtras(in []game.ExtraDesc) []DumpExtraDesc {
	out := make([]DumpExtraDesc, 0, len(in))
	for _, e := range in {
		out = append(out, DumpExtraDesc{Keywords: Text(e.Keywords), Desc: Text(e.Description)})
	}
	return out
}

func diceString(d game.Dice) string {
	return fmt.Sprintf("%dd%d+%d", d.Number, d.Size, d.Bonus)
}

// WriteDump writes the parity format to w, indented so a diff points at a
// field rather than at a single enormous line.
func WriteDump(w io.Writer, d *Dump) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(d)
}
