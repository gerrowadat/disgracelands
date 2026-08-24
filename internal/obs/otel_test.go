// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package obs

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"
)

// jsonLogger returns a logger writing the OpenTelemetry-shaped JSON format
// into buf, plus a records() that decodes whatever it has written so far.
func jsonLogger(t *testing.T, res Resource, opts ...func(*LogOptions)) (*slog.Logger, func() []map[string]any) {
	t.Helper()

	buf := &bytes.Buffer{}
	o := LogOptions{Level: slog.LevelDebug, Resource: res}
	for _, fn := range opts {
		fn(&o)
	}

	logger := slog.New(newOTelHandler(buf, o))
	return logger, func() []map[string]any {
		t.Helper()
		var out []map[string]any
		for line := range strings.SplitSeq(strings.TrimSpace(buf.String()), "\n") {
			if line == "" {
				continue
			}
			var rec map[string]any
			if err := json.Unmarshal([]byte(line), &rec); err != nil {
				t.Fatalf("log line is not JSON: %v\n%s", err, line)
			}
			out = append(out, rec)
		}
		return out
	}
}

// nested returns the object at key, or fails.
func nested(t *testing.T, rec map[string]any, key string) map[string]any {
	t.Helper()
	sub, ok := rec[key].(map[string]any)
	if !ok {
		t.Fatalf("record has no %q object: %v", key, rec)
	}
	return sub
}

// TestJSONRecordIsTheOTelDataModel.
//
// The whole envelope in one assertion, because the field *names* are the
// contract: a log pipeline is configured against them once and then nobody
// looks at it again until it silently stops matching.
func TestJSONRecordIsTheOTelDataModel(t *testing.T) {
	logger, records := jsonLogger(t, Resource{"service.name": "dlmud", "service.version": "v0.1.0"})
	logger.Info("entered the world", "character", "Zod", "room", 3001)

	recs := records()
	if len(recs) != 1 {
		t.Fatalf("got %d records, want 1", len(recs))
	}
	rec := recs[0]

	if rec["_msg"] != "entered the world" {
		t.Errorf("_msg = %v, want the message; slog's own key is msg, and VictoriaLogs will not find it there", rec["_msg"])
	}
	if rec["severity_text"] != "INFO" {
		t.Errorf("severity_text = %v, want INFO", rec["severity_text"])
	}
	if rec["severity_number"] != float64(9) {
		t.Errorf("severity_number = %v, want 9 (the data model's INFO)", rec["severity_number"])
	}

	// The record's own attributes are grouped, so that a caller cannot
	// collide with the fields above by logging one called "severity_text".
	attrs := nested(t, rec, "attributes")
	if attrs["character"] != "Zod" || attrs["room"] != float64(3001) {
		t.Errorf("attributes = %v, want character=Zod room=3001", attrs)
	}

	res := nested(t, rec, "resource")
	if res["service.name"] != "dlmud" || res["service.version"] != "v0.1.0" {
		t.Errorf("resource = %v, want the service name and version", res)
	}

	// Nothing else at the top level: anything that escaped `attributes`
	// would be a field a backend has to be told about by hand.
	want := []string{"_msg", "_time", "attributes", "resource", "severity_number", "severity_text"}
	if got := slices.Sorted(maps.Keys(rec)); !slices.Equal(got, want) {
		t.Errorf("top-level fields = %v, want %v", got, want)
	}
}

// TestJSONTimestampIsFixedWidthRFC3339.
//
// Nanoseconds because that is the data model's resolution, and a fixed nine
// digits because a trimmed fraction makes the raw text sort out of order —
// ".9Z" sorts after ".10Z".
func TestJSONTimestampIsFixedWidthRFC3339(t *testing.T) {
	logger, records := jsonLogger(t, nil)
	logger.Info("boot")

	raw, ok := records()[0]["_time"].(string)
	if !ok {
		t.Fatalf("_time is not a string: %v", records()[0]["_time"])
	}
	if _, err := time.Parse(time.RFC3339Nano, raw); err != nil {
		t.Errorf("_time %q does not parse as RFC 3339: %v", raw, err)
	}
	if !strings.HasSuffix(raw, "Z") {
		t.Errorf("_time = %q, want UTC", raw)
	}
	if frac := strings.TrimSuffix(raw[strings.Index(raw, ".")+1:], "Z"); len(frac) != 9 {
		t.Errorf("_time = %q has %d fractional digits, want 9", raw, len(frac))
	}
}

// TestSeverityNumber pins the four levels the data model's ranges are
// anchored on. severityNumber is a straight offset rather than a switch, so
// this is what would catch an slog that respaced its own constants.
func TestSeverityNumber(t *testing.T) {
	for _, tc := range []struct {
		level slog.Level
		want  int
	}{
		{slog.LevelDebug, 5},
		{slog.LevelInfo, 9},
		{slog.LevelWarn, 13},
		{slog.LevelError, 17},
		// In-between levels land on the data model's own DEBUG2/INFO3/...,
		// which is exactly what it reserves them for.
		{slog.LevelInfo + 2, 11},
		// And the ends clamp rather than running off either edge.
		{slog.LevelDebug - 100, 1},
		{slog.LevelError + 100, 24},
	} {
		if got := severityNumber(tc.level); got != tc.want {
			t.Errorf("severityNumber(%v) = %d, want %d", tc.level, got, tc.want)
		}
	}
}

// TestJSONNestsDerivedLoggers.
//
// Every session in the server logs through a logger derived with .With(), and
// the per-connection loggers add .WithGroup() on top. Both have to end up
// inside `attributes` rather than escaping to the top level, which is what
// they would do if the handler's own group were applied in the wrong order.
func TestJSONNestsDerivedLoggers(t *testing.T) {
	logger, records := jsonLogger(t, Resource{"service.name": "dlmud"})
	logger.With("conn", 7).WithGroup("gmcp").Debug("charset agreed", "charset", "UTF-8")

	rec := records()[0]
	attrs := nested(t, rec, "attributes")
	if attrs["conn"] != float64(7) {
		t.Errorf("attributes.conn = %v, want 7 (a .With() attribute escaped the group)", attrs["conn"])
	}
	if gmcp := nested(t, attrs, "gmcp"); gmcp["charset"] != "UTF-8" {
		t.Errorf("attributes.gmcp = %v, want charset=UTF-8", gmcp)
	}
	if _, ok := rec["conn"]; ok {
		t.Errorf("conn is at the top level as well: %v", rec)
	}
	// The resource still has to survive being wrapped.
	if res := nested(t, rec, "resource"); res["service.name"] != "dlmud" {
		t.Errorf("resource = %v, want the service name", res)
	}
}

// TestJSONSourceBecomesCode checks --log-level=debug's caller information
// against the semantic convention's names for it.
func TestJSONSourceBecomesCode(t *testing.T) {
	logger, records := jsonLogger(t, nil, func(o *LogOptions) { o.AddSource = true })
	logger.Info("boot")

	code := nested(t, records()[0], "code")
	if path, _ := code["file.path"].(string); !strings.HasSuffix(path, "otel_test.go") {
		t.Errorf("code.file.path = %v, want this file", code["file.path"])
	}
	if line, _ := code["line.number"].(float64); line <= 0 {
		t.Errorf("code.line.number = %v, want a line number", code["line.number"])
	}
	if fn, _ := code["function.name"].(string); !strings.HasSuffix(fn, "TestJSONSourceBecomesCode") {
		t.Errorf("code.function.name = %v, want this test", code["function.name"])
	}
}

// TestJSONWithoutAResourceOmitsTheBlock: dlctl and the tests have nothing to
// say about what they are, and an empty object on every line is noise a
// backend still has to index.
func TestJSONWithoutAResourceOmitsTheBlock(t *testing.T) {
	logger, records := jsonLogger(t, nil)
	logger.Info("boot")

	if _, ok := records()[0]["resource"]; ok {
		t.Errorf("empty resource still emitted a block: %v", records()[0])
	}
}

// TestJSONEmitsWizVisAttributes.
//
// The in-game echo tags are ordinary attributes as far as the encoder is
// concerned, and they are worth keeping on the line rather than stripping:
// "which lines did a god see" is a real query, and it is only answerable if
// the tag ships with the record.
func TestJSONEmitsWizVisAttributes(t *testing.T) {
	logger, records := jsonLogger(t, nil)
	logger.Info("Zod bug: the door is stuck", WizLevel(31), WizType(LogComplete))

	attrs := nested(t, records()[0], "attributes")
	if attrs[WizVisKey] != float64(31) || attrs[WizVisTypeKey] != float64(LogComplete) {
		t.Errorf("attributes = %v, want %s=31 %s=%d", attrs, WizVisKey, WizVisTypeKey, LogComplete)
	}
}

func TestDetectResourceDefaults(t *testing.T) {
	none := func(string) (string, bool) { return "", false }
	res := DetectResource(none, "dlmud", "v1.2.3")

	if res["service.name"] != "dlmud" || res["service.version"] != "v1.2.3" {
		t.Errorf("resource = %v, want the caller's service and version", res)
	}
	if res["host.name"] == "" {
		t.Errorf("resource = %v, want a host name", res)
	}
	if res["process.pid"] == "" {
		t.Errorf("resource = %v, want a pid", res)
	}
}

// TestDetectResourceOmitsAnUnknownVersion: buildinfo hands back "" for a
// binary built without -ldflags and without a VCS stamp, and an attribute set
// to the empty string is worse than an absent one.
func TestDetectResourceOmitsAnUnknownVersion(t *testing.T) {
	none := func(string) (string, bool) { return "", false }
	if res := DetectResource(none, "dlmud", ""); res["service.version"] != "" {
		t.Errorf("service.version = %q, want it absent", res["service.version"])
	}
}

func TestDetectResourceReadsTheEnvironment(t *testing.T) {
	env := map[string]string{
		"OTEL_SERVICE_NAME": "dlmud-staging",
		// service.name here loses to OTEL_SERVICE_NAME, which the
		// specification requires; the space is percent-encoded, which is the
		// W3C Baggage format it also requires.
		"OTEL_RESOURCE_ATTRIBUTES": "service.name=ignored, deployment.environment=staging ,service.namespace=mud%20one,malformed,",
	}
	res := DetectResource(func(k string) (string, bool) { v, ok := env[k]; return v, ok }, "dlmud", "v1.2.3")

	for key, want := range map[string]string{
		"service.name":           "dlmud-staging",
		"deployment.environment": "staging",
		"service.namespace":      "mud one",
		"service.version":        "v1.2.3",
	} {
		if res[key] != want {
			t.Errorf("resource[%q] = %q, want %q", key, res[key], want)
		}
	}
	if _, ok := res["malformed"]; ok {
		t.Errorf("a pair with no = became an attribute: %v", res)
	}
}

// TestResourceAttrsAreSorted: two runs of the same server should produce
// byte-identical resource blocks, or a diff of two log files is unreadable.
func TestResourceAttrsAreSorted(t *testing.T) {
	res := Resource{"service.version": "v1", "host.name": "h", "service.name": "dlmud"}
	var keys []string
	for _, a := range res.attrs() {
		keys = append(keys, a.Key)
	}
	if want := []string{"host.name", "service.name", "service.version"}; !slices.Equal(keys, want) {
		t.Errorf("resource attribute order = %v, want %v", keys, want)
	}
}
