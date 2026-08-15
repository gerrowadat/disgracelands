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
// Strings are emitted as-is. They are not UTF-8 in general (lib/world
// contains CP1252 bytes), so the JSON encoder escapes what it must and the
// bytes survive a round trip.

// DumpRoom is a room in the parity format.
type DumpRoom struct {
	Vnum       game.RoomVnum   `json:"vnum"`
	Zone       game.ZoneVnum   `json:"zone"`
	Name       string          `json:"name"`
	Desc       string          `json:"desc"`
	Flags      string          `json:"flags"`
	FlagBits   uint64          `json:"flag_bits"`
	Sector     int32           `json:"sector"`
	Exits      []*DumpExit     `json:"exits"`
	ExtraDescs []DumpExtraDesc `json:"extra_descs"`
}

// DumpExit is one exit; null in the exits array means no exit that way.
type DumpExit struct {
	Dir      string        `json:"dir"`
	Desc     string        `json:"desc"`
	Keywords string        `json:"keywords"`
	DoorFlag int32         `json:"door_flag"`
	Key      game.ObjVnum  `json:"key"`
	ToRoom   game.RoomVnum `json:"to_room"`
}

// DumpExtraDesc is a keyword-triggered description.
type DumpExtraDesc struct {
	Keywords string `json:"keywords"`
	Desc     string `json:"desc"`
}

// DumpMob is a mobile prototype in the parity format.
type DumpMob struct {
	Vnum        game.MobVnum `json:"vnum"`
	Keywords    string       `json:"keywords"`
	ShortDesc   string       `json:"short_desc"`
	LongDesc    string       `json:"long_desc"`
	Desc        string       `json:"desc"`
	ActFlags    string       `json:"act_flags"`
	ActBits     uint64       `json:"act_bits"`
	AffFlags    string       `json:"aff_flags"`
	AffBits     uint64       `json:"aff_bits"`
	Alignment   int32        `json:"alignment"`
	Enhanced    bool         `json:"enhanced"`
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
	Keywords   string           `json:"keywords"`
	ShortDesc  string           `json:"short_desc"`
	Desc       string           `json:"desc"`
	ActionDesc string           `json:"action_desc"`
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
	Name      string        `json:"name"`
	Bottom    game.RoomVnum `json:"bottom"`
	Top       game.RoomVnum `json:"top"`
	Lifespan  int32         `json:"lifespan"`
	ResetMode int32         `json:"reset_mode"`
	Commands  []DumpReset   `json:"commands"`
}

// DumpReset is one reset command. Arg3 is omitted for the two-argument
// opcodes rather than dumped as a zero, because the C loader leaves it
// uninitialised there and a zero would be a lie.
type DumpReset struct {
	Command string `json:"command"`
	IfFlag  int32  `json:"if_flag"`
	Arg1    int32  `json:"arg1"`
	Arg2    int32  `json:"arg2"`
	Arg3    *int32 `json:"arg3"`
}

// Dump is the whole thing, with counts up front so a truncated comparison
// still catches a missing record.
type Dump struct {
	Counts  DumpCounts `json:"counts"`
	Zones   []DumpZone `json:"zones"`
	Rooms   []DumpRoom `json:"rooms"`
	Mobiles []DumpMob  `json:"mobiles"`
	Objects []DumpObj  `json:"objects"`
}

// DumpCounts is the record census.
type DumpCounts struct {
	Rooms   int `json:"rooms"`
	Mobiles int `json:"mobiles"`
	Objects int `json:"objects"`
	Zones   int `json:"zones"`
}

// BuildDump converts a loaded world into the parity format.
func BuildDump(w *game.World) *Dump {
	d := &Dump{
		Counts: DumpCounts{
			Rooms: len(w.Rooms), Mobiles: len(w.Mobiles),
			Objects: len(w.Objects), Zones: len(w.Zones),
		},
		Zones:   make([]DumpZone, 0, len(w.Zones)),
		Rooms:   make([]DumpRoom, 0, len(w.Rooms)),
		Mobiles: make([]DumpMob, 0, len(w.Mobiles)),
		Objects: make([]DumpObj, 0, len(w.Objects)),
	}

	for _, z := range w.Zones {
		dz := DumpZone{
			Vnum: z.Vnum, Name: z.Name, Bottom: z.Bottom, Top: z.Top,
			Lifespan: z.Lifespan, ResetMode: z.ResetMode,
			Commands: make([]DumpReset, 0, len(z.Commands)),
		}
		for _, c := range z.Commands {
			dr := DumpReset{
				Command: string(c.Command), IfFlag: c.IfFlag,
				Arg1: c.Arg1, Arg2: c.Arg2,
			}
			if c.NumArgs() == 3 {
				arg3 := c.Arg3
				dr.Arg3 = &arg3
			}
			dz.Commands = append(dz.Commands, dr)
		}
		d.Zones = append(d.Zones, dz)
	}

	for _, r := range w.Rooms {
		dr := DumpRoom{
			Vnum: r.Vnum, Zone: r.Zone, Name: r.Name, Desc: r.Description,
			Flags: r.Flags.String(), FlagBits: uint64(r.Flags),
			Sector:     r.SectorType,
			Exits:      make([]*DumpExit, game.NumDirections),
			ExtraDescs: dumpExtras(r.ExtraDescs),
		}
		for i, e := range r.Exits {
			if e == nil {
				continue
			}
			dr.Exits[i] = &DumpExit{
				Dir: game.Direction(i).String(), Desc: e.Description,
				Keywords: e.Keywords, DoorFlag: e.DoorFlag,
				Key: e.Key, ToRoom: e.ToRoom,
			}
		}
		d.Rooms = append(d.Rooms, dr)
	}

	for _, m := range w.Mobiles {
		especs := m.Especs
		if especs == nil {
			especs = []game.Espec{}
		}
		d.Mobiles = append(d.Mobiles, DumpMob{
			Vnum: m.Vnum, Keywords: m.Keywords, ShortDesc: m.ShortDesc,
			LongDesc: m.LongDesc, Desc: m.Description,
			ActFlags: m.ActionFlags.String(), ActBits: uint64(m.ActionFlags),
			AffFlags: m.AffectionFlags.String(), AffBits: uint64(m.AffectionFlags),
			Alignment: m.Alignment, Enhanced: m.Enhanced,
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
			Vnum: o.Vnum, Keywords: o.Keywords, ShortDesc: o.ShortDesc,
			Desc: o.Description, ActionDesc: o.ActionDesc, Type: o.Type,
			ExtraFlags: o.ExtraFlags.String(), ExtraBits: uint64(o.ExtraFlags),
			WearFlags: o.WearFlags.String(), WearBits: uint64(o.WearFlags),
			PermAffect: o.PermAffect, Values: o.Values,
			Weight: o.Weight, Cost: o.Cost, RentPerDay: o.RentPerDay,
			MinLevel: o.MinLevel, Affects: affects,
			ExtraDescs: dumpExtras(o.ExtraDescs),
		})
	}

	return d
}

func dumpExtras(in []game.ExtraDesc) []DumpExtraDesc {
	out := make([]DumpExtraDesc, 0, len(in))
	for _, e := range in {
		out = append(out, DumpExtraDesc{Keywords: e.Keywords, Desc: e.Description})
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
