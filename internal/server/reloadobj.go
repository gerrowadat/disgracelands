// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"context"
	"fmt"

	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/persist/world"
)

// `reloadobj` — reloadmob's own object counterpart: hot-reloads an object
// prototype from disk without a restart. New capability, not a C port;
// see docs/deviations.md and docs/design/go-port-plan.md.

// ReloadObject implements session.ObjectReloader: re-reads vnum's
// definition from the world data this server booted with, and applies
// it to the running world's prototype. Runs inline on the world
// goroutine, the same deliberate exception ReloadMobile already takes
// and documents.
func (s *Server) ReloadObject(w *game.Live, vnum game.ObjVnum) error {
	if s.worldDir == "" {
		return ErrWorldReloadNotConfigured
	}

	src, err := world.Open(DataFormat, world.Config{Dir: s.worldDir, Mini: s.worldMini})
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()

	defs, err := src.Load(context.Background())
	if err != nil {
		return fmt.Errorf("reading the world data: %w", err)
	}

	var fresh *game.ObjDef
	for _, o := range defs.Objects {
		if o.Vnum == vnum {
			fresh = o
			break
		}
	}
	if fresh == nil {
		return fmt.Errorf("object #%d does not exist", vnum)
	}

	if !w.ReloadObject(fresh) {
		return fmt.Errorf("object #%d is not in the running world", vnum)
	}
	return nil
}
