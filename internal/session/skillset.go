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
)

// do_skillset (modify.c:259): set one skill on one character to one number.
//
// The quoting is the command's whole awkwardness and it is deliberate — skill
// names have spaces in them, so `skillset bob 'magic missile' 100` needs the
// quotes to know where the name ends. The C lowercases the characters between
// them *in place* while it looks for the closing quote, which is how the
// lookup gets a lowercase name without a second pass.
func doSkillset(c *Context) error {
	name, rest := halfChop(c.Arg)

	// No argument at all prints the list of skills, four to a line.
	if name == "" {
		c.Send("Syntax: skillset <name> '<skill>' <value>\r\n")
		c.Send("%s", skillList())
		return nil
	}

	victim := c.findAnywhere(name)
	if victim == nil {
		c.Send("No-one by that name here.\r\n")
		return nil
	}

	rest = strings.TrimSpace(rest)
	if rest == "" {
		c.Send("Skill name expected.\r\n")
		return nil
	}
	if !strings.HasPrefix(rest, "'") {
		c.Send("Skill must be enclosed in: ''\r\n")
		return nil
	}
	end := strings.Index(rest[1:], "'")
	if end < 0 {
		c.Send("Skill must be enclosed in: ''\r\n")
		return nil
	}
	skillName := strings.ToLower(rest[1 : 1+end])
	rest = strings.TrimSpace(rest[end+2:])

	skill, ok := game.SpellNumberByName(skillName)
	if !ok || skill <= 0 {
		c.Send("Unrecognized skill.\r\n")
		return nil
	}

	valueWord, _ := oneArgument(rest)
	if valueWord == "" {
		c.Send("Learned value expected.\r\n")
		return nil
	}
	// atoi, so anything unparseable is zero rather than an error — which is
	// why "Minimum value for learned is 0." is unreachable for a word and
	// reachable only for a negative number.
	value := atoiC(valueWord)
	switch {
	case value < 0:
		c.Send("Minimum value for learned is 0.\r\n")
		return nil
	case value > 100:
		c.Send("Max value for learned is 100.\r\n")
		return nil
	}

	// The NPC check comes *after* the range checks in the C, so `skillset
	// somemob 'sneak' 200` says the value is too high rather than that it is a
	// mobile. Order reproduced.
	if victim.IsNPC() || victim.Record == nil {
		c.Send("You can't set NPC skills.\r\n")
		return nil
	}

	if victim.Record.Skills == nil {
		victim.Record.Skills = map[int32]int32{}
	}
	victim.Record.Skills[skill] = value

	c.Send("You change %s's %s to %d.\r\n", victim.Name, game.SpellName(skill), value)
	c.saveVictim(victim)
	return nil
}

// skillList is the C's help text: every named skill, eighteen columns wide and
// four to a line.
func skillList() string {
	var b strings.Builder
	b.WriteString("Skill being one of the following:\r\n")

	shown := 0
	for i := int32(1); i <= game.MaxSkills; i++ {
		name := game.SpellName(i)
		if name == "!UNUSED!" {
			continue
		}
		fmt.Fprintf(&b, "%18s", name)
		shown++
		if shown%4 == 0 {
			b.WriteString("\r\n")
		}
	}
	if shown%4 != 0 {
		b.WriteString("\r\n")
	}
	b.WriteString("\r\n")
	return b.String()
}

// atoiC is C's atoi: leading sign, then digits, stopping at the first thing
// that is not one, and zero for anything it cannot read at all.
func atoiC(s string) int32 {
	s = strings.TrimSpace(s)
	sign := int32(1)
	if strings.HasPrefix(s, "-") {
		sign, s = -1, s[1:]
	} else if strings.HasPrefix(s, "+") {
		s = s[1:]
	}
	var n int32
	for i := 0; i < len(s) && s[i] >= '0' && s[i] <= '9'; i++ {
		n = n*10 + int32(s[i]-'0')
		if n > 1_000_000 {
			return sign * 1_000_000
		}
	}
	return sign * n
}
