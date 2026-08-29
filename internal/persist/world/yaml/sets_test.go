// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package yaml

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gerrowadat/disgracelands/internal/persist/world"
)

// twoZoneDir writes a world directory with zones 1 and 2 in it, and
// whatever sets.yaml the caller asks for.
func twoZoneDir(t *testing.T, sets string) string {
	t.Helper()

	dir := t.TempDir()
	zone := func(vnum int, name string) string {
		return "schema: " + ZoneSchema + "\n" +
			"zone:\n" +
			"  vnum: " + itoa(vnum) + "\n" +
			"  name: " + name + "\n" +
			"  range:\n" +
			"  - " + itoa(vnum*100) + "\n" +
			"  - " + itoa(vnum*100+99) + "\n" +
			"  lifespan: 30\n" +
			"  reset: always\n" +
			"rooms:\n" +
			"- vnum: " + itoa(vnum*100) + "\n" +
			"  name: Room " + itoa(vnum) + "\n" +
			"  sector: inside\n" +
			"  desc: A room.\n"
	}
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("1-one.yaml", zone(1, "One"))
	write("2-two.yaml", zone(2, "Two"))
	write(ManifestFile, "schema: "+ManifestSchema+"\nzones:\n- 1\n- 2\n")
	if sets != "" {
		write(SetsFile, sets)
	}
	return dir
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func loadDir(t *testing.T, dir string, mini bool) (rooms int, err error) {
	t.Helper()

	src, nerr := New(world.Config{Dir: dir, Mini: mini})
	if nerr != nil {
		t.Fatal(nerr)
	}
	w, lerr := src.Load(context.Background())
	if lerr != nil {
		return 0, lerr
	}
	return len(w.Rooms), nil
}

// The whole of issue #274 in one assertion: --mini-mud has to load less.
//
// It was accepted, validated and passed to world.Config.Mini, which only the
// classic source read — and cmd/dlmud stopped linking classic when yaml-only
// landed. `dlmud --mini-mud` booted all 30 zones and 1,878 rooms, identically
// to `dlmud` without it, with no test failing and nothing printed.
func TestMiniLoadsOnlyTheMiniSet(t *testing.T) {
	dir := twoZoneDir(t, "schema: "+SetsSchema+"\nsets:\n  mini:\n  - 1\n")

	full, err := loadDir(t, dir, false)
	if err != nil {
		t.Fatalf("full load: %v", err)
	}
	if full != 2 {
		t.Fatalf("full load has %d rooms, want 2", full)
	}

	mini, err := loadDir(t, dir, true)
	if err != nil {
		t.Fatalf("mini load: %v", err)
	}
	if mini != 1 {
		t.Errorf("mini load has %d rooms, want 1 — --mini-mud loaded the whole world", mini)
	}
}

// Asking for a subset a directory does not have is an error, not "load
// everything".
//
// This is the specific shape #274 was: a flag that quietly does nothing looks
// exactly like a flag that worked, and nobody notices for as long as nobody
// counts the rooms. It is also what the C does — index_boot exits when the
// index file it was told to open is missing.
func TestMiniWithoutASetIsAnError(t *testing.T) {
	for _, tc := range []struct {
		name string
		sets string
		want string
	}{
		{"no sets.yaml at all", "", `no "mini" set`},
		{"sets.yaml with other sets", "schema: " + SetsSchema + "\nsets:\n  builders:\n  - 2\n", `no "mini" set`},
		{"an empty mini set", "schema: " + SetsSchema + "\nsets:\n  mini: []\n", "is empty"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := twoZoneDir(t, tc.sets)

			if _, err := loadDir(t, dir, false); err != nil {
				t.Fatalf("the full load must still work: %v", err)
			}

			_, err := loadDir(t, dir, true)
			if err == nil {
				t.Fatal("--mini-mud succeeded with no mini set; it must refuse rather than load everything")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to mention %q", err, tc.want)
			}
			if !strings.Contains(err.Error(), SetsFile) {
				t.Errorf("error = %q, want it to name %s so the operator knows what to write", err, SetsFile)
			}
		})
	}
}

// A set naming a zone the manifest does not is a mistake worth reporting:
// the two files are edited separately, and the alternative is a silently
// smaller world.
func TestAMiniSetNamingAnUnknownZoneIsReported(t *testing.T) {
	dir := twoZoneDir(t, "schema: "+SetsSchema+"\nsets:\n  mini:\n  - 1\n  - 99\n")

	src, err := New(world.Config{Dir: dir, Mini: true})
	if err != nil {
		t.Fatal(err)
	}
	_, warnings, err := src.LoadWithWarnings(context.Background())
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	var found bool
	for _, w := range warnings {
		if w.Severity == world.Error && strings.Contains(w.Message, "99") {
			found = true
		}
	}
	if !found {
		t.Errorf("no error about zone 99, which is in the mini set and not in the manifest; got %v", warnings)
	}
}

// A wrong schema tag is refused rather than read as an empty document,
// matching every other file in this format.
func TestSetsSchemaIsChecked(t *testing.T) {
	dir := twoZoneDir(t, "schema: dl/sets@99\nsets:\n  mini:\n  - 1\n")

	_, err := loadDir(t, dir, true)
	if err == nil || !strings.Contains(err.Error(), "schema") {
		t.Errorf("err = %v, want a complaint about the schema tag", err)
	}
}

// WriteSets writes nothing for a source that has no subsets, rather than a
// file that says nothing — `dlctl import` should not manufacture a sets.yaml
// for a lib/ that had no index.mini.
func TestWriteSetsWritesNothingForNoSets(t *testing.T) {
	dir := t.TempDir()
	for _, sets := range []map[string][]int32{nil, {}, {"mini": {}}} {
		if err := WriteSets(dir, sets); err != nil {
			t.Fatalf("WriteSets(%v): %v", sets, err)
		}
		if _, err := os.Stat(filepath.Join(dir, SetsFile)); !os.IsNotExist(err) {
			t.Fatalf("WriteSets(%v) wrote a file; it should write none", sets)
		}
	}

	if err := WriteSets(dir, map[string][]int32{"mini": {30, 0, 12}}); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(dir, SetsFile)) //nolint:gosec // test temp dir
	if err != nil {
		t.Fatal(err)
	}
	// Sorted, like zones.yaml: §10.3's determinism rule applies to every
	// file this format writes, so a re-import cannot churn the diff.
	want := "schema: " + SetsSchema + "\nsets:\n  mini:\n  - 0\n  - 12\n  - 30\n"
	if string(body) != want {
		t.Errorf("sets.yaml =\n%s\nwant\n%s", body, want)
	}
}
