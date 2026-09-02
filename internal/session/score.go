// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package session

import (
	"fmt"
	"time"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// doScore shows a character their own numbers, porting do_score
// (act.informative.c).
//
// The line about armour class says "/10" out loud, which is the C admitting
// that armour is stored in tenths and compared in whole points — see
// docs/weirdnumbers.md.
func doScore(c *Context) error {
	rec := c.Character.Record
	if c.Character.IsNPC() || rec == nil {
		return nil
	}
	now := time.Now()

	// "  It's your birthday today." on the day itself, which is month zero and
	// day zero of the age rather than of the calendar — so it lands on the
	// anniversary of the character being rolled up, and a character made in
	// the first hour of a mud month has one every mud year.
	//
	// Two spaces before it, and the C appends the newline in each branch
	// rather than after the join. Found by the session-parity harness, which
	// created a character and read their score in the same minute.
	age := game.AgeOf(rec, now)
	if age.Month == 0 && age.Day == 0 {
		c.Send("You are %d years old.  It's your birthday today.\r\n", game.Age(rec, now))
	} else {
		c.Send("You are %d years old.\r\n", game.Age(rec, now))
	}
	c.Send("You have %d(%d) hit, %d(%d) mana and %d(%d) movement points.\r\n",
		rec.Points.Hit, rec.Points.MaxHit,
		rec.Points.Mana, rec.Points.MaxMana,
		rec.Points.Move, rec.Points.MaxMove)
	c.Send("Your armor class is %d/10, and your alignment is %d.\r\n",
		game.ComputeArmorClass(rec, combatantOf(c.Character)), rec.Alignment)
	c.Send("You have scored %d exp, and have %d gold coins.\r\n",
		rec.Points.Exp, rec.Points.Gold)

	if rec.Level < game.LevelImmortal {
		c.Send("You need %d exp to reach your next level.\r\n", game.ExpToLevel(rec))
	}

	// The C stops at hours, because real_time_passed fills a
	// time_info_data (utils.c:309, structs.h:745) and that struct has no
	// minutes field to fill — not because a shorter session was thought
	// not worth reporting. do_stat_character prints the same play time to
	// the minute (act.wizard.c:2247), so the minutes exist everywhere but
	// here; this line now shows them too. docs/deviations.md has it.
	played := rec.Played + now.Sub(rec.LastLogon)
	days := int(played.Hours()) / 24
	hours := int(played.Hours()) % 24
	minutes := int(played.Minutes()) % 60
	c.Send("You have been playing for %d day%s, %d hour%s and %d minute%s.\r\n",
		days, plural(days), hours, plural(hours), minutes, plural(minutes))

	c.Send("This ranks you as %s %s (level %d).\r\n", c.Character.Name, rec.Title, rec.Level)
	c.Send("%s", positionLine(c.Character))
	return nil
}

// positionLine is the sentence do_score ends on.
func positionLine(c *game.Character) string {
	switch c.Position {
	case game.PosDead:
		return "You are DEAD!\r\n"
	case game.PosMortallyWounded:
		return "You are mortally wounded!  You should seek help!\r\n"
	case game.PosIncapacitated:
		return "You are incapacitated, slowly fading away...\r\n"
	case game.PosStunned:
		return "You are stunned!  You can't move!\r\n"
	case game.PosSleeping:
		return "You are sleeping.\r\n"
	case game.PosResting:
		return "You are resting.\r\n"
	case game.PosSitting:
		return "You are sitting.\r\n"
	case game.PosFighting:
		if c.Fighting != nil {
			return "You are fighting " + c.Fighting.Name + ".\r\n"
		}
		return "You are fighting thin air.\r\n"
	}
	return "You are standing.\r\n"
}

// doExits lists the ways out, porting do_exits (act.informative.c:376).
//
// Three things here were wrong and are the C's now, all of them found by
// playing `exits` at both servers rather than by reading:
//
//   - **A closed exit is not listed at all.** The loop's condition is
//     `... && !EXIT_FLAGGED(EXIT(ch, door), EX_CLOSED)`, so a room whose only
//     way out is a shut door answers " None." This port listed "East - The
//     door is closed.", a line that appears nowhere in the C tree.
//   - **An immortal sees the destination's vnum**, `[%5d]`, which is most of
//     what makes the command useful while building.
//   - **Blindness is checked first**, with do_look's own wording rather than
//     a room lookup.
func doExits(c *Context) error {
	if isBlind(c.Character) {
		c.Send("You can't see a damned thing, you're blind!\r\n")
		return nil
	}
	room := c.World.Room(c.Character.Room)
	if room == nil {
		c.Send("You are nowhere at all. That should not be possible.\r\n")
		return nil
	}

	immortal := c.Character.Level() >= game.LevelImmortal

	var lines []string
	for dir := game.Direction(0); dir < game.NumDirections; dir++ {
		exit := room.Exits[dir]
		if exit == nil || exit.ToRoom == game.NoRoom || exit.State.Has(game.ExitClosed) {
			continue
		}

		var name string
		if destination := c.World.Room(exit.ToRoom); destination != nil {
			name = destination.Name
		}
		switch {
		case immortal:
			lines = append(lines, fmt.Sprintf("%-5s - [%5d] %s\r\n",
				capitaliseFirst(dir.String()), exit.ToRoom, name))
		case c.World.RoomIsDark(exit.ToRoom) && !game.CanSeeInDark(c.Character):
			// The darkness test is on the room being *looked into*, not the
			// one being stood in, so a lit room can list a dark one as
			// unknowable.
			lines = append(lines, fmt.Sprintf("%-5s - Too dark to tell\r\n",
				capitaliseFirst(dir.String())))
		default:
			lines = append(lines, fmt.Sprintf("%-5s - %s\r\n",
				capitaliseFirst(dir.String()), name))
		}
	}

	// The header goes out before the list is examined, so it appears even
	// when there is nothing under it.
	c.Send("Obvious exits:\r\n")
	if len(lines) == 0 {
		c.Send(" None.\r\n")
		return nil
	}
	for _, line := range lines {
		c.Send("%s", line)
	}
	return nil
}

// combatantOf adapts a character for the armour-class formula, which needs
// to know whether they are awake.
type scoreCombatant struct{ c *game.Character }

func (f scoreCombatant) IsNPC() bool             { return f.c.IsNPC() }
func (f scoreCombatant) Position() game.Position { return f.c.Position }
func (f scoreCombatant) Wielded() *game.Object   { return f.c.Equipment[game.WearWield] }
func (f scoreCombatant) Sanctuary() bool {
	return f.c.Record != nil && f.c.Record.AffectFlags.Has(game.AffectSanctuary)
}

func combatantOf(c *game.Character) game.Fighter { return scoreCombatant{c} }
