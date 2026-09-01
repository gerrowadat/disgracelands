// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

// Package session owns a player's connection: reading lines from it, writing
// to it, and the login sequence that turns a socket into a character.
//
// Two goroutines per connection, neither of which touches the world. They
// hand parsed input to the engine and receive output on a buffered channel;
// everything about rooms and characters happens on the engine's goroutine.
// See docs/design/go-port-plan.md §3.1.
package session

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/gerrowadat/disgracelands/internal/colour"
	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/obs"
	"github.com/gerrowadat/disgracelands/internal/telnet"
)

// State is where a connection has got to.
//
// The names follow the C server's CON_* constants (structs.h:272) so the two
// can be read against each other. Only the states this phase implements are
// here; the menu, the editors and the deletion confirmations arrive with the
// features that need them.
type State int

const (
	// StateGetName: "By what name do you wish to be known?"
	StateGetName State = iota
	// StateConfirmName: a new character confirming their spelling.
	StateConfirmName
	// StatePassword: an existing character proving who they are.
	StatePassword
	// StateNewPassword and StateConfirmPassword: a new character setting one.
	StateNewPassword
	StateConfirmPassword
	// StateQuerySex and StateQueryClass complete the creation sequence,
	// matching the C's CON_QSEX and CON_QCLASS.
	StateQuerySex
	StateQueryClass
	// StateReadMOTD: press return, having read the message of the day.
	StateReadMOTD
	// StateMenu: the main menu, the C's CON_MENU. A character is not in the
	// world until they choose to enter it.
	StateMenu
	// StateEnterDescription: typing the description others see on `look`,
	// terminated by a lone '@'.
	StateEnterDescription
	// StateEditing is the general line editor, the C's string_write: collect
	// lines until a lone '@' and hand the text to whoever asked. Writing on a
	// bulletin board is the one thing that uses it so far, and mail will be
	// the second.
	StateEditing
	// StatePaging is the C's pager (page_string, modify.c:436): a long text
	// is shown one screenful at a time, and the connection waits for
	// RETURN/Q/R/B/a page number between pages rather than an ordinary
	// command.
	StatePaging
	// The three states of changing a password from the menu, matching
	// CON_CHPWD_GETOLD, CON_CHPWD_GETNEW and CON_CHPWD_VRFY.
	StateChangePasswordOld
	StateChangePasswordNew
	StateChangePasswordVerify
	// StateDeleteVerify and StateDeleteConfirm are the two confirmations the
	// C asks for before a character deletes itself.
	StateDeleteVerify
	StateDeleteConfirm
	// StatePlaying: in the world.
	StatePlaying
	// StateClosed: gone.
	StateClosed
)

// String names the state, for logs.
func (s State) String() string {
	switch s {
	case StateGetName:
		return "get-name"
	case StateConfirmName:
		return "confirm-name"
	case StatePassword:
		return "password"
	case StateNewPassword:
		return "new-password"
	case StateConfirmPassword:
		return "confirm-password"
	case StateQuerySex:
		return "query-sex"
	case StateQueryClass:
		return "query-class"
	case StateReadMOTD:
		return "read-motd"
	case StateMenu:
		return "menu"
	case StateEnterDescription:
		return "enter-description"
	case StateEditing:
		return "editing"
	case StatePaging:
		return "paging"
	case StateChangePasswordOld:
		return "change-password-old"
	case StateChangePasswordNew:
		return "change-password-new"
	case StateChangePasswordVerify:
		return "change-password-verify"
	case StateDeleteVerify:
		return "delete-verify"
	case StateDeleteConfirm:
		return "delete-confirm"
	case StatePlaying:
		return "playing"
	case StateClosed:
		return "closed"
	}
	return "?"
}

// outputQueue is how many pending writes a connection may have before it is
// considered unable to keep up.
//
// The C server grows an unbounded txt_q here, which is a way for one stuck
// client to exhaust the server's memory. A bounded queue turns that into one
// dropped connection.
const outputQueue = 256

// outgoing is one queued write.
//
// raw marks bytes that are already in wire form — telnet negotiation, GMCP —
// which must not be charset-converted or IAC-escaped on the way out. Escaping
// them would turn IAC WILL ECHO into a literal 0xFF followed by two bytes of
// junk data, which is precisely what happened when this was one flat queue of
// byte slices.
type outgoing struct {
	data []byte
	raw  bool
}

// Session is one player's connection.
type Session struct {
	id   uint64
	conn net.Conn
	// transport names how they arrived, for logs and the who-list.
	transport string
	host      string
	// loginTime is when the connection was accepted, which is the C's
	// `d->login_time`. Only `users` reads it, and it is the *connection's*
	// time rather than the character's: a descriptor sitting at the name
	// prompt has one too.
	loginTime time.Time

	// nextCommand is the earliest moment this connection may act again —
	// the C's one-command-per-pulse pacing, see pace. Owned by the read
	// goroutine and touched by nothing else, so it needs no lock.
	nextCommand time.Time

	out    chan outgoing
	logger *slog.Logger

	// owesPrompt marks output that has gone out with no prompt behind it —
	// the inverse of the C's `has_prompt` (comm.c). See PromptIfOwed.
	owesPrompt atomic.Bool

	// state is the connection state, and it is an atomic because it is not
	// only read by this session's own goroutine. `users` walks every
	// session and prints each one's state (users.go, do_users'
	// `connected_types[STATE(d)]`), and so does `show snoop`; both are
	// commands, so both read this from the world goroutine while the
	// session that owns it may be writing.
	//
	// #134 made that concrete — a plain int here produced a -race report
	// on the first run once the syslog echo started walking sessions.
	// The echo reads the C's PLR_WRITING flag instead now (#214), but the
	// two command call sites remain and the atomic with them.
	state atomic.Int32
	// character is atomic for the same reason state is, and it is the
	// third field of this shape to need it — state in #134, Record.Level
	// in #210, this one in #251, which is where the -race report is.
	//
	// Two goroutines write it and both are unavoidable. The session's own
	// goroutine sets it at the end of login and character creation
	// (login.go); the *world* goroutine sets it in SwitchInto/SwitchBack,
	// because `switch` and `return` are commands. And the world goroutine
	// reads it for a session that is not its own, in `users` (do_users
	// walks every descriptor) and `show snoop`, and in the dupe check on
	// behalf of a different connection entirely. So this is not a field
	// one call site can be moved off, the way #134 moved echoWizVis onto
	// w.Players(): the write is on the far side too.
	character atomic.Pointer[game.Character]
	// original is set while this session is switched into somebody else,
	// and is atomic for the same reason: SwitchInto writes it on the world
	// goroutine and perform_dupe_check reads it there for somebody else's
	// connection (internal/server/server.go's dupe pass).
	original atomic.Pointer[game.Character]
	// input is the command history `!`, `!<prefix>` and `^old^new` work
	// against — process_input's own, and readLoop's alone. See input.go.
	input inputHistory

	// snooping is the session this one is watching; snoopedBy is the session
	// watching this one. Guarded by mu because Send runs from the world
	// goroutine and the commands that set them run there too, but Close can
	// come from the connection goroutine.
	snooping  *Session
	snoopedBy *Session
	// snoopMu guards the two above.
	snoopMu sync.Mutex

	// pending holds what has been gathered so far during creation.
	pendingName     string
	pendingPassword string
	pendingSex      game.Sex

	// badPasswords is the C's `d->bad_pws` (structs.h:1019): wrong passwords
	// typed on *this connection*, which is what GameTuning.MaxBadPws is
	// measured against. It is deliberately not the character's own tally —
	// that one lives on the record, survives the disconnect, and is only
	// read to say "N LOGIN FAILURES SINCE LAST SUCCESSFUL LOGIN".
	badPasswords int32

	// editorBuf holds a multi-line entry — a description, or anything the
	// improved editor is open on — until its terminator arrives. It is
	// flat rather than a list of lines because the C's `*d->str` is, and
	// the improved editor's commands are defined against that: see
	// editor.go's editText, which also carries the C's own distinction
	// between an empty buffer and no buffer at all.
	editorBuf editText
	// editorMax and editorDone belong to StateEditing: the length limit and
	// what to do with the finished text. saved is false only for an
	// improved-editor /a (abort, improved-edit.c:39-40) — text is then
	// always "", the same as the C freeing *d->str on the way out
	// (modify.c:170).
	editorMax  int
	editorDone func(text string, saved bool)

	// pagerPages and pagerIndex belong to StatePaging: the whole text,
	// pre-split (paginate_string's own eager approach, not lazy), and
	// the C's own d->showstr_page — the *next* page to show, already
	// past every one shown so far.
	pagerPages []string
	pagerIndex int
	// pagerReturn is the state paging interrupted, restored when the last
	// page has been shown or the reader quits — this port's own way of
	// reproducing the fact that the C never changes STATE(d) while paging
	// at all (comm.c:811's showstr_count check runs before the state
	// switch). Captured by sendPaged from the state itself, so every caller
	// gets this for free.
	pagerReturn State

	// proto is the telnet state: options, charset, GMCP.
	proto protocol

	closed atomic.Bool
	quit   atomic.Bool
	rented atomic.Bool
	// extracted is set once the character has been taken out of the world by
	// extract_char while the connection stayed open — which is what `quit`
	// does. See MarkExtracted.
	extracted atomic.Bool
	// displaced is set when perform_dupe_check has taken this connection's
	// character away from it — see MarkDisplaced.
	displaced atomic.Bool
	closer    sync.Once
	// done is closed when the session ends. The output channel deliberately
	// is not: Send runs on whichever goroutine is talking to this player —
	// the world's, a shutdown watcher's, a timer's — and closing a channel
	// out from under a concurrent send is a panic, not a race that resolves
	// itself. Signalling on a separate channel lets the writer stop without
	// the senders having to synchronise with it.
	done chan struct{}
	// written is closed by the write loop when it has finished draining, so
	// the backstop close below cannot cut the last line off mid-flight.
	written chan struct{}
}

// MarkQuit records that the player left deliberately rather than losing
// their connection. The two are handled differently: a quitter is removed
// from the world, a dropped connection leaves the character standing so it
// can be reconnected to.
func (s *Session) MarkQuit() { s.quit.Store(true) }

// MarkRented records that this session ended by renting, so the disconnect
// handling knows the objects have already been dealt with.
//
// Without it the teardown crash-saves a character whose things have just been
// stored and extracted — which writes an empty file over the rent file, or
// deletes it, depending on which write lands last. The C has the same shape
// and no such problem, because extract_char does not crash-save.
func (s *Session) MarkRented() { s.rented.Store(true) }

// Rented reports whether the session ended at an inn.
func (s *Session) Rented() bool { return s.rented.Load() }

// MarkExtracted records that extract_char has already run for this
// connection's character: it is out of the world, saved and crash-saved, and
// the connection is sitting at the menu.
//
// This is the difference between the C's CON_MENU and CON_PLAYING in
// close_socket (comm.c:1956). A descriptor closed while playing gets "$n has
// lost $s link.", a save and a body left standing; a descriptor closed from
// the menu gets none of that, because there is nothing left to do — which is
// exactly the state `quit` leaves it in.
//
// Cleared again by entering the world, so a player who quits to the menu and
// then plays on is an ordinary session again.
func (s *Session) MarkExtracted()  { s.extracted.Store(true) }
func (s *Session) clearExtracted() { s.extracted.Store(false) }

// Extracted reports whether the character is already out of the world.
func (s *Session) Extracted() bool { return s.extracted.Load() }

// ReturnToMenu is extract_char_final's `STATE(ch->desc) = CON_MENU;
// write_to_output(ch->desc, "%s", MENU);` (handler.c:931).
//
// It is the whole difference between `quit` on the C server and `quit` here
// before #187: the connection stays open and the player is back at "Make your
// choice:", free to enter the game again without dialling in.
//
// The character stays attached — the C only frees it when there is no
// descriptor (handler.c:988) — so choosing 1 puts the same record back into
// the world.
func (s *Session) ReturnToMenu(menu string) {
	s.MarkExtracted()
	s.setState(StateMenu)
	s.Send("%s", menu)
}

// Quit reports whether the player left deliberately.
func (s *Session) Quit() bool { return s.quit.Load() }

// MarkDisplaced records that somebody has logged in as this connection's
// character and taken the body over, so the teardown must not touch it.
//
// This is `k->character = NULL` in perform_dupe_check (interpreter.c:1211,
// :1218) — the C nulls the old descriptor's pointer precisely so that
// closing it does not extract, save or crash-save a character that now
// belongs to somebody else. Doing it as a flag rather than by clearing
// s.character is not squeamishness: the dupe check runs on the world
// goroutine, on behalf of a *different* connection, and clearing the
// character there would be reaching into a session to change what it is,
// not merely to read it. An atomic flag says the same thing without that.
// (The field itself is an atomic.Pointer since #251, so the write would
// no longer be a *race* — it would still be the wrong thing to do.)
func (s *Session) MarkDisplaced() { s.displaced.Store(true) }

// Displaced reports whether this connection's character was taken over.
func (s *Session) Displaced() bool { return s.displaced.Load() }

// Deps are what a session needs from the rest of the server.
type Deps struct {
	Logger *slog.Logger
	// Text supplies the files the licence requires be shown, and the ones
	// players expect.
	Text TextFiles
	// Login performs the parts of the sequence that need the world or the
	// player store.
	Login LoginHandler
	// Commands dispatches what a playing character types.
	Commands CommandHandler
	// CommandInterval is how often a connection may act: one line per
	// interval, which is the C's one command per pulse. Zero disables the
	// pacing, which is what a unit test holding a Session directly wants.
	// See Session.pace.
	CommandInterval time.Duration
}

// TextFiles are the server's canned texts.
//
// The greeting and the credits are not decoration: the CircleMUD licence
// requires the login sequence to name the DikuMUD and CircleMUD creators and
// the credits to be displayed intact (docs/design/go-port-plan.md §12).
// They are a dependency of the session rather than a detail inside it so
// that no transport can be added which forgets them.
type TextFiles interface {
	Greeting() string
	MOTD() string
	// ImmortalMOTD is shown instead of MOTD to characters of immortal level,
	// as the C does at interpreter.c:1504.
	ImmortalMOTD() string
	Credits() string
	// Welcome and Start are WELC_MESSG and START_MESSG: the first shown to
	// everyone entering the game, the second only to a character doing it for
	// the first time.
	Welcome() string
	Start() string
	// Menu is the main menu shown after the message of the day, and again
	// after anything reached from it finishes.
	Menu() string
	// Background is the story behind menu choice 3.
	Background() string
	// The rest of the canned files, each behind its own command
	// (do_gen_ps). A missing one is empty rather than an error: the C ships
	// placeholders for most of them and a server with no news is quiet
	// rather than broken.
	News() string
	Info() string
	Policies() string
	Handbook() string
	WizList() string
	ImmList() string
	// HelpScreen is what bare `help` shows instead of a lookup
	// (HELP_PAGE_FILE, db.h:78).
	HelpScreen() string
	// Help is do_help's lookup (act.informative.c:966-988): the entry
	// text for a query, and whether anything matched.
	Help(query string) (string, bool)
}

// DupeMode is which of perform_dupe_check's outcomes happened, and decides
// what the new connection is told (interpreter.c:1281-1301).
type DupeMode int

const (
	// DupeNone: nobody else was logged in as this character. The ordinary
	// case, and the only one where the caller carries on into the menu.
	DupeNone DupeMode = iota
	// DupeReconnect is the C's RECON: a body left standing when a
	// connection dropped. "Reconnecting."
	DupeReconnect
	// DupeUsurp is the C's USURP: a body somebody was *playing* when this
	// login arrived. Their socket is closed and this one takes the body.
	DupeUsurp
	// DupeUnswitch is the C's UNSWITCH: the older connection was switched
	// into somebody else (do_switch), so what is handed back is the
	// character it switched *from*.
	DupeUnswitch
)

// LoginHandler performs the steps that need more than the connection.
type LoginHandler interface {
	// Exists reports whether a character is known.
	// BanFor reports how much of a site is banned: "" for not at all, or one
	// of "new", "select", "all". Checked at the name prompt, which is where
	// the C checks it — a banned site gets as far as being asked its name.
	BanFor(host string) string
	// DisallowedName reports whether name matches an entry in the xnames
	// list — Valid_Name's substring check (ban.c:255), separate from
	// BanFor's site check but consulted at the same CON_GET_NAME prompt.
	DisallowedName(name string) bool
	Exists(ctx context.Context, name string) (bool, error)
	// Authenticate checks a password and returns the character on success.
	// A nil character with a nil error means the password was wrong.
	Authenticate(ctx context.Context, name, password string) (*game.Character, error)
	// RecordBadPassword is nanny's `GET_BAD_PWS(d->character)++;
	// save_char(d->character);` (interpreter.c:1466-1467): the *persistent*
	// tally of consecutive failed logins, which the next successful login
	// reports and then clears. Separate from Session.badPasswords, which is
	// the C's per-connection `d->bad_pws` and is what max_bad_pws is
	// measured against.
	//
	// Best-effort by design: a failure to write it is logged by the
	// implementation and must not stop the login sequence, exactly as the C
	// ignores save_char's return.
	RecordBadPassword(ctx context.Context, name string)
	// NewCharactersAllowed is nanny's bare `if (circle_restrict)` at
	// CON_NAME_CNFRM (interpreter.c:1421): any wizlock at all stops a
	// character being made. Checked before the password prompt rather than
	// at Create, so the refusal is the C's own — "Sorry, new players can't
	// be created at the moment." — instead of a creation error the session
	// has to guess at.
	NewCharactersAllowed() bool
	// AllowedIn is the other half of the same global:
	// `GET_LEVEL(d->character) < circle_restrict` at CON_PASSWORD
	// (interpreter.c:1491), which turns an *existing* character away once
	// their password has been accepted. That is what a wizlock above 1
	// exists for, and until #211 nothing called it.
	AllowedIn(level int32) bool
	// Create makes a new character. The request carries everything the C's
	// creation sequence gathers before a character exists.
	Create(ctx context.Context, req CreateRequest) (*game.Character, error)
	// DupeCheck is perform_dupe_check (interpreter.c:1184), the last thing
	// nanny does once a password has been accepted (:1500).
	//
	// It answers one question — "is this player already here?" — and the
	// answer has three shapes, because there are three ways to be. A body
	// standing linkdead is reconnected to. A body somebody is actively
	// playing is *taken over*, and their connection closed. A connection
	// switched into something else is unswitched. In every case the older
	// connections are disconnected and any surplus bodies destroyed, so
	// that one character means one body and one socket.
	//
	// The implementation disconnects the losers and hands back the body to
	// adopt, or nil and DupeNone for an ordinary first login. Attaching to
	// it is the caller's half: the session owns its own state.
	DupeCheck(ctx context.Context, s *Session, c *game.Character) (*game.Character, DupeMode)
	// Enter puts an authenticated character into the world, and reports what
	// happened to the things they left with. The C's CON_MENU has one line
	// that depends on it (interpreter.c:1690).
	Enter(ctx context.Context, s *Session, c *game.Character) (EnterResult, error)
	// Leave takes them out again.
	Leave(ctx context.Context, s *Session, c *game.Character) error

	// CheckPassword verifies a password for a character already logged in.
	// The menu asks twice — before changing a password and before deleting a
	// character — and neither is a login, so neither goes through
	// Authenticate.
	CheckPassword(ctx context.Context, c *game.Character, password string) (bool, error)
	// SetPassword replaces a character's credential and saves it.
	SetPassword(ctx context.Context, c *game.Character, password string) error
	// Save writes a character's record back, for the menu's editors.
	Save(ctx context.Context, c *game.Character) error
	// Delete removes a character permanently.
	Delete(ctx context.Context, c *game.Character) error
}

// CommandHandler runs what a playing character types.
type CommandHandler interface {
	Do(ctx context.Context, s *Session, line string) error
	// InWorld runs f on the world goroutine and waits for it, the same
	// way Do runs a command.
	//
	// The line editor needs it. Everything a *command* touches is already
	// serialised by Do; the editor is the one thing a playing character
	// drives from the session's own goroutine, line by line, and its
	// cleanup writes world state — the PLR_WRITING bit, a board's message
	// list, a note's action description. In the C all of that runs inside
	// string_add, in the game loop, like everything else (modify.c:117).
	InWorld(ctx context.Context, f func(*game.Live)) error
}

// New wraps a connection in a session.
func New(id uint64, conn net.Conn, transport string, logger *slog.Logger) *Session {
	host, _, err := net.SplitHostPort(conn.RemoteAddr().String())
	if err != nil {
		host = conn.RemoteAddr().String()
	}
	s := &Session{
		id:        id,
		conn:      conn,
		transport: transport,
		host:      host,
		loginTime: time.Now(),
		out:       make(chan outgoing, outputQueue),
		done:      make(chan struct{}),
		written:   make(chan struct{}),
		logger:    logger.With("session", id, "transport", transport, "host", host),
		proto:     protocol{neg: telnet.NewNegotiator(policy)},
	}
	// StateGetName is zero, so this is not strictly needed — written out
	// anyway, because "the first state is whichever constant happens to
	// be zero" is not something a reader should have to check.
	s.setState(StateGetName)
	return s
}

// ID identifies the session in logs.
func (s *Session) ID() uint64 { return s.id }

// Transport names how the player connected.
func (s *Session) Transport() string { return s.transport }

// Host is the address they came from.
func (s *Session) Host() string { return s.host }

// LoginTime is when the connection was accepted — `d->login_time`. `users`
// prints the clock time of it, and nothing else wants it.
func (s *Session) LoginTime() time.Time { return s.loginTime }

// State returns where the session has got to.
func (s *Session) State() State { return State(s.state.Load()) }

// setState moves the connection to a new state. Every write goes through
// here; see the field's own note for why it is atomic.
func (s *Session) setState(st State) {
	s.state.Store(int32(st)) //nolint:gosec // State is a small enum, not a computed int
}

// Character returns the logged-in character, or nil.
func (s *Session) Character() *game.Character { return s.character.Load() }

// SetCharacter attaches a character to the session.
func (s *Session) SetCharacter(c *game.Character) { s.character.Store(c) }

// Original is the character this session belongs to when it has been
// switched into somebody else's body, or nil.
//
// The C keeps this on the descriptor as `d->original` and swaps
// `d->character` — so while switched, everything the session does happens as
// the *victim*, and `return` is the only way back. Note what that means for
// the level check on every command: a god switched into a rat is a rat, and
// the interpreter refuses them their own commands. The C has a message for
// exactly that case ("You can't use immortal commands while switched").
func (s *Session) Original() *game.Character { return s.original.Load() }

// SwitchedFromLevel answers game.Character.RealLevel: the level of the
// character this connection really belongs to, and whether it is switched at
// all.
//
// This is the whole of GET_REAL_LEVEL (utils.h:268), and `CAN_SEE` is its only
// consumer — a god switched into a rat still sees the invisible immortals
// their own level entitles them to. Everything else about a switched god uses
// the body's level.
func (s *Session) SwitchedFromLevel() (int32, bool) {
	if s == nil {
		return 0, false
	}
	original := s.original.Load()
	if original == nil {
		return 0, false
	}
	return original.Level(), true
}

// SwitchInto puts this session in charge of another character.
//
// On the world goroutine, like every command — which is why both fields it
// writes are atomic.
func (s *Session) SwitchInto(victim *game.Character) {
	if s.original.Load() == nil {
		s.original.Store(s.character.Load())
	}
	s.character.Store(victim)
	victim.Client = s
}

// SwitchBack undoes it, returning the character that was borrowed.
func (s *Session) SwitchBack() *game.Character {
	original := s.original.Load()
	if original == nil {
		return nil
	}
	borrowed := s.character.Load()
	if borrowed != nil && borrowed.Client == game.Client(s) {
		borrowed.Client = nil
	}
	s.character.Store(original)
	s.original.Store(nil)
	// original cannot be nil here: SwitchInto only stores it when it has a
	// character to store, and the early return above covers the rest.
	original.Client = s
	return borrowed
}

// SnoopOn makes this session a copy of everything written to another. A nil
// argument stops.
func (s *Session) SnoopOn(other *Session) {
	s.snoopMu.Lock()
	s.snooping = other
	s.snoopMu.Unlock()

	if other != nil {
		other.snoopMu.Lock()
		other.snoopedBy = s
		other.snoopMu.Unlock()
	}
}

// Snooping returns the session being watched, or nil.
func (s *Session) Snooping() *Session {
	s.snoopMu.Lock()
	defer s.snoopMu.Unlock()
	return s.snooping
}

// SnoopedBy returns the session watching this one, or nil.
func (s *Session) SnoopedBy() *Session {
	s.snoopMu.Lock()
	defer s.snoopMu.Unlock()
	return s.snoopedBy
}

// StopSnooping breaks the link from both ends.
func (s *Session) StopSnooping() {
	s.snoopMu.Lock()
	watched := s.snooping
	s.snooping = nil
	s.snoopMu.Unlock()

	if watched != nil {
		watched.snoopMu.Lock()
		if watched.snoopedBy == s {
			watched.snoopedBy = nil
		}
		watched.snoopMu.Unlock()
	}
}

// Send queues output. It never blocks: a client that cannot keep up is
// disconnected rather than allowed to stall the world.
func (s *Session) Send(format string, args ...any) {
	s.SendAt(colour.Normal, format, args...)
}

// SendPaged is Send for a text long enough to want page_string's pager —
// see pager.go.
func (s *Session) SendPaged(format string, args ...any) {
	s.sendPaged(colour.Normal, format, args...)
}

// SendAt is Send for a message that wants a colour level other than the
// ordinary one.
//
// The level is a *threshold*: the C writes `CCYEL(ch, C_CMP)` on the combat
// messages, so somebody who has asked for "normal" colour sees the fight in
// plain text and somebody who asked for "complete" sees it in yellow. Almost
// everything is C_NRM, which is why Send has it as the default and this exists
// for the handful that do not.
func (s *Session) SendAt(want colour.Level, format string, args ...any) {
	if s.closed.Load() {
		return
	}
	text := format
	if len(args) > 0 {
		text = fmt.Sprintf(format, args...)
	}
	text = colour.Render(text, want, s.colourLevel())
	s.sendRendered(text)
}

// sendRendered writes text that has already been through colour.Render —
// the common tail SendAt and the pager (pager.go) both need, factored out
// so pagination does not re-render each page (and double-count what
// next_page's own ANSI-skip logic would otherwise have to undo).
func (s *Session) sendRendered(text string) {
	// Every line that reaches a player goes through here, which is what
	// makes it the place to notice that one has gone out with no prompt
	// after it. Cleared again by sendPrompt. Telnet and GMCP bytes do not
	// come this way and do not count, which is right: the C's has_prompt
	// tracks its *output buffer*, not its negotiation.
	s.owesPrompt.Store(true)

	select {
	case s.out <- outgoing{data: []byte(normalise(text))}:
	case <-s.done:
	default:
		s.logger.Warn("output queue full; dropping the connection")
		s.Close()
	}

	// Anybody snooping sees it too. The C copies at the descriptor's write,
	// so a snooper sees everything including prompts — which is what makes
	// snooping useful and also what makes it noisy.
	//
	// Already rendered, so the snooper sees the colour the *snooped* character
	// would have seen rather than their own. That is the C's too: the copy
	// happens at write_to_descriptor, long after the macros have expanded.
	if watcher := s.SnoopedBy(); watcher != nil {
		watcher.Send("%s", text)
	}
}

// colourLevel is how much colour this session has asked for. A connection with
// no character yet — the greeting, the name prompt — gets none, which matches
// the C: every one of its macros takes a char_data and there is not one.
func (s *Session) colourLevel() colour.Level {
	c := s.Character()
	if c == nil || c.Record == nil || c.IsNPC() {
		return colour.Off
	}
	return colour.LevelOf(
		c.Record.Preferences.Has(game.PrefColour1),
		c.Record.Preferences.Has(game.PrefColour2),
	)
}

// SendRaw queues bytes without line-ending translation, for telnet control
// sequences — option negotiation, the ECHO toggle a password prompt uses,
// GMCP — which are always in wire form already.
//
// A websocket session never gets any of it. The browser terminal at the
// other end is a WebSocket text stream, not a telnet client — there is
// nobody to negotiate with, and IAC bytes arriving as if they were part of
// the game's own output would render as garbage rather than being
// understood, corrupting exactly the "looks like a telnet session"
// experience the web interface exists to give. Gating this single choke
// point rather than each call site means a telnet control sequence added
// here later is safe for a websocket session by construction, not by
// whoever adds it remembering to check.
//
// A websocket session's own echo signal (protocol.go's webEchoOff/OnMarker)
// is not a telnet control sequence and does not come through here — see
// sendRawAlways.
func (s *Session) SendRaw(b []byte) {
	if s.transport == "websocket" {
		return
	}
	s.sendRawAlways(b)
}

// sendRawAlways is SendRaw without the websocket gate: queues bytes
// exactly as given, for the one thing a websocket session still needs
// sent raw — its own private echo marker, which is meaningful to that
// transport specifically rather than being a telnet sequence it must not
// see.
func (s *Session) sendRawAlways(b []byte) {
	// Nothing to send is the ordinary answer from the negotiator — a request
	// already in flight owes no bytes — so callers pass its result straight
	// in rather than testing it every time.
	if len(b) == 0 || s.closed.Load() {
		return
	}
	select {
	case s.out <- outgoing{data: b, raw: true}:
	case <-s.done:
	default:
		s.logger.Warn("output queue full; dropping the connection")
		s.Close()
	}
}

// normalise converts bare newlines to CRLF.
//
// Telnet's line ending is CRLF and always has been, and the world files
// already contain it — see the parser's handling of fread_string. This
// fixes up text written in Go, without doubling the CRs already present.
func normalise(s string) string {
	if !strings.Contains(s, "\n") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 16)
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' && (i == 0 || s[i-1] != '\r') {
			b.WriteString("\r\n")
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// Close disconnects the session. It is safe to call more than once.
func (s *Session) Close() {
	s.closer.Do(func() {
		s.closed.Store(true)
		close(s.done)
		// Unblock the reader without closing the socket: the writer still has
		// to get the last line out. "Goodbye, friend.. Come back soon!" is
		// sent and then the session is closed in the same breath, so closing
		// the connection here would lose it.
		_ = s.conn.SetReadDeadline(time.Now())
	})
}

// Closed reports whether the session has been disconnected.
func (s *Session) Closed() bool { return s.closed.Load() }

// Serve runs the session until the connection ends or ctx is cancelled.
func (s *Session) Serve(ctx context.Context, deps Deps) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer s.Close()

	go s.writeLoop(ctx)

	// Offer the options before anything else is sent, so a client that speaks
	// GMCP has it on for the login sequence itself rather than from the first
	// prompt after it.
	s.offer()

	// The licence requires the login sequence to name the DikuMUD and
	// CircleMUD creators, and this is the login sequence. Every transport
	// reaches this line; none may skip it.
	//
	// Only the greeting is sent. The file ends with the name prompt itself —
	// that is where the C server's prompt comes from too, since it sends
	// GREETINGS on connect and nanny() prints nothing until input arrives.
	// Adding a prompt here would show it twice.
	s.Send("%s", deps.Text.Greeting())

	if err := s.readLoop(ctx, deps); err != nil && !isDisconnect(err) {
		s.logger.Info("session ended", "error", err)
	}

	// Not for a connection whose character has been taken over by a newer
	// login: the body is somebody else's now, and Leave would save it,
	// crash-save it and announce it as having lost its link. The worst of
	// those is the crash-save — a duplicate sitting at the menu carries
	// nothing, so it would write an empty rent file over the real one and
	// cost the player everything they owned.
	// Nor for one whose character `quit` has already extracted: it is out of
	// the world, saved and crash-saved, and the connection has been sitting
	// at the menu ever since. close_socket takes the same branch —
	// IS_PLAYING(d) is false at CON_MENU, so the C neither announces a lost
	// link nor saves again (comm.c:1956).
	if character := s.Character(); character != nil && !s.Displaced() && !s.Extracted() {
		if err := deps.Login.Leave(context.WithoutCancel(ctx), s, character); err != nil {
			s.logger.Error("removing the character from the world", "error", err)
		}
	} else if character != nil && !s.Displaced() {
		// close_socket's `else` (comm.c:1977-1979): a descriptor closing
		// with a character attached but not IS_PLAYING — which after #187
		// is the ordinary end of a session, the player having typed
		// `quit` and then hung up at the menu. CMP, so only an immortal
		// running syslog complete sees it; the "Closing link to" line
		// Server.Leave logs is the interesting one and it is NRM.
		//
		// close_socket's third branch, `mudlog("Losing descriptor without
		// char.", CMP, LVL_IMMORT, TRUE)` (comm.c:1982), has no
		// counterpart here and deliberately so: the C allocates
		// d->character at the *name* prompt, so that line means "a
		// connection that never typed anything", plus the displaced
		// descriptor whose pointer perform_dupe_check just nulled. This
		// port has no half-built character to lose — a session with no
		// s.character is a connection that never authenticated — so the
		// line would fire on every idle port-scan instead.
		wizlog(s.logger, obs.LogComplete, game.LevelImmortal,
			"Losing player: %s.", character.Name)
	}
	// Give the writer a moment to finish before the backstop close. Without
	// this, a session that ends by saying something — "Wrong password.", or
	// the goodbye — races the socket being shut under it, and the player sees
	// the connection drop with no explanation.
	select {
	case <-s.written:
	case <-time.After(2 * time.Second):
	}
	_ = s.conn.Close()
	s.logger.Info("disconnected")
}

// del is DEL, what a terminal's Backspace key actually sends. RFC 854 calls
// it Erase Character; the C tests for it as the bare 127 (comm.c:1787).
const del = 0x7f

// maxLineLength bounds the *buffer*, not the line.
//
// The line's own limit is MAX_INPUT_LENGTH, applied once the line is
// finished (input.go's truncateInput), because that is where the C applies
// it and because a player who typed too much has to be told so. This is the
// other bound, and it is a different job: a client that never sends a
// newline at all would otherwise grow this slice without limit, and there is
// nobody to tell.
const maxLineLength = 64 * 1024

// readLoop reads lines and dispatches them.
//
// Input passes through the telnet parser first, so negotiation never reaches
// the interpreter as text — which it does in the C, where a client offering
// window size has its NAWS bytes read as a command.
func (s *Session) readLoop(ctx context.Context, deps Deps) error {
	var (
		parser telnet.Parser
		buf    = make([]byte, 4096)
		line   []byte
		// lastEOL is the line ending just seen, so the other half of a CR LF
		// pair is swallowed rather than read as a second, empty line. It
		// lives out here because a pair can be split across two reads.
		lastEOL byte
	)

	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, err := s.conn.Read(buf)
		if n > 0 {
			data := parser.Feed(nil, buf[:n])

			for _, ev := range parser.Events() {
				s.handleTelnet(ev)
			}

			for _, b := range data {
				// A line ends in every form a client sends one. Telnet's NVT
				// line ending is CR LF (RFC 854), a client sending a bare
				// carriage return sends CR NUL, and a Unix one sends a lone
				// LF. All three are Enter and all three end the line here.
				//
				// CR NUL is not a curiosity: it is what telnet(1) sends for
				// the Enter key once it is in character-at-a-time mode, which
				// is the mode it enters as soon as this server offers
				// SUPPRESS-GO-AHEAD. Waiting only for LF meant Enter did
				// nothing at all in the most ordinary client there is.
				if b == '\r' || b == '\n' {
					// The second half of a CR LF pair — which may arrive in a
					// later read than its CR, so the state outlives the loop.
					if lastEOL != 0 && b != lastEOL {
						lastEOL = 0
						continue
					}
					lastEOL = b

					text := string(line)
					line = line[:0]

					// process_input's line stage, in its order: truncate,
					// copy to a snooper, then `!`/`^`. See input.go.
					if truncated, cut := truncateInput(text); cut {
						text = truncated
						s.Send("Line too long.  Truncated to:\r\n%s\r\n", text)
					}
					s.snoopInput(text)
					text, run := s.recallInput(text)
					if !run {
						// A failed `^old^new` has already been answered
						// and is not run — `if (!failed_subst)
						// write_to_q(...)`. The C's player still gets a
						// prompt, because the game loop writes one whether
						// a command ran or not; this port's prompt comes
						// out of the dispatcher, so an empty line through
						// it is how to ask for the same thing. Only while
						// playing: an empty line means something else
						// everywhere else, and at the name prompt it
						// closes the connection.
						if s.State() == StatePlaying {
							if handleErr := s.handle(ctx, deps, ""); handleErr != nil {
								return handleErr
							}
						}
						continue
					}

					// perform_alias (comm.c:803), run once per line actually
					// read off the socket rather than once per command: a
					// complex alias expands to several, run here in order,
					// before anything further is read — the same effect as
					// the C pushing them to the front of the input queue. See
					// alias.go's expandAliasedLine.
					//
					// After the history stage, not before: the C queues the
					// substituted line and perform_alias runs on what comes
					// back off the queue, so `!` recalls what was typed and
					// the alias expands what `!` produced.
					for _, part := range s.expandAliasedLine(text) {
						if handleErr := s.handle(ctx, deps, part); handleErr != nil {
							return handleErr
						}
						if s.State() == StateClosed || s.closed.Load() {
							return nil
						}
					}
					continue
				}
				// NUL is never data. RFC 854 sends it as the filler after a
				// bare CR, and says to ignore it wherever it appears.
				if b == 0 {
					lastEOL = 0
					continue
				}
				// Erase, which the C does too and this loop did not.
				//
				// process_input's copy loop drops a backspace or a DEL and
				// takes back the character before it (comm.c:1787, and
				// CircleMUD's own comm.c:1712 — the comment there reads
				// "handle backspacing or delete key"). It is the C server's
				// only line discipline, and it is there because a client that
				// has gone character-at-a-time sends every keystroke as it is
				// typed, backspace included: nothing between the keyboard and
				// the interpreter has edited the line, so this is where the
				// editing has to happen.
				//
				// A line-mode telnet client never sends these — its terminal
				// driver does the editing and only the finished line leaves the
				// client — which is why the gap went unnoticed here for so long.
				// But this server agrees to SUPPRESS-GO-AHEAD the moment a
				// client asks for it (protocol.go), and the browser terminal
				// sends keystroke-at-a-time unconditionally
				// (internal/server/web_templates.go: the pager depends on a
				// keypress arriving without an Enter after it), with no driver
				// of its own to edit for it. A player there who typed
				// "Newcomerr", noticed the second r and erased it, saw
				// "Newcomer" on screen while the server read "Newcomerr\x7f":
				// the erase was cosmetic, and only ever on the client. Issue
				// #233.
				//
				// Erasing nothing is not an error — `write_point > tmp` guards
				// the same case in the C — it is what backspace at the start of
				// a line does everywhere.
				//
				// One *rune*, and this is where it parts company with the
				// C. process_input drops everything that is not
				// `isascii && isprint` a few lines further on
				// (comm.c:1796), so a multi-byte character could never
				// reach the buffer it is erasing from and one byte was
				// always one character there. This port has no such
				// filter and takes UTF-8 -- invalidName reads a name with
				// unicode.IsLetter -- so erasing a byte leaves a broken
				// rune behind: "Zoë", backspace, "e" became "Zo\xc3e",
				// and the login answered "Names may only contain
				// letters." DecodeLastRune returns (RuneError, 1) for a
				// trailing malformed byte, which erases exactly that byte
				// and is the right answer for it too.
				if b == del || b == '\b' {
					lastEOL = 0
					if len(line) > 0 {
						_, size := utf8.DecodeLastRune(line)
						line = line[:len(line)-size]
					}
					continue
				}
				// The `isascii(*ptr) && isprint(*ptr)` filter
				// (comm.c:1796): anything else is dropped rather than
				// copied, so a control character never reaches a command.
				//
				// Half of it, and deliberately. The C's test excludes
				// every byte above 127 as well, which this port cannot
				// adopt: it takes UTF-8 on purpose — invalidName reads a
				// name with unicode.IsLetter, and the backspace above
				// erases a rune because of it — so `isascii` would throw
				// away exactly the characters that support is for. The
				// `isprint` half has nothing to do with encoding and
				// everything to do with what a terminal sends by accident:
				// an escape sequence typed at the name prompt arrived as
				// ESC, `[` and `A` and was read as command text, which is
				// why the browser has to swallow arrow keys before they
				// are sent at all (#235). ESC is dropped here now; `[A`
				// still is not, exactly as in the C.
				//
				// NUL, CR, LF and DEL are all handled above, so what is
				// left to drop is the rest of the C0 controls — tab
				// included, which `isprint` excludes too.
				if b < 0x20 {
					lastEOL = 0
					continue
				}
				lastEOL = 0
				if len(line) < maxLineLength {
					line = append(line, b)
				}
			}
		}
		if err != nil {
			return err
		}
	}
}

// writeLoop drains queued output to the connection.
func (s *Session) writeLoop(ctx context.Context) {
	defer close(s.written)
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.done:
			// Drain whatever is already queued, so a goodbye written just
			// before the close still reaches the player, and only then hang
			// up.
			s.flush()
			_ = s.conn.Close()
			return
		case msg := <-s.out:
			b := msg.data
			if !msg.raw {
				b = s.encodeOutbound(b)
			}
			// A write that stalls forever holds a goroutine and a socket; a
			// deadline turns that into a disconnect.
			_ = s.conn.SetWriteDeadline(time.Now().Add(30 * time.Second))
			if _, err := s.conn.Write(b); err != nil {
				if !isDisconnect(err) {
					s.logger.Debug("write failed", "error", err)
				}
				s.Close()
				return
			}
		}
	}
}

func isDisconnect(err error) bool {
	return err == nil ||
		errors.Is(err, io.EOF) ||
		errors.Is(err, net.ErrClosed) ||
		errors.Is(err, context.Canceled)
}

// CreateRequest is everything gathered before a character exists.
type CreateRequest struct {
	Name     string
	Password string
	Sex      game.Sex
	Class    game.Class
}

// flush writes whatever is already queued and gives up on the rest.
//
// It is what makes "Goodbye, friend.. Come back soon!" arrive: the command
// that sends it closes the session immediately afterwards, so without a drain
// the message would still be sitting in the queue.
func (s *Session) flush() {
	for {
		select {
		case msg := <-s.out:
			b := msg.data
			if !msg.raw {
				b = s.encodeOutbound(b)
			}
			_ = s.conn.SetWriteDeadline(time.Now().Add(time.Second))
			if _, err := s.conn.Write(b); err != nil {
				return
			}
		default:
			return
		}
	}
}

// connectedNames are `connected_types[]` (constants.c:226), indexed by the
// C's CON_* value rather than by this port's State.
//
// The two orders are not the same and there is no reason they should be: the C
// numbers its states by where somebody happened to add them, and this port
// numbers them by the order a player passes through. `users` prints these
// words, so the mapping has to be explicit — and a test re-parses constants.c
// and checks it, because a table transcribed by eye is how a state ends up
// labelled as its neighbour.
//
// The nolint is for gosec, which reads "Get password" as a hardcoded
// credential. They are the C's labels for a prompt, not a secret.
var connectedNames = map[State]string{ //nolint:gosec // prompt labels, not credentials
	StateGetName:              "Get name",
	StateConfirmName:          "Confirm name",
	StatePassword:             "Get password",
	StateNewPassword:          "Get new PW",
	StateConfirmPassword:      "Confirm new PW",
	StateQuerySex:             "Select sex",
	StateQueryClass:           "Select class",
	StateReadMOTD:             "Reading MOTD",
	StateMenu:                 "Main Menu",
	StateEnterDescription:     "Get descript.",
	StateChangePasswordOld:    "Changing PW 1",
	StateChangePasswordNew:    "Changing PW 2",
	StateChangePasswordVerify: "Changing PW 3",
	StateDeleteVerify:         "Self-Delete 1",
	StateDeleteConfirm:        "Self-Delete 2",
	StatePlaying:              "Playing",
	StateClosed:               "Disconnecting",
	// StateEditing has no CON_ of its own: the C reaches its line editor
	// through string_write from whatever state asked for it, so a player
	// writing on a board is still shown as whatever they were. "Get descript."
	// is the closest thing the C would print, since that is the state its own
	// editor runs in.
	StateEditing: "Get descript.",
	// StatePaging has no CON_ of its own either — the C's showstr_count
	// check runs *before* the STATE(d) switch entirely (comm.c:811), so
	// STATE(d) never changes while paging at all, and `users` would still
	// show whatever state paging interrupted. This entry is only the
	// fallback for State.ConnectedName's own pure, session-less callers
	// (the coverage test below among them); users.go calls
	// Session.ConnectedName instead, which reads pagerReturn and shows
	// the *real* interrupted state — "Playing" for the ordinary case,
	// since every paginated command but `background` runs from
	// StatePlaying, but "Reading MOTD" while `background`'s own page is
	// open.
	StatePaging: "Playing",
}

// ConnectedName is what `users` calls this state, as a pure function of
// the enum alone — see Session.ConnectedName for the version that knows
// which state StatePaging actually interrupted.
func (s State) ConnectedName() string {
	if name, ok := connectedNames[s]; ok {
		return name
	}
	return "Unknown"
}

// ConnectedName is `users`' own lookup: State.ConnectedName, except for
// StatePaging, which reports whatever state paging interrupted
// (pagerReturn) rather than the fallback "Playing" every other caller of
// State.ConnectedName has to live with.
func (s *Session) ConnectedName() string {
	if s.State() == StatePaging {
		return s.pagerReturn.ConnectedName()
	}
	return s.State().ConnectedName()
}

// pace holds a connection to one command per interval, porting the C's
// input pacing (comm.c:806-830).
//
// game_loop takes **one** command off a descriptor's queue per pass and then
// does this:
//
//	GET_WAIT_STATE(d->character) = 1;
//
// so the next pass spends that 1 decrementing back to zero and only the pass
// after it can dequeue again. At OPT_USEC's 100ms that is a ceiling of ten
// commands a second, whatever anybody types or pastes — the classic Diku
// pacing, and a flood limit as much as a rhythm. This port had none: it ran
// commands as fast as the socket delivered them, five `look`s in 151ms
// against the C's 603ms (#386).
//
// **It does not stack with a skill's own lag, and does not need to.** The C
// sets the 1 *before* running the command, so `kick` overwrites it with
// three rounds and the larger simply wins. Here the same thing falls out of
// the two being measured differently: this is an absolute moment and
// Character.BusyUntil is another, so a 100ms pace inside a two-second wait
// is absorbed by it rather than added to it.
//
// Reading is not what is paced, in either server. process_input runs every
// pass regardless, so a player can always type ahead; what is rationed is
// how fast the queued lines are spent. This port's read loop does block
// while a line is being handled, so the type-ahead sits in the socket buffer
// rather than in a queue of the server's own — the same effect from the
// player's side, and the reason nothing here needs a queue.
//
// # Who is paced
//
// The C's guard is `if (d->character)`, and so is this: a connection with no
// character yet is not paced. The two disagree about *when* that becomes
// true, because the C allocates a char_data at CON_GET_NAME and this port
// does not build one until the login succeeds (docs/deviations.md — it is
// why there is no "Losing descriptor without char"). So the C paces the name
// and password lines and this does not, which is a difference of a few
// hundred milliseconds during login and nothing else.
func (s *Session) pace(ctx context.Context, every time.Duration) error {
	if every <= 0 || s.Character() == nil {
		return nil
	}

	if wait := time.Until(s.nextCommand); wait > 0 {
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	s.nextCommand = time.Now().Add(every)
	return nil
}

// sendPrompt writes the prompt and settles the debt sendRendered records.
//
// Every prompt in the game goes through here for one reason: a prompt is
// itself output, so a version that did not clear the flag would owe another
// prompt for having sent one, and PromptIfOwed would send a fresh prompt
// every pulse forever.
//
// **On the world goroutine only.** prompt() reads hit points, mana and
// movement off the live record.
func (s *Session) sendPrompt() {
	s.Send("%s", prompt(s))
	s.owesPrompt.Store(false)
}

// PromptIfOwed gives this connection a prompt if something has been said to
// it since the last one, porting the half of game_loop that has no counterpart
// in a command (#385).
//
// The C sends a prompt after *anything* that writes to you, not only after
// something you typed. process_output appends make_prompt to whatever it is
// flushing (comm.c:1469), so every unsolicited line — somebody speaking, a
// round of combat, a tick — arrives with a fresh prompt behind it, and the
// loop after it prints one to any descriptor that still has none
// (comm.c:865-869).
//
// That matters more than it sounds, because the prompt is where your hit
// points are. On the real server a fight is a stream of damage messages each
// followed by an updated `22H 100M 85V >`, which is how you know when to run
// — and what `wimpy` (#375) exists to automate. Without this the numbers
// froze at whatever they were when you last typed something.
//
// Called once a pulse for every connection, which is the same rhythm the C
// flushes on. It sends nothing when nothing has been said, and nothing when
// prompt() itself is empty — which is the C's answer at the menu and
// anywhere else that is not playing, editing or paging (docs/deviations.md,
// "No prompt at the menu").
//
// **On the world goroutine only**, for sendPrompt's reason.
func (s *Session) PromptIfOwed() {
	if s.Closed() || !s.owesPrompt.Load() {
		return
	}
	if prompt(s) == "" {
		// Nothing to send, but the debt is settled either way: a menu that
		// owes a prompt it has no way to write would otherwise be asked
		// again every pulse for the rest of the connection.
		s.owesPrompt.Store(false)
		return
	}
	s.sendPrompt()
}
