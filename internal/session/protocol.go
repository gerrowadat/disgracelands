// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package session

import (
	"encoding/json"
	"sync"

	"golang.org/x/text/transform"

	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/telnet"
)

// protocol is a session's telnet state: which options are on, what charset
// the client reads, and which GMCP packages it asked for.
//
// It is guarded by a mutex because the read loop changes it and the write
// loop reads it, and those are different goroutines. The world goroutine
// never touches it.
type protocol struct {
	mu sync.Mutex

	// neg owns option state and does its own locking, so it sits outside the
	// mutex above rather than under it.
	neg *telnet.Negotiator

	// echo is true while the server has told the client to stop echoing,
	// which is how a password is hidden.
	echo bool

	gmcp     bool
	supports telnet.Supports

	// charset is what the client reads, and encoder converts to it. A nil
	// encoder means UTF-8, which is what the server already sends.
	charset string
	encoder transform.Transformer
}

// policy is what the server will agree to negotiate.
//
// Nothing is allowed on the client's side: the server asks for none of the
// options a client can turn on for itself, and an option agreed and then
// ignored is worse than one refused, because the client will start sending
// for it.
var policy = telnet.Policy{
	Us: []byte{telnet.OptEcho, telnet.OptSuppressGoAhead, telnet.OptCharset, telnet.OptGMCP},
}

// offer sends the options the server volunteers.
//
// CHARSET and GMCP, and deliberately **not** SUPPRESS-GO-AHEAD. Offering SGA
// is what puts telnet(1) into character-at-a-time mode, where the terminal
// stops doing line editing and the server is expected to take it over — so a
// player typing at the login prompt got a literal ^M for the Enter key and a
// literal ^? for backspace. The C server negotiates nothing and stays in line
// mode, which is the experience these players actually had, and a client that
// wants SGA still gets it: the policy above agrees the moment one asks.
//
// The reason for offering the other two first stands. A client that supports
// GMCP has it on before the login sequence starts, so a web front end can
// render the name prompt itself rather than scraping it.
//
// That web front end is not this one. internal/server/web.go's browser
// terminal is a plain ANSI byte stream rendered by xterm.js, which has no
// telnet awareness at all — there is nothing on the other end to negotiate
// with, and SendRaw already drops every telnet control sequence for a
// websocket session regardless, so offering is skipped outright rather
// than sent and ignored. A GMCP-aware web client, if one is ever built, is
// a different transport policy from this basic one, not a reason to
// change what SendRaw does here.
func (s *Session) offer() {
	if s.transport == "websocket" {
		return
	}
	s.SendRaw(s.proto.neg.Enable(telnet.SideUs, telnet.OptCharset))
	s.SendRaw(s.proto.neg.Enable(telnet.SideUs, telnet.OptGMCP))
}

// handleTelnet acts on one command taken out of the input stream.
func (s *Session) handleTelnet(ev telnet.Event) {
	switch ev.Kind {
	case telnet.EventNegotiation:
		s.handleNegotiation(ev)
	case telnet.EventSubnegotiation:
		s.handleSubnegotiation(ev)
	}
}

// handleNegotiation answers one WILL/WONT/DO/DONT.
//
// The answer itself is the negotiator's business — which is where the RFC
// 1143 state machine lives, and why a repeated offer is not answered twice —
// and what is left here is what an agreed option *means* to the session.
func (s *Session) handleNegotiation(ev telnet.Event) {
	reply, change := s.proto.neg.Receive(ev.Command, ev.Option)
	s.SendRaw(reply)

	if !change.Changed || change.Side != telnet.SideUs {
		return
	}

	switch change.Option {
	case telnet.OptCharset:
		if change.Enabled {
			// The client will talk about charsets, so ask which one it reads.
			s.SendRaw(telnet.CharsetRequestBytes())
		}
	case telnet.OptGMCP:
		s.proto.mu.Lock()
		s.proto.gmcp = change.Enabled
		s.proto.mu.Unlock()
		s.logger.Debug("gmcp", "enabled", change.Enabled)
	case telnet.OptSuppressGoAhead:
		// Agreed on request rather than offered; see offer(). The server
		// sends no GA either way, so there is nothing to change.
		s.logger.Debug("suppress-go-ahead", "enabled", change.Enabled)
	}
}

func (s *Session) handleSubnegotiation(ev telnet.Event) {
	switch ev.Option {
	case telnet.OptCharset:
		name, ok := telnet.ParseCharsetResponse(ev.Payload)
		if !ok {
			s.logger.Debug("client kept its own charset")
			return
		}
		enc := telnet.Encoder(name)
		s.proto.mu.Lock()
		s.proto.charset, s.proto.encoder = name, enc
		s.proto.mu.Unlock()
		s.logger.Debug("charset agreed", "charset", name, "transcoding", enc != nil)

	case telnet.OptGMCP:
		msg, err := telnet.ParseGMCP(ev.Payload)
		if err != nil {
			s.logger.Debug("bad GMCP message", "error", err)
			return
		}
		s.handleGMCP(msg)
	}
}

// handleGMCP acts on a message from the client.
func (s *Session) handleGMCP(msg telnet.GMCPMessage) {
	switch msg.Package {
	case "Core.Hello":
		// Identification only. Worth logging, since knowing what people
		// connect with is how the question of what to support gets answered.
		s.logger.Debug("gmcp hello", "payload", string(msg.Data))

	case "Core.Supports.Set", "Core.Supports.Add", "Core.Supports.Remove":
		var list []string
		if err := json.Unmarshal(msg.Data, &list); err != nil {
			s.logger.Debug("bad Core.Supports payload", "error", err)
			return
		}
		s.proto.mu.Lock()
		switch msg.Package {
		case "Core.Supports.Set":
			s.proto.supports.Set(list)
		case "Core.Supports.Add":
			s.proto.supports.Add(list)
		case "Core.Supports.Remove":
			s.proto.supports.Remove(list)
		}
		s.proto.mu.Unlock()

	case "Core.Ping":
		s.SendGMCP("Core.Ping", nil)

	default:
		s.logger.Debug("unhandled GMCP package", "package", msg.Package)
	}
}

// SendGMCP sends one out-of-band message, if the client turned GMCP on and
// asked for that package.
func (s *Session) SendGMCP(pkg string, data any) {
	s.proto.mu.Lock()
	on := s.proto.gmcp && s.proto.supports.Wants(pkg)
	s.proto.mu.Unlock()
	if !on {
		return
	}

	wire, err := telnet.GMCP(pkg, data)
	if err != nil {
		s.logger.Error("encoding GMCP", "package", pkg, "error", err)
		return
	}
	s.SendRaw(wire)
}

// EchoOff tells the client to stop echoing, which is how the password prompt
// hides what is typed. This is the C's echo_off_str (comm.c).
func (s *Session) EchoOff() {
	s.proto.mu.Lock()
	s.proto.echo = true
	s.proto.mu.Unlock()
	// Announce rather than Enable: see Negotiator.Announce for why ECHO is
	// the one option that does not wait to be agreed with.
	s.SendRaw(s.proto.neg.Announce(telnet.SideUs, telnet.OptEcho, true))
}

// EchoOn undoes EchoOff. It must be called on every path away from a password
// prompt, including the failing ones, or the player is left typing blind.
func (s *Session) EchoOn() {
	s.proto.mu.Lock()
	wasOff := s.proto.echo
	s.proto.echo = false
	s.proto.mu.Unlock()
	if wasOff {
		s.SendRaw(s.proto.neg.Announce(telnet.SideUs, telnet.OptEcho, false))
	}
}

// Charset returns the encoding the client reads, for the who-list and logs.
func (s *Session) Charset() string {
	s.proto.mu.Lock()
	defer s.proto.mu.Unlock()
	if s.proto.charset == "" {
		return "UTF-8"
	}
	return s.proto.charset
}

// encodeOutbound applies the client's charset and telnet escaping to one
// chunk of text.
func (s *Session) encodeOutbound(b []byte) []byte {
	s.proto.mu.Lock()
	enc := s.proto.encoder
	s.proto.mu.Unlock()

	b = telnet.EncodeTo(enc, b)
	if telnet.NeedsEscaping(b) {
		return telnet.Escape(make([]byte, 0, len(b)+8), b)
	}
	return b
}

// Vitals is the Char.Vitals GMCP package: a client's view of the prompt,
// without having to parse the prompt.
type Vitals struct {
	HP       int32 `json:"hp"`
	MaxHP    int32 `json:"maxhp"`
	Mana     int32 `json:"mana"`
	MaxMana  int32 `json:"maxmana"`
	Moves    int32 `json:"moves"`
	MaxMoves int32 `json:"maxmoves"`
	Level    int32 `json:"level"`
	Exp      int32 `json:"exp"`
}

// RoomInfo is the Room.Info GMCP package.
type RoomInfo struct {
	Vnum  int32    `json:"num"`
	Name  string   `json:"name"`
	Desc  string   `json:"desc,omitempty"`
	Exits []string `json:"exits"`
	Zone  string   `json:"zone,omitempty"`
}

// SendVitals sends Char.Vitals for a character.
func (s *Session) SendVitals(c *game.Character) {
	if c == nil || c.Record == nil {
		return
	}
	p := c.Record.Points
	s.SendGMCP("Char.Vitals", Vitals{
		HP: p.Hit, MaxHP: p.MaxHit,
		Mana: p.Mana, MaxMana: p.MaxMana,
		Moves: p.Move, MaxMoves: p.MaxMove,
		Level: c.Record.Level, Exp: p.Exp,
	})
}
