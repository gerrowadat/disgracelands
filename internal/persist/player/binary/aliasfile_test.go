// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package binary

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/persist/player"
)

// aliasCases are the shapes an alias file actually takes. The leading space
// on every replacement is not decoration: do_alias builds one with
// any_one_arg, which stops on the whitespace separating the alias's name
// from the rest of the line without skipping it, so that space is part of
// the value the C holds in memory and this port holds too.
var aliasCases = []game.Alias{
	{Name: "h", Replacement: " track nobleman"},
	{Name: "dg", Replacement: " cast 'dispel good' $1"},
	{Name: "swr", Replacement: " remove ruby;wear sapphire"},
	{Name: "all", Replacement: " say $*"},
	{Name: "x", Replacement: " look"},
}

// TestAliasFileMatchesTheC runs write_aliases and read_aliases themselves
// and requires this package to produce the same bytes and read back the
// same values.
//
// The pairing worth checking is the one that is easy to read past:
// write_aliases stores `strlen(replacement) - 1` and `replacement + 1`,
// read_aliases puts the space back with `*xbuf = ' '`. Get it wrong in
// either direction and every replacement loses or gains a leading
// character — which, for a simple alias, nothing reports and nobody sees
// until they use it.
//
// No -m32 here, unlike the other file-format oracles: this format is
// fprintf and fgets rather than an fwrite of a struct, so nothing in it
// depends on the data model.
func TestAliasFileMatchesTheC(t *testing.T) {
	gcc, err := exec.LookPath("gcc")
	if err != nil {
		t.Skip("gcc not found; skipping the alias oracle")
	}
	src, err := filepath.Abs(filepath.Join("..", "..", "..", "..", "reference", "tools", "aliasoracle.c"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "aliasoracle")
	if out, err := exec.Command(gcc, "-std=gnu89", "-w", "-o", bin, src).CombinedOutput(); err != nil {
		t.Fatalf("building the oracle: %v\n%s", err, out)
	}

	args := []string{filepath.Join(dir, "c.alias")}
	for _, a := range aliasCases {
		args = append(args, a.Name, a.Replacement)
	}
	out, err := exec.Command(bin, args...).Output()
	if err != nil {
		t.Fatalf("running the oracle: %v", err)
	}

	var wantFile string
	var gotBack []game.Alias
	var pending game.Alias
	for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		key, value, _ := strings.Cut(line, " ")
		value = strings.ReplaceAll(value, "\\n", "\n")
		switch key {
		case "file":
			wantFile = value
		case "alias":
			pending = game.Alias{Name: value}
		case "replacement":
			pending.Replacement = value
			gotBack = append(gotBack, pending)
		}
	}

	written, err := encodeAliases(aliasCases)
	if err != nil {
		t.Fatalf("encoding: %v", err)
	}
	if got := string(written); got != wantFile {
		t.Errorf("written file differs from write_aliases':\n got %q\nwant %q", got, wantFile)
	}
	// What the C reads back out of its own file is what this must too.
	if !reflect.DeepEqual(gotBack, aliasCases) {
		t.Errorf("read_aliases round trip:\n got %+v\nwant %+v", gotBack, aliasCases)
	}
	decoded, err := decodeAliases([]byte(wantFile), "zaphod")
	if err != nil {
		t.Fatalf("decoding the C's own file: %v", err)
	}
	if !reflect.DeepEqual(decoded, aliasCases) {
		t.Errorf("decoding the C's own file:\n got %+v\nwant %+v", decoded, aliasCases)
	}
}

func TestAliasStoreRoundTrips(t *testing.T) {
	dir := t.TempDir()
	s, err := NewAliasStore(player.Config{Dir: dir})
	if err != nil {
		t.Fatalf("NewAliasStore: %v", err)
	}
	if err := s.SaveAliases("Zaphod", aliasCases); err != nil {
		t.Fatalf("SaveAliases: %v", err)
	}
	got, err := s.LoadAliases("Zaphod")
	if err != nil {
		t.Fatalf("LoadAliases: %v", err)
	}
	if !reflect.DeepEqual(got, aliasCases) {
		t.Errorf("round trip:\n got %+v\nwant %+v", got, aliasCases)
	}
}

// TestAReplacementWithNoLeadingSpaceIsRefused is #242.
//
// encodeAliases dropped the first character of every non-empty
// replacement, on the assumption that it was the space do_alias leaves in
// front of one. The assumption is right about the game — any_one_arg does
// not skip the whitespace it stops on, so an in-memory replacement always
// begins with that space — and was unchecked about everything else, so
// anything constructing a game.Alias programmatically got silent
// corruption: "get all corpse" stored and read back as " et all corpse".
//
// Refusing rather than repairing is this package's posture everywhere
// else (putStr refuses a name too long for its field rather than
// truncating it), and repairing would be a guess: whether the caller meant
// to include the space or forgot it is not something the encoder knows.
func TestAReplacementWithNoLeadingSpaceIsRefused(t *testing.T) {
	dir := t.TempDir()
	s, err := NewAliasStore(player.Config{Dir: dir})
	if err != nil {
		t.Fatalf("NewAliasStore: %v", err)
	}

	err = s.SaveAliases("Zaphod", []game.Alias{{Name: "gc", Replacement: "get all corpse"}})
	if err == nil {
		t.Fatal("saving a replacement with no leading space was accepted; it would have " +
			"stored \" et all corpse\"")
	}
	if !strings.Contains(err.Error(), "gc") {
		t.Errorf("the error does not name the alias it is about: %v", err)
	}
	// And nothing was written: a refused save leaves the file alone
	// rather than half of it.
	if _, statErr := os.Stat(filepath.Join(dir, "plralias", "U-Z", "zaphod.alias")); !os.IsNotExist(statErr) {
		t.Errorf("a refused save wrote a file anyway: %v", statErr)
	}

	// An empty replacement is not this case. The C cannot produce one —
	// do_alias treats it as a delete — and it is written as the empty
	// string rather than as a negative length, which is what the encoder
	// has always done.
	if err := s.SaveAliases("Zaphod", []game.Alias{{Name: "x", Replacement: ""}}); err != nil {
		t.Errorf("an empty replacement was refused: %v", err)
	}
}

// TestAliasStoreBucketsByFirstLetter pins the path, which is get_filename's
// (utils.c:518) and shared with the rent files.
func TestAliasStoreBucketsByFirstLetter(t *testing.T) {
	dir := t.TempDir()
	s, err := NewAliasStore(player.Config{Dir: dir})
	if err != nil {
		t.Fatalf("NewAliasStore: %v", err)
	}
	if err := s.SaveAliases("Zaphod", aliasCases); err != nil {
		t.Fatalf("SaveAliases: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "plralias", "U-Z", "zaphod.alias")); err != nil {
		t.Errorf("expected U-Z/zaphod.alias: %v", err)
	}
}

// TestAliasStoreRemovesTheFileWhenNoneAreLeft is write_aliases' own
// `remove(fn)` before its NULL check. It matters rather than being tidy:
// read_aliases has no empty-file case, so a zero-byte file left behind is
// one the C cannot read.
func TestAliasStoreRemovesTheFileWhenNoneAreLeft(t *testing.T) {
	dir := t.TempDir()
	s, err := NewAliasStore(player.Config{Dir: dir})
	if err != nil {
		t.Fatalf("NewAliasStore: %v", err)
	}
	if err := s.SaveAliases("Zaphod", aliasCases); err != nil {
		t.Fatalf("SaveAliases: %v", err)
	}
	if err := s.SaveAliases("Zaphod", nil); err != nil {
		t.Fatalf("SaveAliases(nil): %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "plralias", "U-Z", "zaphod.alias")); !os.IsNotExist(err) {
		t.Errorf("alias file should be gone: %v", err)
	}
	if _, err := s.LoadAliases("Zaphod"); err != player.ErrNotFound { //nolint:errorlint // sentinel from this call
		t.Errorf("LoadAliases after clearing: got %v, want ErrNotFound", err)
	}
}

// TestAliasStoreMissingFileIsNotAnError: most characters have no alias
// file, because write_aliases removes it rather than writing an empty one.
func TestAliasStoreMissingFileIsNotAnError(t *testing.T) {
	s, err := NewAliasStore(player.Config{Dir: t.TempDir()})
	if err != nil {
		t.Fatalf("NewAliasStore: %v", err)
	}
	if _, err := s.LoadAliases("Nobody"); err != player.ErrNotFound { //nolint:errorlint // sentinel from this call
		t.Errorf("got %v, want ErrNotFound", err)
	}
}

// TestAliasStoreRejectsAnUnsafeName: the name is a path component, exactly
// as it is in the C, which gets away with it because _parse_name has
// already refused anything but letters.
func TestAliasStoreRejectsAnUnsafeName(t *testing.T) {
	s, err := NewAliasStore(player.Config{Dir: t.TempDir()})
	if err != nil {
		t.Fatalf("NewAliasStore: %v", err)
	}
	if _, err := s.LoadAliases("../../etc/passwd"); err == nil {
		t.Error("expected an error for a name that is not letters")
	}
}
