// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import "testing"

// DisallowedName ports Valid_Name's xnames half (ban.c:255-286): a
// case-insensitive substring match against a loaded list.
func TestDisallowedName(t *testing.T) {
	s := &Server{names: []string{"fuck", "cunt"}}

	for _, tc := range []struct {
		name string
		want bool
	}{
		{"Zod", false},
		{"Fuckface", true}, // substring, mixed case
		{"CUNTFACE", true}, // wholly upper-case
		{"MotherFucker", true},
		{"Innocent", false},
	} {
		if got := s.DisallowedName(tc.name); got != tc.want {
			t.Errorf("DisallowedName(%q) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestDisallowedNameWithNoListDisallowsNothing(t *testing.T) {
	s := &Server{}
	if s.DisallowedName("Fuckface") {
		t.Error("a server with no xnames list disallowed a name anyway")
	}
}

// A name matching the xnames list is refused at the prompt, and the player
// is asked to try again — not disconnected, matching how every other
// invalidName reason behaves.
func TestXNamesRefusesACharacterNameAtCreation(t *testing.T) {
	srv, _ := newTestServer(t)
	srv.names = []string{"fuck"}

	c := dialClient(t, listening(t, srv))
	c.expect("By what name")
	c.send("Fuckface")
	c.expect("That name is not allowed.")
	c.expect("By what name")

	// The prompt is re-entered, not the connection closed: a clean name
	// still works afterwards.
	c.create("Zod", "password123", "m", "w")
	c.send("look")
	c.expect("> ")
}

// The reserved words (interpreter.c:580-591) are refused by name alone,
// with no server-side list involved — an exact match, not xnames' substring
// one.
func TestReservedWordsAreRefusedAsNames(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.expect("By what name")
	c.send("Someone")
	c.expect("That name is reserved.")
	c.expect("By what name")
}
