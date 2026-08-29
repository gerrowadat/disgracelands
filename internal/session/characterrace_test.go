// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package session

import (
	"sync"
	"testing"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// Issue #251: Session.character was a plain field written on the session's
// own goroutine at the end of login and character creation, and read on the
// *world* goroutine by any command that walks the descriptor list — `users`
// (do_users' own loop, users.go's `who = s.Character()`), `show snoop`, and
// perform_dupe_check on behalf of a different connection entirely.
//
// The third field of this shape to need an atomic. Session.state was a plain
// int with the same readers (#134) and Dispatcher.Do read Record.Level off
// the world goroutine (#210). This one could not be fixed the way #134's
// echoWizVis was — by reading the world instead of the session list —
// because `switch` and `return` are commands, so SwitchInto and SwitchBack
// write the field *from* the world goroutine.
//
// This test asserts the property directly rather than reproducing the
// scenario, and that is deliberate. The real window is narrow: the write at
// login.go's `s.SetCharacter(character)` lands just after the DoSync that
// made the character, and the wizlog a few lines later queues a task to the
// world goroutine, which is a channel send and so an ordering edge that
// closes the window again within microseconds. An end-to-end reproduction
// therefore catches it some runs and not others — which is exactly how it
// was reported, and no way to write a regression test. Two goroutines
// calling the accessors is the thing that has to be safe, so that is what
// is checked, and `-race` is the whole assertion.
func TestTheCharacterAccessorsAreSafeFromTwoGoroutines(t *testing.T) {
	const rounds = 2000

	s := &Session{}
	first := &game.Character{Name: "Zod"}
	second := &game.Character{Name: "Bystander"}

	var wg sync.WaitGroup
	wg.Add(2)

	// The session's own goroutine, finishing a login.
	go func() {
		defer wg.Done()
		for i := range rounds {
			if i%2 == 0 {
				s.SetCharacter(first)
			} else {
				s.SetCharacter(second)
			}
		}
	}()

	// The world goroutine, running `users` for somebody else.
	go func() {
		defer wg.Done()
		for range rounds {
			if c := s.Character(); c != nil {
				_ = c.Name
			}
		}
	}()

	wg.Wait()
}

// TestSwitchingIsSafeAgainstAReadFromAnotherGoroutine is the other half of
// #251, and the reason the field could not simply be read from the world
// instead.
//
// `switch` and `return` are commands, so SwitchInto and SwitchBack run on
// the world goroutine and write both character and original there — while
// the connection's own goroutine reads the character on every line it
// echoes (colourLevel, in Send) and again in its teardown.
func TestSwitchingIsSafeAgainstAReadFromAnotherGoroutine(t *testing.T) {
	const rounds = 2000

	s := &Session{}
	god := &game.Character{Name: "Zod"}
	rat := &game.Character{Name: "rat"}
	s.SetCharacter(god)

	var wg sync.WaitGroup
	wg.Add(2)

	// The world goroutine: a god switching into a rat and back.
	go func() {
		defer wg.Done()
		for range rounds {
			s.SwitchInto(rat)
			s.SwitchBack()
		}
	}()

	// The connection's goroutine, reading what it is currently driving.
	go func() {
		defer wg.Done()
		for range rounds {
			if c := s.Character(); c != nil {
				_ = c.Name
			}
			_, _ = s.SwitchedFromLevel()
			_ = s.Original()
		}
	}()

	wg.Wait()

	// And it ends where it started: a switch and a return in a row leave
	// the god driving their own body, whatever the reader was doing.
	if got := s.Character(); got != god {
		t.Errorf("after %d switch/return pairs the session is driving %v, want the god", rounds, got)
	}
	if got := s.Original(); got != nil {
		t.Errorf("original is %v after returning, want nil", got)
	}
}
