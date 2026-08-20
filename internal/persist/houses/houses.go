// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

// Package houses reads and writes the house control file, porting
// House_save_control and House_boot (house.c:226, :243).
//
// The control file is the whole `house_control` array written with one
// fwrite, so it is an array of `struct house_control_rec` and the format is
// that struct's memory layout — the fourth of these in the archive, after the
// player database, the rent files and the boards.
//
// The objects inside a house are a separate file per room, `<vnum>.house`,
// and those are a bare sequence of `obj_file_elem` records with no header —
// the same record the rent files use. See internal/persist/player/binary.
package houses

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var byteOrder = binary.LittleEndian

// MaxHouses and MaxGuests are the C's (house.h:1).
const (
	MaxHouses = 100
	MaxGuests = 10
)

// ModePrivate is HOUSE_PRIVATE, and the only mode there is: the switch in
// House_can_enter has one case and the field was never used for anything
// else.
const ModePrivate int32 = 0

// The ILP32 layout of `struct house_control_rec`. Verified against gcc -m32
// by layout_test.go.
//
// Note the two-byte hole at offset 6: three sh_ints followed by a
// four-aligned time_t. The C never writes it, because the record is filled
// field by field on the stack and then fwritten whole — so every house
// control file in the archive has two bytes of stack residue per record, and
// so do the eight spare longs and the unused tail of the guest array.
const (
	offVnum        = 0
	offAtrium      = 2
	offExitNum     = 4
	offBuiltOn     = 8
	offMode        = 12
	offOwner       = 16
	offNumGuests   = 20
	offGuests      = 24
	offLastPayment = 64
	offSpare0      = 68
	recordSize     = 100
	guestStride    = 4
)

// House is one control record.
type House struct {
	// Vnum is the room that is the house; Atrium the room its door opens
	// into.
	Vnum   int32
	Atrium int32
	// ExitNum is the direction of the house's door, which must be two-way.
	ExitNum int32

	BuiltOn     time.Time
	LastPayment time.Time

	Mode  int32
	Owner int64
	// Guests are the id numbers the owner has let in, at most MaxGuests.
	Guests []int64
}

// Store reads and writes the control file.
type Store struct {
	// path is the control file; dir is where the per-house object files live.
	path     string
	dir      string
	readOnly bool
	mu       sync.RWMutex
}

// New opens the house files. control is the hcontrol path; dir is the
// directory holding the `<vnum>.house` object files.
func New(control, dir string, readOnly bool) (*Store, error) {
	if control == "" || dir == "" {
		return nil, fmt.Errorf("houses: no paths configured")
	}
	return &Store{path: control, dir: dir, readOnly: readOnly}, nil
}

// Load reads the control file. A missing file is not an error: it is a server
// with no houses on it, which is what the archive has.
func (s *Store) Load() ([]House, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	b, err := os.ReadFile(s.path) //nolint:gosec // a configured path
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading the house control file: %w", err)
	}

	var out []House
	for off := 0; off+recordSize <= len(b) && len(out) < MaxHouses; off += recordSize {
		out = append(out, decode(b[off:off+recordSize]))
	}
	return out, nil
}

// Save writes the control file.
func (s *Store) Save(houses []House) error {
	if s.readOnly {
		return fmt.Errorf("houses: the data directory is open read-only")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]byte, 0, len(houses)*recordSize)
	for _, h := range houses {
		out = append(out, encode(h)...)
	}

	if err := os.MkdirAll(filepath.Dir(s.path), 0o750); err != nil {
		return fmt.Errorf("writing the house control file: %w", err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, out, 0o600); err != nil {
		return fmt.Errorf("writing the house control file: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("writing the house control file: %w", err)
	}
	return nil
}

// ObjectFile is House_get_filename: `<vnum>.house` under the house
// directory.
func (s *Store) ObjectFile(vnum int32) string {
	return filepath.Join(s.dir, fmt.Sprintf("%d.house", vnum))
}

// LoadObjects reads a house's contents. Returns nil for a house nobody has
// left anything in.
func (s *Store) LoadObjects(vnum int32) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	b, err := os.ReadFile(s.ObjectFile(vnum)) //nolint:gosec // built from a vnum
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading house %d: %w", vnum, err)
	}
	return b, nil
}

// SaveObjects writes a house's contents, or removes the file when there are
// none.
func (s *Store) SaveObjects(vnum int32, b []byte) error {
	if s.readOnly {
		return fmt.Errorf("houses: the data directory is open read-only")
	}
	path := s.ObjectFile(vnum)

	s.mu.Lock()
	defer s.mu.Unlock()

	if len(b) == 0 {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("emptying house %d: %w", vnum, err)
		}
		return nil
	}
	if err := os.MkdirAll(s.dir, 0o750); err != nil {
		return fmt.Errorf("writing house %d: %w", vnum, err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return fmt.Errorf("writing house %d: %w", vnum, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("writing house %d: %w", vnum, err)
	}
	return nil
}

// DeleteObjects removes a house's object file, porting House_delete_file.
func (s *Store) DeleteObjects(vnum int32) error {
	if s.readOnly {
		return fmt.Errorf("houses: the data directory is open read-only")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.Remove(s.ObjectFile(vnum)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("deleting house %d: %w", vnum, err)
	}
	return nil
}

func decode(rec []byte) House {
	h := House{
		Vnum:    int32(int16(byteOrder.Uint16(rec[offVnum:]))),    //nolint:gosec // reinterpretation
		Atrium:  int32(int16(byteOrder.Uint16(rec[offAtrium:]))),  //nolint:gosec // reinterpretation
		ExitNum: int32(int16(byteOrder.Uint16(rec[offExitNum:]))), //nolint:gosec // reinterpretation
		Mode:    int32(byteOrder.Uint32(rec[offMode:])),           //nolint:gosec // reinterpretation
		Owner:   int64(int32(byteOrder.Uint32(rec[offOwner:]))),   //nolint:gosec // reinterpretation
	}
	if secs := int32(byteOrder.Uint32(rec[offBuiltOn:])); secs != 0 { //nolint:gosec // reinterpretation
		h.BuiltOn = time.Unix(int64(secs), 0).UTC()
	}
	if secs := int32(byteOrder.Uint32(rec[offLastPayment:])); secs != 0 { //nolint:gosec // reinterpretation
		h.LastPayment = time.Unix(int64(secs), 0).UTC()
	}

	n := int(int32(byteOrder.Uint32(rec[offNumGuests:]))) //nolint:gosec // reinterpretation
	if n < 0 {
		n = 0
	}
	if n > MaxGuests {
		n = MaxGuests
	}
	for i := 0; i < n; i++ {
		id := int64(int32(byteOrder.Uint32(rec[offGuests+i*guestStride:]))) //nolint:gosec // reinterpretation
		h.Guests = append(h.Guests, id)
	}
	return h
}

func encode(h House) []byte {
	rec := make([]byte, recordSize)
	byteOrder.PutUint16(rec[offVnum:], uint16(int16(h.Vnum)))       //nolint:gosec // the format's width
	byteOrder.PutUint16(rec[offAtrium:], uint16(int16(h.Atrium)))   //nolint:gosec // ditto
	byteOrder.PutUint16(rec[offExitNum:], uint16(int16(h.ExitNum))) //nolint:gosec // ditto
	byteOrder.PutUint32(rec[offMode:], uint32(h.Mode))              //nolint:gosec // reinterpretation
	byteOrder.PutUint32(rec[offOwner:], uint32(int32(h.Owner)))     //nolint:gosec // ids are 32-bit here
	byteOrder.PutUint32(rec[offBuiltOn:], unixOrZero(h.BuiltOn))
	byteOrder.PutUint32(rec[offLastPayment:], unixOrZero(h.LastPayment))

	n := len(h.Guests)
	if n > MaxGuests {
		n = MaxGuests
	}
	byteOrder.PutUint32(rec[offNumGuests:], uint32(n)) //nolint:gosec // bounded above
	for i := 0; i < n; i++ {
		byteOrder.PutUint32(rec[offGuests+i*guestStride:], uint32(int32(h.Guests[i]))) //nolint:gosec // ids are 32-bit here
	}

	// The padding hole and the eight spares are left at zero. The C leaves
	// stack residue there; zero is the same to every reader and does not leak
	// anything.
	_ = offSpare0
	return rec
}

func unixOrZero(t time.Time) uint32 {
	if t.IsZero() {
		return 0
	}
	return uint32(int32(t.Unix())) //nolint:gosec // 2038, and the format's fault
}
