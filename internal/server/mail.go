// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/persist/mail"
	"github.com/gerrowadat/disgracelands/internal/session"
)

// The server half of the mail system: the store, and the name/id lookups the
// postmaster needs.
//
// get_id_by_name and get_name_by_id are one-line lookups into the C's
// in-memory player index. There is no such index here — the roster is behind
// player.Store — so these walk the listing, which is cheap enough at the size
// of a roster and is not on any hot path: sending mail and reading it are the
// only callers.

// mailSystem implements session.MailSystem.
type mailSystem struct{ s *Server }

func (m *mailSystem) Available() bool { return m.s.mail != nil }

func (m *mailSystem) HasMail(id int64) bool { return m.s.mail.HasMail(id) }

// Send stores a message. Off the world goroutine, like every other write.
func (m *mailSystem) Send(to, from int64, sent time.Time, text string) {
	s := m.s
	s.background(func() {
		if err := s.mail.Send(mail.Message{To: to, From: from, Sent: sent, Text: text}); err != nil {
			s.logger.Error("storing mail", "to", to, "from", from, "error", err)
		}
	})
}

// Receive takes the next letter and renders it, porting the header
// read_delete builds (mail.c:461).
//
// The sender's and recipient's names are looked up now rather than stored, so
// a letter written to somebody who has since been renamed still says who they
// are — and one from a character who has been deleted says "Unknown", which
// is the C's word for it.
func (m *mailSystem) Receive(id int64) (string, bool) {
	msg, ok, err := m.s.mail.Receive(id)
	if err != nil {
		m.s.logger.Error("reading mail", "id", id, "error", err)
		return "", false
	}
	if !ok {
		return "", false
	}

	ctx := context.Background()
	to := m.nameByID(ctx, msg.To)
	from := m.nameByID(ctx, msg.From)

	// asctime's format, with its trailing newline stripped, which is what the
	// C does by hand: `*(tmstr + strlen(tmstr) - 1) = '\0'`.
	stamp := msg.Sent.Format("Mon Jan _2 15:04:05 2006")

	var b strings.Builder
	b.WriteString(" * * * * Midgaard Mail System * * * *\r\n")
	fmt.Fprintf(&b, "Date: %s\r\n", stamp)
	fmt.Fprintf(&b, "  To: %s\r\n", to)
	fmt.Fprintf(&b, "From: %s\r\n\r\n", from)
	b.WriteString(msg.Text)
	return b.String(), true
}

// IDByName is get_id_by_name plus mail_recip_ok: the id of a character who
// exists and has not been deleted.
func (m *mailSystem) IDByName(name string) (int64, bool) {
	ctx := context.Background()
	for entry, err := range m.s.players.List(ctx) {
		if err != nil {
			continue
		}
		if !strings.EqualFold(entry.Name, name) {
			continue
		}
		// mail_recip_ok loads the whole record to check one bit. The listing
		// carries the flags, so this does not have to.
		if entry.Flags.Has(game.PlayerDeleted) {
			return -1, false
		}
		return entry.IDNum, true
	}
	return -1, false
}

// nameByID is get_name_by_id, with the C's answer for a character who is no
// longer there.
func (m *mailSystem) nameByID(ctx context.Context, id int64) string {
	for entry, err := range m.s.players.List(ctx) {
		if err != nil {
			continue
		}
		if entry.IDNum == id {
			return entry.Name
		}
	}
	return "Unknown"
}

// mailOrNil returns the mail system as the interface, or a nil interface when
// there is none.
//
// A typed nil in an interface is not nil, and the postmaster tests
// `sc.Mail == nil` — so returning m.Mail() directly would make every server
// without a mail file report that the mail is merely "having technical
// difficulties" rather than absent.
func mailOrNil(s *Server) session.MailSystem {
	if s.mail == nil {
		return nil
	}
	return &mailSystem{s: s}
}
