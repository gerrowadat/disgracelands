// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

// Package native implements docs/proposals/data-format.md §9's
// state/boards.yaml: every board's messages in one file, keyed by the name
// classic's Store used as a filename (BoardDef.File, e.g. "board.mort") —
// there is no reason for six tiny boards to be six tiny files. classic's
// raw `struct board_msginfo` dump (a live pointer written to disk, whose
// *width* decides where everything after it lands) has no counterpart here
// at all: a message is just a heading, a level and a body.
package native

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/goccy/go-yaml"

	"github.com/gerrowadat/disgracelands/internal/persist/boards"
	worldtext "github.com/gerrowadat/disgracelands/internal/persist/world/native"
)

// NestedText is the world format's own block-scalar/quoting logic
// (docs/proposals/data-format.md §10.3, §4.6), reused rather than
// duplicated — the same choice internal/persist/player/native made for
// descriptions. A board message's body needs it for the same reason a
// player's description does and a bare Go string field does not: it is
// CRLF-joined in memory (session.handleEditing's strings.Join(lines,
// "\r\n"), the same convention world descriptions use), and nested three
// levels deep (boards -> name -> list -> body), well past the one
// structurally-guaranteed depth Text is safe at — confirmed the hard way,
// not assumed: a bare string field here round-tripped "Hello.\r\n" as
// "Hello.\n" through goccy/go-yaml's own default block-scalar writer,
// which mishandles an embedded \r outside the controlled Text/NestedText
// path the same way §12's "keep chomping" finding already documented for
// a different symptom of the same underlying library behaviour.
type NestedText = worldtext.NestedText

// FormatName is the name this format registers under.
const FormatName = "native"

// boardSchema is the document's schema tag, docs/proposals/data-format.md
// §10.1.
const boardSchema = "dl/boards@1"

// fileName is state/boards.yaml under whatever directory Config.Dir names.
const fileName = "boards.yaml"

func init() {
	boards.Register(FormatName, func(cfg boards.Config) (boards.Store, error) {
		return New(cfg)
	})
}

type doc struct {
	Schema string                  `yaml:"schema"`
	Boards map[string][]messageDoc `yaml:"boards,omitempty"`
}

type messageDoc struct {
	Heading string     `yaml:"heading"`
	Level   int32      `yaml:"level,omitempty"`
	Body    NestedText `yaml:"body,omitempty"`
}

// Store keeps every board's messages in one YAML file, <dir>/boards.yaml.
// Held in memory and rewritten whole on any Save — the same posture
// classic's per-file Store has, just with one file instead of several,
// since the whole archive's boards together are a handful of messages.
type Store struct {
	path     string
	readOnly bool

	mu     sync.RWMutex
	boards map[string][]boards.Message
}

// New opens the board file. A missing file is not an error: no board has
// been posted to yet, which is every fresh install.
func New(cfg boards.Config) (*Store, error) {
	if cfg.Dir == "" {
		return nil, fmt.Errorf("boards: no directory configured")
	}
	s := &Store{path: filepath.Join(cfg.Dir, fileName), readOnly: cfg.ReadOnly}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

// Name implements boards.Store.
func (s *Store) Name() string { return FormatName }

// Close implements boards.Store.
func (s *Store) Close() error { return nil }

func (s *Store) load() error {
	b, err := os.ReadFile(s.path) //nolint:gosec // a configured path
	if os.IsNotExist(err) {
		s.boards = map[string][]boards.Message{}
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading %s: %w", s.path, err)
	}

	var d doc
	if err := yaml.UnmarshalWithOptions(b, &d, yaml.Strict()); err != nil {
		return fmt.Errorf("reading %s: %w", s.path, err)
	}

	s.boards = make(map[string][]boards.Message, len(d.Boards))
	for name, msgs := range d.Boards {
		out := make([]boards.Message, 0, len(msgs))
		for _, m := range msgs {
			out = append(out, boards.Message{
				Heading: m.Heading, Level: m.Level, Body: worldtext.FromStored(string(m.Body)),
			})
		}
		s.boards[name] = out
	}
	return nil
}

// Load implements boards.Store.
func (s *Store) Load(name string) ([]boards.Message, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	msgs, ok := s.boards[name]
	if !ok {
		return nil, boards.ErrNotFound
	}
	return append([]boards.Message(nil), msgs...), nil
}

// Save implements boards.Store, or removes the board's entry entirely when
// there are none, matching classic's own "an emptied board leaves no
// trace" rule (Board_save_board).
func (s *Store) Save(name string, msgs []boards.Message) error {
	if s.readOnly {
		return fmt.Errorf("boards: the data directory is open read-only")
	}

	s.mu.Lock()
	if len(msgs) == 0 {
		delete(s.boards, name)
	} else {
		s.boards[name] = append([]boards.Message(nil), msgs...)
	}
	s.mu.Unlock()

	return s.save()
}

func (s *Store) save() error {
	s.mu.RLock()
	d := doc{Schema: boardSchema}
	if len(s.boards) > 0 {
		d.Boards = make(map[string][]messageDoc, len(s.boards))
		for name, msgs := range s.boards {
			out := make([]messageDoc, 0, len(msgs))
			for _, m := range msgs {
				out = append(out, messageDoc{
					Heading: m.Heading, Level: m.Level, Body: NestedText(worldtext.ToStored(m.Body)),
				})
			}
			d.Boards[name] = out
		}
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
