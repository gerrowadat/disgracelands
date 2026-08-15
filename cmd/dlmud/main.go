// Command dlmud is the Disgracelands server.
//
// Phase 0 of docs/proposals/go-port-plan.md: this wires up configuration, logging,
// metrics, health and signal handling. There is no game in it yet — it boots,
// reports itself ready, serves diagnostics, and shuts down cleanly. Each later
// phase fills in a layer between "ready" and "shutting down".
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gerrowadat/disgracelands/internal/buildinfo"
	"github.com/gerrowadat/disgracelands/internal/config"
	"github.com/gerrowadat/disgracelands/internal/obs"
)

// shutdownTimeout bounds the graceful shutdown. The C server's autosave pulse
// was 60s, so an ungraceful stop could lose a minute of play; the whole point
// of handling SIGTERM is to save instead, and that needs room to finish.
const shutdownTimeout = 30 * time.Second

func main() {
	if err := run(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintf(os.Stderr, "dlmud: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	cfg, err := config.Load(args, os.LookupEnv, os.Stderr)
	if err != nil {
		return err
	}

	info := buildinfo.Get()
	if cfg.ShowVersion {
		fmt.Println(info)
		return nil
	}

	logger, closer, err := obs.NewLogger(obs.LogOptions{
		File:      cfg.LogFile,
		Format:    cfg.LogFormat,
		Level:     cfg.LogLevel,
		AddSource: cfg.LogLevel <= -4, // debug
	})
	if err != nil {
		return err
	}
	if closer != nil {
		defer func() { _ = closer.Close() }()
	}

	logger.Info("starting",
		"version", info.Version,
		"commit", info.ShortCommit(),
		"go", info.GoVersion,
		"lib_dir", cfg.LibDir,
		"player_format", cfg.PlayerFormat,
		"world_format", cfg.WorldFormat,
	)
	for _, w := range cfg.Warnings() {
		logger.Warn(w)
	}

	metrics := obs.NewMetrics(cfg.PulseInterval)
	metrics.BuildInfo.WithLabelValues(info.Version, info.ShortCommit(), info.GoVersion).Set(1)

	health := &obs.Health{}
	diag, err := obs.Serve(obs.ServerOptions{
		MetricsAddr: cfg.MetricsAddr,
		DebugAddr:   cfg.DebugAddr,
		Metrics:     metrics,
		Health:      health,
		Logger:      logger,
	})
	if err != nil {
		return err
	}

	// Signal handling first, so a SIGTERM arriving during a slow boot is
	// honoured rather than killing the process outright.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Phase 1+ inserts world loading here, Phase 2 player storage, and Phase 3
	// the listeners and pulse loop. Readiness flips once those are up.
	logger.Warn("no game engine yet: this is a Phase 0 foundations build, see docs/proposals/go-port-plan.md")
	health.SetReady(true)

	logger.Info("ready")
	<-ctx.Done()

	// Stop treating further signals specially: a second Ctrl-C should kill a
	// wedged shutdown rather than being swallowed.
	stop()
	logger.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	health.SetReady(false)
	if err := diag.Shutdown(shutdownCtx); err != nil {
		logger.Error("diagnostics shutdown failed", "error", err)
	}

	logger.Info("stopped")
	return nil
}
