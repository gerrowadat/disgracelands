// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package session

import (
	"strings"

	"github.com/gerrowadat/disgracelands/internal/colour"
	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/obs"
)

// remort and redeem, ported from do_remort (act.wizard.c:355) and
// do_wizutil's SCMD_REDEEM branch (act.wizard.c:2056). Both are `<DoC>` local
// additions and neither exists in stock CircleMUD.
//
// This is the mechanic the whole port has been carrying support for since
// Phase 3: the `IS_<CLASS>` macros consult a per-character bit vector rather
// than the class field, so a character who has *ever* been a thief practises,
// casts, wears and is treated as one for ever. `remort` is the only thing that
// sets a bit in that vector, and it takes an implementor to do it.
//
// Paladin is the end of the road rather than a step on it: it is the one class
// with no `IS_` macro reading its bit, and the only way into it.

// doRemort is do_remort (act.wizard.c:355).
//
// Three shapes: no argument at all, a name alone (report what they have), or a
// name and a class (grant it, or take it back with a leading `-`).
func doRemort(c *Context) error {
	name, class, _ := twoArguments(c.Arg)

	if name == "" {
		c.Send("Whom do you wish to remort?\r\n")
		return nil
	}

	victim := c.findAnywhere(name)
	if victim == nil {
		// The two messages differ by a full stop in the C — one dot for the
		// report form, two for the grant form. Reproduced; somebody put it
		// there and it is what a god reading their screen saw.
		if class == "" {
			c.Send("That is not a valid player name, or they are not logged in.\r\n")
		} else {
			c.Send("That is not a valid player name, or they are not logged in..\r\n")
		}
		return nil
	}

	if class == "" {
		c.Send("Player currently has access to skills/spells of:%s.\r\n",
			remortClassList(victim))
		return nil
	}

	// A leading `-` undoes a remort.
	undo := strings.HasPrefix(class, "-")
	which, ok := game.ParseShortClassName(strings.TrimPrefix(class, "-"))
	if !ok {
		c.Send("Invalid class.\r\n")
		return nil
	}

	rec := victim.Record
	if rec == nil {
		c.Send("You can't do that to a mob!\r\n")
		return nil
	}

	mask := game.RemortMask(which)
	vector := game.RemortFlagsOf(rec)
	// Captured before game.Remort moves it: the confirmation names the class
	// they were and the statistic pinned for it.
	oldClass := rec.Class

	// Refused if they *are* that class: there is nothing to do, and it is
	// the C's own first guard. Applies to an undo too — you cannot take away
	// the class somebody is currently walking around as.
	if which == rec.Class {
		short := game.ClassShortNames[which]
		c.Send("But %s is already a %s! To undo a remort, try 'remort %s -%s'. "+
			"Remember, you cannot undo a remort if the player is currently that class.\r\n",
			victim.Name, short, victim.Name, short)
		return nil
	}

	// The C's *second* guard — refuse if the bit is already set — is gone,
	// and #262 is why. It meant "they already have those abilities", which
	// was the whole of what remorting did; now that remorting also makes
	// them that class and starts them again at level one, having been a mage
	// before is no reason to refuse to make somebody a mage now. Undo still
	// needs the bit to be there, since there is otherwise nothing to clear.
	if undo && !vector.Has(mask) {
		short := game.ClassShortNames[which]
		c.Send("But %s has never been a %s.\r\n", victim.Name, short)
		return nil
	}

	// **Paladin is earned, not chosen: you must have been both a warrior and
	// a cleric.** That is the game's own rule, from the person who ran it
	// (#262), and the C never enforced it — `do_remort` has no such check,
	// so it was a rule the wizards kept by hand. Enforcing it is a
	// deviation, and it is in docs/deviations.md.
	//
	// "Has been" is the IS_<CLASS> question, not the class field: being a
	// warrior right now counts, and so does having been one and moved on,
	// which is exactly what those macros already answer.
	//
	// Remorting *away* from paladin is allowed, and nothing here stops it.
	// An earlier version of this refused it on the grounds that paladin had
	// no bit in the remort vector to remember it by. That was simply wrong —
	// paladin's mask is 16 (class.c:82) like every other class's, and what
	// it lacks is an IS_PALADIN macro reading it.
	if !undo && which == game.ClassPaladin {
		if !game.IsWarrior(rec) || !game.IsCleric(rec) {
			c.Send("%s has not earned paladinhood: it takes somebody who has been "+
				"both a warrior and a cleric.\r\n", victim.Name)
			return nil
		}
	}

	if undo {
		// Undo stays exactly what it was: a vector operation and nothing
		// else. It does not un-set a class, un-reset a level or hand back
		// hit points, because there is nothing recorded anywhere that says
		// what those were before — the character has been played since.
		// Deliberately the smaller half of #262.
		//
		// The C's XOR is kept. It is only reachable now when the bit is
		// set, since the guard above rejects the other case, so the "undo a
		// remort the character never had and it *grants* it" behaviour
		// docs/weirdnumbers.md records is no longer reachable through this
		// command. The entry says so.
		vector = vector ^ mask
		game.SetRemortFlags(rec, vector)
	} else {
		// Everything else: the class, the level, the body. game.Remort sets
		// the outgoing class's bit itself, which is what stops the class
		// change from taking away the abilities remorting exists to keep.
		game.Remort(rec, which, c.RNG)
	}

	short := game.ClassShortNames[which]
	if undo {
		// The C has no message here at all: the `snprintf` into buf2 is
		// guarded on `undo == 0` and the `send_to_char(buf2, ch)` after it is
		// not, so a god undoing a remort is sent whatever was last in that
		// buffer — which is the argument they just typed. A deviation, and
		// recorded as one.
		c.Send("%s is no longer a %s.\r\n", victim.Name, short)
		victim.Tell("You sink to the ground, aghast, as you feel your %shood slip away!\r\n", short)
	} else {
		c.Send("%s remorted to become a %s!\r\n", victim.Name, short)
		// The god gets told what the command did on their behalf, because
		// they used to type all of it themselves and will otherwise reach
		// for `set` out of habit (#262). Naming the pinned statistic
		// matters most: it is the least obvious line of the procedure this
		// replaces, and the one whose absence would be noticed last.
		c.Send("%s is now a level 1 %s: %d hit, %d mana, %d move, no practices banked, "+
			"and %s pinned at 18 for the %s they were.\r\n",
			victim.Name, short, rec.Points.MaxHit, rec.Points.MaxMana, rec.Points.MaxMove,
			game.PrimeAbilityName(oldClass), game.ClassShortNames[oldClass])
		victim.Tell("You fall to the ground, clutching your chest, as an unearthly force "+
			"bestows new knowledge and powers on you!\r\nYou gain the skills and privileges of a %s!\r\n",
			short)
		// And the player, who is about to look at a very different score
		// sheet and should know why rather than assume something broke.
		victim.Tell("Your old strength leaves you, and you begin again at level 1 — " +
			"but everything you have ever been, you still are.\r\n")
		// send_to_all_color (act.wizard.c:465): the whole game hears it, in
		// cyan, and anybody mid-edit does not. The comment said "in cyan" and
		// the call was the *uncoloured* broadcast — the fourth of the family
		// #212 is about, and the only one that was reaching players at all.
		//
		// Note the C's own unbalanced quoting, kept: the whisper opens with a
		// `'` and closes with a newline, never a matching one.
		c.broadcastAt(game.AnnouncementRare, colour.Normal,
			"{{cyan}}A voice whispers in your ear, 'All hail %s! Living again as a %s!{{/}}\r\n",
			victim.Name, short)
	}

	c.Send("This player now has access to skills/spells of:%s.\r\n", remortClassList(victim))
	c.saveVictim(victim)
	return nil
}

// remortClassList is the C's trailing loop, twice over: every class whose bit
// is set, in pc_class_snames order, each with a leading space.
//
// The leading space is why the caller's format string has no space before the
// list — "of:" runs straight into " mage cleric".
func remortClassList(victim *game.Character) string {
	if victim.Record == nil {
		return ""
	}
	vector := game.RemortFlagsOf(victim.Record)

	var b strings.Builder
	for _, class := range game.ClassShortNameOrder {
		mask := game.RemortMask(class)
		// Paladin appears here like any other class. It has a mask in the C's
		// table (16, class.c:82) and always has; what it lacks is an
		// IS_PALADIN macro reading it, which is a fact about how paladin
		// abilities are gated and not about whether the vector remembers.
		// An earlier comment here had that backwards.
		if mask != 0 && vector.Has(mask) {
			b.WriteString(" ")
			b.WriteString(game.ClassShortNames[class])
		}
	}
	return b.String()
}

// doRedeem is do_wizutil's SCMD_REDEEM (act.wizard.c:2056): lift a paladin's
// fallen state.
//
// A paladin whose alignment drops far enough is cast out and never casts
// again, whatever their alignment does afterwards — see game.PaladinFallen.
// This is the only way back, and it takes a greater god.
func doRedeem(c *Context) error {
	victim := c.wizutilTarget()
	if victim == nil {
		return nil
	}
	rec := victim.Record

	if !game.SpecFlagsOf(rec).Has(game.PaladinFallen) {
		c.Send("Your victim has not fallen!\r\n")
		return nil
	}

	game.SetSpecFlags(rec, game.SpecFlagsOf(rec).Clear(game.PaladinFallen))
	c.Send("Redeemed.\r\n")
	victim.Tell("You feel your paladinly powers restored! Rejoice! You live again in God's glory!\r\n")
	// mudlog(buf, BRF, MAX(LVL_GOD, GET_INVIS_LEV(ch)), TRUE)
	// (act.wizard.c:2065-2066) — a `<DoC>` addition that still follows
	// SCMD_PARDON's shape exactly, victim's name first.
	c.wizlogInvis(obs.LogBrief, game.LevelGod, c.Character,
		"(GC) %s redeemed by %s", victim.Name, c.Character.Name)
	c.saveVictim(victim)
	return nil
}
