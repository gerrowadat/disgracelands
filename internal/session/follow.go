// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package session

import (
	"fmt"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// Following and grouping: do_follow (act.movement.c:713), do_group,
// do_ungroup and print_group (act.other.c).
//
// The two are different things that look alike. Following is a relationship
// between two characters and is what makes somebody walk after you; being in
// a group is a *flag*, set by the leader on people who already follow them,
// and is what makes group spells and shared experience find you. You can
// follow without being grouped, which is what happens the moment you type
// `follow` — and being grouped without following is not possible, which is
// why leaving a group stops the following too.

// groupLevelSpread is the local rule in perform_group: more than seven levels
// apart and the two of you cannot group at all. Stock CircleMUD has no such
// limit, and the C carries this one between `<DoC>` markers.
const groupLevelSpread int32 = 7

// doFollow, porting do_follow.
func doFollow(c *Context) error {
	name, _ := oneArgument(c.Arg)
	if name == "" {
		c.Send("Whom do you wish to follow?\r\n")
		return nil
	}

	leader := c.World.FindInRoom(c.Character.Room, name)
	if leader == nil {
		c.Send("No-one by that name here.\r\n")
		return nil
	}

	if c.Character.Master == leader {
		c.Send("You are already following %s.\r\n", leader.Objective())
		return nil
	}

	// A charmed character does not get to choose. The C prints this and
	// nothing else, so a charmed follower cannot even stop following.
	if c.Character.Charmed() && c.Character.Master != nil {
		c.Send("But you only feel like following %s!\r\n", c.Character.Master.Name)
		return nil
	}

	// `follow self` is how you stop.
	if leader == c.Character {
		if c.Character.Master == nil {
			c.Send("You are already following yourself.\r\n")
			return nil
		}
		c.stopFollowing(c.Character)
		return nil
	}

	if game.CircleFollow(c.Character, leader) {
		c.Send("Sorry, but following in loops is not allowed.\r\n")
		return nil
	}
	if c.Character.Master != nil {
		c.stopFollowing(c.Character)
	}

	// Following somebody new drops you out of your old group, but does not
	// put you in theirs: the leader has to enrol you.
	c.Character.SetGrouped(false)
	c.addFollower(c.Character, leader)
	return nil
}

// addFollower attaches a follower and tells all three parties, porting the
// message half of add_follower.
func (c *Context) addFollower(follower, leader *game.Character) {
	c.World.AddFollower(follower, leader)

	follower.Tell("You now follow %s.\r\n", leader.Name)
	leader.Tell("%s starts following you.\r\n", follower.Name)
	for _, other := range c.World.Occupants(follower.Room) {
		if other != follower && other != leader {
			other.Tell("%s starts to follow %s.\r\n", follower.Name, leader.Name)
		}
	}
}

// stopFollowing detaches a follower and tells all three parties, porting the
// message half of stop_follower.
//
// A charmed follower gets an entirely different set of messages, and they are
// the best lines in the game.
func (c *Context) stopFollowing(follower *game.Character) {
	leader := follower.Master
	if leader == nil {
		return
	}

	charmed := follower.Charmed()
	toChar := fmt.Sprintf("You stop following %s.\r\n", leader.Name)
	toLeader := fmt.Sprintf("%s stops following you.\r\n", follower.Name)
	toRoom := fmt.Sprintf("%s stops following %s.\r\n", follower.Name, leader.Name)
	if charmed {
		toChar = fmt.Sprintf("You realize that %s is a jerk!\r\n", leader.Name)
		toLeader = fmt.Sprintf("%s hates your guts!\r\n", follower.Name)
		toRoom = fmt.Sprintf("%s realizes that %s is a jerk!\r\n", follower.Name, leader.Name)
	}

	follower.Tell("%s", toChar)
	leader.Tell("%s", toLeader)
	for _, other := range c.World.Occupants(follower.Room) {
		if other != follower && other != leader {
			other.Tell("%s", toRoom)
		}
	}

	c.World.StopFollowing(follower)
}

// doGroup, porting do_group. With no argument it lists the group; with one it
// enrols or expels somebody.
func doGroup(c *Context) error {
	name, _ := oneArgument(c.Arg)
	if name == "" {
		c.printGroup()
		return nil
	}

	if c.Character.Master != nil {
		c.Send("You can not enroll group members without being head of a group.\r\n")
		return nil
	}

	if name == "all" {
		// The leader joins their own group first, so that `group all` works
		// on somebody who has not grouped themselves yet.
		c.performGroup(c.Character)

		var found bool
		for _, f := range append([]*game.Character(nil), c.Character.Followers...) {
			if c.performGroup(f) {
				found = true
			}
		}
		if !found {
			c.Send("Everyone following you is already in your group.\r\n")
		}
		return nil
	}

	victim := c.World.FindInRoom(c.Character.Room, name)
	switch {
	case victim == nil:
		c.Send("No-one by that name here.\r\n")
	case victim.Master != c.Character && victim != c.Character:
		c.Send("%s must follow you to enter your group.\r\n", victim.Name)
	case !victim.Grouped():
		c.performGroup(victim)
	default:
		// Already in: naming them again throws them out.
		if victim != c.Character {
			c.Send("%s is no longer a member of your group.\r\n", victim.Name)
		}
		victim.Tell("You have been kicked out of %s's group!\r\n", c.Character.Name)
		for _, other := range c.World.Occupants(c.Character.Room) {
			if other != c.Character && other != victim {
				other.Tell("%s has been kicked out of %s's group!\r\n",
					victim.Name, c.Character.Name)
			}
		}
		victim.SetGrouped(false)
	}
	return nil
}

// performGroup enrols one person, porting perform_group.
//
// It returns true when it had something to say, which is not the same as
// having enrolled anybody: the two level refusals return 1 as well, so
// `group all` counts a refusal as progress and does not print "everyone is
// already in your group". That is the C's arithmetic.
func (c *Context) performGroup(victim *game.Character) bool {
	if victim.Grouped() {
		return false
	}

	// The local level rule, and it reads in both directions.
	switch {
	case c.Character.Level() > victim.Level()+groupLevelSpread:
		c.Send("%s cannot hope to keep up with you!\r\n", victim.Name)
		victim.Tell("You simply cannot keep up with %s's group.\r\n", c.Character.Name)
		return true
	case c.Character.Level() < victim.Level()-groupLevelSpread:
		c.Send("%s is far too tough to group with little ol' you.\r\n", victim.Name)
		victim.Tell("%s? C'mon, you can do better than that.\r\n", c.Character.Name)
		return true
	}

	victim.SetGrouped(true)
	if c.Character != victim {
		c.Send("%s is now a member of your group.\r\n", victim.Name)
	}
	victim.Tell("You are now a member of %s's group.\r\n", c.Character.Name)
	for _, other := range c.World.Occupants(c.Character.Room) {
		if other != c.Character && other != victim {
			other.Tell("%s is now a member of %s's group.\r\n", victim.Name, c.Character.Name)
		}
	}
	return true
}

// printGroup lists the group, porting print_group.
func (c *Context) printGroup() {
	if !c.Character.Grouped() {
		c.Send("But you are not the member of a group!\r\n")
		return
	}
	c.Send("Your group consists of:\r\n")

	leader := c.Character.GroupLeader()
	line := func(who *game.Character, suffix string) {
		rec := who.Record
		if rec == nil {
			return
		}
		c.Send("     [%3dH %3dM %3dV] [%2d %s] %s%s\r\n",
			rec.Points.Hit, rec.Points.Mana, rec.Points.Move,
			rec.Level, game.ClassAbbrevs[rec.Class], who.Name, suffix)
	}

	if leader.Grouped() {
		line(leader, " (Head of group)")
	}
	for _, f := range leader.Followers {
		if f.Grouped() {
			line(f, "")
		}
	}
}

// doUngroup, porting do_ungroup: with no argument it disbands, with one it
// expels.
//
// Expelling somebody also stops them following, unless they are charmed — a
// charmed follower has no say in the matter and stays.
func doUngroup(c *Context) error {
	name, _ := oneArgument(c.Arg)

	if name == "" {
		if c.Character.Master != nil || !c.Character.Grouped() {
			c.Send("But you lead no group!\r\n")
			return nil
		}
		for _, f := range append([]*game.Character(nil), c.Character.Followers...) {
			if !f.Grouped() {
				continue
			}
			f.SetGrouped(false)
			f.Tell("%s has disbanded the group.\r\n", c.Character.Name)
			if !f.Charmed() {
				c.stopFollowing(f)
			}
		}
		c.Character.SetGrouped(false)
		c.Send("You disband the group.\r\n")
		return nil
	}

	victim := c.World.FindInRoom(c.Character.Room, name)
	switch {
	case victim == nil:
		c.Send("There is no such person!\r\n")
		return nil
	case victim.Master != c.Character:
		c.Send("That person is not following you!\r\n")
		return nil
	case !victim.Grouped():
		c.Send("That person isn't in your group.\r\n")
		return nil
	}

	victim.SetGrouped(false)
	c.Send("%s is no longer a member of your group.\r\n", victim.Name)
	victim.Tell("You have been kicked out of %s's group!\r\n", c.Character.Name)
	for _, other := range c.World.Occupants(c.Character.Room) {
		if other != c.Character && other != victim {
			other.Tell("%s has been kicked out of %s's group!\r\n",
				victim.Name, c.Character.Name)
		}
	}
	if !victim.Charmed() {
		c.stopFollowing(victim)
	}
	return nil
}

// moveFollowers walks everyone who was following a character out of the room
// they were both in, porting the second half of perform_move.
//
// Only followers who were in the room they left and who are on their feet
// come along, so somebody who was sleeping or fighting stays behind.
func (c *Context) moveFollowers(leader *game.Character, from game.RoomVnum, dir game.Direction) {
	for _, f := range append([]*game.Character(nil), leader.Followers...) {
		if f.Room != from || f.Position < game.PosStanding {
			continue
		}
		f.Tell("You follow %s.\r\n", leader.Name)
		c.moveCharacter(f, dir)
	}
}
