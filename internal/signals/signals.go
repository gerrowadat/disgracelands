// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

// Package signals is this server's whole signal disposition: one channel,
// one goroutine, one handler per signal.
//
// The C sets its up in signal_setup (comm.c:2165) and traps eight. The
// shape here is deliberately the same shape as that one, for a reason that
// has outlived the original: a C handler may not call anything that is not
// async-signal-safe, so the C's handlers set a byte (reread_wizlists,
// comm.c:2087) and the game loop acts on it after the heartbeat
// (comm.c:877). Go has no such restriction, and the discipline still holds
// here because the world is owned by a single goroutine — a handler either
// publishes an atomic value or hands a closure to engine.DoSync, and never
// reaches into the world itself.
//
// docs/proposals/signal-handling.md is the design: what each signal does,
// what a signal may reload and what only an in-game command may, and why
// SIGQUIT is deliberately absent from every table here.
package signals

import (
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

// A Handler is one signal and what receiving it does.
//
// Run is called on the dispatcher's own goroutine, one signal at a time, so
// it must return promptly: a handler that blocks delays every later signal,
// and the one that matters is the second SIGINT — the one an operator sends
// because the first shutdown is wedged. Anything slow belongs in a
// goroutine the handler starts, not in the handler.
type Handler struct {
	// Signal is what to trap.
	Signal os.Signal
	// Does is a short phrase for the log line written when the handler is
	// installed and again each time it fires: "graceful shutdown",
	// "reload the configuration".
	Does string
	// Run is the work. Never nil.
	Run func()
}

// A Set is a running dispatcher. Stop it to restore the default
// dispositions.
type Set struct {
	ch   chan os.Signal
	done chan struct{}
	once sync.Once
}

// Install traps every handler's signal and starts dispatching.
//
// Anything not named here keeps its default disposition, which for SIGQUIT
// is the point rather than an oversight: the Go runtime's own handler dumps
// every goroutine's stack, which is what a wedged server needs and what the
// C's deadlock watchdog (checkpointing, comm.c:2109 — one log line and
// abort()) could not produce.
//
// Two handlers for the same signal is a programming error rather than a
// way to chain them: the later one wins, silently. Two signals sharing one
// Run — which is what SIGINT and SIGTERM do — is the supported shape.
func Install(logger *slog.Logger, handlers ...Handler) *Set {
	byName := make(map[os.Signal]Handler, len(handlers))
	trapped := make([]os.Signal, 0, len(handlers))
	installed := make([]string, 0, len(handlers))
	for _, h := range handlers {
		byName[h.Signal] = h
		trapped = append(trapped, h.Signal)
		installed = append(installed, Name(h.Signal)+": "+h.Does)
	}

	// One slot per trapped signal: signal.Notify never blocks, so a
	// full channel drops the signal, and the dispatcher goroutine can
	// be inside a handler when the next one lands.
	s := &Set{
		ch:   make(chan os.Signal, len(trapped)+1),
		done: make(chan struct{}),
	}
	signal.Notify(s.ch, trapped...)

	logger.Info("signal handling installed", "handlers", installed)

	go func() {
		defer close(s.done)
		for sig := range s.ch {
			h, ok := byName[sig]
			if !ok {
				// Only reachable if something called signal.Notify
				// on this channel from outside, which nothing does.
				logger.Warn("signal received with no handler", "signal", Name(sig))
				continue
			}
			logger.Info("signal received", "signal", Name(sig), "action", h.Does)
			h.Run()
		}
	}()

	return s
}

// Stop restores the default disposition of every trapped signal and waits
// for the dispatcher to finish.
//
// This is what makes a second Ctrl-C kill a shutdown that will not
// complete: once the relaying stops, SIGINT goes back to meaning what it
// means to any other process. Calling it more than once is safe, so a
// deferred Stop and an explicit one on the shutdown path can coexist.
func (s *Set) Stop() {
	s.once.Do(func() {
		// signal.Stop guarantees no further sends on the channel once
		// it returns, which is what makes closing it here safe.
		signal.Stop(s.ch)
		close(s.ch)
		<-s.done
	})
}

// Name is the conventional spelling of a signal, for logs.
//
// os.Signal.String gives the strerror-style wording — "hangup",
// "terminated" — and an operator reading a log is looking for the name they
// typed at kill(1).
func Name(sig os.Signal) string {
	switch sig {
	case syscall.SIGHUP:
		return "SIGHUP"
	case syscall.SIGINT:
		return "SIGINT"
	case syscall.SIGTERM:
		return "SIGTERM"
	case syscall.SIGUSR1:
		return "SIGUSR1"
	case syscall.SIGUSR2:
		return "SIGUSR2"
	case syscall.SIGQUIT:
		return "SIGQUIT"
	}
	return sig.String()
}
