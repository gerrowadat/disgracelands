// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package binary

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/persist/atomicfile"
	"github.com/gerrowadat/disgracelands/internal/persist/player"
)

// Rent files, ported from objsave.c and `struct rent_info` /
// `struct obj_file_elem` (structs.h:662, structs.h:678).
//
// Same discipline as the player database and for the same reason: the file is
// a raw fwrite of a struct, so the format *is* the struct's memory layout. The
// offsets come from the layout engine in layout.go rather than being written
// out here, and reference/tools/objfilelayout.c checks them against what gcc
// actually chooses.
//
// One file per player, under `plrobjs/`, split into five directories by first
// letter. That split is not decoration: this is a filesystem from 1993 and a
// directory with two thousand entries in it was slow to open.

// objsDir is LIB_PLROBJS (db.h:37). The C resolves it against the mud's
// own cwd, which is lib/, so in an archived tree it sits beside etc/ rather
// than inside it — see player.Config.ObjectsDir, which is how a caller says
// so.
const objsDir = "plrobjs"

// objsSuffix is SUF_OBJS (db.h:45).
const objsSuffix = "objs"

// USE_AUTOEQ is 0 in this tree (structs.h:30), so `struct obj_file_elem` has
// no `location` member and auto_equip is never reached with anything but
// zero.
//
// The consequence is worth stating plainly, because it is a behaviour players
// noticed: **renting empties your bags and strips your body.** Everything you
// were wearing and everything inside your containers comes back loose in your
// inventory, in the order you had it. Crash_save still walks containers and
// still computes a location for each item, and the file still cannot record
// where any of it was.
const useAutoEQ = false

// rentInfo is `struct rent_info`, declared in order.
//
// Fourteen ints, eight of them spare. The comment above it in structs.h reads
// "BEWARE: Changing it will ruin rent files", which is why there are eight
// spares and why none of them was ever used.
var rentInfo = []field{
	{name: "time", kind: kInt},
	{name: "rentcode", kind: kInt},
	{name: "net_cost_per_diem", kind: kInt},
	{name: "gold", kind: kInt},
	{name: "account", kind: kInt},
	{name: "nitems", kind: kInt},
	{name: "spare0", kind: kInt},
	{name: "spare1", kind: kInt},
	{name: "spare2", kind: kInt},
	{name: "spare3", kind: kInt},
	{name: "spare4", kind: kInt},
	{name: "spare5", kind: kInt},
	{name: "spare6", kind: kInt},
	{name: "spare7", kind: kInt},
}

// objFileElem is `struct obj_file_elem`, declared in order.
//
// Note `time` in rent_info is an `int` and not a `time_t`, and `bitvector`
// here is a `long` where `extra_flags` beside it is an `int` — both of those
// are the C's, not a transcription slip. The `long` is why this struct's size
// changes between data models at all.
var objFileElem = []field{
	{name: "item_number", kind: kInt},
	// The `#if USE_AUTOEQ` location field would go here. It is not compiled.
	{name: "value", kind: kInt, count: game.NumObjValues},
	{name: "extra_flags", kind: kInt},
	{name: "weight", kind: kInt},
	{name: "timer", kind: kInt},
	{name: "bitvector", kind: kLong},
	{name: "affected", kind: kStruct, count: game.MaxObjAffects, fields: []field{
		{name: "location", kind: kChar},
		{name: "modifier", kind: kChar},
	}},
}

// objCodec reads and writes rent files under one data model.
type objCodec struct {
	header *layout
	elem   *layout
}

func newObjCodec(m dataModel) *objCodec {
	if useAutoEQ {
		panic("binary: USE_AUTOEQ is 0 in the reference tree; objFileElem has no location member")
	}
	return &objCodec{
		header: computeLayoutOf(rentInfo, m),
		elem:   computeLayoutOf(objFileElem, m),
	}
}

// decode reads a whole rent file.
//
// The C reads the header with one fread and then loops freading elements
// until feof, which means a file whose length is not a whole number of
// elements is read up to the last complete one and the remainder ignored.
// That is reproduced rather than treated as corruption: a rent file truncated
// by a full disk is exactly how it happens, and the objects before the tear
// are still good.
func (c *objCodec) decode(b []byte) (*player.RentFile, error) {
	if len(b) < c.header.Size {
		return nil, fmt.Errorf("rent file is %d bytes, too short for a %d-byte header",
			len(b), c.header.Size)
	}

	h := &codec{layout: c.header}
	f := &player.RentFile{
		Code:       player.RentCode(h.i32(b, "rentcode")),
		CostPerDay: h.i32(b, "net_cost_per_diem"),
		Gold:       h.i32(b, "gold"),
		Bank:       h.i32(b, "account"),
	}
	if secs := h.i32(b, "time"); secs != 0 {
		f.Written = time.Unix(int64(secs), 0).UTC()
	}

	e := &codec{layout: c.elem}
	for off := c.header.Size; off+c.elem.Size <= len(b); off += c.elem.Size {
		rec := b[off : off+c.elem.Size]
		obj := player.StoredObject{
			Vnum:       game.ObjVnum(e.i32(rec, "item_number")),
			ExtraFlags: game.Flags(e.i32(rec, "extra_flags")), //nolint:gosec // reinterpretation: the field is a bitvector
			Weight:     e.i32(rec, "weight"),
			Timer:      e.i32(rec, "timer"),
			PermAffect: game.Flags(e.varInt(rec, "bitvector")), //nolint:gosec // ditto
			Affects:    make([]game.ObjAffect, game.MaxObjAffects),
		}
		values := c.elem.at("value")
		for i := range obj.Values {
			obj.Values[i] = widen32(byteOrder.Uint32(rec[values.Offset+i*values.Stride:]))
		}
		affects := c.elem.at("affected")
		loc, mod := c.elem.at("affected.location"), c.elem.at("affected.modifier")
		for i := range obj.Affects {
			base := affects.Offset + i*affects.Stride
			obj.Affects[i] = game.ObjAffect{
				// `location` is a `byte` (unsigned) and `modifier` an
				// `sbyte`. An apply of 200 is not a thing, but reading the
				// two the way the struct declares them is free.
				Location: int32(rec[base+loc.Offset-affects.Offset]),
				Modifier: widen8(rec[base+mod.Offset-affects.Offset]),
			}
		}
		f.Objects = append(f.Objects, obj)
	}
	return f, nil
}

// encode writes a whole rent file.
func (c *objCodec) encode(f *player.RentFile) ([]byte, error) {
	b := make([]byte, c.header.Size+len(f.Objects)*c.elem.Size)

	h := &codec{layout: c.header}
	h.putI32(b, "rentcode", int32(f.Code))
	h.putI32(b, "net_cost_per_diem", f.CostPerDay)
	h.putI32(b, "gold", f.Gold)
	h.putI32(b, "account", f.Bank)
	h.putI32(b, "nitems", int32(len(f.Objects))) //nolint:gosec // an inventory, not a quantity that can overflow
	if err := putUnixSeconds(h, b, "time", f.Written); err != nil {
		return nil, err
	}

	e := &codec{layout: c.elem}
	for i, obj := range f.Objects {
		rec := b[c.header.Size+i*c.elem.Size:][:c.elem.Size]
		e.putI32(rec, "item_number", int32(obj.Vnum))
		e.putI32(rec, "extra_flags", int32(obj.ExtraFlags)) //nolint:gosec // reinterpretation: the field is a bitvector
		e.putI32(rec, "weight", obj.Weight)
		e.putI32(rec, "timer", obj.Timer)
		e.putVar(rec, "bitvector", int64(obj.PermAffect)) //nolint:gosec // reinterpretation: the field is a bitvector
		values := c.elem.at("value")
		for j := range obj.Values {
			byteOrder.PutUint32(rec[values.Offset+j*values.Stride:], narrow32(obj.Values[j]))
		}
		affects := c.elem.at("affected")
		loc, mod := c.elem.at("affected.location"), c.elem.at("affected.modifier")
		for j := 0; j < game.MaxObjAffects; j++ {
			var a game.ObjAffect
			if j < len(obj.Affects) {
				a = obj.Affects[j]
			}
			base := affects.Offset + j*affects.Stride
			rec[base+loc.Offset-affects.Offset] = narrowU8(a.Location)
			rec[base+mod.Offset-affects.Offset] = narrow8(a.Modifier)
		}
	}
	return b, nil
}

// putUnixSeconds writes a timestamp into an `int` field.
//
// rent_info.time is an int rather than a time_t, so it is four bytes under
// every data model and overflows in 2038 whatever the machine. Refusing
// beats writing a date in 1901, on the same reasoning as putTime.
func putUnixSeconds(c *codec, rec []byte, name string, t time.Time) error {
	if t.IsZero() {
		c.putI32(rec, name, 0)
		return nil
	}
	secs := t.Unix()
	if secs > 0x7fffffff || secs < -0x80000000 {
		return fmt.Errorf("%s is %s, which does not fit in this format's 32-bit timestamp (it overflows on 2038-01-19)",
			name, t.Format(time.RFC3339))
	}
	c.putI32(rec, name, int32(secs)) //nolint:gosec // range checked immediately above
	return nil
}

// --- the file store ---------------------------------------------------

// ObjectStore reads and writes the per-player rent files.
type ObjectStore struct {
	dir      string
	readOnly bool
	codec    *objCodec

	// mu guards the whole directory. Rent files are written on quit and on
	// the crash-save sweep, which is not often enough for anything finer to
	// be worth the risk of getting it wrong.
	mu sync.RWMutex
}

// NewObjectStore opens the rent files under a player data directory, or
// under cfg.ObjectsDir if the caller has said where they really are.
func NewObjectStore(cfg player.Config) (*ObjectStore, error) {
	if cfg.Dir == "" {
		return nil, fmt.Errorf("binary: no player directory configured")
	}
	return &ObjectStore{
		dir:      ObjectsPath(cfg),
		readOnly: cfg.ReadOnly,
		codec:    newObjCodec(ilp32),
	}, nil
}

// ObjectsPath is the plrobjs/ directory a configuration names: cfg.ObjectsDir
// when it is set, and Dir/plrobjs when it is not.
func ObjectsPath(cfg player.Config) string {
	if cfg.ObjectsDir != "" {
		return cfg.ObjectsDir
	}
	return filepath.Join(cfg.Dir, objsDir)
}

// pathFor is get_filename(name, ..., CRASH_FILE) (utils.c:518).
func (s *ObjectStore) pathFor(name string) (string, error) {
	return bucketedPath(s.dir, name, objsSuffix, "rent file")
}

// bucketedPath is get_filename (utils.c:518) itself, shared by every
// per-character file the C buckets this way: the rent files under plrobjs/
// and the alias files under plralias/ differ only in their directory and
// their suffix.
//
// The bucket is by first letter, in five ranges, with anything that does not
// start with a letter going to `ZZZ`. No name can reach ZZZ, because
// _parse_name rejects a name with a non-alphabetic character in it — but the
// C has the branch and so does this, since a hand-made file is still a file.
func bucketedPath(dir, name, suffix, what string) (string, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return "", fmt.Errorf("binary: empty character name")
	}
	// A name is used unescaped as a path component here, exactly as the C
	// does. The C gets away with it because _parse_name has already refused
	// anything but letters; this checks rather than trusting, because a
	// character called `../../etc/passwd` would otherwise be a file called
	// that.
	for _, r := range name {
		if r < 'a' || r > 'z' {
			return "", fmt.Errorf("binary: %q is not a valid character name for a %s", name, what)
		}
	}

	var bucket string
	switch {
	case name[0] <= 'e':
		bucket = "A-E"
	case name[0] <= 'j':
		bucket = "F-J"
	case name[0] <= 'o':
		bucket = "K-O"
	case name[0] <= 't':
		bucket = "P-T"
	default:
		bucket = "U-Z"
	}
	return filepath.Join(dir, bucket, name+"."+suffix), nil
}

// LoadObjects implements player.ObjectStore.
func (s *ObjectStore) LoadObjects(_ context.Context, name string) (*player.RentFile, error) {
	path, err := s.pathFor(name)
	if err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	b, err := os.ReadFile(path) //nolint:gosec // the path is built from a validated name
	if errors.Is(err, os.ErrNotExist) {
		return nil, player.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s's rent file: %w", name, err)
	}
	if len(b) == 0 {
		// "SYSERR: Crash_load: %s's rent file was empty!" (objsave.c:462).
		// The C logs it and returns the temple; here it is an error and the
		// caller does the same.
		return nil, fmt.Errorf("%s's rent file is empty", name)
	}
	f, err := s.codec.decode(b)
	if err != nil {
		return nil, fmt.Errorf("reading %s's rent file: %w", name, err)
	}
	return f, nil
}

// SaveObjects implements player.ObjectStore.
func (s *ObjectStore) SaveObjects(_ context.Context, name string, f *player.RentFile) error {
	if s.readOnly {
		return fmt.Errorf("binary: the player data is open read-only")
	}
	path, err := s.pathFor(name)
	if err != nil {
		return err
	}
	// struct obj_file_elem has no location member (USE_AUTOEQ is 0 — see
	// newObjCodec above), so nothing on disk can say what was inside what.
	// A copy is flattened for encoding rather than f itself, since f is the
	// caller's and this is the only format that needs to throw the shape
	// away — player.FlattenStoredObjects.
	flat := *f
	flat.Objects = player.FlattenStoredObjects(f.Objects)
	b, err := s.codec.encode(&flat)
	if err != nil {
		return fmt.Errorf("writing %s's rent file: %w", name, err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("writing %s's rent file: %w", name, err)
	}
	// Written to a temporary and renamed. The C fopen()s with "wb", which
	// truncates first, so a crash mid-write leaves a file that is neither the
	// old contents nor the new — and this is the crash-save path, where a
	// crash mid-write is the case that matters.
	if err := atomicfile.Write(path, b, 0o600); err != nil {
		return fmt.Errorf("writing %s's rent file: %w", name, err)
	}
	return nil
}

// DeleteObjects implements player.ObjectStore.
func (s *ObjectStore) DeleteObjects(_ context.Context, name string) error {
	if s.readOnly {
		return fmt.Errorf("binary: the player data is open read-only")
	}
	path, err := s.pathFor(name)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("deleting %s's rent file: %w", name, err)
	}
	return nil
}

// MarkCrashed implements player.ObjectStore.
//
// Crash_load rewinds the file it has just read and rewrites the header
// (objsave.c:617), leaving the objects in place. Doing it as a header-only
// rewrite rather than a re-encode of the whole file is not an optimisation:
// it means an object this port could not decode is still there for the C
// server to read.
func (s *ObjectStore) MarkCrashed(_ context.Context, name string, at time.Time) error {
	if s.readOnly {
		return fmt.Errorf("binary: the player data is open read-only")
	}
	path, err := s.pathFor(name)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	b, err := os.ReadFile(path) //nolint:gosec // the path is built from a validated name
	if errors.Is(err, os.ErrNotExist) {
		return player.ErrNotFound
	}
	if err != nil || len(b) < s.codec.header.Size {
		return fmt.Errorf("re-marking %s's rent file: %w", name, err)
	}

	h := &codec{layout: s.codec.header}
	h.putI32(b, "rentcode", int32(player.RentCrash))
	if err := putUnixSeconds(h, b, "time", at); err != nil {
		return err
	}
	if err := os.WriteFile(path, b, 0o600); err != nil { //nolint:gosec // the path is built from a validated name
		return fmt.Errorf("re-marking %s's rent file: %w", name, err)
	}
	return nil
}

// --- bare object sequences --------------------------------------------
//
// A house file is a sequence of `obj_file_elem` records with no header at all
// (house.c:126). Same record, same layout, no rent_info in front — so the
// codec is shared rather than written twice.

// EncodeStoredObjects writes objects as a bare sequence of records, with no
// header.
func EncodeStoredObjects(objs []player.StoredObject) ([]byte, error) {
	c := newObjCodec(ilp32)
	b, err := c.encode(&player.RentFile{Objects: objs})
	if err != nil {
		return nil, err
	}
	return b[c.header.Size:], nil
}

// DecodeStoredObjects reads a bare sequence of records.
//
// A trailing partial record is ignored, which is what the C's
// fread-until-feof loop does with one.
func DecodeStoredObjects(b []byte) ([]player.StoredObject, error) {
	c := newObjCodec(ilp32)
	// decode wants a header in front, and there is none. Prepending an empty
	// one is cheaper and less error-prone than a second decode path.
	with := make([]byte, c.header.Size, c.header.Size+len(b))
	with = append(with, b...)
	f, err := c.decode(with)
	if err != nil {
		return nil, err
	}
	return f.Objects, nil
}
