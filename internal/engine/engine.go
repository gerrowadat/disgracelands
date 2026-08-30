// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

// Package engine runs the game loop.
//
// One goroutine owns the world. Everything that reads or writes it arrives
// here as a function to run, and runs in turn — so nothing in internal/game
// needs a lock, and the C code's habit of passing pointers between rooms,
// characters and objects ports across intact. See
// docs/design/go-port-plan.md §3.1 for why this and not an
// actor-per-entity design.
//
// The loop pulses at a fixed interval, 100ms by default, matching the C
// server's OPT_USEC. Everything timed in the game is a multiple of it.
package engine

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync/atomic"
	"time"

	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/obs"
)

// Task is work to run on the world goroutine.
type Task func(w *game.Live)

// Engine is the game loop.
type Engine struct {
	world  *game.Live
	tasks  chan Task
	logger *slog.Logger

	interval time.Duration
	metrics  *obs.Metrics
	periodic []Periodic

	// pulse counts elapsed ticks. Everything periodic is scheduled against
	// it, the way the C server's PULSE_* constants are.
	//
	// It counts *elapsed* ticks and not received ones, which is the whole
	// of the difference #321 was about — see tick.
	pulse atomic.Uint64

	// started is when Run began, and is what pulse is measured from. Owned
	// by the loop goroutine and written before it starts ticking.
	started time.Time

	// missed counts pulses that went by without the loop being there to
	// take them. Read by Missed, for a caller reporting it.
	missed atomic.Uint64
}

// Options configure an Engine.
type Options struct {
	World    *game.Live
	Interval time.Duration
	Logger   *slog.Logger
	Metrics  *obs.Metrics

	// QueueDepth bounds the pending task queue. A full queue means the world
	// goroutine is not keeping up, which is a condition to notice rather
	// than to absorb silently.
	QueueDepth int

	// Periodic is work to run on a schedule, in pulses. Everything the C
	// hangs off its PULSE_* constants goes here, and each entry runs on the
	// world goroutine like any other task.
	Periodic []Periodic
}

// Periodic is work the game loop runs every so often.
//
// The C schedules everything this way — heartbeat() counts pulses and calls
// point_update, mobile_activity, perform_violence and zone_update when the
// count divides evenly (comm.c). Naming the entries rather than writing a
// chain of modulo tests means a slow one can be reported by name.
type Periodic struct {
	// Name identifies it in logs and metrics.
	Name string
	// Every is how many pulses apart it runs. Zero or less never runs.
	Every uint64
	// Run does the work, on the world goroutine.
	Run func(w *game.Live)
}

// New creates an Engine.
func New(opts Options) *Engine {
	if opts.QueueDepth <= 0 {
		opts.QueueDepth = 1024
	}
	if opts.Interval <= 0 {
		opts.Interval = 100 * time.Millisecond
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	return &Engine{
		world:    opts.World,
		tasks:    make(chan Task, opts.QueueDepth),
		logger:   opts.Logger,
		interval: opts.Interval,
		metrics:  opts.Metrics,
		periodic: opts.Periodic,
	}
}

// SetPeriodic replaces the scheduled work. It must be called before Run: the
// schedule is read from the game loop's goroutine and changing it under a
// running loop would be a data race.
func (e *Engine) SetPeriodic(p []Periodic) { e.periodic = p }

// ErrBusy is returned when the world goroutine is too far behind to accept
// more work.
var ErrBusy = fmt.Errorf("engine: the game loop is not keeping up")

// Do queues a task and returns without waiting for it.
//
// It never blocks. A player typing into a wedged server should be told the
// game is busy, not be left hanging on a channel send — and an unbounded
// queue would turn a stall into an out-of-memory kill.
func (e *Engine) Do(t Task) error {
	select {
	case e.tasks <- t:
		return nil
	default:
		return ErrBusy
	}
}

// DoSync queues a task and waits for it to finish. Used by anything that
// needs an answer from the world, and by tests.
func (e *Engine) DoSync(ctx context.Context, t Task) error {
	done := make(chan struct{})
	wrapped := func(w *game.Live) {
		defer close(done)
		t(w)
	}
	select {
	case e.tasks <- wrapped:
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Pulse returns how many ticks have elapsed.
func (e *Engine) Pulse() uint64 { return e.pulse.Load() }

// Missed returns how many pulses have gone by with the loop too busy to
// take them. Anything above zero means the world has been running behind
// real time; it is a count of pulses, not of occasions.
func (e *Engine) Missed() uint64 { return e.missed.Load() }

// Run drives the loop until ctx is cancelled. It must be called on exactly
// one goroutine; that goroutine is the one that owns the world.
func (e *Engine) Run(ctx context.Context) {
	ticker := time.NewTicker(e.interval)
	defer ticker.Stop()

	e.started = time.Now()
	e.logger.Info("game loop started", "interval", e.interval)

	for {
		select {
		case <-ctx.Done():
			e.drain()
			e.logger.Info("game loop stopped",
				"pulses", e.pulse.Load(), "missed", e.missed.Load())
			return

		case t := <-e.tasks:
			e.runTask(t)

		case <-ticker.C:
			e.tick(time.Now())
		}
	}
}

// tick runs one pulse: everything that was queued, then the periodic work
// that has fallen due since the last one.
//
// now is the moment the tick fired, and the pulse count is derived from it
// rather than incremented. That is deliberate and is #321.
// [time.Ticker]'s channel holds one tick and *drops* the rest, so under a
// ticker an overrunning pulse does not make the next one late — it makes
// the next one not happen. Counting received ticks therefore lost time,
// silently and cumulatively, and the loop had no way to know.
//
// It mattered because the game has two clocks and only one of them was on
// the pulse. Everything periodic is scheduled on the pulse count, but the
// mud calendar is derived from real elapsed time (game.Live.MudTime), and
// point-update reads the hour out of the calendar to decide what to
// announce (server/tick.go's weatherAndTime, whose comment asserts that
// "the pulse is one mud hour long by construction" — true only if no tick
// is ever dropped). Once the two had drifted 75 seconds apart, two mud
// hours passed between consecutive point-updates, and a sunrise is a
// switch on four exact hours: the hour skipped is an announcement nobody
// ever sees, and a mud hour of regeneration, hunger and affect ageing
// nobody ever gets.
//
// Work that fell due during a gap runs **once**, however many times it was
// due. Catching up would be running six mud hours of regeneration in one
// pulse on a server that has just proved it cannot keep up with one, which
// is a way of turning a hiccup into a stall.
func (e *Engine) tick(now time.Time) {
	start := time.Now()

	previous := e.pulse.Load()
	pulse := previous + 1
	if elapsed := uint64(now.Sub(e.started) / e.interval); elapsed > pulse { //nolint:gosec // elapsed is a count of intervals since Run began
		e.missed.Add(elapsed - pulse)
		pulse = elapsed
	}
	e.pulse.Store(pulse)

	// Drain what is waiting before doing periodic work, so a burst of player
	// input is handled in the pulse it arrived in rather than trickling out
	// one command per tick.
	//
	// Bounded by what was queued when the pulse began, rather than draining
	// until the queue is empty. Tasks queue tasks — Server.echoWizVis calls
	// Do from the world goroutine for every wizvis-tagged log line — so an
	// unbounded drain has no ceiling on how long one pulse can take, which
	// is the mechanism that loses the pulses above. Anything queued *during*
	// the pulse is picked up by Run's own select as soon as this returns,
	// which is sooner than the next tick.
	waiting := len(e.tasks)
	for i := 0; i < waiting; i++ {
		select {
		case t := <-e.tasks:
			e.runTask(t)
		default:
			// Somebody else took the last one; nothing to wait for.
			i = waiting
		}
	}

	// Then the periodic work, on the same goroutine and with the same panic
	// containment as anything else.
	//
	// "Due since the last pulse" rather than "due on this pulse": with no
	// gap the two are the same test written differently, and with one this
	// is what stops a skipped pulse skipping the work that was on it.
	for _, p := range e.periodic {
		if p.Every == 0 || pulse/p.Every == previous/p.Every {
			continue
		}
		began := time.Now()
		e.runTask(p.Run)
		if took := time.Since(began); took > e.interval {
			e.logger.Warn("periodic work overran a pulse",
				"name", p.Name, "took", took, "budget", e.interval)
		}
	}

	if e.metrics != nil {
		e.metrics.PulseDuration.Observe(time.Since(start).Seconds())
		if missed := pulse - previous - 1; missed > 0 {
			e.metrics.PulsesMissed.Add(float64(missed))
		}
	}

	if missed := pulse - previous - 1; missed > 0 {
		// Louder than the overrun warning below, because this is the
		// consequence rather than the cause: the world has just skipped
		// that many pulses of real time for everybody at once.
		e.logger.Warn("the game loop fell behind real time",
			"pulses_missed", missed, "pulse", pulse, "budget", e.interval)
	}

	// A pulse that overruns its budget means the world is falling behind
	// real time for everyone at once, which is the single most useful thing
	// a MUD can complain about.
	if took := time.Since(start); took > e.interval {
		e.logger.Warn("pulse overran its budget",
			"took", took, "budget", e.interval, "pulse", pulse)
	}
}

// runTask runs one task, surviving a panic in it.
//
// A command that panics should disconnect the player who typed it, not take
// the world down with everyone else standing in it.
func (e *Engine) runTask(t Task) {
	defer func() {
		if r := recover(); r != nil {
			e.logger.Error("a task panicked and was contained",
				"panic", r, "stack", string(debug.Stack()))
		}
	}()
	t(e.world)
}

// drain runs whatever is already queued, so work accepted before shutdown is
// not silently dropped — a queued save is the case that matters.
func (e *Engine) drain() {
	for {
		select {
		case t := <-e.tasks:
			e.runTask(t)
		default:
			return
		}
	}
}
