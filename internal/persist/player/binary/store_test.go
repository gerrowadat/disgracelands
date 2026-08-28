// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package binary

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/persist/player"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := New(player.Config{Dir: t.TempDir()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func sampleRecord(name string) *game.PlayerRecord {
	return &game.PlayerRecord{
		Name:       name,
		Title:      "the Tester",
		Level:      30,
		IDNum:      7,
		Credential: game.Credential{Scheme: game.SchemeLegacyDES, Hash: "abXYZ12345"},
		Skills:     map[int32]int32{1: 95},
		Points:     game.Points{Hit: 100, MaxHit: 100, Gold: 500},
	}
}

func TestMissingFileIsAnEmptyRoster(t *testing.T) {
	// A blank roster is the normal fresh-install state, not an error: the C
	// server creates the file on demand and promotes whoever registers first.
	s := newTestStore(t)
	ctx := context.Background()

	ok, err := s.Exists(ctx, "Nobody")
	if err != nil {
		t.Fatalf("Exists on a missing file: %v", err)
	}
	if ok {
		t.Error("Exists = true for an empty roster")
	}

	for _, err := range s.List(ctx) {
		if err != nil {
			t.Errorf("List on a missing file: %v", err)
		}
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	want := sampleRecord("Zod")
	if err := s.Save(ctx, want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := s.Load(ctx, "Zod")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Name != want.Name || got.Level != want.Level || got.IDNum != want.IDNum {
		t.Errorf("loaded %+v, want name/level/idnum from %+v", got, want)
	}
	if got.Credential != want.Credential {
		t.Errorf("credential = %+v, want %+v", got.Credential, want.Credential)
	}
	if got.Skills[1] != 95 {
		t.Errorf("skill 1 = %d, want 95", got.Skills[1])
	}
}

func TestLoadIsCaseInsensitive(t *testing.T) {
	// Players type their names however they like, and every existing format
	// matches them case-insensitively.
	s := newTestStore(t)
	ctx := context.Background()

	if err := s.Save(ctx, sampleRecord("Zod")); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Zod", "zod", "ZOD", "zOd"} {
		if _, err := s.Load(ctx, name); err != nil {
			t.Errorf("Load(%q): %v", name, err)
		}
	}
}

func TestLoadMissingCharacter(t *testing.T) {
	s := newTestStore(t)
	_, err := s.Load(context.Background(), "Nobody")
	if !errors.Is(err, player.ErrNotFound) {
		t.Errorf("Load of a missing character = %v, want ErrNotFound", err)
	}
}

func TestSaveUpdatesInPlace(t *testing.T) {
	// Records are addressed by position and other files reference those
	// positions, so an update must not move anyone.
	s := newTestStore(t)
	ctx := context.Background()

	for _, n := range []string{"Alice", "Bob", "Carol"} {
		if err := s.Save(ctx, sampleRecord(n)); err != nil {
			t.Fatal(err)
		}
	}

	updated := sampleRecord("Bob")
	updated.Level = 34
	if err := s.Save(ctx, updated); err != nil {
		t.Fatal(err)
	}

	var names []string
	for e, err := range s.List(ctx) {
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, e.Name)
	}
	if len(names) != 3 {
		t.Fatalf("roster is %v, want three characters", names)
	}
	// Lower case: player.IndexEntry.Name is the C's own player_table
	// name, which boot_db lowercases as it builds it (db.c:607). This
	// used to be "Bob" here and lower case in ascii, which is how the
	// three formats came to disagree about a shared model field.
	if names[1] != "bob" {
		t.Errorf("roster order changed: %v", names)
	}

	got, err := s.Load(ctx, "Bob")
	if err != nil {
		t.Fatal(err)
	}
	if got.Level != 34 {
		t.Errorf("Bob's level = %d, want 34", got.Level)
	}
}

func TestDelete(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	for _, n := range []string{"Alice", "Bob"} {
		if err := s.Save(ctx, sampleRecord(n)); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Delete(ctx, "Alice"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if ok, _ := s.Exists(ctx, "Alice"); ok {
		t.Error("Alice still exists after Delete")
	}
	if ok, _ := s.Exists(ctx, "Bob"); !ok {
		t.Error("Bob disappeared when Alice was deleted")
	}
	if err := s.Delete(ctx, "Nobody"); !errors.Is(err, player.ErrNotFound) {
		t.Errorf("Delete of a missing character = %v, want ErrNotFound", err)
	}
}

func TestReadOnlyRefusesWrites(t *testing.T) {
	s, err := New(player.Config{Dir: t.TempDir(), ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := s.Save(ctx, sampleRecord("Zod")); err == nil {
		t.Error("Save succeeded on a read-only store")
	}
	if err := s.Delete(ctx, "Zod"); err == nil {
		t.Error("Delete succeeded on a read-only store")
	}
}

func TestSaveRefusesAnOversizedName(t *testing.T) {
	// Truncating would produce a different character. The plan is explicit
	// that this has to fail loudly rather than silently (§5.1).
	s := newTestStore(t)
	rec := sampleRecord("ThisNameIsFarTooLongForTheFormat")
	err := s.Save(context.Background(), rec)
	if err == nil {
		t.Fatal("Save accepted a name longer than the format allows")
	}
	if got := s.Capabilities().MaxNameLength; got != 20 {
		t.Errorf("MaxNameLength = %d, want 20", got)
	}
}

func TestSaveRefusesAModernCredential(t *testing.T) {
	// The password field is eleven bytes. A modern hash does not fit, and
	// this is why moving off this format is a prerequisite for modern
	// password hashing rather than an independent improvement.
	s := newTestStore(t)
	rec := sampleRecord("Zod")
	rec.Credential = game.Credential{Scheme: game.SchemeArgon2id, Hash: "$argon2id$v=19$m=65536,t=3,p=4$c29tZXNhbHQ$hash"}

	err := s.Save(context.Background(), rec)
	if err == nil {
		t.Fatal("Save accepted an argon2id credential")
	}
	if s.Capabilities().Supports(game.SchemeArgon2id) {
		t.Error("Capabilities claims to support argon2id")
	}
	if !s.Capabilities().Supports(game.SchemeLegacyDES) {
		t.Error("Capabilities does not claim to support the scheme it actually stores")
	}
}

func TestCapabilitiesReportTheFormatsRealLimits(t *testing.T) {
	c := newTestStore(t).Capabilities()
	for _, tc := range []struct {
		name      string
		got, want int
	}{
		{"MaxNameLength", c.MaxNameLength, 20},
		{"MaxTitleLength", c.MaxTitleLength, 80},
		{"MaxDescriptionLength", c.MaxDescriptionLength, 239},
		{"MaxAffects", c.MaxAffects, 32},
		{"MaxSkillNumber", c.MaxSkillNumber, 200},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %d, want %d", tc.name, tc.got, tc.want)
		}
	}
	if !c.TimestampsOverflowIn2038 {
		t.Error("TimestampsOverflowIn2038 = false, but this format stores 32-bit time_t")
	}
}

func TestWrongDataModelIsExplained(t *testing.T) {
	// A file in the 64-bit layout can only have come from a modern rebuild of
	// the C server. Reading it as 32-bit would misread every field past the
	// first `long` while producing plausible numbers, so it has to be
	// recognised and refused rather than accepted.
	dir := t.TempDir()
	lp := computeLayout(lp64).Size
	if err := os.WriteFile(filepath.Join(dir, FileName), make([]byte, lp*3), 0o600); err != nil {
		t.Fatal(err)
	}

	s, err := New(player.Config{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Load(context.Background(), "Anyone")
	if err == nil {
		t.Fatal("a 64-bit-layout file was accepted")
	}
	for _, want := range []string{"64-bit", "misread"} {
		if !contains(err.Error(), want) {
			t.Errorf("error = %q, want it to mention %q", err, want)
		}
	}
}

func TestTruncatedFileIsReported(t *testing.T) {
	dir := t.TempDir()
	size := computeLayout(ilp32).Size
	if err := os.WriteFile(filepath.Join(dir, FileName), make([]byte, size+7), 0o600); err != nil {
		t.Fatal(err)
	}
	s, _ := New(player.Config{Dir: dir})
	if _, err := s.Load(context.Background(), "Anyone"); err == nil {
		t.Fatal("a truncated file was accepted")
	}
}

func TestVerify(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	for _, n := range []string{"Alice", "Bob"} {
		if err := s.Save(ctx, sampleRecord(n)); err != nil {
			t.Fatal(err)
		}
	}

	r, err := s.Verify(ctx)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if r.Records != 2 || r.Named != 2 {
		t.Errorf("Verify = %+v, want 2 records and 2 named", r)
	}
	if r.LegacyPasswords != 2 {
		t.Errorf("LegacyPasswords = %d, want 2", r.LegacyPasswords)
	}
	if len(r.Problems) != 0 {
		t.Errorf("Problems = %v, want none", r.Problems)
	}
	if r.RecordSize != 1288 {
		t.Errorf("RecordSize = %d, want 1288", r.RecordSize)
	}
}

func TestSaveIsAtomic(t *testing.T) {
	// The C server writes in place, so an interrupted save corrupts the
	// roster. This one writes and renames, so the directory should never
	// contain a partial database under the real name.
	s := newTestStore(t)
	ctx := context.Background()
	if err := s.Save(ctx, sampleRecord("Zod")); err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(filepath.Dir(s.path))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != FileName {
			t.Errorf("left a stray file behind: %s", e.Name())
		}
	}

	info, err := os.Stat(s.path)
	if err != nil {
		t.Fatal(err)
	}
	// The file is password hashes and connection hosts.
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("database mode is %o, want 600", perm)
	}
}

func TestRegisteredUnderItsName(t *testing.T) {
	found := false
	for _, n := range player.Formats() {
		if n == FormatName {
			found = true
		}
	}
	if !found {
		t.Errorf("Formats() = %v, want it to include %q", player.Formats(), FormatName)
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
