// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

// Package telnet speaks enough of the telnet protocol for a MUD.
//
// The C server does the minimum: it sends IAC WILL ECHO to hide a password
// and IAC WONT ECHO afterwards (comm.c's echo_off_str), and otherwise leaves
// negotiation bytes in the input stream for the interpreter to trip over.
// That was survivable in 1993 because everyone used a telnet client that
// negotiated nothing. It is not survivable now: a client that offers window
// size gets its NAWS bytes treated as a command.
//
// So this package parses the stream properly, which also buys the things a
// modern client expects and docs/design/go-port-plan.md §0 asks for —
// CHARSET, so the server can say it speaks UTF-8, and GMCP, so a browser
// front end can be a real client rather than a screen-scraper.
package telnet

import "fmt"

// Telnet commands (RFC 854).
const (
	// SE ends a subnegotiation.
	SE byte = 240
	// NOP, DataMark, Break and the rest of the RFC 854 commands are accepted
	// and ignored; they are listed so the parser's skip is deliberate rather
	// than a fallthrough.
	NOP      byte = 241
	DataMark byte = 242
	Break    byte = 243
	IP       byte = 244
	AO       byte = 245
	AYT      byte = 246
	EC       byte = 247
	EL       byte = 248
	GA       byte = 249
	// SB begins a subnegotiation.
	SB   byte = 250
	WILL byte = 251
	WONT byte = 252
	DO   byte = 253
	DONT byte = 254
	// IAC introduces every command. A literal 255 in data is sent twice.
	IAC byte = 255
)

// Telnet options.
const (
	// OptEcho is what hides a password: the server says WILL ECHO, the client
	// stops echoing locally, and the server echoes nothing.
	OptEcho byte = 1
	// OptSuppressGoAhead is universal in practice and stops a client waiting
	// for a GA that a MUD never sends.
	OptSuppressGoAhead byte = 3
	OptTerminalType    byte = 24
	OptEndOfRecord     byte = 25
	OptWindowSize      byte = 31
	// OptCharset is RFC 2066: how the server declares that it speaks UTF-8.
	OptCharset byte = 42
	// OptGMCP carries structured out-of-band data. 201 is not registered with
	// IANA; it is what every MUD client uses, which is what matters here.
	OptGMCP byte = 201
)

// CHARSET subnegotiation commands (RFC 2066).
const (
	CharsetRequest        byte = 1
	CharsetAccepted       byte = 2
	CharsetRejected       byte = 3
	CharsetTTableIs       byte = 4
	CharsetTTableRejected byte = 5
	CharsetTTableAck      byte = 6
	CharsetTTableNak      byte = 7
)

// OptionName returns a readable name for logs.
func OptionName(opt byte) string {
	switch opt {
	case OptEcho:
		return "echo"
	case OptSuppressGoAhead:
		return "suppress-go-ahead"
	case OptTerminalType:
		return "terminal-type"
	case OptEndOfRecord:
		return "end-of-record"
	case OptWindowSize:
		return "window-size"
	case OptCharset:
		return "charset"
	case OptGMCP:
		return "gmcp"
	}
	return fmt.Sprintf("option-%d", opt)
}

// CommandName returns a readable name for logs.
func CommandName(cmd byte) string {
	switch cmd {
	case WILL:
		return "WILL"
	case WONT:
		return "WONT"
	case DO:
		return "DO"
	case DONT:
		return "DONT"
	case SB:
		return "SB"
	}
	return fmt.Sprintf("command-%d", cmd)
}

// Negotiate builds a three-byte option negotiation.
func Negotiate(cmd, opt byte) []byte { return []byte{IAC, cmd, opt} }

// Subnegotiate builds IAC SB <opt> <payload> IAC SE.
//
// The payload is escaped, because a payload byte of 255 would otherwise end
// the subnegotiation early and leave the rest of it being read as commands.
func Subnegotiate(opt byte, payload []byte) []byte {
	out := make([]byte, 0, len(payload)+6)
	out = append(out, IAC, SB, opt)
	out = Escape(out, payload)
	return append(out, IAC, SE)
}

// Escape appends data to dst with every IAC byte doubled, as RFC 854
// requires for data that is not a command.
func Escape(dst, data []byte) []byte {
	for _, b := range data {
		if b == IAC {
			dst = append(dst, IAC)
		}
		dst = append(dst, b)
	}
	return dst
}

// NeedsEscaping reports whether data contains a byte that Escape would
// double. Almost no output ever does, so the common case can skip the copy.
func NeedsEscaping(data []byte) bool {
	for _, b := range data {
		if b == IAC {
			return true
		}
	}
	return false
}
