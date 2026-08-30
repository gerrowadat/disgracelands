// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.
//
// The format implemented here is the public ascii_pfiles 2.1 patch by Alan K.
// Miles, building on an original by Chris Jacobson.

package ascii

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// realFile is a genuine ascii pfile, from
// docs/investigations/ascii-pfile-format.md's worked example — which was
// taken from welmar/WipeMud/lib/pfiles/z/zod with the password replaced.
// Parsing it is the check that this package reads what the format's own
// implementation wrote, rather than only what this package writes.
const realFile = `Name: Zod
Pass: abFAKEHASH
Titl: the Implementor
Sex : 1
Clas: 1
Levl: 54
Home: 3001
Brth: 1043363000
Plyd: 123456
Last: 1043364802
Host: 136.206.15.10:4444
Hite: 180
Wate: 200
Str : 18/100
Int : 17
Wis : 16
Dex : 15
Con : 14
Cha : 13
Hit : 500/500
Mana: 100/100
Move: 82/82
Ac  : -100
Gold: 12345
Bank: 67890
Exp : 9999999
Hrol: 5
Drol: 6
Alin: 1000
Id  : 2
Act : 128
Pref: efghmnoqv
Thr1: -10
Thr5: -50
Wimp: 50
Lern: 3
Desc:
A tall figure stands here.
~
Skil:
1 100
2 85
0 0
Affs:
23 12 0 0 0
0 0 0 0 0
`

func TestDecodeARealFile(t *testing.T) {
	rec, unknown, err := Decode(strings.NewReader(realFile))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(unknown) > 0 {
		t.Errorf("unrecognised fields in a genuine file: %v", unknown)
	}

	// The %T is not decoration. This helper compares through `any`, so a
	// `got` and a `want` of different *types* are unequal however they
	// print — and while docs/proposals/idiomatic-go.md's step 2 gives the
	// enumerations their own types, that is exactly what keeps happening.
	// Three separate conversions have failed here with messages of the form
	// "Class = 1, want 1", which reads as a value mismatch and is not one.
	check := func(what string, got, want any) {
		if got != want {
			t.Errorf("%s = %v (%T), want %v (%T)", what, got, got, want, want)
		}
	}
	check("Name", rec.Name, "Zod")
	check("Title", rec.Title, "the Implementor")
	check("Sex", rec.Sex, game.SexMale)
	check("Class", rec.Class, game.ClassCleric)
	check("Level", rec.Level, int32(54))
	check("Hometown", rec.Hometown, int32(3001))
	check("Host", rec.Host, "136.206.15.10:4444")
	check("Height", rec.Height, int32(180))
	check("Weight", rec.Weight, int32(200))
	check("Strength", rec.Abilities.Strength, int32(18))
	check("StrengthPercentile", rec.Abilities.StrengthPercentile, int32(100))
	check("Hit", rec.Points.Hit, int32(500))
	check("MaxHit", rec.Points.MaxHit, int32(500))
	check("Move", rec.Points.Move, int32(82))
	check("Armor", rec.Points.Armor, int32(-100))
	check("Gold", rec.Points.Gold, int32(12345))
	check("BankGold", rec.Points.BankGold, int32(67890))
	check("Alignment", rec.Alignment, int32(1000))
	check("IDNum", rec.IDNum, int64(2))
	check("WimpLevel", rec.WimpLevel, int32(50))
	check("SpellsToLearn", rec.SpellsToLearn, int32(3))
	check("Birth", rec.Birth, time.Unix(1043363000, 0).UTC())
	check("Played", rec.Played, 123456*time.Second)
	check("Credential", rec.Credential, game.Credential{Scheme: game.SchemeLegacyDES, Hash: "abFAKEHASH"})

	// "Act : 128" is the decimal form; the reader accepts it and the letter
	// form alike, which is the branch the format document calls load-bearing.
	check("PlayerFlags", rec.PlayerFlags, game.SetFromRaw[game.PlayerFlag](128))

	// "efghmnoqv" is bits 4,5,6,7,12,13,14,16,21 per the format document.
	var wantPrefBits uint64
	for _, bit := range []uint{4, 5, 6, 7, 12, 13, 14, 16, 21} {
		wantPrefBits |= 1 << bit
	}
	wantPref := game.SetFromRaw[game.PrefFlag](wantPrefBits)
	check("Preferences", rec.Preferences, wantPref)

	// Saving throws are one tag each, and absent ones stay zero.
	check("Thr1", rec.SavingThrows[0], int32(-10))
	check("Thr2", rec.SavingThrows[1], int32(0))
	check("Thr5", rec.SavingThrows[4], int32(-50))

	check("Description", rec.Description, "A tall figure stands here.")
	check("skills", len(rec.Skills), 2)
	check("skill 1", rec.Skills[1], int32(100))
	check("skill 2", rec.Skills[2], int32(85))

	if len(rec.Affects) != 1 {
		t.Fatalf("got %d affects, want 1", len(rec.Affects))
	}
	check("affect type", rec.Affects[0].Type, int32(23))
	check("affect duration", rec.Affects[0].Duration, int32(12))
}

func TestBothFlagEncodingsAreAccepted(t *testing.T) {
	// The reader takes the letter form or a plain decimal number, and the
	// two must mean the same thing. Bit 7 is "h" or 128.
	for _, form := range []string{"h", "128"} {
		rec, _, err := Decode(strings.NewReader("Name: X\nAct : " + form + "\n"))
		if err != nil {
			t.Fatalf("Decode with Act=%q: %v", form, err)
		}
		if rec.PlayerFlags.Raw() != 128 {
			t.Errorf("Act %q decoded to %d, want 128", form, rec.PlayerFlags.Raw())
		}
	}
}

func TestRoundTrip(t *testing.T) {
	want := &game.PlayerRecord{
		Name:         "Zod",
		Title:        "the Implementor",
		Description:  "A tall figure stands here.\nWearing a hat.",
		Sex:          1,
		Class:        3,
		Race:         2,
		Level:        54,
		Hometown:     3001,
		Birth:        time.Unix(1043363000, 0).UTC(),
		LastLogon:    time.Unix(1208649600, 0).UTC(),
		Played:       123456 * time.Second,
		Host:         "redbrick.dcu.ie",
		Credential:   game.Credential{Scheme: game.SchemeLegacyDES, Hash: "abFAKEHASH"},
		Weight:       200,
		Height:       180,
		Abilities:    game.Abilities{Strength: 18, StrengthPercentile: 100, Intelligence: 17, Wisdom: 16, Dexterity: 15, Constitution: 14, Charisma: 13},
		Points:       game.Points{Hit: 500, MaxHit: 600, Mana: 100, MaxMana: 110, Move: 82, MaxMove: 90, Armor: -100, Gold: 12345, BankGold: 67890, Exp: 9999999, HitRoll: 5, DamRoll: 6},
		Alignment:    1000,
		IDNum:        2,
		PlayerFlags:  game.SetFromRaw[game.PlayerFlag](128),
		AffectFlags:  game.SetFromRaw[game.AffectFlag](1 << 3),
		Preferences:  game.SetFromRaw[game.PrefFlag](1<<4 | 1<<21),
		SavingThrows: [5]int32{-10, -20, -30, -40, -50},
		Skills:       map[int32]int32{1: 100, 2: 85, 200: 42},
		Affects: []game.Affect{
			{Type: 23, Duration: 12, Modifier: 3, Location: 1, Bits: game.SetFromRaw[game.AffectFlag](1 << 5)},
			{Type: 24, Duration: 6, Modifier: -2, Location: 2},
		},
		Aliases: []game.Alias{
			{Name: "gbb", Replacement: "get bread bag"},
			{Name: "mm", Replacement: "cast 'magic missile'"},
		},
		Conditions:    [3]int32{-1, -1, -1},
		WimpLevel:     50,
		FreezeLevel:   34,
		InvisLevel:    31,
		LoadRoom:      3001,
		BadPasswords:  2,
		SpellsToLearn: 3,
		RemortVector:  game.SetFromRaw[game.Class](5),
	}

	var buf strings.Builder
	if err := Encode(&buf, want); err != nil {
		t.Fatalf("Encode: %v", err)
	}
	got, unknown, err := Decode(strings.NewReader(buf.String()))
	if err != nil {
		t.Fatalf("Decode: %v\n%s", err, buf.String())
	}
	if len(unknown) > 0 {
		t.Errorf("our own output has unrecognised fields: %v", unknown)
	}

	// Compared field by field rather than with reflect.DeepEqual, so a
	// failure names the field.
	for _, tc := range []struct {
		what      string
		got, want any
	}{
		{"Name", got.Name, want.Name},
		{"Title", got.Title, want.Title},
		{"Description", got.Description, want.Description},
		{"Sex", got.Sex, want.Sex},
		{"Class", got.Class, want.Class},
		{"Race", got.Race, want.Race},
		{"Level", got.Level, want.Level},
		{"Hometown", got.Hometown, want.Hometown},
		{"Birth", got.Birth, want.Birth},
		{"LastLogon", got.LastLogon, want.LastLogon},
		{"Played", got.Played, want.Played},
		{"Host", got.Host, want.Host},
		{"Credential", got.Credential, want.Credential},
		{"Weight", got.Weight, want.Weight},
		{"Height", got.Height, want.Height},
		{"Abilities", got.Abilities, want.Abilities},
		{"Points", got.Points, want.Points},
		{"Alignment", got.Alignment, want.Alignment},
		{"IDNum", got.IDNum, want.IDNum},
		{"PlayerFlags", got.PlayerFlags, want.PlayerFlags},
		{"AffectFlags", got.AffectFlags, want.AffectFlags},
		{"Preferences", got.Preferences, want.Preferences},
		{"SavingThrows", got.SavingThrows, want.SavingThrows},
		{"Conditions", got.Conditions, want.Conditions},
		{"WimpLevel", got.WimpLevel, want.WimpLevel},
		{"FreezeLevel", got.FreezeLevel, want.FreezeLevel},
		{"InvisLevel", got.InvisLevel, want.InvisLevel},
		{"LoadRoom", got.LoadRoom, want.LoadRoom},
		{"BadPasswords", got.BadPasswords, want.BadPasswords},
		{"SpellsToLearn", got.SpellsToLearn, want.SpellsToLearn},
		{"RemortVector", got.RemortVector, want.RemortVector},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %v, want %v", tc.what, tc.got, tc.want)
		}
	}

	if len(got.Skills) != len(want.Skills) {
		t.Errorf("skills = %v, want %v", got.Skills, want.Skills)
	}
	for k, v := range want.Skills {
		if got.Skills[k] != v {
			t.Errorf("skill %d = %d, want %d", k, got.Skills[k], v)
		}
	}
	if len(got.Affects) != len(want.Affects) {
		t.Fatalf("affects = %v, want %v", got.Affects, want.Affects)
	}
	for i := range want.Affects {
		if got.Affects[i] != want.Affects[i] {
			t.Errorf("affect %d = %+v, want %+v", i, got.Affects[i], want.Affects[i])
		}
	}
	if !reflect.DeepEqual(got.Aliases, want.Aliases) {
		t.Errorf("aliases = %+v, want %+v", got.Aliases, want.Aliases)
	}
}

func TestEncodeFollowsTheFormatsConventions(t *testing.T) {
	rec := &game.PlayerRecord{
		Name: "Zod", IDNum: 2, Level: 30,
		Credential: game.Credential{Scheme: game.SchemeLegacyDES, Hash: "abFAKEHASH"},
	}
	var buf strings.Builder
	if err := Encode(&buf, rec); err != nil {
		t.Fatal(err)
	}
	out := buf.String()

	// Tags are exactly four characters, space-padded. The reference reader
	// takes a fixed-width slice before comparing, so "Id: 2" would not parse.
	for _, want := range []string{"Name: Zod\n", "Id  : 2\n", "Levl: 30\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("output is missing %q:\n%s", want, out)
		}
	}

	// Six fields are written whatever their value; the rest are omitted at
	// their default, which is what keeps a fresh character's file short.
	for _, want := range []string{"Name:", "Pass:", "Brth:", "Plyd:", "Last:", "Id  :"} {
		if !strings.Contains(out, want) {
			t.Errorf("always-written field %q is missing:\n%s", want, out)
		}
	}
	for _, unwanted := range []string{"Sex :", "Clas:", "Gold:", "Ac  :", "Wimp:", "Desc:", "Skil:", "Affs:"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("field %q was written at its default value:\n%s", unwanted, out)
		}
	}
}

func TestEncodeIsDeterministic(t *testing.T) {
	// Skills live in a map. Ranging it directly would make every save a
	// different file, and every backup a spurious diff.
	rec := &game.PlayerRecord{
		Name: "Zod", IDNum: 1,
		Skills: map[int32]int32{5: 50, 1: 100, 200: 42, 3: 75, 99: 10},
	}
	var first strings.Builder
	if err := Encode(&first, rec); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		var again strings.Builder
		if err := Encode(&again, rec); err != nil {
			t.Fatal(err)
		}
		if again.String() != first.String() {
			t.Fatalf("two encodings of the same record differ:\n%s\n---\n%s", first.String(), again.String())
		}
	}
	if !strings.Contains(first.String(), "1 100\n3 75\n5 50\n99 10\n200 42\n0 0\n") {
		t.Errorf("skills are not in numeric order:\n%s", first.String())
	}
}

func TestModernCredentialRoundTrips(t *testing.T) {
	// This is the point of moving off the binary format: its password field
	// is eleven bytes and cannot hold this at all.
	want := game.Credential{
		Scheme: game.SchemeArgon2id,
		Hash:   "$argon2id$v=19$m=65536,t=3,p=4$c29tZXNhbHQ$aGFzaGhhc2hoYXNo",
	}
	rec := &game.PlayerRecord{Name: "Zod", IDNum: 1, Credential: want}

	var buf strings.Builder
	if err := Encode(&buf, rec); err != nil {
		t.Fatal(err)
	}
	got, _, err := Decode(strings.NewReader(buf.String()))
	if err != nil {
		t.Fatal(err)
	}
	if got.Credential != want {
		t.Errorf("credential = %+v, want %+v", got.Credential, want)
	}
}

func TestBareHashIsTreatedAsLegacy(t *testing.T) {
	// A password with no scheme prefix is a DES hash by definition: that is
	// all the format ever held before this. A DES hash cannot contain a
	// colon, so this cannot misclassify one.
	rec, _, err := Decode(strings.NewReader("Name: X\nPass: abCDEFGHIJK\n"))
	if err != nil {
		t.Fatal(err)
	}
	if rec.Credential.Scheme != game.SchemeLegacyDES {
		t.Errorf("scheme = %q, want %q", rec.Credential.Scheme, game.SchemeLegacyDES)
	}
	if !rec.Credential.NeedsUpgrade() {
		t.Error("a legacy credential does not report that it needs upgrading")
	}
}

func TestUnknownTagsAreReportedNotDiscarded(t *testing.T) {
	// The format has no comment syntax and the reference reader logs and
	// skips what it does not recognise. Skipping silently would lose data
	// another server wrote.
	rec, unknown, err := Decode(strings.NewReader("Name: X\nQuux: 42\nLevl: 3\n"))
	if err != nil {
		t.Fatalf("an unknown tag made the file unreadable: %v", err)
	}
	if rec.Level != 3 {
		t.Error("a field after the unknown tag was not read")
	}
	if len(unknown) != 1 || !strings.Contains(unknown[0], "Quux") {
		t.Errorf("unknown = %v, want one entry naming Quux", unknown)
	}
}

func TestDecodeRequiresAName(t *testing.T) {
	if _, _, err := Decode(strings.NewReader("Levl: 30\n")); err == nil {
		t.Error("a file with no Name was accepted")
	}
}

func TestMultiLineDescription(t *testing.T) {
	const in = "Name: X\nDesc:\nline one\nline two\n~\nLevl: 5\n"
	rec, _, err := Decode(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if rec.Description != "line one\nline two" {
		t.Errorf("Description = %q", rec.Description)
	}
	// Parsing must resume after the block's terminator.
	if rec.Level != 5 {
		t.Errorf("Level = %d, want 5; the field after the block was missed", rec.Level)
	}
}

func TestZeroSkillsAreDropped(t *testing.T) {
	// A skill at zero percent is not known, and the binary format cannot
	// express the difference, so keeping it would make the two formats
	// disagree about what a character knows.
	rec, _, err := Decode(strings.NewReader("Name: X\nSkil:\n1 100\n2 0\n0 0\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(rec.Skills) != 1 || rec.Skills[1] != 100 {
		t.Errorf("skills = %v, want only skill 1 at 100", rec.Skills)
	}
}

func TestZeroTimesAreNeverNot1970(t *testing.T) {
	// A character who has never logged in has 0 here, and turning that into
	// a 1970 date would put fictional history into the record.
	rec, _, err := Decode(strings.NewReader("Name: X\nBrth: 0\nLast: 0\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !rec.Birth.IsZero() || !rec.LastLogon.IsZero() {
		t.Errorf("Birth = %v, LastLogon = %v; want both zero", rec.Birth, rec.LastLogon)
	}
}

func TestTimestampsSurvive2038(t *testing.T) {
	// The whole reason this format is what the server runs on: its times are
	// decimal text, not 32-bit seconds.
	future := time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)
	rec := &game.PlayerRecord{Name: "X", IDNum: 1, Birth: future, LastLogon: future}

	var buf strings.Builder
	if err := Encode(&buf, rec); err != nil {
		t.Fatal(err)
	}
	got, _, err := Decode(strings.NewReader(buf.String()))
	if err != nil {
		t.Fatal(err)
	}
	if !got.Birth.Equal(future) {
		t.Errorf("Birth = %v, want %v", got.Birth, future)
	}
}
