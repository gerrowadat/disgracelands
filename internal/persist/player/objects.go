// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package player

import (
	"context"
	"time"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// What a player was carrying when they left, ported from objsave.c.
//
// This is a second file per player, separate from the roster: `plrobjs/A-E/
// zod.objs`. It holds a header saying *why* it was written and what it costs
// to keep, followed by one fixed-size record per object.
//
// The C calls it the rent file and the crash file interchangeably, because it
// is both. Renting at an inn writes it; so does quitting, so does the server
// going down. The header's rent code is the difference, and it decides
// whether you come back where you left off or in the temple.

// A RentCode says why the file was written (structs.h:469).
type RentCode int32

const (
	// RentUndef is a file with no code, which should not happen and is
	// treated as a crash.
	RentUndef RentCode = 0
	// RentCrash is a quit, a link loss, or the server going down. Free, and
	// you come back in the temple.
	RentCrash RentCode = 1
	// RentRented is a stay at an inn, paid for by the day. You come back
	// where you left off.
	RentRented RentCode = 2
	// RentCryo is the cryogenic freezer: a one-off charge instead of a daily
	// one, and likewise you come back where you left off.
	RentCryo RentCode = 3
	// RentForced is a god renting somebody out.
	RentForced RentCode = 4
	// RentTimedOut is the idle-timeout auto-rent, charged at double.
	RentTimedOut RentCode = 5
)

// String names a rent code for logs.
func (r RentCode) String() string {
	switch r {
	case RentUndef:
		return "undefined"
	case RentCrash:
		return "crash"
	case RentRented:
		return "rented"
	case RentCryo:
		return "cryo"
	case RentForced:
		return "forced"
	case RentTimedOut:
		return "timed out"
	}
	return "unknown"
}

// KeepsLoadRoom reports whether coming back from this file leaves the
// character where they were.
//
// Crash_load returns 0 for RENT_RENTED and RENT_CRYO and 1 for everything
// else, and the caller reads 1 as "put them in the temple" (objsave.c:625).
// Paying for a bed is what buys you the right to wake up in it.
func (r RentCode) KeepsLoadRoom() bool { return r == RentRented || r == RentCryo }

// RentFile is a decoded rent file: the header and the objects.
type RentFile struct {
	// Code is why the file was written.
	Code RentCode
	// Written is when. Together with CostPerDay it is what the arrears are
	// computed from on the way back in.
	Written time.Time
	// CostPerDay is the daily charge, already doubled for a forced rent.
	CostPerDay int32
	// Gold and Bank are what the character had at the time. The C stores
	// them and then never reads them back — see the deviations note.
	Gold int32
	Bank int32
	// Objects are the saved items, in the order the file has them.
	Objects []StoredObject
}

// StoredObject is one `struct obj_file_elem`.
//
// It is strikingly little: a vnum and the handful of fields that can differ
// from the prototype. Everything else — the name, the description, the wear
// flags, the cost — comes back from the prototype at load time, which is why
// editing a zone file changes items players are already carrying.
type StoredObject struct {
	// Vnum identifies the prototype. An object whose prototype has since been
	// deleted is dropped on load, silently, as read_object returning NULL.
	Vnum game.ObjVnum

	Values     [game.NumObjValues]int32
	ExtraFlags game.Flags
	Weight     int32
	Timer      int32
	// PermAffect is obj_flags.bitvector: the AFF_* bits wearing it confers.
	PermAffect game.Flags
	// Affects are the apply slots. Exactly MaxObjAffect of them are stored,
	// including the empty ones.
	Affects []game.ObjAffect
}

// ObjectStore reads and writes rent files.
//
// Separate from Store because they are separate files with separate formats
// and separate failure modes: a roster that will not load stops the server,
// and a rent file that will not load costs one player their backpack. A
// format that implements one may implement the other.
type ObjectStore interface {
	// LoadObjects reads a character's rent file. A character who has none —
	// which is every character who has never left the game carrying anything
	// — gets ErrNotFound.
	LoadObjects(ctx context.Context, name string) (*RentFile, error)

	// SaveObjects writes one, replacing whatever was there.
	SaveObjects(ctx context.Context, name string, f *RentFile) error

	// DeleteObjects removes it. Renting with nothing to store deletes rather
	// than writing an empty file (Crash_idlesave, objsave.c:829).
	DeleteObjects(ctx context.Context, name string) error

	// MarkCrashed rewrites just the header's rent code and timestamp,
	// leaving the objects alone.
	//
	// Crash_load does this to the file it has just read (objsave.c:617) so
	// that a player who unrents and then crashes gets their things back
	// without paying twice — and so the same file cannot be un-rented twice.
	MarkCrashed(ctx context.Context, name string, at time.Time) error
}
