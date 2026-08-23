// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package session

import (
	"testing"
	"time"
)

// TestDefaultRoundLengthIsPulseViolence.
//
// A combat round is PULSE_VIOLENCE, which structs.h:512 defines as
// `(2 RL_SEC)`. This is asserted here rather than in internal/server because
// the server's tests deliberately run on a much shorter round — see
// testRoundLength there — so this package is the one that still sees the real
// number. Without it, shortening a round for the tests could shorten it for
// players and nothing would notice.
func TestDefaultRoundLengthIsPulseViolence(t *testing.T) {
	if DefaultRoundLength != 2*time.Second {
		t.Errorf("a combat round is %s, want the C's PULSE_VIOLENCE of two seconds", DefaultRoundLength)
	}
}

// TestAContextWithNoRoundLengthUsesTheRealOne, which is what makes the knob
// safe: every Context built without a thought for it — the ones the special
// procedures build for a mobile acting as a player, chiefly — lags the way
// the C does.
func TestAContextWithNoRoundLengthUsesTheRealOne(t *testing.T) {
	var c Context
	if got := c.roundLength(); got != DefaultRoundLength {
		t.Errorf("a zero Context's round is %s, want %s", got, DefaultRoundLength)
	}
}

// TestARoundLengthOverrideIsUsed.
func TestARoundLengthOverrideIsUsed(t *testing.T) {
	c := Context{RoundLength: 50 * time.Millisecond}
	if got := c.roundLength(); got != 50*time.Millisecond {
		t.Errorf("the round is %s, want the 50ms it was set to", got)
	}
}
