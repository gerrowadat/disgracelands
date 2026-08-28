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

	"github.com/gerrowadat/disgracelands/internal/colour"
	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/obs"
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
	switch s.State() {
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
	case StatePaging:
		return s.handlePaging(line)
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
	//
	// SELECT is not here: reading the C closely (interpreter.c:1482-1490)
	// found it is checked much later than this comment used to say — at
	// CON_PASSWORD, after a password has already been verified, against
	// the loaded character's own PLR_SITEOK bit, not at the name prompt at
	// all. handlePassword is where that lives now.
	switch deps.Login.BanFor(s.Host()) {
	case "all":
		s.Send("You are not welcome here.\r\n")
		s.logger.Info("refused a banned site", "host", s.Host(), "ban", "all")
		// mudlog(buf2, CMP, LVL_GOD, TRUE) (comm.c:1397-1398). The C
		// refuses a BAN_ALL site in new_descriptor, before a byte is read,
		// so its line names only the host; this port refuses at the name
		// prompt instead, and keeps the C's text rather than adding the
		// name it now happens to know.
		wizlog(s.logger, obs.LogComplete, game.LevelGod,
			"Connection attempt denied from [%s]", s.Host())
		s.Close()
		return nil
	case "new":
		if !exists {
			s.Send("Sorry, new characters are not allowed from your site!\r\n")
			s.logger.Info("refused a new character from a banned site", "host", s.Host())
			// mudlog(buf, NRM, LVL_GOD, TRUE) (interpreter.c:1414-1416),
			// which the C reaches at CON_NAME_CNFRM — one prompt later
			// than this, after the "Did I get that right?" — so the name
			// it prints is one the player has already confirmed. Same
			// refusal either way; the message is the C's.
			wizlog(s.logger, obs.LogNormal, game.LevelGod,
				"Request for new char %s denied from [%s] (siteban)", name, s.Host())
			s.Close()
			return nil
		}
	}

	s.pendingName = name
	if exists {
		s.setState(StatePassword)
		s.EchoOff()
		s.Send("Password: ")
		return nil
	}
	s.setState(StateConfirmName)
	s.Send("Did I get that right, %s (Y/N)? ", name)
	return nil
}

func (s *Session) handleConfirmName(deps Deps, line string) error {
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		s.setState(StateNewPassword)
		s.EchoOff()
		s.Send("New character.\r\nGive me a password for %s: ", s.pendingName)
	case "n", "no":
		s.pendingName = ""
		s.setState(StateGetName)
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

	// An empty line hangs up, silently and without counting as a strike
	// (`if (!*arg) STATE(d) = CON_CLOSE;`, interpreter.c:1459-1460). It is
	// checked before the password is, so somebody who just presses return is
	// gone rather than being told anything — and, more to the point here,
	// cannot use empty lines to keep a connection at the prompt for ever.
	if password == "" {
		s.Close()
		return nil
	}

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
		s.logger.Info("failed login", "name", s.pendingName, "host", s.Host())
		// mudlog(buf, BRF, LVL_GOD, TRUE) (interpreter.c:1463-1464). The
		// C only reaches this with a *known* name and a wrong password;
		// this port cannot tell the two apart at this point on purpose
		// (see above), so an unknown name logs the same line. That is a
		// deliberate widening, not a mis-port.
		wizlog(s.logger, obs.LogBrief, game.LevelGod,
			"Bad PW: %s [%s]", s.pendingName, s.Host())

		// Two counters, and they are not the same counter — this is the
		// thing the C is easy to misread on. The character's own tally is
		// persistent, survives the disconnect, and exists only so the next
		// successful login can say how many attempts there were. `d->bad_pws`
		// belongs to the socket, starts at zero on every fresh connection,
		// and is the one max_bad_pws is compared against — so three wrong
		// passwords disconnect you, and dialling back in gives you three
		// more (interpreter.c:1466-1474).
		deps.Login.RecordBadPassword(ctx, s.pendingName)

		s.badPasswords++
		if s.badPasswords >= game.Tuning().MaxBadPws {
			s.Send("Wrong password... disconnecting.\r\n")
			s.Close()
			return nil
		}
		// Still at the password prompt, so echo goes back off: the EchoOn
		// above was for the line just typed, not for the next one.
		s.Send("Wrong password.\r\n")
		s.EchoOff()
		s.Send("Password: ")
		return nil
	}

	// Password was correct, so the socket's own strike count goes back to
	// zero (interpreter.c:1480). The character's persistent tally is left
	// alone until reportLoginFailures has had a chance to say what it was.
	s.badPasswords = 0

	// The SELECT ban (interpreter.c:1482-1490): once the password is right,
	// a site-banned connection is still refused unless this particular
	// character carries PLR_SITEOK. Every new character gets that bit for
	// free (game.ApplyNewCharacterDefaults, "Sometimes siteok is off for new
	// players" — interpreter.c:1623, a Disgracelands `<DoC>` addition), so
	// this only ever bites an older record nobody has cleared, exactly as
	// intended — `set <name> siteok` is how an immortal clears one.
	if deps.Login.BanFor(s.Host()) == "select" && !siteOK(character) {
		s.logger.Info("refused a select-banned site", "host", s.Host(), "character", character.Name)
		// mudlog(buf, NRM, LVL_GOD, TRUE) (interpreter.c:1486-1488) —
		// and note it is `denied from %s`, no brackets round the host,
		// unlike every other line in nanny.
		wizlog(s.logger, obs.LogNormal, game.LevelGod,
			"Connection attempt for %s denied from %s", character.Name, s.Host())
		s.Send("Sorry, this char has not been cleared for login from your site!\r\n")
		s.Close()
		return nil
	}

	// perform_dupe_check, the last thing nanny does with an accepted
	// password (interpreter.c:1500). If this player is already here in any
	// sense — a body left standing by a dropped link, a body somebody is
	// playing right now, a connection switched into something else — this
	// is where that is resolved into one body on one socket, and the older
	// sockets are disconnected. `character`, the record just loaded from
	// disk, is dropped on the floor in that case: the body already in the
	// world is the live one, and it is the one with the last half hour of
	// play in it. (The C is explicit about this — `free_char(d->character)`
	// before `d->character = target`, interpreter.c:1272.)
	if existing, mode := deps.Login.DupeCheck(ctx, s, character); existing != nil {
		s.logger.Info("taking over an existing body",
			"character", existing.Name, "mode", mode)
		s.character = existing
		s.setState(StatePlaying)

		// The three mudlogs of do_perform_dupe_check's tail, one per
		// mode, all NRM at MAX(LVL_IMMORT, GET_INVIS_LEV(d->character))
		// (interpreter.c:1286, 1295, 1300). RECON and UNSWITCH share a
		// line and USURP has its own — which is the whole point of
		// having three: a god watching wants to know whether somebody
		// walked back in or shoved somebody else out.
		switch mode {
		case DupeUsurp:
			s.Send("You take over your own body, already in use!\r\n")
			wizlog(s.logger, obs.LogNormal, wizlogLevel(game.LevelImmortal, existing),
				"%s has re-logged in ... disconnecting old socket.", existing.Name)
		case DupeUnswitch:
			s.Send("Reconnecting to unswitched char.")
			wizlog(s.logger, obs.LogNormal, wizlogLevel(game.LevelImmortal, existing),
				"%s [%s] has reconnected.", existing.Name, s.Host())
		default:
			s.Send("Reconnecting.\r\n")
			wizlog(s.logger, obs.LogNormal, wizlogLevel(game.LevelImmortal, existing),
				"%s [%s] has reconnected.", existing.Name, s.Host())
		}
		return deps.Commands.Do(ctx, s, "look")
	}

	s.character = character
	s.setState(StateReadMOTD)
	s.Send("%s\r\n", motdFor(deps, character))
	// mudlog(buf, BRF, MAX(LVL_IMMORT, GET_INVIS_LEV(d->character)), TRUE)
	// (interpreter.c:1508-1509), after the MOTD is queued and before the
	// login-failure notice. BRF, so it reaches an immortal running the
	// lowest syslog setting there is — this is the line gods actually
	// watch for.
	wizlog(s.logger, obs.LogBrief, wizlogLevel(game.LevelImmortal, character),
		"%s [%s] has connected.", character.Name, s.Host())
	s.reportLoginFailures()
	s.Send("*** PRESS RETURN: ")
	return nil
}

// reportLoginFailures is interpreter.c:1511-1518, the notice that follows the
// MOTD when somebody has been guessing at your password.
//
// The count is the character's own persistent tally, cleared here — the C
// reads it into `load_result` at :1478 and zeroes it there too, then zeroes
// it a second time inside this block. The reset is not saved on its own; the
// record is written on the way into the world and on every save after, which
// is what makes the tally "since last successful login" rather than
// "for ever".
//
// C_SPR, so the red is there for anybody who asked for any colour at all,
// and three bells, which the C really does mean.
func (s *Session) reportLoginFailures() {
	rec := s.character.Record
	if rec == nil || rec.BadPasswords <= 0 {
		return
	}
	plural := ""
	if rec.BadPasswords > 1 {
		plural = "S"
	}
	s.SendAt(colour.Sparse, "\r\n\r\n\007\007\007{{red}}%d LOGIN FAILURE%s SINCE LAST SUCCESSFUL LOGIN.{{/}}\r\n",
		rec.BadPasswords, plural)
	rec.BadPasswords = 0
}

// siteOK is PLR_FLAGGED(d->character, PLR_SITEOK), guarding against a
// character loaded with no record at all — the same defensiveness every
// other Record-reading helper in this tree already has.
func siteOK(c *game.Character) bool {
	return c != nil && c.Record != nil && c.Record.PlayerFlags.Has(game.PlayerSiteOK)
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
	s.setState(StateConfirmPassword)
	s.Send("\r\nPlease retype password: ")
	return nil
}

func (s *Session) handleConfirmPassword(deps Deps, line string) error {
	if strings.TrimSpace(line) != s.pendingPassword {
		s.pendingPassword = ""
		s.setState(StateNewPassword)
		s.Send("\r\nPasswords don't match.\r\nGive me a password for %s: ", s.pendingName)
		return nil
	}
	s.EchoOn()
	// The C asks sex next, then class. Same order, same wording.
	s.setState(StateQuerySex)
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
	s.setState(StateQueryClass)
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
	s.setState(StateReadMOTD)
	// mudlog(buf, NRM, LVL_IMMORT, TRUE) (interpreter.c:1629). The C
	// builds this string at :1606 and then, in the `<DoC>` block that
	// follows, reuses `buf` for the "All hail" broadcast — so the line
	// that actually reaches the log there is the *broadcast*, not "new
	// player", and no immortal ever saw the message this call site was
	// written to send. See docs/weirdnumbers.md; the port logs what the
	// call site meant.
	wizlog(s.logger, obs.LogNormal, game.LevelImmortal,
		"%s [%s] new player.", character.Name, s.Host())
	// **The mortal message of the day, unconditionally**, even for the first
	// character on the roster — who `init_char` has just made an implementor.
	//
	// The C has two paths and only one of them checks: an existing character
	// logging in gets `imotd` if they are immortal (interpreter.c:1503), and a
	// character who has *just been created* gets `motd` whatever their level
	// (:1603), one line after init_char set it to 34. So the founding
	// implementor sees the mortal file on the day they are made and the
	// immortal one every time after.
	//
	// Found by the session-parity harness on its first run, which is the sort
	// of thing it is for: nothing about the code looks wrong, and the two
	// servers simply said different things.
	s.Send("\r\n%s\r\n*** PRESS RETURN: ", deps.Text.MOTD())
	return nil
}

// handleReadMOTD is the return pressed after the message of the day, and
// after the background story. Either way it leads to the menu, which is what
// the C's CON_RMOTD does.
func (s *Session) handleReadMOTD(deps Deps) error {
	s.setState(StateMenu)
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
