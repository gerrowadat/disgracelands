// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

// Package boards defines the pluggable bulletin-board interface and the
// registry of implementations — the same shape internal/persist/world,
// internal/persist/player and internal/persist/bans use: `classic`
// (Board_save_board/Board_load_board's raw struct dump, moved to
// internal/persist/boards/classic without a behaviour change) and `yaml`
// (docs/design/data-format.md §9's state/boards.yaml).
//
// Board *definitions* — which vnum, which levels, which file — are not part
// of this package: internal/game/board.go's BoardDef table is a hardcoded
// Go table, matching the C's own compiled-in board_info[] (boards.c is not
// data-driven either), so there is nothing here for a format to convert.
package boards

import (
	"errors"
	"fmt"
	"sort"
	"sync"
)

// ErrNotFound is returned for a board with no file, which is every board
// nobody has posted to yet.
var ErrNotFound = errors.New("no such board file")

// Message is one post as a format holds it.
type Message struct {
	// Heading is the whole formatted line the C built at post time.
	Heading string
	Level   int32
	Body    string
}

// Store reads and writes bulletin board files.
type Store interface {
	// Name returns the registered format name.
	Name() string
	// Close releases any held resources.
	Close() error

	// Load reads one board's messages, by the name in its BoardDef.File.
	Load(name string) ([]Message, error)
	// Save writes one board's messages, or removes it when there are none.
	Save(name string, msgs []Message) error
}

// Config is what a factory needs to open a store.
type Config struct {
	// Dir is the board-data directory.
	Dir string
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
		panic("boards: duplicate format registered: " + name)
	}
	registry[name] = f
}

// Open creates a Store for the named format.
func Open(name string, cfg Config) (Store, error) {
	mu.RLock()
	f, ok := registry[name]
	mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown board format %q (have: %v)", name, Formats())
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
