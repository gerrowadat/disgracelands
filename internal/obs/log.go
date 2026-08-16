// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

// Package obs provides the server's logging, metrics and health endpoints.
//
// The C server's mudlog() had a second job beyond writing to the log file: it
// echoed messages to online immortals whose level and preferences matched. That
// behaviour is how gods actually watch the game and it is preserved here as the
// WizVis attribute (see [WizLevel]) rather than being dropped in favour of
// plain structured logging. Nothing consumes it yet; the session layer will.
package obs

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
)

// WizVisKey is the attribute key carrying the minimum immortal level that
// should see a log line echoed in-game. Lines without it are log-only.
const WizVisKey = "wizvis"

// WizLevel returns an attribute marking a log line for in-game echo to
// immortals of at least the given level, mirroring mudlog()'s level argument.
func WizLevel(level int) slog.Attr {
	return slog.Int(WizVisKey, level)
}

// LogOptions configures [NewLogger].
type LogOptions struct {
	// File is the log destination. "-" or "" means stderr, matching the C
	// server's default of writing to stderr when -o is absent.
	File string
	// Format is "text" or "json".
	Format string
	// Level is the minimum level to emit.
	Level slog.Level
	// AddSource includes the emitting file and line. Useful at debug level.
	AddSource bool
}

// NewLogger builds a logger from opts. The returned closer is non-nil only
// when a file was opened; closing stderr would be unhelpful.
func NewLogger(opts LogOptions) (*slog.Logger, io.Closer, error) {
	var (
		w      io.Writer = os.Stderr
		closer io.Closer
	)
	if opts.File != "" && opts.File != "-" {
		// 0600: the log carries connecting hosts and player names, which is
		// the same class of data the pfile policy keeps off disk elsewhere.
		f, err := os.OpenFile(opts.File, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			return nil, nil, fmt.Errorf("open log file: %w", err)
		}
		w, closer = f, f
	}

	handlerOpts := &slog.HandlerOptions{
		Level:     opts.Level,
		AddSource: opts.AddSource,
	}

	var h slog.Handler
	switch strings.ToLower(opts.Format) {
	case "json":
		h = slog.NewJSONHandler(w, handlerOpts)
	case "text", "":
		h = slog.NewTextHandler(w, handlerOpts)
	default:
		return nil, nil, fmt.Errorf("unknown log format %q", opts.Format)
	}

	return slog.New(h), closer, nil
}
