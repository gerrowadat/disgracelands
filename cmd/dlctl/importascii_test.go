// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/persist/player"
	"github.com/gerrowadat/disgracelands/internal/persist/player/ascii"
	playeryaml "github.com/gerrowadat/disgracelands/internal/persist/player/yaml"

	"github.com/gerrowadat/disgracelands/internal/persist/convert"
)

// #314: the one-pass import threw --from-format away and defaulted pfile
// to binary, so an ascii tree — which keeps its roster in pfiles/, not
// etc/ — imported nobody, said "imported 0 character(s)", and exited 0
// with the verify pass agreeing that an empty roster matched an empty one.
//
// ascii was the Go server's *default* pfile format before yaml-only, so
// this is the group docs/design/yaml-only.md §8 calls "the group with the
// least warning", following the instruction that document gives them.

// asciiRosterDir writes a small ascii roster into base/pfiles and returns
// the names it wrote.
func asciiRosterDir(t *testing.T, base string) []string {
	t.Helper()

	dir := filepath.Join(base, "pfiles")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	store, err := ascii.New(player.Config{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	names := []string{"ariadne", "belisarius"}
	for i, name := range names {
		rec := &game.PlayerRecord{
			Name:  strings.ToUpper(name[:1]) + name[1:],
			IDNum: int64(i + 1),
			Level: int32(10 + i),
		}
		if err := store.Save(context.Background(), rec); err != nil {
			t.Fatalf("seeding %s: %v", name, err)
		}
	}
	return names
}

// importedNames is who ended up in the converted yaml roster.
func importedNames(t *testing.T, toDir string) []string {
	t.Helper()

	store, err := playeryaml.New(player.Config{Dir: filepath.Join(toDir, "players"), ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	var got []string
	for entry, err := range store.List(context.Background()) {
		if err != nil {
			t.Fatalf("listing the converted roster: %v", err)
		}
		got = append(got, entry.Name)
	}
	return got
}

func TestImportPfileFindsAnAsciiRosterWithoutBeingTold(t *testing.T) {
	from := t.TempDir()
	want := asciiRosterDir(t, from)

	var out bytes.Buffer
	if err := importPfile(importOptions{fromDir: from, toDir: t.TempDir(), encName: convert.DefaultEncoding}, &out); err != nil {
		t.Fatalf("importPfile: %v", err)
	}
	if !strings.Contains(out.String(), "imported 2 character(s)") {
		t.Errorf("the ascii roster was not found:\n%s", out.String())
	}

	// And the flag, when given, still says which one — the documented
	// route in yaml-only.md §8.
	out.Reset()
	if err := importPfile(importOptions{fromDir: from, toDir: t.TempDir(), fromFormat: "ascii", encName: convert.DefaultEncoding}, &out); err != nil {
		t.Fatalf("importPfile --from-format=ascii: %v", err)
	}
	if !strings.Contains(out.String(), "imported 2 character(s)") {
		t.Errorf("--from-format=ascii did not find the roster:\n%s", out.String())
	}
	_ = want
}

// TestImportAllCarriesAnAsciiRoster is the actual reported failure: the
// no---type command docs/operations.md presents as *the* migration command,
// against a tree whose roster is ascii. It is not enough that importPfile
// can do it — cmdImportAll blanked --from-format before calling it, and
// that is what nobody noticed.
func TestImportAllCarriesAnAsciiRoster(t *testing.T) {
	from := t.TempDir()
	want := asciiRosterDir(t, from)

	// A world too, so the one-pass import has something to do besides the
	// roster and reaches the pfile step the way a real migration does.
	copyTree(t, stockBinaryDir, from)

	to := t.TempDir()
	if err := run([]string{"import", "--from-dir", from, "--to-dir", to}); err != nil {
		t.Fatalf("run([import]): %v", err)
	}

	got := importedNames(t, to)
	if len(got) != len(want) {
		t.Fatalf("the one-pass import converted %d characters (%v), want %d (%v)",
			len(got), got, len(want), want)
	}
	for _, name := range want {
		if !slices.Contains(got, name) {
			t.Errorf("%s is not in the converted roster %v", name, got)
		}
	}
}

// TestImportAllHonoursFromFormatForThePfileStep is the other half of the
// same bug: the flag was accepted and discarded, so the documented
// `--from-format=ascii` was indistinguishable from not passing it.
func TestImportAllHonoursFromFormatForThePfileStep(t *testing.T) {
	from := t.TempDir()
	want := asciiRosterDir(t, from)
	copyTree(t, stockBinaryDir, from)

	to := t.TempDir()
	if err := run([]string{
		"import", "--from-format", "ascii", "--from-dir", from, "--to-dir", to,
	}); err != nil {
		t.Fatalf("run([import --from-format=ascii]): %v", err)
	}

	if got := importedNames(t, to); len(got) != len(want) {
		t.Errorf("--from-format=ascii converted %d characters (%v), want %d", len(got), got, len(want))
	}
}

// TestImportPfileSaysSoWhenThereIsNoRoster covers the case that is *not*
// an error and must not become one: a lib with nobody in it.
// examples/stock/binary is exactly that, and it is what the end-to-end
// import tests run on. What changed is that it now says so instead of
// reporting a successful conversion of nothing.
func TestImportPfileSaysSoWhenThereIsNoRoster(t *testing.T) {
	var out bytes.Buffer
	if err := importPfile(importOptions{fromDir: stockBinaryDir, toDir: t.TempDir(), encName: convert.DefaultEncoding}, &out); err != nil {
		t.Fatalf("importPfile on a rosterless tree: %v", err)
	}
	text := out.String()
	if !strings.Contains(text, "no roster found") {
		t.Errorf("a rosterless import said nothing about it:\n%s", text)
	}
	for _, marker := range []string{filepath.Join("etc", "players"), filepath.Join("pfiles", "plr_index")} {
		if !strings.Contains(text, marker) {
			t.Errorf("the message does not name %s, so it does not say where to look:\n%s", marker, text)
		}
	}
}

// TestImportPfileRefusesTwoRosters: a tree holding both is one this tool
// cannot speak for, and choosing would be choosing which half of somebody's
// players to lose.
func TestImportPfileRefusesTwoRosters(t *testing.T) {
	from := t.TempDir()
	asciiRosterDir(t, from)
	copyTree(t, "../../examples/torture/binary", from)

	err := importPfile(importOptions{fromDir: from, toDir: t.TempDir(), encName: convert.DefaultEncoding}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("a tree with both rosters was imported without complaint")
	}
	if !strings.Contains(err.Error(), "--from-format") {
		t.Errorf("the error does not say how to resolve it: %v", err)
	}

	// And --from-format resolves it, which is what the error promises.
	var out bytes.Buffer
	if err := importPfile(importOptions{
		fromDir: from, toDir: t.TempDir(), fromFormat: "ascii", encName: convert.DefaultEncoding,
	}, &out); err != nil {
		t.Fatalf("--from-format=ascii did not resolve the ambiguity: %v", err)
	}
	if !strings.Contains(out.String(), "imported 2 character(s)") {
		t.Errorf("--from-format=ascii picked the wrong roster:\n%s", out.String())
	}
}
