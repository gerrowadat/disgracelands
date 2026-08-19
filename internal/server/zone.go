// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/session"
)

// Zone ageing, ported from zone_update (db.c).
//
// Each zone has a lifespan in minutes and a reset mode. Every minute its age
// goes up; when the age reaches the lifespan the zone is queued, and it is
// reset as soon as the mode allows — immediately for mode 2, or once the last
// player has left for mode 1. Mode 0 never resets at all.

// pulseZone is PULSE_ZONE: the queue is examined every ten seconds.
const pulseZone = 10 * pulsesPerSecond

// zoneDead is ZO_DEAD: the age a queued zone is parked at so it is not
// queued twice while it waits for the room to empty.
const zoneDead int32 = 32000

// Reset modes, from the zone file's fourth header field.
const (
	resetNever      int32 = 0
	resetWhenEmpty  int32 = 1
	resetRegardless int32 = 2
)

// zoneState is the ageing state the C keeps in the zone table itself. It is
// separate here because the zone definitions come from the world files and
// the loader owns them.
type zoneState struct {
	age    int32
	queued bool
}

// BootReset populates the world, porting the reset of every zone that
// boot_db does before the first player connects.
//
// Without this the world is 2,981 rooms and nothing else: every mobile and
// every object a player ever sees is created here or by a later reset.
func (s *Server) BootReset(w *game.Live) {
	// The socials are commands, so they have to be in the table before
	// anybody can type one. The C boots them in boot_db alongside the world.
	if socials := s.text.Socials(); len(socials) > 0 {
		added, unknown := session.RegisterSocials(socials)
		s.logger.Info("socials loaded", "count", added, "commands", len(session.Commands))
		for _, name := range unknown {
			// The C's "SYSERR: Unknown social '%s' in social file."
			s.logger.Warn("social has no command table entry", "social", name)
		}
	}

	// Special procedures are attached before the reset, because the reset is
	// what instantiates the mobiles that carry them. `-s` skips it, which is
	// what the C's no_specials does.
	if s.noSpecials {
		s.logger.Info("special procedures disabled")
	} else {
		attached, missing := w.AssignSpecials()
		s.logger.Info("special procedures assigned",
			"attached", attached, "missing_vnums", missing,
			"implemented", len(session.SpecialNames()))
	}

	var mobiles, objects, problems int

	for _, zone := range w.Zones() {
		report := w.ResetZone(zone, s.rng)
		mobiles += report.Mobiles
		objects += report.Objects
		problems += len(report.Problems)

		for _, problem := range report.Problems {
			s.logger.Warn("zone reset", "zone", zone.Vnum, "name", zone.Name, "problem", problem)
		}
	}

	s.logger.Info("world populated",
		"zones", len(w.Zones()), "mobiles", mobiles, "objects", objects, "problems", problems)
}

// zoneUpdate ages the zones and resets the ones that are due, porting
// zone_update.
//
// The C counts pulses and does the ageing when a minute has passed, then
// walks its queue every time regardless. Same shape here: the minute counter
// drives ageing, the queue is drained on every call.
func (s *Server) zoneUpdate(w *game.Live) {
	s.zoneTicks++

	// pulseZone is ten seconds, so six calls make a minute.
	if s.zoneTicks*pulseZone/pulsesPerSecond >= 60 {
		s.zoneTicks = 0
		s.ageZones(w)
	}

	s.drainZoneQueue(w)
}

// ageZones advances every zone's clock and queues the ones that are due.
func (s *Server) ageZones(w *game.Live) {
	for i, zone := range w.Zones() {
		state := s.zoneStateFor(i)
		if zone.ResetMode == resetNever {
			continue
		}

		if state.age < zone.Lifespan {
			state.age++
		}
		if state.age >= zone.Lifespan && state.age < zoneDead {
			state.queued = true
			// Parked, so a zone waiting for its players to leave is not
			// queued again every minute.
			state.age = zoneDead
		}
	}
}

// drainZoneQueue resets whichever queued zones are allowed to.
func (s *Server) drainZoneQueue(w *game.Live) {
	for i, zone := range w.Zones() {
		state := s.zoneStateFor(i)
		if !state.queued {
			continue
		}
		if zone.ResetMode != resetRegardless && !w.ZoneIsEmpty(zone) {
			continue
		}

		report := w.ResetZone(zone, s.rng)
		state.queued = false
		state.age = 0

		s.logger.Debug("zone reset",
			"zone", zone.Vnum, "name", zone.Name,
			"mobiles", report.Mobiles, "objects", report.Objects)
		for _, problem := range report.Problems {
			s.logger.Warn("zone reset", "zone", zone.Vnum, "problem", problem)
		}
	}
}

func (s *Server) zoneStateFor(i int) *zoneState {
	if s.zones == nil {
		s.zones = map[int]*zoneState{}
	}
	if s.zones[i] == nil {
		s.zones[i] = &zoneState{}
	}
	return s.zones[i]
}
