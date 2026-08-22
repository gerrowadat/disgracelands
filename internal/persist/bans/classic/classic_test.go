// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package classic

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gerrowadat/disgracelands/internal/persist/bans"
)

func newStore(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "badsites")
	s, err := New(bans.Config{Path: path})
	if err != nil {
		t.Fatalf("opening: %v", err)
	}
	return s, path
}

func TestBansRoundTrip(t *testing.T) {
	s, path := newStore(t)

	when := time.Unix(1_000_000_000, 0).UTC()
	for _, ban := range []bans.Ban{
		{Site: "example.com", Type: bans.TypeAll, When: when, By: "Zod"},
		{Site: "spam.net", Type: bans.TypeNew, When: when, By: "Welmar"},
	} {
		added, err := s.Add(ban)
		if err != nil || !added {
			t.Fatalf("adding %s: added=%v err=%v", ban.Site, added, err)
		}
	}

	again, err := New(bans.Config{Path: path})
	if err != nil {
		t.Fatalf("reopening: %v", err)
	}
	got := again.List()
	if len(got) != 2 {
		t.Fatalf("read %d bans, want 2", len(got))
	}
	// Newest first, because the C pushes onto the front of the list and
	// writes the file backwards.
	if got[0].Site != "spam.net" {
		t.Errorf("the list starts with %q, want the newest ban first", got[0].Site)
	}
	if got[0].Type != bans.TypeNew || got[0].By != "Welmar" || !got[0].When.Equal(when) {
		t.Errorf("the ban round-tripped as %+v", got[0])
	}
}

// The file is four whitespace-separated fields per line, and the C's fscanf
// loop reads it back.
func TestTheFileFormatIsWhatTheCWrites(t *testing.T) {
	s, path := newStore(t)
	if _, err := s.Add(bans.Ban{
		Site: "example.com", Type: bans.TypeSelect,
		When: time.Unix(1_700_000_000, 0), By: "Zod",
	}); err != nil {
		t.Fatalf("adding: %v", err)
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	want := "select example.com 1700000000 Zod\n"
	if string(b) != want {
		t.Errorf("the file is %q, want %q", b, want)
	}
}

// isbanned is a *substring* match, which is much broader than it looks.
func TestCheckIsASubstringMatch(t *testing.T) {
	s, _ := newStore(t)
	if _, err := s.Add(bans.Ban{Site: "example.com", Type: bans.TypeAll}); err != nil {
		t.Fatalf("adding: %v", err)
	}

	for _, tc := range []struct {
		host string
		want bans.Type
	}{
		{"example.com", bans.TypeAll},
		{"EXAMPLE.COM", bans.TypeAll},
		{"mail.example.com", bans.TypeAll},
		// The surprising one: a substring match catches this too.
		{"notexample.computer", bans.TypeAll},
		{"example.org", bans.TypeNone},
		{"", bans.TypeNone},
	} {
		if got := s.Check(tc.host); got != tc.want {
			t.Errorf("Check(%q) = %v, want %v", tc.host, got, tc.want)
		}
	}
}

// The worst matching ban wins.
func TestCheckTakesTheWorstMatch(t *testing.T) {
	s, _ := newStore(t)
	for _, ban := range []bans.Ban{
		{Site: "example.com", Type: bans.TypeNew},
		{Site: "mail.example.com", Type: bans.TypeAll},
	} {
		if _, err := s.Add(ban); err != nil {
			t.Fatalf("adding: %v", err)
		}
	}
	if got := s.Check("mail.example.com"); got != bans.TypeAll {
		t.Errorf("Check gave %v, want the worst of the two", got)
	}
	if got := s.Check("www.example.com"); got != bans.TypeNew {
		t.Errorf("Check gave %v, want the only one that matches", got)
	}
}

// A site already banned is refused rather than changed, which is what the
// C's message says to do about it.
func TestAddingTwiceIsRefused(t *testing.T) {
	s, _ := newStore(t)
	if added, _ := s.Add(bans.Ban{Site: "example.com", Type: bans.TypeNew}); !added {
		t.Fatal("the first add was refused")
	}
	if added, _ := s.Add(bans.Ban{Site: "EXAMPLE.COM", Type: bans.TypeAll}); added {
		t.Error("adding an already-banned site succeeded")
	}
	if got := s.Check("example.com"); got != bans.TypeNew {
		t.Errorf("the ban type changed to %v", got)
	}
}

func TestRemove(t *testing.T) {
	s, _ := newStore(t)
	if _, err := s.Add(bans.Ban{Site: "example.com", Type: bans.TypeAll}); err != nil {
		t.Fatalf("adding: %v", err)
	}

	if _, found, _ := s.Remove("nowhere.invalid"); found {
		t.Error("removing a site that was not banned succeeded")
	}
	ban, found, err := s.Remove("EXAMPLE.COM")
	if err != nil || !found {
		t.Fatalf("removing: found=%v err=%v", found, err)
	}
	if ban.Type != bans.TypeAll {
		t.Errorf("the removed ban was %v", ban.Type)
	}
	if got := s.Check("example.com"); got != bans.TypeNone {
		t.Errorf("the site is still banned: %v", got)
	}
}

// A line with the wrong number of fields stops the read, as fscanf's `== 4`
// loop does — the tail is lost rather than the boot refused.
func TestAShortLineStopsTheRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "badsites")
	body := strings.Join([]string{
		"all example.com 1000000000 Zod",
		"rubbish",
		"new spam.net 1000000000 Welmar",
	}, "\n") + "\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("writing: %v", err)
	}

	s, err := New(bans.Config{Path: path})
	if err != nil {
		t.Fatalf("opening: %v", err)
	}
	if got := len(s.List()); got != 1 {
		t.Errorf("read %d bans past a malformed line, want 1", got)
	}
}

func TestAMissingFileIsNoBans(t *testing.T) {
	s, err := New(bans.Config{Path: filepath.Join(t.TempDir(), "nothing")})
	if err != nil {
		t.Fatalf("opening: %v", err)
	}
	if got := len(s.List()); got != 0 {
		t.Errorf("a missing ban file produced %d bans", got)
	}
}

func TestAReadOnlyStoreRefusesToWrite(t *testing.T) {
	s, err := New(bans.Config{Path: filepath.Join(t.TempDir(), "badsites"), ReadOnly: true})
	if err != nil {
		t.Fatalf("opening: %v", err)
	}
	if _, err := s.Add(bans.Ban{Site: "example.com", Type: bans.TypeAll}); err == nil {
		t.Error("a read-only store wrote a ban")
	}
}
