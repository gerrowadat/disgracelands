// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package obs

import (
	"io"
	"log/slog"
	"maps"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
)

// The JSON log format is OpenTelemetry's log data model
// (https://opentelemetry.io/docs/specs/otel/logs/data-model/) written one
// record per line, rather than slog's own default envelope:
//
//	{"_time":"2026-08-24T09:14:02.417365991Z","severity_text":"INFO",
//	 "severity_number":9,"_msg":"entered the world",
//	 "resource":{"host.name":"mud1","process.pid":"7","service.name":"dlmud",
//	             "service.version":"v0.1.0"},
//	 "attributes":{"character":"Zod","room":3001}}
//
// Two things are worth knowing about the field names.
//
// `_time` and `_msg` are not OpenTelemetry's names for those fields — the
// data model calls them Timestamp and Body — they are VictoriaLogs' two
// special fields, the ones it will not guess at (_time_field and _msg_field,
// defaulting to exactly these). Naming them this way is what makes a line
// ingestable with no per-source configuration at all, which is the whole
// point of picking a shape rather than leaving slog's. Everything else on
// the line is a semantic-convention name, so a backend that does understand
// OpenTelemetry finds what it expects: severity_text/severity_number,
// resource, attributes, code.
//
// Nesting is deliberate too, and is the reason a record's own attributes are
// not simply left at the top level. VictoriaLogs flattens nested JSON into
// dotted field names on ingestion, so `resource` and `attributes` cost
// nothing there — they arrive as `resource.service.name` and
// `attributes.character` — while making it structurally impossible for a
// caller logging an attribute called "severity_text" or "_time" to collide
// with the record's own.
const (
	// timeKey and messageKey are the two VictoriaLogs reads by name.
	timeKey    = "_time"
	messageKey = "_msg"

	severityTextKey   = "severity_text"
	severityNumberKey = "severity_number"

	resourceKey   = "resource"
	attributesKey = "attributes"
	codeKey       = "code"
)

// otelTimeLayout is RFC 3339 with a fixed nine-digit fractional second.
// slog's own JSON handler emits milliseconds and trims nothing; the data
// model's Timestamp is nanoseconds, and a fixed width means the raw text
// sorts in timestamp order, which is worth the eight bytes when the file is
// being read with sort(1) rather than a query language.
const otelTimeLayout = "2006-01-02T15:04:05.000000000Z07:00"

// Resource describes the emitting service: OpenTelemetry's Resource, as
// semantic-convention attribute names against their values. Values are
// strings because that is what the environment supplies and what OTLP/JSON
// encodes even integers as.
type Resource map[string]string

// DetectResource assembles the resource for this process. service and version
// are the caller's defaults; the two standard environment variables override
// them, so a deployment can label its logs the same way it labels everything
// else it runs without this binary growing flags of its own:
//
//	OTEL_SERVICE_NAME=dlmud-test
//	OTEL_RESOURCE_ATTRIBUTES=deployment.environment=staging,service.namespace=mud
//
// lookupEnv is usually os.LookupEnv, and is a parameter for the same reason
// [config.Load] takes one: so a test need not mutate process state.
func DetectResource(lookupEnv func(string) (string, bool), service, version string) Resource {
	r := Resource{}

	// OTEL_RESOURCE_ATTRIBUTES first, so that OTEL_SERVICE_NAME can override
	// a service.name set in it. The specification requires that precedence
	// explicitly, and the format it defines is W3C Baggage: comma-separated
	// key=value with percent-encoded values. PathUnescape rather than
	// QueryUnescape, because "+" is a literal plus in that encoding and not
	// a space.
	if raw, ok := lookupEnv("OTEL_RESOURCE_ATTRIBUTES"); ok {
		for pair := range strings.SplitSeq(raw, ",") {
			key, value, found := strings.Cut(pair, "=")
			key, value = strings.TrimSpace(key), strings.TrimSpace(value)
			if !found || key == "" {
				continue
			}
			if decoded, err := url.PathUnescape(value); err == nil {
				value = decoded
			}
			r[key] = value
		}
	}
	if name, ok := lookupEnv("OTEL_SERVICE_NAME"); ok && name != "" {
		r["service.name"] = name
	}

	r.setDefault("service.name", service)
	r.setDefault("service.version", version)
	if host, err := os.Hostname(); err == nil {
		r.setDefault("host.name", host)
	}
	r.setDefault("process.pid", strconv.Itoa(os.Getpid()))
	return r
}

// setDefault fills in a value the environment did not supply. An empty value
// is not a value: buildinfo reports "" rather than a version for a binary
// built without -ldflags, and a resource attribute set to the empty string is
// worse than an absent one.
func (r Resource) setDefault(key, value string) {
	if value == "" {
		return
	}
	if _, ok := r[key]; !ok {
		r[key] = value
	}
}

// attrs renders the resource as slog attributes, sorted by key so that two
// runs of the same server produce byte-identical resource blocks.
func (r Resource) attrs() []slog.Attr {
	attrs := make([]slog.Attr, 0, len(r))
	for _, key := range slices.Sorted(maps.Keys(r)) {
		attrs = append(attrs, slog.String(key, r[key]))
	}
	return attrs
}

// newOTelHandler builds the JSON handler described at the top of this file.
//
// It is slog's own JSON handler with the built-in attributes renamed rather
// than a handler written from scratch, which is worth being explicit about
// because the two structural pieces both look like accidents otherwise:
//
//   - The resource goes on with WithAttrs *before* WithGroup, because a
//     handler's preformatted attributes are written before its open groups
//     are. That ordering is what puts `resource` at the top level and
//     everything a caller logs afterwards inside `attributes`.
//   - WithGroup(attributesKey) is left open forever. slog's built-in
//     time/level/source/message are emitted outside any group, so they are
//     unaffected; every attribute from a Record or a later .With() lands
//     inside it. A logger derived with .With() keeps working because the
//     handler preformats the open group along with the attributes.
func newOTelHandler(w io.Writer, opts LogOptions) slog.Handler {
	var h slog.Handler = slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level:       opts.Level,
		AddSource:   opts.AddSource,
		ReplaceAttr: otelReplaceAttr,
	})
	if res := opts.Resource.attrs(); len(res) > 0 {
		h = h.WithAttrs([]slog.Attr{{Key: resourceKey, Value: slog.GroupValue(res...)}})
	}
	return h.WithGroup(attributesKey)
}

// otelReplaceAttr rewrites slog's four built-in attributes into the data
// model's fields.
func otelReplaceAttr(groups []string, a slog.Attr) slog.Attr {
	// Only the built-ins are written outside a group — anything a caller
	// logged is already inside `attributes` by the time it gets here — so an
	// open group is the reliable signal that this attribute is somebody
	// else's and must be passed through untouched. It also stops the
	// rewrites below recursing: slog feeds the attributes they return back
	// through this same function.
	if len(groups) > 0 {
		return a
	}

	switch a.Key {
	case slog.TimeKey:
		return slog.String(timeKey, a.Value.Time().UTC().Format(otelTimeLayout))

	case slog.MessageKey:
		return slog.Attr{Key: messageKey, Value: a.Value}

	case slog.LevelKey:
		level, ok := a.Value.Any().(slog.Level)
		if !ok {
			return a
		}
		// One built-in attribute has to become two fields. A group with an
		// empty key is inlined rather than nested — slog's documented
		// behaviour, and the only way to do this from ReplaceAttr.
		return slog.Attr{Key: "", Value: slog.GroupValue(
			slog.String(severityTextKey, level.String()),
			slog.Int(severityNumberKey, severityNumber(level)),
		)}

	case slog.SourceKey:
		src, ok := a.Value.Any().(*slog.Source)
		if !ok || src.File == "" {
			// An empty Attr is dropped, which is what a record with no
			// recoverable caller should produce rather than a `code` block
			// of empty strings.
			return slog.Attr{}
		}
		return slog.Attr{Key: codeKey, Value: slog.GroupValue(
			slog.String("file.path", src.File),
			slog.Int("line.number", src.Line),
			slog.String("function.name", src.Function),
		)}
	}

	return a
}

// The data model's SeverityNumber range: 1-4 TRACE, 5-8 DEBUG, 9-12 INFO,
// 13-16 WARN, 17-20 ERROR, 21-24 FATAL.
const (
	severityMin = 1
	severityMax = 24

	// slog's levels are spaced to make this a straight offset:
	// Debug(-4)+9 = 5 = DEBUG, Info(0)+9 = 9 = INFO, Warn(4)+9 = 13 = WARN,
	// Error(8)+9 = 17 = ERROR. The offset rather than a four-way switch
	// because it carries slog's in-between levels across too — Info+2 is
	// INFO3, which is exactly what the data model reserves those for.
	severityOffset = 9
)

// severityNumber maps an slog level onto the data model's SeverityNumber.
// TestSeverityNumber pins the four standard levels, so a future slog that
// respaced them fails a test rather than quietly mislabelling every line.
func severityNumber(level slog.Level) int {
	return min(max(int(level)+severityOffset, severityMin), severityMax)
}
