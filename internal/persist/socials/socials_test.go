// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package socials

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gerrowadat/disgracelands/internal/game"
)

func TestLoadClassicMissingFileIsEmptyNotError(t *testing.T) {
	got, err := Load("classic", filepath.Join(t.TempDir(), "does-not-exist"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got != nil {
		t.Errorf("Load(missing) = %v, want nil", got)
	}
}

func TestLoadYamlMissingFileIsEmptyNotError(t *testing.T) {
	got, err := Load("yaml", t.TempDir())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got != nil {
		t.Errorf("Load(missing yaml) = %v, want nil", got)
	}
}

func TestYamlRoundTrips(t *testing.T) {
	dir := t.TempDir()
	want := []game.Social{
		{
			// A social that takes a target, with every field filled in.
			Name:              "accuse",
			Hide:              false,
			MinVictimPosition: game.PosStanding,
			CharNoArg:         "Accuse who??",
			// OthersNoArg left empty: classic's own "#" for one line
			// within a pair, not the whole no_arg block.
			CharFound:   "You look accusingly at $M.",
			OthersFound: "$n looks accusingly at $N.",
			VictFound:   "$n looks accusingly at you.",
			NotFound:    "Accuse somebody who's not even there??",
			CharAuto:    "You accuse yourself.",
			OthersAuto:  "$n seems to have a bad conscience.",
		},
		{
			// A social with no target at all: found/not_found/self are
			// all empty because the classic parser never reads them for
			// one of these (CharFound == "" is the C's TakesTarget()
			// gate), not because they happen to be blank this time.
			Name:              "applaud",
			Hide:              true,
			MinVictimPosition: game.PosSleeping,
			CharNoArg:         "Clap, clap, clap.",
			OthersNoArg:       "$n gives a round of applause.",
		},
	}

	if err := Save("yaml", dir, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load("yaml", dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("round-trip = %+v, want %+v", got, want)
	}
}

func TestYamlOmitsBlocksTheClassicParserNeverPopulates(t *testing.T) {
	dir := t.TempDir()
	if err := Save("yaml", dir, []game.Social{
		{Name: "beg", MinVictimPosition: game.PosStanding, CharNoArg: "You beg."},
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dir, YamlFile))
	if err != nil {
		t.Fatalf("reading raw file: %v", err)
	}
	for _, absent := range []string{"found:", "not_found:", "self:"} {
		if strings.Contains(string(b), absent) {
			t.Errorf("raw file contains %q, want it omitted for a social with no target:\n%s", absent, b)
		}
	}
}

func TestYamlUnknownPositionIsAnError(t *testing.T) {
	dir := t.TempDir()
	fixture := "schema: dl/socials@1\nsocials:\n- command: nonsense\n  hide: false\n  min_victim_position: nonsense\n"
	if err := os.WriteFile(filepath.Join(dir, YamlFile), []byte(fixture), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	if _, err := Load("yaml", dir); err == nil {
		t.Error("Load with an unrecognised min_victim_position succeeded, want an error")
	}
}

func TestSaveClassicIsRefused(t *testing.T) {
	if err := Save("classic", t.TempDir(), nil); err == nil {
		t.Error("Save(classic) succeeded, want a refusal")
	}
}

func TestUnknownFormatIsRefused(t *testing.T) {
	if _, err := Load("nonsense", "x"); err == nil {
		t.Error("Load(nonsense) succeeded, want a refusal")
	}
	if err := Save("nonsense", t.TempDir(), nil); err == nil {
		t.Error("Save(nonsense) succeeded, want a refusal")
	}
}

// Against the real archive: classic parses it (already covered by
// game.ParseSocials's own tests), and importing it into yaml and reading
// it back produces byte-identical records.
func TestClassicToYamlImportAgainstTheRealArchive(t *testing.T) {
	classic, err := Load("classic", "../../../data/misc/socials")
	if err != nil {
		t.Fatalf("Load(classic): %v", err)
	}
	if len(classic) != 104 {
		t.Fatalf("got %d records from the real archive, want 104", len(classic))
	}

	dir := t.TempDir()
	if err := Save("yaml", dir, classic); err != nil {
		t.Fatalf("Save(yaml): %v", err)
	}
	yaml, err := Load("yaml", dir)
	if err != nil {
		t.Fatalf("Load(yaml): %v", err)
	}
	if !reflect.DeepEqual(yaml, classic) {
		t.Fatalf("yaml round-trip does not match the classic parse")
	}
}
