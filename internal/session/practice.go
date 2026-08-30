// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package session

import (
	"fmt"
	"sort"
	"strings"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// practice, ported from do_practice (act.other.c), SPECIAL(guild) and
// list_skills (spec_procs.c).
//
// The command itself only lists what you know. The *teaching* is the
// guildmaster's, in `specGuild` — which is where it was until this port put
// it here for want of special procedures, and where it is again.

func doPractice(c *Context) error {
	if c.Character.IsNPC() || c.Character.Record == nil {
		return nil
	}
	// A guildmaster in the room would have taken this command before it got
	// here. Reaching do_practice at all means there is nobody to teach you.
	if strings.TrimSpace(c.Arg) != "" {
		c.Send("You can only practice skills in your guild.\r\n")
		return nil
	}
	return c.listSkills()
}

// practise raises one skill, porting the body of SPECIAL(guild).
func (c *Context) practise(arg string) error {
	rec := c.Character.Record

	if rec.SpellsToLearn <= 0 {
		c.Send("You do not seem to be able to practice now.\r\n")
		return nil
	}

	number, ok := game.SpellNumberByName(arg)
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
		rec.Skills = map[game.SpellID]int32{}
	}
	rec.Skills[number] = min(learned, rec.Skills[number]+game.PracticeGain(rec))

	if rec.Skills[number] >= learned {
		c.Send("You are now learned in that area.\r\n")
	}
	return nil
}

// listSkills shows what a character knows, porting list_skills — which
// builds the whole listing into one buffer and pages it (spec_procs.c:193),
// not a line at a time.
func (c *Context) listSkills() error {
	rec := c.Character.Record

	var b strings.Builder
	if rec.SpellsToLearn == 0 {
		b.WriteString("You have no practice sessions remaining.\r\n")
	} else {
		fmt.Fprintf(&b, "You have %d practice session%s remaining.\r\n",
			rec.SpellsToLearn, plural(int(rec.SpellsToLearn)))
	}
	fmt.Fprintf(&b, "You know of the following %ss:\r\n", game.PracticeNoun(rec.Class))

	// Sorted by name, as the C does with spell_sort_info.
	type entry struct {
		name    string
		percent int32
	}
	var entries []entry

	for number := game.SpellID(1); number <= game.MaxSkills; number++ {
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
		fmt.Fprintf(&b, "%-20s %s\r\n", e.name, game.HowGood(e.percent))
	}
	c.SendPaged("%s", b.String())
	return nil
}
