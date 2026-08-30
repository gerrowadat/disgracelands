// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package player_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/persist/player"
	"github.com/gerrowadat/disgracelands/internal/persist/player/ascii"
	"github.com/gerrowadat/disgracelands/internal/persist/player/binary"
)

// These tests live above both formats because what they check is the
// relationship between them: that a roster can move from the one the game was
// played on to the one the server runs on without losing anything the game
// cares about.

// fullRecord exercises every field the binary format can hold, at values that
// would catch a field being dropped, swapped or truncated.
func fullRecord(name string) *game.PlayerRecord {
	return &game.PlayerRecord{
		Name:         name,
		Title:        "the Implementor of Everything",
		Description:  "A weathered adventurer.\nScars everywhere.",
		Sex:          1,
		Class:        3,
		Level:        34,
		Hometown:     3001,
		Birth:        time.Unix(1015200000, 0).UTC(),
		LastLogon:    time.Unix(1208649600, 0).UTC(),
		Played:       360000 * time.Second,
		Host:         "redbrick.dcu.ie",
		Credential:   game.Credential{Scheme: game.SchemeLegacyDES, Hash: "abFAKEHASH"},
		Weight:       200,
		Height:       180,
		Abilities:    game.Abilities{Strength: 18, StrengthPercentile: 100, Intelligence: 17, Wisdom: 16, Dexterity: 15, Constitution: 14, Charisma: 13},
		Points:       game.Points{Hit: 500, MaxHit: 600, Mana: 100, MaxMana: 110, Move: 82, MaxMove: 90, Armor: -100, Gold: 12345, BankGold: 67890, Exp: 9999999, HitRoll: 5, DamRoll: 6},
		Alignment:    -750,
		IDNum:        42,
		PlayerFlags:  game.SetFromRaw[game.PlayerFlag](1 << 7),
		AffectFlags:  game.SetFromRaw[game.AffectFlag](1<<3 | 1<<9),
		Preferences:  game.SetFromRaw[game.PrefFlag](1<<4 | 1<<21),
		SavingThrows: [5]int32{-10, -20, -30, -40, -50},
		Skills:       map[int32]int32{1: 100, 2: 85, 200: 42},
		Affects: []game.Affect{
			{Type: 23, Duration: 12, Modifier: 3, Location: 1, Bits: game.SetFromRaw[game.AffectFlag](1 << 5)},
			{Type: 24, Duration: 6, Modifier: -2, Location: 2},
		},
		Conditions:    [3]int32{-1, -1, -1},
		WimpLevel:     50,
		FreezeLevel:   34,
		InvisLevel:    31,
		LoadRoom:      3001,
		BadPasswords:  2,
		SpellsToLearn: 3,
		RemortVector:  0b0101,
	}
}

// compare reports every field that differs, naming it.
func compare(t *testing.T, got, want *game.PlayerRecord) {
	t.Helper()

	for _, tc := range []struct {
		what      string
		got, want any
	}{
		{"Name", got.Name, want.Name},
		{"Title", got.Title, want.Title},
		{"Description", got.Description, want.Description},
		{"Sex", got.Sex, want.Sex},
		{"Class", got.Class, want.Class},
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
}

// TestBinaryToASCIILosesNothing is the migration this phase exists to make
// safe. The remort vector matters here in particular: it is the headline
// local feature and it lives in what upstream calls a spare slot, so it is
// exactly the sort of thing a conversion drops without anyone noticing until
// a character's skills stop working.
func TestBinaryToASCIILosesNothing(t *testing.T) {
	ctx := context.Background()

	src, err := binary.New(player.Config{Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	dst, err := ascii.New(player.Config{Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}

	want := fullRecord("Zod")
	if err := src.Save(ctx, want); err != nil {
		t.Fatalf("saving to the binary format: %v", err)
	}

	loaded, err := src.Load(ctx, "Zod")
	if err != nil {
		t.Fatalf("loading from the binary format: %v", err)
	}
	if err := dst.Save(ctx, loaded); err != nil {
		t.Fatalf("saving to the ascii format: %v", err)
	}

	got, err := dst.Load(ctx, "Zod")
	if err != nil {
		t.Fatalf("loading from the ascii format: %v", err)
	}
	compare(t, got, want)
}

// TestASCIIToBinaryLosesNothing checks the other direction, which conversion
// needs: going back is how you compare a converted roster against the C
// server, and how you undo a migration that turned out to be premature.
func TestASCIIToBinaryLosesNothing(t *testing.T) {
	ctx := context.Background()

	src, err := ascii.New(player.Config{Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	dst, err := binary.New(player.Config{Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}

	want := fullRecord("Zod")
	if err := src.Save(ctx, want); err != nil {
		t.Fatal(err)
	}
	loaded, err := src.Load(ctx, "Zod")
	if err != nil {
		t.Fatal(err)
	}
	if err := dst.Save(ctx, loaded); err != nil {
		t.Fatalf("saving to the binary format: %v", err)
	}
	got, err := dst.Load(ctx, "Zod")
	if err != nil {
		t.Fatal(err)
	}
	compare(t, got, want)
}

// TestTheBinaryFormatCannotHoldAModernCredential is the reason the server
// runs on ascii. It is not a preference: the binary password field is eleven
// bytes, so this is a property of the format rather than a policy.
func TestTheBinaryFormatCannotHoldAModernCredential(t *testing.T) {
	ctx := context.Background()
	modern := game.Credential{
		Scheme: game.SchemeArgon2id,
		Hash:   "$argon2id$v=19$m=65536,t=3,p=4$c29tZXNhbHQ$aGFzaGhhc2hoYXNo",
	}

	rec := fullRecord("Zod")
	rec.Credential = modern

	asc, _ := ascii.New(player.Config{Dir: t.TempDir()})
	if err := asc.Save(ctx, rec); err != nil {
		t.Errorf("the ascii format refused a modern credential: %v", err)
	}

	bin, _ := binary.New(player.Config{Dir: t.TempDir()})
	err := bin.Save(ctx, rec)
	if err == nil {
		t.Fatal("the binary format accepted a modern credential")
	}
	if bin.Capabilities().Supports(game.SchemeArgon2id) {
		t.Error("the binary format claims to support argon2id")
	}
	if !asc.Capabilities().Supports(game.SchemeArgon2id) {
		t.Error("the ascii format does not claim to support argon2id")
	}
}

// TestCapabilitiesDifferInTheWaysThatMatter documents, as an assertion, why
// the migration is worth doing at all.
func TestCapabilitiesDifferInTheWaysThatMatter(t *testing.T) {
	bin, _ := binary.New(player.Config{Dir: t.TempDir()})
	asc, _ := ascii.New(player.Config{Dir: t.TempDir()})

	b, a := bin.Capabilities(), asc.Capabilities()

	if b.MaxNameLength == 0 {
		t.Error("the binary format reports no name limit, but it has one")
	}
	if a.MaxNameLength != 0 {
		t.Error("the ascii format reports a name limit, but it has none")
	}
	if !b.TimestampsOverflowIn2038 {
		t.Error("the binary format's 32-bit timestamps are not reported")
	}
	if a.TimestampsOverflowIn2038 {
		t.Error("the ascii format is reported as overflowing in 2038")
	}
}

// TestARosterConvertsWholesale walks a roster rather than one character,
// which is what the command-line conversion actually does.
func TestARosterConvertsWholesale(t *testing.T) {
	ctx := context.Background()
	src, _ := binary.New(player.Config{Dir: t.TempDir()})
	dst, _ := ascii.New(player.Config{Dir: t.TempDir()})

	names := []string{"Zod", "Welmar", "Aardvark", "Puff"}
	for i, n := range names {
		rec := fullRecord(n)
		rec.IDNum = int64(i + 1)
		rec.Level = int32(20 + i)
		if err := src.Save(ctx, rec); err != nil {
			t.Fatal(err)
		}
	}

	moved := 0
	for entry, err := range src.List(ctx) {
		if err != nil {
			t.Fatal(err)
		}
		rec, err := src.Load(ctx, entry.Name)
		if err != nil {
			t.Fatal(err)
		}
		if err := dst.Save(ctx, rec); err != nil {
			t.Fatal(err)
		}
		moved++
	}
	if moved != len(names) {
		t.Fatalf("converted %d characters, want %d", moved, len(names))
	}

	seen := map[string]bool{}
	for entry, err := range dst.List(ctx) {
		if err != nil {
			t.Fatal(err)
		}
		seen[entry.Name] = true
	}
	for _, n := range names {
		// The index stores lowercased names, matching the filenames.
		if !seen[strings.ToLower(n)] {
			t.Errorf("%s is missing from the converted roster: %v", n, seen)
		}
	}
}
