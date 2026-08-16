// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package telnet

import (
	"strings"
	"testing"
)

func TestCharsetRequestOffersUTF8First(t *testing.T) {
	wire := CharsetRequestBytes()

	var p Parser
	p.Feed(nil, wire)
	events := p.Events()
	if len(events) != 1 || events[0].Option != OptCharset {
		t.Fatalf("got %+v, want one CHARSET subnegotiation", events)
	}

	payload := events[0].Payload
	if payload[0] != CharsetRequest {
		t.Fatalf("payload starts with %d, want REQUEST (%d)", payload[0], CharsetRequest)
	}
	list := string(payload[1:])
	if !strings.HasPrefix(list, ";UTF-8") {
		t.Errorf("charset list is %q, want UTF-8 offered first", list)
	}
}

func TestParseCharsetResponse(t *testing.T) {
	name, ok := ParseCharsetResponse(append([]byte{CharsetAccepted}, "ISO-8859-1"...))
	if !ok || name != "ISO-8859-1" {
		t.Errorf("accepted response parsed as %q, %v", name, ok)
	}

	if _, ok := ParseCharsetResponse([]byte{CharsetRejected}); ok {
		t.Error("a rejection was read as an acceptance")
	}
	if _, ok := ParseCharsetResponse(nil); ok {
		t.Error("an empty payload was read as an acceptance")
	}
	if _, ok := ParseCharsetResponse([]byte{CharsetTTableIs, 'x'}); ok {
		t.Error("a TTABLE-IS was read as an acceptance")
	}
}

// TestUTF8NeedsNoEncoder: the server's output is already UTF-8, so a client
// that accepts it must cost nothing.
func TestUTF8NeedsNoEncoder(t *testing.T) {
	for _, name := range []string{"UTF-8", "utf-8", " UTF-8 ", "something-nobody-offered"} {
		if Encoder(name) != nil {
			t.Errorf("Encoder(%q) returned a transformer, want nil", name)
		}
	}
}

func TestLatin1Encoding(t *testing.T) {
	enc := Encoder("ISO-8859-1")
	if enc == nil {
		t.Fatal("no encoder for ISO-8859-1")
	}

	// A pound sign is one byte in Latin-1 and two in UTF-8.
	got := EncodeTo(enc, []byte("£5"))
	if len(got) != 2 || got[0] != 0xA3 || got[1] != '5' {
		t.Errorf("encoded to % x, want a3 35", got)
	}
}

// TestUnrepresentableCharactersDoNotCostTheSentence: an em dash has no
// Latin-1 byte, and dropping the line it was in would be worse than
// substituting for it.
func TestUnrepresentableCharactersDoNotCostTheSentence(t *testing.T) {
	enc := Encoder("ISO-8859-1")
	got := string(EncodeTo(enc, []byte("a — b")))
	if !strings.HasPrefix(got, "a ") || !strings.HasSuffix(got, " b") {
		t.Errorf("encoded to %q, want the surrounding text intact", got)
	}
}

func TestEncodeToWithNoEncoderIsIdentity(t *testing.T) {
	in := []byte("a — b")
	got := EncodeTo(nil, in)
	if string(got) != string(in) {
		t.Errorf("got %q, want %q", got, in)
	}
}
