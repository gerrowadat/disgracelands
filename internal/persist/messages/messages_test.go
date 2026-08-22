// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package messages

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

func TestLoadNativeMissingFileIsEmptyNotError(t *testing.T) {
	got, err := Load("native", t.TempDir())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got != nil {
		t.Errorf("Load(missing native) = %v, want nil", got)
	}
}

func TestNativeRoundTrips(t *testing.T) {
	dir := t.TempDir()
	want := []game.FightMessage{
		{
			AttackType: game.TypeHit + game.AttackSlash,
			Die:        game.MsgSet{Attacker: "die attacker", Victim: "die victim", Room: "die room"},
			Miss:       game.MsgSet{Attacker: "miss attacker"},
			Hit:        game.MsgSet{Victim: "hit victim"},
			// God left entirely empty: classic's own "#/#/#" for a block,
			// which should round-trip as an absent key, not three empty
			// strings written out.
		},
		{
			AttackType: game.SkillKick,
			Hit:        game.MsgSet{Attacker: "kick hit", Victim: "kicked", Room: "kick room"},
		},
	}

	if err := Save("native", dir, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load("native", dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("round-trip = %+v, want %+v", got, want)
	}
}

func TestNativeOmitsAnEntirelyEmptyMsgSet(t *testing.T) {
	dir := t.TempDir()
	if err := Save("native", dir, []game.FightMessage{
		{AttackType: game.TypeHit + game.AttackHit, Hit: game.MsgSet{Attacker: "hit"}},
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dir, NativeFile))
	if err != nil {
		t.Fatalf("reading raw file: %v", err)
	}
	for _, absent := range []string{"die:", "miss:", "god:"} {
		if strings.Contains(string(b), absent) {
			t.Errorf("raw file contains %q, want the whole empty block omitted:\n%s", absent, b)
		}
	}
}

func TestNativeUnknownAttackTypeIsAnError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, NativeFile), []byte("schema: dl/messages@1\nmessages:\n- attack_type: nonsense\n  hit:\n    attacker: x\n"), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	if _, err := Load("native", dir); err == nil {
		t.Error("Load with an unrecognised attack_type succeeded, want an error")
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
// game.ParseMessagesFile's own tests), and importing it into native and
// reading it back produces byte-identical records — the whole point of
// AttackTypeName/AttackTypeFromName being real inverses of each other,
// not just plausible-looking ones.
func TestClassicToNativeImportAgainstTheRealArchive(t *testing.T) {
	classic, err := Load("classic", "../../../data/misc/messages")
	if err != nil {
		t.Fatalf("Load(classic): %v", err)
	}
	if len(classic) != 55 {
		t.Fatalf("got %d records from the real archive, want 55", len(classic))
	}

	dir := t.TempDir()
	if err := Save("native", dir, classic); err != nil {
		t.Fatalf("Save(native): %v", err)
	}
	native, err := Load("native", dir)
	if err != nil {
		t.Fatalf("Load(native): %v", err)
	}
	if !reflect.DeepEqual(native, classic) {
		t.Fatalf("native round-trip does not match the classic parse")
	}
}
