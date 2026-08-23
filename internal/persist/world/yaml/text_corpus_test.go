// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package yaml

import (
	"context"
	"strings"
	"testing"

	"github.com/gerrowadat/disgracelands/internal/persist/world"
	"github.com/gerrowadat/disgracelands/internal/persist/world/classic"
	"github.com/goccy/go-yaml"
)

// roundTripString encodes s through Text and decodes it back, the same path
// a zone file's writer and reader take.
func roundTripString(s string) (string, error) {
	out, err := yaml.MarshalWithOptions(textDoc{Desc: Text(s)}, yaml.Indent(2))
	if err != nil {
		return "", err
	}
	var back textDoc
	if err := yaml.UnmarshalWithOptions(out, &back, yaml.Strict()); err != nil {
		return "", err
	}
	return string(back.Desc), nil
}

// TestTextRoundTripsRealCorpus is §2.4/§10.3's gate: every string in the
// real world data must survive Text's encode/decode byte-for-byte before
// the format can be trusted with it. It loads data/world via classic (the
// same files scripts/world-parity.sh checks against the C server) and
// round-trips every prose field found on a room, mobile, object or shop.
func TestTextRoundTripsRealCorpus(t *testing.T) {
	src, err := classic.New(world.Config{Dir: "../../../../examples/stock/binary/world"})
	if err != nil {
		t.Fatalf("open classic source: %v", err)
	}
	defer func() { _ = src.Close() }()

	w, err := src.Load(context.Background())
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	var strs []string
	for _, r := range w.Rooms {
		strs = append(strs, r.Name, r.Description)
		for _, e := range r.Exits {
			if e != nil {
				strs = append(strs, e.Description, e.Keywords)
			}
		}
		for _, ed := range r.ExtraDescs {
			strs = append(strs, ed.Keywords, ed.Description)
		}
	}
	for _, m := range w.Mobiles {
		strs = append(strs, m.Keywords, m.ShortDesc, m.LongDesc, m.Description)
	}
	for _, o := range w.Objects {
		strs = append(strs, o.Keywords, o.ShortDesc, o.Description, o.ActionDesc)
		for _, ed := range o.ExtraDescs {
			strs = append(strs, ed.Keywords, ed.Description)
		}
	}

	tested, failed := 0, 0
	for _, s := range strs {
		if s == "" {
			continue
		}
		tested++
		// The yaml format's stored form is LF-only (ToStored/FromStored's
		// doc comment on text.go explains why); a full classic -> yaml ->
		// classic round trip goes through both conversions, which this
		// mirrors without needing a real classic writer to exist yet.
		stored, err := roundTripString(ToStored(s))
		if err != nil {
			t.Errorf("round trip error on %q: %v", s, err)
			failed++
			continue
		}
		got := FromStored(stored)
		if got != s {
			// The one documented, accepted lossy transform (TrimsTrailingBlankLines):
			// a string with 2+ trailing newlines normalises down to exactly one.
			// Anything else is a real failure.
			if TrimsTrailingBlankLines(ToStored(s)) && got == FromStored(strings.TrimRight(ToStored(s), "\n")+"\n") {
				continue
			}
			t.Errorf("round trip mismatch:\n input:  %q\n output: %q", s, got)
			failed++
			if failed > 40 {
				t.Fatal("too many failures, stopping")
			}
		}
	}
	t.Logf("round-tripped %d strings from the real corpus, %d failed (excluding the documented trailing-blank-line normalisation)", tested, failed)
}
