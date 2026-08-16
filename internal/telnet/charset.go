// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package telnet

import (
	"strings"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/transform"
)

// CHARSET negotiation, RFC 2066.
//
// The server's output is UTF-8 (docs/proposals/go-port-plan.md §0). A client
// that cannot read it says so here, and gets its own encoding instead. That
// is a per-connection concern and this is the right place for it — as against
// decoding the world files on every read, which is what `dlctl convert`
// exists to make unnecessary.

// charsetSeparator is the delimiter the server uses in its REQUEST. RFC 2066
// lets the sender pick one; a semicolon is what every client expects.
const charsetSeparator = ";"

// offeredCharsets is what the server will speak, best first.
//
// UTF-8 leads because that is what the data is. Latin-1 follows because the
// archived world files are full of bytes that were Latin-1 when they were
// typed, and a client stuck on it should see the pound signs and accents
// rather than mojibake.
var offeredCharsets = []string{"UTF-8", "ISO-8859-1", "US-ASCII"}

// CharsetRequestBytes builds the server's REQUEST subnegotiation.
func CharsetRequestBytes() []byte {
	payload := []byte{CharsetRequest}
	for _, name := range offeredCharsets {
		payload = append(payload, charsetSeparator...)
		payload = append(payload, name...)
	}
	return Subnegotiate(OptCharset, payload)
}

// Encoder returns a transformer that converts the server's UTF-8 output into
// the named charset, or nil if the name is UTF-8 or is not one the server
// offered.
//
// A nil transformer means "send the bytes as they are", which is the right
// answer both for UTF-8 and for a charset that should never have been
// accepted, since mangling output is worse than sending it unconverted.
//
// Every encoder replaces what it cannot represent rather than failing: one
// character the client cannot show must not cost them the sentence it was in.
func Encoder(name string) transform.Transformer {
	var enc *encoding.Encoder
	switch normaliseCharset(name) {
	case "iso-8859-1", "latin1", "iso8859-1", "iso_8859-1", "cp819":
		enc = charmap.ISO8859_1.NewEncoder()
	case "us-ascii", "ascii":
		// There is no ASCII encoder in x/text, and Latin-1 is a superset, so
		// this at least keeps every byte a client can read intact rather than
		// refusing to convert at all.
		enc = charmap.ISO8859_1.NewEncoder()
	case "cp1252", "windows-1252":
		enc = charmap.Windows1252.NewEncoder()
	default:
		return nil
	}
	return encoding.ReplaceUnsupported(enc)
}

// ParseCharsetResponse reads an ACCEPTED or REJECTED subnegotiation and
// returns the accepted charset name. A rejection returns "".
func ParseCharsetResponse(payload []byte) (name string, accepted bool) {
	if len(payload) == 0 {
		return "", false
	}
	switch payload[0] {
	case CharsetAccepted:
		return strings.TrimSpace(string(payload[1:])), true
	case CharsetRejected:
		return "", false
	}
	// TTABLE-IS and friends: the server never offers a translation table, so
	// anything else is a client saying something that was not asked for.
	return "", false
}

// normaliseCharset lowercases and strips the punctuation clients vary on.
func normaliseCharset(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	return strings.TrimPrefix(name, "charset ")
}

// EncodeTo converts UTF-8 text with the given encoder, returning the input
// unchanged if it cannot be converted.
//
// Encoder already substitutes for characters the charset lacks, so an error
// here means something worse — and sending the original is still better than
// sending nothing.
func EncodeTo(enc transform.Transformer, b []byte) []byte {
	if enc == nil {
		return b
	}
	out, _, err := transform.Bytes(enc, b)
	if err != nil {
		return b
	}
	return out
}
