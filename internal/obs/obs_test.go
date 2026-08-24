// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package obs

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	dto "github.com/prometheus/client_model/go"
)

func TestNewLoggerWritesJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.log")
	logger, closer, err := NewLogger(LogOptions{File: path, Format: "json", Level: slog.LevelInfo})
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	logger.Info("boot", "lib_dir", "data")
	if err := closer.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var rec map[string]any
	if err := json.Unmarshal(data, &rec); err != nil {
		t.Fatalf("log line is not JSON: %v\n%s", err, data)
	}
	// The shape itself is otel_test.go's subject; this is only that the
	// file destination and the JSON format compose.
	attrs, _ := rec["attributes"].(map[string]any)
	if rec["_msg"] != "boot" || attrs["lib_dir"] != "data" {
		t.Errorf("log record = %v, want _msg=boot attributes.lib_dir=data", rec)
	}
}

func TestNewLoggerRespectsLevel(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.log")
	logger, closer, err := NewLogger(LogOptions{File: path, Format: "text", Level: slog.LevelWarn})
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	logger.Info("should not appear")
	logger.Warn("should appear")
	_ = closer.Close()

	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "should not appear") {
		t.Errorf("info line emitted at warn level:\n%s", data)
	}
	if !strings.Contains(string(data), "should appear") {
		t.Errorf("warn line missing:\n%s", data)
	}
}

func TestNewLoggerRejectsUnknownFormat(t *testing.T) {
	if _, _, err := NewLogger(LogOptions{Format: "xml"}); err == nil {
		t.Fatal("NewLogger with format=xml succeeded, want an error")
	}
}

func TestNewLoggerToStderrHasNoCloser(t *testing.T) {
	// Closing stderr would be actively harmful, so "-" must not hand back
	// something the caller will dutifully close.
	_, closer, err := NewLogger(LogOptions{File: "-", Format: "text"})
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	if closer != nil {
		t.Error("closer is non-nil for stderr, want nil")
	}
}

func TestWizLevelAttr(t *testing.T) {
	// mudlog()'s in-game echo level has to survive into the structured record
	// for the session layer to act on later.
	attr := WizLevel(31)
	if attr.Key != WizVisKey {
		t.Errorf("key = %q, want %q", attr.Key, WizVisKey)
	}
	if got := attr.Value.Int64(); got != 31 {
		t.Errorf("value = %d, want 31", got)
	}
}

func TestWizTypeAttr(t *testing.T) {
	attr := WizType(LogComplete)
	if attr.Key != WizVisTypeKey {
		t.Errorf("key = %q, want %q", attr.Key, WizVisTypeKey)
	}
	if got := attr.Value.Int64(); got != LogComplete {
		t.Errorf("value = %d, want %d", got, LogComplete)
	}
}

// recordingEcho collects every call, for tests that only care what reached
// the relay rather than what a real Server would then do with it.
type recordingEcho struct {
	calls []struct {
		typ, level int
		message    string
	}
}

func (r *recordingEcho) echo(typ, level int, message string) {
	r.calls = append(r.calls, struct {
		typ, level int
		message    string
	}{typ, level, message})
}

// discardHandler is a slog.Handler that does nothing, so WithWizVisEcho's
// own tests can build one without a real log destination.
type discardHandler struct{}

func (discardHandler) Enabled(context.Context, slog.Level) bool  { return true }
func (discardHandler) Handle(context.Context, slog.Record) error { return nil }
func (h discardHandler) WithAttrs([]slog.Attr) slog.Handler      { return h }
func (h discardHandler) WithGroup(string) slog.Handler           { return h }

func TestWithWizVisEchoFiresOnlyForTaggedRecords(t *testing.T) {
	var rec recordingEcho
	h := WithWizVisEcho(discardHandler{}, rec.echo)
	logger := slog.New(h)

	logger.Info("plain line, no tags at all")
	logger.Info("level only", WizLevel(31))
	logger.Info("type only", WizType(LogComplete))
	logger.Info("Zod bug: the door is stuck", WizLevel(31), WizType(LogComplete))

	if len(rec.calls) != 1 {
		t.Fatalf("echo fired %d times, want 1 (only the fully-tagged record):\n%+v", len(rec.calls), rec.calls)
	}
	got := rec.calls[0]
	if got.typ != LogComplete || got.level != 31 || got.message != "Zod bug: the door is stuck" {
		t.Errorf("echo call = %+v, want {typ:%d level:31 message:%q}", got, LogComplete, "Zod bug: the door is stuck")
	}
}

func TestWithWizVisEchoNilIsANoop(t *testing.T) {
	// A caller with nothing to echo to yet (dlctl, or boot before a Server
	// exists) must get the base handler back unchanged, not a wrapper that
	// would panic dereferencing a nil callback.
	base := discardHandler{}
	if got := WithWizVisEcho(base, nil); got != slog.Handler(base) {
		t.Error("WithWizVisEcho(base, nil) did not return base unchanged")
	}
}

func TestWithWizVisEchoSurvivesWithAttrsAndWithGroup(t *testing.T) {
	// A logger built with .With(...) or a group must keep relaying —
	// slog.Handler's own contract is that WithAttrs/WithGroup return
	// something that behaves like the original, and a naive wrapper that
	// only overrides Handle would silently stop doing that here.
	var rec recordingEcho
	h := WithWizVisEcho(discardHandler{}, rec.echo)
	logger := slog.New(h).With("component", "test").WithGroup("g")

	logger.Info("Zod bug: still stuck", WizLevel(31), WizType(LogComplete))

	if len(rec.calls) != 1 {
		t.Fatalf("echo fired %d times through With/WithGroup, want 1", len(rec.calls))
	}
}

// startTestServers brings up the diagnostics listeners on ephemeral ports and
// returns the metrics base URL.
func startTestServers(t *testing.T, health *Health) (string, *Servers) {
	t.Helper()
	metrics := NewMetrics(100 * time.Millisecond)
	servers, err := Serve(ServerOptions{
		MetricsAddr: "127.0.0.1:0",
		Metrics:     metrics,
		Health:      health,
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("Serve: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = servers.Shutdown(ctx)
	})
	return "http://" + servers.Addr("metrics"), servers
}

// gatherHistogram pulls one histogram back out of the registry by name.
func gatherHistogram(m *Metrics, name string) (*dto.Histogram, error) {
	families, err := m.Registry.Gather()
	if err != nil {
		return nil, err
	}
	for _, f := range families {
		if f.GetName() != name {
			continue
		}
		if len(f.GetMetric()) == 0 {
			return nil, fmt.Errorf("%s has no metrics", name)
		}
		return f.GetMetric()[0].GetHistogram(), nil
	}
	return nil, fmt.Errorf("%s not found in registry", name)
}

func get(t *testing.T, url string) (int, string) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

func TestHealthAndReadiness(t *testing.T) {
	health := &Health{}
	base, _ := startTestServers(t, health)

	// Liveness does not depend on readiness: a booting server is alive.
	if code, _ := get(t, base+"/healthz"); code != http.StatusOK {
		t.Errorf("GET /healthz = %d, want 200", code)
	}
	if code, _ := get(t, base+"/readyz"); code != http.StatusServiceUnavailable {
		t.Errorf("GET /readyz before ready = %d, want 503", code)
	}

	health.SetReady(true)
	if code, _ := get(t, base+"/readyz"); code != http.StatusOK {
		t.Errorf("GET /readyz after SetReady = %d, want 200", code)
	}

	health.SetReady(false)
	if code, _ := get(t, base+"/readyz"); code != http.StatusServiceUnavailable {
		t.Errorf("GET /readyz after shutdown began = %d, want 503", code)
	}
}

func TestMetricsEndpoint(t *testing.T) {
	base, _ := startTestServers(t, &Health{})
	code, body := get(t, base+"/metrics")
	if code != http.StatusOK {
		t.Fatalf("GET /metrics = %d, want 200", code)
	}
	for _, want := range []string{"dlmud_pulse_duration_seconds", "go_goroutines"} {
		if !strings.Contains(body, want) {
			t.Errorf("/metrics missing %q", want)
		}
	}
}

func TestPulseBucketsBracketTheBudget(t *testing.T) {
	// The buckets are derived from the configured interval, so a pulse that
	// exactly meets its budget must land on a boundary rather than in the
	// overflow bucket. This is the metric's whole point.
	metrics := NewMetrics(100 * time.Millisecond)
	metrics.PulseDuration.Observe(0.1)

	got, err := gatherHistogram(metrics, "dlmud_pulse_duration_seconds")
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	var atBudget uint64
	for _, b := range got.GetBucket() {
		if b.GetUpperBound() == 0.1 {
			atBudget = b.GetCumulativeCount()
		}
	}
	if atBudget != 1 {
		t.Errorf("observation of exactly the budget landed outside the budget bucket (count %d)", atBudget)
	}
}

func TestServeSkipsDisabledListeners(t *testing.T) {
	servers, err := Serve(ServerOptions{Metrics: NewMetrics(time.Millisecond), Health: &Health{}})
	if err != nil {
		t.Fatalf("Serve with no addresses: %v", err)
	}
	if len(servers.servers) != 0 {
		t.Errorf("started %d listeners with no addresses configured, want 0", len(servers.servers))
	}
	if err := servers.Shutdown(context.Background()); err != nil {
		t.Errorf("Shutdown: %v", err)
	}
}

func TestServeReportsBindFailure(t *testing.T) {
	// A bad address must fail at startup, not silently in a goroutine.
	_, err := Serve(ServerOptions{
		MetricsAddr: "256.256.256.256:1",
		Metrics:     NewMetrics(time.Millisecond),
		Health:      &Health{},
	})
	if err == nil {
		t.Fatal("Serve with an unbindable address succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "256.256.256.256:1") {
		t.Errorf("error = %q, want it to name the address", err)
	}
}
