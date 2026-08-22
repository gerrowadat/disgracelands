// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"context"
	"time"

	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/persist/clock"
)

// RunClockSave writes the mud clock's epoch back periodically, porting
// comm.c:921-922's `if (!(pulse % PULSE_TIMESAVE)) save_mud_time(...)`.
// Mirrors RunAutosave's shape.
func (s *Server) RunClockSave(ctx context.Context) {
	if s.clockPath == "" {
		return
	}
	ticker := time.NewTicker(clockSaveInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.saveClock(ctx)
		}
	}
}

// saveClock is save_mud_time (db.c:534), called here and once more at
// shutdown (SaveEverything, mirroring comm.c:441's call right after
// save_all()). A no-op when no clock path is configured — the same
// "nothing to disable, just nothing configured" posture bans/boards/mail
// take when their Store is nil.
func (s *Server) saveClock(ctx context.Context) {
	if s.clockPath == "" {
		return
	}
	var epoch time.Time
	if err := s.engine.DoSync(ctx, func(w *game.Live) {
		epoch = w.SavedEpoch(time.Now())
	}); err != nil {
		s.logger.Error("collecting the mud clock to save", "error", err)
		return
	}
	s.background(func() {
		if err := clock.Save(s.clockFormat, s.clockPath, epoch); err != nil {
			s.logger.Error("writing the mud clock", "error", err)
		}
	})
}
