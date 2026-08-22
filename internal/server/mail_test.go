// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/persist/mail"
	mailnative "github.com/gerrowadat/disgracelands/internal/persist/mail/native"
	"github.com/gerrowadat/disgracelands/internal/persist/player"
	"github.com/gerrowadat/disgracelands/internal/persist/player/ascii"
	"github.com/gerrowadat/disgracelands/internal/persist/player/binary"
)

// The post office, end to end.

// withPostmaster puts a postmaster in the character's room.
func withPostmaster(t *testing.T, srv *Server, name string) {
	t.Helper()
	inWorld(t, srv, func(w *game.Live) {
		who := w.Find(name)
		if who == nil {
			t.Error("the character is not in the world")
			return
		}
		def := w.MobileDef(testShopkeeperVnum)
		if def == nil {
			t.Error("no mobile to make a postmaster of")
			return
		}
		def.Spec = "postmaster"
		if w.SpawnMobile(testShopkeeperVnum, who.Room, srv.rng) == nil {
			t.Error("could not put a postmaster in the room")
		}
	})
}

func setGold(t *testing.T, srv *Server, name string, gold int32) {
	t.Helper()
	inWorld(t, srv, func(w *game.Live) {
		if who := w.Find(name); who != nil && who.Record != nil {
			who.Record.Points.Gold = gold
		}
	})
}

func idOf(t *testing.T, srv *Server, name string) int64 {
	t.Helper()
	var id int64 = -1
	inWorld(t, srv, func(w *game.Live) {
		if who := w.Find(name); who != nil && who.Record != nil {
			id = who.Record.IDNum
		}
	})
	return id
}

func TestSendingAndReceivingMail(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	// Two characters, so there is somebody to write to. The first on an
	// empty roster is an implementor; the second is a mortal.
	first := dialClient(t, addr)
	first.create("Sender", "postagepaid", "m", "m")

	second := dialClient(t, addr)
	second.create("Recipient", "waitingfor", "m", "m")
	recipientID := idOf(t, srv, "Recipient")
	second.send("quit")
	second.expect("Goodbye")
	second.close()
	waitForLogout(t, srv, "Recipient")

	withPostmaster(t, srv, "Sender")
	setGold(t, srv, "Sender", 1000)

	first.send("mail Recipient")
	first.expect("I'll take 150 coins for the stamp.")
	first.expect("Write your message, use @ on a new line when done.")
	first.send("Come back, all is forgiven.")
	first.send("@")
	first.settle()

	// The stamp is taken before the message is written.
	if gold := goldOf(t, srv, "Sender"); gold != 850 {
		t.Errorf("after a 150-coin stamp the sender has %d gold, want 850", gold)
	}

	// The write is pushed off the world goroutine, so wait for it.
	if !eventually(5*time.Second, func() bool { return srv.mail.HasMail(recipientID) }) {
		t.Fatal("the message never reached the mail file")
	}

	// And now the recipient comes back for it.
	back := dialClient(t, addr)
	back.login("Recipient", "waitingfor")
	withPostmaster(t, srv, "Recipient")

	back.send("check")
	back.expect("You have mail waiting.")

	back.send("receive")
	back.expect("gives you a piece of mail.")

	back.send("read letter")
	back.expect("Midgaard Mail System")
	// Lower case, and not a mistake: get_name_by_id reads the C's player
	// table, and boot_db lowercases every name as it builds it (db.c:607). So
	// the mail header has always shouted in lower case.
	back.expect("To: recipient")
	back.expect("From: sender")
	back.expect("Come back, all is forgiven.")

	back.send("check")
	back.expect("Sorry, you don't have any mail waiting.")
}

// The same fixture as TestSendingAndReceivingMail, on native — proving the
// live send/receive path actually reaches mail/native's Store.
func TestSendingAndReceivingMailUnderNative(t *testing.T) {
	mailStore, err := mailnative.New(mail.Config{Path: filepath.Join(t.TempDir(), "state")})
	if err != nil {
		t.Fatal(err)
	}
	store, err := ascii.New(player.Config{Dir: filepath.Join(t.TempDir(), "pfiles")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	objects, err := binary.NewObjectStore(player.Config{Dir: filepath.Join(t.TempDir(), "plrobjs-lib")})
	if err != nil {
		t.Fatal(err)
	}
	srv, _ := newTestServerWith(t, store, objects, nil, nil, mailStore, nil)
	addr := listening(t, srv)

	first := dialClient(t, addr)
	first.create("Sender", "postagepaid", "m", "m")

	second := dialClient(t, addr)
	second.create("Recipient", "waitingfor", "m", "m")
	recipientID := idOf(t, srv, "Recipient")
	second.send("quit")
	second.expect("Goodbye")
	second.close()
	waitForLogout(t, srv, "Recipient")

	withPostmaster(t, srv, "Sender")
	setGold(t, srv, "Sender", 1000)

	first.send("mail Recipient")
	first.expect("I'll take 150 coins for the stamp.")
	first.expect("Write your message, use @ on a new line when done.")
	first.send("Come back, all is forgiven.")
	first.send("@")

	if !eventually(5*time.Second, func() bool { return srv.mail.HasMail(recipientID) }) {
		t.Fatal("the message never reached the mail file")
	}

	back := dialClient(t, addr)
	back.login("Recipient", "waitingfor")
	withPostmaster(t, srv, "Recipient")

	back.send("check")
	back.expect("You have mail waiting.")
	back.send("receive")
	back.expect("gives you a piece of mail.")
	back.send("read letter")
	back.expect("Come back, all is forgiven.")
}

func TestMailToNobody(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Writer", "toowhomever", "m", "m")
	withPostmaster(t, srv, "Writer")
	setGold(t, srv, "Writer", 1000)

	c.send("mail")
	c.expect("You need to specify an addressee!")

	c.send("mail Nobodyhere")
	c.expect("No one by that name is registered here!")
}

func TestAStampHasToBeAfforded(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Skinflint", "notenough", "m", "m")
	withPostmaster(t, srv, "Skinflint")
	setGold(t, srv, "Skinflint", 10)

	c.send("mail Skinflint")
	c.expect("A stamp costs 150 coins.")
	c.expect("...which I see you can't afford.")

	if gold := goldOf(t, srv, "Skinflint"); gold != 10 {
		t.Errorf("a refused stamp cost %d gold", 10-gold)
	}
}

// Level 1 characters cannot send mail, which is the whole of the C's
// anti-spam measure.
func TestLevelOneCannotSendMail(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Newbie", "justarrived", "m", "m")
	withPostmaster(t, srv, "Newbie")
	setGold(t, srv, "Newbie", 1000)

	inWorld(t, srv, func(w *game.Live) {
		if who := w.Find("Newbie"); who != nil && who.Record != nil {
			who.Record.Level = 1
		}
	})

	c.send("mail Newbie")
	c.expect("Sorry, you have to be level 2 to send mail!")
}

func TestNoMailWaiting(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Lonely", "nobodywrites", "m", "m")
	withPostmaster(t, srv, "Lonely")

	c.send("check")
	c.expect("Sorry, you don't have any mail waiting.")

	c.send("receive")
	c.expectCount("Sorry, you don't have any mail waiting.", 2)
}

func TestMailCommandsDoNothingAwayFromAPostOffice(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Isolated", "nopostbox", "m", "m")

	for _, command := range []string{"mail somebody", "check", "receive"} {
		c.send(command)
		c.expect("Sorry, but you cannot do that here!")
	}
}

// A letter longer than one block is stored in several and comes back whole.
func TestALongLetterSurvives(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	c := dialClient(t, addr)
	c.create("Novelist", "manywords", "m", "m")
	withPostmaster(t, srv, "Novelist")
	setGold(t, srv, "Novelist", 1000)

	id := idOf(t, srv, "Novelist")

	// Five lines, comfortably past the 79 characters a header block holds.
	lines := []string{
		strings.Repeat("a", 60),
		strings.Repeat("b", 60),
		strings.Repeat("c", 60),
		strings.Repeat("d", 60),
		strings.Repeat("e", 60),
	}
	c.send("mail Novelist")
	c.expect("Write your message, use @ on a new line when done.")
	for _, line := range lines {
		c.send(line)
	}
	c.send("@")
	c.settle()

	if !eventually(5*time.Second, func() bool { return srv.mail.HasMail(id) }) {
		t.Fatal("the message never reached the mail file")
	}

	c.send("receive")
	c.expect("gives you a piece of mail.")
	c.send("read letter")
	for _, line := range lines {
		c.expect(line)
	}
}

// The interface is what the postmaster tests, so a server with no mail file
// must present a nil one rather than a working-looking one.
func TestAServerWithNoMailFileHasNoMailSystem(t *testing.T) {
	var s Server
	// mailOrNil returns the interface type, which is the whole point: a
	// typed nil pointer wrapped in an interface is not nil, and the
	// postmaster tests `sc.Mail == nil`.
	if got := mailOrNil(&s); got != nil {
		t.Error("a server with no mail file reports a mail system")
	}
}
