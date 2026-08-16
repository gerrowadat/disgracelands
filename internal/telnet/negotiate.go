// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package telnet

import "sync"

// Option negotiation, by RFC 1143's "Q Method".
//
// The naive way to answer negotiation — reply to every WILL with a DO, to
// every DO with a WILL — loops. Both ends see a request, both answer, both
// read the answer as a fresh request, and two clients doing this to each
// other will keep it up until one of them gives out. RFC 1143 exists because
// everybody wrote that bug: it defines six states per option per side, so
// that a request already in flight is not re-sent, an answer is told apart
// from a request, and a queued change of mind is applied once the outstanding
// negotiation completes.
//
// It is implemented here rather than taken from a library because no
// maintained Go telnet package speaks the options a MUD needs — CHARSET and
// GMCP are ours either way (see charset.go, gmcp.go) — and the one package
// that does implement this algorithm has no upstream repository and no tests.
// The algorithm is a published specification; this is a port of it, with the
// state table in the tests.

// qstate is one side's state for one option, per RFC 1143.
type qstate byte

const (
	// qNo: the option is off, and nothing is in flight.
	qNo qstate = iota
	// qYes: the option is on.
	qYes
	// qWantNo: we have sent a request to turn it off and are waiting.
	qWantNo
	// qWantNoOpposite: as qWantNo, but we have since changed our mind and
	// want it on once the outstanding negotiation finishes.
	qWantNoOpposite
	// qWantYes: we have sent a request to turn it on and are waiting.
	qWantYes
	// qWantYesOpposite: as qWantYes, with a queued change of mind.
	qWantYesOpposite
)

// Side identifies which end of an option a change refers to.
type Side byte

const (
	// SideUs is the server's own side: WILL/WONT sent, DO/DONT received.
	SideUs Side = iota
	// SideHim is the client's side: DO/DONT sent, WILL/WONT received.
	SideHim
)

// Change reports an option that finished negotiating.
type Change struct {
	Side    Side
	Option  byte
	Enabled bool
	// Changed is false when the exchange left the state where it was, which
	// is the common case for a duplicate or an unsolicited refusal.
	Changed bool
}

// Policy says what the server is willing to agree to. Anything not listed is
// refused — an option accepted and then ignored is worse than one refused,
// because the client will start sending for it.
type Policy struct {
	// Us are the options the server may enable on its own side.
	Us []byte
	// Him are the options the server will let the client enable.
	Him []byte
}

func (p Policy) allow(side Side, opt byte) bool {
	list := p.Us
	if side == SideHim {
		list = p.Him
	}
	for _, o := range list {
		if o == opt {
			return true
		}
	}
	return false
}

// Negotiator tracks option state for one connection.
//
// It is safe for concurrent use: the read loop feeds it received commands and
// the game goroutine asks it to enable options, and those are different
// goroutines.
type Negotiator struct {
	mu     sync.Mutex
	policy Policy
	us     map[byte]qstate
	him    map[byte]qstate
}

// NewNegotiator returns a Negotiator that agrees to what the policy allows.
func NewNegotiator(p Policy) *Negotiator {
	return &Negotiator{
		policy: p,
		us:     map[byte]qstate{},
		him:    map[byte]qstate{},
	}
}

// state returns the map for a side. The caller holds the lock.
func (n *Negotiator) states(side Side) map[byte]qstate {
	if side == SideHim {
		return n.him
	}
	return n.us
}

// commands returns the request/refusal pair a side sends: WILL/WONT for our
// own options, DO/DONT for the client's.
func requestCommands(side Side) (enable, disable byte) {
	if side == SideHim {
		return DO, DONT
	}
	return WILL, WONT
}

// Enable asks for an option to be turned on, returning the bytes to send,
// which are nil when the request is already in flight or already agreed.
func (n *Negotiator) Enable(side Side, opt byte) []byte {
	n.mu.Lock()
	defer n.mu.Unlock()

	enable, _ := requestCommands(side)
	states := n.states(side)

	switch states[opt] {
	case qNo:
		states[opt] = qWantYes
		return Negotiate(enable, opt)
	case qWantNo:
		// A disable is in flight; queue the change of mind rather than
		// sending a second request into it.
		states[opt] = qWantNoOpposite
	case qWantYesOpposite:
		// We had queued a disable; cancel that instead of asking again.
		states[opt] = qWantYes
	}
	return nil
}

// Disable asks for an option to be turned off, returning the bytes to send.
func (n *Negotiator) Disable(side Side, opt byte) []byte {
	n.mu.Lock()
	defer n.mu.Unlock()

	_, disable := requestCommands(side)
	states := n.states(side)

	switch states[opt] {
	case qYes:
		states[opt] = qWantNo
		return Negotiate(disable, opt)
	case qWantNoOpposite:
		states[opt] = qWantNo
	case qWantYes:
		states[opt] = qWantYesOpposite
	}
	return nil
}

// Announce forces an option on or off without waiting for agreement,
// returning the bytes to send.
//
// This is a deliberate departure from the algorithm above, for ECHO and
// nothing else. RFC 1143's queueing is right for options that change how
// bytes are interpreted, where getting ahead of the client corrupts the
// stream. ECHO around a password prompt is not one of those: it is a display
// toggle, and its two failure modes are not symmetric. An extra WONT ECHO is
// ignored by every client; a WONT ECHO that is never sent — because the
// client obeyed the WILL without answering it, leaving the state waiting —
// leaves a player typing blind for the rest of the session. The C server
// sends both unconditionally (comm.c's echo_off_str) and no client has ever
// minded.
func (n *Negotiator) Announce(side Side, opt byte, on bool) []byte {
	n.mu.Lock()
	defer n.mu.Unlock()

	enable, disable := requestCommands(side)
	states := n.states(side)

	if on {
		states[opt] = qYes
		return Negotiate(enable, opt)
	}
	states[opt] = qNo
	return Negotiate(disable, opt)
}

// Receive processes one negotiation command from the client. It returns the
// reply to send, which is nil when none is owed, and what the exchange
// settled.
func (n *Negotiator) Receive(cmd, opt byte) ([]byte, Change) {
	// WILL and WONT are about the client's side; DO and DONT about ours.
	side := SideHim
	positive := cmd == WILL
	if cmd == DO || cmd == DONT {
		side = SideUs
		positive = cmd == DO
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	enable, disable := requestCommands(side)
	states := n.states(side)
	before := states[opt]
	var reply []byte

	if positive {
		// WILL (or DO): the peer is offering, or agreeing to, an option.
		switch before {
		case qNo:
			if n.policy.allow(side, opt) {
				states[opt] = qYes
				reply = Negotiate(enable, opt)
			} else {
				reply = Negotiate(disable, opt)
			}
		case qYes:
			// Already on; a repeat needs no answer, and answering is how the
			// loop this algorithm exists to prevent gets started.
		case qWantNo:
			// The peer answered our request to turn it off by turning it on.
			// RFC 1143 calls this an error on their part and settles on off.
			states[opt] = qNo
		case qWantNoOpposite:
			states[opt] = qYes
		case qWantYes:
			states[opt] = qYes
		case qWantYesOpposite:
			states[opt] = qWantNo
			reply = Negotiate(disable, opt)
		}
	} else {
		// WONT (or DONT): the peer is refusing, or turning off, an option.
		switch before {
		case qNo:
			// Already off; nothing owed. Answering here loops.
		case qYes:
			states[opt] = qNo
			reply = Negotiate(disable, opt)
		case qWantNo:
			states[opt] = qNo
		case qWantNoOpposite:
			states[opt] = qWantYes
			reply = Negotiate(enable, opt)
		case qWantYes:
			states[opt] = qNo
		case qWantYesOpposite:
			states[opt] = qNo
		}
	}

	after := states[opt]
	return reply, Change{
		Side:    side,
		Option:  opt,
		Enabled: after == qYes,
		Changed: (before == qYes) != (after == qYes),
	}
}

// EnabledUs reports whether an option is on for the server's side.
func (n *Negotiator) EnabledUs(opt byte) bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.us[opt] == qYes
}

// EnabledHim reports whether an option is on for the client's side.
func (n *Negotiator) EnabledHim(opt byte) bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.him[opt] == qYes
}
