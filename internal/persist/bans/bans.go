// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

// Package bans reads and writes the site ban list, porting ban.c.
//
// The one archive file in this whole port that is *text*: four
// whitespace-separated fields per line, read with a bare `fscanf(fl, " %s %s
// %d %s ", ...)`. No struct dump, no layout, no oracle — which after the
// player database, the rent files, the boards, the mail and the house control
// records is worth remarking on.
package bans

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
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
// out-of-range answer and is never written to the file.
var typeNames = []string{"no", "new", "select", "all"}

// String names the type as the file spells it.
func (t Type) String() string {
	if t < 0 || int(t) >= len(typeNames) {
		return "ERROR"
	}
	return typeNames[t]
}

// ParseType reads a type name, as `ban` does from what a god typed.
func ParseType(s string) (Type, bool) {
	for i, name := range typeNames {
		if strings.EqualFold(s, name) {
			return Type(i), true //nolint:gosec // an index into a fixed table
		}
	}
	return TypeNone, false
}

// MaxSiteLength is BANNED_SITE_LENGTH (ban.h), and the C truncates to it.
const MaxSiteLength = 50

// Ban is one line of the file.
type Ban struct {
	// Site is the substring matched against a connecting host, lower-cased.
	Site string
	Type Type
	// When the ban was made, and who by.
	When time.Time
	By   string
}

// Store is the ban file.
//
// Held in memory and rewritten whole, because it is consulted on every single
// connection and is a few dozen lines at most.
type Store struct {
	path     string
	readOnly bool

	mu   sync.RWMutex
	bans []Ban
}

// New opens the ban file. A missing file is not an error: it is a server
// nobody has been thrown off yet, which is what the archive has.
func New(path string, readOnly bool) (*Store, error) {
	if path == "" {
		return nil, fmt.Errorf("bans: no file configured")
	}
	s := &Store{path: path, readOnly: readOnly}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

// load is load_banned (ban.c:46).
//
// The C's `fscanf(... ) == 4` loop stops at the first line that does not
// have four fields and silently drops the rest of the file. Reproduced: a
// half-written ban file should lose the tail rather than refuse to boot.
func (s *Store) load() error {
	f, err := os.Open(s.path) //nolint:gosec // a configured path
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading the ban file: %w", err)
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 4 {
			break
		}
		kind, ok := ParseType(fields[0])
		if !ok {
			// The C's loop leaves `type` at whatever CREATE zeroed it to,
			// which is BAN_NOT — a line naming a type it does not recognise
			// bans nothing at all.
			kind = TypeNone
		}
		seconds, err := strconv.ParseInt(fields[2], 10, 64)
		if err != nil {
			break
		}
		ban := Ban{Site: fields[1], Type: kind, By: fields[3]}
		if seconds != 0 {
			ban.When = time.Unix(seconds, 0).UTC()
		}
		// Onto the *front*, as `next_node->next = ban_list` does. Together
		// with save() writing backwards this is what makes a round trip
		// stable: the file is oldest-first, the list is newest-first, and
		// each reverses the other.
		s.bans = append([]Ban{ban}, s.bans...)
	}
	return scanner.Err()
}

// List returns the bans, newest first — the order the C's list is in, because
// it pushes each new one onto the front.
func (s *Store) List() []Ban {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]Ban(nil), s.bans...)
}

// Check is isbanned (ban.c:82): the worst ban matching this host.
//
// The match is a *substring* test, not a suffix or a glob — so banning
// "example.com" also bans "notexample.computer", and banning "1" bans a
// remarkable number of addresses. That is the C's, and it is the reason the
// ban list was always short and carefully written.
func (s *Store) Check(host string) Type {
	if host == "" {
		return TypeNone
	}
	host = strings.ToLower(host)

	s.mu.RLock()
	defer s.mu.RUnlock()

	worst := TypeNone
	for _, ban := range s.bans {
		if strings.Contains(host, ban.Site) && ban.Type > worst {
			worst = ban.Type
		}
	}
	return worst
}

// Add records a ban, reporting false when the site is already on the list —
// the C refuses rather than changing the type, and says so.
func (s *Store) Add(ban Ban) (bool, error) {
	ban.Site = strings.ToLower(ban.Site)
	if len(ban.Site) > MaxSiteLength {
		ban.Site = ban.Site[:MaxSiteLength]
	}

	s.mu.Lock()
	for _, existing := range s.bans {
		if strings.EqualFold(existing.Site, ban.Site) {
			s.mu.Unlock()
			return false, nil
		}
	}
	// Onto the front, as the C's linked list does.
	s.bans = append([]Ban{ban}, s.bans...)
	s.mu.Unlock()

	return true, s.save()
}

// Remove is do_unban's half, reporting the ban that went.
func (s *Store) Remove(site string) (Ban, bool, error) {
	s.mu.Lock()
	for i, ban := range s.bans {
		if strings.EqualFold(ban.Site, site) {
			s.bans = append(s.bans[:i], s.bans[i+1:]...)
			s.mu.Unlock()
			return ban, true, s.save()
		}
	}
	s.mu.Unlock()
	return Ban{}, false, nil
}

// save is write_ban_list (ban.c:114).
//
// The C writes the list *backwards* — `_write_one_node` recurses to the tail
// before printing — so the file comes out oldest-first and reloading it
// reverses the list back into newest-first. Reproduced, so the file this port
// writes reads the same way to the C.
func (s *Store) save() error {
	if s.readOnly {
		return fmt.Errorf("bans: the data directory is open read-only")
	}

	s.mu.RLock()
	var b strings.Builder
	for i := len(s.bans) - 1; i >= 0; i-- {
		ban := s.bans[i]
		seconds := int64(0)
		if !ban.When.IsZero() {
			seconds = ban.When.Unix()
		}
		fmt.Fprintf(&b, "%s %s %d %s\n", ban.Type, ban.Site, seconds, ban.By)
	}
	s.mu.RUnlock()

	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, []byte(b.String()), 0o600); err != nil {
		return fmt.Errorf("writing the ban file: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("writing the ban file: %w", err)
	}
	return nil
}
