// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package session

import (
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

	c.Send("You are %d years old.\r\n", game.Age(rec, now))
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

	played := rec.Played + now.Sub(rec.LastLogon)
	days := int(played.Hours()) / 24
	hours := int(played.Hours()) % 24
	c.Send("You have been playing for %d day%s and %d hour%s.\r\n",
		days, plural(days), hours, plural(hours))

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

// doExits lists the ways out, porting do_exits.
func doExits(c *Context) error {
	room := c.World.Room(c.Character.Room)
	if room == nil {
		c.Send("You are nowhere at all. That should not be possible.\r\n")
		return nil
	}

	var any bool
	c.Send("Obvious exits:\r\n")
	for dir := game.Direction(0); dir < game.NumDirections; dir++ {
		exit := room.Exits[dir]
		if exit == nil || exit.ToRoom == game.NoRoom {
			continue
		}
		if exit.State.Has(game.ExitClosed) {
			// A closed door is listed by its name, not its destination —
			// which is how a player knows there is something to open.
			name := exit.Keywords
			if name == "" {
				name = "door"
			}
			c.Send("%-5s - The %s is closed.\r\n", capitaliseFirst(dir.String()), name)
			any = true
			continue
		}

		destination := c.World.Room(exit.ToRoom)
		name := "Too dark to tell"
		if destination != nil {
			name = destination.Name
		}
		c.Send("%-5s - %s\r\n", capitaliseFirst(dir.String()), name)
		any = true
	}

	if !any {
		c.Send(" None.\r\n")
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
