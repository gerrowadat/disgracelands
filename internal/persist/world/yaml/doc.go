// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package yaml

// The zone file's document shape, §4 of docs/design/data-format.md.
// Field order here is the file's key order (§10.3's "fixed key order"):
// goccy's struct encoder writes fields in declaration order, so getting
// this order right once here is the whole implementation of that
// requirement, rather than something the writer has to do separately.

// ZoneSchema is the schema tag every zone file carries.
const ZoneSchema = "dl/world@1"

type zoneDoc struct {
	Schema  string     `yaml:"schema"`
	Zone    zoneMeta   `yaml:"zone"`
	Rooms   []roomDoc  `yaml:"rooms,omitempty"`
	Mobiles []mobDoc   `yaml:"mobiles,omitempty"`
	Objects []objDoc   `yaml:"objects,omitempty"`
	Shops   []shopDoc  `yaml:"shops,omitempty"`
	Resets  []resetDoc `yaml:"resets,omitempty"`
}

type zoneMeta struct {
	Vnum     int32    `yaml:"vnum"`
	Name     string   `yaml:"name"`
	Range    [2]int32 `yaml:"range"`
	Lifespan int32    `yaml:"lifespan"`
	Reset    string   `yaml:"reset"`
}

type roomDoc struct {
	Vnum       int32          `yaml:"vnum"`
	Name       Text           `yaml:"name"`
	Sector     string         `yaml:"sector"`
	Flags      []string       `yaml:"flags,omitempty"`
	FlagsRaw   uint64         `yaml:"flags_raw,omitempty"`
	Desc       Text           `yaml:"desc"`
	Exits      *exitSetDoc    `yaml:"exits,omitempty"`
	ExtraDescs []extraDescDoc `yaml:"extra_descs,omitempty"`
}

// exitSetDoc is keyed by direction as named fields, not a map: §10.3
// requires a fixed key order, and a Go map has none. Field order here is
// game.Direction's file order (north, east, south, west, up, down).
type exitSetDoc struct {
	North *exitDoc `yaml:"north,omitempty"`
	East  *exitDoc `yaml:"east,omitempty"`
	South *exitDoc `yaml:"south,omitempty"`
	West  *exitDoc `yaml:"west,omitempty"`
	Up    *exitDoc `yaml:"up,omitempty"`
	Down  *exitDoc `yaml:"down,omitempty"`
}

func (e *exitSetDoc) slots() [6]**exitDoc {
	if e == nil {
		return [6]**exitDoc{}
	}
	return [6]**exitDoc{&e.North, &e.East, &e.South, &e.West, &e.Up, &e.Down}
}

type exitDoc struct {
	To       int32      `yaml:"to"`
	Desc     NestedText `yaml:"desc,omitempty"`
	Door     string     `yaml:"door,omitempty"`
	Key      int32      `yaml:"key,omitempty"`
	Keywords string     `yaml:"keywords,omitempty"`
}

type extraDescDoc struct {
	Keywords []string   `yaml:"keywords"`
	Desc     NestedText `yaml:"desc"`
}

type mobDoc struct {
	Vnum            int32      `yaml:"vnum"`
	Keywords        []string   `yaml:"keywords"`
	Short           Text       `yaml:"short"`
	Long            Text       `yaml:"long"`
	Desc            Text       `yaml:"desc,omitempty"`
	Act             []string   `yaml:"act,omitempty"`
	ActRaw          uint64     `yaml:"act_raw,omitempty"`
	Affected        []string   `yaml:"affected,omitempty"`
	AffectedRaw     uint64     `yaml:"affected_raw,omitempty"`
	Alignment       int32      `yaml:"alignment"`
	Level           int32      `yaml:"level"`
	Thac0           int32      `yaml:"thac0"`
	AC              int32      `yaml:"ac"`
	HP              string     `yaml:"hp"`
	Damage          string     `yaml:"damage"`
	Gold            int32      `yaml:"gold"`
	Exp             int32      `yaml:"exp"`
	Position        string     `yaml:"position"`
	DefaultPosition string     `yaml:"default_position"`
	Sex             string     `yaml:"sex"`
	Abilities       *Abilities `yaml:"abilities,omitempty"`
}

type objDoc struct {
	Vnum       int32    `yaml:"vnum"`
	Keywords   []string `yaml:"keywords"`
	Short      Text     `yaml:"short"`
	Desc       Text     `yaml:"desc"`
	ActionDesc Text     `yaml:"action_desc,omitempty"`
	Type       string   `yaml:"type"`
	Wear       []string `yaml:"wear,omitempty"`
	WearRaw    uint64   `yaml:"wear_raw,omitempty"`
	Flags      []string `yaml:"flags,omitempty"`
	FlagsRaw   uint64   `yaml:"flags_raw,omitempty"`
	PermAffect []string `yaml:"perm_affect,omitempty"`
	// PermAffectRaw is perm_affect's escape hatch, the same one wear_raw
	// and flags_raw are for their own fields: an AFF_* bit with no name in
	// game.YamlAffectFlagNames survives here rather than being dropped.
	// It was missing until examples/torture went looking, and its absence
	// was silent — an object with an unnamed permanent-affect bit came out
	// of a conversion without it and nothing said so.
	PermAffectRaw uint64           `yaml:"perm_affect_raw,omitempty"`
	Weapon        *WeaponValues    `yaml:"weapon,omitempty"`
	Armor         *ArmorValues     `yaml:"armor,omitempty"`
	Container     *ContainerValues `yaml:"container,omitempty"`
	Drink         *DrinkValues     `yaml:"drink,omitempty"`
	Light         *LightValues     `yaml:"light,omitempty"`
	Charges       *ChargesValues   `yaml:"charges,omitempty"`
	Values        *[4]int32        `yaml:"values,omitempty"`
	Weight        int32            `yaml:"weight"`
	Cost          int32            `yaml:"cost"`
	Rent          int32            `yaml:"rent"`
	MinLevel      int32            `yaml:"min_level,omitempty"`
	Affects       []objAffectDoc   `yaml:"affects,omitempty"`
	ExtraDescs    []extraDescDoc   `yaml:"extra_descs,omitempty"`
}

type objAffectDoc struct {
	Location string `yaml:"location"`
	Modifier int32  `yaml:"modifier"`
}

type shopDoc struct {
	Vnum     int32            `yaml:"vnum"`
	Keeper   int32            `yaml:"keeper"`
	Rooms    []int32          `yaml:"rooms"`
	Sells    []int32          `yaml:"sells,omitempty"`
	Buys     []shopBuyDoc     `yaml:"buys,omitempty"`
	Markup   float32          `yaml:"markup"`
	Markdown float32          `yaml:"markdown"`
	Hours    [][2]int32       `yaml:"hours,omitempty"`
	Flags    []string         `yaml:"flags,omitempty"`
	Refuses  []string         `yaml:"refuses,omitempty"`
	Temper   int32            `yaml:"temper,omitempty"`
	Messages *shopMessagesDoc `yaml:"messages,omitempty"`
}

type shopBuyDoc struct {
	Type    string `yaml:"type"`
	Keyword string `yaml:"keyword,omitempty"`
}

type shopMessagesDoc struct {
	NoSuchItemKeeper string `yaml:"no_such_item_keeper,omitempty"`
	NoSuchItemPlayer string `yaml:"no_such_item_player,omitempty"`
	DoNotBuy         string `yaml:"do_not_buy,omitempty"`
	KeeperBroke      string `yaml:"keeper_broke,omitempty"`
	PlayerBroke      string `yaml:"player_broke,omitempty"`
	Buy              string `yaml:"buy,omitempty"`
	Sell             string `yaml:"sell,omitempty"`
}

// resetDoc is one top-level reset chain entry — the union of the seven
// opcodes' argument shapes, with omitempty picking out which one is
// present. Discriminating on which fields are set (rather than a `kind:`
// tag) matches §4.4's table, where each opcode has a visibly different
// shape rather than a shared envelope.
type resetDoc struct {
	Mob    *int32     `yaml:"mob,omitempty"`
	Room   *int32     `yaml:"room,omitempty"`
	Object *int32     `yaml:"object,omitempty"`
	Give   *int32     `yaml:"give,omitempty"`
	Equip  *int32     `yaml:"equip,omitempty"`
	Slot   string     `yaml:"slot,omitempty"`
	Put    *int32     `yaml:"put,omitempty"`
	Into   *int32     `yaml:"into,omitempty"`
	Door   *int32     `yaml:"door,omitempty"`
	Dir    string     `yaml:"dir,omitempty"`
	State  string     `yaml:"state,omitempty"`
	Remove *int32     `yaml:"remove,omitempty"`
	Max    *int32     `yaml:"max,omitempty"`
	Then   []resetDoc `yaml:"then,omitempty"`
}
