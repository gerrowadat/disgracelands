// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

// Package binary reads and writes the original CircleMUD player database:
// a flat file of fixed-size `struct char_file_u` records, written by a raw
// fwrite() of the struct.
//
// Because it is a raw struct dump, the file format *is* the struct's memory
// layout — every offset, every field width, and every byte of padding the
// compiler inserted. Reading it means reproducing that layout exactly, and a
// single wrong offset silently misreads everything after it while still
// producing plausible-looking numbers.
//
// The layout is not hard-coded here. It is computed from a declaration of the
// struct plus a data model (how wide `long`, `time_t` and pointers are), by
// the same alignment rules the System V ABI gives compilers. That makes the
// difference between the 32-bit format the real data is in and the 64-bit
// format a modern rebuild would produce a *parameter* rather than an
// assumption — which is the whole of docs/proposals/go-port-plan.md §4 in one
// place.
//
// The engine is checked against gcc: reference/tools/pfilelayout.c prints the
// offsets the compiler actually chose, and a test asserts this package
// computes the same ones. See layout_test.go.
package binary

import "fmt"

// A dataModel describes how wide the implementation-defined types are.
//
// These three are the entire difference between the format the archived
// database is in and the format a native build on any modern machine would
// write. Nothing else in the struct changes.
type dataModel struct {
	name string
	// longSize is sizeof(long).
	longSize int
	// timeSize is sizeof(time_t).
	timeSize int
	// ptrSize is sizeof(void *) — which matters because struct affected_type
	// ends in a `next` pointer and the array of them is written to disk.
	ptrSize int
}

// ilp32 is the model the real player database was written under: a 32-bit
// FreeBSD/i386 build, where long, time_t and pointers are all four bytes.
//
// This is the model that matters. Every archived record is in it.
var ilp32 = dataModel{name: "ilp32", longSize: 4, timeSize: 4, ptrSize: 4}

// lp64 is what a native build on a modern 64-bit machine produces. No real
// data exists in this layout; it is here so the engine can be checked against
// a compiler that can actually be run today, and so a file accidentally
// written by such a build can be recognised rather than misread.
var lp64 = dataModel{name: "lp64", longSize: 8, timeSize: 8, ptrSize: 8}

// kind is a C type as it appears in the struct declaration.
type kind int

const (
	kChar   kind = iota // char, and the byte/sbyte/ubyte/bool typedefs over it
	kShort              // sh_int, and the room_vnum typedef over it
	kInt                // int
	kLong               // long — width varies by model
	kTime               // time_t — width varies by model
	kPtr                // any pointer — width varies by model
	kStruct             // a nested struct, described by its own fields
)

// sizeAlign returns a scalar kind's size and alignment under a model.
//
// On both models of interest every scalar's alignment equals its size, which
// is what the System V ABI specifies for these types on i386 and x86-64. The
// one classic exception — 64-bit types being only 4-aligned on i386 — cannot
// arise here, because under ilp32 nothing in this struct is 8 bytes wide.
func (k kind) sizeAlign(m dataModel) (size, align int) {
	switch k {
	case kChar:
		return 1, 1
	case kShort:
		return 2, 2
	case kInt:
		return 4, 4
	case kLong:
		return m.longSize, m.longSize
	case kTime:
		return m.timeSize, m.timeSize
	case kPtr:
		return m.ptrSize, m.ptrSize
	}
	panic(fmt.Sprintf("binary: sizeAlign called on kind %d", k))
}

// field is one member of a C struct declaration.
type field struct {
	name string
	kind kind
	// count is the array length; 0 and 1 both mean a single value. A count
	// changes the size but never the alignment.
	count int
	// fields describes the members, for kStruct.
	fields []field
}

// placed is a field after layout: where it sits and how big it is.
type placed struct {
	Name   string
	Offset int
	Size   int
	// Stride is the element size for arrays, so a caller can walk one
	// without recomputing it. Equal to Size for a single value.
	Stride int
	Kind   kind
	// Fields holds the members of a nested struct, at absolute offsets.
	Fields []placed
}

// layout is a fully placed struct.
type layout struct {
	Model  dataModel
	Size   int
	Align  int
	Fields []placed
	// byName indexes every field, nested ones under "outer.inner".
	byName map[string]placed
}

// at returns a field by name, panicking if it is absent. Every name it is
// called with is a compile-time constant in this package, so absence is a
// programming error rather than a runtime condition.
func (l *layout) at(name string) placed {
	p, ok := l.byName[name]
	if !ok {
		panic("binary: no such field in layout: " + name)
	}
	return p
}

// alignUp rounds n up to the next multiple of a.
func alignUp(n, a int) int {
	if a <= 1 {
		return n
	}
	if r := n % a; r != 0 {
		return n + a - r
	}
	return n
}

// measure returns the total size, the alignment, and the per-element stride
// of one field under a model. An array's alignment is its element's; only its
// size changes.
func measure(f field, m dataModel) (size, align, stride int) {
	if f.kind == kStruct {
		stride, align = measureStruct(f.fields, m)
	} else {
		stride, align = f.kind.sizeAlign(m)
	}
	n := f.count
	if n < 1 {
		n = 1
	}
	return stride * n, align, stride
}

// measureStruct lays out a member list and returns the total size and
// alignment, following the C rules: each member is placed at the next offset
// satisfying its alignment, the struct's alignment is the widest member's,
// and the total size is rounded up to that alignment so arrays of the struct
// stay aligned.
func measureStruct(fields []field, m dataModel) (size, align int) {
	offset, maxAlign := 0, 1
	for _, f := range fields {
		sz, al, _ := measure(f, m)
		if al > maxAlign {
			maxAlign = al
		}
		offset = alignUp(offset, al)
		offset += sz
	}
	return alignUp(offset, maxAlign), maxAlign
}

// place lays out fields starting at base, appending each to index under the
// given name prefix.
func place(fields []field, m dataModel, base int, prefix string, index map[string]placed) ([]placed, int, int) {
	offset, maxAlign := base, 1
	out := make([]placed, 0, len(fields))

	for _, f := range fields {
		sz, al, stride := measure(f, m)
		if al > maxAlign {
			maxAlign = al
		}
		offset = alignUp(offset, al)

		p := placed{Name: f.name, Offset: offset, Size: sz, Stride: stride, Kind: f.kind}
		if f.kind == kStruct {
			// Nested members are indexed at absolute offsets under
			// "outer.inner", matching how pfilelayout.c names them.
			sub, _, _ := place(f.fields, m, offset, prefix+f.name+".", index)
			p.Fields = sub
		}
		index[prefix+f.name] = p
		out = append(out, p)
		offset += sz
	}

	return out, alignUp(offset-base, maxAlign), maxAlign
}

// computeLayout places the whole player record under a data model.
func computeLayout(m dataModel) *layout { return computeLayoutOf(charFileU, m) }

// computeLayoutOf places any declared struct under a data model. The player
// record is the big one, but the rent files (objfile.go) are raw struct dumps
// by exactly the same mechanism and get their offsets from here too.
func computeLayoutOf(decl []field, m dataModel) *layout {
	index := map[string]placed{}
	fields, size, align := place(decl, m, 0, "", index)
	return &layout{Model: m, Size: size, Align: align, Fields: fields, byName: index}
}

// leaf is one scalar or character array in the record, with arrays of structs
// expanded so every element appears.
type leaf struct {
	Offset int
	Size   int
	Kind   kind
}

// leaves enumerates every scalar in the record, in offset order.
//
// It exists to answer "which bytes of this record actually mean anything",
// which is not all of them: the gaps between fields are padding the compiler
// inserted and the C server never writes, because it fills a stack local
// field by field and fwrites the lot (db.c:2204). Those gaps hold whatever
// was on the stack.
func (l *layout) leaves() []leaf {
	var out []leaf
	var walk func(ps []placed)
	walk = func(ps []placed) {
		for _, p := range ps {
			if p.Kind != kStruct {
				out = append(out, leaf{Offset: p.Offset, Size: p.Size, Kind: p.Kind})
				continue
			}
			// An array of structs: repeat the element's members at each
			// stride. A single struct is the same thing with one element.
			n := p.Size / p.Stride
			for i := 0; i < n; i++ {
				shifted := make([]placed, len(p.Fields))
				for j, f := range p.Fields {
					f.Offset += i * p.Stride
					shifted[j] = f
				}
				walk(shifted)
			}
		}
	}
	walk(l.Fields)
	return out
}

// significantBytes reports, for one on-disk record, which bytes carry
// information.
//
// Two kinds of byte do not:
//
//   - Padding between and after fields. The C server fwrites an
//     uninitialised stack local (db.c:2204), so padding holds stack
//     residue that differs between two saves of the same character.
//   - Everything after the terminating NUL of a fixed-width string. The C
//     code strcpy()s into those buffers, so the tail is whatever the buffer
//     held before — in practice, fragments of other players' names.
//
// That makes a byte-for-byte round trip of this format impossible, and not
// because of any defect in a reader: those bytes are not part of the record.
// Comparing only the significant ones is the strongest check the format
// admits.
func (l *layout) significantBytes(rec []byte) []bool {
	sig := make([]bool, l.Size)
	for _, f := range l.leaves() {
		n := f.Size
		if f.Kind == kChar && f.Size > 1 {
			// A character array: significant up to and including its NUL.
			n = f.Size
			for i := 0; i < f.Size; i++ {
				if rec[f.Offset+i] == 0 {
					n = i + 1
					break
				}
			}
		}
		for i := 0; i < n; i++ {
			sig[f.Offset+i] = true
		}
	}
	return sig
}

// charFileU is `struct char_file_u` from structs.h, declared in order.
//
// Field order is the format. Anything reordered here reads a different file.
// The names match reference/tools/pfilelayout.c so the two can be compared
// mechanically.
var charFileU = []field{
	// struct char_player_data, inlined
	{name: "name", kind: kChar, count: maxNameLength + 1},
	{name: "description", kind: kChar, count: exdscrLength},
	{name: "title", kind: kChar, count: maxTitleLength + 1},
	{name: "sex", kind: kChar},
	{name: "chclass", kind: kChar},
	{name: "level", kind: kChar},
	{name: "hometown", kind: kShort},
	{name: "birth", kind: kTime},
	{name: "played", kind: kInt},
	{name: "weight", kind: kChar},
	{name: "height", kind: kChar},
	{name: "pwd", kind: kChar, count: maxPwdLength + 1},

	{name: "cs", kind: kStruct, fields: []field{
		{name: "alignment", kind: kInt},
		{name: "idnum", kind: kLong},
		{name: "act", kind: kLong},
		{name: "affected_by", kind: kLong},
		{name: "apply_saving_throw", kind: kShort, count: 5},
	}},

	{name: "ps", kind: kStruct, fields: []field{
		{name: "skills", kind: kChar, count: maxSkills + 1},
		{name: "PADDING0", kind: kChar},
		{name: "talks", kind: kChar, count: maxTongue},
		{name: "wimp_level", kind: kInt},
		{name: "freeze_level", kind: kChar},
		{name: "invis_level", kind: kShort},
		{name: "load_room", kind: kShort},
		{name: "pref", kind: kLong},
		{name: "bad_pws", kind: kChar},
		{name: "conditions", kind: kChar, count: 3},
		{name: "spare0", kind: kChar},
		{name: "spare1", kind: kChar},
		{name: "spare2", kind: kChar},
		{name: "spare3", kind: kChar},
		{name: "spare4", kind: kChar},
		{name: "spare5", kind: kChar},
		{name: "spells_to_learn", kind: kInt},
		{name: "remort_vector", kind: kInt},
		{name: "specflags", kind: kInt},
		{name: "olc_zone", kind: kInt},
		{name: "spare10", kind: kInt},
		{name: "spare11", kind: kInt},
		{name: "spare12", kind: kInt},
		{name: "spare13", kind: kInt},
		{name: "spare14", kind: kInt},
		{name: "spare15", kind: kInt},
		{name: "spare16", kind: kInt},
		{name: "spare17", kind: kLong},
		{name: "spare18", kind: kLong},
		{name: "spare19", kind: kLong},
		{name: "spare20", kind: kLong},
		{name: "spare21", kind: kLong},
	}},

	{name: "ab", kind: kStruct, fields: []field{
		{name: "str", kind: kChar},
		{name: "str_add", kind: kChar},
		{name: "intel", kind: kChar},
		{name: "wis", kind: kChar},
		{name: "dex", kind: kChar},
		{name: "con", kind: kChar},
		{name: "cha", kind: kChar},
	}},

	{name: "pt", kind: kStruct, fields: []field{
		{name: "mana", kind: kShort},
		{name: "max_mana", kind: kShort},
		{name: "hit", kind: kShort},
		{name: "max_hit", kind: kShort},
		{name: "move", kind: kShort},
		{name: "max_move", kind: kShort},
		{name: "armor", kind: kShort},
		{name: "gold", kind: kInt},
		{name: "bank_gold", kind: kInt},
		{name: "exp", kind: kInt},
		{name: "hitroll", kind: kChar},
		{name: "damroll", kind: kChar},
	}},

	// struct affected_type[MAX_AFFECT]. The trailing `next` pointer is
	// meaningless once written to a file, but it takes up space and its
	// width changes with the data model, so it is part of the format.
	{name: "affected", kind: kStruct, count: maxAffect, fields: []field{
		{name: "type", kind: kShort},
		{name: "duration", kind: kShort},
		{name: "modifier", kind: kChar},
		{name: "location", kind: kChar},
		{name: "bitvector", kind: kLong},
		{name: "next", kind: kPtr},
	}},

	{name: "last_logon", kind: kTime},
	{name: "host", kind: kChar, count: hostLength + 1},
}

// Sizes from structs.h, every one of them marked *DO*NOT*CHANGE* there
// because they are baked into the file format.
const (
	maxNameLength  = 20
	maxPwdLength   = 10
	maxTitleLength = 80
	hostLength     = 30
	exdscrLength   = 240
	maxTongue      = 3
	maxSkills      = 200
	maxAffect      = 32
)
