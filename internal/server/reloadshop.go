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

// `reloadshop` — reloadmob's own shop counterpart: hot-reloads a shop's
// configuration from disk without a restart. New capability, not a C
// port; see docs/deviations.md and docs/proposals/go-port-plan.md.

// ReloadShop implements session.ShopReloader: re-reads vnum's
// configuration from the world data this server booted with, and
// applies it to the running shop. Runs inline on the world goroutine,
// the same deliberate exception ReloadMobile already takes and
// documents.
func (s *Server) ReloadShop(w *game.Live, vnum game.ShopVnum) error {
	if s.worldDir == "" {
		return ErrWorldReloadNotConfigured
	}

	src, err := world.Open(s.worldFormat, world.Config{Dir: s.worldDir, Mini: s.worldMini})
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()

	defs, err := src.Load(context.Background())
	if err != nil {
		return fmt.Errorf("reading the world data: %w", err)
	}

	var fresh *game.ShopDef
	for _, sh := range defs.Shops {
		if sh.Vnum == vnum {
			fresh = sh
			break
		}
	}
	if fresh == nil {
		return fmt.Errorf("shop #%d does not exist", vnum)
	}

	if !w.ReloadShop(fresh) {
		return fmt.Errorf("shop #%d is not in the running world", vnum)
	}
	return nil
}
