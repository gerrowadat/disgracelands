// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

// Package reports defines the pluggable bug/idea/typo report interface and
// the registry of implementations, the same shape internal/persist/bans
// (and boards, mail, houses) use: `classic` (do_gen_write's three
// append-only text logs, act.other.c:867-924) and `native` (docs/proposals/
// data-format.md §9's state/reports.yaml).
package reports

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

// Kind is which of the three reports this is — SCMD_BUG/SCMD_TYPO/SCMD_IDEA
// (interpreter.h:167-169), spelled out as the command name rather than the
// C's arbitrary subcommand numbers.
type Kind string

const (
	KindBug  Kind = "bug"
	KindIdea Kind = "idea"
	KindTypo Kind = "typo"
)

// DefaultMaxFileSize is max_filesize (config.c:233): do_gen_write refuses to
// append once a report file reaches this many bytes. Out of scope for the
// game-config refactor (docs/proposals/data-format.md §11 step 6b survey) —
// kept as a plain constant like every other scattered C tuning value in
// this tree, ported alone rather than waiting on that refactor.
const DefaultMaxFileSize = 50000

// Report is one bug/idea/typo submission, the model every format reads and
// writes.
type Report struct {
	Kind     Kind
	Reporter string
	// Room is the vnum the reporter was standing in when they wrote it
	// (GET_ROOM_VNUM(IN_ROOM(ch)), act.other.c:918).
	Room int32
	Body string
	// When is when it was written. Zero for a report read back from
	// classic: asctime's own 6-character slice in that format holds only a
	// month and day, with no year, so a full timestamp cannot be
	// reconstructed from it — the same "omitempty, matches classic's
	// own... case" posture bans/native's When field already takes,
	// reused here rather than inventing a false precision.
	When time.Time
}

// Store reads and writes bug/idea/typo reports.
//
// Implementations must be safe for concurrent use: a report can be filed by
// any connected character at any time.
type Store interface {
	// Name returns the registered format name.
	Name() string
	// Close releases any held resources.
	Close() error

	// Append adds a report, reporting false when the destination is full —
	// do_gen_write's own stat()-before-append gate (act.other.c:908-911),
	// which the game shows the player as a refusal rather than an error.
	Append(r Report) (bool, error)
	// All is a non-destructive read of every report, for `dlctl state
	// import`/`fmt` and for anything that wants to see the backlog at once.
	All() ([]Report, error)
}

// Config is what a factory needs to open a store.
type Config struct {
	// Dir holds classic's three files (bugs/ideas/typos) or native's one
	// reports.yaml.
	Dir      string
	ReadOnly bool
	// MaxFileSize gates classic's per-kind file size. Zero means
	// DefaultMaxFileSize.
	MaxFileSize int64
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
		panic("reports: duplicate format registered: " + name)
	}
	registry[name] = f
}

// Open creates a Store for the named format.
func Open(name string, cfg Config) (Store, error) {
	mu.RLock()
	f, ok := registry[name]
	mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown reports format %q (have: %v)", name, Formats())
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
