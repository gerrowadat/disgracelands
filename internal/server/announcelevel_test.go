// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"strings"
	"testing"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// `announce`, which is a local addition and not a port: the C's
// send_to_all_color reaches everybody playing and not mid-edit, with no way
// to turn it down. See docs/deviations.md.
//
// The messages themselves, and the fidelity half of all this, are in
// announce_test.go.

// The default is All, and it has to be: every record ever written has these
// two bits clear, so anything else would have muted the whole roster the day
// this shipped. The bits count suppression for exactly that reason.
func TestAnnouncementsDefaultToAll(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	c.send("announce")
	c.expect("Your announcements are currently All.")

	// And the record says so with no bits set at all, which is the property
	// that matters for an archived pfile rather than a freshly made one.
	var pref game.Flags
	inWorld(t, srv, func(w *game.Live) {
		if who := w.Find("Zod"); who != nil && who.Record != nil {
			pref = who.Record.Preferences
		}
	})
	if pref.HasAny(game.PrefNoAnnounce1 | game.PrefNoAnnounce2) {
		t.Errorf("a new character has announcement-suppression bits set: %#x", pref)
	}
}

// Brief keeps the rare ones and drops the level gains, which is the whole
// point of having three settings rather than two: the level line is the only
// one that fires on an ordinary kill.
func TestAnnounceBriefDropsLevelsAndKeepsTheRest(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	god, mortal := twoInARoom(t, srv, addr)

	quiet := dialClient(t, addr)
	quiet.create("Quiet", "leavemealone", "m", "w")
	quiet.send("announce brief")
	quiet.expect("Your announcements are now Brief.")

	god.send("advance Bystander 5")
	god.expect("Okay.")
	mortal.settle()
	quiet.settle()

	if quiet.seen("has gained 4 levels!!!") {
		t.Errorf("Brief still heard a level gain:\n%s", quiet.transcript())
	}

	// The rare stream still arrives. A newcomer is the cheapest of the three
	// to provoke.
	mark := len(quiet.transcript())
	newcomer := dialClient(t, addr)
	newcomer.create("Fresh", "justarrived", "f", "w")
	quiet.settle()
	if !strings.Contains(quiet.transcript()[mark:], "All hail Fresh, a newcomer!") {
		t.Errorf("Brief lost the newcomer hail too:\n%s", quiet.transcript()[mark:])
	}
}

// Off drops both streams.
func TestAnnounceOffDropsEverything(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	god, mortal := twoInARoom(t, srv, addr)

	silent := dialClient(t, addr)
	silent.create("Silent", "nothanks", "m", "w")
	silent.send("announce off")
	silent.expect("Your announcements are now Off.")

	mark := len(silent.transcript())

	god.send("advance Bystander 5")
	god.expect("Okay.")
	newcomer := dialClient(t, addr)
	newcomer.create("Fresh", "justarrived", "f", "w")

	mortal.settle()
	silent.settle()

	after := silent.transcript()[mark:]
	for _, unwanted := range []string{"has gained", "a newcomer!"} {
		if strings.Contains(after, unwanted) {
			t.Errorf("Off still heard %q:\n%s", unwanted, after)
		}
	}
}

// Turning it back up restores both. The setting is a filter on delivery, not
// a subscription anybody has to re-establish.
func TestAnnounceAllRestoresTheStream(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	god, mortal := twoInARoom(t, srv, addr)

	quiet := dialClient(t, addr)
	quiet.create("Quiet", "leavemealone", "m", "w")
	quiet.send("announce off")
	quiet.expect("Your announcements are now Off.")
	quiet.send("announce all")
	quiet.expect("Your announcements are now All.")

	god.send("advance Bystander 2")
	god.expect("Okay.")
	mortal.settle()
	quiet.settle()

	if !quiet.seen("has gained a level!") {
		t.Errorf("All did not restore the level announcements:\n%s", quiet.transcript())
	}
}

// Prefix matching, the way `color` and `syslog` match theirs, and the usage
// line for a word that is none of them.
func TestAnnounceMatchesOnAPrefix(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	c.send("announce b")
	c.expect("Your announcements are now Brief.")
	c.send("announce o")
	c.expect("Your announcements are now Off.")
	c.send("announce a")
	c.expect("Your announcements are now All.")

	c.send("announce sideways")
	c.expect("Usage: announce { Off | Brief | All }")
}

// It survives a logout, which is the reason it is a preference bit and not a
// field on the session.
func TestTheAnnounceLevelIsSaved(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	first := dialClient(t, addr)
	first.create("Zod", "swordfish", "m", "w")

	c := dialClient(t, addr)
	c.create("Quiet", "leavemealone", "m", "w")
	c.send("announce brief")
	c.expect("Your announcements are now Brief.")
	c.send("quit")
	c.expectCount("Make your choice:", 2)
	c.close()

	back := dialClient(t, addr)
	back.login("Quiet", "leavemealone")
	back.send("announce")
	back.expect("Your announcements are currently Brief.")
}

// `toggle` lists it, because a setting `toggle` does not show is a setting
// nobody finds. A local row on a ported listing; docs/deviations.md records
// it.
func TestToggleListsTheAnnounceLevel(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	c.send("announce brief")
	c.expect("Your announcements are now Brief.")

	c.send("toggle")
	if got := c.expect("Announcements:"); !strings.Contains(got, "Announcements: Brief") {
		t.Errorf("toggle did not show the announce level:\n%s", got)
	}
}

// `an` is the only abbreviation this adds, and it must not have taken one.
// `a` is `alias` for a mortal (interpreter.c:226) and `au` is still the
// auction/autoexit pair it always was.
func TestAnnounceTookNobodyElsesAbbreviation(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	first := dialClient(t, addr)
	first.create("Zod", "swordfish", "m", "w")

	c := dialClient(t, addr)
	c.create("Mortal", "swordfish", "m", "w")

	c.send("a")
	c.expect("Currently defined aliases:")

	c.send("an")
	c.expect("Your announcements are currently All.")
}
