// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

// Package yaml implements docs/design/data-format.md §9's
// state/bans.yaml: the site ban list as a flat YAML list rather than
// classic's four-whitespace-fields-per-line text file. No struct dump was
// ever involved (see internal/persist/bans/classic's own package comment),
// so unlike world/player this is a change of syntax only — every field
// classic holds, yaml holds too, and neither drops or truncates anything.
package yaml

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/goccy/go-yaml"

	"github.com/gerrowadat/disgracelands/internal/persist/bans"
)

// FormatName is the name this format registers under.
const FormatName = "yaml"

// banSchema is the document's schema tag, docs/design/data-format.md
// §10.1.
const banSchema = "dl/bans@1"

// fileName is state/bans.yaml under whatever directory Config.Path names.
const fileName = "bans.yaml"

func init() {
	bans.Register(FormatName, func(cfg bans.Config) (bans.Store, error) {
		return New(cfg)
	})
}

type doc struct {
	Schema string   `yaml:"schema"`
	Bans   []banDoc `yaml:"bans,omitempty"`
}

type banDoc struct {
	Site string `yaml:"site"`
	Type string `yaml:"type"`
	// When is RFC 3339, empty for a ban with no recorded time — classic's
	// own "seconds == 0 means no timestamp" case (classic/classic.go's
	// load), which does happen: a ban set by hand-editing the file, or one
	// this package itself never fails to record but classic's archive
	// might have.
	When string `yaml:"when,omitempty"`
	By   string `yaml:"by,omitempty"`
}

// Store keeps the ban list as one YAML file, <dir>/bans.yaml. Held in
// memory and rewritten whole, same posture as classic's Store and for the
// same reason: consulted on every connection, a few dozen lines at most.
type Store struct {
	path     string
	readOnly bool

	mu   sync.RWMutex
	bans []bans.Ban
}

// New opens the ban file. A missing file is not an error, matching
// classic: a server nobody has been thrown off yet.
func New(cfg bans.Config) (*Store, error) {
	if cfg.Path == "" {
		return nil, fmt.Errorf("bans: no directory configured")
	}
	s := &Store{path: filepath.Join(cfg.Path, fileName), readOnly: cfg.ReadOnly}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

// Name implements bans.Store.
func (s *Store) Name() string { return FormatName }

// Close implements bans.Store.
func (s *Store) Close() error { return nil }

// Rewrite writes the file back in its current, canonical form without
// changing its contents — `dlctl state fmt`'s way of reformatting bans,
// since Add/Remove are the only ways to change what is stored and neither
// is the right shape for "no changes, just rewrite."
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

	s.bans = make([]bans.Ban, 0, len(d.Bans))
	for _, bd := range d.Bans {
		kind, ok := bans.ParseType(bd.Type)
		if !ok {
			return fmt.Errorf("%s: %q: unknown ban type %q", s.path, bd.Site, bd.Type)
		}
		ban := bans.Ban{Site: bd.Site, Type: kind, By: bd.By}
		if bd.When != "" {
			when, err := time.Parse(time.RFC3339, bd.When)
			if err != nil {
				return fmt.Errorf("%s: %q: %w", s.path, bd.Site, err)
			}
			ban.When = when.UTC()
		}
		s.bans = append(s.bans, ban)
	}
	return nil
}

// List implements bans.Store, newest first — the order Add/save keep it in,
// matching classic's own list order so the two formats agree about what
// "the list" means.
func (s *Store) List() []bans.Ban {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]bans.Ban(nil), s.bans...)
}

// Check implements bans.Store. Same substring match as classic — this is
// the model's own rule (bans.Ban's doc comment), not something either
// format decides for itself.
func (s *Store) Check(host string) bans.Type {
	if host == "" {
		return bans.TypeNone
	}
	host = strings.ToLower(host)

	s.mu.RLock()
	defer s.mu.RUnlock()

	worst := bans.TypeNone
	for _, ban := range s.bans {
		if strings.Contains(host, ban.Site) && ban.Type > worst {
			worst = ban.Type
		}
	}
	return worst
}

// Add implements bans.Store, reporting false when the site is already
// listed.
func (s *Store) Add(ban bans.Ban) (bool, error) {
	ban.Site = strings.ToLower(ban.Site)
	if len(ban.Site) > bans.MaxSiteLength {
		// yaml has no fixed-width field to enforce this, but truncating
		// silently would make the same site collide under classic and not
		// under yaml — kept for behavioural parity, not a format limit.
		ban.Site = ban.Site[:bans.MaxSiteLength]
	}

	s.mu.Lock()
	for _, existing := range s.bans {
		if strings.EqualFold(existing.Site, ban.Site) {
			s.mu.Unlock()
			return false, nil
		}
	}
	s.bans = append([]bans.Ban{ban}, s.bans...)
	s.mu.Unlock()

	return true, s.save()
}

// Remove implements bans.Store, reporting the ban that went.
func (s *Store) Remove(site string) (bans.Ban, bool, error) {
	s.mu.Lock()
	for i, ban := range s.bans {
		if strings.EqualFold(ban.Site, site) {
			s.bans = append(s.bans[:i], s.bans[i+1:]...)
			s.mu.Unlock()
			return ban, true, s.save()
		}
	}
	s.mu.Unlock()
	return bans.Ban{}, false, nil
}

func (s *Store) save() error {
	if s.readOnly {
		return fmt.Errorf("bans: the data directory is open read-only")
	}

	s.mu.RLock()
	d := doc{Schema: banSchema, Bans: make([]banDoc, 0, len(s.bans))}
	for _, ban := range s.bans {
		bd := banDoc{Site: ban.Site, Type: ban.Type.String(), By: ban.By}
		if !ban.When.IsZero() {
			bd.When = ban.When.UTC().Format(time.RFC3339)
		}
		d.Bans = append(d.Bans, bd)
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
