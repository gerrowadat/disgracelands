// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package engine

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// The pulse is what everything in the game is timed off, so what the loop
// does when it cannot keep up is not a detail — it is the difference
// between the world running slow and the world skipping.
//
// These drive tick() directly with the moment it "fired", rather than
// waiting on a real ticker. The thing under test is arithmetic over
// elapsed time, and a test that sleeps to produce that elapsed time is
// both slower and less able to express the case that matters: a gap
// bigger than one interval, which on a healthy machine will not happen
// on demand.

func testEngine(t *testing.T, interval time.Duration, periodic []Periodic) *Engine {
	t.Helper()

	e := New(Options{
		World:    &game.Live{},
		Interval: interval,
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Periodic: periodic,
	})
	e.started = time.Now()
	return e
}

// fireAt runs one tick as though it had happened `after` into the run.
func fireAt(e *Engine, after time.Duration) {
	e.tick(e.started.Add(after))
}

// TestPulseFollowsElapsedTimeNotTickCount is #321: time.Ticker drops the
// ticks a busy receiver is not there for, so counting received ticks lost
// time silently. Twelve intervals elapse and the loop is only there for
// three of them; the pulse count must say twelve.
func TestPulseFollowsElapsedTimeNotTickCount(t *testing.T) {
	const interval = 100 * time.Millisecond
	e := testEngine(t, interval, nil)

	fireAt(e, interval)
	if got := e.Pulse(); got != 1 {
		t.Fatalf("after one interval the pulse is %d, want 1", got)
	}

	// The loop was busy for nine intervals and the ticker dropped them.
	fireAt(e, 10*interval)
	if got := e.Pulse(); got != 10 {
		t.Errorf("after ten intervals the pulse is %d, want 10", got)
	}
	if got := e.Missed(); got != 8 {
		t.Errorf("%d pulses reported missed, want 8", got)
	}

	fireAt(e, 12*interval)
	if got := e.Pulse(); got != 12 {
		t.Errorf("after twelve intervals the pulse is %d, want 12", got)
	}
	if got := e.Missed(); got != 9 {
		t.Errorf("%d pulses reported missed in total, want 9", got)
	}
}

// TestAPulseIsNeverLostToASlowTick is the same claim from the other end:
// however lumpy the ticks are, the pulse count after N intervals is N.
// That is what keeps the pulse-scheduled work in step with
// game.Live.MudTime, which is derived from real elapsed time and cannot
// fall behind.
func TestAPulseIsNeverLostToASlowTick(t *testing.T) {
	const interval = 10 * time.Millisecond
	e := testEngine(t, interval, nil)

	// A deliberately awful pattern: on time, on time, very late, on time.
	for _, at := range []time.Duration{1, 2, 3, 47, 48, 49, 50} {
		fireAt(e, time.Duration(at)*interval)
	}
	if got := e.Pulse(); got != 50 {
		t.Errorf("after 50 intervals the pulse is %d, want 50", got)
	}
}

// TestPeriodicWorkDueDuringAGapRunsExactlyOnce is the half that decides
// what a missed pulse actually costs.
//
// Not zero times: that is the bug — a point-update skipped is a mud hour
// of regeneration, hunger and affect ageing nobody gets, and a sunrise
// (a switch on four exact hours) nobody ever sees.
//
// And not once per pulse it was due: catching up means running six mud
// hours of regeneration in one pulse on a server that has just proved it
// cannot keep up with one.
func TestPeriodicWorkDueDuringAGapRunsExactlyOnce(t *testing.T) {
	const interval = 10 * time.Millisecond

	var everyPulse, everyTen, everySeven int
	e := testEngine(t, interval, []Periodic{
		{Name: "every-pulse", Every: 1, Run: func(*game.Live) { everyPulse++ }},
		{Name: "every-ten", Every: 10, Run: func(*game.Live) { everyTen++ }},
		{Name: "every-seven", Every: 7, Run: func(*game.Live) { everySeven++ }},
	})

	// One ordinary pulse, then a gap that swallows pulses 2 through 25.
	fireAt(e, interval)
	if everyPulse != 1 || everyTen != 0 || everySeven != 0 {
		t.Fatalf("after pulse 1: %d/%d/%d, want 1/0/0", everyPulse, everyTen, everySeven)
	}

	fireAt(e, 25*interval)

	// every-pulse was due 24 more times and runs once; every-ten crossed
	// 10 and 20 and runs once; every-seven crossed 7, 14 and 21 and runs
	// once.
	if everyPulse != 2 {
		t.Errorf("every-pulse ran %d times, want 2 (one per tick, not one per pulse)", everyPulse)
	}
	if everyTen != 1 {
		t.Errorf("every-ten ran %d times, want 1", everyTen)
	}
	if everySeven != 1 {
		t.Errorf("every-seven ran %d times, want 1", everySeven)
	}

	// And the schedule is not left phase-shifted by the gap: the next
	// crossing of a multiple of ten is pulse 30.
	fireAt(e, 29*interval)
	if everyTen != 1 {
		t.Errorf("every-ten ran at pulse 29 (%d runs); the gap moved its schedule", everyTen)
	}
	fireAt(e, 30*interval)
	if everyTen != 2 {
		t.Errorf("every-ten did not run at pulse 30 (%d runs)", everyTen)
	}
}

// TestPeriodicWorkRunsOnScheduleWithNoGap checks that the "due since the
// last pulse" test is the same test as the old `pulse % Every == 0` when
// nothing is missed — which is the case that has to keep working exactly.
func TestPeriodicWorkRunsOnScheduleWithNoGap(t *testing.T) {
	const interval = time.Millisecond

	var ran []uint64
	e := testEngine(t, interval, []Periodic{
		{Name: "every-three", Every: 3, Run: func(*game.Live) { ran = append(ran, 0) }},
	})

	for i := 1; i <= 12; i++ {
		fireAt(e, time.Duration(i)*interval)
	}
	if len(ran) != 4 {
		t.Errorf("every-three ran %d times over 12 pulses, want 4", len(ran))
	}
}

// TestTheDrainIsBoundedByWhatWasAlreadyQueued: a task that queues a task
// must not be able to extend the pulse it is running in. echoWizVis does
// exactly that, from the world goroutine, for every wizvis-tagged log
// line; with an unbounded drain there is no ceiling on how long one pulse
// can take, which is the mechanism that loses pulses in the first place.
func TestTheDrainIsBoundedByWhatWasAlreadyQueued(t *testing.T) {
	e := testEngine(t, 10*time.Millisecond, nil)

	var ran int
	var queue func()
	queue = func() {
		_ = e.Do(func(*game.Live) {
			ran++
			queue() // and so on, for ever, if the drain lets it
		})
	}
	queue()

	done := make(chan struct{})
	go func() {
		defer close(done)
		fireAt(e, 10*time.Millisecond)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the pulse never ended: a self-feeding task extended it without limit")
	}
	if ran != 1 {
		t.Errorf("the drain ran %d tasks; only the one already queued should have run", ran)
	}
}

// TestRunStopsAndDrains covers the shutdown path: work accepted before
// cancellation is not silently dropped, which is what stops a queued save
// being lost.
func TestRunStopsAndDrains(t *testing.T) {
	e := testEngine(t, time.Hour, nil) // long enough that no tick fires

	ctx, cancel := context.WithCancel(context.Background())
	ran := make(chan struct{}, 1)
	if err := e.Do(func(*game.Live) { ran <- struct{}{} }); err != nil {
		t.Fatal(err)
	}
	cancel()
	e.Run(ctx)

	select {
	case <-ran:
	default:
		t.Error("a task queued before shutdown was dropped")
	}
}

// TestDoRefusesRatherThanBlocking: a player typing into a wedged server is
// told the game is busy rather than left hanging, and an unbounded queue
// would turn a stall into an out-of-memory kill.
func TestDoRefusesRatherThanBlocking(t *testing.T) {
	e := New(Options{
		World:      &game.Live{},
		Interval:   time.Hour,
		QueueDepth: 2,
		Logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	for i := 0; i < 2; i++ {
		if err := e.Do(func(*game.Live) {}); err != nil {
			t.Fatalf("queueing task %d: %v", i, err)
		}
	}
	if err := e.Do(func(*game.Live) {}); err != ErrBusy {
		t.Errorf("a full queue returned %v, want ErrBusy", err)
	}
}
