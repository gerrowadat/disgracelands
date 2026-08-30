// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"context"
	"testing"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// The autosave sweep's read of the world (#325).
//
// It used to be a `w.Players()` inside one DoSync, followed by a
// per-character `Save` — which opens a DoSync of its own — and then reads
// of `c.Client` and `c.Name` straight off the retained `*game.Character`
// values, on the autosave goroutine, while the world goroutine writes
// `c.Client` (Leave nils it). That last part is a data race that `-race`
// could never have found, because the autosave ticker is sixty seconds and
// nothing in the suite runs for sixty seconds.
//
// So what is asserted here is the *shape*: one pass, everything the sweep
// will later need copied on the goroutine that owns it.

func TestSnapshotPlayersTakesEverybodyInOnePass(t *testing.T) {
	srv, _ := newTestServer(t)

	linked, _ := place(t, srv, &game.PlayerRecord{Name: "Ariadne", Level: 20}, 3001)
	unlinked, _ := place(t, srv, &game.PlayerRecord{Name: "Belisarius", Level: 5}, 3001)

	// A body whose player has dropped their connection: still in the world,
	// no client. This is what the sweep's linkdead reaping is looking for,
	// and the only thing that distinguishes it from anybody else.
	inWorld(t, srv, func(*game.Live) { unlinked.Client = nil })

	got, err := srv.snapshotPlayers(context.Background())
	if err != nil {
		t.Fatalf("snapshotPlayers: %v", err)
	}

	byName := map[string]playerSnapshot{}
	for _, p := range got {
		byName[p.name] = p
	}

	if len(byName) != 2 {
		t.Fatalf("the sweep saw %d characters (%v), want 2", len(byName), byName)
	}
	if p, ok := byName["Ariadne"]; !ok || !p.linked {
		t.Errorf("Ariadne: %+v; want present and linked", p)
	}
	if p, ok := byName["Belisarius"]; !ok || p.linked {
		t.Errorf("Belisarius: %+v; want present and not linked", p)
	}

	// The identity carried through is the character itself, because the
	// reaping keys a map on it and hands it back to the world goroutine for
	// w.Remove. Nothing else about it may be touched off that goroutine.
	if byName["Ariadne"].character != linked {
		t.Error("the snapshot does not carry the character it was taken from")
	}

	// And the record is a copy, not a view of the live one. The sweep
	// hands it to the store on a background goroutine, so if it aliased
	// the record the world goroutine is still editing, every save would be
	// a read of live world state from off the world goroutine.
	before := byName["Ariadne"].record.Level
	inWorld(t, srv, func(*game.Live) { linked.Record.Level = before + 7 })
	if after := byName["Ariadne"].record.Level; after != before {
		t.Errorf("the snapshot followed a later change to the live record (%d became %d)", before, after)
	}
}

// TestSnapshotPlayersLeavesMobilesAlone is save_char's first line, which is
// load-bearing rather than defensive: a mobile has a PlayerRecord like
// anybody else, so a sweep that included one would write a player file
// called "a large dog", whose spaces make the roster index unparseable —
// and then every login in the game fails, for everybody.
func TestSnapshotPlayersLeavesMobilesAlone(t *testing.T) {
	srv, _ := newTestServer(t)

	place(t, srv, &game.PlayerRecord{Name: "Ariadne", Level: 20}, 3001)
	inWorld(t, srv, func(w *game.Live) {
		mob := &game.Character{
			Name:     "a large dog",
			NPC:      true,
			Record:   &game.PlayerRecord{Name: "a large dog"},
			Position: game.PosStanding,
		}
		if err := w.Enter(mob, 3001); err != nil {
			t.Errorf("entering the world: %v", err)
		}
		w.Track(mob)
	})

	got, err := srv.snapshotPlayers(context.Background())
	if err != nil {
		t.Fatalf("snapshotPlayers: %v", err)
	}
	for _, p := range got {
		if p.name == "a large dog" {
			t.Fatal("the sweep would have written a player file for a mobile")
		}
	}
	if len(got) != 1 {
		t.Errorf("the sweep saw %d characters, want 1", len(got))
	}
}

// TestSnapshotPlayersWritesWhatBaseRecordSays: the record handed to the
// store is the *base* one, with whatever a spell or a worn shield is
// currently contributing taken back off. Writing the totalled figures is
// how a character's armour class permanently absorbs the shield they
// happened to be wearing at the moment the sweep ran.
func TestSnapshotPlayersWritesWhatBaseRecordSays(t *testing.T) {
	srv, _ := newTestServer(t)

	rec := &game.PlayerRecord{Name: "Ariadne", Level: 20}
	rec.Points.Armor = 100
	game.SnapshotReal(rec)
	rec.Points.Armor = 42 // as if something worn were contributing

	place(t, srv, rec, 3001)

	got, err := srv.snapshotPlayers(context.Background())
	if err != nil {
		t.Fatalf("snapshotPlayers: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("the sweep saw %d characters, want 1", len(got))
	}
	want := game.BaseRecord(*rec)
	if got[0].record.Points.Armor != want.Points.Armor {
		t.Errorf("the sweep would write armour %d, BaseRecord says %d",
			got[0].record.Points.Armor, want.Points.Armor)
	}
}
