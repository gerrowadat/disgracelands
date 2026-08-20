// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"context"
	"errors"
	"time"

	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/persist/player"
	"github.com/gerrowadat/disgracelands/internal/session"
)

// Crash_load and Crash_crashsave, ported from objsave.c.
//
// What you were carrying when you left. The C calls the file the rent file
// and the crash file interchangeably because it is both: renting at an inn
// writes it, and so does quitting, and so does the server going down. The
// header's rent code is the difference.
//
// The format is in internal/persist/player/binary/objfile.go. This is the
// part that turns it into objects in somebody's hands, and back.

// secondsPerRealDay is SECS_PER_REAL_DAY (structs.h), the divisor the daily
// rent is prorated by.
const secondsPerRealDay = 24 * time.Hour

// loadObjects is Crash_load (objsave.c:432).
//
// It reports whether the character's things were lost to unpaid rent, which
// is the only one of the C's three return values the caller acts on — see
// the note on the load room below.
func (s *Server) loadObjects(ctx context.Context, c *game.Character) (lost bool, err error) {
	if s.objects == nil || c == nil || c.Record == nil {
		return false, nil
	}

	f, err := s.objects.LoadObjects(ctx, c.Name)
	if errors.Is(err, player.ErrNotFound) {
		// "%s entering game with no equipment." Not an error: it is every
		// character who has never left the game carrying anything.
		s.logger.Info("entering the game with no equipment", "character", c.Name)
		return false, nil
	}
	if err != nil {
		// The C sends a NOTICE telling them to contact a god, logs, and lets
		// them in empty-handed. Losing a backpack is not a reason to refuse
		// somebody entry.
		s.logger.Error("reading a rent file", "character", c.Name, "error", err)
		c.Tell("\r\n********************* NOTICE *********************\r\n" +
			"There was a problem loading your objects from disk.\r\n" +
			"Contact a God for assistance.\r\n")
		return false, nil
	}

	// Arrears, for the two codes that charge by the day.
	if f.Code == player.RentRented || f.Code == player.RentTimedOut {
		// The C computes the elapsed days as a float and multiplies an int
		// by it, so the total is truncated toward zero and not rounded: a
		// stay of 29 hours at 10 a day costs 12, and a stay of six hours
		// costs nothing at all. Reproduced deliberately.
		days := float64(time.Since(f.Written)) / float64(secondsPerRealDay)
		cost := int32(float64(f.CostPerDay) * days) //nolint:gosec // truncation is the C's arithmetic

		if cost > c.Record.Points.Gold+c.Record.Points.BankGold {
			s.logger.Info("entering the game, rented equipment lost (no $)",
				"character", c.Name, "cost", cost,
				"gold", c.Record.Points.Gold, "bank", c.Record.Points.BankGold)
			// The C crash-saves them here — with nothing, since the objects
			// were never loaded — which is what actually destroys the file's
			// contents. "Donated to the Salvation Army", says the message.
			if err := s.crashSave(ctx, c); err != nil {
				s.logger.Error("crash-saving after losing rented equipment",
					"character", c.Name, "error", err)
			}
			return true, nil
		}
		c.Record.Points.BankGold -= max(cost-c.Record.Points.Gold, 0)
		c.Record.Points.Gold = max(c.Record.Points.Gold-cost, 0)
	}

	s.logger.Info("retrieving saved items and entering the game",
		"character", c.Name, "rentcode", f.Code.String(), "items", len(f.Objects))

	if err := s.engine.DoSync(ctx, func(w *game.Live) {
		restoreObjects(w, c, f.Objects)
	}); err != nil {
		return false, err
	}

	// Turn it into a crash file, as the C does by rewinding and rewriting the
	// header (objsave.c:617): the same file cannot be un-rented twice, and a
	// player who unrents and then crashes still gets their things back.
	if err := s.objects.MarkCrashed(ctx, c.Name, time.Now()); err != nil {
		s.logger.Error("re-marking a rent file", "character", c.Name, "error", err)
	}
	return false, nil
}

// restoreObjects turns stored records back into objects in somebody's hands.
//
// Everything lands loose in inventory. That is not a simplification: with
// USE_AUTOEQ 0 the file has no location field, so the C cannot put anything
// back on your body or into your bags either. See the note in objfile.go.
//
// The file is walked backwards because the C's obj_to_char prepends and this
// port's appends. Same order out either way, and the file stays readable by
// the C server.
func restoreObjects(w *game.Live, c *game.Character, stored []player.StoredObject) {
	for i := len(stored) - 1; i >= 0; i-- {
		st := stored[i]
		obj := w.NewObject(st.Vnum)
		if obj == nil {
			// read_object returning NULL: the prototype has been deleted from
			// the world since the file was written. The C skips it silently
			// and so does this, because there is nothing to make.
			continue
		}
		// Only the fields the file stores are overwritten. Everything else —
		// the name, the descriptions, the wear flags, the cost — comes from
		// the prototype, which is why editing a zone file changes items
		// players are already carrying.
		obj.Values = st.Values
		obj.ExtraFlags = st.ExtraFlags
		obj.Weight = st.Weight
		obj.Timer = st.Timer
		obj.PermAffect = st.PermAffect
		obj.Affects = append([]game.ObjAffect(nil), st.Affects...)
		w.ObjectToChar(obj, c)
	}
}

// crashSave is Crash_crashsave (objsave.c:743): everything, saved for free,
// under the code that brings them back in the temple.
//
// Called on quit, on a dropped link, and on shutdown. It does not extract the
// objects — the C leaves that to extract_char — so a character who crash-saves
// and keeps playing still has their things.
func (s *Server) crashSave(ctx context.Context, c *game.Character) error {
	if s.objects == nil || c == nil || c.Record == nil {
		return nil
	}

	f := &player.RentFile{Code: player.RentCrash, Written: time.Now()}
	if err := s.engine.DoSync(ctx, func(_ *game.Live) {
		f.Gold, f.Bank = c.Record.Points.Gold, c.Record.Points.BankGold
		// Equipment first, in wear-position order, then inventory — the order
		// Crash_crashsave writes them in.
		for _, worn := range c.Equipment {
			f.Objects = appendForStorage(f.Objects, worn)
		}
		for i := len(c.Carrying) - 1; i >= 0; i-- {
			f.Objects = appendForStorage(f.Objects, c.Carrying[i])
		}
	}); err != nil {
		return err
	}

	if len(f.Objects) == 0 {
		// Nothing to store. The C writes the header anyway on this path and
		// only deletes on the idle-save one; deleting here is the same thing
		// to a reader and leaves no file behind for a naked character.
		return s.objects.DeleteObjects(ctx, c.Name)
	}
	return s.objects.SaveObjects(ctx, c.Name, f)
}

// appendForStorage flattens one object and anything inside it, in the order
// Crash_save's recursion produces: the rest of the list, then the contents,
// then the object itself (objsave.c:640).
//
// Contents come out *before* their container, and on the way back in they are
// no longer inside it — the file cannot say that they were.
func appendForStorage(out []player.StoredObject, obj *game.Object) []player.StoredObject {
	if obj == nil {
		return out
	}
	for i := len(obj.Contents) - 1; i >= 0; i-- {
		out = appendForStorage(out, obj.Contents[i])
	}
	return append(out, storedFrom(obj))
}

// storedFrom is Obj_to_store (objsave.c:99).
func storedFrom(obj *game.Object) player.StoredObject {
	st := player.StoredObject{
		Vnum:       game.NoObject,
		Values:     obj.Values,
		ExtraFlags: obj.ExtraFlags,
		Weight:     obj.Weight,
		Timer:      obj.Timer,
		PermAffect: obj.PermAffect,
		Affects:    make([]game.ObjAffect, game.MaxObjAffects),
	}
	if obj.Def != nil {
		st.Vnum = obj.Def.Vnum
	}
	copy(st.Affects, obj.Affects)
	return st
}

// SaveEverything writes every player's record and rent file, for shutdown.
func (s *Server) SaveEverything(ctx context.Context) {
	var players []*game.Character
	if err := s.engine.DoSync(ctx, func(w *game.Live) {
		players = append(players, w.Players()...)
	}); err != nil {
		s.logger.Error("collecting players to save", "error", err)
		return
	}
	for _, c := range players {
		if err := s.Save(ctx, c); err != nil {
			s.logger.Error("saving on shutdown", "character", c.Name, "error", err)
		}
	}
	s.crashSaveAll(ctx)
	s.SaveChangedHouses(ctx)
}

// RentCharacter is Crash_rentsave / Crash_cryosave (objsave.c:868, :914) plus
// the extract_char that follows them.
//
// The receptionist calls this on the world goroutine, so the file writing and
// the disconnect are pushed off it — the same reasoning as Save. The
// character's belongings are read on the world goroutine before that happens,
// because they are about to be destroyed.
func (s *Server) RentCharacter(w *game.Live, c *game.Character, mode session.RentMode, cost int32) {
	if c == nil || c.Record == nil {
		return
	}

	code := player.RentRented
	if mode == session.RentModeCryo {
		code = player.RentCryo
		// Cryo takes its fee once, up front, and stores no daily cost.
		c.Record.Points.Gold = max(0, c.Record.Points.Gold-cost)
		cost = 0
	}

	f := &player.RentFile{
		Code:       code,
		Written:    time.Now(),
		CostPerDay: cost,
		Gold:       c.Record.Points.Gold,
		Bank:       c.Record.Points.BankGold,
	}
	// Crash_extract_norent_eq and Crash_extract_norents run first, so an
	// unrentable item is destroyed rather than stored. The receptionist has
	// already refused the whole transaction if there was one, so this only
	// matters for a rent forced by something other than the desk.
	for _, worn := range c.Equipment {
		f.Objects = appendRentable(f.Objects, worn)
	}
	for i := len(c.Carrying) - 1; i >= 0; i-- {
		f.Objects = appendRentable(f.Objects, c.Carrying[i])
	}

	// Everything is destroyed: Crash_rentsave ends with Crash_extract_objs.
	// The file is the only copy now.
	for _, worn := range c.Equipment {
		w.ExtractObject(worn)
	}
	for _, obj := range append([]*game.Object(nil), c.Carrying...) {
		w.ExtractObject(obj)
	}

	// extract_char (objsave.c:1144). Done here, on the world goroutine,
	// rather than left to the session teardown: a renting character must not
	// be left standing as a linkdead body for somebody to reconnect to, and
	// relying on the socket closing first is a race that the tests lose.
	//
	// MarkQuit as well, so that if the teardown does get there it agrees.
	leaver, _ := c.Client.(interface {
		MarkQuit()
		MarkRented()
		Close()
	})
	if leaver != nil {
		leaver.MarkQuit()
		// And that the objects are already dealt with, so the disconnect
		// handling does not crash-save an empty character over the rent file
		// that was just written.
		leaver.MarkRented()
	}
	w.Remove(c)
	s.background(func() {
		ctx := context.Background()
		if err := s.objects.SaveObjects(ctx, c.Name, f); err != nil {
			s.logger.Error("writing a rent file", "character", c.Name, "error", err)
		}
		if err := s.Save(ctx, c); err != nil {
			s.logger.Error("saving a renting character", "character", c.Name, "error", err)
		}
		s.logger.Info("has rented", "character", c.Name,
			"mode", code.String(), "per_day", cost,
			"total", c.Record.Points.Gold+c.Record.Points.BankGold)
		// extract_char: they leave the game. Closing the session takes the
		// usual Leave path, which is what removes them from the world.
		if leaver != nil {
			leaver.Close()
		}
	})
}

// appendRentable is appendForStorage with the unrentables dropped, which is
// what Crash_extract_norents leaves behind.
func appendRentable(out []player.StoredObject, obj *game.Object) []player.StoredObject {
	if obj == nil {
		return out
	}
	for i := len(obj.Contents) - 1; i >= 0; i-- {
		out = appendRentable(out, obj.Contents[i])
	}
	if game.IsUnrentable(obj) {
		return out
	}
	return append(out, storedFrom(obj))
}

// crashSaveAll is Crash_save_all (objsave.c:1163), run on shutdown.
//
// The C only writes for characters with PLR_CRASH set, a bit raised whenever
// somebody picks something up. That is an optimisation for a machine that
// counted disk writes; this writes for everyone in the world, which is a few
// hundred small files at most and cannot miss anybody.
func (s *Server) crashSaveAll(ctx context.Context) {
	if s.objects == nil {
		return
	}
	var players []*game.Character
	if err := s.engine.DoSync(ctx, func(w *game.Live) {
		players = append(players, w.Players()...)
	}); err != nil {
		s.logger.Error("crash-saving everybody", "error", err)
		return
	}
	for _, c := range players {
		if err := s.crashSave(ctx, c); err != nil {
			s.logger.Error("crash-saving", "character", c.Name, "error", err)
		}
	}
}
