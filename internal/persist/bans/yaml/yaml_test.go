// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package yaml

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gerrowadat/disgracelands/internal/persist/bans"
)

func TestBansRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s, err := New(bans.Config{Path: dir})
	if err != nil {
		t.Fatalf("opening: %v", err)
	}

	when := time.Unix(1_000_000_000, 0).UTC()
	for _, ban := range []bans.Ban{
		{Site: "example.com", Type: bans.TypeAll, When: when, By: "Zod"},
		{Site: "spam.net", Type: bans.TypeNew, When: when, By: "Welmar"},
		// A ban with no recorded time, matching classic's own "seconds == 0"
		// case.
		{Site: "notime.example", Type: bans.TypeSelect, By: "Zod"},
	} {
		added, err := s.Add(ban)
		if err != nil || !added {
			t.Fatalf("adding %s: added=%v err=%v", ban.Site, added, err)
		}
	}

	again, err := New(bans.Config{Path: dir})
	if err != nil {
		t.Fatalf("reopening: %v", err)
	}
	got := again.List()
	if len(got) != 3 {
		t.Fatalf("read %d bans, want 3", len(got))
	}
	// Newest first.
	if got[0].Site != "notime.example" {
		t.Errorf("the list starts with %q, want the newest ban first", got[0].Site)
	}
	if !got[0].When.IsZero() {
		t.Errorf("a ban with no recorded time came back as %v", got[0].When)
	}
	if got[2].Type != bans.TypeAll || got[2].By != "Zod" || !got[2].When.Equal(when) {
		t.Errorf("the oldest ban round-tripped as %+v", got[2])
	}
}

func TestAMissingFileIsNoBans(t *testing.T) {
	s, err := New(bans.Config{Path: t.TempDir()})
	if err != nil {
		t.Fatalf("opening: %v", err)
	}
	if got := len(s.List()); got != 0 {
		t.Errorf("a missing ban file produced %d bans", got)
	}
}

func TestAReadOnlyStoreRefusesToWrite(t *testing.T) {
	s, err := New(bans.Config{Path: t.TempDir(), ReadOnly: true})
	if err != nil {
		t.Fatalf("opening: %v", err)
	}
	if _, err := s.Add(bans.Ban{Site: "example.com", Type: bans.TypeAll}); err == nil {
		t.Error("a read-only store wrote a ban")
	}
}

func TestCheckAndRemove(t *testing.T) {
	s, err := New(bans.Config{Path: t.TempDir()})
	if err != nil {
		t.Fatalf("opening: %v", err)
	}
	if _, err := s.Add(bans.Ban{Site: "example.com", Type: bans.TypeAll}); err != nil {
		t.Fatalf("adding: %v", err)
	}
	if got := s.Check("mail.example.com"); got != bans.TypeAll {
		t.Errorf("Check gave %v, want TypeAll (substring match)", got)
	}
	ban, found, err := s.Remove("EXAMPLE.COM")
	if err != nil || !found || ban.Type != bans.TypeAll {
		t.Fatalf("Remove: ban=%+v found=%v err=%v", ban, found, err)
	}
	if got := s.Check("example.com"); got != bans.TypeNone {
		t.Errorf("the site is still banned: %v", got)
	}
}

func TestUnknownTypeIsRejected(t *testing.T) {
	dir := t.TempDir()
	body := "schema: dl/bans@1\nbans:\n  - site: example.com\n    type: nonsense\n"
	if err := os.WriteFile(filepath.Join(dir, fileName), []byte(body), 0o600); err != nil {
		t.Fatalf("writing: %v", err)
	}
	if _, err := New(bans.Config{Path: dir}); err == nil {
		t.Fatal("expected an error loading a ban with an unknown type")
	}
}

// TestWrittenFileIsReadableStrictYAML pins the on-disk shape down directly,
// the same way classic's own TestTheFileFormatIsWhatTheCWrites does.
func TestWrittenFileIsReadableStrictYAML(t *testing.T) {
	dir := t.TempDir()
	s, err := New(bans.Config{Path: dir})
	if err != nil {
		t.Fatalf("opening: %v", err)
	}
	when := time.Date(2008, 6, 19, 2, 31, 55, 0, time.UTC)
	if _, err := s.Add(bans.Ban{Site: "example.com", Type: bans.TypeSelect, When: when, By: "Zod"}); err != nil {
		t.Fatalf("adding: %v", err)
	}

	b, err := os.ReadFile(filepath.Join(dir, fileName))
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	// goccy/go-yaml's own choices, verified rather than assumed: no extra
	// indent before a list dash relative to its parent key, and an RFC 3339
	// timestamp quoted because its colons would otherwise need one anyway.
	want := "schema: dl/bans@1\nbans:\n- site: example.com\n  type: select\n  when: \"2008-06-19T02:31:55Z\"\n  by: Zod\n"
	if string(b) != want {
		t.Errorf("the file is:\n%s\nwant:\n%s", b, want)
	}
}
