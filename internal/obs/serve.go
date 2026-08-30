// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package obs

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/pprof"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Health tracks whether the process is alive and whether it is ready to take
// players. Liveness is implicit: if the HTTP handler answers, the process is
// running. Readiness is set by the boot sequence once the world is loaded and
// the listeners are accepting.
type Health struct {
	ready atomic.Bool
}

// SetReady marks the server ready (or not) to accept players.
func (h *Health) SetReady(ready bool) { h.ready.Store(ready) }

// Ready reports whether the server is accepting players.
func (h *Health) Ready() bool { return h.ready.Load() }

// Metrics holds the process-wide Prometheus registry and the metrics that are
// meaningful before any game code exists. Game metrics register themselves
// against Registry as their packages are built.
type Metrics struct {
	Registry *prometheus.Registry

	// PulseDuration measures how long each game loop pulse takes. A MUD's
	// single most useful health signal is a pulse running longer than its
	// interval, because that is the point at which the world starts lagging.
	PulseDuration prometheus.Histogram

	// PulsesMissed counts pulses that went by with the game loop too busy
	// to take them.
	//
	// Separate from PulseDuration, and not derivable from it: a histogram
	// of how long the pulses that *ran* took says nothing about the ones
	// that did not happen at all. This is the number that says the world
	// is behind real time rather than merely working hard, and any value
	// above zero is worth looking at. See engine.tick and #321.
	PulsesMissed prometheus.Counter

	// BuildInfo is the conventional always-1 gauge carrying version labels.
	BuildInfo *prometheus.GaugeVec
}

// NewMetrics creates a registry with the Go runtime and process collectors
// plus the server's own baseline metrics.
func NewMetrics(pulseInterval time.Duration) *Metrics {
	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	// Bucket around the configured pulse interval rather than at fixed
	// wall-clock values: what matters is the ratio of pulse duration to the
	// budget, and the budget is configurable.
	budget := pulseInterval.Seconds()
	m := &Metrics{
		Registry: reg,
		PulseDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "dlmud",
			Name:      "pulse_duration_seconds",
			Help:      "Time taken by each game loop pulse.",
			Buckets: []float64{
				budget / 100, budget / 20, budget / 10, budget / 4,
				budget / 2, budget, budget * 2, budget * 10,
			},
		}),
		PulsesMissed: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "dlmud",
			Name:      "pulses_missed_total",
			Help:      "Pulses the game loop was too busy to take, so the world fell behind real time.",
		}),
		BuildInfo: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "dlmud",
			Name:      "build_info",
			Help:      "Build information; always 1, carried by its labels.",
		}, []string{"version", "commit", "go_version"}),
	}
	reg.MustRegister(m.PulseDuration, m.PulsesMissed, m.BuildInfo)
	return m
}

// ServerOptions configures the diagnostics HTTP server.
type ServerOptions struct {
	// MetricsAddr serves /metrics, /healthz and /readyz. Empty disables it.
	MetricsAddr string
	// DebugAddr serves /debug/pprof. Empty disables it. Never expose this.
	DebugAddr string

	Metrics *Metrics
	Health  *Health
	Logger  *slog.Logger
}

// Servers is the set of running diagnostic listeners, shut down together.
type Servers struct {
	servers []*http.Server
	// addrs holds the actually-bound address per server, which differs from
	// the configured one whenever port 0 was requested.
	addrs  map[string]string
	logger *slog.Logger
}

// Addr returns the bound address of a named listener ("metrics" or "debug"),
// or the empty string if it is not running. Useful when port 0 was requested.
func (s *Servers) Addr(name string) string { return s.addrs[name] }

// Serve starts the diagnostics listeners described by opts. Listeners with an
// empty address are skipped. It returns once the listeners are bound, so a
// bind failure surfaces at startup rather than in a background goroutine.
func Serve(opts ServerOptions) (*Servers, error) {
	s := &Servers{logger: opts.Logger, addrs: map[string]string{}}

	if opts.MetricsAddr != "" {
		mux := http.NewServeMux()
		mux.Handle("GET /metrics", promhttp.HandlerFor(opts.Metrics.Registry, promhttp.HandlerOpts{
			Registry: opts.Metrics.Registry,
		}))
		mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
			writePlain(w, http.StatusOK, "ok")
		})
		mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) {
			if opts.Health.Ready() {
				writePlain(w, http.StatusOK, "ready")
				return
			}
			writePlain(w, http.StatusServiceUnavailable, "not ready")
		})
		if err := s.start("metrics", opts.MetricsAddr, mux); err != nil {
			return nil, err
		}
	}

	if opts.DebugAddr != "" {
		mux := http.NewServeMux()
		mux.HandleFunc("/debug/pprof/", pprof.Index)
		mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
		mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
		mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
		if err := s.start("debug", opts.DebugAddr, mux); err != nil {
			_ = s.Shutdown(context.Background())
			return nil, err
		}
	}

	return s, nil
}

func (s *Servers) start(name, addr string, h http.Handler) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second,
	}
	ln, err := listen(addr)
	if err != nil {
		return err
	}
	s.servers = append(s.servers, srv)
	s.addrs[name] = ln.Addr().String()
	if s.logger != nil {
		s.logger.Info("diagnostics listener started", "name", name, "addr", ln.Addr().String())
	}
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) && s.logger != nil {
			s.logger.Error("diagnostics listener failed", "name", name, "error", err)
		}
	}()
	return nil
}

// Shutdown stops all diagnostic listeners.
func (s *Servers) Shutdown(ctx context.Context) error {
	var errs []error
	for _, srv := range s.servers {
		if err := srv.Shutdown(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func writePlain(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body + "\n"))
}
