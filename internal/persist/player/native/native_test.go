// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package native

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/persist/player"
)

// richRecord is a character with every persisted field populated to
// something distinctive, so a round trip that silently zeroes or swaps two
// fields fails a comparison instead of passing by coincidence.
func richRecord() *game.PlayerRecord {
	return &game.PlayerRecord{
		Name:       "Zaphod",
		Credential: game.Credential{Scheme: game.SchemeArgon2id, Hash: "$argon2id$v=19$..."},
		Title:      "the Confused",
		Sex:        game.SexMale,
		Class:      game.ClassWarrior,
		Race:       3,
		Level:      34,
		Hometown:   3001,
		Birth:      time.Date(2001, 11, 3, 21, 14, 7, 0, time.UTC),
		LastLogon:  time.Date(2008, 6, 19, 2, 31, 55, 0, time.UTC),
		Played:     1847293 * time.Second,
		Host:       "136.206.1.2:4000",
		Height:     183,
		Weight:     187,
		Abilities: game.Abilities{
			Strength: 18, StrengthPercentile: 100, Intelligence: 13,
			Wisdom: 11, Dexterity: 16, Constitution: 17, Charisma: 12,
		},
		Points: game.Points{
			Hit: 412, MaxHit: 412, Mana: 100, MaxMana: 100, Move: 82, MaxMove: 96,
			Armor: -47, Gold: 1207, BankGold: 84000, Exp: 4102993, HitRoll: 9, DamRoll: 11,
		},
		Alignment:     350,
		IDNum:         42,
		PlayerFlags:   game.PlayerSiteOK | game.PlayerCryo,
		AffectFlags:   game.AffectSanctuary | game.AffectDetectInvis,
		Preferences:   game.PrefAutoExit | game.PrefColour1 | game.PrefColour2,
		SavingThrows:  [5]int32{-12, -10, -11, -13, -14},
		Skills:        map[int32]int32{game.SkillBash: 85, game.SkillKick: 100},
		Affects:       []game.Affect{{Type: game.SpellSanctuary, Duration: 12, Modifier: 0, Location: 0, Bits: game.AffectSanctuary}},
		Aliases:       []game.Alias{{Name: "gbb", Replacement: "get bread bag"}, {Name: "gac", Replacement: "get all corpse"}},
		Conditions:    [3]int32{21, 20, 0},
		WimpLevel:     40,
		FreezeLevel:   0,
		InvisLevel:    0,
		LoadRoom:      3001,
		BadPasswords:  0,
		SpellsToLearn: 3,
		RemortVector:  1 << game.ClassThief,
	}
}

func TestSaveLoadRoundTripsARichRecord(t *testing.T) {
	s, err := New(player.Config{Dir: t.TempDir()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = s.Close() }()

	want := richRecord()
	if err := s.Save(context.Background(), want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := s.Load(context.Background(), want.Name)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip mismatch:\n got: %+v\nwant: %+v", got, want)
	}
}

func TestSaveLoadRoundTripsAMinimalRecord(t *testing.T) {
	// A fresh character with almost nothing set -- exercises every
	// omitempty path and every -1/0 boundary the rich record does not.
	s, err := New(player.Config{Dir: t.TempDir()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = s.Close() }()

	want := &game.PlayerRecord{Name: "Newbie", Conditions: [3]int32{-1, -1, -1}}
	if err := s.Save(context.Background(), want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := s.Load(context.Background(), "Newbie")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip mismatch:\n got: %+v\nwant: %+v", got, want)
	}
}

func TestExistsAndDelete(t *testing.T) {
	s, err := New(player.Config{Dir: t.TempDir()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()

	if ok, _ := s.Exists(ctx, "Nobody"); ok {
		t.Fatal("Nobody should not exist yet")
	}
	if err := s.Save(ctx, &game.PlayerRecord{Name: "Nobody"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if ok, err := s.Exists(ctx, "Nobody"); !ok || err != nil {
		t.Fatalf("Exists = %v, %v, want true, nil", ok, err)
	}
	if err := s.Delete(ctx, "Nobody"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if ok, _ := s.Exists(ctx, "Nobody"); ok {
		t.Fatal("Nobody should be gone")
	}
	if err := s.Delete(ctx, "Nobody"); err == nil {
		t.Fatal("deleting an absent character should fail")
	}
}

func TestList(t *testing.T) {
	s, err := New(player.Config{Dir: t.TempDir()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()

	for _, rec := range []*game.PlayerRecord{
		{Name: "Alice", IDNum: 1, Level: 10},
		{Name: "Bob", IDNum: 2, Level: 20, PlayerFlags: game.PlayerSiteOK},
		{Name: "Zod", IDNum: 3, Level: 34},
	} {
		if err := s.Save(ctx, rec); err != nil {
			t.Fatalf("Save(%s): %v", rec.Name, err)
		}
	}

	var entries []player.IndexEntry
	for e, err := range s.List(ctx) {
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		entries = append(entries, e)
	}
	if len(entries) != 3 {
		t.Fatalf("List returned %d entries, want 3: %+v", len(entries), entries)
	}
	byName := map[string]player.IndexEntry{}
	for _, e := range entries {
		byName[e.Name] = e
	}
	if byName["Bob"].Level != 20 || !byName["Bob"].Flags.Has(game.PlayerSiteOK) {
		t.Fatalf("Bob's index entry wrong: %+v", byName["Bob"])
	}
}

// nestedRentFile builds a bag containing two items, worn/carried by nobody
// in particular -- a direct test of ObjectStore's own round trip, separate
// from internal/server/rent.go's tree-building (tested there against a
// live character).
func nestedRentFile() *player.RentFile {
	return &player.RentFile{
		Code: player.RentCrash, Written: time.Date(2008, 6, 19, 3, 0, 0, 0, time.UTC),
		Gold: 100, Bank: 200,
		Objects: []player.StoredObject{
			{
				Vnum: 3032, Weight: 5,
				Contains: []player.StoredObject{
					{Vnum: 3009, Weight: 1},
					{Vnum: 3010, Weight: 2, ExtraFlags: game.ItemGlow, Affects: []game.ObjAffect{{Location: 1, Modifier: 3}}},
				},
			},
			{Vnum: 3022, Weight: 3},
		},
	}
}

func TestObjectStoreRoundTripsNesting(t *testing.T) {
	s, err := New(player.Config{Dir: t.TempDir()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()

	if err := s.Save(ctx, &game.PlayerRecord{Name: "Bilbo"}); err != nil {
		t.Fatalf("Save (roster): %v", err)
	}

	want := nestedRentFile()
	if err := s.SaveObjects(ctx, "Bilbo", want); err != nil {
		t.Fatalf("SaveObjects: %v", err)
	}
	got, err := s.LoadObjects(ctx, "Bilbo")
	if err != nil {
		t.Fatalf("LoadObjects: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip mismatch:\n got: %+v\nwant: %+v", got, want)
	}

	// The roster entry survives untouched.
	rec, err := s.Load(ctx, "Bilbo")
	if err != nil || rec.Name != "Bilbo" {
		t.Fatalf("Load after SaveObjects: %+v, %v", rec, err)
	}
}

func TestSaveObjectsWithoutARosterEntryFails(t *testing.T) {
	s, err := New(player.Config{Dir: t.TempDir()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.SaveObjects(context.Background(), "Ghost", nestedRentFile()); err == nil {
		t.Fatal("expected an error saving objects for a character with no roster entry")
	}
}

func TestObjectStoreNotFoundForACharacterWithNothing(t *testing.T) {
	s, err := New(player.Config{Dir: t.TempDir()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()
	if err := s.Save(ctx, &game.PlayerRecord{Name: "Empty"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := s.LoadObjects(ctx, "Empty"); !errors.Is(err, player.ErrNotFound) {
		t.Fatalf("LoadObjects = %v, want ErrNotFound", err)
	}
}

func TestDeleteObjectsClearsInventoryButKeepsRoster(t *testing.T) {
	s, err := New(player.Config{Dir: t.TempDir()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()
	if err := s.Save(ctx, &game.PlayerRecord{Name: "Frodo"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := s.SaveObjects(ctx, "Frodo", nestedRentFile()); err != nil {
		t.Fatalf("SaveObjects: %v", err)
	}
	if err := s.DeleteObjects(ctx, "Frodo"); err != nil {
		t.Fatalf("DeleteObjects: %v", err)
	}
	if _, err := s.LoadObjects(ctx, "Frodo"); !errors.Is(err, player.ErrNotFound) {
		t.Fatalf("LoadObjects after DeleteObjects = %v, want ErrNotFound", err)
	}
	if ok, _ := s.Exists(ctx, "Frodo"); !ok {
		t.Fatal("Frodo's roster entry should still exist")
	}
}

func TestMarkCrashedRewritesOnlyTheHeader(t *testing.T) {
	s, err := New(player.Config{Dir: t.TempDir()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()
	if err := s.Save(ctx, &game.PlayerRecord{Name: "Sam"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	f := nestedRentFile()
	f.Code = player.RentRented
	if err := s.SaveObjects(ctx, "Sam", f); err != nil {
		t.Fatalf("SaveObjects: %v", err)
	}

	at := time.Date(2008, 7, 1, 12, 0, 0, 0, time.UTC)
	if err := s.MarkCrashed(ctx, "Sam", at); err != nil {
		t.Fatalf("MarkCrashed: %v", err)
	}
	got, err := s.LoadObjects(ctx, "Sam")
	if err != nil {
		t.Fatalf("LoadObjects: %v", err)
	}
	if got.Code != player.RentCrash {
		t.Fatalf("Code = %v, want RentCrash", got.Code)
	}
	if !got.Written.Equal(at) {
		t.Fatalf("Written = %v, want %v", got.Written, at)
	}
	if len(got.Objects) != len(f.Objects) {
		t.Fatalf("MarkCrashed lost objects: got %d, want %d", len(got.Objects), len(f.Objects))
	}
}
