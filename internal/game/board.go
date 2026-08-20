// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import "strings"

// Bulletin boards, ported from boards.c.
//
// A board is an *object* with a special procedure on it, and the special
// takes `look`, `read`, `write` and `remove` away from the ordinary commands
// while you are standing next to it. There is no `post` command and never
// was: you write on the board by writing at it.
//
// The comment at the top of boards.c is a four-step guide to adding one, and
// somebody here followed it three times — half the boards on this server are
// local additions.

// MaxBoardMessages is MAX_BOARD_MESSAGES (boards.h:12).
const MaxBoardMessages = 60

// MaxBoardMessageLength is MAX_MESSAGE_LENGTH (boards.h:13). The headline is
// separately truncated to 80.
const MaxBoardMessageLength = 4096

// MaxBoardHeadline is the truncation in Board_write_message, marked in the C
// with a dated initial: "JE 27 Oct 95 - Truncate headline at 80 chars".
const MaxBoardHeadline = 80

// NewestAtTop is the #define at the top of boards.c, and it is FALSE — so
// message 1 is the *oldest* and a busy board buries new posts at the bottom.
const NewestAtTop = false

// BoardDef is one entry of board_info[] (boards.c:66): which object is a
// board, and who may do what to it.
type BoardDef struct {
	Vnum ObjVnum
	// ReadLevel, WriteLevel and RemoveLevel are the three thresholds. Note
	// that removing your *own* message ignores RemoveLevel entirely — the
	// check is skipped when your name is in the heading.
	ReadLevel   int32
	WriteLevel  int32
	RemoveLevel int32
	// File is the name under the etc directory. The C stores a full path.
	File string
}

// Boards is board_info[], in file order.
//
// Three of these are stock — the mortal board, the immortal board and the
// freeze log. The other three are this server's: a social board, a pkill
// board that only gods may write to, and a suggestion box. That is what the
// four-step guide at the top of boards.c was for.
var Boards = []BoardDef{
	{Vnum: 3099, ReadLevel: 0, WriteLevel: 0, RemoveLevel: LevelGod, File: "board.mort"},
	{Vnum: 3098, ReadLevel: LevelImmortal, WriteLevel: LevelImmortal, RemoveLevel: LevelGreaterGod, File: "board.immort"},
	// LVL_FREEZE is LVL_GRGOD (structs.h:495): an immortal may read the
	// freeze log, but only a greater god may add to it.
	{Vnum: 3097, ReadLevel: LevelImmortal, WriteLevel: LevelGreaterGod, RemoveLevel: LevelImplementor, File: "board.freeze"},
	{Vnum: 3096, ReadLevel: 0, WriteLevel: 0, RemoveLevel: LevelImmortal, File: "board.social"},
	{Vnum: 3095, ReadLevel: 0, WriteLevel: LevelGod, RemoveLevel: LevelGod, File: "board.pkill"},
	{Vnum: 3094, ReadLevel: 0, WriteLevel: 0, RemoveLevel: LevelGod, File: "board.suggestion"},
}

// BoardMessage is one post: a heading and a body.
//
// The C splits these across two arrays — an index of headings and a pool of
// message bodies addressed by slot number — because it wants to free and
// reuse body storage while somebody is still writing into it. Nothing here
// needs that, so a message is one value.
type BoardMessage struct {
	// Heading is the whole formatted line, not just the headline: the C
	// builds "Aug 20 2026 (Zod)        :: headline" once, at post time, and
	// stores that. It is also what the ownership check searches for "(Name)"
	// in, which is why a message cannot be re-attributed.
	Heading string
	// Level is the poster's level, so that nobody removes a message holier
	// than themselves.
	Level int32
	Body  string
}

// Board is a board's runtime state.
type Board struct {
	Def      BoardDef
	Messages []BoardMessage
}

// BoardFor returns the board an object is, or nil.
func (l *Live) BoardFor(obj *Object) *Board {
	if obj == nil || obj.Def == nil {
		return nil
	}
	for _, b := range l.boards {
		if b.Def.Vnum == obj.Def.Vnum {
			return b
		}
	}
	return nil
}

// BoardInRoom is find_board (boards.c:100): which board is lying in the room
// this character is standing in.
//
// The C searches the room's contents rather than trusting the object the
// special was attached to, which means a room with two boards in it always
// finds the first — and the message you write goes to that one whichever you
// were addressing. Reproduced, because that is what the game did.
func (l *Live) BoardInRoom(room RoomVnum) *Board {
	for _, obj := range l.RoomObjects(room) {
		if b := l.BoardFor(obj); b != nil {
			return b
		}
	}
	return nil
}

// SetBoards installs the loaded boards. Called once at boot.
func (l *Live) SetBoards(boards []*Board) { l.boards = boards }

// AllBoards returns every board, for the save-everything sweep.
func (l *Live) AllBoards() []*Board { return l.boards }

// PostedBy reports whether this message's heading names this character, which
// is what lets somebody remove their own post from a board they could not
// otherwise remove from.
//
// The C does `sprintf(buf, "(%s)", GET_NAME(ch))` and then `strstr`. A
// substring search, so a character called `Zod` matches a message posted by
// `Zod` and also one whose *headline* happens to contain "(Zod)". Nobody ever
// noticed, and it is reproduced rather than tightened.
func (m BoardMessage) PostedBy(name string) bool {
	return strings.Contains(m.Heading, "("+name+")")
}
