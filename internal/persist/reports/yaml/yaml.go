// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

// Package yaml implements docs/design/data-format.md §9's
// state/reports.yaml: bugs, ideas and typos as one flat YAML list with a
// `kind` field, rather than classic's three separate append-only text
// files split by kind instead.
package yaml

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/goccy/go-yaml"

	"github.com/gerrowadat/disgracelands/internal/persist/yamlenc"

	"github.com/gerrowadat/disgracelands/internal/persist/reports"
)

// FormatName is the name this format registers under.
const FormatName = "yaml"

// reportSchema is the document's schema tag (data-format.md §10.1).
const reportSchema = "dl/reports@1"

// fileName is state/reports.yaml under whatever directory Config.Dir names.
const fileName = "reports.yaml"

func init() {
	reports.Register(FormatName, func(cfg reports.Config) (reports.Store, error) {
		return New(cfg)
	})
}

type doc struct {
	Schema  string      `yaml:"schema"`
	Reports []reportDoc `yaml:"reports,omitempty"`
}

type reportDoc struct {
	Kind     string `yaml:"kind"`
	Reporter string `yaml:"reporter"`
	Room     int32  `yaml:"room,omitempty"`
	Body     string `yaml:"body"`
	// When is RFC 3339, empty for a report imported from classic — see
	// reports.Report.When's own doc comment.
	When string `yaml:"when,omitempty"`
}

// Store keeps the reports as one YAML file, <dir>/reports.yaml. Held in
// memory and rewritten whole, same posture as bans/yaml: append-only in
// practice, and a report log stays small for the same reason a ban list
// does.
type Store struct {
	path     string
	readOnly bool

	mu      sync.RWMutex
	reports []reports.Report
}

// New opens the report file. A missing file is not an error: nobody has
// filed one yet.
func New(cfg reports.Config) (*Store, error) {
	if cfg.Dir == "" {
		return nil, fmt.Errorf("reports: no directory configured")
	}
	s := &Store{path: filepath.Join(cfg.Dir, fileName), readOnly: cfg.ReadOnly}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

// Name implements reports.Store.
func (s *Store) Name() string { return FormatName }

// Close implements reports.Store.
func (s *Store) Close() error { return nil }

// Rewrite writes the file back in its current, canonical form without
// changing its contents — `dlctl fmt --type=state`'s way of reformatting reports,
// mirroring bans/yaml's Rewrite for the same reason: Append is the only
// other way to change what is stored, and it is the wrong shape for "no
// changes, just rewrite."
func (s *Store) Rewrite() error { return s.save() }

func (s *Store) load() error {
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

	s.reports = make([]reports.Report, 0, len(d.Reports))
	for _, rd := range d.Reports {
		r := reports.Report{Kind: reports.Kind(rd.Kind), Reporter: rd.Reporter, Room: rd.Room, Body: rd.Body}
		if rd.When != "" {
			when, err := time.Parse(time.RFC3339, rd.When)
			if err != nil {
				return fmt.Errorf("%s: %q: %w", s.path, rd.Reporter, err)
			}
			r.When = when.UTC()
		}
		s.reports = append(s.reports, r)
	}
	return nil
}

// Append implements reports.Store. yaml has no fixed-size file to fill,
// so it never refuses — max_filesize is classic's own limit
// (act.other.c:908-911, config.c:233), not a property of "a report", and
// a format with no struct-dump-shaped file behind it has nothing forcing
// one on it either.
//
// r.When is stored exactly as given, including zero. Filling in time.Now()
// here would be right for a freshly filed report and wrong for one being
// imported from classic — this function cannot tell those apart, so that
// choice belongs to the caller that can (the live `bug`/`idea`/`typo`
// command sets When itself; `dlctl import --type=state` deliberately does not,
// since classic's own report genuinely has no recoverable timestamp).
func (s *Store) Append(r reports.Report) (bool, error) {
	if s.readOnly {
		return false, fmt.Errorf("reports: the data directory is open read-only")
	}

	s.mu.Lock()
	s.reports = append(s.reports, r)
	s.mu.Unlock()

	return true, s.save()
}

// All implements reports.Store.
func (s *Store) All() ([]reports.Report, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]reports.Report(nil), s.reports...), nil
}

func (s *Store) save() error {
	if s.readOnly {
		return fmt.Errorf("reports: the data directory is open read-only")
	}

	s.mu.RLock()
	d := doc{Schema: reportSchema, Reports: make([]reportDoc, 0, len(s.reports))}
	for _, r := range s.reports {
		rd := reportDoc{Kind: string(r.Kind), Reporter: r.Reporter, Room: r.Room, Body: r.Body}
		if !r.When.IsZero() {
			rd.When = r.When.UTC().Format(time.RFC3339)
		}
		d.Reports = append(d.Reports, rd)
	}
	s.mu.RUnlock()

	out, err := yaml.MarshalWithOptions(d, yamlenc.Options()...)
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
