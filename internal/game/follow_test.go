// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import "testing"

// followWorld puts three characters in one room.
func followWorld(t *testing.T) (*Live, *Character, *Character, *Character) {
	t.Helper()

	l := objectWorld()
	a, b, c := newCharacter("Ann"), newCharacter("Bob"), newCharacter("Cid")
	for _, who := range []*Character{a, b, c} {
		if err := l.Enter(who, 3001); err != nil {
			t.Fatal(err)
		}
	}
	return l, a, b, c
}

// TestCircleFollowCatchesEveryLoopLength. A cycle in the follower chain hangs
// the next thing to walk it, and the walker is the movement code — so the
// server locks up the moment anybody in the loop takes a step.
func TestCircleFollowCatchesEveryLoopLength(t *testing.T) {
	l, ann, bob, cid := followWorld(t)

	// Direct: Ann follows Bob, so Bob may not follow Ann.
	l.AddFollower(ann, bob)
	if !CircleFollow(bob, ann) {
		t.Error("a two-character loop was not caught")
	}
	if CircleFollow(cid, bob) {
		t.Error("Cid following Bob is not a loop")
	}

	// Indirect: Bob follows Cid, so Cid may not follow Ann.
	l.AddFollower(bob, cid)
	if !CircleFollow(cid, ann) {
		t.Error("a three-character loop was not caught")
	}
}

// TestStopFollowingUnpicksBothEnds, which is the bookkeeping the C gets wrong
// often enough to have a comment about it.
func TestStopFollowingUnpicksBothEnds(t *testing.T) {
	l, ann, bob, cid := followWorld(t)

	l.AddFollower(ann, cid)
	l.AddFollower(bob, cid)
	if len(cid.Followers) != 2 {
		t.Fatalf("Cid has %d followers, want 2", len(cid.Followers))
	}

	l.StopFollowing(ann)
	if ann.Master != nil {
		t.Error("Ann still has a master")
	}
	if len(cid.Followers) != 1 || cid.Followers[0] != bob {
		t.Errorf("Cid's followers are %v, want just Bob", cid.Followers)
	}

	// And the group flag comes off with it.
	bob.SetGrouped(true)
	l.StopFollowing(bob)
	if bob.Grouped() {
		t.Error("leaving the leader left the group flag on")
	}
}

// TestDyingUnpicksEverything, in both directions at once.
func TestDyingUnpicksEverything(t *testing.T) {
	l, ann, bob, cid := followWorld(t)

	// Ann follows Bob, Cid follows Ann. Ann dies.
	l.AddFollower(ann, bob)
	l.AddFollower(cid, ann)

	orphaned := l.DieFollower(ann)
	if len(orphaned) != 1 || orphaned[0] != cid {
		t.Errorf("orphaned %v, want just Cid", orphaned)
	}
	if ann.Master != nil || len(ann.Followers) != 0 {
		t.Error("the dead character is still attached to somebody")
	}
	if len(bob.Followers) != 0 {
		t.Errorf("Bob still lists a dead follower: %v", bob.Followers)
	}
	if cid.Master != nil {
		t.Error("Cid still follows a dead character")
	}
}

// TestGroupMembersPutsTheCasterLast, because a group spell can move or kill
// everybody and the caster has to be the last one it happens to.
func TestGroupMembersPutsTheCasterLast(t *testing.T) {
	l, ann, bob, cid := followWorld(t)

	l.AddFollower(bob, ann)
	l.AddFollower(cid, ann)
	for _, who := range []*Character{ann, bob, cid} {
		who.SetGrouped(true)
	}

	members := ann.GroupMembers(ann.Room)
	if len(members) != 3 {
		t.Fatalf("%d members, want 3", len(members))
	}
	if members[len(members)-1] != ann {
		t.Errorf("the caster is not last: %v", members)
	}

	// Somebody in another room is not in the group for this purpose.
	if err := l.Enter(cid, 3002); err != nil {
		t.Fatal(err)
	}
	if got := len(ann.GroupMembers(ann.Room)); got != 2 {
		t.Errorf("%d members after Cid left the room, want 2", got)
	}

	// A follower who is not grouped is not counted either.
	bob.SetGrouped(false)
	if got := len(ann.GroupMembers(ann.Room)); got != 1 {
		t.Errorf("%d members with only the leader grouped, want 1", got)
	}
}

// TestGroupShareRoundsUp. `(exp/3 + members - 1) / members` is integer
// division with a rounding trick in front of it, and the effect is that a
// group earns slightly *more* in total than one person would. That is the
// incentive to group, and it looks accidental until you notice it is not.
func TestGroupShareRoundsUp(t *testing.T) {
	victim := &PlayerRecord{Points: Points{Exp: 30}} // ten to share

	// The `+ members - 1` makes it ceiling division exactly.
	for members, want := range map[int32]int32{
		1: 10, // 10 / 1
		2: 5,  // ceil(10 / 2)
		3: 4,  // ceil(10 / 3), so three people take twelve from a ten-point kill
		4: 3,  // ceil(10 / 4)
	} {
		if got := GroupShare(victim, true, members); got != want {
			t.Errorf("%d members share %d, want %d each", members, got, want)
		}
	}

	// Six people splitting ten points get two each — twelve points from a ten
	// point kill.
	if got := GroupShare(victim, true, 6); got != 2 {
		t.Errorf("six members get %d each, want 2", got)
	}

	// Never less than one, however many there are.
	if got := GroupShare(&PlayerRecord{}, true, 20); got != 1 {
		t.Errorf("a worthless kill pays %d, want the floor of 1", got)
	}
}

// TestGroupShareMessageWordsItThreeWays.
func TestGroupShareMessageWordsItThreeWays(t *testing.T) {
	member := &PlayerRecord{Class: ClassWarrior, Level: 10}

	for share, want := range map[int32]string{
		0: "You receive your share of experience -- Nothing! Ha!!\r\n",
		1: "You receive your share of experience -- one measly little point!\r\n",
	} {
		if got := GroupShareMessage(member, share); got != want {
			t.Errorf("a share of %d says %q, want %q", share, got, want)
		}
	}

	if got := GroupShareMessage(member, 50); got != "You receive your share of experience -- 50 points.\r\n" {
		t.Errorf("a share of 50 says %q", got)
	}
}
