// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// max_bad_pws (config.c:236) and the two counters behind it
// (interpreter.c:1466-1474, :1511-1518).
//
// The pair is easy to conflate and the C keeps them deliberately apart:
// `d->bad_pws` is the socket's, resets on every fresh connection and is what
// max_bad_pws is compared against; GET_BAD_PWS is the character's, is saved
// to the pfile, and exists only so the next successful login can say how many
// attempts there were while they were away.

// savedCharacter puts a character on the roster without logging in as them,
// so a test about the password prompt does not have to walk creation first.
func savedCharacter(t *testing.T, srv *Server, name, password string) *game.PlayerRecord {
	t.Helper()
	rec := &game.PlayerRecord{Name: name, Class: game.ClassWarrior}
	game.InitChar(rec, testRNG(), false)
	cred, err := testAuth.NewCredential(password)
	if err != nil {
		t.Fatal(err)
	}
	rec.Credential = cred
	if err := srv.players.Save(context.Background(), rec); err != nil {
		t.Fatal(err)
	}
	return rec
}

// badPasswordsOnDisk reads the character's persistent tally back off the
// roster, which is the only place it lives between connections.
func badPasswordsOnDisk(t *testing.T, srv *Server, name string) int32 {
	t.Helper()
	rec, err := srv.players.Load(context.Background(), name)
	if err != nil {
		t.Fatalf("loading %s: %v", name, err)
	}
	return rec.BadPasswords
}

// TestBadPasswordsRepromptThenDisconnect: the first two are re-prompts, the
// third is the door. Before this the very first wrong password hung up, which
// is not what the C does and made a typo indistinguishable from an attack.
func TestBadPasswordsRepromptThenDisconnect(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	savedCharacter(t, srv, "Welmar", "swordfish")

	c := dialClient(t, addr)
	c.expect("By what name")
	c.send("Welmar")
	c.expect("Password:")

	// Two strikes: each says "Wrong password." and asks again. The prompt is
	// counted rather than merely matched — expect() would return at once on
	// the *first* "Password:", the one from before any of this.
	for i := 1; i <= 2; i++ {
		c.send("not-the-password")
		c.expectCount("Wrong password.", i)
		c.expectCount("Password:", i+1)
	}

	// Three, and the C says something different on the way out.
	c.send("still-not-the-password")
	got := c.expectEOF()
	if !strings.Contains(got, "Wrong password... disconnecting.") {
		t.Errorf("the third wrong password did not say so:\n%s", got)
	}
}

// TestBadPasswordsAreCountedPerConnection is the `d->bad_pws` half: the
// counter lives on the descriptor (structs.h:1019), so hanging up and dialling
// back in starts again from zero. Worth pinning, because storing this on the
// character instead would look like a stricter, better lockout and would in
// fact be a different game.
func TestBadPasswordsAreCountedPerConnection(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	savedCharacter(t, srv, "Welmar", "swordfish")

	burn := func() {
		c := dialClient(t, addr)
		c.expect("By what name")
		c.send("Welmar")
		c.expect("Password:")
		for i := 0; i < 3; i++ {
			c.send("wrong")
		}
		c.expectEOF()
		c.close()
	}
	burn()
	burn()

	// A fresh connection still gets its full three, and the right password
	// still works on it.
	c := dialClient(t, addr)
	c.expect("By what name")
	c.send("Welmar")
	c.expect("Password:")
	c.send("wrong")
	c.expectCount("Password:", 2)
	c.send("swordfish")
	c.expect("PRESS RETURN")
}

// TestLoginFailuresAreReportedOnTheNextLogin is the GET_BAD_PWS half
// (interpreter.c:1511-1518): the tally is persistent, survives the
// disconnect, is announced after the MOTD, and is cleared by being announced.
func TestLoginFailuresAreReportedOnTheNextLogin(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	savedCharacter(t, srv, "Welmar", "swordfish")

	attacker := dialClient(t, addr)
	attacker.expect("By what name")
	attacker.send("Welmar")
	attacker.expect("Password:")
	attacker.send("guess1")
	attacker.expectCount("Password:", 2)
	attacker.send("guess2")
	// Two, not three: the point here is the tally, not the disconnect, and
	// the third strike would close the socket before it could be checked.
	// The re-prompt is the barrier — RecordBadPassword's save happens before
	// the prompt is written (internal/session/login.go).
	attacker.expectCount("Password:", 3)
	attacker.close()

	if got := badPasswordsOnDisk(t, srv, "Welmar"); got != 2 {
		t.Fatalf("the persistent tally after two wrong passwords = %d, want 2", got)
	}

	c := dialClient(t, addr)
	c.expect("By what name")
	c.send("Welmar")
	c.expect("Password:")
	c.send("swordfish")
	got := c.expect("PRESS RETURN")
	if !strings.Contains(got, "2 LOGIN FAILURES SINCE LAST SUCCESSFUL LOGIN.") {
		t.Errorf("the successful login did not report the two failures:\n%s", got)
	}
	c.send("")
	c.menuEnter()

	// Announced is cleared: log out and back in, and there is nothing left
	// to report.
	c.send("quit")
	c.expect("Goodbye")
	c.close()
	if !eventually(5*time.Second, func() bool { return badPasswordsOnDisk(t, srv, "Welmar") == 0 }) {
		t.Errorf("the tally was still %d after being reported", badPasswordsOnDisk(t, srv, "Welmar"))
	}

	again := dialClient(t, addr)
	again.expect("By what name")
	again.send("Welmar")
	again.expect("Password:")
	again.send("swordfish")
	if got := again.expect("PRESS RETURN"); strings.Contains(got, "LOGIN FAILURE") {
		t.Errorf("a clean login was told about failures anyway:\n%s", got)
	}
}

// TestOneLoginFailureIsSingular: `(load_result > 1) ? "S" : ""`
// (interpreter.c:1515). A small thing, and the sort of small thing a rewrite
// silently drops.
func TestOneLoginFailureIsSingular(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	savedCharacter(t, srv, "Welmar", "swordfish")

	c := dialClient(t, addr)
	c.expect("By what name")
	c.send("Welmar")
	c.expect("Password:")
	c.send("wrong")
	c.expectCount("Password:", 2)
	c.send("swordfish")
	got := c.expect("PRESS RETURN")
	if !strings.Contains(got, "1 LOGIN FAILURE SINCE LAST SUCCESSFUL LOGIN.") {
		t.Errorf("one failure was not reported in the singular:\n%s", got)
	}
}

// TestAnEmptyPasswordHangsUpWithoutCounting: `if (!*arg) STATE(d) =
// CON_CLOSE;` (interpreter.c:1459-1460), checked before the password is and
// so before any strike is counted. It matters more now that a wrong password
// re-prompts: without it, empty lines would be a way to hold a connection at
// the password prompt indefinitely.
func TestAnEmptyPasswordHangsUpWithoutCounting(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	savedCharacter(t, srv, "Welmar", "swordfish")

	c := dialClient(t, addr)
	c.expect("By what name")
	c.send("Welmar")
	c.expect("Password:")
	c.send("")
	got := c.expectEOF()
	if strings.Contains(got, "Wrong password") {
		t.Errorf("an empty password was treated as a wrong one:\n%s", got)
	}
	if n := badPasswordsOnDisk(t, srv, "Welmar"); n != 0 {
		t.Errorf("an empty password counted as a strike: tally = %d, want 0", n)
	}
}

// TestMaxBadPwsIsTunable: the point of issue #135's second half — the
// constant is GameTuning.MaxBadPws, so an operator can make the door narrower.
func TestMaxBadPwsIsTunable(t *testing.T) {
	orig := game.Tuning()
	t.Cleanup(func() { game.SetTuning(orig) })
	tuning := orig
	tuning.MaxBadPws = 1
	game.SetTuning(tuning)

	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	savedCharacter(t, srv, "Welmar", "swordfish")

	c := dialClient(t, addr)
	c.expect("By what name")
	c.send("Welmar")
	c.expect("Password:")
	c.send("wrong")
	got := c.expectEOF()
	if !strings.Contains(got, "Wrong password... disconnecting.") {
		t.Errorf("max_bad_pws: 1 did not disconnect on the first attempt:\n%s", got)
	}
}

// TestABadPasswordDoesNotClobberALivePlayer.
//
// The C saves `d->character` here — the copy load_char left on the descriptor
// — so guessing at the password of somebody who is *already playing* writes
// their login-time snapshot over whatever they have done since. This port
// updates the on-disk record instead and never touches the live character;
// see docs/deviations.md.
func TestABadPasswordDoesNotClobberALivePlayer(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	// The first character on the roster is an Implementor; a second one is
	// an ordinary mortal, which is what this is about.
	dialClient(t, addr).create("Filler", "swordfish", "m", "w")
	victim := dialClient(t, addr)
	victim.create("Welmar", "swordfish2", "m", "w")

	// Something worth losing, recorded after the login that would be the
	// stale snapshot.
	victim.send("gold")
	victim.settle()
	var before int
	inWorld(t, srv, func(w *game.Live) {
		if c := w.Find("Welmar"); c != nil && c.Record != nil {
			c.Record.Points.Gold = 12345
			before = int(c.Record.Points.Gold)
		}
	})
	if before != 12345 {
		t.Fatal("the victim was not in the world to be robbed")
	}

	attacker := dialClient(t, addr)
	attacker.expect("By what name")
	attacker.send("Welmar")
	attacker.expect("Password:")
	attacker.send("wrong")
	attacker.expectCount("Password:", 2)
	attacker.close()

	// The live character still has it, and still has it after their own save.
	victim.send("save")
	victim.expect("Saving Welmar.")
	victim.settle()
	srv.WaitForWrites()

	rec, err := srv.players.Load(context.Background(), "Welmar")
	if err != nil {
		t.Fatal(err)
	}
	if rec.Points.Gold != 12345 {
		t.Errorf("the saved record's gold = %d, want 12345 — a failed login overwrote a live player", rec.Points.Gold)
	}
}
