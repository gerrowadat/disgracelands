// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

// Package world defines the pluggable world-data interface and the registry
// of implementations.
//
// Implementations are compiled in and selected by name at runtime, not loaded
// through Go's plugin package — plugins cannot be combined with the
// CGO_ENABLED=0 static build the container image needs. See
// docs/design/go-port-plan.md §5.1.
package world

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// Source reads world data. Implementations must be safe to call from one
// goroutine at a time; nothing calls them concurrently.
type Source interface {
	// Name returns the registered format name, for logs and errors.
	Name() string

	// Load reads the entire world. Returning a partially populated world
	// alongside an error is allowed and expected: the linter reports every
	// problem it can find rather than stopping at the first.
	Load(ctx context.Context) (*game.World, error)

	// Close releases any held resources.
	Close() error
}

// Sink is implemented by formats that can write world data back. Online
// building (OasisOLC's descendants) requires it; a read-only source — a
// tarball, an embedded filesystem — simply does not implement it, and the
// building commands report that the world is read-only instead of failing
// halfway through a write.
type Sink interface {
	Source

	// WriteZone writes one zone and everything whose vnum falls inside it.
	WriteZone(ctx context.Context, zone *game.ZoneDef, w *game.World) error
}

// Config is what a factory needs to open a source.
type Config struct {
	// Dir is the world-data directory.
	Dir string
	// Mini selects the reduced index used by --mini-mud.
	Mini bool
}

// Factory opens a Source for a given configuration.
type Factory func(cfg Config) (Source, error)

var (
	mu       sync.RWMutex
	registry = map[string]Factory{}
)

// Register adds a format to the registry. It panics on a duplicate name,
// because that is a programming error discoverable at startup rather than a
// runtime condition. Implementations call this from an init function.
func Register(name string, f Factory) {
	mu.Lock()
	defer mu.Unlock()
	if _, dup := registry[name]; dup {
		panic("world: duplicate format registered: " + name)
	}
	registry[name] = f
}

// Open creates a Source for the named format.
func Open(name string, cfg Config) (Source, error) {
	mu.RLock()
	f, ok := registry[name]
	mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown world format %q (have: %v)", name, Formats())
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
