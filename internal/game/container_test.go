// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import (
	"os"
	"regexp"
	"strconv"
	"testing"
)

// handlerCSource is the C money_desc was transcribed from.
const handlerCSource = "../../reference/moderncserver/src/handler.c"

// TestMoneyDescriptionsMatchTheCSource re-parses money_desc's table.
//
// Fourteen thresholds and fourteen phrases, none of them guessable: a hundred
// and ninety coins is "a small pile" and two hundred and one is "a pile". They
// are checked against the source rather than trusted, and every boundary is
// checked on both sides, because an off-by-one here is invisible until
// somebody notices the wrong noun on the floor.
func TestMoneyDescriptionsMatchTheCSource(t *testing.T) {
	src, err := os.ReadFile(handlerCSource)
	if err != nil {
		t.Fatalf("reading the C source money_desc came from: %v", err)
	}

	// The table entries are `{ limit, "description" },` inside money_desc,
	// which is the only thing in handler.c shaped like that.
	entry := regexp.MustCompile(`\{\s*(\d+),\s*"([^"]+)"\s*\}`)
	matches := entry.FindAllStringSubmatch(string(src), -1)
	if len(matches) != len(moneyDescriptions) {
		t.Fatalf("parsed %d entries from the C, have %d", len(matches), len(moneyDescriptions))
	}

	for i, m := range matches {
		limit, err := strconv.ParseInt(m[1], 10, 32)
		if err != nil {
			t.Fatalf("entry %d: %v", i, err)
		}
		if got := moneyDescriptions[i]; int64(got.limit) != limit || got.desc != m[2] {
			t.Errorf("entry %d is {%d, %q}, want {%d, %q}", i, got.limit, got.desc, limit, m[2])
		}

		// The boundary itself takes this description; one more takes the
		// next.
		if got := MoneyDescription(int32(limit)); got != m[2] { //nolint:gosec // parsed as int32
			t.Errorf("%d coins is %q, want %q", limit, got, m[2])
		}
		if i+1 < len(matches) {
			if got := MoneyDescription(int32(limit) + 1); got != matches[i+1][2] { //nolint:gosec // parsed as int32
				t.Errorf("%d coins is %q, want %q", limit+1, got, matches[i+1][2])
			}
		}
	}

	// Past the end of the table there is one more phrase, and the C returns it
	// for anything larger.
	if got := MoneyDescription(1_000_001); got != "an absolutely colossal mountain of gold coins" {
		t.Errorf("a million and one coins is %q", got)
	}
}

// TestContainerFlagsLiveInValueOne, which is the part that is easy to get
// wrong: a container's open/closed state is an object *value*, not an object
// flag, and the two are different fields entirely.
func TestContainerFlagsLiveInValueOne(t *testing.T) {
	chest := &Object{Type: ItemContainer}
	chest.Values[containerCapacity] = 100
	chest.Values[containerKeyValue] = 3010

	if chest.ContainerClosed() || chest.ContainerLocked() {
		t.Error("a container with no flags starts closed or locked")
	}

	chest.SetContainerFlag(ContClosed, ContLocked)
	// The literal 12 rather than a computed mask, on purpose: value 1 is
	// the *file format*, so what this test is for is that CONT_CLOSED and
	// CONT_LOCKED are still bits 2 and 3 in the slot the C reads. A mask
	// built from the same two constants the implementation uses would
	// agree with itself whatever they were.
	if want := int32(1<<ContClosed | 1<<ContLocked); chest.Values[containerFlagsValue] != want || want != 12 {
		t.Errorf("value 1 is %d, want %d (bits 2 and 3)", chest.Values[containerFlagsValue], want)
	}
	if !chest.ContainerClosed() || !chest.ContainerLocked() {
		t.Error("the flags did not take")
	}

	chest.ClearContainerFlag(ContLocked)
	if chest.ContainerLocked() || !chest.ContainerClosed() {
		t.Error("unlocking cleared the wrong bit")
	}

	if chest.Capacity() != 100 {
		t.Errorf("capacity is %d, want 100", chest.Capacity())
	}
	if chest.ContainerKey() != 3010 {
		t.Errorf("the key is %d, want 3010", chest.ContainerKey())
	}

	// A container with no key at all reads as NoObject rather than vnum zero,
	// which is what makes "you can't seem to find a keyhole" reachable.
	if (&Object{Type: ItemContainer}).ContainerKey() != NoObject {
		t.Error("a keyless container claims to have key 0")
	}
}

// TestACorpseIsAContainerThatHoldsNothingMore. A corpse's capacity is zero, so
// perform_put's weight check refuses everything — which is how the C stops you
// using bodies as luggage.
func TestACorpseIsAContainerThatHoldsNothingMore(t *testing.T) {
	l := objectWorld()
	welmar := newCharacter("Welmar")
	if err := l.Enter(welmar, 3001); err != nil {
		t.Fatal(err)
	}
	corpse := l.MakeCorpse(welmar)

	if !corpse.IsContainer() {
		t.Error("a corpse is not a container")
	}
	if corpse.Capacity() != 0 {
		t.Errorf("a corpse holds %d, want 0", corpse.Capacity())
	}
	if corpse.ContainerClosed() {
		t.Error("a corpse is closed")
	}
}
