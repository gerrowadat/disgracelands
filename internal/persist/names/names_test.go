// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package names

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadClassicSkipsBlankAndCommentLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "xnames")
	content := "fuck\n* a comment\n\ncunt\r\nshit\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	got, err := Load("classic", path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := []string{"fuck", "cunt", "shit"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Load(classic) = %v, want %v", got, want)
	}
}

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
	want := []string{"fuck", "cunt", "shit", "asshole"}

	if err := Save("yaml", dir, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load("yaml", dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("round-trip = %v, want %v", got, want)
	}
}

func TestClassicToYamlImport(t *testing.T) {
	srcDir := t.TempDir()
	classicPath := filepath.Join(srcDir, "xnames")
	if err := os.WriteFile(classicPath, []byte("fuck\ncunt\n"), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	list, err := Load("classic", classicPath)
	if err != nil {
		t.Fatalf("Load(classic): %v", err)
	}

	dstDir := t.TempDir()
	if err := Save("yaml", dstDir, list); err != nil {
		t.Fatalf("Save(yaml): %v", err)
	}

	got, err := Load("yaml", dstDir)
	if err != nil {
		t.Fatalf("Load(yaml): %v", err)
	}
	if !reflect.DeepEqual(got, list) {
		t.Errorf("imported = %v, want %v", got, list)
	}
}

func TestSaveClassicIsRefused(t *testing.T) {
	if err := Save("classic", t.TempDir(), []string{"x"}); err == nil {
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
