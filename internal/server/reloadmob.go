// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"context"
	"errors"
	"fmt"

	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/persist/world"
	"github.com/gerrowadat/disgracelands/internal/session"
)

// `reloadmob` — hot-reloading a mobile's definition from disk without a
// restart. This is new capability, not a port of anything in
// reference/moderncserver: interpreter.c has no equivalent command. See
// docs/deviations.md and docs/proposals/go-port-plan.md for why it exists
// and where OasisOLC's in-game editors were decided against instead.

// ErrWorldReloadNotConfigured is returned when worldDir is empty — the
// same posture an empty clockPath already takes for clock persistence,
// and what a test world gets by default.
var ErrWorldReloadNotConfigured = errors.New("world reload is not configured")

// ReloadMobile implements session.MobReloader: re-reads vnum's definition
// from the world data this server booted with, and applies it to the
// running world. Runs inline on the world goroutine — the same
// deliberate exception Text.Reload already takes and documents ("a dozen
// small text files... the pulse budget is 100ms and the read is well
// inside it"), extended here to a full world read for the same reasons:
// an implementor-only command, run about as often as a builder fixes a
// mistake.
func (s *Server) ReloadMobile(w *game.Live, vnum game.MobVnum) (refreshed int, err error) {
	if s.worldDir == "" {
		return 0, ErrWorldReloadNotConfigured
	}

	src, err := world.Open(s.worldFormat, world.Config{Dir: s.worldDir, Mini: s.worldMini})
	if err != nil {
		return 0, err
	}
	defer func() { _ = src.Close() }()

	defs, err := src.Load(context.Background())
	if err != nil {
		return 0, fmt.Errorf("reading the world data: %w", err)
	}

	var fresh *game.MobDef
	for _, m := range defs.Mobiles {
		if m.Vnum == vnum {
			fresh = m
			break
		}
	}
	if fresh == nil {
		return 0, fmt.Errorf("mob #%d does not exist", vnum)
	}

	n, ok := w.ReloadMobile(fresh, s.rng)
	if !ok {
		return 0, session.ErrMobEngaged
	}
	return n, nil
}
