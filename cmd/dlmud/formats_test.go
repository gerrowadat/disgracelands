// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package main

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/gerrowadat/disgracelands/internal/persist/bans"
	"github.com/gerrowadat/disgracelands/internal/persist/boards"
	"github.com/gerrowadat/disgracelands/internal/persist/houses"
	"github.com/gerrowadat/disgracelands/internal/persist/mail"
	"github.com/gerrowadat/disgracelands/internal/persist/player"
	"github.com/gerrowadat/disgracelands/internal/persist/reports"
	"github.com/gerrowadat/disgracelands/internal/persist/world"
)

// TestOnlyTheYamlFormatIsRegistered asserts the property
// docs/proposals/yaml-only.md §3.2 asks to be tested for: **a legacy
// format is not merely rejected by the server, it is absent from it.**
//
// The registries stay — `dlctl` opens classic, binary and ascii by name
// and always will, and a Source backed by a tarball or an embedded FS is
// still something someone might want. What changes is who registers what,
// and the whole of that is a list of blank imports in main.go, which is
// exactly the kind of thing a well-meaning edit puts back.
func TestOnlyTheYamlFormatIsRegistered(t *testing.T) {
	for _, reg := range []struct {
		what    string
		formats []string
	}{
		{"world", world.Formats()},
		{"player", player.Formats()},
		{"bans", bans.Formats()},
		{"boards", boards.Formats()},
		{"mail", mail.Formats()},
		{"houses", houses.Formats()},
		{"reports", reports.Formats()},
	} {
		t.Run(reg.what, func(t *testing.T) {
			if len(reg.formats) != 1 || reg.formats[0] != "yaml" {
				t.Errorf("the %s registry holds %v, want [yaml] only — something has "+
					"blank-imported a legacy decoder back into cmd/dlmud", reg.what, reg.formats)
			}
		})
	}
}

// TestTheLegacyDecodersAreNotLinkedIn is the stronger half of the same
// property, asked of the *build* rather than of a registry.
//
// A registry check can only see packages whose init ran. This asks `go
// list` what cmd/dlmud actually depends on, which catches a legacy
// package pulled in by something other than a blank import — a helper
// borrowed from classic, a type referenced for convenience — which would
// leave the registry clean and the decoder in the binary.
//
// The packages are not deleted from the tree and never will be: classic
// is the world-format parity oracle for as long as the C server is
// authoritative and is how the 1,184 dated nightly world backups get
// read, and binary is the only thing that can read the archived roster at
// all (docs/design/data-format.md §11). They are simply not part of this
// program.
func TestTheLegacyDecodersAreNotLinkedIn(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", ".").Output()
	if err != nil {
		t.Skipf("go list: %v", err)
	}

	forbidden := []string{
		"internal/persist/world/classic",
		"internal/persist/player/ascii",
		"internal/persist/player/binary",
		"internal/persist/bans/classic",
		"internal/persist/boards/classic",
		"internal/persist/mail/classic",
		"internal/persist/houses/classic",
		"internal/persist/reports/classic",
	}
	deps := string(out)
	for _, pkg := range forbidden {
		if strings.Contains(deps, "disgracelands/"+pkg) {
			t.Errorf("cmd/dlmud depends on %s; the server reads one format and the legacy "+
				"decoders belong to dlctl (docs/proposals/yaml-only.md §3.2)", pkg)
		}
	}
}
