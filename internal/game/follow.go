// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

// Following, ported from add_follower, stop_follower, die_follower and
// circle_follow (utils.c:376).
//
// One relationship carries a surprising amount of the game: a group is a
// leader's follower list filtered by a flag, a charmed mobile is a follower
// with AFF_CHARM on it, and a summoned zombie is a mobile added to the list
// at birth. Group spells, charm, summons and walking somewhere with a friend
// are all the same two fields.
//
// The C keeps them as `ch->master` and a hand-rolled linked list of
// `follow_type`, and every path that removes a character from the world has
// to remember to unpick both ends. Forgetting is how you get a follower list
// pointing at somebody who was extracted three rooms ago.

// FollowResult is what happened when somebody tried to follow somebody.
type FollowResult int

const (
	// FollowOK: they are now following.
	FollowOK FollowResult = iota
	// FollowCircular: it would make a loop.
	FollowCircular
	// FollowAlready: they already follow that person.
	FollowAlready
	// FollowCharmed: they are charmed and cannot choose.
	FollowCharmed
	// FollowSelfNotFollowing: they asked to follow themselves and were not
	// following anybody.
	FollowSelfNotFollowing
	// FollowStopped: they asked to follow themselves and have stopped
	// following whoever they were.
	FollowStopped
)

// CircleFollow reports whether making c follow leader would make a loop,
// porting circle_follow.
//
// It walks up from the prospective leader looking for the follower. Without
// it, two characters following each other make a cycle that hangs the next
// thing to walk the chain — which in the C is the movement code, so the
// server locks up the moment either of them takes a step.
func CircleFollow(c, leader *Character) bool {
	for above := leader; above != nil; above = above.Master {
		if above == c {
			return true
		}
	}
	return false
}

// AddFollower makes c follow leader, porting add_follower.
//
// The C's comment is a warning: "Do NOT call this before having checked if a
// circle of followers will arise." It does not check, and neither does this —
// the callers do, because charm and follow report the failure differently.
func (l *Live) AddFollower(c, leader *Character) {
	if c == nil || leader == nil || c.Master != nil {
		return
	}
	c.Master = leader
	// The C pushes onto the head of the list, so followers are listed in the
	// reverse of the order they joined. `group` shows them that way.
	leader.Followers = append([]*Character{c}, leader.Followers...)
}

// StopFollowing detaches c from whoever they were following, porting
// stop_follower's bookkeeping.
//
// The messages are the caller's, because the C's differ entirely depending on
// whether the follower was charmed — one of them is "You realize that $N is a
// jerk!" — and both need somebody to say them to. What is here is the part
// that must not be got wrong: both ends of the link, and the two flags that
// go with it.
func (l *Live) StopFollowing(c *Character) {
	if c == nil || c.Master == nil {
		return
	}

	leader := c.Master
	kept := leader.Followers[:0]
	for _, f := range leader.Followers {
		if f != c {
			kept = append(kept, f)
		}
	}
	leader.Followers = kept
	c.Master = nil

	// Charm is an affect and comes off with the affect; group is a bare flag.
	if c.Record != nil {
		RemoveAffectsOf(c.Record, SpellCharm)
		c.Record.BaseAffectFlags = c.Record.BaseAffectFlags.Clear(AffectCharm | AffectGroup)
		RecomputeAffects(c.Record)
		c.Record.AffectFlags = c.Record.AffectFlags.Clear(AffectCharm | AffectGroup)
	}
}

// DieFollower unpicks every following relationship a character is part of,
// porting die_follower. Called when they leave the world by any route.
func (l *Live) DieFollower(c *Character) []*Character {
	if c == nil {
		return nil
	}

	var orphaned []*Character
	if c.Master != nil {
		l.StopFollowing(c)
	}
	// A snapshot: StopFollowing edits the list it is walking.
	for _, f := range append([]*Character(nil), c.Followers...) {
		orphaned = append(orphaned, f)
		l.StopFollowing(f)
	}
	return orphaned
}

// Charmed reports whether a character is under AFF_CHARM.
func (c *Character) Charmed() bool {
	return c != nil && c.Record != nil && c.Record.AffectFlags.Has(AffectCharm)
}

// Grouped reports whether a character is in a group.
func (c *Character) Grouped() bool {
	return c != nil && c.Record != nil && c.Record.AffectFlags.Has(AffectGroup)
}

// SetGrouped adds or removes the group flag. It is a bare flag rather than an
// affect — nothing expires it, and it survives everything except leaving the
// group.
func (c *Character) SetGrouped(in bool) {
	if c == nil || c.Record == nil {
		return
	}
	if in {
		c.Record.BaseAffectFlags = c.Record.BaseAffectFlags.Set(AffectGroup)
		c.Record.AffectFlags = c.Record.AffectFlags.Set(AffectGroup)
		return
	}
	c.Record.BaseAffectFlags = c.Record.BaseAffectFlags.Clear(AffectGroup)
	c.Record.AffectFlags = c.Record.AffectFlags.Clear(AffectGroup)
}

// GroupLeader is whoever heads a character's group: their master, or
// themselves. The C computes this inline in three places.
func (c *Character) GroupLeader() *Character {
	if c == nil {
		return nil
	}
	if c.Master != nil {
		return c.Master
	}
	return c
}

// GroupMembers lists everyone in a character's group who is in a given room,
// in the order the C's mag_groups visits them: the leader's followers first,
// then the leader, then the caster.
//
// The caster comes last deliberately — a group heal heals everybody else
// before it heals you, which matters when the spell can kill the caster.
func (c *Character) GroupMembers(room RoomVnum) []*Character {
	if c == nil || !c.Grouped() {
		return nil
	}

	leader := c.GroupLeader()
	var out []*Character

	for _, f := range leader.Followers {
		if f.Room != room || !f.Grouped() || f == c {
			continue
		}
		out = append(out, f)
	}
	if leader != c && leader.Grouped() {
		out = append(out, leader)
	}
	return append(out, c)
}

// NumFollowersCharmed counts a character's charmed followers, porting
// num_followers_charmed. Nothing uses it yet; it is the check the C makes
// before letting somebody charm one more.
func NumFollowersCharmed(c *Character) int {
	var total int
	for _, f := range c.Followers {
		if f.Charmed() && f.Master == c {
			total++
		}
	}
	return total
}
