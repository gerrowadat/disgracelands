// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"context"
	"log/slog"
	"slices"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// What the server log says about a death (#370).
//
// Two halves, and they fail for different reasons. logDeath's own shape —
// which message, which attributes — is a pure function of the victim and
// the killer, and is tested as one. Whether the killer *arrives* is the
// plumbing through damage(), kill(), RawKill() and suffer(), and is tested
// by killing somebody for real and reading the log back, because a
// `s.kill(w, victim, nil)` slipped into any of those paths would pass every
// shape test there is.

// testLog is the handler every test server logs through: see the swap in
// newTestServerWith.
//
// Discarding by default, because a test server's log is noise, and
// recording once a test asks for it. Two things make it look the way it
// does rather than simpler:
//
//   - The switch is *inside* the handler rather than a test assigning a
//     fresh logger over Server.logger. The world goroutine, every
//     connection goroutine and every background write all log through that
//     field, so writing it under a running server is a data race — one
//     -race would find one run in ten, which CLAUDE.md's "never
//     flaky-until-proven" rule is exactly about.
//   - It cannot be reached back through Server.logger either. New() wraps
//     whatever it is given in obs.WithWizVisEcho, whose handler type is
//     unexported, so srv.logger.Handler() is that wrapper and there is no
//     way down to this. Hence testLogs below: the handler is registered
//     against the server it was built for, at the one moment both are in
//     scope.
type testLog struct {
	state *testLogState
	// with is what .With(...) added — session.New derives a per-connection
	// logger that way, so a handler that dropped these would quietly lose
	// attributes rather than fail.
	with []slog.Attr
}

type testLogState struct {
	// watching is atomic and read *before* the mutex, so a discarded
	// record costs one atomic load and nothing else. That is not
	// micro-optimisation: this handler stands in for an io.Discard that
	// contended with nothing, and taking a shared lock on every line
	// instead would serialise goroutines that never met before — the world
	// goroutine, every connection goroutine and every background write all
	// log through it. Changing the timing of every test in the package is
	// not a thing a test helper should do.
	watching atomic.Bool

	mu      sync.Mutex
	records []loggedRecord
}

// loggedRecord is one log line, flattened to strings: the tests here ask
// "what did it say", not "what type was it".
type loggedRecord struct {
	message string
	attrs   map[string]string
}

func newTestLog() *testLog { return &testLog{state: &testLogState{}} }

func (l *testLog) Enabled(context.Context, slog.Level) bool { return true }
func (l *testLog) WithGroup(string) slog.Handler            { return l }

func (l *testLog) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &testLog{state: l.state, with: append(slices.Clip(l.with), attrs...)}
}

func (l *testLog) Handle(_ context.Context, r slog.Record) error {
	if !l.state.watching.Load() {
		return nil
	}

	l.state.mu.Lock()
	defer l.state.mu.Unlock()

	rec := loggedRecord{message: r.Message, attrs: map[string]string{}}
	for _, a := range l.with {
		rec.attrs[a.Key] = a.Value.String()
	}
	r.Attrs(func(a slog.Attr) bool {
		rec.attrs[a.Key] = a.Value.String()
		return true
	})
	l.state.records = append(l.state.records, rec)
	return nil
}

// testLogs maps a test server to the handler underneath its logger. See
// testLog's own comment for why the link has to be kept here rather than
// recovered from the server.
var testLogs = struct {
	mu sync.Mutex
	m  map[*Server]*testLog
}{m: map[*Server]*testLog{}}

func registerTestLog(srv *Server, l *testLog) {
	testLogs.mu.Lock()
	defer testLogs.mu.Unlock()
	testLogs.m[srv] = l
}

// watchLog starts recording what srv logs. Everything before the call is
// discarded, so a test sees only what it caused.
func watchLog(t *testing.T, srv *Server) *testLogState {
	t.Helper()

	testLogs.mu.Lock()
	l, ok := testLogs.m[srv]
	testLogs.mu.Unlock()
	if !ok {
		t.Fatal("this server was not built by newTestServerWith, so nothing is recording its log")
	}
	t.Cleanup(func() {
		testLogs.mu.Lock()
		delete(testLogs.m, srv)
		testLogs.mu.Unlock()
	})

	l.state.mu.Lock()
	l.state.records = nil
	l.state.mu.Unlock()
	// After the reset, so nothing can be recorded and then thrown away.
	l.state.watching.Store(true)
	return l.state
}

// only returns the one record carrying the given message, and fails if
// there is not exactly one. "Exactly one" is the assertion that matters
// here: a second "a player died" for the same death would mean die() had
// been reached twice, which is the shape of bug the death cry has already
// had once (see die()'s own comment).
func (s *testLogState) only(t *testing.T, message string) loggedRecord {
	t.Helper()

	s.mu.Lock()
	defer s.mu.Unlock()

	var found []loggedRecord
	var messages []string
	for _, r := range s.records {
		messages = append(messages, r.message)
		if r.message == message {
			found = append(found, r)
		}
	}
	if len(found) != 1 {
		t.Fatalf("want exactly one %q in the log, got %d; the log said %q",
			message, len(found), messages)
	}
	return found[0]
}

// noneSaid fails if anything was logged with the given message.
func (s *testLogState) noneSaid(t *testing.T, message string) {
	t.Helper()

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range s.records {
		if r.message == message {
			t.Errorf("the log said %q, and should not have", message)
		}
	}
}

// wants checks the attributes a record must carry, and that it carries
// none of the ones it must not.
func (r loggedRecord) wants(t *testing.T, want map[string]string, absent ...string) {
	t.Helper()

	for k, v := range want {
		got, ok := r.attrs[k]
		if !ok {
			t.Errorf("%q has no %q attribute; it has %v", r.message, k, r.attrs)
			continue
		}
		if got != v {
			t.Errorf("%q has %s=%q, want %q", r.message, k, got, v)
		}
	}
	for _, k := range absent {
		if got, ok := r.attrs[k]; ok {
			t.Errorf("%q carries %s=%q, and should not", r.message, k, got)
		}
	}
}

// dog is a mobile to kill or be killed by, with the hit points the test
// needs and a prototype vnum, because naming the prototype is half of what
// #370 asked for.
func dog(name string, level, hit int32) *game.Character {
	return &game.Character{
		Name: name, Keywords: "dog", NPC: true,
		Position: game.PosStanding,
		MobDef:   &game.MobDef{Vnum: 999, ShortDesc: name, Keywords: "dog"},
		Record: &game.PlayerRecord{
			Name: name, Level: level,
			Points: game.Points{Hit: hit, MaxHit: hit},
		},
	}
}

// enterWorld puts a mobile into a room and tracks it, the way a zone reset
// would.
func enterWorld(t *testing.T, srv *Server, mob *game.Character, room game.RoomVnum) {
	t.Helper()
	if err := srv.engine.DoSync(context.Background(), func(w *game.Live) {
		if err := w.Enter(mob, room); err != nil {
			// Not t.Fatal: this is inside a world-goroutine closure, and
			// Fatal's runtime.Goexit would take the world goroutine with
			// it and hang every later DoSync in the binary.
			t.Errorf("placing %s: %v", mob.Name, err)
			return
		}
		w.Track(mob)
	}); err != nil {
		t.Fatal(err)
	}
}

// TestADeadPlayerIsLoggedAsAPlayerAndNamesTheMobileThatDidIt is the
// ordinary death: something in the world killed a character.
//
// Before #370 this line read "a character died", with the name and the room
// and nothing else — the same line a dead rat produced, and a log made
// mostly of dead rats is one nobody reads. The C's own mudlog does name a
// killer (fight.c:953) but only reaches immortals who are online and
// watching the syslog at the time, which is no help to an operator reading
// the log afterwards.
func TestADeadPlayerIsLoggedAsAPlayerAndNamesTheMobileThatDidIt(t *testing.T) {
	srv, _ := newTestServer(t)

	victim, _ := place(t, srv, fighterRecord("Welmar", 5, 20), MortalStartRoom)
	killer := dog("a large dog", 30, 500)
	enterWorld(t, srv, killer, MortalStartRoom)

	log := watchLog(t, srv)
	if err := srv.engine.DoSync(context.Background(), func(w *game.Live) {
		srv.Damage(w, killer, victim, 1000)
	}); err != nil {
		t.Fatal(err)
	}

	log.only(t, "a player died").wants(t, map[string]string{
		"character":   "Welmar",
		"killer":      "a large dog",
		"killer_type": "mobile",
		"killer_vnum": "999",
	},
		// A player has no prototype, so nothing may claim they have one.
		"vnum")
	log.noneSaid(t, "a mobile died")
}

// TestADeadMobileIsLoggedAsAMobileWithItsVnum is the other side of the same
// fight, and the case the C deliberately does not log at all
// (`if (!IS_NPC(victim))`, fight.c:938).
//
// The vnum is the point. A mobile's name is its short description, there
// are eleven "a large dog"s in a stock world, and "which one" is the first
// question anybody asks of a line saying one died.
func TestADeadMobileIsLoggedAsAMobileWithItsVnum(t *testing.T) {
	srv, _ := newTestServer(t)

	killer, _ := place(t, srv, fighterRecord("Zod", 30, 500), MortalStartRoom)
	victim := dog("a large dog", 5, 20)
	enterWorld(t, srv, victim, MortalStartRoom)

	log := watchLog(t, srv)
	if err := srv.engine.DoSync(context.Background(), func(w *game.Live) {
		srv.Damage(w, killer, victim, 1000)
	}); err != nil {
		t.Fatal(err)
	}

	log.only(t, "a mobile died").wants(t, map[string]string{
		"character":   "a large dog",
		"vnum":        "999",
		"killer":      "Zod",
		"killer_type": "player",
	},
		// A player killer has no prototype either.
		"killer_vnum")
	log.noneSaid(t, "a player died")
}

// TestAnImplementorsKillNamesTheImplementor is the one death the C never
// attributes to anybody.
//
// `kill` reaches raw_kill directly (act.offensive.c:96), and raw_kill takes
// only the victim — the line that names a killer lives in damage(), which
// this path goes instead of. So a god quietly slaying somebody was, in the
// C and in this port until #370, indistinguishable in the log from a death
// with no cause at all. That is precisely the death an operator is most
// likely to be asked about.
func TestAnImplementorsKillNamesTheImplementor(t *testing.T) {
	srv, _ := newTestServer(t)

	implementor, _ := place(t, srv, fighterRecord("Zod", 34, 500), MortalStartRoom)
	victim, _ := place(t, srv, fighterRecord("Welmar", 5, 200), MortalStartRoom)

	log := watchLog(t, srv)
	if err := srv.engine.DoSync(context.Background(), func(w *game.Live) {
		srv.RawKill(w, implementor, victim)
	}); err != nil {
		t.Fatal(err)
	}

	log.only(t, "a player died").wants(t, map[string]string{
		"character":   "Welmar",
		"killer":      "Zod",
		"killer_type": "player",
	})
}

// TestBleedingToDeathNamesNobody: a character with nothing attacking them.
//
// suffer() is point_update's damage(ch, ch, n, TYPE_SUFFERING) — poison and
// bleeding out — and the C's spelling would make the victim their own
// killer. The log says nothing instead: "killed by Welmar" for a Welmar who
// bled out is worse than silence, and the absence of a killer is itself the
// information that nothing was attacking them.
func TestBleedingToDeathNamesNobody(t *testing.T) {
	srv, _ := newTestServer(t)

	victim, _ := place(t, srv, fighterRecord("Welmar", 5, 5), MortalStartRoom)

	log := watchLog(t, srv)
	if err := srv.engine.DoSync(context.Background(), func(w *game.Live) {
		srv.suffer(w, victim, 100)
	}); err != nil {
		t.Fatal(err)
	}

	log.only(t, "a player died").wants(t, map[string]string{"character": "Welmar"},
		"killer", "killer_type", "killer_vnum")
}

// TestTheOldUndifferentiatedDeathLineIsGone is a guard on the line #370 was
// filed about, rather than on any of the lines that replaced it.
//
// The four tests above would all still pass if "a character died" were
// logged *as well*, and an operator's grep would still be finding it. This
// is the assertion that it is not there at all.
func TestTheOldUndifferentiatedDeathLineIsGone(t *testing.T) {
	srv, _ := newTestServer(t)

	victim, _ := place(t, srv, fighterRecord("Welmar", 5, 20), MortalStartRoom)
	killer := dog("a large dog", 30, 500)
	enterWorld(t, srv, killer, MortalStartRoom)

	log := watchLog(t, srv)
	if err := srv.engine.DoSync(context.Background(), func(w *game.Live) {
		srv.Damage(w, killer, victim, 1000)
	}); err != nil {
		t.Fatal(err)
	}

	log.noneSaid(t, "a character died")
}
