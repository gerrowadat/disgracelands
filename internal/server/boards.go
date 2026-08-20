// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"errors"

	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/persist/boards"
)

// init_boards (boards.c:118) and the saving half of Board_save_board.
//
// The C loads the boards lazily — the first time anybody looks at one, from
// inside the special procedure, guarded by a `static int loaded`. That is a
// reasonable thing to do with globals and an unreasonable thing to do on a
// world goroutine, so they are loaded at boot here instead. The only
// observable difference is when a corrupt board file is reported.

// loadBoards reads every board file and installs them in the world.
func (s *Server) loadBoards(w *game.Live) {
	loaded := make([]*game.Board, 0, len(game.Boards))
	messages := 0

	for _, def := range game.Boards {
		b := &game.Board{Def: def}

		// A board whose object does not exist is fatal in the C — it logs
		// "Fatal board error: board vnum %d does not exist!" and exits. Here
		// it is a warning: the shipped stock data is Midgaard and three of
		// these six boards are this server's own, so a mini-mud world is
		// missing most of them and refusing to boot over it helps nobody.
		if w.ObjectDef(def.Vnum) == nil {
			s.logger.Warn("board object does not exist in this world",
				"vnum", def.Vnum, "file", def.File)
			continue
		}

		if s.boards != nil {
			msgs, err := s.boards.Load(def.File)
			switch {
			case errors.Is(err, boards.ErrNotFound):
				// Nobody has posted to it. Normal.
			case err != nil:
				// The C deletes the file here. This keeps it and starts the
				// board empty, so whatever is wrong can still be looked at.
				s.logger.Error("reading a board file; starting it empty",
					"file", def.File, "error", err)
			default:
				for _, m := range msgs {
					b.Messages = append(b.Messages, game.BoardMessage{
						Heading: m.Heading, Level: m.Level, Body: m.Body,
					})
				}
			}
		}

		messages += len(b.Messages)
		loaded = append(loaded, b)
	}

	w.SetBoards(loaded)
	s.logger.Info("bulletin boards loaded", "boards", len(loaded), "messages", messages)
}

// SaveBoard implements session.BoardSaver.
//
// Called on the world goroutine, so the write is pushed off it — same
// reasoning as Save. The messages are copied first, because the board is
// about to be readable by everybody else again.
func (s *Server) SaveBoard(b *game.Board) {
	if s.boards == nil || b == nil {
		return
	}
	msgs := make([]boards.Message, 0, len(b.Messages))
	for _, m := range b.Messages {
		msgs = append(msgs, boards.Message{Heading: m.Heading, Level: m.Level, Body: m.Body})
	}
	file := b.Def.File

	go func() {
		if err := s.boards.Save(file, msgs); err != nil {
			s.logger.Error("writing a board file", "file", file, "error", err)
		}
	}()
}
