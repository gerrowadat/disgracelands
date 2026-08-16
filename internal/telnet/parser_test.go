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

func feed(p *Parser, in []byte) []byte { return p.Feed(nil, in) }

// TestNegotiationIsRemovedFromTheStream is the whole reason this package
// exists: the C server leaves these bytes in the input, so a client that
// offers window size has its NAWS bytes interpreted as a command.
func TestNegotiationIsRemovedFromTheStream(t *testing.T) {
	var p Parser
	in := []byte("look")
	in = append(in, IAC, DO, OptSuppressGoAhead)
	in = append(in, " north\r\n"...)

	if got := string(feed(&p, in)); got != "look north\r\n" {
		t.Errorf("data is %q, want %q", got, "look north\r\n")
	}

	events := p.Events()
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	if events[0].Command != DO || events[0].Option != OptSuppressGoAhead {
		t.Errorf("event is %s %s, want DO suppress-go-ahead",
			CommandName(events[0].Command), OptionName(events[0].Option))
	}
	if p.Events() != nil {
		t.Error("events were returned twice")
	}
}

// TestCommandsSplitAcrossReads is the case a scan-and-splice implementation
// gets wrong. An IAC at the end of one packet and its option at the start of
// the next is ordinary TCP, not a malformed client.
func TestCommandsSplitAcrossReads(t *testing.T) {
	var p Parser
	var data []byte

	for _, chunk := range [][]byte{
		[]byte("hel"),
		{IAC},
		{WILL},
		{OptGMCP},
		[]byte("lo\r\n"),
	} {
		data = p.Feed(data, chunk)
	}

	if string(data) != "hello\r\n" {
		t.Errorf("data is %q, want %q", data, "hello\r\n")
	}
	events := p.Events()
	if len(events) != 1 || events[0].Command != WILL || events[0].Option != OptGMCP {
		t.Fatalf("got %+v, want one WILL GMCP", events)
	}
}

// TestEscapedIACIsData: a literal 255 byte is sent twice and must arrive once.
func TestEscapedIACIsData(t *testing.T) {
	var p Parser
	got := feed(&p, []byte{'a', IAC, IAC, 'b'})
	if !bytes.Equal(got, []byte{'a', IAC, 'b'}) {
		t.Errorf("got % x, want % x", got, []byte{'a', IAC, 'b'})
	}
	if evs := p.Events(); evs != nil {
		t.Errorf("an escaped IAC produced events: %+v", evs)
	}
}

func TestSubnegotiation(t *testing.T) {
	var p Parser
	in := []byte{IAC, SB, OptGMCP}
	in = append(in, `Core.Hello {"client":"tf"}`...)
	in = append(in, IAC, SE)
	in = append(in, "look\r\n"...)

	if got := string(feed(&p, in)); got != "look\r\n" {
		t.Errorf("data is %q, want %q", got, "look\r\n")
	}
	events := p.Events()
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	if events[0].Kind != EventSubnegotiation || events[0].Option != OptGMCP {
		t.Fatalf("got %+v, want a GMCP subnegotiation", events[0])
	}
	if string(events[0].Payload) != `Core.Hello {"client":"tf"}` {
		t.Errorf("payload is %q", events[0].Payload)
	}
}

func TestEscapedIACInsideSubnegotiation(t *testing.T) {
	var p Parser
	in := []byte{IAC, SB, OptGMCP, 'x', IAC, IAC, 'y', IAC, SE}
	feed(&p, in)

	events := p.Events()
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	if !bytes.Equal(events[0].Payload, []byte{'x', IAC, 'y'}) {
		t.Errorf("payload is % x, want % x", events[0].Payload, []byte{'x', IAC, 'y'})
	}
}

// TestAnUnterminatedSubnegotiationIsBounded: a client that opens IAC SB and
// never closes it must not be able to grow the server's memory without limit.
func TestAnUnterminatedSubnegotiationIsBounded(t *testing.T) {
	var p Parser
	p.Feed(nil, []byte{IAC, SB, OptGMCP})
	for i := 0; i < 64; i++ {
		p.Feed(nil, bytes.Repeat([]byte{'x'}, 4096))
	}
	if len(p.sub) > maxSubnegotiation {
		t.Errorf("buffered %d bytes, want at most %d", len(p.sub), maxSubnegotiation)
	}

	// And the oversized one is dropped rather than delivered truncated, which
	// would be a message the client did not send.
	p.Feed(nil, []byte{IAC, SE})
	if evs := p.Events(); len(evs) != 0 {
		t.Errorf("an over-long subnegotiation was delivered: %+v", evs)
	}

	// The parser must still work afterwards.
	if got := string(feed(&p, []byte("look\r\n"))); got != "look\r\n" {
		t.Errorf("after recovery, data is %q", got)
	}
}

// TestBareCommandsAreDropped: NOP, GA and friends carry nothing a MUD needs,
// but they must not reach the interpreter as text.
func TestBareCommandsAreDropped(t *testing.T) {
	var p Parser
	got := feed(&p, []byte{'a', IAC, NOP, 'b', IAC, GA, 'c', IAC, AYT, 'd'})
	if string(got) != "abcd" {
		t.Errorf("data is %q, want %q", got, "abcd")
	}
	if evs := p.Events(); evs != nil {
		t.Errorf("bare commands produced events: %+v", evs)
	}
}

func TestEscapeAndSubnegotiateRoundTrip(t *testing.T) {
	payload := []byte{'a', IAC, 'b', SE, 'c'}
	wire := Subnegotiate(OptGMCP, payload)

	var p Parser
	if data := feed(&p, wire); len(data) != 0 {
		t.Errorf("a subnegotiation produced %q of data", data)
	}
	events := p.Events()
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	if !bytes.Equal(events[0].Payload, payload) {
		t.Errorf("payload came back as % x, want % x", events[0].Payload, payload)
	}
}

func TestNeedsEscaping(t *testing.T) {
	if NeedsEscaping([]byte("ordinary text")) {
		t.Error("ordinary text was said to need escaping")
	}
	if !NeedsEscaping([]byte{'a', IAC}) {
		t.Error("a byte of 255 was said not to need escaping")
	}
}
