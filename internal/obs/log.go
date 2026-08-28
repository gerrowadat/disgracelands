// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

// Package obs provides the server's logging, metrics and health endpoints.
//
// The C server's mudlog() had a second job beyond writing to the log file: it
// echoed messages to online immortals whose level and preferences matched.
// That behaviour is how gods actually watch the game and it is preserved
// here as the WizVis attributes ([WizLevel], [WizType]) rather than being
// dropped in favour of plain structured logging. This package cannot reach
// the live connections that would consume them — that would be an import
// cycle — so [WithWizVisEcho] wraps a handler with a callback instead,
// which the server package supplies.
package obs

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
)

// WizVisKey is the attribute key carrying the minimum immortal level that
// should see a log line echoed in-game. Lines without it are log-only, and
// so are lines carrying a *negative* one — mudlog returns before its echo
// loop when `level < 0` (utils.c:238-239), which do_skillset relies on
// (modify.c:344). Applying that is the echo's job, not this package's:
// see [WizVisEcho].
const WizVisKey = "wizvis"

// WizVisTypeKey carries mudlog()'s own `type` argument (BRF/NRM/CMP/OFF,
// utils.h:89-92): an online immortal's own syslog verbosity (the two
// PRF_LOG bits read together as one number) has to be at least this for
// the line to reach them, mudlog()'s own `if (tp < type) continue`
// (utils.c:252-253). A record carrying WizVisKey but not this one is not
// echoed at all — see [WithWizVisEcho].
const WizVisTypeKey = "wizvis_type"

// The four syslog verbosities, `OFF`/`BRF`/`NRM`/`CMP` (utils.h:89-92). OFF
// is the least restrictive as a message's own *type*, not a reader's
// setting: mudlog()'s `tp < type` can never be true against it, so a
// LogOff-typed line reaches every qualifying immortal regardless of their
// own syslog preference.
const (
	LogOff = iota
	LogBrief
	LogNormal
	LogComplete
)

// WizLevel returns an attribute marking a log line for in-game echo to
// immortals of at least the given level, mirroring mudlog()'s level
// argument — including a negative one, which reaches nobody. Needs a
// [WizType] alongside it, or [WithWizVisEcho] will not echo the line at
// all.
func WizLevel(level int) slog.Attr {
	return slog.Int(WizVisKey, level)
}

// WizType returns an attribute carrying mudlog()'s own type argument (one
// of the Log* constants) — see [WizVisTypeKey].
func WizType(typ int) slog.Attr {
	return slog.Int(WizVisTypeKey, typ)
}

// WizVisEcho delivers one wizvis-tagged log line to whichever online
// immortals qualify for it — mudlog()'s own in-game half (utils.c:236-258),
// the `level < 0` early return included.
// A seam rather than a direct dependency: this package cannot import
// internal/session (the connections it would need to reach live there), so
// the server package supplies the implementation instead, the same shape
// every other Server-side interface in this tree already has.
type WizVisEcho func(typ, level int, message string)

// WithWizVisEcho wraps base so that any record carrying both WizVisKey and
// WizVisTypeKey also reaches echo, in addition to being handled normally —
// every record still reaches base exactly as it would have otherwise,
// whether or not it is wizvis-tagged. A nil echo makes this a no-op
// wrapper, safe to call before there is anything to echo to (boot, or a
// caller — dlctl — with no live connections at all).
func WithWizVisEcho(base slog.Handler, echo WizVisEcho) slog.Handler {
	if echo == nil {
		return base
	}
	return &wizVisHandler{Handler: base, echo: echo}
}

// wizVisHandler embeds the wrapped handler so Enabled passes through
// unchanged, and overrides only the three methods that need to know about
// the relay: Handle (to fire it), and WithAttrs/WithGroup (so a logger
// derived via .With(...) or a group keeps relaying rather than silently
// reverting to the bare base handler).
type wizVisHandler struct {
	slog.Handler
	echo WizVisEcho
}

func (h *wizVisHandler) Handle(ctx context.Context, r slog.Record) error {
	var typ, level int
	var hasType, hasLevel bool
	r.Attrs(func(a slog.Attr) bool {
		switch a.Key {
		case WizVisTypeKey:
			typ, hasType = int(a.Value.Int64()), true
		case WizVisKey:
			level, hasLevel = int(a.Value.Int64()), true
		}
		return true
	})
	if hasType && hasLevel {
		h.echo(typ, level, r.Message)
	}
	return h.Handler.Handle(ctx, r)
}

func (h *wizVisHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &wizVisHandler{Handler: h.Handler.WithAttrs(attrs), echo: h.echo}
}

func (h *wizVisHandler) WithGroup(name string) slog.Handler {
	return &wizVisHandler{Handler: h.Handler.WithGroup(name), echo: h.echo}
}

// LogOptions configures [NewLogger].
type LogOptions struct {
	// File is the log destination. "-" or "" means stderr, matching the C
	// server's default of writing to stderr when -o is absent.
	File string
	// Format is "text" or "json". "json" is the OpenTelemetry-shaped
	// envelope described in otel.go, not slog's own.
	Format string
	// Resource labels every JSON record with what emitted it — see
	// [DetectResource]. Ignored by the text format, which is read by
	// somebody who already knows which server they are looking at.
	Resource Resource
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
		h = newOTelHandler(w, opts)
	case "text", "":
		h = slog.NewTextHandler(w, handlerOpts)
	default:
		return nil, nil, fmt.Errorf("unknown log format %q", opts.Format)
	}

	return slog.New(h), closer, nil
}
