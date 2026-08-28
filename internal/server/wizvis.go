// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"fmt"

	"github.com/gerrowadat/disgracelands/internal/colour"
	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/obs"
	"github.com/gerrowadat/disgracelands/internal/session"
)

// wizlog is one mudlog(str, type, level, TRUE) call from the server side —
// the same job internal/session's own wizlog does for a command, for the
// pieces of mudlog()'s call graph that live out here instead: close_socket,
// the zone reset queue, the idle force-rent and the rent-file loader.
//
// The message is the C's `str` verbatim, because echoWizVis relays a
// record's message and mudlog's own `str` served both halves the same way.
func (s *Server) wizlog(typ int, level int32, format string, args ...any) {
	s.logger.Info(fmt.Sprintf(format, args...),
		obs.WizLevel(int(level)), obs.WizType(typ))
}

// wizlogInvis is the same, for the C's `MAX(LVL_x, GET_INVIS_LEV(ch))`
// spelling: a wizinvis god's doings are reported no lower than the level
// they are hiding at.
func (s *Server) wizlogInvis(typ int, level int32, actor *game.Character, format string, args ...any) {
	if actor != nil && actor.Record != nil {
		level = max(level, actor.Record.InvisLevel)
	}
	s.wizlog(typ, level, format, args...)
}

// echoWizVis is mudlog()'s own in-game half (utils.c:243-258), supplied to
// obs.WithWizVisEcho so every wizvis-tagged log call site reaches somebody.
// The producers are the two helpers above and internal/session's pair of the
// same name; docs/deviations.md lists which of the C's mudlog() call sites
// are ported to them and which are not.
//
// The C's own selection, reproduced condition for condition rather than
// approximated — see sendWizVis below, which is that loop.
//
// **The selection runs on the world goroutine**, queued rather than done
// inline, and that is not tidiness. Deciding who a line reaches means
// reading levels, player flags and syslog preferences off live
// PlayerRecords, which the world goroutine owns; doing it wherever the
// log call happened to be made is a data race. It was a latent one while
// `bug` was the only producer, because a command runs on the world
// goroutine already. #134's audit added producers on the login goroutine,
// on a connection's teardown and on a background save, and -race found it
// within one run of the suite.
//
// `Do` rather than `DoSync`: this is called *from* the world goroutine as
// often as not, where DoSync would deadlock, and a queue full enough to
// refuse the task is a server with worse problems than a missed syslog
// line. The cost is that the echo lands a task later than the log write
// rather than inside it — invisible to a player, but not to a test: a
// command's own reply is no longer proof that its echo has been decided,
// so the tests wait on the line itself or on a later task.
//
// A Server built without an engine — nothing in the tree does, but the
// field is an option — logs and echoes nothing, rather than panicking.
func (s *Server) echoWizVis(typ, level int, message string) {
	// `if (level < 0) return;` (utils.c:238-239), which mudlog does after
	// the file write and before the loop — so a negative level means
	// "log this and show it to nobody", not "show it to everybody". The
	// distinction is not academic: do_skillset is `mudlog(buf2, BRF, -1,
	// TRUE)` (modify.c:344), the one call site in the tree that passes a
	// level rather than an LVL_ constant, and reading it as an unusually
	// low threshold gets it exactly backwards. See docs/weirdnumbers.md.
	if level < 0 {
		return
	}
	if s.engine == nil {
		return
	}
	_ = s.engine.Do(func(w *game.Live) { sendWizVis(w, typ, level, message) })
}

// sendWizVis is echoWizVis' loop, on the world goroutine. See above.
//
// It walks the world's players rather than the server's sessions, which is
// the same set the C's `descriptor_list` plus `STATE(i) == CON_PLAYING`
// picks out and is the one this goroutine owns: a character in the world
// has been put there by Enter, and its Record — the level, the player
// flags and the syslog preference this all turns on — is world state. A
// linkdead body is in that list with no Client, and TellAt on it is a
// no-op, which is the right answer for a descriptor that is not there.
//
// `IS_NPC(i->character)` is still checked, even though Players() already
// excludes mobiles, because in the C it is what excludes a switched god:
// their *current* character is the mobile they are inside. Here the same
// thing happens one step earlier — a switched god's own body is what
// stayed in the world — so this is belt and braces rather than the
// load-bearing check it is there.
func sendWizVis(w *game.Live, typ, level int, message string) {
	for _, ch := range w.Players() {
		if ch.IsNPC() {
			continue
		}
		rec := ch.Record
		if rec == nil || int(rec.Level) < level {
			continue
		}
		// PLR_WRITING (utils.c:248): mid-edit, a line arriving inside
		// somebody's own text buffer would be worse than not seeing it
		// at all. The flag is real as of #214 — Session.beginEditor sets
		// it and the editor's cleanup clears it, both on this goroutine,
		// exactly as string_write and string_add do (modify.c:100-101,
		// :218-219).
		//
		// #134 had a stand-in here, reading StateEditing or StatePaging
		// off the client, and it is gone with the flag in place. Note
		// that it also excluded somebody *paging*, which the C does not:
		// page_string never changes STATE(d), so a reader halfway
		// through a listing is still CON_PLAYING and still gets the
		// line, interleaved with the page. That is the behaviour now.
		if rec.PlayerFlags.Has(game.PlayerWriting) {
			continue
		}
		if session.SyslogLevel(rec) < typ {
			continue
		}
		// CCGRN(i->character, C_NRM) ... "[ %s ]\r\n" ... CCNRM (utils.c:255-257).
		ch.TellAt(colour.Normal, "{{green}}[ %s ]{{/}}\r\n", message)
	}
}
