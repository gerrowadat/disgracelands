// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

// The dispatcher is exercised by actually sending signals, and both
// syscall.Kill and SIGWINCH are Unix-only -- so this file is too. Nothing
// is lost: the tests run on Linux, and what Windows needs from this
// package is that it compiles, which scripts/build-dist.sh checks at
// release time.
//go:build unix

package signals

import (
	"io"
	"log/slog"
	"os"
	"syscall"
	"testing"
	"time"
)

// Every test here signals the test binary itself, so the signal it uses has
// to be one whose *default* disposition is to be ignored — the whole point
// of Stop is to put that default back, and a test that then sent SIGHUP
// would kill the test binary rather than fail. SIGWINCH is ignored by
// default and the Go runtime has no use for it. (SIGURG looks equally safe
// and is not: the runtime uses it for goroutine preemption.)
const testSignal = syscall.SIGWINCH

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestInstallRunsTheHandler(t *testing.T) {
	fired := make(chan struct{}, 1)
	set := Install(quietLogger(), Handler{
		Signal: testSignal,
		Does:   "test",
		Run:    func() { fired <- struct{}{} },
	})
	defer set.Stop()

	if err := syscall.Kill(syscall.Getpid(), testSignal); err != nil {
		t.Fatalf("signalling this process: %v", err)
	}

	select {
	case <-fired:
	case <-time.After(5 * time.Second):
		t.Fatal("the handler did not run within 5s")
	}
}

// TestStopRestoresTheDefault is the second-Ctrl-C contract: once Stop has
// returned, the process is no longer trapping the signal at all, so a
// shutdown that will not finish can still be interrupted.
func TestStopRestoresTheDefault(t *testing.T) {
	fired := make(chan struct{}, 1)
	set := Install(quietLogger(), Handler{
		Signal: testSignal,
		Does:   "test",
		Run:    func() { fired <- struct{}{} },
	})
	set.Stop()

	if err := syscall.Kill(syscall.Getpid(), testSignal); err != nil {
		t.Fatalf("signalling this process: %v", err)
	}

	select {
	case <-fired:
		t.Fatal("the handler ran after Stop")
	case <-time.After(100 * time.Millisecond):
	}
}

// TestStopIsIdempotent covers the way cmd/dlmud uses it: a deferred Stop for
// the error paths, and an explicit one on the shutdown path, both of which
// run on an ordinary shutdown.
func TestStopIsIdempotent(t *testing.T) {
	set := Install(quietLogger(), Handler{Signal: testSignal, Does: "test", Run: func() {}})
	set.Stop()
	set.Stop()
}

// TestHandlersRunOneAtATime is what the Handler doc comment promises, and
// the reason it asks for handlers that return promptly: they share one
// goroutine, so a slow one delays the next signal rather than running
// beside it.
func TestHandlersRunOneAtATime(t *testing.T) {
	var (
		running = make(chan struct{})
		release = make(chan struct{})
		second  = make(chan struct{}, 1)
		first   = true
	)
	set := Install(quietLogger(), Handler{
		Signal: testSignal,
		Does:   "test",
		Run: func() {
			if first {
				first = false
				close(running)
				<-release
				return
			}
			second <- struct{}{}
		},
	})
	defer set.Stop()

	pid := syscall.Getpid()
	if err := syscall.Kill(pid, testSignal); err != nil {
		t.Fatalf("signalling this process: %v", err)
	}
	<-running

	if err := syscall.Kill(pid, testSignal); err != nil {
		t.Fatalf("signalling this process: %v", err)
	}
	select {
	case <-second:
		t.Fatal("a second handler ran while the first was still going")
	case <-time.After(100 * time.Millisecond):
	}

	close(release)
	select {
	case <-second:
	case <-time.After(5 * time.Second):
		t.Fatal("the queued signal was never handled")
	}
}

func TestNameUsesTheConventionalSpelling(t *testing.T) {
	for sig, want := range map[os.Signal]string{
		syscall.SIGHUP:  "SIGHUP",
		syscall.SIGINT:  "SIGINT",
		syscall.SIGTERM: "SIGTERM",
		syscall.SIGUSR1: "SIGUSR1",
		syscall.SIGUSR2: "SIGUSR2",
		syscall.SIGQUIT: "SIGQUIT",
	} {
		if got := Name(sig); got != want {
			t.Errorf("Name(%v) = %q, want %q", sig, got, want)
		}
	}
	// Anything else falls back to the strerror wording rather than
	// inventing one.
	if got := Name(testSignal); got != testSignal.String() {
		t.Errorf("Name(SIGWINCH) = %q, want %q", got, testSignal.String())
	}
}
