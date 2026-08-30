// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package yaml

import (
	"strconv"
	"time"

	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/persist/player"
	worldtext "github.com/gerrowadat/disgracelands/internal/persist/world/yaml"
)

// docFromRecord builds the roster half of a player.yaml document — every
// field of playerDoc except Rent/Inventory, which come from a RentFile
// separately (see applyRentFile) since Store.Save and ObjectStore.SaveObjects
// are called at different times over the same file (yaml.go's
// read-merge-write).
func docFromRecord(rec *game.PlayerRecord) playerDoc {
	// NameOrNumber rather than NameByValue: a value outside the table
	// used to be written as the empty string and read back as 0, so a
	// record with an out-of-range sex or class came back as neither.
	sex := game.NameOrNumber(rec.Sex.Number(), game.YamlSexNames())
	class := game.NameOrNumber(rec.Class.Number(), game.YamlClassNames())
	remort, remortRaw := game.NameBits(rec.RemortVector.Raw(), game.YamlClassNames())
	act, actRaw := game.NameBits(rec.PlayerFlags.Raw(), game.YamlPlayerFlagNames())
	aff, affRaw := game.NameBits(rec.AffectFlags.Raw(), game.YamlAffectFlagNames())
	prefs, prefsRaw := game.NameBits(rec.Preferences.Raw(), game.YamlPreferenceNames())

	doc := playerDoc{
		Schema:     playerSchema,
		ID:         rec.IDNum,
		Name:       rec.Name,
		Credential: credentialString(rec.Credential),
		Identity: identityDoc{
			Title:       rec.Title,
			Sex:         sex,
			Class:       class,
			Race:        rec.Race.Number(),
			Level:       rec.Level,
			Remort:      remort,
			RemortRaw:   remortRaw,
			Home:        int32(rec.Hometown),
			LoadRoom:    int32(rec.LoadRoom),
			Description: Text(worldtext.ToStored(rec.Description)),
		},
		Times: timesDoc{
			Created:   rfc3339OrEmpty(rec.Birth),
			LastLogin: rfc3339OrEmpty(rec.LastLogon),
			Played:    strconv.Itoa(int(rec.Played/time.Second)) + "s",
			LastHost:  rec.Host,
		},
		Body: bodyDoc{
			Height: rec.Height,
			Weight: rec.Weight,
			Str:    rec.Abilities.Strength,
			StrAdd: rec.Abilities.StrengthPercentile,
			Int:    rec.Abilities.Intelligence,
			Wis:    rec.Abilities.Wisdom,
			Dex:    rec.Abilities.Dexterity,
			Con:    rec.Abilities.Constitution,
			Cha:    rec.Abilities.Charisma,
		},
		Pools: poolsDoc{
			Hit:  poolDoc{Current: rec.Points.Hit, Max: rec.Points.MaxHit},
			Mana: poolDoc{Current: rec.Points.Mana, Max: rec.Points.MaxMana},
			Move: poolDoc{Current: rec.Points.Move, Max: rec.Points.MaxMove},
		},
		Combat: combatDoc{
			AC:        rec.Points.Armor,
			HitRoll:   rec.Points.HitRoll,
			DamRoll:   rec.Points.DamRoll,
			Alignment: rec.Alignment,
			Saves:     savesDocFrom(rec.SavingThrows),
			Wimpy:     rec.WimpLevel,
		},
		Wealth: wealthDoc{
			Gold: rec.Points.Gold,
			Bank: rec.Points.BankGold,
			Exp:  rec.Points.Exp,
		},
		Conditions: conditionsDoc{
			Drunk:  rec.Conditions[0],
			Hunger: rec.Conditions[1],
			Thirst: rec.Conditions[2],
		},
		Flags: playerFlagsDoc{
			Act: act, ActRaw: actRaw,
			Affected: aff, AffRaw: affRaw,
			Prefs: prefs, PrefsRaw: prefsRaw,
		},
		PracticeSessions:    rec.SpellsToLearn,
		InvisibilityLevel:   rec.InvisLevel,
		FrozenByLevel:       rec.FreezeLevel,
		BadPasswordAttempts: rec.BadPasswords,
		SpecFlags:           uint64(uint32(rec.SpecFlags)), //nolint:gosec // reinterpretation, not truncation
		OLCZone:             rec.OLCZone,
		Skills:              skillsDocFrom(rec.Skills),
		Affects:             affectsDocFrom(rec.Affects),
		Aliases:             aliasesDocFrom(rec.Aliases),
	}
	return doc
}

func savesDocFrom(saves [5]int32) savesDoc {
	return savesDoc{Paralyze: saves[0], Rod: saves[1], Petrify: saves[2], Breath: saves[3], Spell: saves[4]}
}

func skillsDocFrom(skills map[game.SpellID]int32) map[string]int32 {
	if len(skills) == 0 {
		return nil
	}
	out := make(map[string]int32, len(skills))
	for n, pct := range skills {
		out[game.SpellNameOrNumber(n)] = pct
	}
	return out
}

func affectsDocFrom(affects []game.Affect) []affectDoc {
	if len(affects) == 0 {
		return nil
	}
	out := make([]affectDoc, 0, len(affects))
	for _, a := range affects {
		sets, setsRaw := game.NameBits(a.Bits.Raw(), game.YamlAffectFlagNames())
		location := game.NameOrNumber(a.Location.Number(), game.YamlApplyTypeNames())
		out = append(out, affectDoc{
			Spell: game.SpellNameOrNumber(a.Type), Duration: a.Duration, Modifier: a.Modifier,
			Location: location, Sets: sets, SetsRaw: setsRaw,
		})
	}
	return out
}

func aliasesDocFrom(aliases []game.Alias) []aliasDoc {
	if len(aliases) == 0 {
		return nil
	}
	out := make([]aliasDoc, 0, len(aliases))
	for _, a := range aliases {
		out = append(out, aliasDoc{Name: a.Name, Replacement: a.Replacement})
	}
	return out
}

// credentialString is ascii's own credentialField (ascii/codec.go),
// duplicated rather than shared because it is three lines and the two
// packages otherwise share nothing about how a record is laid out.
func credentialString(c game.Credential) string {
	switch c.Scheme {
	case game.SchemeNone:
		return ""
	case game.SchemeLegacyDES:
		return c.Hash
	default:
		return string(c.Scheme) + ":" + c.Hash
	}
}

func rfc3339OrEmpty(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// applyRentFile overlays f onto doc's Rent/Inventory fields, replacing
// whatever was there — the ObjectStore.SaveObjects half of the
// read-merge-write yaml.go's Store does over one file.
func applyRentFile(doc *playerDoc, f *player.RentFile) {
	doc.Rent = &rentDoc{
		Code:       f.Code.String(),
		Written:    rfc3339OrEmpty(f.Written),
		CostPerDay: f.CostPerDay,
		Gold:       f.Gold,
		Bank:       f.Bank,
	}
	doc.Inventory = make([]ObjInstanceDoc, 0, len(f.Objects))
	for _, obj := range f.Objects {
		doc.Inventory = append(doc.Inventory, ObjInstanceDocFrom(obj))
	}
}

func ObjInstanceDocFrom(st player.StoredObject) ObjInstanceDoc {
	extra, extraRaw := game.NameBits(st.ExtraFlags.Raw(), game.YamlItemExtraFlagNames())
	perm, permRaw := game.NameBits(st.PermAffect.Raw(), game.YamlAffectFlagNames())

	od := ObjInstanceDoc{
		Vnum:   int32(st.Vnum),
		Values: st.Values[:],
		Flags:  extra, FlagsRaw: extraRaw,
		Weight: st.Weight, Timer: st.Timer,
		PermAffect: perm, PermAffectRaw: permRaw,
	}
	for _, a := range st.Affects {
		if a.Location == 0 && a.Modifier == 0 {
			// An empty slot: storedFrom always pads to MaxObjAffects, and
			// the vast majority of those slots are unused on any real item.
			continue
		}
		location := game.NameOrNumber(a.Location.Number(), game.YamlApplyTypeNames())
		od.Affects = append(od.Affects, ObjAffectDoc{Location: location, Modifier: a.Modifier})
	}
	for _, inner := range st.Contains {
		od.Contains = append(od.Contains, ObjInstanceDocFrom(inner))
	}
	return od
}
