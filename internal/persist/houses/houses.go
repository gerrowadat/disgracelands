// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

// Package houses defines the pluggable house-control interface and the
// registry of implementations — the same shape internal/persist/world,
// internal/persist/player, internal/persist/bans, internal/persist/boards
// and internal/persist/mail use: `classic` (House_save_control's raw struct
// dump plus the per-room `<vnum>.house` object files, moved to internal/
// persist/houses/classic without a behaviour change) and `yaml`
// (docs/design/data-format.md §9's state/houses.yaml).
//
// A house's contents are stored in the same object-instance model the rent
// files use (see internal/persist/player.StoredObject) — §8 calls it "the
// shared object-instance schema used by corpses, house crash files and
// anything else that has to persist an object." Real containment (a bag's
// contents staying nested inside it) is not part of this: that was step 5's
// own explicitly-scoped, user-approved deviation for player rent files
// specifically, and extending it to house crash files is a separate
// decision nobody has made — LoadObjects/SaveObjects here always deal in a
// flat list, matching what internal/server/houses.go's loadHouseObjects
// already does with it (every entry lands loose in the room, never
// recursed into).
package houses

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/gerrowadat/disgracelands/internal/persist/player"
)

// MaxHouses and MaxGuests are the C's (house.h:1).
const (
	MaxHouses = 100
	MaxGuests = 10
)

// ModePrivate is HOUSE_PRIVATE, and the only mode there is: the switch in
// House_can_enter has one case and the field was never used for anything
// else.
const ModePrivate int32 = 0

// House is one control record, the model every format reads and writes.
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

// Store reads and writes house control records and their contents.
type Store interface {
	// Name returns the registered format name.
	Name() string
	// Close releases any held resources.
	Close() error

	// Load reads every control record.
	Load() ([]House, error)
	// Save writes every control record, replacing whatever was there.
	Save(houses []House) error

	// LoadObjects reads a house's contents. Returns nil for a house nobody
	// has left anything in.
	LoadObjects(vnum int32) ([]player.StoredObject, error)
	// SaveObjects writes a house's contents, or clears them when there are
	// none.
	SaveObjects(vnum int32, objs []player.StoredObject) error
	// DeleteObjects removes a house's contents entirely.
	DeleteObjects(vnum int32) error
}

// Config is what a factory needs to open a store.
type Config struct {
	// ControlPath is the house control file (classic only — house.c's
	// hcontrol has no counterpart in yaml, which folds control records
	// and contents into the one file ObjectDir names).
	ControlPath string
	// ObjectDir is the directory holding the per-room `<vnum>.house` files
	// (classic), or the yaml data directory (yaml).
	ObjectDir string
	// ReadOnly opens the store for inspection only.
	ReadOnly bool
}

// Factory opens a Store for a configuration.
type Factory func(cfg Config) (Store, error)

var (
	mu       sync.RWMutex
	registry = map[string]Factory{}
)

// Register adds a format. It panics on a duplicate name, which is a
// programming error discoverable at startup.
func Register(name string, f Factory) {
	mu.Lock()
	defer mu.Unlock()
	if _, dup := registry[name]; dup {
		panic("houses: duplicate format registered: " + name)
	}
	registry[name] = f
}

// Open creates a Store for the named format.
func Open(name string, cfg Config) (Store, error) {
	mu.RLock()
	f, ok := registry[name]
	mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown house format %q (have: %v)", name, Formats())
	}
	return f(cfg)
}

// Formats lists the registered format names, sorted.
func Formats() []string {
	mu.RLock()
	defer mu.RUnlock()
	names := make([]string, 0, len(registry))
	for n := range registry {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
