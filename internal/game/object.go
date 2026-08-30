// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import "strings"

// Object instances, and where they can be.
//
// ObjDef is a prototype loaded from the world files; Object is one that
// exists. The C keeps both in one struct and distinguishes them by which
// list they are on, which is why obj_proto and object_list are so easy to
// confuse there. Here they are separate types and the compiler keeps them
// apart.
//
// An object is in exactly one place at a time, and that invariant is the
// whole point of this file: the C maintains it by hand across a dozen
// obj_to_*/obj_from_* functions, and a leak — an object on two lists, or on
// none — is how items get duplicated.

// ItemType values, from structs.h:326.
const (
	ItemLight      int32 = 1
	ItemScroll     int32 = 2
	ItemWand       int32 = 3
	ItemStaff      int32 = 4
	ItemWeapon     int32 = 5
	ItemFireWeapon int32 = 6 // unimplemented in the C too
	ItemMissile    int32 = 7 // unimplemented in the C too
	ItemTreasure   int32 = 8
	ItemArmor      int32 = 9
	ItemPotion     int32 = 10
	ItemWorn       int32 = 11 // unimplemented in the C too
	ItemOther      int32 = 12
	ItemTrash      int32 = 13
	ItemTrap       int32 = 14 // unimplemented in the C too
	ItemContainer  int32 = 15
	ItemNote       int32 = 16
	ItemDrinkCon   int32 = 17
	ItemKey        int32 = 18
	ItemFood       int32 = 19
	ItemMoney      int32 = 20
	ItemPen        int32 = 21
	ItemBoat       int32 = 22
	ItemFountain   int32 = 23
)

// ItemTypeNames is constants.c's item_types[], indexed by type number. Index
// 0 is "UNDEFINED", which is why the table starts there rather than at
// ItemLight.
//
// It is a table rather than a switch because the shop file format matches
// against it *by index*: a shop's trade list may name a type instead of
// numbering it, and the number it means is its position here.
var ItemTypeNames = []string{
	"UNDEFINED", "LIGHT", "SCROLL", "WAND", "STAFF", "WEAPON", "FIRE WEAPON",
	"MISSILE", "TREASURE", "ARMOR", "POTION", "WORN", "OTHER", "TRASH",
	"TRAP", "CONTAINER", "NOTE", "LIQ CONTAINER", "KEY", "FOOD", "MONEY",
	"PEN", "BOAT", "FOUNTAIN",
}

// ItemTypeByName resolves a type name to its number, the way shop.c's
// read_type_list does: the first table entry that is a case-insensitive
// prefix of s wins, and the rest of s is returned for the caller to use as
// the entry's keyword.
//
// Prefix matching is the C's behaviour rather than a convenience: it is what
// lets a shop file write "WEAPON sword" and have the parser take "WEAPON" as
// the type and "sword" as a keyword, and it means "LIQ CONTAINER" is matched
// before anything shorter that shares its start would be.
func ItemTypeByName(s string) (typ int32, rest string, ok bool) {
	for i, name := range ItemTypeNames {
		if len(s) < len(name) || !strings.EqualFold(s[:len(name)], name) {
			continue
		}
		return int32(i), strings.TrimSpace(s[len(name):]), true //nolint:gosec // a table index
	}
	return 0, s, false
}

// Extra flags, from structs.h:368 — what an object *is*, as opposed to where
// it can be worn.
//
// ItemNoDrop is the one with teeth: it is the curse flag, and it makes an
// object refuse to leave your hands. ItemNoLocate is not stock CircleMUD at
// all — it is a Disgracelands local mod, marked in the C by a `/*humbug*/`
// comment on either side of it, and it hides an object from `locate object`.
// ExtraFlag is one of them, and ExtraFlagSet is an object's set. Bit
// indices, not masks: docs/proposals/idiomatic-go.md §4.1, and §4.1.1 for
// the trap. extra_bits[] in constants.c is the name table.
//
// Named ExtraFlagSet rather than ExtraFlags because ObjDef.ExtraFlags is
// the field — the same call WearFlagSet and ContainerFlagSet made.
type ExtraFlag int

// ExtraFlagSet is a set of ExtraFlag.
type ExtraFlagSet = Set[ExtraFlag]

const (
	ItemGlow          ExtraFlag = 0
	ItemHum           ExtraFlag = 1
	ItemNoRent        ExtraFlag = 2
	ItemNoDonate      ExtraFlag = 3
	ItemNoInvis       ExtraFlag = 4
	ItemInvisible     ExtraFlag = 5
	ItemMagic         ExtraFlag = 6
	ItemNoDrop        ExtraFlag = 7
	ItemBless         ExtraFlag = 8
	ItemAntiGood      ExtraFlag = 9
	ItemAntiEvil      ExtraFlag = 10
	ItemAntiNeutral   ExtraFlag = 11
	ItemAntiMagicUser ExtraFlag = 12
	ItemAntiCleric    ExtraFlag = 13
	ItemAntiThief     ExtraFlag = 14
	ItemAntiWarrior   ExtraFlag = 15
	ItemNoSell        ExtraFlag = 16
	// ItemNoLocate is local: structs.h:388, between the two /*humbug*/
	// markers.
	ItemNoLocate ExtraFlag = 17
)

// Wear flags, from structs.h:352. ItemWearTake is the odd one out: it is not
// a place to wear something, it is whether the object can be picked up at
// all.
// WearFlag is one of them, and WearFlagSet is an object's set. Bit
// indices, not masks: docs/proposals/idiomatic-go.md §4.1, and §4.1.1 for
// the trap. wear_bits[] in constants.c is the name table and
// bitnames_test.go is what keeps these numbers the C's.
//
// Named WearFlagSet rather than WearFlags because ObjDef.WearFlags is the
// field, and a type with the same name as the field it holds reads badly
// at the declaration — the same call ContainerFlagSet made, for the same
// reason.
type WearFlag int

// WearFlagSet is a set of WearFlag.
type WearFlagSet = Set[WearFlag]

const (
	ItemWearTake   WearFlag = 0
	ItemWearFinger WearFlag = 1
	ItemWearNeck   WearFlag = 2
	ItemWearBody   WearFlag = 3
	ItemWearHead   WearFlag = 4
	ItemWearLegs   WearFlag = 5
	ItemWearFeet   WearFlag = 6
	ItemWearHands  WearFlag = 7
	ItemWearArms   WearFlag = 8
	ItemWearShield WearFlag = 9
	ItemWearAbout  WearFlag = 10
	ItemWearWaist  WearFlag = 11
	ItemWearWrist  WearFlag = 12
	ItemWearWield  WearFlag = 13
	ItemWearHold   WearFlag = 14
)

// WearPosition is a slot on a body, from structs.h:300.
type WearPosition int

// The wear positions, in the C's order. The order is the order `equipment`
// lists them in, so it is player-visible.
const (
	WearLight WearPosition = iota
	WearFingerRight
	WearFingerLeft
	WearNeck1
	WearNeck2
	WearBody
	WearHead
	WearLegs
	WearFeet
	WearHands
	WearArms
	WearShield
	WearAbout
	WearWaist
	WearWristRight
	WearWristLeft
	WearWield
	WearHold
	// NumWears must be the number of equipment positions, as the C's comment
	// insists in capitals.
	NumWears
)

// wearNames are the phrases `equipment` prints beside each slot, from
// constants.c's wear_where.
var wearNames = [NumWears]string{
	WearLight:       "<used as light>      ",
	WearFingerRight: "<worn on finger>     ",
	WearFingerLeft:  "<worn on finger>     ",
	WearNeck1:       "<worn around neck>   ",
	WearNeck2:       "<worn around neck>   ",
	WearBody:        "<worn on body>       ",
	WearHead:        "<worn on head>       ",
	WearLegs:        "<worn on legs>       ",
	WearFeet:        "<worn on feet>       ",
	WearHands:       "<worn on hands>      ",
	WearArms:        "<worn on arms>       ",
	WearShield:      "<worn as shield>     ",
	WearAbout:       "<worn about body>    ",
	WearWaist:       "<worn around waist>  ",
	WearWristRight:  "<worn around wrist>  ",
	WearWristLeft:   "<worn around wrist>  ",
	WearWield:       "<wielded>            ",
	WearHold:        "<held>               ",
}

// String names the slot as `equipment` shows it.
func (w WearPosition) String() string {
	if w < 0 || w >= NumWears {
		return "<nowhere>"
	}
	return wearNames[w]
}

// wearFlagFor maps a slot to the flags an object needs to go in it.
//
// A set per slot rather than a single flag, and the empty set for the light
// slot rather than a zero. Under the old bitmask model `WearLight: 0` was
// "no flag required" and `Has(0)` was trivially true; under bit indices 0
// is ItemWearTake, so the same literal would have quietly started demanding
// a takeable object. The empty set keeps the old meaning exactly —
// Contains reports true for it, which is the C's IS_SET(x, 0). Neither
// reader can actually reach this entry (both special-case WearLight before
// the lookup), but a table that means the wrong thing when read is worth
// fixing whether or not anything reads it.
var wearFlagFor = [NumWears]WearFlagSet{
	WearLight:       {}, // anything can be a light; the C checks the type
	WearFingerRight: NewSet(ItemWearFinger),
	WearFingerLeft:  NewSet(ItemWearFinger),
	WearNeck1:       NewSet(ItemWearNeck),
	WearNeck2:       NewSet(ItemWearNeck),
	WearBody:        NewSet(ItemWearBody),
	WearHead:        NewSet(ItemWearHead),
	WearLegs:        NewSet(ItemWearLegs),
	WearFeet:        NewSet(ItemWearFeet),
	WearHands:       NewSet(ItemWearHands),
	WearArms:        NewSet(ItemWearArms),
	WearShield:      NewSet(ItemWearShield),
	WearAbout:       NewSet(ItemWearAbout),
	WearWaist:       NewSet(ItemWearWaist),
	WearWristRight:  NewSet(ItemWearWrist),
	WearWristLeft:   NewSet(ItemWearWrist),
	WearWield:       NewSet(ItemWearWield),
	WearHold:        NewSet(ItemWearHold),
}

// CanWearAt reports whether a prototype may go in a slot.
func CanWearAt(def *ObjDef, pos WearPosition) bool {
	if def == nil || pos < 0 || pos >= NumWears {
		return false
	}
	if pos == WearLight {
		return def.Type == ItemLight
	}
	return def.WearFlags.Contains(wearFlagFor[pos])
}

// FitsAt reports whether an object may go in a slot, porting perform_wear's
// wear_bitvectors[] check.
//
// The light and hold slots ask only for ITEM_WEAR_TAKE, which is the C's
// comment explaining itself: "you can hold any object, not just an object
// with a HOLD bit". The commands that reach those slots do their own
// checking, which is why `hold` still refuses a sword.
func (o *Object) FitsAt(pos WearPosition) bool {
	if o == nil || pos < 0 || pos >= NumWears {
		return false
	}
	needs := wearFlagFor[pos]
	if pos == WearLight || pos == WearHold {
		needs = NewSet(ItemWearTake)
	}
	return o.WearFlags.Contains(needs)
}

// ActionDescription is the object's `action_description`: what the room is
// told when it is used. A wand with one says that instead of "$n points $p at
// $N", which is how a builder gives an item its own voice — and what a note
// holds once somebody has written on it.
func (o *Object) ActionDescription() string {
	if o == nil {
		return ""
	}
	if o.ActionDesc != "" {
		return o.ActionDesc
	}
	if o.Def == nil {
		return ""
	}
	return o.Def.ActionDesc
}

// MinLevel is the level needed to use the object. Local: the C tests
// GET_OBJ_LEVEL in do_wear and perform_wear between `<DoC>` markers, and
// stock CircleMUD does not.
func (o *Object) MinLevel() int32 {
	if o == nil || o.Def == nil {
		return 0
	}
	return o.Def.MinLevel
}

// Location says where an object is. Exactly one of these is true at a time,
// which is the invariant the whole file exists to keep.
type Location int

const (
	// InNowhere: newly created, or extracted and not yet discarded.
	InNowhere Location = iota
	// InRoom: lying on the floor.
	InRoom
	// CarriedBy: in somebody's inventory.
	CarriedBy
	// WornBy: equipped.
	WornBy
	// InObject: inside a container.
	InObject
)

// Object is one object that exists in the world.
type Object struct {
	// ID is unique for the life of the server. The C has no such thing and
	// identifies objects by pointer, which is why its logs are so hard to
	// follow.
	ID uint64

	// Def is the prototype. Nil for an object built at runtime — a corpse,
	// or a pile of coins.
	Def *ObjDef

	// The fields below shadow the prototype's so that one object can differ
	// from its prototype: a corpse's name, a drink container's contents, a
	// wand's remaining charges.
	Keywords    string
	ShortDesc   string
	Description string
	// ActionDesc is what the room is told when the object is used, and it is
	// per-object rather than per-prototype because `write` puts your words on
	// *this* note and not on every note of its kind.
	ActionDesc string

	Type       int32
	ExtraFlags ExtraFlagSet
	WearFlags  WearFlagSet
	Values     [NumObjValues]int32
	Weight     int32
	Cost       int32

	// Affects are the object's `A` lines: what wearing it changes. Copied
	// from the prototype so that one object can be enchanted without every
	// other of its kind changing with it.
	Affects []ObjAffect
	// PermAffect are the AFF_* bits wearing it confers — sanctuary from a
	// cloak, infravision from a helm. The C calls this
	// obj->obj_flags.bitvector and reads it with GET_OBJ_AFFECT.
	PermAffect Flags

	// Timer counts down for objects that decay. A corpse is the only thing
	// that uses it in the stock game.
	Timer int32

	// Where the object is. Set only through the Put/Take functions below.
	Location  Location
	Room      RoomVnum
	Holder    *Character
	WornAt    WearPosition
	Container *Object

	// Contents is what is inside this object, if it is a container.
	Contents []*Object
}

// NewObject instantiates a prototype, porting read_object (db.c).
func NewObject(id uint64, def *ObjDef) *Object {
	o := &Object{
		ID:       id,
		Def:      def,
		Location: InNowhere,
		WornAt:   -1,
	}
	if def != nil {
		o.Keywords = def.Keywords
		o.ShortDesc = def.ShortDesc
		o.Description = def.Description
		o.Type = def.Type
		o.ExtraFlags = def.ExtraFlags
		o.WearFlags = def.WearFlags
		o.Values = def.Values
		o.Weight = def.Weight
		o.Cost = def.Cost
		o.Affects = append([]ObjAffect(nil), def.Affects...)
		o.PermAffect = Flags(uint32(def.PermAffect)) //nolint:gosec // a bitfield, read as written
	}
	return o
}

// Vnum returns the prototype's number, or NoVnum for a runtime-built object.
func (o *Object) Vnum() ObjVnum {
	if o == nil || o.Def == nil {
		return NoObject
	}
	return o.Def.Vnum
}

// Name is what to call the object in a message.
func (o *Object) Name() string {
	if o == nil {
		return "something"
	}
	if o.ShortDesc != "" {
		return o.ShortDesc
	}
	if o.Keywords != "" {
		return o.Keywords
	}
	return "something"
}

// TotalWeight is the object's own weight plus everything inside it, porting
// GET_OBJ_WEIGHT's use with containers.
func (o *Object) TotalWeight() int32 {
	if o == nil {
		return 0
	}
	total := o.Weight
	for _, inside := range o.Contents {
		total += inside.TotalWeight()
	}
	return total
}

// Takeable reports whether the object can be picked up: ITEM_WEAR_TAKE.
func (o *Object) Takeable() bool { return o != nil && o.WearFlags.Has(ItemWearTake) }

// Matches reports whether a typed word names this object, as the C's
// isname() does: any whitespace-separated keyword the word is a prefix of.
func (o *Object) Matches(word string) bool {
	if o == nil || word == "" {
		return false
	}
	return matchesKeywords(o.Keywords, word)
}
