// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package yaml

import (
	"context"
	"reflect"
	"testing"

	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/persist/world"
	"github.com/gerrowadat/disgracelands/internal/persist/world/classic"
)

func TestNestFlattenResetsRoundTripsRealZones(t *testing.T) {
	src, err := classic.New(world.Config{Dir: "../../../../data/world"})
	if err != nil {
		t.Fatalf("open classic source: %v", err)
	}
	defer func() { _ = src.Close() }()

	w, err := src.Load(context.Background())
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(w.Zones) == 0 {
		t.Fatal("no zones loaded")
	}

	total := 0
	for _, z := range w.Zones {
		nested := NestResets(z.Commands)
		flat := FlattenResets(nested)
		if !reflect.DeepEqual(flat, z.Commands) {
			t.Errorf("zone %d: flatten(nest(cmds)) != cmds\n got:  %+v\n want: %+v", z.Vnum, flat, z.Commands)
		}
		total += len(z.Commands)
	}
	t.Logf("round-tripped reset chains across %d zones, %d commands total", len(w.Zones), total)
}

func TestNestResetsGroupsConsecutiveIfFlagAsSiblings(t *testing.T) {
	cmds := []game.ResetCommand{
		{Command: 'M', IfFlag: 0, Arg1: 3000},
		{Command: 'G', IfFlag: 1, Arg1: 3050},
		{Command: 'G', IfFlag: 1, Arg1: 3051},
		{Command: 'M', IfFlag: 0, Arg1: 3001},
	}
	nested := NestResets(cmds)
	if len(nested) != 2 {
		t.Fatalf("got %d top-level nodes, want 2", len(nested))
	}
	if len(nested[0].Then) != 2 {
		t.Fatalf("got %d children under the first mob, want 2 (flat siblings, not a deepening chain)", len(nested[0].Then))
	}
	if len(nested[0].Then[0].Then) != 0 {
		t.Fatalf("a child should not itself carry nested children from NestResets")
	}
}
