// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package clock

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestClassicLoadMissingFileIsDefaultEpoch(t *testing.T) {
	got, err := Load("classic", filepath.Join(t.TempDir(), "does-not-exist"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Unix() != DefaultEpoch {
		t.Errorf("Load(missing) = %v (%d), want the default epoch %d", got, got.Unix(), DefaultEpoch)
	}
}

func TestClassicLoadZeroIsDefaultEpoch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "time")
	if err := os.WriteFile(path, []byte("0\n"), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	got, err := Load("classic", path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Unix() != DefaultEpoch {
		t.Errorf("Load(zero) = %d, want the default epoch %d", got.Unix(), DefaultEpoch)
	}
}

func TestClassicLoadGarbageIsDefaultEpoch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "time")
	if err := os.WriteFile(path, []byte("not a number\n"), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	got, err := Load("classic", path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Unix() != DefaultEpoch {
		t.Errorf("Load(garbage) = %d, want the default epoch %d", got.Unix(), DefaultEpoch)
	}
}

func TestClassicRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "time")
	want := time.Date(2001, 3, 14, 12, 0, 0, 0, time.UTC)

	if err := Save("classic", path, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load("classic", path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !got.Equal(want) {
		t.Errorf("round-trip = %v, want %v", got, want)
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading raw file: %v", err)
	}
	if string(b) != "984571200\n" {
		t.Errorf("raw file = %q, want a bare integer line", string(b))
	}
}

func TestNativeLoadMissingFileIsDefaultEpoch(t *testing.T) {
	got, err := Load("native", t.TempDir())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Unix() != DefaultEpoch {
		t.Errorf("Load(missing native) = %d, want the default epoch %d", got.Unix(), DefaultEpoch)
	}
}

func TestNativeRoundTrips(t *testing.T) {
	dir := t.TempDir()
	want := time.Date(2001, 3, 14, 12, 0, 0, 0, time.UTC)

	if err := Save("native", dir, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load("native", dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !got.Equal(want) {
		t.Errorf("round-trip = %v, want %v", got, want)
	}
}

func TestClassicToNativeImport(t *testing.T) {
	classicPath := filepath.Join(t.TempDir(), "time")
	if err := os.WriteFile(classicPath, []byte("984571200\n"), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	epoch, err := Load("classic", classicPath)
	if err != nil {
		t.Fatalf("Load(classic): %v", err)
	}

	dstDir := t.TempDir()
	if err := Save("native", dstDir, epoch); err != nil {
		t.Fatalf("Save(native): %v", err)
	}
	got, err := Load("native", dstDir)
	if err != nil {
		t.Fatalf("Load(native): %v", err)
	}
	if !got.Equal(epoch) {
		t.Errorf("imported = %v, want %v", got, epoch)
	}
}

func TestUnknownFormatIsRefused(t *testing.T) {
	if _, err := Load("nonsense", "x"); err == nil {
		t.Error("Load(nonsense) succeeded, want a refusal")
	}
	if err := Save("nonsense", t.TempDir(), time.Now()); err == nil {
		t.Error("Save(nonsense) succeeded, want a refusal")
	}
}
