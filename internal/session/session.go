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
	"bufio"
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

	"github.com/gerrowadat/disgracelands/internal/game"
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
	// StateReadMOTD: press return, having read the message of the day.
	StateReadMOTD
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
	case StateReadMOTD:
		return "read-motd"
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

// Session is one player's connection.
type Session struct {
	id   uint64
	conn net.Conn
	// transport names how they arrived, for logs and the who-list.
	transport string
	host      string

	out    chan []byte
	logger *slog.Logger

	state     State
	character *game.Character

	// pending holds the name and password being entered during login.
	pendingName string
	pendingHash string

	closed atomic.Bool
	closer sync.Once
}

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
	Credits() string
}

// LoginHandler performs the steps that need more than the connection.
type LoginHandler interface {
	// Exists reports whether a character is known.
	Exists(ctx context.Context, name string) (bool, error)
	// Authenticate checks a password and returns the character on success.
	// A nil character with a nil error means the password was wrong.
	Authenticate(ctx context.Context, name, password string) (*game.Character, error)
	// Create makes a new character with the given password.
	Create(ctx context.Context, name, password string) (*game.Character, error)
	// Enter puts an authenticated character into the world.
	Enter(ctx context.Context, s *Session, c *game.Character) error
	// Leave takes them out again.
	Leave(ctx context.Context, s *Session, c *game.Character) error
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
		out:       make(chan []byte, outputQueue),
		logger:    logger.With("session", id, "transport", transport, "host", host),
		state:     StateGetName,
	}
}

// ID identifies the session in logs.
func (s *Session) ID() uint64 { return s.id }

// Transport names how the player connected.
func (s *Session) Transport() string { return s.transport }

// Host is the address they came from.
func (s *Session) Host() string { return s.host }

// State returns where the session has got to.
func (s *Session) State() State { return s.state }

// Character returns the logged-in character, or nil.
func (s *Session) Character() *game.Character { return s.character }

// SetCharacter attaches a character to the session.
func (s *Session) SetCharacter(c *game.Character) { s.character = c }

// Send queues output. It never blocks: a client that cannot keep up is
// disconnected rather than allowed to stall the world.
func (s *Session) Send(format string, args ...any) {
	if s.closed.Load() {
		return
	}
	text := format
	if len(args) > 0 {
		text = fmt.Sprintf(format, args...)
	}
	select {
	case s.out <- []byte(normalise(text)):
	default:
		s.logger.Warn("output queue full; dropping the connection")
		s.Close()
	}
}

// SendRaw queues bytes without line-ending translation, for text that is
// already in wire form.
func (s *Session) SendRaw(b []byte) {
	if s.closed.Load() {
		return
	}
	select {
	case s.out <- b:
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
		close(s.out)
		_ = s.conn.Close()
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

	// The licence requires the login sequence to name the DikuMUD and
	// CircleMUD creators, and this is the login sequence. Every transport
	// reaches this line; none may skip it.
	s.Send("%s", deps.Text.Greeting())
	s.Send("By what name do you wish to be known? ")

	if err := s.readLoop(ctx, deps); err != nil && !isDisconnect(err) {
		s.logger.Info("session ended", "error", err)
	}

	if s.character != nil {
		if err := deps.Login.Leave(context.WithoutCancel(ctx), s, s.character); err != nil {
			s.logger.Error("removing the character from the world", "error", err)
		}
	}
	s.logger.Info("disconnected")
}

// readLoop reads lines and dispatches them.
func (s *Session) readLoop(ctx context.Context, deps Deps) error {
	sc := bufio.NewScanner(s.conn)
	// A line longer than this is not something a person typed.
	sc.Buffer(make([]byte, 0, 4096), 64*1024)

	for sc.Scan() {
		if err := ctx.Err(); err != nil {
			return err
		}
		line := strings.TrimRight(sc.Text(), "\r\n")

		if err := s.handle(ctx, deps, line); err != nil {
			return err
		}
		if s.state == StateClosed || s.closed.Load() {
			return nil
		}
	}
	return sc.Err()
}

// writeLoop drains queued output to the connection.
func (s *Session) writeLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case b, ok := <-s.out:
			if !ok {
				return
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
