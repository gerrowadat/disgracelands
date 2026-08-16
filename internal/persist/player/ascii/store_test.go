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
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func sample(name string) *game.PlayerRecord {
	return &game.PlayerRecord{
		Name:       name,
		Title:      "the Tester",
		Level:      30,
		IDNum:      7,
		LastLogon:  time.Unix(1208649600, 0).UTC(),
		Credential: game.Credential{Scheme: game.SchemeLegacyDES, Hash: "abFAKEHASH"},
		Skills:     map[int32]int32{1: 95},
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if err := s.Save(ctx, sample("Zod")); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := s.Load(ctx, "Zod")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Name != "Zod" || got.Level != 30 || got.IDNum != 7 {
		t.Errorf("loaded %+v", got)
	}
}

func TestFileLayoutMatchesTheFormat(t *testing.T) {
	// <dir>/<first letter, lowercased>/<name, lowercased>, which is what the
	// C-side tooling expects to find.
	s := newTestStore(t)
	if err := s.Save(context.Background(), sample("Zod")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(s.dir, "z", "zod")); err != nil {
		t.Errorf("expected <dir>/z/zod: %v", err)
	}
}

func TestLoadIsCaseInsensitive(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if err := s.Save(ctx, sample("Zod")); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Zod", "zod", "ZOD", "zOd"} {
		if _, err := s.Load(ctx, name); err != nil {
			t.Errorf("Load(%q): %v", name, err)
		}
	}
}

func TestNamesThatWouldEscapeTheDirectoryAreRefused(t *testing.T) {
	// A character name becomes a filename here, so anything that could climb
	// out has to be refused rather than sanitised: a character called
	// "../../etc/passwd" is not a character.
	s := newTestStore(t)
	ctx := context.Background()
	for _, name := range []string{"../etc", "a/b", `a\b`, ".", "..", "a.b", ""} {
		if err := s.Save(ctx, sample(name)); err == nil {
			t.Errorf("Save accepted the name %q", name)
		}
		if _, err := s.Load(ctx, name); err == nil {
			t.Errorf("Load accepted the name %q", name)
		}
	}
}

func TestMissingRosterIsEmptyNotAnError(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	for _, err := range s.List(ctx) {
		if err != nil {
			t.Errorf("List on an empty directory: %v", err)
		}
	}
	if ok, err := s.Exists(ctx, "Nobody"); err != nil || ok {
		t.Errorf("Exists = %v, %v; want false, nil", ok, err)
	}
	if _, err := s.Load(ctx, "Nobody"); !errors.Is(err, player.ErrNotFound) {
		t.Errorf("Load = %v, want ErrNotFound", err)
	}
}

func TestIndexIsWrittenAndRead(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	for _, n := range []string{"Zod", "Aardvark", "Welmar"} {
		if err := s.Save(ctx, sample(n)); err != nil {
			t.Fatal(err)
		}
	}

	data, err := os.ReadFile(filepath.Join(s.dir, IndexFile))
	if err != nil {
		t.Fatalf("reading %s: %v", IndexFile, err)
	}
	text := string(data)

	// Five whitespace-separated fields per line, terminated by "~".
	if !strings.HasSuffix(text, "~\n") {
		t.Errorf("%s does not end with its terminator:\n%s", IndexFile, text)
	}
	for _, line := range strings.Split(strings.TrimSuffix(text, "~\n"), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if n := len(strings.Fields(line)); n != 5 {
			t.Errorf("index line %q has %d fields, want 5", line, n)
		}
	}
	// Names in the index are lowercased, matching the filenames.
	if !strings.Contains(text, " zod ") {
		t.Errorf("index does not carry the lowercased name:\n%s", text)
	}

	var names []string
	for e, err := range s.List(ctx) {
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, e.Name)
	}
	if len(names) != 3 {
		t.Errorf("List returned %v, want three characters", names)
	}
}

func TestFlagsInTheIndexAreNeverEmpty(t *testing.T) {
	// The index is read with whitespace-separated fields, so an empty flags
	// column would shift the last-logon time into its place. The format
	// writes a literal "0" instead.
	s := newTestStore(t)
	if err := s.Save(context.Background(), sample("Zod")); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(s.dir, IndexFile))
	fields := strings.Fields(strings.Split(string(data), "\n")[0])
	if len(fields) != 5 {
		t.Fatalf("index line has %d fields: %q", len(fields), fields)
	}
	if fields[3] != "0" {
		t.Errorf("flags column = %q, want %q for a character with no flags", fields[3], "0")
	}
}

func TestIndexSurvivesDeletion(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	for _, n := range []string{"Zod", "Welmar"} {
		if err := s.Save(ctx, sample(n)); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Delete(ctx, "Zod"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	var names []string
	for e, err := range s.List(ctx) {
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, e.Name)
	}
	if len(names) != 1 || names[0] != "welmar" {
		t.Errorf("after deleting Zod the roster is %v", names)
	}
	if err := s.Delete(ctx, "Nobody"); !errors.Is(err, player.ErrNotFound) {
		t.Errorf("Delete of a missing character = %v, want ErrNotFound", err)
	}
}

func TestRebuildIndexRepairsIt(t *testing.T) {
	// A roster produced by conversion, or one whose index was lost, has to be
	// repairable without saving every character.
	s := newTestStore(t)
	ctx := context.Background()
	for _, n := range []string{"Zod", "Welmar"} {
		if err := s.Save(ctx, sample(n)); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Remove(filepath.Join(s.dir, IndexFile)); err != nil {
		t.Fatal(err)
	}

	count := 0
	for range s.List(ctx) {
		count++
	}
	if count != 0 {
		t.Errorf("List found %d characters with no index; it should read the index", count)
	}

	if err := s.RebuildIndex(); err != nil {
		t.Fatalf("RebuildIndex: %v", err)
	}
	count = 0
	for _, err := range s.List(ctx) {
		if err != nil {
			t.Fatal(err)
		}
		count++
	}
	if count != 2 {
		t.Errorf("after rebuilding, List found %d characters, want 2", count)
	}
}

func TestReadOnlyRefusesWrites(t *testing.T) {
	dir := t.TempDir()
	rw, _ := New(player.Config{Dir: dir})
	if err := rw.Save(context.Background(), sample("Zod")); err != nil {
		t.Fatal(err)
	}

	ro, _ := New(player.Config{Dir: dir, ReadOnly: true})
	ctx := context.Background()
	if err := ro.Save(ctx, sample("Welmar")); err == nil {
		t.Error("Save succeeded on a read-only store")
	}
	if err := ro.Delete(ctx, "Zod"); err == nil {
		t.Error("Delete succeeded on a read-only store")
	}
	if err := ro.RebuildIndex(); err == nil {
		t.Error("RebuildIndex succeeded on a read-only store")
	}
	if _, err := ro.Load(ctx, "Zod"); err != nil {
		t.Errorf("Load failed on a read-only store: %v", err)
	}
}

func TestFilesAreNotWorldReadable(t *testing.T) {
	// A player file holds a password hash and the host they last connected
	// from.
	s := newTestStore(t)
	if err := s.Save(context.Background(), sample("Zod")); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{filepath.Join(s.dir, "z", "zod"), filepath.Join(s.dir, IndexFile)} {
		info, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Errorf("%s is mode %o, want 600", p, perm)
		}
	}
}

func TestNoStrayTemporaryFiles(t *testing.T) {
	// Saves go through a temporary file and a rename, so an interrupted save
	// leaves the previous contents. Nothing should be left behind afterwards.
	s := newTestStore(t)
	if err := s.Save(context.Background(), sample("Zod")); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			t.Errorf("left a temporary file behind: %s", e.Name())
		}
	}
}

func TestCapabilitiesAreUnlimited(t *testing.T) {
	// The zeroes are the point: no fixed-width fields, so no limits, and
	// timestamps that outlive 2038.
	c := newTestStore(t).Capabilities()
	for _, tc := range []struct {
		name  string
		limit int
	}{
		{"MaxNameLength", c.MaxNameLength},
		{"MaxTitleLength", c.MaxTitleLength},
		{"MaxDescriptionLength", c.MaxDescriptionLength},
		{"MaxAffects", c.MaxAffects},
		{"MaxSkillNumber", c.MaxSkillNumber},
	} {
		if tc.limit != 0 {
			t.Errorf("%s = %d, want 0 (no limit)", tc.name, tc.limit)
		}
	}
	if c.TimestampsOverflowIn2038 {
		t.Error("TimestampsOverflowIn2038 = true, but this format stores decimal text")
	}
	if !c.Supports(game.SchemeArgon2id) {
		t.Error("the format does not claim to support a modern credential")
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
