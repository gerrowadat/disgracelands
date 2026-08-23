// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package yaml

import (
	"context"
	"testing"

	"github.com/gerrowadat/disgracelands/internal/persist/world"
	"github.com/gerrowadat/disgracelands/internal/persist/world/classic"
)

func TestAbilitiesRoundTripsRealMobiles(t *testing.T) {
	src, err := classic.New(world.Config{Dir: "../../../../examples/stock/binary/world"})
	if err != nil {
		t.Fatalf("open classic source: %v", err)
	}
	defer func() { _ = src.Close() }()

	w, err := src.Load(context.Background())
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	enhanced := 0
	for _, m := range w.Mobiles {
		if len(m.Especs) == 0 {
			continue
		}
		enhanced++
		abilities, unknown := AbilitiesFromEspecs(m.Especs)
		if len(unknown) != 0 {
			t.Errorf("mobile #%d: unrecognised espec keys: %v", m.Vnum, unknown)
			continue
		}
		back := EspecsFromAbilities(abilities)
		if len(back) != len(m.Especs) {
			t.Errorf("mobile #%d: got %d especs back, want %d (%+v vs %+v)",
				m.Vnum, len(back), len(m.Especs), back, m.Especs)
			continue
		}
		// Order need not match the file's (espec order isn't semantic —
		// EspecsFromAbilities uses a fixed key order), but the set of
		// key/value pairs must.
		want := map[string]string{}
		for _, e := range m.Especs {
			want[e.Key] = e.Value
		}
		for _, e := range back {
			if want[e.Key] != e.Value {
				t.Errorf("mobile #%d: espec %s = %s, want %s", m.Vnum, e.Key, e.Value, want[e.Key])
			}
		}
	}
	t.Logf("round-tripped abilities for %d enhanced mobiles", enhanced)
}
