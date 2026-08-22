// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package session

import (
	"context"
	"strings"
	"unicode"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// This is the port of interpreter.c's nanny(): the state machine every
// connection passes through before it is a character in a room.
//
// It is deliberately close to the original in shape and in wording, because
// the login sequence is the part of a MUD players remember, and because §12
// requires specific things be said during it. Where it differs from the C —
// the password rules, mainly — the difference is commented.

// maxNameLength is the longest character name accepted.
//
// The ascii player format has no limit of its own, but a name is also a
// filename there, and the C server's who-list and index formatting assume
// something short. Twenty matches what the binary format held, which keeps
// conversion in both directions lossless.
const maxNameLength = 20

// What a new password must be is auth.MinPasswordLength, beside the hashing
// it is a floor for; badNewPassword is the only thing here that cares.

// handle advances the login state machine by one line of input.
func (s *Session) handle(ctx context.Context, deps Deps, line string) error {
	switch s.state {
	case StateGetName:
		return s.handleGetName(ctx, deps, line)
	case StateConfirmName:
		return s.handleConfirmName(deps, line)
	case StatePassword:
		return s.handlePassword(ctx, deps, line)
	case StateNewPassword:
		return s.handleNewPassword(deps, line)
	case StateConfirmPassword:
		return s.handleConfirmPassword(deps, line)
	case StateQuerySex:
		return s.handleQuerySex(deps, line)
	case StateQueryClass:
		return s.handleQueryClass(ctx, deps, line)
	case StateReadMOTD:
		return s.handleReadMOTD(deps)
	case StateMenu:
		return s.handleMenu(ctx, deps, line)
	case StateEnterDescription:
		return s.handleEnterDescription(ctx, deps, line)
	case StateEditing:
		return s.handleEditing(line)
	case StateChangePasswordOld:
		return s.handleChangePasswordOld(ctx, deps, line)
	case StateChangePasswordNew:
		return s.handleChangePasswordNew(deps, line)
	case StateChangePasswordVerify:
		return s.handleChangePasswordVerify(ctx, deps, line)
	case StateDeleteVerify:
		return s.handleDeleteVerify(ctx, deps, line)
	case StateDeleteConfirm:
		return s.handleDeleteConfirm(ctx, deps, line)
	case StatePlaying:
		return deps.Commands.Do(ctx, s, line)
	}
	return nil
}

// handleGetName takes the name and decides whether this is a returning
// player or a new one.
func (s *Session) handleGetName(ctx context.Context, deps Deps, line string) error {
	name := strings.TrimSpace(line)
	if name == "" {
		s.Close()
		return nil
	}
	if reason := invalidName(name); reason != "" {
		s.Send("%s\r\nBy what name do you wish to be known? ", reason)
		return nil
	}
	// Valid_Name's xnames half (ban.c:255-286): a case-insensitive
	// substring match against a loaded list, separate from invalidName's
	// checks because the list is per-server data, not a pure function of
	// the name alone.
	if deps.Login.DisallowedName(name) {
		s.Send("That name is not allowed.\r\nBy what name do you wish to be known? ")
		return nil
	}

	name = capitalise(name)
	exists, err := deps.Login.Exists(ctx, name)
	if err != nil {
		s.logger.Error("looking up a character", "name", name, "error", err)
		s.Send("Something went wrong looking you up. Try again shortly.\r\n")
		s.Close()
		return nil
	}

	// The ban check, at the name prompt (interpreter.c's CON_GET_NAME). A
	// site banned outright is refused whoever they say they are; one banned
	// to new players is refused only if the name is new.
	switch deps.Login.BanFor(s.Host()) {
	case "all":
		s.Send("You are not welcome here.\r\n")
		s.logger.Info("refused a banned site", "host", s.Host(), "ban", "all")
		s.Close()
		return nil
	case "new":
		if !exists {
			s.Send("Sorry, new characters are not allowed from your site!\r\n")
			s.logger.Info("refused a new character from a banned site", "host", s.Host())
			s.Close()
			return nil
		}
	case "select":
		// SELECT lets in only characters flagged PLR_SITEOK. Treated as `all`,
		// which is the conservative reading: letting everybody through would
		// make `ban select` do nothing at all and say nothing about it.
		//
		// `set <name> siteok` landed in 5i-e, so the flag is settable now and
		// the real check is a lookup of the named character's record here.
		// Noted in docs/deviations.md rather than left to be discovered.
		s.Send("You are not welcome here.\r\n")
		s.logger.Info("refused a banned site", "host", s.Host(), "ban", "select")
		s.Close()
		return nil
	}

	s.pendingName = name
	if exists {
		s.state = StatePassword
		s.EchoOff()
		s.Send("Password: ")
		return nil
	}
	s.state = StateConfirmName
	s.Send("Did I get that right, %s (Y/N)? ", name)
	return nil
}

func (s *Session) handleConfirmName(deps Deps, line string) error {
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		s.state = StateNewPassword
		s.EchoOff()
		s.Send("New character.\r\nGive me a password for %s: ", s.pendingName)
	case "n", "no":
		s.pendingName = ""
		s.state = StateGetName
		s.Send("Okay, what IS it, then? ")
	default:
		s.Send("Please type Yes or No: ")
	}
	return nil
}

func (s *Session) handlePassword(ctx context.Context, deps Deps, line string) error {
	password := strings.TrimSpace(line)
	// Whatever happens next, they are done typing a password, and a player
	// left with echo off is a player typing blind.
	s.EchoOn()
	s.Send("\r\n")

	character, err := deps.Login.Authenticate(ctx, s.pendingName, password)
	if err != nil {
		s.logger.Error("authenticating", "name", s.pendingName, "error", err)
		s.Send("Something went wrong checking that. Try again shortly.\r\n")
		s.Close()
		return nil
	}
	if character == nil {
		// Deliberately the same message whether the name or the password was
		// wrong, and logged with the host, matching the C server's "Bad PW"
		// line. Telling an anonymous connection which half it got right is
		// how a roster becomes a list of accounts to attack.
		s.logger.Info("failed login", "name", s.pendingName)
		s.Send("Wrong password.\r\n")
		s.Close()
		return nil
	}

	// If their previous connection dropped, their character is still
	// standing where they left it. Reconnect to that body rather than
	// putting a second copy of them into the world.
	if existing := deps.Login.Reconnect(ctx, character.Name); existing != nil {
		s.logger.Info("reconnecting to a linkdead character", "character", existing.Name)
		s.character = existing
		s.state = StatePlaying
		existing.Client = s
		s.Send("Reconnecting.\r\n")
		return deps.Commands.Do(ctx, s, "look")
	}

	s.character = character
	s.state = StateReadMOTD
	s.Send("%s\r\n*** PRESS RETURN: ", motdFor(deps, character))
	return nil
}

// motdFor picks the message of the day, which is a different file for
// immortals (interpreter.c:1504).
func motdFor(deps Deps, c *game.Character) string {
	if c != nil && c.Record != nil && c.Record.Level >= game.LevelImmortal {
		return deps.Text.ImmortalMOTD()
	}
	return deps.Text.MOTD()
}

func (s *Session) handleNewPassword(deps Deps, line string) error {
	password := strings.TrimSpace(line)
	if reason := badNewPassword(password, s.pendingName); reason != "" {
		s.Send("\r\n%s\r\nPassword: ", reason)
		return nil
	}
	s.pendingPassword = password
	s.state = StateConfirmPassword
	s.Send("\r\nPlease retype password: ")
	return nil
}

func (s *Session) handleConfirmPassword(deps Deps, line string) error {
	if strings.TrimSpace(line) != s.pendingPassword {
		s.pendingPassword = ""
		s.state = StateNewPassword
		s.Send("\r\nPasswords don't match.\r\nGive me a password for %s: ", s.pendingName)
		return nil
	}
	s.EchoOn()
	// The C asks sex next, then class. Same order, same wording.
	s.state = StateQuerySex
	s.Send("\r\nWhat is your sex (M/F)? ")
	return nil
}

func (s *Session) handleQuerySex(deps Deps, line string) error {
	arg := strings.TrimSpace(line)
	if arg == "" {
		s.Send("That is not a sex.\r\nWhat IS your sex? ")
		return nil
	}
	sex := game.ParseSex(arg[0])
	if sex < 0 {
		s.Send("That is not a sex.\r\nWhat IS your sex? ")
		return nil
	}
	s.pendingSex = sex
	s.state = StateQueryClass
	s.Send("%s\r\nClass: ", game.CreationMenu)
	return nil
}

func (s *Session) handleQueryClass(ctx context.Context, deps Deps, line string) error {
	arg := strings.TrimSpace(line)
	class := game.ClassUndefined
	if arg != "" {
		// ParseCreationClass, not ParseClass: Paladin is reached by
		// remorting and is not on the menu. See internal/game/class.go for
		// the discrepancy in the C that this deliberately does not
		// reproduce.
		class = game.ParseCreationClass(arg[0])
	}
	if class == game.ClassUndefined {
		s.Send("\r\nThat's not a class.\r\nClass: ")
		return nil
	}

	character, err := deps.Login.Create(ctx, CreateRequest{
		Name:     s.pendingName,
		Password: s.pendingPassword,
		Sex:      s.pendingSex,
		Class:    class,
	})
	s.pendingPassword = ""
	if err != nil {
		s.logger.Error("creating a character", "name", s.pendingName, "error", err)
		s.Send("Something went wrong creating your character. Try again shortly.\r\n")
		s.Close()
		return nil
	}

	s.character = character
	s.state = StateReadMOTD
	s.Send("\r\n%s\r\n*** PRESS RETURN: ", motdFor(deps, character))
	return nil
}

// handleReadMOTD is the return pressed after the message of the day, and
// after the background story. Either way it leads to the menu, which is what
// the C's CON_RMOTD does.
func (s *Session) handleReadMOTD(deps Deps) error {
	s.state = StateMenu
	s.Send("%s", deps.Text.Menu())
	return nil
}

// invalidName reports why a name cannot be used, or "".
//
// The C server checks length and that every character is a letter
// (interpreter.c's _parse_name). The same rules are kept, with the reasons
// spelled out rather than the C's single "Illegal name" — a player who typed
// an apostrophe deserves to know that is the problem. The xnames substring
// check (Valid_Name, ban.c:255) is not here: it needs per-server data
// invalidName has no way to receive, so it is a separate call at the
// handleGetName call site instead — see [Session.handleGetName].
func invalidName(name string) string {
	if len(name) < 2 {
		return "That name is too short."
	}
	if len(name) > maxNameLength {
		return "That name is too long."
	}
	for _, r := range name {
		if !unicode.IsLetter(r) {
			return "Names may only contain letters."
		}
	}
	// A name is also a filename in the ascii player format, and these are
	// what a filesystem would make of one.
	switch strings.ToLower(name) {
	case "con", "nul", "aux", "prn":
		return "That name is not available."
	}
	// reserved_word (interpreter.c:952), checked alongside fill_word right
	// beside Valid_Name at every CON_GET_NAME call site — an exact match,
	// not the substring one xnames does below.
	if reservedNames[strings.ToLower(name)] {
		return "That name is reserved."
	}
	return ""
}

// capitalise renders a name the way the game stores it: first letter upper,
// rest lower. The C server does this in _parse_name so that "ZOD" and "zod"
// are the same character and both display as "Zod".
func capitalise(name string) string {
	lower := strings.ToLower(name)
	r := []rune(lower)
	if len(r) == 0 {
		return ""
	}
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}
