// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/persist/player"
	"github.com/gerrowadat/disgracelands/internal/session"
)

// The server half of the operational commands: the list of live connections,
// the wizlock level, and the switch that stops the world.
//
// The C keeps all three as globals — `descriptor_list`, `circle_restrict`
// and `circle_shutdown`. Here they are on the Server, because a test builds
// several of those in one process and globals would make them share.

// registry tracks live sessions in the order they connected, which is the
// order `users` numbers them and `dc` addresses them.
type registry struct {
	mu       sync.Mutex
	next     int
	sessions map[*session.Session]int
}

func (r *registry) add(s *session.Session) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.sessions == nil {
		r.sessions = map[*session.Session]int{}
	}
	r.next++
	r.sessions[s] = r.next
}

func (r *registry) remove(s *session.Session) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.sessions, s)
}

// list returns the live sessions, oldest first.
func (r *registry) list() []*session.Session {
	r.mu.Lock()
	defer r.mu.Unlock()

	out := make([]*session.Session, 0, len(r.sessions))
	for s := range r.sessions {
		out = append(out, s)
	}
	order := r.sessions
	sort.Slice(out, func(i, j int) bool { return order[out[i]] < order[out[j]] })
	return out
}

// Sessions implements session.Operator.
func (s *Server) Sessions() []*session.Session { return s.connections.list() }

// Restrict implements session.Operator: the C's `circle_restrict`.
func (s *Server) Restrict() int32 { return s.wizlock.Load() }

// SetRestrict implements session.Operator.
func (s *Server) SetRestrict(level int32) { s.wizlock.Store(level) }

// BootTime implements session.Operator.
func (s *Server) BootTime() time.Time { return s.booted }

// Shutdown implements session.Operator.
//
// It does not stop anything itself: it closes a channel that main() is
// waiting on, which then runs the ordinary shutdown — the saves, the
// crash-saves, the waiting for writes. A `shutdown` that skipped those would
// be worse than no `shutdown` at all.
func (s *Server) Shutdown(reboot bool) {
	s.rebootWanted.Store(reboot)
	s.shutdownOnce.Do(func() { close(s.shutdownWanted) })
}

// ShutdownRequested returns a channel closed when a god asks the server to
// stop.
func (s *Server) ShutdownRequested() <-chan struct{} { return s.shutdownWanted }

// RebootWanted reports whether the shutdown asked to come back.
//
// The C touches one of several files on its way out and lets the wrapper
// script decide; this port has no wrapper — the container runtime restarts
// it, see docs/operations.md — so the answer is an exit code instead.
func (s *Server) RebootWanted() bool { return s.rebootWanted.Load() }

// LastLogin implements session.Operator: read out of the roster rather than
// the world, so it works for somebody who is not logged in.
func (s *Server) LastLogin(name string) (session.LastLogin, bool) {
	rec, err := s.players.Load(context.Background(), name)
	if err != nil {
		return session.LastLogin{}, false
	}
	return session.LastLogin{
		IDNum: rec.IDNum,
		Level: rec.Level,
		Class: rec.Class,
		Name:  rec.Name,
		Host:  rec.Host,
		When:  rec.LastLogon,
	}, true
}

// ShowRent implements session.Operator: `show rent`, reading a name's rent
// file without loading it — s.objects is the same ObjectStore rent.go itself
// reads and writes, and this never calls anything that would spend the file
// the way entering the world does.
func (s *Server) ShowRent(name string) (session.RentListing, bool) {
	if s.objects == nil {
		return session.RentListing{}, false
	}
	f, err := s.objects.LoadObjects(context.Background(), name)
	if err != nil {
		// Both of Crash_listrent's failure cases — no file at all
		// (player.ErrNotFound) and a file that exists but would not read
		// — answer the same way here: there is nothing to list. The C's
		// own two messages ("has no rent file" vs "Error reading rent
		// information") differ only in wording, not in what a caller
		// should do next.
		return session.RentListing{}, false
	}

	vnums := make([]game.ObjVnum, len(f.Objects))
	for i, obj := range f.Objects {
		vnums[i] = obj.Vnum
	}
	return session.RentListing{Code: rentListingCode(f.Code), Vnums: vnums}, true
}

// rentListingCode is Crash_listrent's own rentcode switch (objsave.c:368-385):
// RENT_FORCED shares TimedOut's word with RENT_TIMEDOUT, and anything else —
// RENT_UNDEF included — is "Undef".
func rentListingCode(code player.RentCode) string {
	switch code {
	case player.RentRented:
		return "Rent"
	case player.RentCrash:
		return "Crash"
	case player.RentCryo:
		return "Cryo"
	case player.RentTimedOut, player.RentForced:
		return "TimedOut"
	default:
		return "Undef"
	}
}

// ReloadText implements session.Operator: `reload`.
//
// The known/error split lets the session layer print the C's "Unknown reload
// option." for a name it does not recognise, without the command having to
// know what the names are.
func (s *Server) ReloadText(what string) (bool, error) {
	if s.text == nil {
		return false, nil
	}
	err := s.text.Reload(what)
	if ErrUnknownReload(err) {
		return false, nil
	}
	return true, err
}
