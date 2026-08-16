// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package telnet

import (
	"bytes"
	"testing"
)

// testPolicy is the server's: it may enable these for itself, and agrees to
// nothing on the client's side.
func testPolicy() Policy {
	return Policy{Us: []byte{OptEcho, OptSuppressGoAhead, OptCharset, OptGMCP}}
}

func TestAnOfferIsAcceptedOnce(t *testing.T) {
	n := NewNegotiator(testPolicy())

	reply, change := n.Receive(DO, OptSuppressGoAhead)
	if !bytes.Equal(reply, Negotiate(WILL, OptSuppressGoAhead)) {
		t.Fatalf("first DO was answered % x, want IAC WILL SGA", reply)
	}
	if !change.Enabled || !change.Changed || change.Side != SideUs {
		t.Fatalf("first DO settled as %+v, want enabled on our side", change)
	}

	// The loop RFC 1143 exists to prevent: answering a repeat restarts the
	// exchange, and two ends doing that never stop.
	reply, change = n.Receive(DO, OptSuppressGoAhead)
	if reply != nil {
		t.Errorf("a repeated DO was answered with % x, want nothing", reply)
	}
	if change.Changed {
		t.Errorf("a repeated DO reported a change: %+v", change)
	}
	if !n.EnabledUs(OptSuppressGoAhead) {
		t.Error("the option went off after a repeated DO")
	}
}

func TestAnOptionOutsideThePolicyIsRefused(t *testing.T) {
	n := NewNegotiator(testPolicy())

	// The server agrees to nothing on the client's side.
	reply, change := n.Receive(WILL, OptWindowSize)
	if !bytes.Equal(reply, Negotiate(DONT, OptWindowSize)) {
		t.Errorf("an unwanted WILL was answered % x, want IAC DONT NAWS", reply)
	}
	if change.Enabled {
		t.Error("an option outside the policy was enabled")
	}

	// And a refusal of something never on is not answered at all, which is
	// the other half of the same loop.
	if reply, _ := n.Receive(WONT, OptWindowSize); reply != nil {
		t.Errorf("a WONT for an option that was off was answered % x", reply)
	}
}

func TestARequestInFlightIsNotSentTwice(t *testing.T) {
	n := NewNegotiator(testPolicy())

	first := n.Enable(SideUs, OptGMCP)
	if !bytes.Equal(first, Negotiate(WILL, OptGMCP)) {
		t.Fatalf("Enable sent % x, want IAC WILL GMCP", first)
	}
	if again := n.Enable(SideUs, OptGMCP); again != nil {
		t.Errorf("Enable sent % x while the first request was unanswered", again)
	}

	if _, change := n.Receive(DO, OptGMCP); !change.Enabled || !change.Changed {
		t.Errorf("the answer to our request settled as %+v, want enabled", change)
	}
}

func TestARefusedRequestLeavesTheOptionOff(t *testing.T) {
	n := NewNegotiator(testPolicy())
	n.Enable(SideUs, OptCharset)

	reply, change := n.Receive(DONT, OptCharset)
	if reply != nil {
		t.Errorf("a refusal of our request was answered % x, want nothing", reply)
	}
	if change.Enabled || n.EnabledUs(OptCharset) {
		t.Error("a refused option was left on")
	}
}

func TestTurningAnAgreedOptionOff(t *testing.T) {
	n := NewNegotiator(testPolicy())
	n.Enable(SideUs, OptGMCP)
	n.Receive(DO, OptGMCP)

	if reply := n.Disable(SideUs, OptGMCP); !bytes.Equal(reply, Negotiate(WONT, OptGMCP)) {
		t.Fatalf("Disable sent % x, want IAC WONT GMCP", reply)
	}
	if _, change := n.Receive(DONT, OptGMCP); change.Enabled {
		t.Error("the option stayed on after both ends turned it off")
	}

	// A client turning off an option the server had on gets an answer, since
	// that half of the exchange is owed.
	n.Enable(SideUs, OptGMCP)
	n.Receive(DO, OptGMCP)
	reply, change := n.Receive(DONT, OptGMCP)
	if !bytes.Equal(reply, Negotiate(WONT, OptGMCP)) {
		t.Errorf("a client turning the option off was answered % x, want IAC WONT GMCP", reply)
	}
	if change.Enabled || !change.Changed {
		t.Errorf("settled as %+v, want a change to disabled", change)
	}
}

// TestAChangeOfMindIsQueued covers the states the naive implementation does
// not have: a request is outstanding and the answer to it is no longer the
// answer wanted.
func TestAChangeOfMindIsQueued(t *testing.T) {
	n := NewNegotiator(testPolicy())

	// Ask to turn it on, change our mind before the answer arrives.
	n.Enable(SideUs, OptGMCP)
	if reply := n.Disable(SideUs, OptGMCP); reply != nil {
		t.Errorf("the change of mind was sent immediately as % x; it has to wait", reply)
	}

	// The answer to the original request arrives, and the queued disable goes
	// out on the back of it.
	reply, change := n.Receive(DO, OptGMCP)
	if !bytes.Equal(reply, Negotiate(WONT, OptGMCP)) {
		t.Errorf("the queued disable was not sent: % x", reply)
	}
	if change.Enabled {
		t.Error("the option was left on despite the queued disable")
	}
}

// TestAnnounceDoesNotWait documents the one option that skips the algorithm.
func TestAnnounceDoesNotWait(t *testing.T) {
	n := NewNegotiator(testPolicy())

	if got := n.Announce(SideUs, OptEcho, true); !bytes.Equal(got, Negotiate(WILL, OptEcho)) {
		t.Fatalf("Announce sent % x, want IAC WILL ECHO", got)
	}
	if !n.EnabledUs(OptEcho) {
		t.Error("Announce did not take effect immediately")
	}

	// The point of it: a client that obeyed the WILL without answering it
	// must still get the WONT, or it is left with echo off forever.
	if got := n.Announce(SideUs, OptEcho, false); !bytes.Equal(got, Negotiate(WONT, OptEcho)) {
		t.Errorf("Announce sent % x, want IAC WONT ECHO", got)
	}
	if n.EnabledUs(OptEcho) {
		t.Error("echo stayed on after being announced off")
	}
}
