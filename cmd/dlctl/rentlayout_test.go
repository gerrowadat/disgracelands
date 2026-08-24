// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/persist/player"
	"github.com/gerrowadat/disgracelands/internal/persist/player/binary"
	playeryaml "github.com/gerrowadat/disgracelands/internal/persist/player/yaml"
)

// writeRosterWithRent builds a binary roster in rosterDir and one rent file
// in objsDir, and returns the character's name. The two directories are
// passed separately on purpose: which one holds which is exactly what these
// tests are about.
func writeRosterWithRent(t *testing.T, rosterDir, objsDir string) string {
	t.Helper()
	if err := os.MkdirAll(rosterDir, 0o750); err != nil {
		t.Fatalf("mkdir roster: %v", err)
	}
	if err := os.MkdirAll(objsDir, 0o750); err != nil {
		t.Fatalf("mkdir objs: %v", err)
	}

	roster, err := binary.New(player.Config{Dir: rosterDir})
	if err != nil {
		t.Fatalf("open roster: %v", err)
	}
	defer func() { _ = roster.Close() }()

	rec := &game.PlayerRecord{Name: "Zaphod", IDNum: 1, Level: 5, Conditions: [3]int32{-1, -1, -1}}
	if err := roster.Save(context.Background(), rec); err != nil {
		t.Fatalf("save roster entry: %v", err)
	}

	objs, err := binary.NewObjectStore(player.Config{Dir: rosterDir, ObjectsDir: objsDir})
	if err != nil {
		t.Fatalf("open object store: %v", err)
	}
	// The alias files sit beside the rent files, under the same root and
	// with the same bucketing, so they follow the layout in lockstep.
	aliases, err := binary.NewAliasStore(player.Config{
		Dir: rosterDir, AliasDir: filepath.Join(filepath.Dir(objsDir), "plralias"),
	})
	if err != nil {
		t.Fatalf("open alias store: %v", err)
	}
	if err := aliases.SaveAliases(rec.Name, []game.Alias{{Name: "h", Replacement: " track nobleman"}}); err != nil {
		t.Fatalf("save aliases: %v", err)
	}
	rent := &player.RentFile{
		Code: player.RentRented, Written: time.Unix(1000000, 0).UTC(), CostPerDay: 10,
		Objects: []player.StoredObject{{Vnum: 3001, Weight: 4}},
	}
	if err := objs.SaveObjects(context.Background(), rec.Name, rent); err != nil {
		t.Fatalf("save rent file: %v", err)
	}
	return rec.Name
}

func importedAliases(t *testing.T, dir, name string) []game.Alias {
	t.Helper()
	dst, err := playeryaml.New(player.Config{Dir: dir, ReadOnly: true})
	if err != nil {
		t.Fatalf("open imported roster: %v", err)
	}
	defer func() { _ = dst.Close() }()

	rec, err := dst.Load(context.Background(), name)
	if err != nil {
		t.Fatalf("load imported record: %v", err)
	}
	return rec.Aliases
}

func importedInventory(t *testing.T, dir, name string) int {
	t.Helper()
	dst, err := playeryaml.New(player.Config{Dir: dir, ReadOnly: true})
	if err != nil {
		t.Fatalf("open imported roster: %v", err)
	}
	defer func() { _ = dst.Close() }()

	f, err := dst.LoadObjects(context.Background(), name)
	if err != nil {
		t.Fatalf("load imported rent file: %v", err)
	}
	return len(f.Objects)
}

// TestPfileImportFindsRentFilesBesideTheRoster is the C's own layout: the
// mud runs with its cwd set to lib/ and builds `etc/players` and `plrobjs/`
// from there, so in an archived tree the rent files are a *sibling* of the
// directory the roster is in.
//
// The regression is not that this failed loudly. It reported "0 with a
// rent/crash file" and exited 0 — a sentence that reads as a fact about the
// roster and was a fact about the path, and one that is completely
// unremarkable, since plenty of characters have no rent file.
func TestPfileImportFindsRentFilesBesideTheRoster(t *testing.T) {
	lib := t.TempDir()
	name := writeRosterWithRent(t, filepath.Join(lib, "etc"), filepath.Join(lib, "plrobjs"))

	to := filepath.Join(t.TempDir(), "players")
	if err := run([]string{"pfile", "import", "--from-dir", filepath.Join(lib, "etc"), "--to-dir", to}); err != nil {
		t.Fatalf("run([pfile import]): %v", err)
	}
	if got := importedInventory(t, to, name); got != 1 {
		t.Errorf("imported inventory: got %d objects, want 1", got)
	}
	if got := importedAliases(t, to, name); len(got) != 1 {
		t.Errorf("imported aliases: got %d, want 1", len(got))
	}
}

// TestPfileImportFindsRentFilesInsideTheRoster is this port's own layout —
// one directory holding a roster and the rent files that go with it, which
// is what a server started with --lib-dir writes. It has to keep working.
func TestPfileImportFindsRentFilesInsideTheRoster(t *testing.T) {
	pfiles := filepath.Join(t.TempDir(), "pfiles")
	name := writeRosterWithRent(t, pfiles, filepath.Join(pfiles, "plrobjs"))

	to := filepath.Join(t.TempDir(), "players")
	if err := run([]string{"pfile", "import", "--from-dir", pfiles, "--to-dir", to}); err != nil {
		t.Fatalf("run([pfile import]): %v", err)
	}
	if got := importedInventory(t, to, name); got != 1 {
		t.Errorf("imported inventory: got %d objects, want 1", got)
	}
}

// TestPfileImportHonoursAnExplicitObjectsDir covers a layout that is
// neither, which is what the flag is for.
func TestPfileImportHonoursAnExplicitObjectsDir(t *testing.T) {
	roster := filepath.Join(t.TempDir(), "roster")
	objs := filepath.Join(t.TempDir(), "somewhere-else", "plrobjs")
	name := writeRosterWithRent(t, roster, objs)

	to := filepath.Join(t.TempDir(), "players")
	if err := run([]string{
		"pfile", "import", "--from-dir", roster, "--from-objs-dir", objs, "--to-dir", to,
	}); err != nil {
		t.Fatalf("run([pfile import]): %v", err)
	}
	if got := importedInventory(t, to, name); got != 1 {
		t.Errorf("imported inventory: got %d objects, want 1", got)
	}
}
