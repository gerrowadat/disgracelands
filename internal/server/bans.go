// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"context"
	"time"

	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/persist/bans"
	"github.com/gerrowadat/disgracelands/internal/session"
)

// The server half of the ban list and of `show`.

// bansOrNil returns the ban list as the interface, or a nil interface when
// there is none — the same typed-nil trap as the mail system.
func bansOrNil(s *Server) session.BanKeeper {
	if s.bans == nil {
		return nil
	}
	return &banKeeper{s: s}
}

type banKeeper struct{ s *Server }

func (k *banKeeper) Bans() []session.BanEntry {
	list := k.s.bans.List()
	out := make([]session.BanEntry, 0, len(list))
	for _, ban := range list {
		out = append(out, session.BanEntry{
			Site: ban.Site, Type: ban.Type.String(), When: ban.When, By: ban.By,
		})
	}
	return out
}

func (k *banKeeper) Ban(site, kind, by string) (bool, error) {
	parsed, ok := bans.ParseType(kind)
	if !ok {
		return false, nil
	}
	return k.s.bans.Add(bans.Ban{Site: site, Type: parsed, When: time.Now(), By: by})
}

func (k *banKeeper) Unban(site string) (string, bool, error) {
	ban, found, err := k.s.bans.Remove(site)
	return ban.Type.String(), found, err
}

func (k *banKeeper) ValidBanType(kind string) bool {
	_, ok := bans.ParseType(kind)
	return ok
}

// ShowPlayer implements session.Operator: the roster record `show player`
// prints, read without putting anybody in the world.
func (s *Server) ShowPlayer(name string) (session.PlayerSummary, bool) {
	rec, err := s.players.Load(context.Background(), name)
	if err != nil {
		return session.PlayerSummary{}, false
	}
	return session.PlayerSummary{
		Name:      rec.Name,
		Sex:       rec.Sex,
		Level:     rec.Level,
		Class:     rec.Class,
		Gold:      rec.Points.Gold,
		Bank:      rec.Points.BankGold,
		Exp:       rec.Points.Exp,
		Alignment: rec.Alignment,
		Lessons:   rec.SpellsToLearn,
		Born:      rec.Birth,
		LastLogon: rec.LastLogon,
		Played:    rec.Played,
	}, true
}

// ZoneAge implements session.Operator: minutes since a zone last reset.
//
// The C keeps this in the zone table itself; this port keeps it beside the
// pulse that drives it, which is why `show zones` has to ask the server
// rather than the world.
func (s *Server) ZoneAge(vnum game.ZoneVnum) int32 {
	if state, ok := s.zones[int(vnum)]; ok && state != nil {
		return state.age
	}
	return 0
}

// BanFor implements session.LoginHandler: the ban type as a word, or "" for
// a site that is not banned.
func (s *Server) BanFor(host string) string {
	kind := s.banFor(host)
	if kind == bans.TypeNone {
		return ""
	}
	return kind.String()
}

// banFor is the typed form, kept separate so the login path and the tests can
// each have the shape they want.
func (s *Server) banFor(host string) bans.Type {
	if s.bans == nil {
		return bans.TypeNone
	}
	return s.bans.Check(host)
}
