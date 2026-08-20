// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package session

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// SPECIAL(gen_board), ported from boards.c:148.
//
// Unlike the shop and the bank, a board intercepts commands that already
// work: `look`, `read`, `write` and `remove` all do something perfectly
// ordinary anywhere else. So this special has to be careful to hand back
// anything that was not aimed at the board — `look` at a sword, `remove` a
// ring — and it does that by returning false, which puts the command back on
// its usual path.
//
// There is no `post`. You write on a board by typing `write <headline>` at
// it, and the body goes into the line editor.

// BoardSaver writes a board back to disk. A seam, like Rent: the store
// belongs to the server.
type BoardSaver func(b *game.Board)

func specGenBoard(sc *SpecialCall) bool {
	if sc.Session == nil || sc.Actor.Client == nil {
		return false
	}
	switch {
	case sc.Is("write"), sc.Is("look"), sc.Is("examine"), sc.Is("read"), sc.Is("remove"):
	default:
		return false
	}

	// find_board: whichever board is lying in the room, *not* the object this
	// special is attached to. A room with two boards in it always finds the
	// first, and the C's own comment on the failure case is "SYSERR:
	// degenerate board!  (what the hell...)".
	board := sc.World.BoardInRoom(sc.Actor.Room)
	if board == nil {
		return false
	}
	obj := sc.Obj

	switch {
	case sc.Is("write"):
		return boardWrite(sc, board)
	case sc.Is("look"), sc.Is("examine"):
		return boardShow(sc, board, obj)
	case sc.Is("read"):
		return boardRead(sc, board, obj)
	case sc.Is("remove"):
		return boardRemove(sc, board)
	}
	return false
}

// boardWrite is Board_write_message (boards.c:184).
func boardWrite(sc *SpecialCall, board *game.Board) bool {
	level := levelOf(sc.Actor)
	if level < board.Def.WriteLevel {
		sc.Tell("You are not holy enough to write on this board.\r\n")
		return true
	}
	if len(board.Messages) >= game.MaxBoardMessages {
		sc.Tell("The board is full.\r\n")
		return true
	}

	headline := strings.TrimSpace(sc.Arg)
	// delete_doubledollar: `$$` becomes `$`, because the heading goes through
	// act() later and a lone `$` there would eat the next character.
	headline = strings.ReplaceAll(headline, "$$", "$")
	if len(headline) > game.MaxBoardHeadline {
		headline = headline[:game.MaxBoardHeadline]
	}
	if headline == "" {
		sc.Tell("We must have a headline!\r\n")
		return true
	}

	heading := boardHeading(time.Now(), sc.Actor.Name, headline)
	sc.Tell("Write your message.  Terminate with a @ on a new line.\r\n\r\n")
	sc.ToRoom("%s starts to write a message.\r\n", sc.Actor.Name)

	// The body is collected by the line editor. The C hands the descriptor a
	// pointer to the message slot and lets string_write fill it in; here the
	// editor is given what to do when it is finished.
	//
	// Note the message is appended *when the editor closes*, not now. The C
	// increments num_of_msgs immediately and lets the half-written message
	// sit in the list — which is why Board_remove_msg has to check whether
	// anybody is still writing into a slot before freeing it. Waiting is
	// simpler and unobservable, since a message with no body reads as empty
	// either way.
	saver, level := sc.SaveBoard, level
	sc.Session.beginEditor(game.MaxBoardMessageLength, func(body string) {
		board.Messages = append(board.Messages, game.BoardMessage{
			Heading: heading, Level: level, Body: body,
		})
		if saver != nil {
			saver(board)
		}
	})
	return true
}

// boardHeading builds the line the board lists, porting the sprintf in
// Board_write_message.
//
//	sprintf(buf, "%6.10s %-12s :: %s", tmstr, buf2, arg);
//
// `%6.10s` on asctime's output is the strangest part: asctime gives
// "Thu Aug 20 01:23:45 2026", and a precision of 10 truncates that to
// "Thu Aug 20" while a width of 6 does nothing at all because the string is
// already longer. So the date on a board post has a weekday and no year, and
// the width in the format is dead.
func boardHeading(at time.Time, name, headline string) string {
	stamp := at.Format("Mon Jan _2 15:04:05 2006")
	if len(stamp) > 10 {
		stamp = stamp[:10]
	}
	return fmt.Sprintf("%6.10s %-12s :: %s", stamp, "("+name+")", headline)
}

// boardShow is Board_show_board (boards.c:233).
func boardShow(sc *SpecialCall, board *game.Board, obj *game.Object) bool {
	name, _ := oneArgument(sc.Arg)
	// `look` with no argument, or at something that is not the board, is not
	// ours — it goes back to the ordinary command.
	if name == "" || obj == nil || !obj.Matches(name) {
		return false
	}
	if levelOf(sc.Actor) < board.Def.ReadLevel {
		sc.Tell("You try but fail to understand the holy words.\r\n")
		return true
	}
	sc.ToRoom("%s studies the board.\r\n", sc.Actor.Name)

	var b strings.Builder
	b.WriteString("This is a bulletin board.  Usage: READ/REMOVE <messg #>, WRITE <header>.\r\n")
	b.WriteString("You will need to look at the board to save your message.\r\n")
	if len(board.Messages) == 0 {
		b.WriteString("The board is empty.\r\n")
	} else {
		fmt.Fprintf(&b, "There are %d messages on the board.\r\n", len(board.Messages))
		// NEWEST_AT_TOP is FALSE, so message 1 is the oldest.
		for i, m := range board.Messages {
			fmt.Fprintf(&b, "%-2d : %s\r\n", i+1, m.Heading)
		}
	}
	sc.Tell("%s", b.String())
	return true
}

// boardRead is Board_display_msg (boards.c:286).
func boardRead(sc *SpecialCall, board *game.Board, obj *game.Object) bool {
	number, _ := oneArgument(sc.Arg)
	if number == "" {
		return false
	}
	// "so 'read board' works" — reading the board itself lists it.
	if obj != nil && obj.Matches(number) {
		return boardShow(sc, board, obj)
	}
	// `read 2.mail` is not a board message; hand it back.
	if !isNumber(number) {
		return false
	}
	msg, err := strconv.Atoi(number)
	if err != nil || msg == 0 {
		return false
	}

	if levelOf(sc.Actor) < board.Def.ReadLevel {
		sc.Tell("You try but fail to understand the holy words.\r\n")
		return true
	}
	if len(board.Messages) == 0 {
		sc.Tell("The board is empty!\r\n")
		return true
	}
	if msg < 1 || msg > len(board.Messages) {
		sc.Tell("That message exists only in your imagination.\r\n")
		return true
	}

	m := board.Messages[msg-1]
	if m.Body == "" {
		sc.Tell("That message seems to be empty.\r\n")
		return true
	}
	sc.Tell("Message %d : %s\r\n\r\n%s\r\n", msg, m.Heading, m.Body)
	return true
}

// boardRemove is Board_remove_msg (boards.c:344).
func boardRemove(sc *SpecialCall, board *game.Board) bool {
	number, _ := oneArgument(sc.Arg)
	// `remove ring` is taking off a ring, not deleting a post.
	if number == "" || !isNumber(number) {
		return false
	}
	msg, err := strconv.Atoi(number)
	if err != nil || msg == 0 {
		return false
	}

	if len(board.Messages) == 0 {
		sc.Tell("The board is empty!\r\n")
		return true
	}
	if msg < 1 || msg > len(board.Messages) {
		sc.Tell("That message exists only in your imagination.\r\n")
		return true
	}

	m := board.Messages[msg-1]
	level := levelOf(sc.Actor)
	// Your own message is always yours to remove, whatever the board's
	// threshold — the level check is skipped entirely when your name is in
	// the heading.
	if level < board.Def.RemoveLevel && !m.PostedBy(sc.Actor.Name) {
		sc.Tell("You are not holy enough to remove other people's messages.\r\n")
		return true
	}
	if level < m.Level {
		sc.Tell("You can't remove a message holier than yourself.\r\n")
		return true
	}

	board.Messages = append(board.Messages[:msg-1], board.Messages[msg:]...)
	sc.Tell("Message removed.\r\n")
	sc.ToRoom("%s just removed message %d.\r\n", sc.Actor.Name, msg)
	if sc.SaveBoard != nil {
		sc.SaveBoard(board)
	}
	return true
}

func levelOf(c *game.Character) int32 {
	if c == nil || c.Record == nil {
		return 0
	}
	return c.Record.Level
}

// doRead is do_look with SCMD_READ (act.informative.c:672).
//
// The whole of the difference from `look`: an argument is required, and it is
// always a target rather than a direction or an `in`. So `read north` reads
// something called north and does not look that way.
func doRead(c *Context) error {
	arg, _ := halfChop(c.Arg)
	if arg == "" {
		c.Send("Read what?\r\n")
		return nil
	}
	return c.lookAtTarget(arg)
}

// doWrite is do_write (act.comm.c:300): a note, and a pen to write on it
// with.
//
// The one-argument form is the fiddly one, and the fiddliness is the point:
// `write note` means "the note in my inventory, with whatever I am holding",
// and `write pen` means the reverse, because it looks at what you named and
// works out which half of the pair it is.
func doWrite(c *Context) error {
	paperName, penName, _ := twoArguments(c.Arg)
	if paperName == "" {
		c.Send("Write?  With what?  ON what?  What are you trying to do?!?\r\n")
		return nil
	}

	var paper, pen *game.Object
	if penName != "" {
		if paper = findObject(c.Character.Carrying, paperName); paper == nil {
			c.Send("You have no %s.\r\n", paperName)
			return nil
		}
		if pen = findObject(c.Character.Carrying, penName); pen == nil {
			c.Send("You have no %s.\r\n", penName)
			return nil
		}
	} else {
		found := findObject(c.Character.Carrying, paperName)
		if found == nil {
			c.Send("There is no %s in your inventory.\r\n", paperName)
			return nil
		}
		switch {
		case found.Type == game.ItemPen:
			pen = found
		case found.Type != game.ItemNote:
			c.Send("That thing has nothing to do with writing.\r\n")
			return nil
		default:
			paper = found
		}
		held := c.Character.Equipment[game.WearHold]
		if held == nil {
			c.Send("You can't write with %s %s alone.\r\n", article(paperName), paperName)
			return nil
		}
		if pen != nil {
			paper = held
		} else {
			pen = held
		}
	}

	switch {
	case pen.Type != game.ItemPen:
		c.Send("%s is no good for writing with.\r\n", capitaliseFirst(pen.Name()))
		return nil
	case paper.Type != game.ItemNote:
		c.Send("You can't write on %s.\r\n", paper.Name())
		return nil
	}

	if existing := paper.ActionDescription(); existing != "" {
		c.Send("There's something written on it already:\r\n")
		c.Send("%s", ensureNewline(existing))
	}
	c.announce("%s begins to jot down a note.\r\n", c.Character.Name)

	if c.Session == nil {
		return nil
	}
	c.Session.Send("Write your message.  Terminate with a @ on a new line.\r\n\r\n")
	target := paper
	c.Session.beginEditor(maxNoteLength, func(text string) {
		target.ActionDesc = text
	})
	return nil
}

// maxNoteLength is MAX_NOTE_LENGTH (structs.h).
const maxNoteLength = 1000
