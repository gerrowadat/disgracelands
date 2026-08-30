// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

// Package atomicfile writes a file by writing a temporary one beside it and
// renaming, so a reader never sees a half-written file and a crash never
// leaves one.
//
// Every persistence format in this tree already did this, and every one of
// them did it by hand with the *same* temporary name: `path + ".tmp"`. That
// works exactly as long as no two writes to the same file overlap, and two
// of them do — the house control file is written by `hcontrol build` on the
// world goroutine and by the crash-save sweep off it, and when they raced,
// the first rename moved the shared temporary out from under the second,
// which then failed with "no such file or directory" and logged an error
// about a file it had in fact just written correctly.
//
// It was found by moving test/play onto yaml (docs/design/yaml-only.md
// §5.4) — a real server, booted on a real directory, playing a real
// scenario, which is the only kind of test that could have found it, and
// exactly what CLAUDE.md says that suite is for.
//
// Eighteen call sites had the bug; they share this now, so there is one
// place to be right rather than eighteen places to stay right.
package atomicfile

import (
	"fmt"
	"os"
	"path/filepath"
)

// Write writes data to path atomically, creating the file with perm.
//
// The temporary is created in the destination's own directory, not in
// TMPDIR: rename is only atomic within a filesystem, and a data directory
// on a different mount from /tmp is ordinary rather than exotic.
func Write(path string, data []byte, perm os.FileMode) error {
	dir, base := filepath.Dir(path), filepath.Base(path)

	f, err := os.CreateTemp(dir, base+".tmp")
	if err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	tmp := f.Name()
	// From here on every failure removes the temporary: a data directory
	// that accumulates half-written files after a full disk is its own
	// problem, arriving at the worst possible moment.
	cleanup := func(err error) error {
		_ = f.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("writing %s: %w", path, err)
	}

	if _, err := f.Write(data); err != nil {
		return cleanup(err)
	}
	// Before the rename, not after: a rename that lands before the data
	// reaches the disk is a file that is atomically the wrong length after
	// a power cut, which is the failure this whole dance is meant to
	// prevent.
	if err := f.Sync(); err != nil {
		return cleanup(err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("writing %s: %w", path, err)
	}
	// CreateTemp always makes 0600; the callers want their own mode.
	if err := os.Chmod(tmp, perm); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("writing %s: %w", path, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}
