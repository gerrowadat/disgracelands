// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package parity

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// StageLib copies a classic lib/ directory into dst and makes it a directory
// two servers can each be given a private copy of.
//
// Both harnesses call this — scripts/session-parity.sh through `dlctl parity
// stage`, and test/parity directly — so that "the two servers were given the
// same world" is one piece of code rather than two that have to be kept in
// agreement. They were two, in Go and in shell, and the shell one is what
// this is a port of.
//
// What it does beyond copying is in the comments below: empty the
// player-bearing directories, and add the two boards the patched C server
// insists on.
func StageLib(dst, src string) error {
	if err := os.MkdirAll(dst, 0o700); err != nil {
		return err
	}
	if err := copyTree(dst, src); err != nil {
		return fmt.Errorf("copying %s: %w", src, err)
	}

	// The player-bearing directories: emptied, not removed, because both
	// servers expect them to exist. An empty roster is what makes the first
	// character created an implementor on each side — db.c's "if this is our
	// first player --- he be God" (~line 2705) — so the two servers are
	// comparing the same character with the same powers.
	for _, d := range []string{
		"pfiles", "plrobjs", "plralias", "house",
		"plrobjs/A", "plrobjs/B", "plrobjs/C", "plrobjs/N",
		"plrobjs/O", "plrobjs/P", "plrobjs/Z",
	} {
		if err := os.RemoveAll(filepath.Join(dst, d)); err != nil {
			return err
		}
	}
	for _, d := range []string{
		"pfiles", "plrobjs", "plralias", "house", "etc", "misc",
		"plrobjs/A", "plrobjs/B", "plrobjs/C", "plrobjs/N",
		"plrobjs/O", "plrobjs/P", "plrobjs/Z",
	} {
		if err := os.MkdirAll(filepath.Join(dst, d), 0o700); err != nil {
			return err
		}
	}
	for _, f := range []string{"etc/players", "etc/plrmail", "etc/hcontrol"} {
		if err := os.Remove(filepath.Join(dst, f)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	if err := os.WriteFile(filepath.Join(dst, "etc/players"), nil, 0o600); err != nil {
		return err
	}

	return addMissingBoards(dst)
}

// addMissingBoards adds the two board objects the patched C server declares
// and the stock world does not have.
//
// boards.c in the patched tree declares six boards (boards.c:67-72) and two of
// them — 3094 "suggestion" and 3095 "pkill" — are Disgracelands additions whose
// objects only ever existed in the archived world. examples/stock/binary/ is
// stock CircleMUD 3.0 bpl20, which has 3096-3099 and no more, so the C server
// hits "SYSERR: Fatal board error: board vnum 3095 does not exist!" and dies
// the moment an immortal looks at the board room.
//
// So the scratch copy gets them, modelled on 3096 and identical for both
// servers. Synthetic data for a test, in a directory that is deleted
// afterwards — examples/stock/binary/ itself is untouched. See
// docs/deviations.md.
func addMissingBoards(dst string) error {
	path := filepath.Join(dst, "world", "obj", "30.obj")
	body, err := os.ReadFile(path) //nolint:gosec // a directory this function just wrote
	if err != nil {
		if os.IsNotExist(err) {
			return nil // A yaml directory, or one without zone 30.
		}
		return err
	}
	if strings.Contains(string(body), "\n#3094\n") {
		return nil
	}

	var b strings.Builder
	scanner := bufio.NewScanner(strings.NewReader(string(body)))
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	inserted := false
	for scanner.Scan() {
		line := scanner.Text()
		// In ascending vnum order, before #3096. That is not tidiness:
		// real_object binary-searches obj_index, which is built in file
		// order, so a record out of order is a record the server cannot
		// find — appending these at the end left them in the file and
		// invisible to the lookup, with the same fatal error as before.
		if !inserted && line == "#3096" {
			for vnum := 3094; vnum <= 3095; vnum++ {
				fmt.Fprintf(&b, "#%d\n", vnum)
				b.WriteString("board bulletin~\n" +
					"a bulletin board~\n" +
					"A bulletin board is mounted on a wall here.~\n" +
					"~\n" +
					"13 0 0\n" +
					"0 0 0 0\n" +
					"0 0 0\n" +
					"E\n" +
					"board~\n" +
					"If you can read this, the board is not working.\n" +
					"~\n")
			}
			inserted = true
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(b.String()), 0o600)
}

// copyTree copies src over dst, following symlinks and making everything
// writable.
//
// Writable matters: the checked-in example data is read-only often enough for
// it to be the difference between a server that boots and one that cannot save
// a player. Following symlinks matters because the copy is of the data, not of
// whatever the working tree happens to point at.
func copyTree(dst, src string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)

		info, err := os.Stat(path) // Stat, not d.Info: follows the symlink.
		if err != nil {
			return err
		}
		if info.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		body, err := os.ReadFile(path) //nolint:gosec // a directory the caller named
		if err != nil {
			return err
		}
		return os.WriteFile(target, body, 0o600) //nolint:gosec // dst joined with a path relative to src
	})
}
