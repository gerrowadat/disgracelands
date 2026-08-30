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
	"github.com/gerrowadat/disgracelands/internal/session"
)

// `reloadzone` — reloadmob's own zone-wide extension: re-reads a zone's
// definition plus every room and mobile in its vnum range from disk, and
// applies them to the running world without a restart. New capability,
// not a C port; see docs/deviations.md and docs/design/go-port-plan.md.

// ReloadZone implements session.ZoneReloader: re-reads vnum's zone,
// rooms and mobiles from the world data this server booted with. Runs
// inline on the world goroutine, the same deliberate exception
// ReloadMobile already takes and documents.
func (s *Server) ReloadZone(w *game.Live, vnum game.ZoneVnum) (game.ReloadZoneResult, error) {
	if s.worldDir == "" {
		return game.ReloadZoneResult{}, ErrWorldReloadNotConfigured
	}

	src, err := world.Open(DataFormat, world.Config{Dir: s.worldDir, Mini: s.worldMini})
	if err != nil {
		return game.ReloadZoneResult{}, err
	}
	defer func() { _ = src.Close() }()

	defs, err := src.Load(context.Background())
	if err != nil {
		return game.ReloadZoneResult{}, fmt.Errorf("reading the world data: %w", err)
	}

	var fresh *game.ZoneDef
	for _, z := range defs.Zones {
		if z.Vnum == vnum {
			fresh = z
			break
		}
	}
	if fresh == nil {
		return game.ReloadZoneResult{}, fmt.Errorf("zone #%d does not exist", vnum)
	}

	result, ok := w.ReloadZone(fresh, defs.Rooms, defs.Mobiles, s.rng)
	if !ok {
		return game.ReloadZoneResult{}, session.ErrZoneEngaged
	}
	return result, nil
}
