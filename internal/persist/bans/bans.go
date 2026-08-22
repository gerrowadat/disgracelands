// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

// Package bans defines the pluggable site-ban interface and the registry of
// implementations — the same shape internal/persist/world and
// internal/persist/player use, applied to the smallest of the "rest of the
// state" formats (docs/proposals/data-format.md §9): `classic` (ban.c's
// four-whitespace-fields-per-line text file, moved to internal/persist/
// bans/classic without a behaviour change) and `native` (docs/proposals/
// data-format.md §9's state/bans.yaml).
package bans

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// Type is how much of a site is banned (ban.h).
type Type int

const (
	// TypeNone is not banned.
	TypeNone Type = iota
	// TypeNew refuses new characters from the site; existing ones may still
	// log in.
	TypeNew
	// TypeSelect refuses everybody except characters flagged SITEOK.
	TypeSelect
	// TypeAll refuses the site outright.
	TypeAll
)

// typeNames are ban_types[] (ban.c:37). The trailing "ERROR" is the C's
// out-of-range answer and is never written to a file.
var typeNames = []string{"no", "new", "select", "all"}

// String names the type as the classic file spells it — reused by native's
// symbolic `type:` field too, since the names are already exactly the
// lower_snake_case-free single words §4.1's naming convention wants.
func (t Type) String() string {
	if t < 0 || int(t) >= len(typeNames) {
		return "ERROR"
	}
	return typeNames[t]
}

// ParseType reads a type name, as `ban` does from what a god typed, and as
// native's reader does from a file.
func ParseType(s string) (Type, bool) {
	for i, name := range typeNames {
		if strings.EqualFold(s, name) {
			return Type(i), true //nolint:gosec // an index into a fixed table
		}
	}
	return TypeNone, false
}

// MaxSiteLength is BANNED_SITE_LENGTH (ban.h), and classic truncates to it.
// native is not fixed-width and does not need to, but the limit is
// reported through Capabilities all the same, matching player.Store's
// posture: a format that cannot hold something says so rather than
// truncating silently.
const MaxSiteLength = 50

// Ban is one ban, the model every format reads and writes — the same
// pattern player.RentFile/StoredObject set: a persist-domain type shared
// by every implementation, kept here rather than duplicated per format.
type Ban struct {
	// Site is the substring matched against a connecting host, lower-cased.
	Site string
	Type Type
	// When the ban was made, and who by.
	When time.Time
	By   string
}

// Store reads and writes the site ban list.
//
// Implementations must be safe for concurrent use: the list is consulted on
// every connection.
type Store interface {
	// Name returns the registered format name.
	Name() string
	// Close releases any held resources.
	Close() error

	// List returns the bans, newest first.
	List() []Ban
	// Check is isbanned (ban.c:82): the worst ban matching this host.
	Check(host string) Type
	// Add records a ban, reporting false when the site is already listed.
	Add(ban Ban) (bool, error)
	// Remove is do_unban's half, reporting the ban that went.
	Remove(site string) (Ban, bool, error)
}

// Config is what a factory needs to open a store.
type Config struct {
	// Path is the ban file (classic) or data directory (native).
	Path string
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
		panic("bans: duplicate format registered: " + name)
	}
	registry[name] = f
}

// Open creates a Store for the named format.
func Open(name string, cfg Config) (Store, error) {
	mu.RLock()
	f, ok := registry[name]
	mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown ban format %q (have: %v)", name, Formats())
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
