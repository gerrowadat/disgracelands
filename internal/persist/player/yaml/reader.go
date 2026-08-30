// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package yaml

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/persist/player"
	worldtext "github.com/gerrowadat/disgracelands/internal/persist/world/yaml"
)

// recordFromDoc is docFromRecord's inverse. unknown collects every name
// none of the closed vocabularies recognise — §4.1's "unknown name is an
// error" — so the caller can refuse the load rather than silently drop
// what a hand-edited or future-written file meant.
func recordFromDoc(doc *playerDoc) (*game.PlayerRecord, []string, error) {
	if doc.Name == "" {
		return nil, nil, fmt.Errorf("player file has no name")
	}
	var unknown []string
	note := func(format string, args ...any) { unknown = append(unknown, fmt.Sprintf(format, args...)) }

	sex, ok := game.ValueByNameOrNumber(doc.Identity.Sex, game.YamlSexNames())
	if doc.Identity.Sex != "" && !ok {
		note("identity.sex: unknown value %q", doc.Identity.Sex)
	}
	class, ok := game.ValueByNameOrNumber(doc.Identity.Class, game.YamlClassNames())
	if doc.Identity.Class != "" && !ok {
		note("identity.class: unknown value %q", doc.Identity.Class)
	}
	remort, remortUnknown := game.ParseBitNames(doc.Identity.Remort, game.YamlClassNames())
	for _, name := range remortUnknown {
		note("identity.remort: unknown class name %q", name)
	}
	remort |= doc.Identity.RemortRaw

	created, err := timeFromRFC3339(doc.Times.Created)
	if err != nil {
		note("times.created: %v", err)
	}
	lastLogin, err := timeFromRFC3339(doc.Times.LastLogin)
	if err != nil {
		note("times.last_login: %v", err)
	}
	played, err := durationFromSeconds(doc.Times.Played)
	if err != nil {
		note("times.played: %v", err)
	}

	// The three _raw fields are ORed back in the same way identity.remort
	// and an affect's sets_raw already were. They had been written and
	// never read — a write-only escape hatch, which is worse than not
	// having one, because the file looks like it carries the bits and the
	// reader silently drops them. A player flag above the last *named*
	// bit survived the export and vanished on the way back in.
	act, actUnknown := game.ParseBitNames(doc.Flags.Act, game.YamlPlayerFlagNames())
	for _, name := range actUnknown {
		note("flags.act: unknown name %q", name)
	}
	act |= doc.Flags.ActRaw
	aff, affUnknown := game.ParseBitNames(doc.Flags.Affected, game.YamlAffectFlagNames())
	for _, name := range affUnknown {
		note("flags.affected: unknown name %q", name)
	}
	aff |= doc.Flags.AffRaw
	prefs, prefsUnknown := game.ParseBitNames(doc.Flags.Prefs, game.YamlPreferenceNames())
	for _, name := range prefsUnknown {
		note("flags.prefs: unknown name %q", name)
	}
	prefs |= doc.Flags.PrefsRaw

	skills, skillsUnknown := skillsFromDoc(doc.Skills)
	for _, name := range skillsUnknown {
		note("skills: unknown spell/skill name %q", name)
	}
	affects, affectsUnknown := affectsFromDoc(doc.Affects)
	unknown = append(unknown, affectsUnknown...)

	rec := &game.PlayerRecord{
		Name:        doc.Name,
		Credential:  parseCredentialString(doc.Credential),
		Title:       doc.Identity.Title,
		Description: worldtext.FromStored(string(doc.Identity.Description)),
		Sex:         sex,
		Class:       game.Class(class),
		Race:        doc.Identity.Race,
		Level:       doc.Identity.Level,
		Hometown:    doc.Identity.Home,
		Birth:       created,
		LastLogon:   lastLogin,
		Played:      played,
		Host:        doc.Times.LastHost,
		Height:      doc.Body.Height,
		Weight:      doc.Body.Weight,
		Abilities: game.Abilities{
			Strength: doc.Body.Str, StrengthPercentile: doc.Body.StrAdd,
			Intelligence: doc.Body.Int, Wisdom: doc.Body.Wis, Dexterity: doc.Body.Dex,
			Constitution: doc.Body.Con, Charisma: doc.Body.Cha,
		},
		Points: game.Points{
			Hit: doc.Pools.Hit.Current, MaxHit: doc.Pools.Hit.Max,
			Mana: doc.Pools.Mana.Current, MaxMana: doc.Pools.Mana.Max,
			Move: doc.Pools.Move.Current, MaxMove: doc.Pools.Move.Max,
			Armor: doc.Combat.AC, HitRoll: doc.Combat.HitRoll, DamRoll: doc.Combat.DamRoll,
			Gold: doc.Wealth.Gold, BankGold: doc.Wealth.Bank, Exp: doc.Wealth.Exp,
		},
		Alignment:     doc.Combat.Alignment,
		SavingThrows:  [5]int32{doc.Combat.Saves.Paralyze, doc.Combat.Saves.Rod, doc.Combat.Saves.Petrify, doc.Combat.Saves.Breath, doc.Combat.Saves.Spell},
		WimpLevel:     doc.Combat.Wimpy,
		IDNum:         doc.ID,
		PlayerFlags:   game.SetFromRaw[game.PlayerFlag](act),
		AffectFlags:   game.SetFromRaw[game.AffectFlag](aff),
		Preferences:   game.SetFromRaw[game.PrefFlag](prefs),
		Skills:        skills,
		Affects:       affects,
		Aliases:       aliasesFromDoc(doc.Aliases),
		Conditions:    [3]int32{doc.Conditions.Drunk, doc.Conditions.Hunger, doc.Conditions.Thirst},
		InvisLevel:    doc.InvisibilityLevel,
		FreezeLevel:   doc.FrozenByLevel,
		LoadRoom:      game.RoomVnum(doc.Identity.LoadRoom),
		BadPasswords:  doc.BadPasswordAttempts,
		SpellsToLearn: doc.PracticeSessions,
		RemortVector:  game.SetFromRaw[game.Class](remort),
		SpecFlags:     int32(doc.SpecFlags), //nolint:gosec // a local bitmask, 32 bits wide in the format it came from
		OLCZone:       doc.OLCZone,
	}
	// The document holds the unaffected figures — what game.BaseRecord
	// wrote — so they are this record's base, and the doc's own `affects:`
	// apply on top of them. See game.SnapshotReal.
	game.SnapshotReal(rec)
	return rec, unknown, nil
}

func timeFromRFC3339(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, err
	}
	return t.UTC(), nil
}

func durationFromSeconds(s string) (time.Duration, error) {
	rest, ok := strings.CutSuffix(s, "s")
	if !ok {
		if s == "" {
			return 0, nil
		}
		return 0, fmt.Errorf("%q is not an integer number of seconds", s)
	}
	n, err := strconv.Atoi(rest)
	if err != nil {
		return 0, fmt.Errorf("%q is not an integer number of seconds", s)
	}
	return time.Duration(n) * time.Second, nil
}

func skillsFromDoc(doc map[string]int32) (map[int32]int32, []string) {
	if len(doc) == 0 {
		return nil, nil
	}
	var unknown []string
	skills := make(map[int32]int32, len(doc))
	for name, pct := range doc {
		n, ok := game.SpellNumberFromNameOrNumber(name)
		if !ok {
			unknown = append(unknown, name)
			continue
		}
		skills[n] = pct
	}
	return skills, unknown
}

func affectsFromDoc(doc []affectDoc) ([]game.Affect, []string) {
	if len(doc) == 0 {
		return nil, nil
	}
	var unknown []string
	affects := make([]game.Affect, 0, len(doc))
	for _, ad := range doc {
		spell, ok := game.SpellNumberFromNameOrNumber(ad.Spell)
		if !ok {
			unknown = append(unknown, fmt.Sprintf("affects: unknown spell/skill name %q", ad.Spell))
		}
		location, ok := game.ValueByNameOrNumber(ad.Location, game.YamlApplyTypeNames())
		if ad.Location != "" && !ok {
			unknown = append(unknown, fmt.Sprintf("affects: unknown location %q", ad.Location))
		}
		bits, bitsUnknown := game.ParseBitNames(ad.Sets, game.YamlAffectFlagNames())
		for _, name := range bitsUnknown {
			unknown = append(unknown, fmt.Sprintf("affects: unknown flag name %q", name))
		}
		bits |= ad.SetsRaw
		affects = append(affects, game.Affect{
			Type: spell, Duration: ad.Duration, Modifier: ad.Modifier, Location: location, Bits: game.SetFromRaw[game.AffectFlag](bits),
		})
	}
	return affects, unknown
}

func aliasesFromDoc(doc []aliasDoc) []game.Alias {
	if len(doc) == 0 {
		return nil
	}
	out := make([]game.Alias, 0, len(doc))
	for _, ad := range doc {
		out = append(out, game.Alias{Name: ad.Name, Replacement: ad.Replacement})
	}
	return out
}

// parseCredentialString is credentialString's inverse — the same rule
// ascii/codec.go's parseCredential uses: "scheme:hash" is a modern
// credential, and a bare value with no colon is the legacy DES hash by
// definition, since a DES crypt(3) hash can never contain one.
func parseCredentialString(v string) game.Credential {
	if v == "" {
		return game.Credential{}
	}
	if scheme, hash, found := strings.Cut(v, ":"); found {
		return game.Credential{Scheme: game.CredentialScheme(scheme), Hash: hash}
	}
	return game.Credential{Scheme: game.SchemeLegacyDES, Hash: v}
}

// rentFileFromDoc is applyRentFile's inverse. ok is false when doc has no
// Rent section at all — a character who has never left the game carrying
// anything, which ObjectStore.LoadObjects reports as player.ErrNotFound.
func rentFileFromDoc(doc *playerDoc) (f *player.RentFile, unknown []string, ok bool) {
	if doc.Rent == nil {
		return nil, nil, false
	}
	written, err := timeFromRFC3339(doc.Rent.Written)
	if err != nil {
		unknown = append(unknown, fmt.Sprintf("rent.written: %v", err))
	}
	f = &player.RentFile{
		Code:       rentCodeByName(doc.Rent.Code),
		Written:    written,
		CostPerDay: doc.Rent.CostPerDay,
		Gold:       doc.Rent.Gold,
		Bank:       doc.Rent.Bank,
	}
	for _, od := range doc.Inventory {
		st, storedUnknown := StoredObjectFromDoc(od)
		unknown = append(unknown, storedUnknown...)
		f.Objects = append(f.Objects, st)
	}
	return f, unknown, true
}

func StoredObjectFromDoc(od ObjInstanceDoc) (player.StoredObject, []string) {
	var unknown []string
	extra, extraUnknown := game.ParseBitNames(od.Flags, game.YamlItemExtraFlagNames())
	for _, name := range extraUnknown {
		unknown = append(unknown, fmt.Sprintf("inventory: object #%d: unknown flag name %q", od.Vnum, name))
	}
	extra |= od.FlagsRaw

	perm, permUnknown := game.ParseBitNames(od.PermAffect, game.YamlAffectFlagNames())
	for _, name := range permUnknown {
		unknown = append(unknown, fmt.Sprintf("inventory: object #%d: unknown perm_affect name %q", od.Vnum, name))
	}
	perm |= od.PermAffectRaw

	st := player.StoredObject{
		Vnum: game.ObjVnum(od.Vnum), Weight: od.Weight, Timer: od.Timer,
		ExtraFlags: game.SetFromRaw[game.ExtraFlag](extra), PermAffect: game.SetFromRaw[game.AffectFlag](perm),
	}
	copy(st.Values[:], od.Values)

	for _, ad := range od.Affects {
		location, ok := game.ValueByNameOrNumber(ad.Location, game.YamlApplyTypeNames())
		if ad.Location != "" && !ok {
			unknown = append(unknown, fmt.Sprintf("inventory: object #%d: unknown affect location %q", od.Vnum, ad.Location))
		}
		st.Affects = append(st.Affects, game.ObjAffect{Location: location, Modifier: ad.Modifier})
	}
	// Pad back to the fixed slot count. The writer drops empty slots
	// because storing five zeroes per object to say nothing is not worth
	// the bytes, but player.StoredObject.Affects is documented as holding
	// exactly MaxObjAffects "including the empty ones", and binary's
	// decoder always produces that many. Two drivers filling the same
	// shared model differently is how a comparison between them (`dlctl
	// verify --against`) reports a difference that is not one — and, worse,
	// how a difference that *is* one hides among them.
	//
	// Padding rather than trimming, because the slot count is the C's:
	// Obj_from_store copies affected[j] by index for all six.
	for len(st.Affects) < game.MaxObjAffects {
		st.Affects = append(st.Affects, game.ObjAffect{})
	}
	for _, inner := range od.Contains {
		innerSt, innerUnknown := StoredObjectFromDoc(inner)
		unknown = append(unknown, innerUnknown...)
		st.Contains = append(st.Contains, innerSt)
	}
	return st, unknown
}

// rentCodeByName is player.RentCode.String()'s inverse.
func rentCodeByName(s string) player.RentCode {
	switch s {
	case "crash":
		return player.RentCrash
	case "rented":
		return player.RentRented
	case "cryo":
		return player.RentCryo
	case "forced":
		return player.RentForced
	case "timed out":
		return player.RentTimedOut
	}
	return player.RentUndef
}
