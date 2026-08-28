// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

// Package player defines the pluggable player-data interface and the
// registry of implementations.
//
// Same shape as internal/persist/world: implementations are compiled in and
// selected by name, not loaded through Go's plugin package, which cannot be
// combined with the static container build. See
// docs/proposals/go-port-plan.md §5.
package player

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"sort"
	"sync"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// ErrNotFound is returned by Load for a character that does not exist.
var ErrNotFound = errors.New("no such character")

// Store reads and writes player records.
//
// Implementations must be safe for concurrent use: the plan's concurrency
// model keeps world mutation on one goroutine, but saves are deliberately
// pushed off it so a slow disk cannot stall the game (§3.1).
type Store interface {
	// Name returns the registered format name.
	Name() string

	// Load reads one character by name. Names are matched
	// case-insensitively, because that is how players type them and how
	// every existing format stores them.
	Load(ctx context.Context, name string) (*game.PlayerRecord, error)

	// Save writes one character, creating it if necessary.
	Save(ctx context.Context, rec *game.PlayerRecord) error

	// Exists reports whether a character is present, without the cost of
	// decoding them. The login sequence asks this before anything else.
	Exists(ctx context.Context, name string) (bool, error)

	// Delete removes a character.
	Delete(ctx context.Context, name string) error

	// List enumerates every character. The iterator yields an error entry
	// rather than stopping, so one corrupt record does not hide the rest.
	List(ctx context.Context) iter.Seq2[IndexEntry, error]

	// Capabilities describes what this format can represent, so a caller can
	// find out before writing rather than after truncating.
	Capabilities() Capabilities

	// Close releases any held resources.
	Close() error
}

// IndexEntry is the summary of a character that listing produces, cheap
// enough to build for every record in the roster.
type IndexEntry struct {
	// Name is lower-cased, matching the C's own player_table: boot_db
	// lowercases every name as it builds it (db.c:607), and
	// get_name_by_id reads that table — which is why the mud mail header
	// has always shouted in lower case (see internal/server/mail.go).
	//
	// Every format has to agree about this, and for a long time they did
	// not: ascii lowercases as it writes its plr_index, binary returned
	// the record's own stored case, and yaml returned the document's. It
	// was invisible while the server only ever ran on ascii, and became a
	// wrong mail header the moment internal/server's tests moved onto
	// yaml (docs/proposals/yaml-only.md §5.4).
	Name  string
	IDNum int64
	Level int32
	// Flags carries the PLR_* bits, which is what `autowiz` and the deleted
	// check need without loading the whole record.
	Flags game.Flags
}

// Capabilities describes a format's limits.
//
// These exist so lossiness is something a caller can ask about rather than
// discover. The plan is explicit that saving a record a format cannot
// represent must be a loud failure, not a silent truncation (§5.1) — a
// truncated name is a different character.
type Capabilities struct {
	// MaxNameLength is the longest character name the format can store, or 0
	// for no limit.
	MaxNameLength int
	// MaxTitleLength and MaxDescriptionLength likewise.
	MaxTitleLength       int
	MaxDescriptionLength int
	// MaxAffects is the number of simultaneous spell effects storable, or 0
	// for no limit.
	MaxAffects int
	// MaxSkillNumber is the highest skill or spell number, or 0 for no limit.
	MaxSkillNumber int

	// CredentialSchemes lists the password schemes the format can store. The
	// binary format can hold only a DES crypt(3) hash, which is why moving
	// off it is a prerequisite for modern password hashing rather than an
	// independent improvement.
	CredentialSchemes []game.CredentialScheme

	// TimestampsOverflowIn2038 is true when the format stores times as 32-bit
	// seconds. Not a limit anyone can fix while staying compatible, but one
	// worth being able to report.
	TimestampsOverflowIn2038 bool
}

// Supports reports whether the format can store a credential of this scheme.
func (c Capabilities) Supports(scheme game.CredentialScheme) bool {
	for _, s := range c.CredentialSchemes {
		if s == scheme {
			return true
		}
	}
	return false
}

// Config is what a factory needs to open a store.
type Config struct {
	// Dir is the player-data directory. What a format makes of it is its own
	// business: the binary format expects a single `players` file inside it,
	// the ascii format a tree of one file per character.
	Dir string
	// ObjectsDir is the `plrobjs/` directory holding the rent and crash
	// files, for the formats that keep them in separate files. Empty means
	// `Dir/plrobjs`, which is this port's own layout — one directory holding
	// a roster and the rent files that go with it.
	//
	// It is a separate setting because the C's layout is not that one. The
	// C runs with its cwd set to lib/ and builds both paths from there:
	// `etc/players` (PLAYER_FILE, db.h) and `plrobjs/` (LIB_PLROBJS,
	// db.h:37). So in an archived lib/ the rent files are a *sibling* of the
	// directory the roster is in, not a child of it, and a tool pointed at
	// the roster the way `dlctl convert --type=pfile --from-dir=lib/etc` is finds
	// no rent files at all unless it is told where they are.
	ObjectsDir string
	// AliasDir is the `plralias/` directory holding the per-character alias
	// files (alias.c). Empty means `Dir/plralias`, and it is a separate
	// setting for exactly the reason ObjectsDir is: LIB_PLRALIAS (db.h:38)
	// is resolved against the mud's own cwd, so in an archived lib/ it is a
	// sibling of the roster's directory rather than a child of it.
	AliasDir string
	// ReadOnly opens the store for inspection only, so a tool that only
	// reads cannot damage a roster by accident.
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
		panic("player: duplicate format registered: " + name)
	}
	registry[name] = f
}

// Open creates a Store for the named format.
func Open(name string, cfg Config) (Store, error) {
	mu.RLock()
	f, ok := registry[name]
	mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown player format %q (have: %v)", name, Formats())
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
