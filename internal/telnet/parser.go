// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package telnet

// EventKind distinguishes the two things a client can send that are not data.
type EventKind int

const (
	// EventNegotiation is a WILL, WONT, DO or DONT.
	EventNegotiation EventKind = iota
	// EventSubnegotiation is a completed IAC SB ... IAC SE.
	EventSubnegotiation
)

// Event is one telnet command taken out of the input stream.
type Event struct {
	Kind    EventKind
	Command byte
	Option  byte
	// Payload is the unescaped subnegotiation body, without the option byte.
	Payload []byte
}

// maxSubnegotiation bounds a subnegotiation body.
//
// A client that sends IAC SB and then never sends IAC SE would otherwise grow
// this buffer until the server runs out of memory — the same unbounded-queue
// problem the session's output side has, from the other direction. GMCP
// packages are a few hundred bytes; sixteen kilobytes is generous.
const maxSubnegotiation = 16 * 1024

type parserState int

const (
	stateData parserState = iota
	stateIAC
	stateNegotiating
	stateSubOption
	stateSubData
	stateSubIAC
)

// Parser strips telnet commands out of a byte stream.
//
// It is a state machine rather than a scan-and-splice because commands may be
// split across reads at any byte — an IAC at the end of one packet and its
// option at the start of the next is normal, not pathological.
type Parser struct {
	state  parserState
	cmd    byte
	opt    byte
	sub    []byte
	events []Event

	// overlong records that a subnegotiation was dropped for length, so it is
	// not silently mistaken for one that never arrived.
	overlong bool
}

// Feed appends the data bytes of in to dst and returns it, absorbing any
// telnet commands. Completed commands are collected for Events.
func (p *Parser) Feed(dst, in []byte) []byte {
	for _, b := range in {
		switch p.state {
		case stateData:
			if b == IAC {
				p.state = stateIAC
				continue
			}
			dst = append(dst, b)

		case stateIAC:
			switch b {
			case IAC:
				// An escaped 255: one literal byte of data.
				dst = append(dst, IAC)
				p.state = stateData
			case WILL, WONT, DO, DONT:
				p.cmd = b
				p.state = stateNegotiating
			case SB:
				p.state = stateSubOption
			default:
				// NOP, GA, and the rest of RFC 854. Nothing a MUD needs to
				// act on, and dropping them is the point of parsing at all.
				p.state = stateData
			}

		case stateNegotiating:
			p.events = append(p.events, Event{
				Kind: EventNegotiation, Command: p.cmd, Option: b,
			})
			p.state = stateData

		case stateSubOption:
			p.opt = b
			p.sub = p.sub[:0]
			p.overlong = false
			p.state = stateSubData

		case stateSubData:
			if b == IAC {
				p.state = stateSubIAC
				continue
			}
			p.appendSub(b)

		case stateSubIAC:
			switch b {
			case IAC:
				// Escaped 255 inside the body.
				p.appendSub(IAC)
				p.state = stateSubData
			case SE:
				if !p.overlong {
					payload := make([]byte, len(p.sub))
					copy(payload, p.sub)
					p.events = append(p.events, Event{
						Kind: EventSubnegotiation, Option: p.opt, Payload: payload,
					})
				}
				p.sub = p.sub[:0]
				p.state = stateData
			default:
				// An IAC inside a subnegotiation that is neither escaped nor
				// SE is malformed. RFC 855 says nothing useful about it; the
				// safe reading is that the subnegotiation has been abandoned,
				// so drop it and go back to reading data.
				p.sub = p.sub[:0]
				p.state = stateData
			}
		}
	}
	return dst
}

func (p *Parser) appendSub(b byte) {
	if len(p.sub) >= maxSubnegotiation {
		p.overlong = true
		return
	}
	p.sub = append(p.sub, b)
}

// Events returns the commands seen since the last call and clears them.
func (p *Parser) Events() []Event {
	if len(p.events) == 0 {
		return nil
	}
	out := p.events
	p.events = nil
	return out
}

// InSubnegotiation reports whether the parser is midway through an IAC SB,
// which is only useful for tests and logs.
func (p *Parser) InSubnegotiation() bool {
	return p.state == stateSubOption || p.state == stateSubData || p.state == stateSubIAC
}
