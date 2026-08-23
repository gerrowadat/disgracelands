// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package session

import (
	"fmt"
	"strings"

	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/obs"
)

// ReportWriter is do_gen_write's file (act.other.c:867-924): the bug/idea/
// typo log. A session-local seam rather than internal/persist/reports.Store
// itself, the same reason BanKeeper/HouseKeeper are their own interfaces
// here instead of the persist package's — this layer does not import
// internal/persist.
type ReportWriter interface {
	// Write appends a report, reporting false when the destination is
	// full — max_filesize's own refusal (act.other.c:908-911), which the
	// C shows the player rather than logging as a failure.
	Write(kind, reporter string, room int32, body string) (bool, error)
}

// doGenWrite is do_gen_write (act.other.c:867-924) for one of the three
// subcommands it handles — bug, idea or typo, matching SCMD_BUG/SCMD_IDEA/
// SCMD_TYPO (interpreter.h:167-169) except spelled as the command name
// rather than the C's arbitrary numbers, since Go closes over which one
// this is instead of switching on it at call time.
func doGenWrite(kind string) func(*Context) error {
	return func(c *Context) error {
		// "Monsters can't have ideas - Go away." (act.other.c:892-895).
		// Every command here already runs for a logged-in player, but a
		// mobile has no way to reach this table at all yet — kept anyway,
		// both because the C's own refusal is part of what is being
		// ported and because Phase 4's special procedures may one day let
		// a scripted mobile call a command.
		if c.Character.IsNPC() {
			c.Send("Monsters can't have ideas - Go away.\r\n")
			return nil
		}

		// skip_spaces + delete_doubledollar (act.other.c:899-900): Arg is
		// already trimmed by the dispatcher, so only the $$ collapse is
		// left — the same idiom boards.go's headline uses, for the same
		// reason: this text may later pass through act().
		arg := strings.ReplaceAll(c.Arg, "$$", "$")
		if arg == "" {
			c.Send("That must be a mistake...\r\n")
			return nil
		}

		// mudlog(buf, CMP, LVL_IMMORT, FALSE) (act.other.c:904-905), before
		// the file-full check — the C logs the attempt even when the write
		// that follows gets refused. buf is `sprintf(buf, "%s %s: %s",
		// GET_NAME(ch), CMD_NAME, argument)` (act.other.c:903) and doubles
		// as both the log line and the exact text an online immortal sees
		// in-game (obs.WithWizVisEcho echoes a record's own message, the
		// same string mudlog's str serves both jobs from) — so the
		// message here is that format, not a generic "<kind> report".
		if c.Session != nil {
			c.Session.logger.Info(fmt.Sprintf("%s %s: %s", c.Character.Name, kind, arg),
				"character", c.Character.Name, "text", arg,
				obs.WizLevel(int(game.LevelImmortal)), obs.WizType(obs.LogComplete))
		}

		if c.Reports == nil {
			c.Send("Could not open the file.  Sorry.\r\n")
			return nil
		}
		ok, err := c.Reports.Write(kind, c.Character.Name, int32(c.Character.Room), arg) //nolint:gosec // room vnums are 32-bit in this format
		if err != nil {
			c.Send("Could not open the file.  Sorry.\r\n")
			return nil
		}
		if !ok {
			c.Send("Sorry, the file is full right now.. try again later.\r\n")
			return nil
		}
		c.Send("Okay.  Thanks!\r\n")
		return nil
	}
}
