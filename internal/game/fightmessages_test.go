// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import (
	"os"
	"strings"
	"testing"

	"github.com/gerrowadat/disgracelands/internal/rng"
)

func TestParseMessagesFileMechanics(t *testing.T) {
	src := `* a leading comment
M
 5
die attacker
die victim
#
miss attacker
#
miss room
hit attacker
hit victim
hit room
#
#
#

* a comment between records
M
 6
d2 a
d2 v
d2 r
m2 a
m2 v
m2 r
h2 a
h2 v
h2 r
g2 a
g2 v
g2 r
`
	records, err := ParseMessagesFile(strings.NewReader(src))
	if err != nil {
		t.Fatalf("ParseMessagesFile: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("got %d records, want 2: %+v", len(records), records)
	}

	r0 := records[0]
	if r0.AttackType != 5 {
		t.Errorf("records[0].AttackType = %d, want 5", r0.AttackType)
	}
	want0 := FightMessage{
		AttackType: 5,
		Die:        MsgSet{Attacker: "die attacker", Victim: "die victim", Room: ""},
		Miss:       MsgSet{Attacker: "miss attacker", Victim: "", Room: "miss room"},
		Hit:        MsgSet{Attacker: "hit attacker", Victim: "hit victim", Room: "hit room"},
		God:        MsgSet{},
	}
	if r0 != want0 {
		t.Errorf("records[0] = %+v, want %+v", r0, want0)
	}

	if records[1].AttackType != 6 || records[1].Hit.Attacker != "h2 a" {
		t.Errorf("records[1] = %+v", records[1])
	}
}

func TestParseMessagesFileUnterminatedRecordIsAnError(t *testing.T) {
	if _, err := ParseMessagesFile(strings.NewReader("M\n5\ndie attacker\n")); err == nil {
		t.Error("ParseMessagesFile with a truncated record succeeded, want an error")
	}
}

func TestParseMessagesFileEmptyIsFine(t *testing.T) {
	records, err := ParseMessagesFile(strings.NewReader("* nothing but a comment\n"))
	if err != nil {
		t.Fatalf("ParseMessagesFile: %v", err)
	}
	if len(records) != 0 {
		t.Errorf("got %d records, want 0", len(records))
	}
}

// Against the real archive: exact record count, and the real weapon-type
// (300-314) entries that make the damage()-dispatch fallback matter —
// confirmed present, not assumed.
func TestParseMessagesFileAgainstTheRealArchive(t *testing.T) {
	f, err := os.Open("../../examples/stock/binary/misc/messages")
	if err != nil {
		t.Fatalf("opening: %v", err)
	}
	defer func() { _ = f.Close() }()

	records, err := ParseMessagesFile(f)
	if err != nil {
		t.Fatalf("ParseMessagesFile: %v", err)
	}
	if len(records) != 55 {
		t.Errorf("got %d records, want 55", len(records))
	}

	fm := NewFightMessages(records)
	for _, attackType := range []int32{300, 301, 303} {
		if _, ok := fm.Pick(attackType, rng.NewRand(rng.NewModern(1))); !ok {
			t.Errorf("no registered message for weapon attack type %d, want one (real archive)", attackType)
		}
	}
	if _, ok := fm.Pick(999999, rng.NewRand(rng.NewModern(1))); ok {
		t.Error("Pick found something for an attack type nothing registers")
	}
}

// The C's own list is built by *prepending* each new record for a type it
// has already seen, so the last-read record ends up first
// (fight.c:174-176) — Pick has to walk that order, not file order, for a
// given dice(1,n) roll to land on the same text the C would show.
func TestFightMessagesPickMatchesThePrependOrder(t *testing.T) {
	records := []FightMessage{
		{AttackType: 300, Hit: MsgSet{Attacker: "first in file"}},
		{AttackType: 300, Hit: MsgSet{Attacker: "second in file"}},
		{AttackType: 300, Hit: MsgSet{Attacker: "third in file"}},
	}
	fm := NewFightMessages(records)

	// The C's list, after three prepends in file order, is
	// [third, second, first] — dice(1,3) == 1 must land on "third".
	seedGivingRollOne := findSeedForDiceOne(t)
	got, ok := fm.Pick(300, rng.NewRand(rng.NewModern(seedGivingRollOne)))
	if !ok || got.Hit.Attacker != "third in file" {
		t.Errorf("Pick landed on %+v, want the third-in-file record (prepend order)", got)
	}
}

// findSeedForDiceOne finds a seed whose first Dice(1,3) call returns 1, so
// the prepend-order test above has a deterministic roll to assert against
// without hard-coding a seed that might stop working if the generator's
// output for it ever changes.
func findSeedForDiceOne(t *testing.T) uint64 {
	t.Helper()
	for seed := uint64(1); seed < 10000; seed++ {
		if rng.NewRand(rng.NewModern(seed)).Dice(1, 3) == 1 {
			return seed
		}
	}
	t.Fatal("no seed in range gave Dice(1,3) == 1")
	return 0
}
