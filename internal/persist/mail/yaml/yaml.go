// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

// Package yaml implements docs/design/data-format.md §9's
// state/mail.yaml: a flat list of messages, oldest first. classic's block
// allocator ("This works much like DOS' FAT.", classic/classic.go's own
// package comment) has no counterpart here — there is no free list to
// manage and no block chain to walk, because nothing here is limited to
// 100-byte records. Delivery order falls out for free, too: Send appends,
// Receive takes the first match, so the list is oldest-first by
// construction rather than by the reversal classic's own findHeader needs
// (see its doc comment for why that one is not as simple as it looks).
package yaml

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/goccy/go-yaml"

	"github.com/gerrowadat/disgracelands/internal/persist/mail"
	worldtext "github.com/gerrowadat/disgracelands/internal/persist/world/yaml"
)

// FormatName is the name this format registers under.
const FormatName = "yaml"

// mailSchema is the document's schema tag, docs/design/data-format.md
// §10.1.
const mailSchema = "dl/mail@1"

// fileName is state/mail.yaml under whatever directory Config.Path names.
const fileName = "mail.yaml"

func init() {
	mail.Register(FormatName, func(cfg mail.Config) (mail.Store, error) {
		return New(cfg)
	})
}

// NestedText is the world format's own block-scalar/quoting logic, reused
// rather than duplicated — the same choice internal/persist/boards/yaml
// made for a message body, and for the same reason: mail text is
// CRLF-joined in memory (session.handleEditing's strings.Join(lines,
// "\r\n"), the same line editor boards' `write` uses) and nested two
// levels deep (mail -> list -> text), so a bare Go string field would hit
// the same goccy/go-yaml default-block-scalar mishandling of an embedded
// \r that boards/yaml's own doc comment traces in detail.
type NestedText = worldtext.NestedText

type doc struct {
	Schema string    `yaml:"schema"`
	Mail   []mailDoc `yaml:"mail,omitempty"`
}

type mailDoc struct {
	To   int64      `yaml:"to"`
	From int64      `yaml:"from"`
	Sent string     `yaml:"sent,omitempty"`
	Text NestedText `yaml:"text"`
}

// Store keeps every message in one YAML file, <dir>/mail.yaml. Held in
// memory and rewritten whole on any Send/Receive — the same posture
// bans/yaml and boards/yaml have, for the same reason: a mud mail
// system's whole contents are a handful of kilobytes.
type Store struct {
	path     string
	readOnly bool

	mu   sync.Mutex
	mail []mail.Message
}

// New opens the mail file.
func New(cfg mail.Config) (*Store, error) {
	if cfg.Path == "" {
		return nil, fmt.Errorf("mail: no directory configured")
	}
	s := &Store{path: filepath.Join(cfg.Path, fileName), readOnly: cfg.ReadOnly}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

// Name implements mail.Store.
func (s *Store) Name() string { return FormatName }

// Close implements mail.Store.
func (s *Store) Close() error { return nil }

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

	s.mail = make([]mail.Message, 0, len(d.Mail))
	for _, md := range d.Mail {
		m := mail.Message{To: md.To, From: md.From, Text: worldtext.FromStored(string(md.Text))}
		if md.Sent != "" {
			sent, err := time.Parse(time.RFC3339, md.Sent)
			if err != nil {
				return fmt.Errorf("%s: %w", s.path, err)
			}
			m.Sent = sent.UTC()
		}
		s.mail = append(s.mail, m)
	}
	return nil
}

// HasMail implements mail.Store.
func (s *Store) HasMail(recipient int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, m := range s.mail {
		if m.To == recipient {
			return true
		}
	}
	return false
}

// All implements mail.Store.
func (s *Store) All() []mail.Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]mail.Message(nil), s.mail...)
}

// Rewrite writes the file back in its current, canonical form without
// changing its contents — `dlctl state fmt`'s way of reformatting mail,
// since Send/Receive are the only ways to change what is stored and
// neither is the right shape for "no changes, just rewrite."
func (s *Store) Rewrite() error { return s.save() }

// Send implements mail.Store.
func (s *Store) Send(m mail.Message) error {
	if m.From < 0 || m.To < 0 || m.Text == "" {
		return fmt.Errorf("mail: refusing to store a message from %d to %d", m.From, m.To)
	}

	s.mu.Lock()
	s.mail = append(s.mail, m)
	s.mu.Unlock()

	return s.save()
}

// Receive implements mail.Store: the first (oldest) message addressed to
// this recipient.
func (s *Store) Receive(recipient int64) (mail.Message, bool, error) {
	s.mu.Lock()
	i := -1
	for j, m := range s.mail {
		if m.To == recipient {
			i = j
			break
		}
	}
	if i < 0 {
		s.mu.Unlock()
		return mail.Message{}, false, nil
	}
	m := s.mail[i]
	s.mail = append(s.mail[:i], s.mail[i+1:]...)
	s.mu.Unlock()

	return m, true, s.save()
}

func (s *Store) save() error {
	if s.readOnly {
		return fmt.Errorf("mail: the data directory is open read-only")
	}

	s.mu.Lock()
	d := doc{Schema: mailSchema}
	if len(s.mail) > 0 {
		d.Mail = make([]mailDoc, 0, len(s.mail))
		for _, m := range s.mail {
			md := mailDoc{To: m.To, From: m.From, Text: NestedText(worldtext.ToStored(m.Text))}
			if !m.Sent.IsZero() {
				md.Sent = m.Sent.UTC().Format(time.RFC3339)
			}
			d.Mail = append(d.Mail, md)
		}
	}
	s.mu.Unlock()

	out, err := yaml.MarshalWithOptions(d, yaml.Indent(2))
	if err != nil {
		return fmt.Errorf("writing %s: %w", s.path, err)
	}

	// A file of nothing but delivered mail is removed, matching classic's
	// own "an emptied mail file leaves no trace" rule (flushLocked).
	if len(d.Mail) == 0 {
		if err := os.Remove(s.path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("removing the empty mail file: %w", err)
		}
		return nil
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
