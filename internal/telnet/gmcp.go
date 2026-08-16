// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package telnet

import (
	"encoding/json"
	"fmt"
	"strings"
)

// GMCP: the Generic MUD Communication Protocol.
//
// A GMCP message is a package name, a space, and a JSON value, carried in a
// telnet subnegotiation on option 201. It is how a client learns a
// character's hit points without parsing them out of a prompt string, and
// §0's intended web front end is the reason it is built here rather than
// bolted on later — a browser client that has to scrape text is not a client,
// it is a terminal emulator with extra steps.

// GMCPMessage is one package and its payload.
type GMCPMessage struct {
	// Package is the dotted name, "Char.Vitals".
	Package string
	// Data is the JSON value, or nil for a bare package name.
	Data json.RawMessage
}

// ParseGMCP splits a subnegotiation payload into a package name and its JSON.
//
// A payload with no JSON at all is valid — "Core.Ping" is a complete message
// — so a missing body is not an error.
func ParseGMCP(payload []byte) (GMCPMessage, error) {
	text := strings.TrimSpace(string(payload))
	if text == "" {
		return GMCPMessage{}, fmt.Errorf("empty GMCP message")
	}

	name, body, found := strings.Cut(text, " ")
	msg := GMCPMessage{Package: name}
	if !found || strings.TrimSpace(body) == "" {
		return msg, nil
	}

	body = strings.TrimSpace(body)
	if !json.Valid([]byte(body)) {
		return msg, fmt.Errorf("GMCP package %q: payload is not valid JSON", name)
	}
	msg.Data = json.RawMessage(body)
	return msg, nil
}

// GMCP builds the bytes for one outgoing message.
//
// data is marshalled to JSON; a nil data sends the package name alone.
func GMCP(pkg string, data any) ([]byte, error) {
	body := []byte(pkg)
	if data != nil {
		encoded, err := json.Marshal(data)
		if err != nil {
			return nil, fmt.Errorf("encoding GMCP package %s: %w", pkg, err)
		}
		body = append(body, ' ')
		body = append(body, encoded...)
	}
	return Subnegotiate(OptGMCP, body), nil
}

// Supports records which GMCP packages a client asked for.
//
// A client sends Core.Supports.Set with a list of "Package n" strings at the
// version it understands. Sending a package nobody asked for is harmless but
// wasteful, and on a chatty package like Char.Vitals — which goes out on
// every prompt — it is worth not doing.
type Supports struct {
	packages map[string]int
	// told records that the client has sent a Core.Supports message. It is
	// not the same as a non-empty set: a client that removed everything has
	// asked for nothing, while a client that has said nothing at all gets
	// the standard packages.
	told bool
}

// Set replaces the supported set from a Core.Supports.Set payload.
func (s *Supports) Set(list []string) {
	s.packages = map[string]int{}
	s.told = true
	s.Add(list)
}

// Add merges a Core.Supports.Add payload into the set.
func (s *Supports) Add(list []string) {
	if s.packages == nil {
		s.packages = map[string]int{}
	}
	s.told = true
	for _, entry := range list {
		name, version := splitSupport(entry)
		if name != "" {
			s.packages[strings.ToLower(name)] = version
		}
	}
}

// Remove applies a Core.Supports.Remove payload.
func (s *Supports) Remove(list []string) {
	s.told = true
	for _, entry := range list {
		name, _ := splitSupport(entry)
		delete(s.packages, strings.ToLower(name))
	}
}

// Wants reports whether the client asked for a package.
//
// Before any Core.Supports message arrives nothing is known, and the answer
// is yes: a client that enabled GMCP and said nothing more still gets the
// standard packages, which is what most of them expect.
func (s *Supports) Wants(pkg string) bool {
	if !s.told {
		return true
	}
	// "Char.Vitals" is covered by support for "Char".
	name := strings.ToLower(pkg)
	for {
		if _, ok := s.packages[name]; ok {
			return true
		}
		cut := strings.LastIndex(name, ".")
		if cut < 0 {
			return false
		}
		name = name[:cut]
	}
}

// splitSupport parses "Char 1" into its name and version.
func splitSupport(entry string) (string, int) {
	entry = strings.TrimSpace(entry)
	name, version, found := strings.Cut(entry, " ")
	if !found {
		return name, 1
	}
	n := 0
	for _, r := range strings.TrimSpace(version) {
		if r < '0' || r > '9' {
			return name, 1
		}
		n = n*10 + int(r-'0')
	}
	return name, n
}
