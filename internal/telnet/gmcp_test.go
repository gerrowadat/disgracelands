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

func TestParseGMCP(t *testing.T) {
	for _, tc := range []struct {
		in      string
		pkg     string
		data    string
		wantErr bool
	}{
		{in: `Core.Hello {"client":"Mudlet","version":"4.0"}`, pkg: "Core.Hello", data: `{"client":"Mudlet","version":"4.0"}`},
		{in: `Core.Supports.Set ["Char 1","Room 1"]`, pkg: "Core.Supports.Set", data: `["Char 1","Room 1"]`},
		// A bare package name is a complete message.
		{in: "Core.Ping", pkg: "Core.Ping"},
		{in: "Core.Ping   ", pkg: "Core.Ping"},
		{in: "", wantErr: true},
		{in: "Core.Hello {not json", pkg: "Core.Hello", wantErr: true},
	} {
		msg, err := ParseGMCP([]byte(tc.in))
		if tc.wantErr {
			if err == nil {
				t.Errorf("ParseGMCP(%q) succeeded, want an error", tc.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseGMCP(%q): %v", tc.in, err)
			continue
		}
		if msg.Package != tc.pkg {
			t.Errorf("ParseGMCP(%q) package = %q, want %q", tc.in, msg.Package, tc.pkg)
		}
		if string(msg.Data) != tc.data {
			t.Errorf("ParseGMCP(%q) data = %q, want %q", tc.in, msg.Data, tc.data)
		}
	}
}

func TestGMCPEncodesAsASubnegotiation(t *testing.T) {
	wire, err := GMCP("Char.Vitals", map[string]int{"hp": 21})
	if err != nil {
		t.Fatal(err)
	}

	var p Parser
	if data := p.Feed(nil, wire); len(data) != 0 {
		t.Errorf("GMCP leaked %q into the data stream", data)
	}
	events := p.Events()
	if len(events) != 1 || events[0].Option != OptGMCP {
		t.Fatalf("got %+v, want one GMCP subnegotiation", events)
	}

	msg, err := ParseGMCP(events[0].Payload)
	if err != nil {
		t.Fatal(err)
	}
	if msg.Package != "Char.Vitals" || string(msg.Data) != `{"hp":21}` {
		t.Errorf("round-tripped to %q %q", msg.Package, msg.Data)
	}
}

func TestGMCPWithNoData(t *testing.T) {
	wire, err := GMCP("Core.Goodbye", nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(wire), " ") {
		t.Errorf("a package with no data got a trailing space: %q", wire)
	}
}

// TestSupportsDefaultsToEverything: a client that turned GMCP on and said
// nothing else still expects the standard packages.
func TestSupportsDefaultsToEverything(t *testing.T) {
	var s Supports
	if !s.Wants("Char.Vitals") {
		t.Error("a silent client was refused Char.Vitals")
	}
}

func TestSupportsMatchesOnPackagePrefix(t *testing.T) {
	var s Supports
	s.Set([]string{"Char 1", "Room 1"})

	for _, pkg := range []string{"Char", "Char.Vitals", "Char.Status", "Room.Info"} {
		if !s.Wants(pkg) {
			t.Errorf("a client supporting Char and Room was refused %s", pkg)
		}
	}
	for _, pkg := range []string{"Comm.Channel", "Chars"} {
		if s.Wants(pkg) {
			t.Errorf("%s was sent to a client that did not ask for it", pkg)
		}
	}
}

func TestSupportsAddAndRemove(t *testing.T) {
	var s Supports
	s.Set([]string{"Char 1"})
	s.Add([]string{"Comm 1"})
	if !s.Wants("Comm.Channel") {
		t.Error("an added package was not honoured")
	}
	s.Remove([]string{"Comm 1"})
	if s.Wants("Comm.Channel") {
		t.Error("a removed package is still being sent")
	}
	// Removing the last entry must not be mistaken for "said nothing".
	s.Remove([]string{"Char 1"})
	if s.Wants("Char.Vitals") {
		t.Error("a client that removed everything is being sent Char.Vitals")
	}
}

func TestSplitSupport(t *testing.T) {
	for in, want := range map[string]struct {
		name    string
		version int
	}{
		"Char 1":   {"Char", 1},
		"Room 2":   {"Room", 2},
		"Char":     {"Char", 1},
		" Char 3 ": {"Char", 3},
		"Char x":   {"Char", 1},
	} {
		name, version := splitSupport(in)
		if name != want.name || version != want.version {
			t.Errorf("splitSupport(%q) = %q, %d; want %q, %d", in, name, version, want.name, want.version)
		}
	}
}
