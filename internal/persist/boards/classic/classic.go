// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

// Package classic reads and writes bulletin board files, porting
// Board_save_board and Board_load_board (boards.c:416, :457).
//
// A board file is a message count followed by, for each message, a raw
// `struct board_msginfo` and then the heading and body bytes. As with the
// player database, the format *is* the struct's memory layout — and this one
// has a live `char *heading` pointer in the middle of it, written straight to
// disk. The value is meaningless once the process exits and the loader
// ignores it, but its width decides where everything after it sits: four
// bytes on the i386 build the archive came from, eight on any 64-bit rebuild.
//
// reference/tools/boardlayout.c prints what the compiler chooses and
// layout_test.go compares.
package classic

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/gerrowadat/disgracelands/internal/persist/boards"
)

// FormatName is the name this format registers under, and the default the
// server runs on.
const FormatName = "classic"

func init() {
	boards.Register(FormatName, func(cfg boards.Config) (boards.Store, error) {
		return New(cfg)
	})
}

// byteOrder is little-endian, as everywhere else in this archive.
var byteOrder = binary.LittleEndian

// The ILP32 layout of `struct board_msginfo`, which is the one every archived
// board file is in. Verified against gcc -m32 by layout_test.go.
const (
	offSlotNum    = 0
	offHeading    = 4 // the pointer; written, never read
	offLevel      = 8
	offHeadingLen = 12
	offMessageLen = 16
	msgInfoSize   = 20
	// countSize is the leading `int` holding how many messages follow.
	countSize = 4
)

// Store reads and writes the board files in a directory.
type Store struct {
	dir      string
	readOnly bool
	mu       sync.RWMutex
}

// New opens the board files under an etc directory.
func New(cfg boards.Config) (*Store, error) {
	if cfg.Dir == "" {
		return nil, fmt.Errorf("boards: no directory configured")
	}
	return &Store{dir: cfg.Dir, readOnly: cfg.ReadOnly}, nil
}

// Name implements boards.Store.
func (s *Store) Name() string { return FormatName }

// Close implements boards.Store.
func (s *Store) Close() error { return nil }

// Load implements boards.Store.
//
// A file that will not parse is an error rather than a silent reset. The C
// logs "Board file %d corrupt.  Resetting." and *deletes* it (boards.c:470);
// deleting the only copy of everything anybody ever posted because one length
// looked wrong is not a behaviour worth keeping, so the caller is told and
// the file is left alone. That is in docs/deviations.md.
func (s *Store) Load(name string) ([]boards.Message, error) {
	path := filepath.Join(s.dir, name)

	s.mu.RLock()
	defer s.mu.RUnlock()

	b, err := os.ReadFile(path) //nolint:gosec // the name comes from the board table, not from input
	if errors.Is(err, os.ErrNotExist) {
		return nil, boards.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("reading board %s: %w", name, err)
	}
	msgs, err := decode(b)
	if err != nil {
		return nil, fmt.Errorf("reading board %s: %w", name, err)
	}
	return msgs, nil
}

// Save implements boards.Store, or removes the file when there are none —
// which is what Board_save_board does, and why an emptied board leaves no
// trace on disk.
func (s *Store) Save(name string, msgs []boards.Message) error {
	if s.readOnly {
		return fmt.Errorf("boards: the data directory is open read-only")
	}
	path := filepath.Join(s.dir, name)

	s.mu.Lock()
	defer s.mu.Unlock()

	if len(msgs) == 0 {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("removing empty board %s: %w", name, err)
		}
		return nil
	}

	if err := os.MkdirAll(s.dir, 0o750); err != nil {
		return fmt.Errorf("writing board %s: %w", name, err)
	}
	// Temporary and rename, as with the rent files: the C truncates first,
	// and a crash mid-write would leave a board that is neither.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, encode(msgs), 0o600); err != nil {
		return fmt.Errorf("writing board %s: %w", name, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("writing board %s: %w", name, err)
	}
	return nil
}

// encode builds a board file.
func encode(msgs []boards.Message) []byte {
	out := make([]byte, countSize)
	byteOrder.PutUint32(out, uint32(len(msgs))) //nolint:gosec // bounded by MaxBoardMessages

	for _, m := range msgs {
		// The C writes strlen+1 and the trailing NUL with it, so a heading of
		// length zero is impossible and a length of 1 means an empty string.
		heading := append([]byte(m.Heading), 0)
		var body []byte
		if m.Body != "" {
			body = append([]byte(m.Body), 0)
		}

		rec := make([]byte, msgInfoSize)
		// slot_num is a live index into the C's message pool. It is written
		// and never read back — the loader assigns a fresh slot — so what
		// goes here does not matter. Zero rather than the C's residue.
		byteOrder.PutUint32(rec[offSlotNum:], 0)
		// The heading pointer. Whatever address the C happened to have; a
		// board file written here has zeroes, which the C's loader ignores
		// exactly as it ignores a real one.
		byteOrder.PutUint32(rec[offHeading:], 0)
		byteOrder.PutUint32(rec[offLevel:], uint32(m.Level))           //nolint:gosec // a level, reinterpreted
		byteOrder.PutUint32(rec[offHeadingLen:], uint32(len(heading))) //nolint:gosec // bounded by the headline limit
		byteOrder.PutUint32(rec[offMessageLen:], uint32(len(body)))    //nolint:gosec // bounded by the message limit

		out = append(out, rec...)
		out = append(out, heading...)
		out = append(out, body...)
	}
	return out
}

// decode reads a board file.
func decode(b []byte) ([]boards.Message, error) {
	if len(b) < countSize {
		return nil, fmt.Errorf("file is %d bytes, too short for a message count", len(b))
	}
	count := int(int32(byteOrder.Uint32(b))) //nolint:gosec // reinterpretation of the count the C wrote
	if count < 1 {
		// The C treats this as corruption and resets the board. Here an empty
		// board is simply an empty board — Save removes the file rather than
		// writing a zero, so a zero can only have come from the C.
		return nil, nil
	}

	msgs := make([]boards.Message, 0, count)
	off := countSize
	for i := 0; i < count; i++ {
		if off+msgInfoSize > len(b) {
			return nil, fmt.Errorf("message %d: file ends inside the index", i+1)
		}
		rec := b[off : off+msgInfoSize]
		off += msgInfoSize

		level := int32(byteOrder.Uint32(rec[offLevel:]))                //nolint:gosec // reinterpretation
		headingLen := int(int32(byteOrder.Uint32(rec[offHeadingLen:]))) //nolint:gosec // reinterpretation
		bodyLen := int(int32(byteOrder.Uint32(rec[offMessageLen:])))    //nolint:gosec // reinterpretation

		// "if ((len1 = ...heading_len) <= 0) ... corrupt". A heading is
		// always written with its NUL, so zero means the file is wrong.
		if headingLen <= 0 {
			return nil, fmt.Errorf("message %d: heading length is %d", i+1, headingLen)
		}
		if bodyLen < 0 || off+headingLen+bodyLen > len(b) {
			return nil, fmt.Errorf("message %d: lengths run past the end of the file", i+1)
		}

		heading := trimNUL(b[off : off+headingLen])
		off += headingLen
		body := ""
		if bodyLen > 0 {
			body = trimNUL(b[off : off+bodyLen])
			off += bodyLen
		}

		msgs = append(msgs, boards.Message{Heading: heading, Level: level, Body: body})
	}
	return msgs, nil
}

// trimNUL drops everything from the first NUL, which is where the C's string
// ended even though it wrote a fixed number of bytes after it.
func trimNUL(b []byte) string {
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}
