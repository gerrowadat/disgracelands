// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/gerrowadat/disgracelands/internal/auth"
	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/persist/player"
)

//nolint:gosec // G101 matches "passwd" in the name; this is help text, not a credential.
const helpPfilePasswd = `Usage: dlctl pfile passwd [options] <name>

Sets a character's password. The new password is read from the terminal
without echo, or from standard input if it is not a terminal:

    dlctl pfile passwd --player-dir=lib/pfiles Bob
    printf '%s\n' "$NEW" | dlctl pfile passwd Bob

Stop the server first. It holds a logged-in character's record in memory and
writes it back on the next save, which would undo this.

Options:
`

// cmdPfilePasswd sets one character's password from outside the game.
//
// Nothing else can. The C has no `set ... password` — act.wizard.c's set_fields
// never had one — and this port's only path is the owner changing their own
// from the menu (interpreter.c:1637, internal/session/menu.go:176). That is
// fine for a live game and useless for an archive: a character whose password
// was a DES hash nobody remembers has no route back in, and the alternative
// an administrator reaches for is editing the `Pass:` line of the pfile by
// hand, which means either pasting a hash from somewhere or inventing one.
// Both are worse than a command that does it properly, so this is one.
//
// It is deliberately not an in-game wizard command. A god who can set another
// character's password can log in as them, and that is a different thing from
// anything the C's immortal levels grant; keeping it offline keeps it to
// whoever already has the pfiles on disk, who could have edited them anyway.
func cmdPfilePasswd(args []string) error {
	fs := flag.NewFlagSet("pfile passwd", flag.ContinueOnError)
	dir, format := pfileFlags(fs)
	fs.Usage = func() {
		_, _ = io.WriteString(fs.Output(), helpPfilePasswd)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return fmt.Errorf("pfile passwd needs exactly one character name")
	}
	name := fs.Arg(0)

	store, err := player.Open(*format, player.Config{Dir: *dir})
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	// Ask the format before asking for a password, so a run against a binary
	// pfile directory fails before anyone has typed anything. The binary
	// record's password field is eleven bytes: an argon2id hash cannot be
	// stored in it at all, and silently writing a DES one instead would be
	// answering a different question than the one asked.
	if !store.Capabilities().Supports(game.SchemeArgon2id) {
		return fmt.Errorf("the %q format cannot store a modern password hash; "+
			"convert the directory first with `dlctl pfile convert`", store.Name())
	}

	ctx := context.Background()
	rec, err := store.Load(ctx, name)
	if err != nil {
		if errors.Is(err, player.ErrNotFound) {
			return fmt.Errorf("no character called %q in %s", name, *dir)
		}
		return err
	}

	password, err := readNewPassword(rec.Name)
	if err != nil {
		return err
	}
	// The same rule the menu applies, from the same place: an administrator
	// must not be able to set a password its owner could not have chosen,
	// because the first thing they will do is try to change it.
	if reason := auth.BadPassword(password, rec.Name); reason != "" {
		return errors.New(strings.TrimSuffix(reason, "."))
	}

	cred, err := auth.NewCredential(password)
	if err != nil {
		return err
	}
	rec.Credential = cred
	if err := store.Save(ctx, rec); err != nil {
		return err
	}

	fmt.Printf("Password set for %s (level %d, last on %s).\n",
		rec.Name, rec.Level, formatTime(rec.LastLogon))
	return nil
}

// readNewPassword collects the new password, twice if there is a human to ask.
//
// A terminal gets a prompt with echo off and a confirmation, exactly as the
// menu does. Anything else — a pipe, a here-string, a CI job — gets one line
// read from stdin and no confirmation, because there is nobody to have made a
// typo. Both are trimmed the way the menu trims (internal/session/menu.go:199),
// so a password settable here is settable in-game and the other way about.
func readNewPassword(name string) (string, error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return "", fmt.Errorf("reading a password from standard input: %w", err)
		}
		return strings.TrimSpace(line), nil
	}

	fmt.Printf("New password for %s: ", name)
	first, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return "", fmt.Errorf("reading a password: %w", err)
	}

	fmt.Print("Retype it: ")
	second, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return "", fmt.Errorf("reading a password: %w", err)
	}

	if strings.TrimSpace(string(first)) != strings.TrimSpace(string(second)) {
		return "", errors.New("passwords don't match")
	}
	return strings.TrimSpace(string(first)), nil
}
