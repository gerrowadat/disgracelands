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
// docs/proposals/go-port-plan.md §3.1 for why this and not an
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

	// pulse counts elapsed ticks. Everything periodic is scheduled against
	// it, the way the C server's PULSE_* constants are.
	pulse atomic.Uint64
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
	}
}

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

// Run drives the loop until ctx is cancelled. It must be called on exactly
// one goroutine; that goroutine is the one that owns the world.
func (e *Engine) Run(ctx context.Context) {
	ticker := time.NewTicker(e.interval)
	defer ticker.Stop()

	e.logger.Info("game loop started", "interval", e.interval)

	for {
		select {
		case <-ctx.Done():
			e.drain()
			e.logger.Info("game loop stopped", "pulses", e.pulse.Load())
			return

		case t := <-e.tasks:
			e.runTask(t)

		case <-ticker.C:
			e.tick()
		}
	}
}

// tick runs one pulse: everything queued, then the periodic work.
func (e *Engine) tick() {
	start := time.Now()
	e.pulse.Add(1)

	// Drain what is waiting before doing periodic work, so a burst of player
	// input is handled in the pulse it arrived in rather than trickling out
	// one command per tick.
	for {
		select {
		case t := <-e.tasks:
			e.runTask(t)
			continue
		default:
		}
		break
	}

	if e.metrics != nil {
		e.metrics.PulseDuration.Observe(time.Since(start).Seconds())
	}

	// A pulse that overruns its budget means the world is falling behind
	// real time for everyone at once, which is the single most useful thing
	// a MUD can complain about.
	if took := time.Since(start); took > e.interval {
		e.logger.Warn("pulse overran its budget",
			"took", took, "budget", e.interval, "pulse", e.pulse.Load())
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
