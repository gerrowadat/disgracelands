// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"context"
	"strings"

	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/persist/houses"
	"github.com/gerrowadat/disgracelands/internal/persist/player"
	"github.com/gerrowadat/disgracelands/internal/session"
)

// House_boot, House_crashsave and House_save_all (house.c:243, :126, :570).
//
// The objects in a house are stored in the same record the rent files use, so
// the codec is shared: a house file is a bare sequence of `obj_file_elem`
// with no header at all.

// houseKeeper implements session.HouseKeeper.
type houseKeeper struct{ s *Server }

// housesOrNilIface returns the housing system as the interface, or a nil
// interface when there is none — the same typed-nil trap as the mail system.
func housesOrNilIface(s *Server) session.HouseKeeper {
	if s.houses == nil {
		return nil
	}
	return &houseKeeper{s: s}
}

// loadHouses is House_boot: read the control file, drop the records that no
// longer make sense, flag the rooms and load the contents.
//
// Every one of the C's five sanity checks is here, and they are not
// paranoia: a house whose owner has been deleted, or whose room has been
// removed from the world, or whose door no longer leads to its atrium, would
// otherwise be a room nobody can enter and nobody can destroy.
func (s *Server) loadHouses(w *game.Live) {
	if s.houses == nil {
		return
	}

	records, err := s.houses.Load()
	if err != nil {
		s.logger.Error("reading the house control file", "error", err)
		return
	}

	keeper := &houseKeeper{s: s}
	loaded := make([]*game.House, 0, len(records))
	objects := 0

	for _, rec := range records {
		vnum, atrium := game.RoomVnum(rec.Vnum), game.RoomVnum(rec.Atrium)
		// The range is checked just below, in the same switch the C checks it.
		dir := game.Direction(rec.ExitNum) //nolint:gosec // range-checked below

		switch {
		case keeper.NameByID(rec.Owner) == "":
			s.logger.Warn("house owner no longer exists; skipping", "house", vnum)
			continue
		case w.Room(vnum) == nil:
			s.logger.Warn("house room does not exist; skipping", "house", vnum)
			continue
		case findLoaded(loaded, vnum) != nil:
			s.logger.Warn("house room is already a house; skipping", "house", vnum)
			continue
		case w.Room(atrium) == nil:
			s.logger.Warn("house has no atrium; skipping", "house", vnum, "atrium", atrium)
			continue
		case rec.ExitNum < 0 || int(rec.ExitNum) >= game.NumDirections:
			s.logger.Warn("house has an invalid exit; skipping", "house", vnum, "exit", rec.ExitNum)
			continue
		}
		exit := w.Exit(vnum, dir)
		if exit == nil || exit.ToRoom != atrium {
			s.logger.Warn("house exit does not lead to its atrium; skipping",
				"house", vnum, "atrium", atrium)
			continue
		}

		h := &game.House{
			Vnum: vnum, Atrium: atrium, ExitNum: dir,
			BuiltOn: rec.BuiltOn, LastPayment: rec.LastPayment,
			Mode: rec.Mode, Owner: rec.Owner,
			Guests: append([]int64(nil), rec.Guests...),
		}
		loaded = append(loaded, h)
		w.AddHouse(h)
		objects += s.loadHouseObjects(w, vnum)
	}

	s.logger.Info("houses loaded", "houses", len(loaded), "objects", objects,
		"skipped", len(records)-len(loaded))

	// House_boot rewrites the control file on the way out, which is how the
	// skipped records are actually removed.
	keeper.SaveControl(w)
}

func findLoaded(houses []*game.House, vnum game.RoomVnum) *game.House {
	for _, h := range houses {
		if h.Vnum == vnum {
			return h
		}
	}
	return nil
}

// loadHouseObjects is House_load: put a house's contents back in the room.
func (s *Server) loadHouseObjects(w *game.Live, vnum game.RoomVnum) int {
	stored, err := s.houses.LoadObjects(int32(vnum))
	if err != nil {
		s.logger.Error("reading a house file", "house", vnum, "error", err)
		return 0
	}
	if len(stored) == 0 {
		return 0
	}

	count := 0
	// Forwards, which is House_load's own direction (house.c:73-81), and
	// correct for the same reason the rent files' is: House_save recurses
	// before writing (house.c:94-96), so the file holds the room's contents
	// back to front, and reading it forwards into a list that grows at the
	// head puts them back where they were. This walked backwards until #193,
	// when it stopped being a compensation for an ObjectToRoom that appended
	// and became a reversal of its own.
	for _, st := range stored {
		obj := w.NewObject(st.Vnum)
		if obj == nil {
			continue
		}
		obj.Values = st.Values
		obj.ExtraFlags = st.ExtraFlags
		obj.Weight = st.Weight
		obj.Timer = st.Timer
		obj.PermAffect = st.PermAffect
		obj.Affects = append([]game.ObjAffect(nil), st.Affects...)
		w.ObjectToRoom(obj, vnum)
		count++
	}
	return count
}

// SaveControl implements session.HouseKeeper.
func (k *houseKeeper) SaveControl(w *game.Live) {
	s := k.s
	var records []houses.House
	// Read on the world goroutine — this is called from a command, which is
	// already on it, so the snapshot is taken here and written elsewhere.
	for _, h := range w.Houses() {
		records = append(records, houses.House{
			Vnum: int32(h.Vnum), Atrium: int32(h.Atrium), ExitNum: int32(h.ExitNum),
			BuiltOn: h.BuiltOn, LastPayment: h.LastPayment,
			Mode: h.Mode, Owner: h.Owner,
			Guests: append([]int64(nil), h.Guests...),
		})
	}

	s.background(func() {
		if err := s.houses.Save(records); err != nil {
			s.logger.Error("writing the house control file", "error", err)
		}
	})
}

// SaveHouse implements session.HouseKeeper: House_crashsave for one room.
//
// Called from a command, so it is already on the world goroutine: the objects
// are read here and the file is written elsewhere.
func (k *houseKeeper) SaveHouse(w *game.Live, vnum game.RoomVnum) {
	s := k.s
	stored := houseObjects(nil, w.RoomObjects(vnum))

	s.background(func() {
		if err := s.houses.SaveObjects(int32(vnum), stored); err != nil {
			s.logger.Error("writing a house file", "house", vnum, "error", err)
		}
	})
}

// houseObjects flattens a room's contents in House_save's order, which is not
// Crash_save's: contents first, then the rest of the list, then the object
// itself (house.c:94). The two functions do the same job and disagree about
// the order, and both are reproduced because both files exist.
func houseObjects(out []player.StoredObject, list []*game.Object) []player.StoredObject {
	if len(list) == 0 {
		return out
	}
	out = houseObjects(out, list[0].Contents)
	out = houseObjects(out, list[1:])
	return append(out, storedFrom(list[0]))
}

// DeleteHouse implements session.HouseKeeper.
func (k *houseKeeper) DeleteHouse(vnum game.RoomVnum) {
	s := k.s
	s.background(func() {
		if err := s.houses.DeleteObjects(int32(vnum)); err != nil {
			s.logger.Error("deleting a house file", "house", vnum, "error", err)
		}
	})
}

// IDByName implements session.HouseKeeper.
func (k *houseKeeper) IDByName(name string) (int64, bool) {
	for entry, err := range k.s.players.List(context.Background()) {
		if err != nil {
			continue
		}
		if strings.EqualFold(entry.Name, name) && !entry.Flags.Has(game.PlayerDeleted) {
			return entry.IDNum, true
		}
	}
	return -1, false
}

// NameByID implements session.HouseKeeper.
func (k *houseKeeper) NameByID(id int64) string {
	for entry, err := range k.s.players.List(context.Background()) {
		if err != nil {
			continue
		}
		if entry.IDNum == id {
			return entry.Name
		}
	}
	return ""
}

// SaveChangedHouses is House_save_all (house.c:570), run on shutdown and by
// the autosave.
//
// Only the houses flagged ROOM_HOUSE_CRASH are written, which is the one
// place the C's dirty bit is worth keeping: a hundred houses rewritten every
// minute is a hundred files rewritten every minute, and the flag is set by
// exactly the thing that makes a rewrite necessary.
func (s *Server) SaveChangedHouses(ctx context.Context) {
	if s.houses == nil {
		return
	}
	type pending struct {
		vnum game.RoomVnum
		objs []player.StoredObject
	}
	var work []pending

	if err := s.engine.DoSync(ctx, func(w *game.Live) {
		for _, h := range w.Houses() {
			room := w.Room(h.Vnum)
			if room == nil || !room.Flags.Has(game.RoomHouseCrash) {
				continue
			}
			work = append(work, pending{vnum: h.Vnum, objs: houseObjects(nil, w.RoomObjects(h.Vnum))})
			room.Flags = room.Flags.Clear(game.RoomHouseCrash)
		}
	}); err != nil {
		s.logger.Error("collecting houses to save", "error", err)
		return
	}

	for _, p := range work {
		if err := s.houses.SaveObjects(int32(p.vnum), p.objs); err != nil {
			s.logger.Error("writing a house file", "house", p.vnum, "error", err)
		}
	}
}
