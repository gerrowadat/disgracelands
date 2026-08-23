// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"github.com/gerrowadat/disgracelands/internal/colour"
	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/session"
)

// echoWizVis is mudlog()'s own in-game half (utils.c:243-258), supplied to
// obs.WithWizVisEcho so every wizvis-tagged log call site — currently just
// bug/idea/typo (internal/session/report.go), the first of what
// docs/deviations.md calls "every mudlog call site in the ported commands,
// a would-be producer" — actually reaches somebody.
//
// The C's own selection, reproduced condition for condition rather than
// approximated: `STATE(i) != CON_PLAYING || IS_NPC(i->character)` in one
// go (a switched god's *current* character is the mobile they are inside,
// so IS_NPC alone already excludes them — no separate "switched" check
// exists in the C and none is added here), then level, then PLR_WRITING
// (mid-edit — a line arriving inside somebody's own text buffer would be
// worse than not seeing it at all), then the reader's own syslog verbosity
// against the message's type.
func (s *Server) echoWizVis(typ, level int, message string) {
	for _, sess := range s.Sessions() {
		if sess.State() != session.StatePlaying {
			continue
		}
		ch := sess.Character()
		if ch == nil || ch.IsNPC() {
			continue
		}
		rec := ch.Record
		if rec == nil || int(rec.Level) < level {
			continue
		}
		if rec.PlayerFlags.Has(game.PlayerWriting) {
			continue
		}
		if session.SyslogLevel(rec) < typ {
			continue
		}
		// CCGRN(i->character, C_NRM) ... "[ %s ]\r\n" ... CCNRM (utils.c:255-257).
		sess.SendAt(colour.Normal, "{{green}}[ %s ]{{/}}\r\n", message)
	}
}
