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
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gerrowadat/disgracelands/internal/auth"
	"github.com/gerrowadat/disgracelands/internal/engine"
	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/obs"
	"github.com/gerrowadat/disgracelands/internal/persist/bans"
	"github.com/gerrowadat/disgracelands/internal/persist/boards"
	"github.com/gerrowadat/disgracelands/internal/persist/houses"
	"github.com/gerrowadat/disgracelands/internal/persist/mail"
	"github.com/gerrowadat/disgracelands/internal/persist/player"
	"github.com/gerrowadat/disgracelands/internal/persist/reports"
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

// clockSaveInterval matches PULSE_TIMESAVE (structs.h:519): thirty real
// minutes, deliberately no finer than the precision a save keeps — see
// docs/weirdnumbers.md's "Saving the clock loses up to an hour, on
// purpose".
const clockSaveInterval = 30 * time.Minute

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
	// objects holds the rent files. Separate from players because it is a
	// separate format with a separate failure mode: a roster that will not
	// load stops the server, and a rent file that will not load costs one
	// player their backpack. Nil disables rent entirely.
	objects player.ObjectStore
	// boards holds the bulletin board files. Nil disables boards, which is
	// what a test world without them gets.
	boards boards.Store
	// mail is the mud mail file. Nil disables the mail system, which is what
	// the C's `no_mail` global does when the file goes wrong.
	mail mail.Store
	// houses is the player housing files. Nil disables housing.
	houses houses.Store
	// bans is the site ban list. Nil disables banning.
	bans bans.Store
	// reports is the bug/idea/typo log. Nil disables `bug`/`idea`/`typo`,
	// the same "nothing configured" posture as boards/mail/houses/bans.
	reports reports.Store
	// names is the xnames disallowed-substring list. Empty (not nil-checked
	// specially) means nothing is disallowed, matching Valid_Name's own
	// posture when num_invalid is 0 (ban.c:263-264).
	names []string
	// clockFormat/clockPath locate the persisted mud-clock epoch
	// (internal/persist/clock). An empty clockPath disables persistence —
	// the clock still runs, it just always starts from time.Now() the way
	// it always used to, which is what a test world gets by default.
	clockFormat string
	clockPath   string
	// worldFormat/worldDir/worldMini locate the world data on disk, kept
	// around after boot (the Source itself is closed once Live exists)
	// so `reloadmob` can re-open the same source and read a definition
	// back afresh. An empty worldDir disables it — the same posture
	// clockPath's absence already takes for clock persistence, and what
	// a test world gets by default.
	worldFormat string
	worldDir    string
	worldMini   bool
	auth        auth.Verifier
	text        *Text
	logger      *slog.Logger

	// restrict refuses new characters, matching the C's -r.
	restrict bool
	// wizlock is the C's `circle_restrict`: the minimum level allowed in,
	// set at runtime by the `wizlock` command.
	wizlock atomic.Int32

	// connections is every live session, for `users` and `dc`.
	connections registry

	// booted is when the server came up, for `uptime`.
	booted time.Time

	// The shutdown switch. A god asking to stop closes the channel; main
	// waits on it and runs the ordinary shutdown.
	shutdownWanted chan struct{}
	shutdownOnce   sync.Once
	rebootWanted   atomic.Bool
	// noSpecials suppresses special procedures, matching the C's -s.
	noSpecials bool
	// roundLength is how long a combat round lasts, and so how long a wait
	// state holds somebody's next command up for. Zero is the real two
	// seconds; see session.DefaultRoundLength.
	roundLength time.Duration

	rng *rng.Rand

	// writes counts the background write goroutines — the saves that are
	// deliberately pushed off the world goroutine so a slow disk cannot stall
	// the game. Nothing waits for them during play; shutdown and the tests
	// both do.
	writes sync.WaitGroup

	// Zone ageing state. Touched only from the world goroutine, which is
	// where every periodic runs.
	zones     map[int]*zoneState
	zoneTicks int32
}

// Options configure a Server.
type Options struct {
	Engine  *engine.Engine
	Players player.Store
	Objects player.ObjectStore
	Boards  boards.Store
	Mail    mail.Store
	Houses  houses.Store
	Bans    bans.Store
	// Reports is the bug/idea/typo log (see Server.reports).
	Reports reports.Store
	// Names is the xnames disallowed-substring list (see Server.names).
	Names []string
	// ClockFormat/ClockPath locate the persisted mud-clock epoch (see
	// Server.clockFormat/clockPath). Loading the epoch itself happens
	// before a Server exists (it has to be applied to the *game.Live
	// before anyone can see the clock) — these are only where a save
	// later goes back to.
	ClockFormat string
	ClockPath   string
	// WorldFormat/WorldDir/WorldMini locate the world data on disk (see
	// Server.worldFormat/worldDir/worldMini) — for `reloadmob` to
	// re-open the same world.Source cmd/dlmud/main.go's own boot path
	// used, and read one definition back afresh.
	WorldFormat string
	WorldDir    string
	WorldMini   bool
	Auth        auth.Verifier
	Text        *Text
	Logger      *slog.Logger
	Restrict    bool
	// NoSpecials suppresses special procedures (C: -s).
	NoSpecials bool
	// RoundLength overrides how long a combat round lasts. Zero — which is
	// every caller that is not a test — means the real two seconds, which
	// is what PULSE_VIOLENCE is. See session.DefaultRoundLength.
	RoundLength time.Duration
	// RNG is the generator the game rolls on. A nil one gets the modern
	// generator seeded from the clock.
	RNG *rng.Rand
}

// New creates a Server.
func New(opts Options) *Server {
	s := &Server{
		engine:      opts.Engine,
		players:     opts.Players,
		objects:     opts.Objects,
		boards:      opts.Boards,
		mail:        opts.Mail,
		houses:      opts.Houses,
		bans:        opts.Bans,
		reports:     opts.Reports,
		names:       opts.Names,
		clockFormat: opts.ClockFormat,
		clockPath:   opts.ClockPath,
		worldFormat: opts.WorldFormat,
		worldDir:    opts.WorldDir,
		worldMini:   opts.WorldMini,
		auth:        opts.Auth,
		text:        opts.Text,
		logger:      opts.Logger,
		restrict:    opts.Restrict,
		noSpecials:  opts.NoSpecials,
		roundLength: opts.RoundLength,
		rng:         opts.RNG,
	}
	s.booted = time.Now()
	s.shutdownWanted = make(chan struct{})
	if s.rng == nil {
		s.rng = rng.NewRand(rng.NewModern(uint64(time.Now().UnixNano()))) //nolint:gosec // a game seed, not a secret
	}
	// mudlog()'s own in-game echo (utils.c:243-258): wraps whatever handler
	// opts.Logger already had so every log call site keeps working exactly
	// as before, and a wizvis-tagged one additionally reaches echoWizVis.
	// Built here, after s exists, rather than passed in — s.connections is
	// what echoWizVis needs to reach, and there is no Server yet at the
	// point main.go builds the logger.
	if s.logger != nil {
		s.logger = slog.New(obs.WithWizVisEcho(s.logger.Handler(), s.echoWizVis))
	}
	return s
}

// background runs a write off the world goroutine, counted so that shutdown
// and the tests can wait for it.
//
// Every save in this package goes through here. The reason is durability at
// one end and tidiness at the other: a process that exits with a save in
// flight loses it, and a test whose t.TempDir() is removed with a save in
// flight fails on "directory not empty" in whichever *other* test the
// scheduler happens to be running by then.
func (s *Server) background(f func()) {
	s.writes.Add(1)
	go func() {
		defer s.writes.Done()
		f()
	}()
}

// WaitForWrites blocks until every background write has finished.
func (s *Server) WaitForWrites() { s.writes.Wait() }

// Text returns the canned files.
func (s *Server) Text() *Text { return s.text }

// TextField implements session.TextEditor: tedit's own read of a canned
// text file's current content.
func (s *Server) TextField(name string) (string, bool) { return s.text.TextField(name) }

// SetTextField implements session.TextEditor: tedit's own save. The
// in-memory update happens now, so every other command sees it
// immediately (the same posture SaveBoard's caller already takes,
// mutating the board before calling this at all); the disk write is
// pushed off the world goroutine, mirroring SaveBoard exactly.
func (s *Server) SetTextField(name, text string) bool {
	path, ok := s.text.SetTextField(name, text)
	if !ok {
		return false
	}
	s.background(func() {
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil { //nolint:gosec // operator-configured data directory
			s.logger.Error("writing a canned text file", "file", path, "error", err)
		}
	})
	return true
}

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
	// `-r` on the command line, and `wizlock 1` or higher at runtime: both
	// close the door to new characters. The C keeps them as separate globals
	// and tests them in different places; one check covers both.
	if s.restrict || s.wizlock.Load() >= 1 {
		return nil, fmt.Errorf("the game is not accepting new characters")
	}

	cred, err := s.auth.NewCredential(req.Password)
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

	// A unique id, which is `player_table[i].id = GET_IDNUM(ch) = ++top_idnum`
	// in init_char (db.c:2746). The C keeps the high-water mark in a global
	// seeded from the roster at boot; this asks the roster directly, which is
	// the same answer and does not need the global.
	//
	// It had been left at zero, which is how it was found: mail addressed by
	// id went to whoever the listing happened to name first, and `reply`
	// would have had the same problem.
	idnum, err := s.nextIDNum(ctx)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	rec := &game.PlayerRecord{
		Name:       req.Name,
		IDNum:      idnum,
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

// nextIDNum returns one more than the highest id on the roster, porting
// `++top_idnum`.
//
// The C computes the high-water mark once at boot (db.c:611) and increments a
// global thereafter. Walking the roster costs one listing per character
// created, which happens once per player ever.
func (s *Server) nextIDNum(ctx context.Context) (int64, error) {
	var top int64
	for entry, err := range s.players.List(ctx) {
		if err != nil {
			return 0, err
		}
		if entry.IDNum > top {
			top = entry.IDNum
		}
	}
	return top + 1, nil
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

// AllowedIn reports whether somebody of this level may enter, porting the
// `circle_restrict` check in nanny (interpreter.c).
//
// A wizlock of 1 closes the game to *new* characters only; anything higher
// is a level threshold and keeps existing ones out too.
func (s *Server) AllowedIn(level int32) bool {
	lock := s.wizlock.Load()
	return lock <= 1 || level >= lock
}

// Enter implements session.LoginHandler: puts a character into the world.
func (s *Server) Enter(ctx context.Context, sess *session.Session, c *game.Character) (session.EnterResult, error) {
	var result session.EnterResult
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

	if err := s.engine.DoSync(ctx, func(w *game.Live) {
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
	}); err != nil {
		return result, err
	}

	// Crash_load, after the character is in a room: the C's comment says
	// "We have to place the character in a room before equipping them or
	// equip_char() will gripe about the person in NOWHERE"
	// (interpreter.c:1648).
	lost, err := s.loadObjects(ctx, c)
	if err != nil {
		return result, err
	}
	result.RentLost = lost

	// Clear the load room unless it is meant to persist (interpreter.c:1676).
	//
	// Without this everybody comes back exactly where they logged out, which
	// is not how the game worked: you woke up in the temple unless a god had
	// set PLR_LOADROOM on you. The port had been keeping it for everyone.
	if c.Record != nil && !c.Record.PlayerFlags.Has(game.PlayerLoadRoom) {
		c.Record.LoadRoom = game.NoRoom
	}

	return result, nil
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
	// Crash_crashsave, as do_quit does (act.other.c:201). Free, and it brings
	// them back in the temple — renting at an inn is what buys anything else.
	// Done for a dropped link too: the C waits for the idle timeout to force
	// a rent, and until that lands this is what stops a link loss costing
	// somebody everything they were carrying.
	//
	// Not for somebody who has just rented: their things are already in the
	// rent file and they are carrying nothing, so a crash-save here would
	// write an empty file over it. The C's extract_char does not crash-save
	// and so never had to think about it.
	if !sess.Rented() {
		if err := s.crashSave(ctx, c); err != nil {
			s.logger.Error("crash-saving on disconnect", "character", c.Name, "error", err)
		}
	}
	// House_crashsave for the room they left from, as do_quit does
	// (act.other.c:203): anything dropped in a house before quitting is
	// theirs to find again.
	s.SaveChangedHouses(ctx)

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
	// save_char's first line is `if (IS_NPC(ch) || ...) return;` (db.c:2206),
	// and it is not defensive programming — it is load-bearing. A mobile has
	// a PlayerRecord like anybody else, so without this guard `set dog str
	// 18` writes a player file called "a large dog", the index rebuild picks
	// it up, and its spaces make the index unparseable. Every login and every
	// character creation then fails, for everybody.
	if c.IsNPC() {
		return nil
	}
	var snapshot game.PlayerRecord
	if err := s.engine.DoSync(ctx, func(_ *game.Live) {
		snapshot = *c.Record
		// LoadRoom is *not* set from the current room. save_char writes
		// whatever is on the record and nothing else touches it: the
		// receptionist sets it when you rent (objsave.c:1143), and the entry
		// sequence clears it again once it has been used
		// (interpreter.c:1676). Writing the current room here — which this
		// did — made every save a persistent load room and quietly undid
		// both.
	}); err != nil {
		return err
	}
	return s.players.Save(ctx, &snapshot)
}

// tickAutosave decides whether one PULSE_AUTOSAVE tick should trigger a save
// sweep, porting comm.c:928-929's two-part check: does autosave run at all,
// and has autosave_time minutes' worth of ticks passed. minsSinceCrashsave is
// the caller's counter, reset to 0 only on a tick that returns true — the
// same "counter only moves forward while gated on" shape the C's static
// mins_since_crashsave has. Split out from RunAutosave so the counter logic
// has a test that does not need a real 60-second ticker.
func tickAutosave(tuning game.GameTuning, minsSinceCrashsave *int32) bool {
	if !tuning.AutoSave {
		return false
	}
	*minsSinceCrashsave++
	if *minsSinceCrashsave < tuning.AutosaveTime {
		return false
	}
	*minsSinceCrashsave = 0
	return true
}

// RunAutosave saves every character periodically until ctx is cancelled, and
// reaps linkdead bodies that nobody came back for.
//
// autosaveInterval (PULSE_AUTOSAVE, 60s) is the tick this loop runs on and is
// not itself tunable; game.Tuning()'s AutoSave and AutosaveTime gate the
// save sweep within it, porting comm.c:928-929's own two-part check ("does
// autosave run at all", "has autosave_time minutes' worth of ticks passed")
// exactly, down to the counter resetting only on a tick that actually saves.
// Linkdead reaping below is a separate mechanism (config.c's idle_void/
// idle_rent_time, not reopened for tunability) and runs every tick
// regardless.
func (s *Server) RunAutosave(ctx context.Context) {
	ticker := time.NewTicker(autosaveInterval)
	defer ticker.Stop()

	linkdeadSince := map[*game.Character]time.Time{}
	var minsSinceCrashsave int32

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

			doSave := tickAutosave(game.Tuning(), &minsSinceCrashsave)

			now := time.Now()
			for _, c := range all {
				if doSave {
					if err := s.Save(ctx, c); err != nil {
						s.logger.Error("autosave failed", "character", c.Name, "error", err)
					}
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
	cred, err := s.auth.NewCredential(password)
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
