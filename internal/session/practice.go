// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package session

import (
	"sort"
	"strings"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// practice, ported from SPECIAL(guild) and list_skills (spec_procs.c).
//
// **This is a command here and a guildmaster's special procedure in the C.**
// Special procedures need the scripting seam (plan §8), and until that exists
// a character has no way to raise a skill at all — which means no way to cast
// anything, since do_cast refuses a spell at zero per cent. A command that
// works everywhere is a deviation; a game where nobody can learn a spell is
// not a game. Recorded in docs/deviations.md, and it moves back to the
// guildmasters when specprocs arrive.

func doPractice(c *Context) error {
	rec := c.Character.Record
	if c.Character.IsNPC() || rec == nil {
		return nil
	}

	if strings.TrimSpace(c.Arg) == "" {
		return c.listSkills()
	}

	if rec.SpellsToLearn <= 0 {
		c.Send("You do not seem to be able to practice now.\r\n")
		return nil
	}

	number, ok := game.SpellNumberByName(c.Arg)
	if !ok {
		c.Send("You do not know of that %s.\r\n", game.PracticeNoun(rec.Class))
		return nil
	}
	info, _ := game.Spell(number)

	// The C checks the plain class here while list_skills checks the remort
	// vector — so a remorted character can *see* a spell they cannot
	// practise. That inconsistency is the C's and is reproduced; see
	// docs/deviations.md.
	if rec.Level < game.MinLevelFor(info, rec.Class) {
		c.Send("You do not know of that %s.\r\n", game.PracticeNoun(rec.Class))
		return nil
	}

	learned := game.LearnedLevel(rec.Class)
	if rec.Skills[number] >= learned {
		c.Send("You are already learned in that area.\r\n")
		return nil
	}

	c.Send("You practice for a while...\r\n")
	rec.SpellsToLearn--

	if rec.Skills == nil {
		rec.Skills = map[int32]int32{}
	}
	rec.Skills[number] = min(learned, rec.Skills[number]+game.PracticeGain(rec))

	if rec.Skills[number] >= learned {
		c.Send("You are now learned in that area.\r\n")
	}
	return nil
}

// listSkills shows what a character knows, porting list_skills.
func (c *Context) listSkills() error {
	rec := c.Character.Record

	if rec.SpellsToLearn == 0 {
		c.Send("You have no practice sessions remaining.\r\n")
	} else {
		c.Send("You have %d practice session%s remaining.\r\n",
			rec.SpellsToLearn, plural(int(rec.SpellsToLearn)))
	}
	c.Send("You know of the following %ss:\r\n", game.PracticeNoun(rec.Class))

	// Sorted by name, as the C does with spell_sort_info.
	type entry struct {
		name    string
		percent int32
	}
	var entries []entry

	for number := int32(1); number <= game.MaxSkills; number++ {
		info, ok := game.Spell(number)
		if !ok {
			continue
		}
		// The remort-aware listing: anything any of their classes knows at
		// their level.
		if !game.KnowsSpell(rec, info) {
			continue
		}
		entries = append(entries, entry{info.Name, rec.Skills[number]})
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].name < entries[j].name })
	for _, e := range entries {
		c.Send("%-20s %s\r\n", e.name, game.HowGood(e.percent))
	}
	return nil
}
