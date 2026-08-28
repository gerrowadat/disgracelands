// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package session

import (
	"fmt"
	"log/slog"

	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/obs"
)

// mudlog() (utils.c:229) did two things with one line: wrote it to the log,
// and echoed it in-game to every online immortal whose own level and syslog
// verbosity qualified. The Go split those apart — the writing is slog, the
// echoing is obs.WithWizVisEcho over a record carrying obs.WizLevel and
// obs.WizType (internal/obs/log.go), delivered by Server.echoWizVis
// (internal/server/wizvis.go) — and the helpers here are what put the two
// halves back together at a call site, so that porting a mudlog() is one
// line rather than four.
//
// The message is the C's own `str` argument verbatim, formatted the same
// way, because it is not just a log line: echoWizVis relays a record's
// message, exactly as mudlog's `str` served both jobs. So these are
// deliberately not the lowercase-noun-plus-attributes shape the rest of
// this tree's structured logging uses. Attributes are still welcome
// alongside — see [Context.wizlogAttrs] — but the message carries the
// text a god actually reads.

// wizlog is one mudlog(str, type, level, TRUE) call. typ is one of the
// obs.Log* verbosities (mudlog()'s BRF/NRM/CMP/OFF) and level is its level
// argument, taken as given: use it where the C passes a bare constant.
//
// A command run with no session behind it — a special procedure driving a
// mobile through SpecialCall, say — has nowhere to log to, and is skipped
// rather than panicking, the same guard `bug` already carries.
func (c *Context) wizlog(typ int, level int32, format string, args ...any) {
	c.wizlogAttrs(typ, level, fmt.Sprintf(format, args...))
}

// wizlogInvis is the same, for the C's much commoner
// `MAX(LVL_x, GET_INVIS_LEV(ch))` spelling of the level argument: a god
// acting while wizinvis must not be reported to anybody who could not have
// seen them do it. actor is the C's own `ch` — whoever the MAX() is taken
// against, which is not always the character running the command
// (close_socket takes it against the player being dropped, comm.c:1974).
func (c *Context) wizlogInvis(typ int, level int32, actor *game.Character, format string, args ...any) {
	c.wizlogAttrs(typ, wizlogLevel(level, actor), fmt.Sprintf(format, args...))
}

// wizlogAttrs is the shared tail, and the form to reach for when a call site
// has structured attributes worth keeping as well as the C's text.
func (c *Context) wizlogAttrs(typ int, level int32, message string, attrs ...any) {
	if c.Session == nil {
		return
	}
	c.Session.logger.Info(message, append(attrs,
		obs.WizLevel(int(level)), obs.WizType(typ))...)
}

// wizlogLevel is `MAX(level, GET_INVIS_LEV(ch))`. GET_INVIS_LEV on a mobile
// or on a character with no record is zero, so the constant wins — which is
// what the C's own macro does, reading a player_specials field that is
// zeroed for anybody who has not set it.
func wizlogLevel(level int32, actor *game.Character) int32 {
	if actor == nil || actor.Record == nil {
		return level
	}
	return max(level, actor.Record.InvisLevel)
}

// wizlogInvis on a SpecialCall is the same helper for a special procedure,
// which has a Session but no Context. A special running on a pulse has no
// session at all — nothing typed it — and logs nothing, the same guard the
// Context form carries.
func (sc *SpecialCall) wizlogInvis(typ int, level int32, actor *game.Character, format string, args ...any) {
	if sc.Session == nil {
		return
	}
	wizlog(sc.Session.logger, typ, wizlogLevel(level, actor), format, args...)
}

// wizlog, the package-level one, is for the pieces of the login and
// connection machinery that have a logger but no Context — nanny's own
// mudlogs happen before there is a command, let alone a Context to run it
// in — and for SpecialCall's method above. Same two attributes, hung off a
// plain *slog.Logger.
func wizlog(logger *slog.Logger, typ int, level int32, format string, args ...any) {
	if logger == nil {
		return
	}
	logger.Info(fmt.Sprintf(format, args...),
		obs.WizLevel(int(level)), obs.WizType(typ))
}
