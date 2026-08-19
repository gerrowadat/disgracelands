// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

// Package server joins the pieces: the world, the player store, credentials
// and connections.
//
// It is what a session talks to when it needs something outside its own
// socket, and it is the only place that knows about all of them at once.
package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/gerrowadat/disgracelands/internal/auth"
	"github.com/gerrowadat/disgracelands/internal/engine"
	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/persist/player"
	"github.com/gerrowadat/disgracelands/internal/rng"
	"github.com/gerrowadat/disgracelands/internal/session"
)

// Start rooms. The numbers live in the game package, where word of recall can
// also reach them; these are here because most of the server refers to them.
const (
	MortalStartRoom = game.MortalStartRoom
	ImmortStartRoom = game.ImmortStartRoom
)

// autosaveInterval matches the C's PULSE_AUTOSAVE.
const autosaveInterval = 60 * time.Second

// linkdeadTimeout is how long a character whose connection dropped stays
// standing before being saved and removed.
//
// The C keeps them until an idle timeout, so a dropped connection can be
// resumed mid-fight without losing position. Two minutes is long enough for a
// reconnect and short enough that a body is not left to be killed while
// nobody is controlling it.
const linkdeadTimeout = 2 * time.Minute

// Server holds everything a session needs from the rest of the world.
type Server struct {
	engine  *engine.Engine
	players player.Store
	auth    auth.Verifier
	text    *Text
	logger  *slog.Logger

	// restrict refuses new characters, matching the C's -r.
	restrict bool

	rng *rng.Rand

	// Zone ageing state. Touched only from the world goroutine, which is
	// where every periodic runs.
	zones     map[int]*zoneState
	zoneTicks int32
}

// Options configure a Server.
type Options struct {
	Engine   *engine.Engine
	Players  player.Store
	Auth     auth.Verifier
	Text     *Text
	Logger   *slog.Logger
	Restrict bool
	// RNG is the generator the game rolls on. A nil one gets the modern
	// generator seeded from the clock.
	RNG *rng.Rand
}

// New creates a Server.
func New(opts Options) *Server {
	s := &Server{
		engine:   opts.Engine,
		players:  opts.Players,
		auth:     opts.Auth,
		text:     opts.Text,
		logger:   opts.Logger,
		restrict: opts.Restrict,
		rng:      opts.RNG,
	}
	if s.rng == nil {
		s.rng = rng.NewRand(rng.NewModern(uint64(time.Now().UnixNano()))) //nolint:gosec // a game seed, not a secret
	}
	return s
}

// Text returns the canned files.
func (s *Server) Text() *Text { return s.text }

// Exists implements session.LoginHandler.
func (s *Server) Exists(ctx context.Context, name string) (bool, error) {
	return s.players.Exists(ctx, name)
}

// Authenticate implements session.LoginHandler.
//
// On a correct legacy password the credential is upgraded and saved before
// the character is returned. That is the only moment the plaintext is known,
// so it is the only moment the upgrade can happen — see
// docs/proposals/go-port-plan.md §5.3.1.
func (s *Server) Authenticate(ctx context.Context, name, password string) (*game.Character, error) {
	rec, err := s.players.Load(ctx, name)
	if errors.Is(err, player.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	result, err := s.auth.Verify(rec.Credential, rec.Name, password)
	if err != nil {
		if errors.Is(err, auth.ErrLegacyRefused) {
			// Distinguishable from a wrong password, because it is not one:
			// this character simply cannot log in until legacy verification
			// is turned back on.
			return nil, err
		}
		return nil, err
	}
	if !result.OK {
		return nil, nil
	}

	if result.Upgraded != nil {
		rec.Credential = *result.Upgraded
		if err := s.players.Save(ctx, rec); err != nil {
			// Not fatal: they typed the right password and should get in.
			// They will simply be asked to upgrade again next time.
			s.logger.Error("saving an upgraded credential", "character", rec.Name, "error", err)
		} else {
			s.logger.Info("upgraded a legacy password", "character", rec.Name)
		}
	}

	return &game.Character{Name: rec.Name, Record: rec}, nil
}

// Create implements session.LoginHandler.
func (s *Server) Create(ctx context.Context, req session.CreateRequest) (*game.Character, error) {
	if s.restrict {
		return nil, fmt.Errorf("the game is not accepting new characters")
	}

	cred, err := auth.NewCredential(req.Password)
	if err != nil {
		return nil, err
	}

	// The first character created on an empty roster becomes the
	// Implementor. This is stock CircleMUD behaviour (db.c's "if this is our
	// first player --- he be God") and it is how a fresh install is
	// bootstrapped; README.md says so, and dropping it would leave a new
	// server with no way to administer itself.
	first, err := s.isRosterEmpty(ctx)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	rec := &game.PlayerRecord{
		Name:       req.Name,
		Sex:        req.Sex,
		Class:      req.Class,
		Birth:      now,
		LastLogon:  now,
		Credential: cred,
	}

	// init_char, then the local block the C runs at the class prompt. Note
	// what is *not* here: do_start. The C defers it until the player actually
	// enters the world, and an Implementor never runs it at all — see
	// game.InitChar for why that distinction matters.
	game.InitChar(rec, s.rng, first)
	game.ApplyNewCharacterDefaults(rec)

	if first {
		s.logger.Info("first character on an empty roster; promoting to Implementor",
			"character", rec.Name, "level", rec.Level)
	}

	if err := s.players.Save(ctx, rec); err != nil {
		return nil, err
	}
	return &game.Character{Name: rec.Name, Record: rec}, nil
}

func (s *Server) isRosterEmpty(ctx context.Context) (bool, error) {
	for _, err := range s.players.List(ctx) {
		if err != nil {
			return false, err
		}
		return false, nil
	}
	return true, nil
}

// Reconnect implements session.LoginHandler.
func (s *Server) Reconnect(ctx context.Context, name string) *game.Character {
	var found *game.Character
	_ = s.engine.DoSync(ctx, func(w *game.Live) {
		if c := w.Find(name); c != nil && c.Client == nil {
			found = c
		}
	})
	return found
}

// Enter implements session.LoginHandler: puts a character into the world.
func (s *Server) Enter(ctx context.Context, sess *session.Session, c *game.Character) error {
	room := MortalStartRoom
	if c.Record != nil {
		if c.Record.Level >= game.LevelImmortal {
			room = ImmortStartRoom
		}
		if c.Record.LoadRoom != game.NoRoom && c.Record.LoadRoom != 0 {
			room = c.Record.LoadRoom
		}
		c.Record.LastLogon = time.Now().UTC()
		c.Record.Host = sess.Host()

		// do_start, at the moment the C runs it: after the load room has been
		// chosen from the level, and only for a character who has never been
		// in the world (interpreter.c:1684). It rolls the abilities, sets the
		// starting points, skills and conditions, and applies the first
		// level's gains.
		if c.Record.Level == 0 {
			game.Start(c.Record, s.rng)
			s.logger.Info("new character entering the world for the first time",
				"character", c.Name, "class", game.ClassName(c.Record.Class))
			if err := s.players.Save(ctx, c.Record); err != nil {
				// The C saves here too. A failure is worth reporting but not
				// worth refusing them entry over: the autosave will catch it.
				s.logger.Error("saving a newly started character", "character", c.Name, "error", err)
			}
		}
	}
	c.Client = sess
	// The C stores POS_STANDING for everyone on load and lets update_pos
	// sort out anyone who should not be — which it will, on the next tick,
	// for a character who logged out while dying.
	c.Position = game.PosStanding
	if c.Record != nil {
		c.Position = game.UpdatePosition(c.Record, c.Position)
	}

	return s.engine.DoSync(ctx, func(w *game.Live) {
		if w.Room(room) == nil {
			// A character whose saved room has been deleted from the world
			// still has to get in somewhere.
			s.logger.Warn("start room does not exist; using the default",
				"character", c.Name, "room", room)
			room = MortalStartRoom
		}
		if err := w.Enter(c, room); err != nil {
			s.logger.Error("entering the world", "character", c.Name, "error", err)
			return
		}
		for _, other := range w.Occupants(room) {
			if other != c {
				other.Tell("%s has entered the game.\r\n", c.Name)
			}
		}
	})
}

// Leave implements session.LoginHandler.
//
// A character whose connection dropped is left standing, with no client, so
// it can be reconnected to — the C's CON_DISCONNECT behaviour. One that quit
// is removed outright. Either way the record is saved first.
func (s *Server) Leave(ctx context.Context, sess *session.Session, c *game.Character) error {
	quit := sess.Quit()

	if err := s.Save(ctx, c); err != nil {
		s.logger.Error("saving on disconnect", "character", c.Name, "error", err)
	}

	return s.engine.DoSync(ctx, func(w *game.Live) {
		if c.Client == sess {
			c.Client = nil
		}
		for _, other := range w.Occupants(c.Room) {
			if other != c {
				if quit {
					other.Tell("%s has left the game.\r\n", c.Name)
				} else {
					other.Tell("%s has lost their link.\r\n", c.Name)
				}
			}
		}
		if quit {
			w.Remove(c)
		}
	})
}

// Save writes a character's record back.
//
// It runs off the world goroutine deliberately: the record is read on that
// goroutine and written here, so a slow disk cannot stall the game.
func (s *Server) Save(ctx context.Context, c *game.Character) error {
	if c == nil || c.Record == nil {
		return nil
	}
	var snapshot game.PlayerRecord
	if err := s.engine.DoSync(ctx, func(w *game.Live) {
		snapshot = *c.Record
		if w.Room(c.Room) != nil {
			snapshot.LoadRoom = c.Room
		}
	}); err != nil {
		return err
	}
	return s.players.Save(ctx, &snapshot)
}

// RunAutosave saves every character periodically until ctx is cancelled, and
// reaps linkdead bodies that nobody came back for.
func (s *Server) RunAutosave(ctx context.Context) {
	ticker := time.NewTicker(autosaveInterval)
	defer ticker.Stop()

	linkdeadSince := map[*game.Character]time.Time{}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			var all []*game.Character
			if err := s.engine.DoSync(ctx, func(w *game.Live) {
				all = w.Players()
			}); err != nil {
				return
			}

			now := time.Now()
			for _, c := range all {
				if err := s.Save(ctx, c); err != nil {
					s.logger.Error("autosave failed", "character", c.Name, "error", err)
				}

				if c.Client != nil {
					delete(linkdeadSince, c)
					continue
				}
				if _, seen := linkdeadSince[c]; !seen {
					linkdeadSince[c] = now
					continue
				}
				if now.Sub(linkdeadSince[c]) < linkdeadTimeout {
					continue
				}
				delete(linkdeadSince, c)
				s.logger.Info("reaping a linkdead character", "character", c.Name)
				_ = s.engine.DoSync(ctx, func(w *game.Live) { w.Remove(c) })
			}
		}
	}
}

// CheckPassword implements session.LoginHandler.
//
// This is not Authenticate: the character is already logged in, and the menu
// is asking them to prove it again before changing a password or deleting
// themselves. A legacy credential is upgraded here too — it is still the only
// moment the plaintext is known.
func (s *Server) CheckPassword(ctx context.Context, c *game.Character, password string) (bool, error) {
	if c == nil || c.Record == nil {
		return false, nil
	}

	result, err := s.auth.Verify(c.Record.Credential, c.Record.Name, password)
	if err != nil {
		return false, err
	}
	if !result.OK {
		return false, nil
	}
	if result.Upgraded != nil {
		c.Record.Credential = *result.Upgraded
		if err := s.players.Save(ctx, c.Record); err != nil {
			s.logger.Error("saving an upgraded credential", "character", c.Name, "error", err)
		}
	}
	return true, nil
}

// SetPassword implements session.LoginHandler.
func (s *Server) SetPassword(ctx context.Context, c *game.Character, password string) error {
	if c == nil || c.Record == nil {
		return fmt.Errorf("no character to set a password for")
	}
	cred, err := auth.NewCredential(password)
	if err != nil {
		return err
	}
	c.Record.Credential = cred
	return s.players.Save(ctx, c.Record)
}

// Delete implements session.LoginHandler.
//
// The C sets PLR_DELETED and saves, because a record in the binary format is
// a slot in a fixed-width file and the flag is what marks the slot reusable
// (interpreter.c:1770). The ascii format has no slots — a character is a file
// — so the file is removed, which is the same intent expressed in the format
// actually in use. See docs/deviations.md.
//
// The C's level check is kept: a greater god or implementor who self-deletes
// is disconnected but not removed, which reads like a safety valve and is
// worth having whether it was meant as one or not.
func (s *Server) Delete(ctx context.Context, c *game.Character) error {
	if c == nil || c.Record == nil {
		return fmt.Errorf("no character to delete")
	}

	if c.Record.Level >= game.LevelGreaterGod {
		s.logger.Warn("refusing to delete a character of greater-god level or above",
			"character", c.Name, "level", c.Record.Level)
		c.Record.PlayerFlags = c.Record.PlayerFlags.Clear(game.PlayerDeleted)
		return s.players.Save(ctx, c.Record)
	}

	// Recorded on the way out as well as removed, so a restored backup of the
	// roster does not quietly bring them back without anyone knowing why.
	c.Record.PlayerFlags = c.Record.PlayerFlags.Set(game.PlayerDeleted)
	return s.players.Delete(ctx, c.Record.Name)
}
