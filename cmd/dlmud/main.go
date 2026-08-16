// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

// Command dlmud is the Disgracelands server.
//
// Phase 0 of docs/proposals/go-port-plan.md: this wires up configuration, logging,
// metrics, health and signal handling. There is no game in it yet — it boots,
// reports itself ready, serves diagnostics, and shuts down cleanly. Each later
// phase fills in a layer between "ready" and "shutting down".
package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gerrowadat/disgracelands/internal/auth"
	"github.com/gerrowadat/disgracelands/internal/buildinfo"
	"github.com/gerrowadat/disgracelands/internal/config"
	"github.com/gerrowadat/disgracelands/internal/engine"
	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/obs"
	"github.com/gerrowadat/disgracelands/internal/persist/player"
	"github.com/gerrowadat/disgracelands/internal/persist/world"
	"github.com/gerrowadat/disgracelands/internal/server"

	// Register the formats the server can be configured to use.
	_ "github.com/gerrowadat/disgracelands/internal/persist/player/ascii"
	_ "github.com/gerrowadat/disgracelands/internal/persist/world/classic"
)

// tlsConfig builds the TLS settings for the telnets listener.
func tlsConfig(cfg *config.Config) (*tls.Config, error) {
	if cfg.TLSCert != "" {
		cert, err := tls.LoadX509KeyPair(cfg.TLSCert, cfg.TLSKey)
		if err != nil {
			return nil, fmt.Errorf("loading the TLS certificate: %w", err)
		}
		// TLS 1.2 is the floor: anything older has no business carrying a
		// password in 2026, and no client anyone still uses needs it.
		return &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}, nil
	}
	if cfg.TLSACMEDomain != "" {
		return nil, fmt.Errorf("ACME certificates are not implemented yet; use --tls-cert and --tls-key")
	}
	return nil, fmt.Errorf("the TLS listener needs --tls-cert and --tls-key")
}

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

	// Load the world before anything can connect to it. A world that will
	// not load is a boot failure, not something to serve around.
	logger.Info("loading the world", "dir", cfg.WorldPath(), "format", cfg.WorldFormat)
	worldSrc, err := world.Open(cfg.WorldFormat, world.Config{Dir: cfg.WorldPath(), Mini: cfg.MiniMUD})
	if err != nil {
		return err
	}
	defer func() { _ = worldSrc.Close() }()

	defs, err := worldSrc.Load(ctx)
	if err != nil {
		return err
	}
	live := game.NewLive(defs)
	logger.Info("world loaded",
		"rooms", len(defs.Rooms), "mobiles", len(defs.Mobiles),
		"objects", len(defs.Objects), "zones", len(defs.Zones), "shops", len(defs.Shops))

	players, err := player.Open(cfg.PlayerFormat, player.Config{Dir: cfg.PlayerPath()})
	if err != nil {
		return err
	}
	defer func() { _ = players.Close() }()

	// The greeting and the credits are licence obligations; LoadText refuses
	// to return if either is missing, which is deliberate.
	text, err := server.LoadText(cfg.LibDir)
	if err != nil {
		return err
	}

	eng := engine.New(engine.Options{
		World: live, Interval: cfg.PulseInterval,
		Logger: logger, Metrics: metrics,
	})
	go eng.Run(ctx)

	srv := server.New(server.Options{
		Engine:   eng,
		Players:  players,
		Auth:     auth.Verifier{AllowLegacy: cfg.AllowLegacyPasswords},
		Text:     text,
		Logger:   logger,
		Restrict: cfg.Restrict,
	})
	go srv.RunAutosave(ctx)

	limits := server.Limits{
		MaxPerHost: cfg.MaxConnsPerIP,
		LoginGrace: cfg.LoginGraceTime,
	}

	var listeners []*server.Listener
	if cfg.TelnetAddr != "" {
		ln, err := server.ListenTelnet(cfg.TelnetAddr)
		if err != nil {
			return err
		}
		listeners = append(listeners, ln)
	}
	if cfg.TelnetsAddr != "" {
		tlsCfg, err := tlsConfig(cfg)
		if err != nil {
			return err
		}
		ln, err := server.ListenTLS(cfg.TelnetsAddr, tlsCfg)
		if err != nil {
			return err
		}
		listeners = append(listeners, ln)
	}
	if len(listeners) == 0 {
		return fmt.Errorf("no listeners could be started")
	}

	for _, ln := range listeners {
		logger.Info("listening", "transport", ln.Name, "addr", ln.Addr().String())
		go func(ln *server.Listener) {
			if err := srv.Accept(ctx, ln, limits); err != nil {
				logger.Error("listener failed", "transport", ln.Name, "error", err)
			}
		}(ln)
	}

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
