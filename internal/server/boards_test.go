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
	"github.com/gerrowadat/disgracelands/internal/persist/boards"
	boardsnative "github.com/gerrowadat/disgracelands/internal/persist/boards/native"
	"github.com/gerrowadat/disgracelands/internal/persist/player"
	"github.com/gerrowadat/disgracelands/internal/persist/player/ascii"
	"github.com/gerrowadat/disgracelands/internal/persist/player/binary"
)

// The bulletin board, end to end.
//
// Unlike the shop, a board intercepts commands that already work, so half of
// what matters here is what it *hands back*: `look` at a sword next to a
// board still looks at the sword.

// atBoard moves a character into the room with the board in it, and puts the
// board object there.
func atBoard(t *testing.T, srv *Server, name string) {
	t.Helper()
	inWorld(t, srv, func(w *game.Live) {
		who := w.Find(name)
		if who == nil {
			t.Error("the character is not in the world")
			return
		}
		if err := w.Enter(who, BoardRoom); err != nil {
			t.Errorf("moving to the board room: %v", err)
			return
		}
		if len(w.RoomObjects(BoardRoom)) == 0 {
			obj := w.NewObject(game.Boards[0].Vnum)
			if obj == nil {
				t.Error("the board prototype is missing")
				return
			}
			w.ObjectToRoom(obj, BoardRoom)
		}
	})
}

func TestWritingReadingAndRemovingAMessage(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Poster", "penandink", "m", "m")
	atBoard(t, srv, "Poster")

	c.send("look board")
	c.expect("This is a bulletin board.")
	c.expect("The board is empty.")

	c.send("write a first post")
	c.expect("Write your message.  Terminate with a @ on a new line.")
	c.send("Hello everybody.")
	c.send("@")
	c.settle()

	c.send("look board")
	c.expect("There are 1 messages on the board.")
	c.expect("(Poster)")
	c.expect(":: a first post")

	c.send("read 1")
	c.expect("Message 1 : ")
	c.expect("Hello everybody.")

	// The message survives a reboot: it is on disk the moment it is written.
	msgs, err := srv.boards.Load(game.Boards[0].File)
	if err != nil {
		t.Fatalf("reading the board file: %v", err)
	}
	if len(msgs) != 1 || !strings.Contains(msgs[0].Heading, "a first post") {
		t.Errorf("the board file holds %+v, want the one post", msgs)
	}

	c.send("remove 1")
	c.expect("Message removed.")

	c.send("look board")
	c.expectCount("The board is empty.", 2)

	// An emptied board leaves no file at all.
	if _, err := srv.boards.Load(game.Boards[0].File); err == nil {
		t.Error("emptying the board left the file behind")
	}
}

// The same fixture as TestWritingReadingAndRemovingAMessage, on native --
// proving the live write/read/remove path actually reaches boards/native's
// Store, not just its own isolated round-trip test.
func TestWritingReadingAndRemovingAMessageUnderNative(t *testing.T) {
	boardStore, err := boardsnative.New(boards.Config{Dir: filepath.Join(t.TempDir(), "state")})
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
	srv, _ := newTestServerWith(t, store, objects, nil, boardStore, nil, nil)
	c := dialClient(t, listening(t, srv))
	c.create("Poster", "penandink", "m", "m")
	atBoard(t, srv, "Poster")

	c.send("write a first post")
	c.expect("Write your message.  Terminate with a @ on a new line.")
	c.send("Hello everybody.")
	c.send("@")
	c.settle()

	c.send("look board")
	c.expect("There are 1 messages on the board.")
	c.expect("(Poster)")

	// SaveBoard backgrounds each save independently (internal/server/
	// boards.go), with no ordering guarantee against any other in-flight
	// one — Server.WaitForWrites is racy to call here (its WaitGroup Add
	// happens on the world goroutine while a command is still being
	// processed, which is the exact "Add concurrent with Wait" pattern
	// sync.WaitGroup documents as unsafe; the race detector agrees). Poll
	// instead, the same eventually() pattern mail_test.go/wizops_test.go
	// already use for backgrounded state.
	var msgs []boards.Message
	if !eventually(5*time.Second, func() bool {
		var loadErr error
		msgs, loadErr = srv.boards.Load(game.Boards[0].File)
		return loadErr == nil && len(msgs) == 1
	}) {
		t.Fatalf("the board file never gained the one post; last read: %+v", msgs)
	}
	if !strings.Contains(msgs[0].Heading, "a first post") {
		t.Errorf("the board file holds %+v, want the one post", msgs)
	}
	if msgs[0].Body != "Hello everybody.\r\n" {
		t.Errorf("the message body is %q, want CRLF-joined text", msgs[0].Body)
	}

	c.send("remove 1")
	c.expect("Message removed.")
	if !eventually(5*time.Second, func() bool {
		_, err := srv.boards.Load(game.Boards[0].File)
		return err != nil
	}) {
		t.Error("emptying the board left the file behind")
	}
}

func TestAMessageNeedsAHeadline(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Blankly", "nothingsaid", "m", "m")
	atBoard(t, srv, "Blankly")

	c.send("write")
	c.expect("We must have a headline!")
}

// You may always remove your own message, whatever the board's threshold —
// the level check is skipped when the heading names you.
func TestYouCanAlwaysRemoveYourOwnMessage(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Author", "myownwords", "m", "m")
	atBoard(t, srv, "Author")

	// A mortal. The mortal board's remove level is LVL_GOD.
	inWorld(t, srv, func(w *game.Live) {
		if who := w.Find("Author"); who != nil && who.Record != nil {
			who.Record.Level = 5
		}
	})

	c.send("write mine")
	c.expect("Terminate with a @")
	c.send("Words.")
	c.send("@")
	c.settle()

	c.send("remove 1")
	c.expect("Message removed.")
}

// Somebody else's message needs the board's remove level.
func TestYouCannotRemoveSomebodyElsesMessage(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Meddler", "notmypost", "m", "m")
	atBoard(t, srv, "Meddler")

	inWorld(t, srv, func(w *game.Live) {
		if who := w.Find("Meddler"); who != nil && who.Record != nil {
			who.Record.Level = 5
		}
		if b := w.BoardInRoom(BoardRoom); b != nil {
			b.Messages = append(b.Messages, game.BoardMessage{
				Heading: "Aug 20 2026 (Somebody)    :: not yours", Level: 5, Body: "Hands off.\r\n",
			})
		}
	})

	c.send("remove 1")
	c.expect("You are not holy enough to remove other people's messages.")
}

// And nobody removes a message posted by somebody holier, even on a board
// they can otherwise clear.
func TestYouCannotRemoveAMessageHolierThanYourself(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Deity", "highandmighty", "m", "m")
	atBoard(t, srv, "Deity")

	inWorld(t, srv, func(w *game.Live) {
		if who := w.Find("Deity"); who != nil && who.Record != nil {
			who.Record.Level = game.LevelGod
		}
		if b := w.BoardInRoom(BoardRoom); b != nil {
			b.Messages = append(b.Messages, game.BoardMessage{
				Heading: "Aug 20 2026 (Bigger)      :: above you",
				Level:   game.LevelImplementor, Body: "Mine.\r\n",
			})
		}
	})

	c.send("remove 1")
	c.expect("You can't remove a message holier than yourself.")
}

func TestAskingForAMessageThatIsNotThere(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Curious", "whatsonit", "m", "m")
	atBoard(t, srv, "Curious")

	c.send("read 1")
	c.expect("The board is empty!")

	inWorld(t, srv, func(w *game.Live) {
		if b := w.BoardInRoom(BoardRoom); b != nil {
			b.Messages = append(b.Messages, game.BoardMessage{
				Heading: "Aug 20 2026 (Someone)     :: one post", Level: 1, Body: "Hi.\r\n",
			})
		}
	})

	c.send("read 9")
	c.expect("That message exists only in your imagination.")

	// "so 'read board' works" — reading the board itself lists it.
	c.send("read board")
	c.expect("This is a bulletin board.")
}

// The board must hand back everything that was not aimed at it. This is the
// half that is easy to get wrong and impossible to notice until somebody
// cannot take off a ring in the wrong room.
func TestABoardDoesNotSwallowOrdinaryCommands(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Ordinary", "normalstuff", "m", "m")
	atBoard(t, srv, "Ordinary")

	inWorld(t, srv, func(w *game.Live) {
		who := w.Find("Ordinary")
		ring := w.NewObject(testRingVnum)
		if who == nil || ring == nil {
			t.Error("could not set up a ring")
			return
		}
		w.ObjectToChar(ring, who)
	})

	// `look` with no argument is the room, not the board.
	c.send("look")
	c.expect("The Notice Board")

	c.send("look ring")
	c.expect("You see nothing special about a gold ring.")

	c.send("wear ring")
	c.expect("You slide a gold ring on to your right ring finger.")

	// `remove ring` is taking it off, not deleting message "ring".
	c.send("remove ring")
	c.expect("You stop using a gold ring.")
}
