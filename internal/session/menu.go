// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package session

import (
	"context"
	"strconv"
	"strings"

	"github.com/gerrowadat/disgracelands/internal/auth"
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
		// The C leaves the connection in CON_RMOTD once background's own
		// page_string call finishes (interpreter.c:1712-1714), so the next
		// line typed brings the menu back — set first, so sendPaged
		// captures it as what paging interrupted (Session.pagerReturn)
		// rather than the menu state background is actually being run
		// from.
		s.state = StateReadMOTD
		s.SendPaged("%s", ensureTrailingNewline(deps.Text.Background()))
		// sendPaged never sends the pager's own "Return to continue" line
		// itself — every other caller relies on Dispatcher.Do's own tail
		// for that (prompt(s) resolves to pagingPrompt() once StatePaging
		// is set), and this menu handler is not run through it. A short
		// background leaves s.state exactly as StateReadMOTD, set above,
		// so this only fires when pagination actually happened.
		if s.state == StatePaging {
			s.Send("%s", prompt(s))
		}
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

// EnterResult is what entering the world turned up, beyond success.
type EnterResult struct {
	// RentLost is true when the character could not pay the rent they owed
	// and their things were destroyed rather than returned.
	RentLost bool
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

	result, err := deps.Login.Enter(ctx, s, s.character)
	if err != nil {
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
	if err := deps.Commands.Do(ctx, s, "look"); err != nil {
		return err
	}

	// Last, after the room — the order the C sends it in, and the \007 is
	// the C's too: this is the one message in the game that rings the bell.
	if result.RentLost {
		s.Send("\r\n\007You could not afford your rent!\r\n" +
			"Your possesions have been donated to the Salvation Army!\r\n")
	}
	return nil
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
// The rule itself is auth.BadPassword: `dlctl pfile passwd` sets passwords
// too, and the two must agree about what is settable or an administrator can
// hand out a password the owner could never have chosen themselves.
func badNewPassword(password, name string) string {
	return auth.BadPassword(password, name)
}

func ensureTrailingNewline(s string) string {
	if s == "" || strings.HasSuffix(s, "\n") {
		return s
	}
	return s + "\r\n"
}

// The general line editor, porting string_write (modify.c).
//
// Collect lines until a lone '@', then hand the text to whoever asked for it.
// The C passes a pointer to the string being filled and a magic number saying
// what to do when it is done; a closure says the same thing without the
// magic.

// beginEditor puts the session into the line editor.
func (s *Session) beginEditor(maxLength int, done func(text string, saved bool)) {
	s.beginEditorSeeded(maxLength, "", done)
}

// beginEditorSeeded is beginEditor with existing content already in the
// buffer, porting string_write's own plain-editor behaviour when the
// pointer it is handed already points at something: string_add's
// non-empty branch (RECREATE+strcat) appends each typed line onto what
// was already there rather than starting fresh. tedit is the first
// caller with anything to seed — do_tedit shows the file's current
// content before handing the descriptor to string_write with that same
// buffer — so board `write`/mail/description, which always compose new
// text, keep going through beginEditor and see no change at all.
//
// seed is split on \r\n, the same line-ending Text's fields and
// game.HelpEntry.Body both already use; a trailing empty element (a seed
// that already ends in \r\n, which every one of Text's fields does) is
// dropped so an empty line is not appended for free.
func (s *Session) beginEditorSeeded(maxLength int, seed string, done func(text string, saved bool)) {
	var lines []string
	if seed != "" {
		lines = strings.Split(seed, "\r\n")
		if len(lines) > 0 && lines[len(lines)-1] == "" {
			lines = lines[:len(lines)-1]
		}
	}
	s.editorLines = lines
	s.editorMax = maxLength
	s.editorDone = done
	s.state = StateEditing
}

// handleEditing collects one line of an edited text, porting string_add
// (modify.c:117): the '@' terminator, then — new here — the five
// improved-editor commands editorCommand answers, then plain buffering.
func (s *Session) handleEditing(line string) error {
	if strings.TrimSpace(line) == descriptionTerminator {
		return s.finishEditing(true)
	}
	if handled, err := s.editorCommand(line); handled {
		return err
	}
	s.editorLines = append(s.editorLines, line)
	return nil
}

// editorCommand runs one improved-editor command, porting
// improved_editor_execute (improved-edit.c:27) for the five commands that
// were both always on in the archived server — CONFIG_IMPROVED_EDITOR is
// hardcoded 1 there, docs/deviations.md — and need no line-range editing
// machinery of their own: /a (abort), /c (clear), /h (help), /l (list) and
// /s (save). /d, /e, /f, /i, /n and /r — delete, edit, format, insert,
// numbered list and replace — are not built; typing one of those, or any
// other letter, falls to the C's own default case, "Invalid option."
//
// Reports whether the line was a command at all: a bare '/' or anything
// not starting with one is not, and falls through to being buffered as
// ordinary text, the same as the C's own `if (*str != '/') return
// STRINGADD_OK`.
func (s *Session) editorCommand(line string) (bool, error) {
	if !strings.HasPrefix(line, "/") {
		return false, nil
	}
	var letter byte
	if len(line) > 1 {
		letter = line[1]
	}
	var args string
	if len(line) > 2 {
		args = line[2:]
	}

	switch letter {
	case 'a':
		return true, s.finishEditing(false)
	case 's':
		return true, s.finishEditing(true)
	case 'c':
		if len(s.editorLines) == 0 {
			s.Send("Current buffer empty.\r\n")
		} else {
			s.editorLines = nil
			s.Send("Current buffer cleared.\r\n")
		}
	case 'h':
		s.Send("%s", editorHelpText)
	case 'l':
		s.editorList(args)
	default:
		s.Send("Invalid option.\r\n")
	}
	return true, nil
}

// editorHelpText is send_editor_help's own PARSE_HELP text
// (improved-edit.c:104-120), trimmed to the five commands this port
// answers — advertising /d, /e, /f, /i, /n or /r here would promise
// something that then says "Invalid option." when typed.
const editorHelpText = "Editor command formats: /<letter>\r\n\r\n" +
	"/a         -  aborts editor\r\n" +
	"/c         -  clears buffer\r\n" +
	"/h         -  list text editor commands\r\n" +
	"/l         -  lists buffer\r\n" +
	"/s         -  saves text\r\n"

// editorList is parse_action's PARSE_LIST_NORM (improved-edit.c:215-276),
// sent directly rather than through the pager: paging mid-edit would need
// StatePaging to remember what state to return to, which it does not —
// session/pager.go's own doc comment names the identical gap for
// `background` — and a buffer within any caller's own length limit rarely
// runs past a screen anyway.
func (s *Session) editorList(args string) {
	if len(s.editorLines) == 0 {
		s.Send("Current buffer empty.\r\n")
		return
	}

	low, high, errMsg := parseEditorRange(args)
	if errMsg != "" {
		s.Send("%s", errMsg)
		return
	}
	if low < 1 {
		s.Send("Line numbers must be greater than 0.\r\n")
		return
	}
	if low > len(s.editorLines) {
		s.Send("Line(s) out of range; no buffer listing.\r\n")
		return
	}
	// The header decision is made from the *requested* range, before
	// clamping to what the buffer actually holds — porting the C's own
	// `if (line_high < 999999 || line_low > 1)` (improved-edit.c:243),
	// which reads line_high exactly as sscanf left it. Deciding it after
	// clamping would suppress the header for "/l 1-500" on a five-line
	// buffer, where the C prints one anyway (500 is not the sentinel).
	header := low > 1 || high < maxEditorLine
	if high > len(s.editorLines) {
		high = len(s.editorLines)
	}

	shown := s.editorLines[low-1 : high]
	if header {
		s.Send("Current buffer range [%d - %d]:\r\n", low, high)
	}
	s.Send("%s\r\n", strings.Join(shown, "\r\n"))
	plural := "s "
	if len(shown) == 1 {
		plural = " "
	}
	s.Send("%d line%sshown.\r\n", len(shown), plural)
}

// maxEditorLine stands in for the C's 999999 (improved-edit.c:225,232),
// its own way of saying "to the end" in an sscanf that always wants two
// numbers.
const maxEditorLine = 999999

// parseEditorRange parses /l's optional line-range argument, porting
// parse_action's `sscanf(string, " %d - %d ", &line_low, &line_high)`
// (improved-edit.c:222) closely enough for what a person actually types:
// nothing selects the whole buffer, one number selects that line alone,
// and two separated by '-' select an inclusive range. Anything that does
// not parse as a number falls back the same way sscanf's own zero-match
// case does, rather than being rejected outright.
func parseEditorRange(args string) (low, high int, errMsg string) {
	args = strings.TrimSpace(args)
	if args == "" {
		return 1, maxEditorLine, ""
	}

	before, after, hasDash := strings.Cut(args, "-")
	lo, err := strconv.Atoi(strings.TrimSpace(before))
	if err != nil {
		return 1, maxEditorLine, ""
	}
	if !hasDash {
		return lo, lo, ""
	}
	hi, err := strconv.Atoi(strings.TrimSpace(after))
	if err != nil {
		return lo, lo, ""
	}
	if hi < lo {
		return 0, 0, "That range is invalid.\r\n"
	}
	return lo, hi, ""
}

// finishEditing ends the line editor, porting string_add's own tail
// (modify.c:159-221). saved is false only for /a: the C frees *d->str and
// restores whatever was there before (modify.c:170-172), which this port
// has never captured a "before" of — every caller already treats an empty
// result as "nothing changed" (tedit's file, mail's send, a board post),
// so handing back "" is the observable-equivalent outcome.
func (s *Session) finishEditing(saved bool) error {
	var text string
	if saved {
		text = strings.Join(s.editorLines, "\r\n")
		if text != "" {
			text += "\r\n"
		}
		if s.editorMax > 0 && len(text) > s.editorMax {
			text = text[:s.editorMax]
			s.Send("Your message was truncated to %d characters.\r\n", s.editorMax)
		}
	}

	done := s.editorDone
	s.editorLines, s.editorDone, s.editorMax = nil, nil, 0
	s.state = StatePlaying
	if done != nil {
		done(text, saved)
	}
	s.Send("%s", prompt(s))
	return nil
}
