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

// minPasswordLength is what a new password must be.
//
// The C server enforced three characters. That was a reasonable floor when
// the hash truncated to eight anyway; now that a password is stored under
// argon2id and used in full, there is no reason to keep it that low. Old
// characters are unaffected: this applies only to passwords being set.
const minPasswordLength = 6

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
		return s.handleConfirmPassword(ctx, deps, line)
	case StateReadMOTD:
		return s.handleReadMOTD(ctx, deps)
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

	name = capitalise(name)
	exists, err := deps.Login.Exists(ctx, name)
	if err != nil {
		s.logger.Error("looking up a character", "name", name, "error", err)
		s.Send("Something went wrong looking you up. Try again shortly.\r\n")
		s.Close()
		return nil
	}

	s.pendingName = name
	if exists {
		s.state = StatePassword
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

	s.character = character
	s.state = StateReadMOTD
	s.Send("%s\r\n*** PRESS RETURN: ", deps.Text.MOTD())
	return nil
}

func (s *Session) handleNewPassword(deps Deps, line string) error {
	password := strings.TrimSpace(line)
	if len(password) < minPasswordLength {
		s.Send("Passwords must be at least %d characters.\r\nPassword: ", minPasswordLength)
		return nil
	}
	if strings.EqualFold(password, s.pendingName) {
		// The C server refuses this too, and it remains good advice.
		s.Send("Illegal password.\r\nPassword: ")
		return nil
	}
	s.pendingHash = password
	s.state = StateConfirmPassword
	s.Send("Please retype password: ")
	return nil
}

func (s *Session) handleConfirmPassword(ctx context.Context, deps Deps, line string) error {
	if strings.TrimSpace(line) != s.pendingHash {
		s.pendingHash = ""
		s.state = StateNewPassword
		s.Send("Passwords don't match.\r\nGive me a password for %s: ", s.pendingName)
		return nil
	}

	character, err := deps.Login.Create(ctx, s.pendingName, s.pendingHash)
	s.pendingHash = ""
	if err != nil {
		s.logger.Error("creating a character", "name", s.pendingName, "error", err)
		s.Send("Something went wrong creating your character. Try again shortly.\r\n")
		s.Close()
		return nil
	}

	s.character = character
	s.state = StateReadMOTD
	s.Send("\r\n%s\r\n*** PRESS RETURN: ", deps.Text.MOTD())
	return nil
}

func (s *Session) handleReadMOTD(ctx context.Context, deps Deps) error {
	if err := deps.Login.Enter(ctx, s, s.character); err != nil {
		s.logger.Error("entering the world", "error", err)
		s.Send("Something went wrong putting you into the world. Try again shortly.\r\n")
		s.Close()
		return nil
	}
	s.state = StatePlaying
	s.logger.Info("entered the world", "character", s.character.Name)

	// Show them where they are, which is what the C server does on entry.
	return deps.Commands.Do(ctx, s, "look")
}

// invalidName reports why a name cannot be used, or "".
//
// The C server checks length and that every character is a letter
// (interpreter.c's _parse_name). The same rules are kept, with the reasons
// spelled out rather than the C's single "Illegal name" — a player who typed
// an apostrophe deserves to know that is the problem.
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
