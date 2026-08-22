// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

// Package mail defines the pluggable mud-mail interface and the registry of
// implementations — the same shape internal/persist/world, internal/
// persist/player, internal/persist/bans and internal/persist/boards use:
// `classic` (mail.c's block-allocator file, moved to internal/persist/
// mail/classic without a behaviour change) and `native` (docs/proposals/
// data-format.md §9's state/mail.yaml).
package mail

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

// MaxMailSize is MAX_MAIL_SIZE (mail.h:26).
const MaxMailSize = 4096

// Message is one piece of mail, the model every format reads and writes.
type Message struct {
	To   int64
	From int64
	Sent time.Time
	Text string
}

// Store reads and writes mud mail.
type Store interface {
	// Name returns the registered format name.
	Name() string
	// Close releases any held resources.
	Close() error

	// HasMail reports whether anything is waiting for this player.
	HasMail(recipient int64) bool
	// Send stores a message.
	Send(m Message) error
	// Receive takes one message for a player, oldest first.
	Receive(recipient int64) (Message, bool, error)
	// All is a non-destructive read of every message in the store, for
	// tooling that needs to see everything at once (`dlctl state import`,
	// chiefly) without draining anyone's inbox to do it.
	All() []Message
}

// Config is what a factory needs to open a store.
type Config struct {
	// Path is the mail file (classic) or data directory (native).
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
		panic("mail: duplicate format registered: " + name)
	}
	registry[name] = f
}

// Open creates a Store for the named format.
func Open(name string, cfg Config) (Store, error) {
	mu.RLock()
	f, ok := registry[name]
	mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown mail format %q (have: %v)", name, Formats())
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
