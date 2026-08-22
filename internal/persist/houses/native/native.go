// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

// Package native implements docs/proposals/data-format.md §9's
// state/houses.yaml: one file holding every house's control record and its
// contents together, in place of classic's hcontrol array plus a
// `<vnum>.house` file per room. Contents reuse internal/persist/player/
// native's object-instance schema directly (ObjInstanceDoc et al.) — the
// same "shared schema" §8 describes — but always as a flat list: real
// containment (a bag's contents staying nested inside it) was step 5's own
// explicitly-scoped deviation for player rent files, and extending it to
// house crash files is a separate decision nobody has made, so a house's
// contents round-trip exactly as flat as classic's always have.
//
// One simplification relative to classic, noted rather than hidden: classic
// never deletes a `<vnum>.house` file just because its hcontrol entry goes
// away (Store.Save only ever touches the control file), so a house removed
// by the boot-time sanity checks in internal/server/houses.go leaves an
// orphaned object file behind forever. native's Save replaces the whole
// roster, control records and contents together — an orphan here is
// dropped rather than kept, since there is no separate file for it to keep
// unnoticed in the first place. That is a real, if narrow, behaviour
// difference between the formats, not something a round trip within one of
// them would ever expose.
package native

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/goccy/go-yaml"

	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/persist/houses"
	"github.com/gerrowadat/disgracelands/internal/persist/player"
	playernative "github.com/gerrowadat/disgracelands/internal/persist/player/native"
)

// FormatName is the name this format registers under.
const FormatName = "native"

// houseSchema is the document's schema tag, docs/proposals/data-format.md
// §10.1.
const houseSchema = "dl/houses@1"

// fileName is state/houses.yaml under whatever directory Config.ObjectDir
// names — native folds hcontrol and the per-room object files into the one
// path, so Config.ControlPath is unused here (see houses.Config's own doc
// comment).
const fileName = "houses.yaml"

func init() {
	houses.Register(FormatName, func(cfg houses.Config) (houses.Store, error) {
		return New(cfg)
	})
}

type doc struct {
	Schema string     `yaml:"schema"`
	Houses []houseDoc `yaml:"houses,omitempty"`
}

type houseDoc struct {
	Vnum        int32                         `yaml:"vnum"`
	Atrium      int32                         `yaml:"atrium"`
	Exit        string                        `yaml:"exit"`
	Mode        int32                         `yaml:"mode,omitempty"`
	Owner       int64                         `yaml:"owner"`
	Built       string                        `yaml:"built,omitempty"`
	LastPayment string                        `yaml:"last_payment,omitempty"`
	Guests      []int64                       `yaml:"guests,omitempty"`
	Contents    []playernative.ObjInstanceDoc `yaml:"contents,omitempty"`
}

type houseEntry struct {
	house    houses.House
	contents []player.StoredObject
}

// Store keeps every house in one YAML file, <dir>/houses.yaml.
type Store struct {
	path     string
	readOnly bool

	mu      sync.RWMutex
	entries map[int32]*houseEntry
}

// New opens the house file. A missing file is not an error: a server with
// no houses on it, matching classic.
func New(cfg houses.Config) (*Store, error) {
	if cfg.ObjectDir == "" {
		return nil, fmt.Errorf("houses: no directory configured")
	}
	s := &Store{path: filepath.Join(cfg.ObjectDir, fileName), readOnly: cfg.ReadOnly}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

// Name implements houses.Store.
func (s *Store) Name() string { return FormatName }

// Close implements houses.Store.
func (s *Store) Close() error { return nil }

func (s *Store) load() error {
	s.entries = map[int32]*houseEntry{}

	b, err := os.ReadFile(s.path) //nolint:gosec // a configured path
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading %s: %w", s.path, err)
	}

	var d doc
	if err := yaml.UnmarshalWithOptions(b, &d, yaml.Strict()); err != nil {
		return fmt.Errorf("reading %s: %w", s.path, err)
	}

	for _, hd := range d.Houses {
		dir, ok := game.ParseDirection(hd.Exit)
		if !ok {
			return fmt.Errorf("%s: house #%d: unknown exit %q", s.path, hd.Vnum, hd.Exit)
		}
		h := houses.House{
			Vnum: hd.Vnum, Atrium: hd.Atrium, ExitNum: int32(dir),
			Mode: hd.Mode, Owner: hd.Owner, Guests: append([]int64(nil), hd.Guests...),
		}
		if hd.Built != "" {
			built, err := time.Parse(time.RFC3339, hd.Built)
			if err != nil {
				return fmt.Errorf("%s: house #%d: %w", s.path, hd.Vnum, err)
			}
			h.BuiltOn = built.UTC()
		}
		if hd.LastPayment != "" {
			paid, err := time.Parse(time.RFC3339, hd.LastPayment)
			if err != nil {
				return fmt.Errorf("%s: house #%d: %w", s.path, hd.Vnum, err)
			}
			h.LastPayment = paid.UTC()
		}

		var contents []player.StoredObject
		for _, od := range hd.Contents {
			st, unknown := playernative.StoredObjectFromDoc(od)
			if len(unknown) > 0 {
				return fmt.Errorf("%s: house #%d: %s", s.path, hd.Vnum, unknown[0])
			}
			contents = append(contents, st)
		}
		s.entries[hd.Vnum] = &houseEntry{house: h, contents: contents}
	}
	return nil
}

// Load implements houses.Store, sorted by vnum for a deterministic file.
func (s *Store) Load() ([]houses.House, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]houses.House, 0, len(s.entries))
	for _, e := range s.entries {
		out = append(out, e.house)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Vnum < out[j].Vnum })
	return out, nil
}

// Save implements houses.Store: replaces the whole roster. See the package
// comment for how this differs from classic's own leave-the-orphan-file
// behaviour.
func (s *Store) Save(list []houses.House) error {
	if s.readOnly {
		return fmt.Errorf("houses: the data directory is open read-only")
	}

	s.mu.Lock()
	next := make(map[int32]*houseEntry, len(list))
	for _, h := range list {
		e := &houseEntry{house: h}
		if old, ok := s.entries[h.Vnum]; ok {
			e.contents = old.contents
		}
		next[h.Vnum] = e
	}
	s.entries = next
	s.mu.Unlock()

	return s.save()
}

// LoadObjects implements houses.Store.
func (s *Store) LoadObjects(vnum int32) ([]player.StoredObject, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.entries[vnum]
	if !ok {
		return nil, nil
	}
	return append([]player.StoredObject(nil), e.contents...), nil
}

// SaveObjects implements houses.Store, or clears them when there are none.
// A vnum with no control record yet gets one created empty rather than
// being refused — SaveHouse can run before the control file's own next
// save, and there is nowhere here to lose the objects to in the meantime.
func (s *Store) SaveObjects(vnum int32, objs []player.StoredObject) error {
	if s.readOnly {
		return fmt.Errorf("houses: the data directory is open read-only")
	}

	s.mu.Lock()
	e, ok := s.entries[vnum]
	if !ok {
		e = &houseEntry{house: houses.House{Vnum: vnum}}
		s.entries[vnum] = e
	}
	e.contents = append([]player.StoredObject(nil), objs...)
	s.mu.Unlock()

	return s.save()
}

// DeleteObjects implements houses.Store.
func (s *Store) DeleteObjects(vnum int32) error {
	if s.readOnly {
		return fmt.Errorf("houses: the data directory is open read-only")
	}

	s.mu.Lock()
	if e, ok := s.entries[vnum]; ok {
		e.contents = nil
	}
	s.mu.Unlock()

	return s.save()
}

func (s *Store) save() error {
	s.mu.RLock()
	d := doc{Schema: houseSchema}
	vnums := make([]int32, 0, len(s.entries))
	for vnum := range s.entries {
		vnums = append(vnums, vnum)
	}
	sort.Slice(vnums, func(i, j int) bool { return vnums[i] < vnums[j] })
	for _, vnum := range vnums {
		e := s.entries[vnum]
		hd := houseDoc{
			Vnum: e.house.Vnum, Atrium: e.house.Atrium, Exit: game.Direction(e.house.ExitNum).String(), //nolint:gosec // a small enum
			Mode: e.house.Mode, Owner: e.house.Owner, Guests: e.house.Guests,
		}
		if !e.house.BuiltOn.IsZero() {
			hd.Built = e.house.BuiltOn.UTC().Format(time.RFC3339)
		}
		if !e.house.LastPayment.IsZero() {
			hd.LastPayment = e.house.LastPayment.UTC().Format(time.RFC3339)
		}
		for _, obj := range e.contents {
			hd.Contents = append(hd.Contents, playernative.ObjInstanceDocFrom(obj))
		}
		d.Houses = append(d.Houses, hd)
	}
	s.mu.RUnlock()

	out, err := yaml.MarshalWithOptions(d, yaml.Indent(2))
	if err != nil {
		return fmt.Errorf("writing %s: %w", s.path, err)
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o750); err != nil {
		return fmt.Errorf("writing %s: %w", s.path, err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, out, 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", s.path, err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("writing %s: %w", s.path, err)
	}
	return nil
}
