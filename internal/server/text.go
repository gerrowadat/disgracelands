// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Text holds the server's canned files.
//
// Two of these are licence obligations rather than content: the greeting must
// name the DikuMUD and CircleMUD creators, and the credits must be shown
// intact by the `credits` command (docs/proposals/go-port-plan.md §12). They
// are loaded at boot and their absence is a startup failure, not a warning —
// a server that cannot meet the licence should not begin serving.
type Text struct {
	greeting string
	motd     string
	credits  string
}

// text files, relative to the data directory.
const (
	greetingFile = "text/greetings"
	motdFile     = "text/motd"
	creditsFile  = "text/credits"
)

// LoadText reads the canned files from a data directory.
func LoadText(dir string) (*Text, error) {
	t := &Text{}

	// Required. The licence names both of these.
	for _, f := range []struct {
		path string
		dst  *string
		why  string
	}{
		{greetingFile, &t.greeting, "the login sequence must name the DikuMUD and CircleMUD creators"},
		{creditsFile, &t.credits, "the credits must be displayed by the `credits` command"},
	} {
		b, err := os.ReadFile(filepath.Join(dir, f.path)) //nolint:gosec // operator-configured data directory
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w (required: %s — see docs/proposals/go-port-plan.md §12)",
				f.path, err, f.why)
		}
		if strings.TrimSpace(string(b)) == "" {
			return nil, fmt.Errorf("%s is empty (required: %s)", f.path, f.why)
		}
		*f.dst = string(b)
	}

	// Optional: a server with no message of the day is merely quiet.
	if b, err := os.ReadFile(filepath.Join(dir, motdFile)); err == nil { //nolint:gosec // as above
		t.motd = string(b)
	}

	return t, nil
}

// Greeting implements session.TextFiles.
func (t *Text) Greeting() string { return t.greeting }

// MOTD implements session.TextFiles.
func (t *Text) MOTD() string { return t.motd }

// Credits implements session.TextFiles.
func (t *Text) Credits() string { return t.credits }
