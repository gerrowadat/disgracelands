// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package main

import (
	"context"
	"testing"

	"github.com/gerrowadat/disgracelands/internal/persist/world"
)

// worldExampleFixtures is every checked-in world/ pair this repo ships as a
// worked example — both a classic (or binary) source and its yaml
// conversion — held to the two things `dlctl lint --type=world` and
// `dlctl dump --type=world` exist to check: the data loads clean, and the
// two formats agree.
var worldExampleFixtures = []struct {
	name          string
	classicDir    string
	yamlDir       string
	classicFormat string
}{
	{"stock", "../../examples/stock/binary/world", "../../examples/stock/yaml/world", "classic"},
	{"mini", "../../examples/mini/binary/world", "../../examples/mini/yaml/world", "classic"},
}

// TestWorldExamplesLintClean is the regression this repo's own examples
// are held to: `dlctl lint --type=world` finds nothing above informational
// severity in either format. examples/mini's own README has the story of
// what this test would have caught before the zone header was widened —
// a mobile or object outside a zone's own bot–top range is not a lint
// error, it is a mobile or object that silently is not there, which only
// a count (here, via TestWorldExamplesClassicAndYamlAgree) catches.
func TestWorldExamplesLintClean(t *testing.T) {
	for _, fx := range worldExampleFixtures {
		for _, format := range []string{fx.classicFormat, "yaml"} {
			t.Run(fx.name+"/"+format, func(t *testing.T) {
				dir := fx.classicDir
				if format == "yaml" {
					dir = fx.yamlDir
				}
				_, findings, err := loadWorld(context.Background(), dir, format, false, world.Options{})
				if err != nil {
					t.Fatalf("loading %s (%s): %v", dir, format, err)
				}
				// Errors only. examples/stock/binary is a straight copy of
				// data/, warnings and all — the shop that sells ten things
				// that do not exist, the room whose exit is locked by a key
				// that does not — docs/operations.md's own "Checking the
				// world files" section names them, and "warnings do not
				// [fail CI]" is this repo's own settled rule for exactly
				// this reason: the shipped world has had several since
				// before this repo existed, and reproducing them faithfully
				// is the point, not a defect to clear.
				for _, f := range findings {
					if f.Severity == world.Error {
						t.Errorf("%s", f)
					}
				}
			})
		}
	}
}

// TestWorldExamplesClassicAndYamlAgreeOnCounts is scripts/world-parity.sh's
// own check — load the same world two ways and the census must match —
// applied to a pair of formats this tree owns both loaders for, rather
// than to the C server.
//
// Counts, not a full deep-equal: examples/stock/binary is a straight copy
// of data/, and data/'s own classic-vs-yaml comparison has one accepted,
// documented difference (examples/stock/README.md; `TestClassicNativeParity`
// in internal/persist/world/yaml) — a description with a blank line before
// its closing `~` loses that one blank line through goccy/go-yaml's own
// literal-block re-print, a library limitation rather than a defect in
// either loader. Re-asserting full equality here would just be the wrong
// check for that fixture; the count is what actually catches the risk
// this test exists for — a mobile or object silently missing from one
// side, the exact shape of the bug examples/mini/README.md's own "Vnums"
// section found (a zone's yaml file only carries what falls inside its
// own bot–top range).
func TestWorldExamplesClassicAndYamlAgreeOnCounts(t *testing.T) {
	for _, fx := range worldExampleFixtures {
		t.Run(fx.name, func(t *testing.T) {
			classic, _, err := loadWorld(context.Background(), fx.classicDir, fx.classicFormat, false, world.Options{})
			if err != nil {
				t.Fatalf("loading %s (%s): %v", fx.classicDir, fx.classicFormat, err)
			}
			yaml, _, err := loadWorld(context.Background(), fx.yamlDir, "yaml", false, world.Options{})
			if err != nil {
				t.Fatalf("loading %s (yaml): %v", fx.yamlDir, err)
			}
			if classic.Counts != yaml.Counts {
				t.Errorf("%s: classic counts %+v, yaml counts %+v", fx.name, classic.Counts, yaml.Counts)
			}
		})
	}
}
