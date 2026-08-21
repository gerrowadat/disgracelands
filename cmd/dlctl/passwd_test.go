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
	"strings"
	"testing"
	"time"

	"github.com/gerrowadat/disgracelands/internal/auth"
	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/persist/player"
	"github.com/gerrowadat/disgracelands/internal/persist/player/ascii"
)

// rosterWith writes a one-character ascii roster and returns its directory.
//
// The character starts on a legacy DES credential because that is the state
// this command exists for: an archived pfile whose password nobody has.
func rosterWith(t *testing.T, name string) string {
	t.Helper()
	dir := t.TempDir()
	store, err := ascii.New(player.Config{Dir: dir})
	if err != nil {
		t.Fatalf("opening a store: %v", err)
	}
	defer func() { _ = store.Close() }()

	rec := &game.PlayerRecord{
		Name:       name,
		Level:      34,
		IDNum:      7,
		LastLogon:  time.Unix(1208649600, 0).UTC(),
		Credential: game.Credential{Scheme: game.SchemeLegacyDES, Hash: "abFAKEHASH123"},
	}
	if err := store.Save(context.Background(), rec); err != nil {
		t.Fatalf("saving %s: %v", name, err)
	}
	return dir
}

// onStdin points os.Stdin at a file holding line, so the command takes the
// not-a-terminal path. A file rather than a pipe: nothing has to drain it,
// and the read sees EOF without a writer to close.
func onStdin(t *testing.T, line string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stdin")
	if err := os.WriteFile(path, []byte(line), 0o600); err != nil {
		t.Fatalf("writing stdin: %v", err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("opening stdin: %v", err)
	}
	saved := os.Stdin
	os.Stdin = f
	t.Cleanup(func() {
		os.Stdin = saved
		_ = f.Close()
	})
}

// loadCredential reads back what the command wrote.
func loadCredential(t *testing.T, dir, name string) game.Credential {
	t.Helper()
	store, err := ascii.New(player.Config{Dir: dir, ReadOnly: true})
	if err != nil {
		t.Fatalf("reopening the store: %v", err)
	}
	defer func() { _ = store.Close() }()
	rec, err := store.Load(context.Background(), name)
	if err != nil {
		t.Fatalf("reloading %s: %v", name, err)
	}
	return rec.Credential
}

func TestPasswdSetsAWorkingPassword(t *testing.T) {
	dir := rosterWith(t, "Zod")
	onStdin(t, "correct horse battery\n")

	if err := run([]string{"pfile", "passwd", "--player-dir", dir, "Zod"}); err != nil {
		t.Fatalf("pfile passwd: %v", err)
	}

	cred := loadCredential(t, dir, "Zod")
	if cred.Scheme != game.SchemeArgon2id {
		t.Fatalf("scheme = %q, want %q", cred.Scheme, game.SchemeArgon2id)
	}

	// The real assertion: the login path accepts it. Comparing hashes would
	// only prove the two sides of this test agree with each other.
	v := auth.Verifier{}
	res, err := v.Verify(cred, "Zod", "correct horse battery")
	if err != nil {
		t.Fatalf("verifying: %v", err)
	}
	if !res.OK {
		t.Error("the password this command set does not verify")
	}

	res, err = v.Verify(cred, "Zod", "something else")
	if err != nil {
		t.Fatalf("verifying a wrong password: %v", err)
	}
	if res.OK {
		t.Error("a wrong password verified")
	}
}

func TestPasswdMatchesTheNameCaseInsensitively(t *testing.T) {
	// Names are matched case-insensitively everywhere else — the ascii store
	// lowercases to build the path — so `dlctl pfile passwd zod` must find
	// Zod rather than reporting no such character.
	dir := rosterWith(t, "Zod")
	onStdin(t, "a good long password\n")

	if err := run([]string{"pfile", "passwd", "--player-dir", dir, "zod"}); err != nil {
		t.Fatalf("pfile passwd zod: %v", err)
	}
	if got := loadCredential(t, dir, "Zod").Scheme; got != game.SchemeArgon2id {
		t.Errorf("scheme = %q, want the password to have been set", got)
	}
}

func TestPasswdAppliesTheSameRulesAsTheMenu(t *testing.T) {
	// Whatever an administrator can set here, the owner must be able to set
	// for themselves at the menu. auth.BadPassword is the one rule; this
	// checks it is actually consulted, and that a rejected password leaves
	// the stored credential alone.
	// The length check runs first, so the name cases need a character whose
	// name is long enough to get past it — "Zod" could never reach the
	// second rule at all.
	cases := []struct {
		name      string
		character string
		password  string
		want      string
	}{
		{"too short", "Zod", "short", "at least 6"},
		{"empty", "Zod", "", "at least 6"},
		{"the character's own name", "Grimalkin", "Grimalkin", "Illegal password"},
		{"the name in another case", "Grimalkin", "gRIMALKIN", "Illegal password"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := rosterWith(t, tc.character)
			onStdin(t, tc.password+"\n")

			err := run([]string{"pfile", "passwd", "--player-dir", dir, tc.character})
			if err == nil {
				t.Fatalf("setting %q succeeded, want a rejection", tc.password)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to mention %q", err, tc.want)
			}
			if got := loadCredential(t, dir, tc.character).Scheme; got != game.SchemeLegacyDES {
				t.Errorf("scheme = %q, want the old credential untouched", got)
			}
		})
	}
}

func TestPasswdRejectsAMissingCharacter(t *testing.T) {
	dir := rosterWith(t, "Zod")
	onStdin(t, "a good long password\n")

	err := run([]string{"pfile", "passwd", "--player-dir", dir, "Nobody"})
	if err == nil {
		t.Fatal("setting a password for a missing character succeeded")
	}
	if !strings.Contains(err.Error(), "Nobody") {
		t.Errorf("error = %q, want it to name the character", err)
	}
}

func TestPasswdRefusesTheBinaryFormat(t *testing.T) {
	// The binary record's password field is eleven bytes, so an argon2id
	// hash cannot go in it. Writing a DES one instead would answer a
	// question nobody asked; this has to fail, and say why.
	dir := rosterWith(t, "Zod")
	onStdin(t, "a good long password\n")

	err := run([]string{"pfile", "passwd", "--player-dir", dir, "--player-format", "binary", "Zod"})
	if err == nil {
		t.Fatal("pfile passwd on a binary roster succeeded, want a refusal")
	}
	if !strings.Contains(err.Error(), "convert") {
		t.Errorf("error = %q, want it to point at pfile convert", err)
	}
}

func TestPasswdNeedsExactlyOneName(t *testing.T) {
	dir := rosterWith(t, "Zod")
	for _, args := range [][]string{
		{"pfile", "passwd", "--player-dir", dir},
		{"pfile", "passwd", "--player-dir", dir, "Zod", "Bob"},
	} {
		if err := run(args); err == nil {
			t.Errorf("run(%v) succeeded, want an argument error", args)
		}
	}
}
