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

	"github.com/gerrowadat/disgracelands/internal/persist/mail"
)

func newStore(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "plrmail")
	s, err := New(mail.Config{Path: path})
	if err != nil {
		t.Fatalf("opening: %v", err)
	}
	return s, path
}

func send(t *testing.T, s *Store, to, from int64, text string) {
	t.Helper()
	if err := s.Send(mail.Message{To: to, From: from, Sent: time.Unix(1_700_000_000, 0), Text: text}); err != nil {
		t.Fatalf("sending: %v", err)
	}
}

func TestAShortMessageFitsInOneBlock(t *testing.T) {
	s, path := newStore(t)
	send(t, s, 7, 3, "hello")

	if !s.HasMail(7) {
		t.Error("the recipient has no mail")
	}
	if s.HasMail(8) {
		t.Error("somebody else has mail")
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Size() != BlockSize {
		t.Errorf("a five-character message took %d bytes, want one %d-byte block", info.Size(), BlockSize)
	}

	got, ok, err := s.Receive(7)
	if err != nil || !ok {
		t.Fatalf("receiving: ok=%v err=%v", ok, err)
	}
	if got.Text != "hello" || got.From != 3 || got.To != 7 {
		t.Errorf("received %+v, want hello from 3 to 7", got)
	}
	if !got.Sent.Equal(time.Unix(1_700_000_000, 0).UTC()) {
		t.Errorf("received with timestamp %s", got.Sent)
	}
}

// A message longer than a header block spills into data blocks, and the split
// is at exactly HEADER_BLOCK_DATASIZE then DATA_BLOCK_DATASIZE.
func TestALongMessageSpansBlocks(t *testing.T) {
	s, path := newStore(t)

	// Three blocks' worth: the header's 79 plus a bit over one data block's
	// 95. Distinct characters so a shuffled middle would be obvious.
	text := strings.Repeat("a", HeaderTextSize) +
		strings.Repeat("b", DataTextSize) +
		strings.Repeat("c", 20)
	send(t, s, 1, 2, text)

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if want := int64(3 * BlockSize); info.Size() != want {
		t.Errorf("the file is %d bytes, want %d — three blocks", info.Size(), want)
	}

	got, ok, err := s.Receive(1)
	if err != nil || !ok {
		t.Fatalf("receiving: ok=%v err=%v", ok, err)
	}
	if got.Text != text {
		t.Errorf("the message came back %d characters, want %d", len(got.Text), len(text))
	}
}

// Mail is delivered oldest first: the C pushes new messages onto the front of
// a per-player list and reads from the back.
func TestMailArrivesOldestFirst(t *testing.T) {
	s, _ := newStore(t)
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

// Freed blocks are reused before the file grows, which is the whole point of
// the free list.
func TestFreedBlocksAreReused(t *testing.T) {
	s, path := newStore(t)

	send(t, s, 1, 2, strings.Repeat("x", 200)) // three blocks
	if _, _, err := s.Receive(1); err != nil {
		t.Fatalf("receiving: %v", err)
	}
	// Everything is free now, so the file is gone.
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("an emptied mail file was left behind")
	}

	// Fill it again and check it does not grow past what it had.
	send(t, s, 1, 2, "short")
	send(t, s, 3, 2, "also short")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if want := int64(2 * BlockSize); info.Size() != want {
		t.Errorf("the file is %d bytes, want %d", info.Size(), want)
	}
}

func TestMailSurvivesReopening(t *testing.T) {
	s, path := newStore(t)
	send(t, s, 42, 1, "still here?")

	again, err := New(mail.Config{Path: path})
	if err != nil {
		t.Fatalf("reopening: %v", err)
	}
	got, ok, err := again.Receive(42)
	if err != nil || !ok {
		t.Fatalf("receiving after a reopen: ok=%v err=%v", ok, err)
	}
	if got.Text != "still here?" {
		t.Errorf("received %q after a reopen", got.Text)
	}
}

// All is non-destructive: everything is still there afterwards, and in the
// same oldest-first order Receive would deliver it in.
func TestAllDoesNotDrainTheMailbox(t *testing.T) {
	s, _ := newStore(t)
	send(t, s, 1, 2, "first")
	send(t, s, 3, 2, "second")
	send(t, s, 1, 2, "third")

	all := s.All()
	if len(all) != 3 {
		t.Fatalf("All returned %d messages, want 3", len(all))
	}

	// Nothing was freed: HasMail and Receive still see everything.
	if !s.HasMail(1) || !s.HasMail(3) {
		t.Error("All() removed mail it only should have read")
	}
	got, ok, err := s.Receive(1)
	if err != nil || !ok || got.Text != "first" {
		t.Errorf("Receive after All() gave %+v, %v, %v", got, ok, err)
	}
}

func TestNonsenseIsRefused(t *testing.T) {
	s, _ := newStore(t)

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
	s, _ := newStore(t)
	if _, ok, err := s.Receive(99); ok || err != nil {
		t.Errorf("receiving for a player with no mail gave ok=%v err=%v", ok, err)
	}
}

func TestAReadOnlyStoreRefusesToWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plrmail")
	s, err := New(mail.Config{Path: path, ReadOnly: true})
	if err != nil {
		t.Fatalf("opening: %v", err)
	}
	if err := s.Send(mail.Message{To: 1, From: 2, Text: "x"}); err == nil {
		t.Error("a read-only store wrote mail")
	}
}
