// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"testing"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// TestSkillsetSetsASkill, quotes and all. The quoting is the command's whole
// awkwardness and it is deliberate: skill names have spaces in them.
func TestSkillsetSetsASkill(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	god, _ := twoInARoom(t, srv, addr)

	god.send("skillset Bystander 'magic missile' 75")
	god.expect("You change Bystander's magic missile to 75.")

	var learned int32
	inWorld(t, srv, func(w *game.Live) {
		learned = w.Find("Bystander").Record.Skills[game.SpellMagicMissile]
	})
	if learned != 75 {
		t.Errorf("the skill is %d, want 75", learned)
	}
}

// TestSkillsetComplaints covers each refusal, including the two range ones and
// the missing quotes.
func TestSkillsetComplaints(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	god, _ := twoInARoom(t, srv, addr)

	for _, tc := range []struct{ command, expect string }{
		{"skillset", "Syntax: skillset <name> '<skill>' <value>"},
		{"skillset nobody 'sneak' 50", "No-one by that name here."},
		{"skillset Bystander", "Skill name expected."},
		{"skillset Bystander sneak 50", "Skill must be enclosed in: ''"},
		{"skillset Bystander 'sneak", "Skill must be enclosed in: ''"},
		{"skillset Bystander 'nonsense' 50", "Unrecognized skill."},
		{"skillset Bystander 'sneak'", "Learned value expected."},
		{"skillset Bystander 'sneak' -1", "Minimum value for learned is 0."},
		{"skillset Bystander 'sneak' 101", "Max value for learned is 100."},
	} {
		god.send(tc.command)
		god.expect(tc.expect)
	}
}

// TestSkillsetListsTheSkills when given no argument at all.
func TestSkillsetListsTheSkills(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	c.send("skillset")
	c.expect("Skill being one of the following:")
	c.expect("magic missile")
}

// TestTrackThroughDoorsIsPerWorld. The setting is on Live rather than in a
// package variable, so two servers in one test run cannot fight over it — and
// the toggle reports which way it went.
func TestTrackThroughDoorsIsPerWorld(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	var on bool
	inWorld(t, srv, func(w *game.Live) { on = w.TrackThroughDoors() })
	if !on {
		t.Error("a fresh world should track through doors, as this server did")
	}

	c.send("trackthru")
	c.expect("Will no longer track through doors.")
	inWorld(t, srv, func(w *game.Live) { on = w.TrackThroughDoors() })
	if on {
		t.Error("the toggle did not take")
	}

	c.send("trackthru")
	c.expect("Will now track through doors.")
}
