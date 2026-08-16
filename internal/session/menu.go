// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package session

import (
	"context"
	"fmt"
	"strings"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// The main menu and everything reachable from it, ported from nanny's
// CON_MENU (interpreter.c:1637) and the states it leads to.
//
// A player does not walk into the world straight off the message of the day:
// the C shows a menu and waits for a choice, and four of the six choices are
// things that can only be done before entering — the description editor, the
// background story, changing a password, and deleting the character.

// maxDescriptionLength is EXDSCR_LENGTH (structs.h:538).
//
// The binary format's field is 240 bytes and the C truncates to it. The ascii
// format has no such limit, but the cap is kept so a description written on
// the Go server still round-trips to the C one — the whole point of §5.5.
const maxDescriptionLength = 240

// descriptionTerminator ends the description editor. The C's string editor
// uses '@' on a line of its own, and so does this.
const descriptionTerminator = "@"

func (s *Session) handleMenu(ctx context.Context, deps Deps, line string) error {
	arg := strings.TrimSpace(line)
	if arg == "" {
		s.Send("%s", deps.Text.Menu())
		return nil
	}

	switch arg[0] {
	case '0':
		s.Send("Goodbye.\r\n")
		s.Close()
		return nil

	case '1':
		return s.enterWorld(ctx, deps)

	case '2':
		if s.character != nil && s.character.Record != nil && s.character.Record.Description != "" {
			s.Send("Old description:\r\n%s", ensureTrailingNewline(s.character.Record.Description))
			// The C frees the old description here, so a player who
			// disconnects mid-edit loses it. That is not a behaviour worth
			// reproducing: the old text is kept until a new one replaces it.
		}
		s.Send("Enter the new text you'd like others to see when they look at you.\r\n")
		s.Send("Terminate with a '%s' on a new line.\r\n", descriptionTerminator)
		s.editorLines = nil
		s.state = StateEnterDescription
		return nil

	case '3':
		s.Send("%s", ensureTrailingNewline(deps.Text.Background()))
		// The C leaves the connection in CON_RMOTD, so the next line typed
		// brings the menu back. Same here.
		s.state = StateReadMOTD
		return nil

	case '4':
		s.EchoOff()
		s.Send("\r\nEnter your old password: ")
		s.state = StateChangePasswordOld
		return nil

	case '5':
		s.EchoOff()
		s.Send("\r\nEnter your password for verification: ")
		s.state = StateDeleteVerify
		return nil
	}

	s.Send("\r\nThat's not a menu choice!\r\n%s", deps.Text.Menu())
	return nil
}

// enterWorld is menu choice 1: the part of the C's CON_MENU that puts a
// character into a room.
func (s *Session) enterWorld(ctx context.Context, deps Deps) error {
	// A character who has never entered the world is still at level zero: the
	// C runs do_start here and not before (interpreter.c:1684), so the first
	// character on an empty roster — made an Implementor during creation —
	// correctly never runs it. Enter does the honours, since it is the side
	// that owns the record.
	firstTime := s.character != nil && s.character.Record != nil && s.character.Record.Level == 0

	s.Send("%s", deps.Text.Welcome())

	if err := deps.Login.Enter(ctx, s, s.character); err != nil {
		s.logger.Error("entering the world", "error", err)
		s.Send("Something went wrong putting you into the world. Try again shortly.\r\n")
		s.Close()
		return nil
	}
	s.state = StatePlaying
	s.logger.Info("entered the world", "character", s.character.Name)

	if firstTime {
		s.Send("%s", deps.Text.Start())
	}

	// Show them where they are, which is what the C server does on entry.
	return deps.Commands.Do(ctx, s, "look")
}

// handleEnterDescription collects lines until the terminator.
func (s *Session) handleEnterDescription(ctx context.Context, deps Deps, line string) error {
	if strings.TrimSpace(line) == descriptionTerminator {
		text := strings.Join(s.editorLines, "\r\n")
		if text != "" {
			text += "\r\n"
		}
		if len(text) > maxDescriptionLength {
			text = text[:maxDescriptionLength]
			s.Send("Your description was truncated to %d characters.\r\n", maxDescriptionLength)
		}
		s.editorLines = nil

		if s.character != nil && s.character.Record != nil {
			s.character.Record.Description = text
			if err := deps.Login.Save(ctx, s.character); err != nil {
				s.logger.Error("saving a description", "character", s.character.Name, "error", err)
			}
		}
		s.state = StateMenu
		s.Send("%s", deps.Text.Menu())
		return nil
	}

	// A description that has already run past the limit is not worth
	// buffering more of; the C's editor stops accepting input at the cap too.
	if s.editorSize() < maxDescriptionLength {
		s.editorLines = append(s.editorLines, line)
	}
	return nil
}

func (s *Session) editorSize() int {
	n := 0
	for _, l := range s.editorLines {
		n += len(l) + 2
	}
	return n
}

// handleChangePasswordOld checks the current password before letting it be
// replaced.
func (s *Session) handleChangePasswordOld(ctx context.Context, deps Deps, line string) error {
	ok, err := deps.Login.CheckPassword(ctx, s.character, strings.TrimSpace(line))
	if err != nil {
		s.EchoOn()
		s.logger.Error("checking a password", "character", s.character.Name, "error", err)
		s.Send("\r\nSomething went wrong checking that.\r\n%s", deps.Text.Menu())
		s.state = StateMenu
		return nil
	}
	if !ok {
		s.EchoOn()
		s.Send("\r\nIncorrect password.\r\n%s", deps.Text.Menu())
		s.state = StateMenu
		return nil
	}
	s.Send("\r\nEnter a new password: ")
	s.state = StateChangePasswordNew
	return nil
}

func (s *Session) handleChangePasswordNew(deps Deps, line string) error {
	password := strings.TrimSpace(line)
	if reason := badNewPassword(password, s.character.Name); reason != "" {
		s.Send("\r\n%s\r\nPassword: ", reason)
		return nil
	}
	s.pendingPassword = password
	s.state = StateChangePasswordVerify
	s.Send("\r\nPlease retype password: ")
	return nil
}

func (s *Session) handleChangePasswordVerify(ctx context.Context, deps Deps, line string) error {
	if strings.TrimSpace(line) != s.pendingPassword {
		s.pendingPassword = ""
		s.state = StateChangePasswordNew
		s.Send("\r\nPasswords don't match... start over.\r\nPassword: ")
		return nil
	}

	password := s.pendingPassword
	s.pendingPassword = ""
	s.EchoOn()

	if err := deps.Login.SetPassword(ctx, s.character, password); err != nil {
		s.logger.Error("changing a password", "character", s.character.Name, "error", err)
		s.Send("\r\nSomething went wrong saving that.\r\n%s", deps.Text.Menu())
		s.state = StateMenu
		return nil
	}
	s.logger.Info("password changed", "character", s.character.Name)
	s.Send("\r\nDone.\r\n%s", deps.Text.Menu())
	s.state = StateMenu
	return nil
}

// handleDeleteVerify is the first of the two confirmations the C asks for.
func (s *Session) handleDeleteVerify(ctx context.Context, deps Deps, line string) error {
	s.EchoOn()

	ok, err := deps.Login.CheckPassword(ctx, s.character, strings.TrimSpace(line))
	if err != nil || !ok {
		if err != nil {
			s.logger.Error("checking a password", "character", s.character.Name, "error", err)
		}
		s.Send("\r\nIncorrect password.\r\n%s", deps.Text.Menu())
		s.state = StateMenu
		return nil
	}

	s.Send("\r\nYOU ARE ABOUT TO DELETE THIS CHARACTER PERMANENTLY.\r\n" +
		"ARE YOU ABSOLUTELY SURE?\r\n\r\n" +
		"Please type \"yes\" to confirm: ")
	s.state = StateDeleteConfirm
	return nil
}

func (s *Session) handleDeleteConfirm(ctx context.Context, deps Deps, line string) error {
	// The C compares against "yes" and "YES" only, so "Yes" does not delete a
	// character. That is a good accident and it is kept: the confirmation
	// being awkward to type is the point.
	answer := strings.TrimSpace(line)
	if answer != "yes" && answer != "YES" {
		s.Send("\r\nCharacter not deleted.\r\n%s", deps.Text.Menu())
		s.state = StateMenu
		return nil
	}

	rec := s.character.Record
	if rec != nil && rec.PlayerFlags.Has(game.PlayerFrozen) {
		s.Send("You try to kill yourself, but the ice stops you.\r\n")
		s.Send("Character not deleted.\r\n\r\n")
		s.Close()
		return nil
	}

	if err := deps.Login.Delete(ctx, s.character); err != nil {
		s.logger.Error("deleting a character", "character", s.character.Name, "error", err)
		s.Send("\r\nSomething went wrong deleting that character.\r\n%s", deps.Text.Menu())
		s.state = StateMenu
		return nil
	}

	s.logger.Info("character self-deleted", "character", s.character.Name, "level", s.character.Level())
	s.Send("Character '%s' deleted!\r\nGoodbye.\r\n", s.character.Name)
	// Not MarkQuit: they were never in the world, so there is nothing to
	// remove from it.
	s.character = nil
	s.Close()
	return nil
}

// badNewPassword reports why a password cannot be set, or "".
//
// The C refuses an empty password, one longer than ten characters, one
// shorter than three, and one equal to the character's name
// (interpreter.c:1526). Two of those four are kept, and the reasons are
// recorded in docs/deviations.md: the ten-character ceiling existed because
// the field in the binary record is ten bytes wide, and the three-character
// floor was reasonable when the hash truncated to eight characters anyway.
// Neither is true now.
func badNewPassword(password, name string) string {
	if len(password) < minPasswordLength {
		return fmt.Sprintf("Passwords must be at least %d characters.", minPasswordLength)
	}
	if strings.EqualFold(password, name) {
		return "Illegal password."
	}
	return ""
}

func ensureTrailingNewline(s string) string {
	if s == "" || strings.HasSuffix(s, "\n") {
		return s
	}
	return s + "\r\n"
}
