// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"time"

	"github.com/gerrowadat/disgracelands/internal/persist/reports"
	"github.com/gerrowadat/disgracelands/internal/session"
)

// reportsOrNil returns the report log as the interface, or a nil interface
// when there is none — the same typed-nil trap as the mail system.
func reportsOrNil(s *Server) session.ReportWriter {
	if s.reports == nil {
		return nil
	}
	return &reportKeeper{s: s}
}

type reportKeeper struct{ s *Server }

// Write implements session.ReportWriter, appending straight through to the
// configured format. Unlike bans/mail/houses this does not go through
// Server.background: do_gen_write's own file-full check (act.other.c:908-
// 911) has to answer *before* the command's reply, so the write cannot be
// pushed off the world goroutine the way a save can — the same reasoning
// boards' write-your-message flow already has for the same shape of
// problem. A report file is a handful of lines appended a handful of
// times a session; this is not autosave's hot path.
func (k *reportKeeper) Write(kind, reporter string, room int32, body string) (bool, error) {
	return k.s.reports.Append(reports.Report{
		Kind: reports.Kind(kind), Reporter: reporter, Room: room, Body: body,
		// A live report is happening now — unlike a report imported from
		// classic, which genuinely has no recoverable timestamp (see
		// reports.Report.When and yaml.Store.Append's own doc comments).
		When: time.Now(),
	})
}
