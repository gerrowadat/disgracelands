// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"context"
	"errors"
	"time"

	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/persist/player"
)

// SweepRentFiles is update_obj_file (objsave.c:332): a boot-time pass over
// every character on the roster, deleting whichever rent or crash file has
// sat unclaimed past its own kind's timeout. The C calls this unconditionally
// unless `-q` was given (db.c:456); `--skip-rent-check` is this port's own
// name for the same flag, and main.go's own boot sequence is what makes the
// call conditional — this method does not check it itself, the same way
// BootReset does not check anything about how it was invoked either.
func (s *Server) SweepRentFiles(ctx context.Context) {
	if s.objects == nil {
		return
	}
	// Nothing expires while rent is free, which is a deliberate difference
	// from the C: update_obj_file runs unconditionally there, whatever
	// free_rent is set to (db.c:456).
	//
	// The argument is that the sweep is the enforcement half of a charge
	// that is not being made. A rent file times out because its owner
	// stopped paying for the room it is in — but nobody on this server ever
	// paid: free_rent is YES (config.c:133), so the receptionist refuses to
	// price a stay at all, do_quit stores Crash_rentsave(ch, 0), and every
	// rent file in the archive carries a per-day cost of zero
	// (game/shopstate.go's own note says the same). Deleting somebody's
	// possessions for falling behind on a bill of nothing is a rule with
	// its reason removed.
	//
	// It also removes the failure #294 was filed for, which is what makes
	// this worth the deviation rather than merely defensible: convert an
	// archived lib/, boot on it, and the first sweep deleted the stored
	// possessions of every character who had not played for thirty days —
	// which, for an archive, is all of them. The conversion's whole purpose
	// is to preserve what was there, and the very next step destroyed a
	// large part of it, silently and with nothing to undo it.
	//
	// An operator who turns rent charging on gets the C's behaviour back
	// unchanged, timeouts and all, because then the charge exists and so
	// does the reason.
	if game.Tuning().FreeRent {
		return
	}
	now := time.Now()
	for entry, err := range s.players.List(ctx) {
		if err != nil {
			// A malformed roster line: nothing to clean a rent file by
			// name for. List has already reported the line elsewhere
			// (docs/deviations.md's own entry on this); skip it here too
			// rather than aborting the whole sweep over one bad line.
			continue
		}
		s.cleanOneRentFile(ctx, entry.Name, now)
	}
}

// crashFileTimeout and rentFileTimeout are config.c's own crash_file_timeout
// (10) and rent_file_timeout (30) — real days, `SECS_PER_REAL_DAY`, not MUD
// ones. Constants rather than configuration, the same reasoning
// docs/deviations.md's "rent settings are constants, not options" entry
// already gives free_rent and the rest: the archive's own values are what
// the game was.
const (
	crashFileTimeout = 10 * 24 * time.Hour
	rentFileTimeout  = 30 * 24 * time.Hour
)

// cleanOneRentFile is Crash_clean_file (objsave.c:276), for one character.
//
// RentCryo and RentUndef are never swept — the C's own if/else-if has no
// case for either, so a cryo-frozen character's things wait for them
// indefinitely and an undefined rentcode (which should not happen) is left
// alone rather than guessed at.
func (s *Server) cleanOneRentFile(ctx context.Context, name string, now time.Time) {
	f, err := s.objects.LoadObjects(ctx, name)
	if err != nil {
		if !errors.Is(err, player.ErrNotFound) {
			// Crash_clean_file's own SYSERR log for an fopen failure that
			// is not ENOENT (objsave.c:290-291) — every character with no
			// rent file at all takes this same early return silently, the
			// overwhelming majority, so only a genuine read failure is
			// worth a line in the log.
			s.logger.Warn("reading a rent file to sweep it", "character", name, "error", err)
		}
		return
	}

	var timeout time.Duration
	var kind string
	switch f.Code {
	case player.RentCrash:
		timeout, kind = crashFileTimeout, "crash"
	case player.RentForced:
		timeout, kind = crashFileTimeout, "forced rent"
	case player.RentTimedOut:
		timeout, kind = crashFileTimeout, "idlesave"
	case player.RentRented:
		timeout, kind = rentFileTimeout, "rent"
	default:
		return
	}
	// A rent file that does not say when it was written is never swept.
	//
	// The C cannot reach this: `rent.time` is an int32 in a struct that is
	// always fully present, so there is no "missing" to have an opinion
	// about. The yaml format can — `written:` is `omitempty`, and a file
	// that lacks it (hand-edited, truncated, or written by something that
	// had no timestamp to give) reads back as the zero time. Every one of
	// those was then `now.Sub(zero)`, about two thousand years, and every
	// one of them was deleted on the next boot.
	//
	// IsZero is the right test and not an approximation, which is the part
	// worth being careful about: a rent file that genuinely says
	// 1970-01-01 reads back as time.Unix(0, 0) — an ordinary instant, not
	// the zero Time, whose year is 1. So this distinguishes "the file did
	// not tell us" from "the file said the epoch", and a real archived
	// timestamp of 0 is still swept exactly as the C sweeps it.
	//
	// Refusing to act is the only safe direction. Deleting somebody's
	// possessions is not undoable, and doing it on the strength of a value
	// we failed to read is the worst of the available outcomes. Part of
	// #294; the rest of that issue is about rent files whose timestamps
	// are real and simply old, which is a policy question and not this.
	if f.Written.IsZero() {
		s.logger.Warn("a rent file has no timestamp, so it will not be swept",
			"character", name, "kind", kind)
		return
	}

	// Strictly older than the timeout, matching the C's own
	// `rent.time < time(0) - timeout` exactly: a file exactly at the
	// boundary survives one more sweep, not zero.
	if now.Sub(f.Written) <= timeout {
		return
	}

	if err := s.objects.DeleteObjects(ctx, name); err != nil {
		s.logger.Error("deleting a timed-out rent file", "character", name, "kind", kind, "error", err)
		return
	}
	// "    Deleting %s's %s file." / "    Deleting %s's rent file."
	// (objsave.c:318,325) — the boot log line an operator would have seen.
	s.logger.Info("deleted a timed-out rent file", "character", name, "kind", kind)
}
