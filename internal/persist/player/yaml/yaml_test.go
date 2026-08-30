// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package yaml

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
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
	r := &game.PlayerRecord{
		Name:       "Zaphod",
		Credential: game.Credential{Scheme: game.SchemeArgon2id, Hash: "$argon2id$v=19$..."},
		Title:      "the Confused",
		// CRLF because that is what a description looks like in memory:
		// the binary pfile stores the line endings the C wrote, and this
		// format converts at its own boundary rather than normalising the
		// record. richRecord carries one so that every round-trip test
		// here covers the field -- before this, none of them did, and a
		// description was the one thing that made a written file
		// unparseable.
		Description: "A tall man with a scar across one cheek.\r\nHe watches you carefully.\r\n",
		Sex:         game.SexMale,
		Class:       game.ClassWarrior,
		Race:        3,
		Level:       34,
		Hometown:    3001,
		Birth:       time.Date(2001, 11, 3, 21, 14, 7, 0, time.UTC),
		LastLogon:   time.Date(2008, 6, 19, 2, 31, 55, 0, time.UTC),
		Played:      1847293 * time.Second,
		Host:        "136.206.1.2:4000",
		Height:      183,
		Weight:      187,
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
	// A record that has been through a decoder has its unaffected figures
	// snapshotted, and a round trip is only a round trip against one that
	// looks the same. See game.SnapshotReal.
	game.SnapshotReal(r)
	return r
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
	// Lower case: player.IndexEntry.Name is the C's own player_table name,
	// which boot_db lowercases as it builds it (db.c:607).
	if byName["bob"].Level != 20 || !byName["bob"].Flags.Has(game.PlayerSiteOK) {
		t.Fatalf("Bob's index entry wrong: %+v", byName["bob"])
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
				Vnum: 3032, Weight: 5, Affects: affectSlots(),
				Contains: []player.StoredObject{
					{Vnum: 3009, Weight: 1, Affects: affectSlots()},
					{Vnum: 3010, Weight: 2, ExtraFlags: game.ItemGlow,
						Affects: affectSlots(game.ObjAffect{Location: 1, Modifier: 3})},
				},
			},
			{Vnum: 3022, Weight: 3, Affects: affectSlots()},
		},
	}
}

// affectSlots is player.StoredObject.Affects' documented shape: exactly
// game.MaxObjAffects slots, "including the empty ones", which is what
// struct obj_file_elem's fixed affected[MAX_OBJ_AFFECT] array is and what
// binary's decoder always produces.
//
// This fixture used to leave the empty slots out, which read fine and made
// this package's own round trip pass while quietly disagreeing with the
// other driver about the shape of a shared model. `dlctl verify --against`
// is what noticed, by comparing a binary rent file to the yaml it had just
// been converted into and reporting "6 element(s) vs 2" on every object.
func affectSlots(set ...game.ObjAffect) []game.ObjAffect {
	out := make([]game.ObjAffect, game.MaxObjAffects)
	copy(out, set)
	return out
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

// TestSaveLoadRoundTripsDescriptions walks the shapes a description
// actually takes, because the field is free text a player typed into the
// string editor and every one of these reaches a different branch of the
// block-scalar writer.
//
// The regression this guards is not a wrong value but an unparseable file:
// Save marshals and writes without reading back, so a description emitted
// at the wrong indentation produced a file that reported success on the
// way out and failed on the way in. Load is what makes that visible, which
// is why every case here is a round trip rather than a golden string.
func TestSaveLoadRoundTripsDescriptions(t *testing.T) {
	for _, tc := range []struct{ name, desc string }{
		{"single line", "A short, stout figure.\r\n"},
		{"several lines", "A short, stout figure.\r\nOne eye is missing.\r\n"},
		{"indented first line", "   An indented opening line.\r\nAnd a second.\r\n"},
		{"tab indented", "\tName: nobody\r\n\tRank: none\r\n"},
		{"blank line between", "One paragraph.\r\n\r\nAnother paragraph.\r\n"},
		{"no trailing newline", "no trailing newline at all"},
		{"colour markup", "{{red}}A figure wreathed in flame.{{/}}\r\n"},
		{"empty", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, err := New(player.Config{Dir: t.TempDir()})
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			defer func() { _ = s.Close() }()

			want := &game.PlayerRecord{
				Name: "Zaphod", Conditions: [3]int32{-1, -1, -1}, Description: tc.desc,
			}
			if err := s.Save(context.Background(), want); err != nil {
				t.Fatalf("Save: %v", err)
			}
			got, err := s.Load(context.Background(), "Zaphod")
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if got.Description != tc.desc {
				t.Fatalf("description round trip mismatch:\n got: %q\nwant: %q",
					got.Description, tc.desc)
			}
		})
	}
}

// TestSavedDescriptionHoldsNoCarriageReturns pins the other half of the
// contract the round trip above cannot see. YAML normalises every
// line-break style to '\n' when it decodes, so a CRLF description has to
// be converted on the way in and back on the way out; storing the CR
// would make the file's meaning depend on a parser detail. This is the
// same ToStored/FromStored boundary the world format applies to a room
// description, applied at the same place for the same reason.
func TestSavedDescriptionHoldsNoCarriageReturns(t *testing.T) {
	dir := t.TempDir()
	s, err := New(player.Config{Dir: dir})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = s.Close() }()

	rec := &game.PlayerRecord{
		Name: "Zaphod", Conditions: [3]int32{-1, -1, -1},
		Description: "First line.\r\nSecond line.\r\n",
	}
	if err := s.Save(context.Background(), rec); err != nil {
		t.Fatalf("Save: %v", err)
	}

	b, err := os.ReadFile(filepath.Join(dir, "z", "zaphod.yaml"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if bytes.ContainsRune(b, '\r') {
		t.Fatalf("saved file holds a carriage return:\n%s", b)
	}
}

// TestBareLineFeedComesBackAsCRLF pins the one text transform
// docs/design/yaml-only.md §4.2 calls genuinely unavoidable and
// genuinely not lossy, so that it cannot change without somebody
// deciding to.
//
// YAML cannot represent CRLF distinctly from LF — the spec folds CR, CRLF
// and LF alike on decode — so the yaml formats store LF and re-derive
// CRLF on load (worldtext.ToStored/FromStored). For everything the game
// itself wrote that is exact, because every such string is CRLF-joined in
// memory and LF-joined on disk already, which is precisely the
// relationship classic's own bytes have to their own in-memory form. A
// string holding a *bare* LF is the exception: it comes back with a
// carriage return in front of it.
//
// §4.2's own conclusion is that this should be "reclassified rather than
// fixed", with a test pinning it and a line in docs/deviations.md saying
// it is settled. This is that test. It is also why
// cmd/dlctl's FuzzBinaryRecordRoundTrip skips an input whose free text
// holds a bare LF: that target asserts an exact round trip, and this is
// the one place the round trip is deliberately not exact.
func TestBareLineFeedComesBackAsCRLF(t *testing.T) {
	s, err := New(player.Config{Dir: t.TempDir()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()

	const bare = "one line\ntwo line\n"
	if err := s.Save(ctx, &game.PlayerRecord{Name: "Bilbo", Description: bare}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := s.Load(ctx, "Bilbo")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	const want = "one line\r\ntwo line\r\n"
	if got.Description != want {
		t.Errorf("a description of %q came back as %q, want %q", bare, got.Description, want)
	}

	// And the ordinary case, which is what every string the game writes
	// actually looks like: CRLF in, CRLF out, unchanged.
	if err := s.Save(ctx, &game.PlayerRecord{Name: "Frodo", Description: want}); err != nil {
		t.Fatalf("Save (CRLF): %v", err)
	}
	back, err := s.Load(ctx, "Frodo")
	if err != nil {
		t.Fatalf("Load (CRLF): %v", err)
	}
	if back.Description != want {
		t.Errorf("a CRLF description came back as %q, want it unchanged", back.Description)
	}
}
