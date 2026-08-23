// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

// Package yaml implements docs/design/data-format.md §8's player
// format: one YAML file per character, folding in the roster entry and the
// rent/crash file both — there is no separate plrobjs/, and it is what lets
// this format restore real containment (see internal/server/rent.go and
// player.StoredObject.Contains), which a flat binary/ascii rent file has
// nowhere to record.
//
// The field set matches what internal/persist/player/ascii/codec.go
// actually reads and writes today, not §8's own illustrative example —
// ascii persists Race, for instance, which the proposal's sketch omits, and
// neither ascii nor binary ever persisted RealAbilities/RealArmor/
// RealHitRoll/RealDamRoll/RealMaxHit/RealMaxMana/RealMaxMove/
// RealSavingThrows/BaseAffectFlags at all — the existing formats treat the
// live figures they do save as the base a fresh login recomputes from, and
// this one does the same rather than inventing a place to keep values
// nothing before it has ever kept.
package yaml

import (
	worldtext "github.com/gerrowadat/disgracelands/internal/persist/world/yaml"
)

// Text is the world format's own block-scalar/quoting logic
// (docs/design/data-format.md §10.3, §4.6), reused rather than
// duplicated: a player's description is prose with exactly the same
// fidelity requirements a room's is. worldtext is internal/persist/
// world/yaml under an alias, since both packages are named yaml and
// Go allows but does not disambiguate that on its own.
type Text = worldtext.Text

// playerDoc is one player.yaml, field order matching the document shape
// docs/design/data-format.md §8 shows.
type playerDoc struct {
	Schema     string         `yaml:"schema"`
	ID         int64          `yaml:"id"`
	Name       string         `yaml:"name"`
	Credential string         `yaml:"credential,omitempty"`
	Identity   identityDoc    `yaml:"identity"`
	Times      timesDoc       `yaml:"times"`
	Body       bodyDoc        `yaml:"body"`
	Pools      poolsDoc       `yaml:"pools"`
	Combat     combatDoc      `yaml:"combat"`
	Wealth     wealthDoc      `yaml:"wealth"`
	Conditions conditionsDoc  `yaml:"conditions"`
	Flags      playerFlagsDoc `yaml:"flags"`

	PracticeSessions    int32 `yaml:"practice_sessions,omitempty"`
	InvisibilityLevel   int32 `yaml:"invisibility_level,omitempty"`
	FrozenByLevel       int32 `yaml:"frozen_by_level,omitempty"`
	BadPasswordAttempts int32 `yaml:"bad_password_attempts,omitempty"`

	Skills  map[string]int32 `yaml:"skills,omitempty"`
	Affects []affectDoc      `yaml:"affects,omitempty"`
	Aliases []aliasDoc       `yaml:"aliases,omitempty"`

	// Rent and Inventory are the folded-in rent/crash file
	// (player.RentFile/StoredObject) — absent for a character who has
	// never left the game carrying anything, the same case ObjectStore's
	// doc comment calls out as ErrNotFound for the split-file formats.
	//
	// There is no separate equipment: section. §8's own sketch has one, but
	// USE_AUTOEQ is 0 in this tree (see docs/deviations.md's "Renting empties
	// your bags and strips your body"), so nothing — in any format — ever
	// puts a loaded item back on a character's body; internal/server/rent.go
	// restores every entry into inventory regardless of what it says about
	// where something used to be worn. A section implying re-equipping
	// happens would be describing a behaviour this server does not have; a
	// flat Inventory list describes exactly what actually comes back.
	Rent      *rentDoc         `yaml:"rent,omitempty"`
	Inventory []ObjInstanceDoc `yaml:"inventory,omitempty"`
}

type identityDoc struct {
	Title string `yaml:"title,omitempty"`
	Sex   string `yaml:"sex,omitempty"`
	Class string `yaml:"class,omitempty"`
	// Race has no symbolic name table: nothing in internal/game ever reads
	// PlayerRecord.Race (confirmed by grep — it is written and read back
	// by ascii/binary and touched nowhere else), so there is no meaning to
	// name it against, only a number to preserve.
	Race      int32    `yaml:"race,omitempty"`
	Level     int32    `yaml:"level,omitempty"`
	Remort    []string `yaml:"remort,omitempty"`
	RemortRaw uint64   `yaml:"remort_raw,omitempty"`
	Home      int32    `yaml:"home,omitempty"`
	// LoadRoom is where they reappear on login. Omitted at 0, matching
	// ascii's own putIntIf(tagRoom, ...) — which means an explicit room
	// vnum 0 cannot be distinguished from "absent" any more than it can
	// there, and an absent value decodes back to 0 rather than
	// game.NoRoom's -1. Existing ascii behaviour, not a new imprecision.
	LoadRoom    int32 `yaml:"load_room,omitempty"`
	Description Text  `yaml:"description,omitempty"`
}

type timesDoc struct {
	Created   string `yaml:"created,omitempty"`
	LastLogin string `yaml:"last_login,omitempty"`
	Played    string `yaml:"played,omitempty"`
	LastHost  string `yaml:"last_host,omitempty"`
}

type bodyDoc struct {
	Height int32 `yaml:"height,omitempty"`
	Weight int32 `yaml:"weight,omitempty"`
	Str    int32 `yaml:"str,omitempty"`
	StrAdd int32 `yaml:"str_add,omitempty"`
	Int    int32 `yaml:"int,omitempty"`
	Wis    int32 `yaml:"wis,omitempty"`
	Dex    int32 `yaml:"dex,omitempty"`
	Con    int32 `yaml:"con,omitempty"`
	Cha    int32 `yaml:"cha,omitempty"`
}

type poolDoc struct {
	Current int32 `yaml:"current,omitempty"`
	Max     int32 `yaml:"max,omitempty"`
}

type poolsDoc struct {
	Hit  poolDoc `yaml:"hit"`
	Mana poolDoc `yaml:"mana"`
	Move poolDoc `yaml:"move"`
}

type savesDoc struct {
	Paralyze int32 `yaml:"paralyze,omitempty"`
	Rod      int32 `yaml:"rod,omitempty"`
	Petrify  int32 `yaml:"petrify,omitempty"`
	Breath   int32 `yaml:"breath,omitempty"`
	Spell    int32 `yaml:"spell,omitempty"`
}

type combatDoc struct {
	AC        int32    `yaml:"ac,omitempty"`
	HitRoll   int32    `yaml:"hitroll,omitempty"`
	DamRoll   int32    `yaml:"damroll,omitempty"`
	Alignment int32    `yaml:"alignment,omitempty"`
	Saves     savesDoc `yaml:"saves"`
	Wimpy     int32    `yaml:"wimpy,omitempty"`
}

type wealthDoc struct {
	Gold int32 `yaml:"gold,omitempty"`
	Bank int32 `yaml:"bank,omitempty"`
	Exp  int32 `yaml:"exp,omitempty"`
}

type conditionsDoc struct {
	// -1 (game.PlayerRecord.Conditions' doc comment) means the condition
	// does not apply — how immortals are stored — which is why these are
	// plain ints rather than omitempty: 0 and "absent" are different
	// values and omitempty cannot tell -1 from unset either, so every
	// value is always written.
	Hunger int32 `yaml:"hunger"`
	Thirst int32 `yaml:"thirst"`
	Drunk  int32 `yaml:"drunk"`
}

type playerFlagsDoc struct {
	Act      []string `yaml:"act,omitempty"`
	ActRaw   uint64   `yaml:"act_raw,omitempty"`
	Affected []string `yaml:"affected,omitempty"`
	AffRaw   uint64   `yaml:"affected_raw,omitempty"`
	Prefs    []string `yaml:"prefs,omitempty"`
	PrefsRaw uint64   `yaml:"prefs_raw,omitempty"`
}

type affectDoc struct {
	Spell    string   `yaml:"spell"`
	Duration int32    `yaml:"duration,omitempty"`
	Modifier int32    `yaml:"modifier,omitempty"`
	Location string   `yaml:"location,omitempty"`
	Sets     []string `yaml:"sets,omitempty"`
	SetsRaw  uint64   `yaml:"sets_raw,omitempty"`
}

type aliasDoc struct {
	Name        string `yaml:"name"`
	Replacement string `yaml:"replacement"`
}

// rentDoc is player.RentFile's header — StoredObject.Contains is what folds
// its Objects into Equipment/Inventory instead of a parallel list here.
type rentDoc struct {
	Code       string `yaml:"code"`
	Written    string `yaml:"written,omitempty"`
	CostPerDay int32  `yaml:"cost_per_day,omitempty"`
	// Gold and Bank are what the character had when the file was written.
	// player.RentFile's own doc comment notes the C stores them and never
	// reads them back — kept anyway, for the same reason LegacySpares is:
	// a field the format has always carried is not this package's to drop.
	Gold int32 `yaml:"gold,omitempty"`
	Bank int32 `yaml:"bank,omitempty"`
}

// ObjInstanceDoc is one player.StoredObject: a vnum plus only what differs
// from the prototype, per §8's "object instances are deltas". Exported
// (along with ObjAffectDoc, ObjInstanceDocFrom and StoredObjectFromDoc in
// reader.go/writer.go) so internal/persist/houses/yaml can reuse the same
// schema for a house's crash-saved contents — §9 calls this "the shared
// object-instance schema used by corpses, house crash files and anything
// else that has to persist an object," and a player's inventory is where it
// was built first, not the only place it belongs.
type ObjInstanceDoc struct {
	Vnum          int32            `yaml:"vnum"`
	Values        []int32          `yaml:"values,omitempty"`
	Flags         []string         `yaml:"flags,omitempty"`
	FlagsRaw      uint64           `yaml:"flags_raw,omitempty"`
	Weight        int32            `yaml:"weight,omitempty"`
	Timer         int32            `yaml:"timer,omitempty"`
	PermAffect    []string         `yaml:"perm_affect,omitempty"`
	PermAffectRaw uint64           `yaml:"perm_affect_raw,omitempty"`
	Affects       []ObjAffectDoc   `yaml:"affects,omitempty"`
	Contains      []ObjInstanceDoc `yaml:"contains,omitempty"`
}

type ObjAffectDoc struct {
	Location string `yaml:"location"`
	Modifier int32  `yaml:"modifier,omitempty"`
}
