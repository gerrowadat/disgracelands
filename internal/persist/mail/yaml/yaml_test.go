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

	"github.com/gerrowadat/disgracelands/internal/persist/mail"
)

func send(t *testing.T, s *Store, to, from int64, text string) {
	t.Helper()
	if err := s.Send(mail.Message{To: to, From: from, Sent: time.Unix(1_700_000_000, 0), Text: text}); err != nil {
		t.Fatalf("sending: %v", err)
	}
}

func TestMailRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s, err := New(mail.Config{Path: dir})
	if err != nil {
		t.Fatalf("opening: %v", err)
	}
	send(t, s, 7, 3, "Hello there.\r\nSecond line.\r\n")

	if !s.HasMail(7) {
		t.Error("the recipient has no mail")
	}
	if s.HasMail(8) {
		t.Error("somebody else has mail")
	}

	again, err := New(mail.Config{Path: dir})
	if err != nil {
		t.Fatalf("reopening: %v", err)
	}
	got, ok, err := again.Receive(7)
	if err != nil || !ok {
		t.Fatalf("receiving: ok=%v err=%v", ok, err)
	}
	if got.Text != "Hello there.\r\nSecond line.\r\n" || got.From != 3 || got.To != 7 {
		t.Errorf("received %+v", got)
	}
	if !got.Sent.Equal(time.Unix(1_700_000_000, 0).UTC()) {
		t.Errorf("received with timestamp %s", got.Sent)
	}
}

// Mail is delivered oldest first.
func TestMailArrivesOldestFirst(t *testing.T) {
	s, err := New(mail.Config{Path: t.TempDir()})
	if err != nil {
		t.Fatalf("opening: %v", err)
	}
	send(t, s, 1, 2, "first")
	send(t, s, 1, 2, "second")
	send(t, s, 1, 2, "third")

	for _, want := range []string{"first", "second", "third"} {
		got, ok, err := s.Receive(1)
		if err != nil || !ok {
			t.Fatalf("receiving %q: ok=%v err=%v", want, ok, err)
		}
		if got.Text != want {
			t.Errorf("received %q, want %q", got.Text, want)
		}
	}
	if s.HasMail(1) {
		t.Error("there is still mail after three of three were read")
	}
}

// An emptied mail file leaves no trace, matching classic's own rule.
func TestEmptyingTheMailboxRemovesTheFile(t *testing.T) {
	dir := t.TempDir()
	s, err := New(mail.Config{Path: dir})
	if err != nil {
		t.Fatalf("opening: %v", err)
	}
	send(t, s, 1, 2, "x")
	if _, _, err := s.Receive(1); err != nil {
		t.Fatalf("receiving: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, fileName)); !os.IsNotExist(err) {
		t.Error("an emptied mail file was left behind")
	}
}

func TestAllDoesNotDrainTheMailbox(t *testing.T) {
	s, err := New(mail.Config{Path: t.TempDir()})
	if err != nil {
		t.Fatalf("opening: %v", err)
	}
	send(t, s, 1, 2, "first")
	send(t, s, 3, 2, "second")

	if got := len(s.All()); got != 2 {
		t.Fatalf("All returned %d messages, want 2", got)
	}
	if !s.HasMail(1) || !s.HasMail(3) {
		t.Error("All() removed mail it only should have read")
	}
}

func TestNonsenseIsRefused(t *testing.T) {
	s, err := New(mail.Config{Path: t.TempDir()})
	if err != nil {
		t.Fatalf("opening: %v", err)
	}
	for _, m := range []mail.Message{
		{To: -1, From: 1, Text: "x"},
		{To: 1, From: -1, Text: "x"},
		{To: 1, From: 2, Text: ""},
	} {
		if err := s.Send(m); err == nil {
			t.Errorf("storing %+v succeeded, want a refusal", m)
		}
	}
}

func TestReceivingNothing(t *testing.T) {
	s, err := New(mail.Config{Path: t.TempDir()})
	if err != nil {
		t.Fatalf("opening: %v", err)
	}
	if _, ok, err := s.Receive(99); ok || err != nil {
		t.Errorf("receiving for a player with no mail gave ok=%v err=%v", ok, err)
	}
}

func TestAReadOnlyStoreRefusesToWrite(t *testing.T) {
	s, err := New(mail.Config{Path: t.TempDir(), ReadOnly: true})
	if err != nil {
		t.Fatalf("opening: %v", err)
	}
	if err := s.Send(mail.Message{To: 1, From: 2, Text: "x"}); err == nil {
		t.Error("a read-only store wrote mail")
	}
}
