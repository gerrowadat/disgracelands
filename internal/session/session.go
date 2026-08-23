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
// See docs/proposals/go-port-plan.md §3.1.
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

	"github.com/gerrowadat/disgracelands/internal/colour"
	"github.com/gerrowadat/disgracelands/internal/game"
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

	out    chan outgoing
	logger *slog.Logger

	state     State
	character *game.Character
	// original is set while this session is switched into somebody else.
	original *game.Character
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
	pendingSex      int32

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
	// switch). Captured by sendPaged from s.state itself, so every caller
	// gets this for free.
	pagerReturn State

	// proto is the telnet state: options, charset, GMCP.
	proto protocol

	closed atomic.Bool
	quit   atomic.Bool
	rented atomic.Bool
	closer sync.Once
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

// Quit reports whether the player left deliberately.
func (s *Session) Quit() bool { return s.quit.Load() }

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
}

// TextFiles are the server's canned texts.
//
// The greeting and the credits are not decoration: the CircleMUD licence
// requires the login sequence to name the DikuMUD and CircleMUD creators and
// the credits to be displayed intact (docs/proposals/go-port-plan.md §12).
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
	// Create makes a new character. The request carries everything the C's
	// creation sequence gathers before a character exists.
	Create(ctx context.Context, req CreateRequest) (*game.Character, error)
	// Reconnect returns a character already in the world under this name,
	// whose connection has dropped, or nil. The C keeps a linkdead body
	// standing rather than removing it, so a dropped connection can be
	// resumed; this is how the login sequence finds it again.
	Reconnect(ctx context.Context, name string) *game.Character
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
}

// New wraps a connection in a session.
func New(id uint64, conn net.Conn, transport string, logger *slog.Logger) *Session {
	host, _, err := net.SplitHostPort(conn.RemoteAddr().String())
	if err != nil {
		host = conn.RemoteAddr().String()
	}
	return &Session{
		id:        id,
		conn:      conn,
		transport: transport,
		host:      host,
		loginTime: time.Now(),
		out:       make(chan outgoing, outputQueue),
		done:      make(chan struct{}),
		written:   make(chan struct{}),
		logger:    logger.With("session", id, "transport", transport, "host", host),
		state:     StateGetName,
		proto:     protocol{neg: telnet.NewNegotiator(policy)},
	}
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
func (s *Session) State() State { return s.state }

// Character returns the logged-in character, or nil.
func (s *Session) Character() *game.Character { return s.character }

// SetCharacter attaches a character to the session.
func (s *Session) SetCharacter(c *game.Character) { s.character = c }

// Original is the character this session belongs to when it has been
// switched into somebody else's body, or nil.
//
// The C keeps this on the descriptor as `d->original` and swaps
// `d->character` — so while switched, everything the session does happens as
// the *victim*, and `return` is the only way back. Note what that means for
// the level check on every command: a god switched into a rat is a rat, and
// the interpreter refuses them their own commands. The C has a message for
// exactly that case ("You can't use immortal commands while switched").
func (s *Session) Original() *game.Character { return s.original }

// SwitchedFromLevel answers game.Character.RealLevel: the level of the
// character this connection really belongs to, and whether it is switched at
// all.
//
// This is the whole of GET_REAL_LEVEL (utils.h:268), and `CAN_SEE` is its only
// consumer — a god switched into a rat still sees the invisible immortals
// their own level entitles them to. Everything else about a switched god uses
// the body's level.
func (s *Session) SwitchedFromLevel() (int32, bool) {
	if s == nil || s.original == nil {
		return 0, false
	}
	return s.original.Level(), true
}

// SwitchInto puts this session in charge of another character.
func (s *Session) SwitchInto(victim *game.Character) {
	if s.original == nil {
		s.original = s.character
	}
	s.character = victim
	victim.Client = s
}

// SwitchBack undoes it, returning the character that was borrowed.
func (s *Session) SwitchBack() *game.Character {
	if s.original == nil {
		return nil
	}
	borrowed := s.character
	if borrowed != nil && borrowed.Client == game.Client(s) {
		borrowed.Client = nil
	}
	s.character = s.original
	s.original = nil
	if s.character != nil {
		s.character.Client = s
	}
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
	if s.character == nil || s.character.Record == nil || s.character.IsNPC() {
		return colour.Off
	}
	return colour.LevelOf(
		s.character.Record.Preferences.Has(game.PrefColour1),
		s.character.Record.Preferences.Has(game.PrefColour2),
	)
}

// SendRaw queues bytes without line-ending translation, for text that is
// already in wire form.
func (s *Session) SendRaw(b []byte) {
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

	if s.character != nil {
		if err := deps.Login.Leave(context.WithoutCancel(ctx), s, s.character); err != nil {
			s.logger.Error("removing the character from the world", "error", err)
		}
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

// maxLineLength bounds one line of input.
//
// A line longer than this is not something a person typed, and a client that
// never sends a newline would otherwise grow this buffer without limit.
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
					// perform_alias (comm.c:803), run once per line actually
					// read off the socket rather than once per command: a
					// complex alias expands to several, run here in order,
					// before anything further is read — the same effect as
					// the C pushing them to the front of the input queue. See
					// alias.go's expandAliasedLine.
					for _, part := range s.expandAliasedLine(text) {
						if handleErr := s.handle(ctx, deps, part); handleErr != nil {
							return handleErr
						}
						if s.state == StateClosed || s.closed.Load() {
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
	Sex      int32
	Class    int32
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
	if s.state == StatePaging {
		return s.pagerReturn.ConnectedName()
	}
	return s.state.ConnectedName()
}
