// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

// Command dlmud is the Disgracelands server.
//
// This is the boot sequence and nothing else: configuration, logging, metrics
// and health, then the world, the player store, the engine and the listeners,
// then wait for a signal and shut down. Everything between "ready" and
// "shutting down" belongs to the packages it starts, and each phase of
// docs/design/go-port-plan.md adds another layer in there — the rules core
// (Phase 4 onwards) is not built yet, so a player can log in and move around a
// world with nothing in it.
package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"syscall"
	"time"

	"github.com/gerrowadat/disgracelands/internal/auth"
	"github.com/gerrowadat/disgracelands/internal/buildinfo"
	"github.com/gerrowadat/disgracelands/internal/config"
	"github.com/gerrowadat/disgracelands/internal/engine"
	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/obs"
	"github.com/gerrowadat/disgracelands/internal/persist/bans"
	"github.com/gerrowadat/disgracelands/internal/persist/boards"
	"github.com/gerrowadat/disgracelands/internal/persist/clock"
	"github.com/gerrowadat/disgracelands/internal/persist/dataversion"
	"github.com/gerrowadat/disgracelands/internal/persist/houses"
	"github.com/gerrowadat/disgracelands/internal/persist/mail"
	"github.com/gerrowadat/disgracelands/internal/persist/names"
	"github.com/gerrowadat/disgracelands/internal/persist/player"
	"github.com/gerrowadat/disgracelands/internal/persist/reports"
	"github.com/gerrowadat/disgracelands/internal/persist/world"
	"github.com/gerrowadat/disgracelands/internal/rng"
	"github.com/gerrowadat/disgracelands/internal/server"
	"github.com/gerrowadat/disgracelands/internal/signals"

	// Register the one format the server reads.
	//
	// The classic, ascii and binary decoders are deliberately *not* here.
	// They are not deleted from the tree — `dlctl` still reads every
	// archived format there ever was, and always will — but they are not
	// linked into the server binary at all, which is a stronger statement
	// than refusing them at startup would be: a legacy format is absent
	// from this program rather than merely rejected by it
	// (docs/design/yaml-only.md §3.2). There is a test for that
	// property, because it is the kind of thing a stray import undoes
	// silently.
	_ "github.com/gerrowadat/disgracelands/internal/persist/bans/yaml"
	_ "github.com/gerrowadat/disgracelands/internal/persist/boards/yaml"
	_ "github.com/gerrowadat/disgracelands/internal/persist/houses/yaml"
	_ "github.com/gerrowadat/disgracelands/internal/persist/mail/yaml"
	_ "github.com/gerrowadat/disgracelands/internal/persist/player/yaml"
	_ "github.com/gerrowadat/disgracelands/internal/persist/reports/yaml"
	_ "github.com/gerrowadat/disgracelands/internal/persist/world/yaml"
)

// tlsConfig builds the TLS settings for the telnets listener.
//
// The certificate is served through a server.CertReloader rather than a
// static tls.Config.Certificates, so a renewed --tls-cert/--tls-key is
// picked up without a restart (issue #147). ctx bounds the reloader's
// background poll; it stops when the server shuts down, the same as every
// other background loop main starts.
func tlsConfig(ctx context.Context, cfg *config.Config, logger *slog.Logger) (*tls.Config, error) {
	if cfg.TLSCert != "" {
		reloader, err := server.NewCertReloader(cfg.TLSCert, cfg.TLSKey, logger)
		if err != nil {
			return nil, err
		}
		go reloader.Run(ctx, cfg.TLSReloadInterval)
		// TLS 1.2 is the floor: anything older has no business carrying a
		// password in 2026, and no client anyone still uses needs it.
		return &tls.Config{GetCertificate: reloader.GetCertificate, MinVersion: tls.VersionTLS12}, nil
	}
	if cfg.TLSACMEDomain != "" {
		return nil, fmt.Errorf("ACME certificates are not implemented yet; use --tls-cert and --tls-key")
	}
	return nil, fmt.Errorf("the TLS listener needs --tls-cert and --tls-key")
}

// serviceName is this process's OpenTelemetry service.name, and the default
// for the `resource` block on every JSON log line. OTEL_SERVICE_NAME
// overrides it, which is how two servers sharing a log backend tell
// themselves apart.
const serviceName = "dlmud"

// shutdownTimeout bounds the graceful shutdown. The C server's autosave pulse
// was 60s, so an ungraceful stop could lose a minute of play; the whole point
// of handling SIGTERM is to save instead, and that needs room to finish.
const shutdownTimeout = 30 * time.Second

// What the process exits with, and the only three answers there are.
//
// This is the replacement for the way the C told its wrapper script what it
// wanted: do_shutdown touches .killscript or .fastboot on the way out
// (act.wizard.c:1082) and the autorun shell script reads them afterwards to
// decide whether to start the server again (autorun:143). There is no
// wrapper here — the container runtime restarts it — so the same
// distinction is carried by the exit code, which is the mechanism that
// runtime already has: under `restart: on-failure`, exitReboot comes back
// by itself and exitOK stays down. docs/operations.md says so to operators.
const (
	exitOK     = 0 // A clean stop: a signal, `shutdown`, `shutdown die`.
	exitFailed = 1 // Boot failure, or a fatal error while running.
	exitReboot = 2 // `shutdown reboot` / `shutdown now`: start me again.
)

// reloadGameTuning is what SIGHUP does: re-read the game-tuning file —
// <lib-dir>/config/game.yaml, or --config where one was given — and publish
// it, without a restart (go-port-plan.md §9.1).
//
// Three things it deliberately does not do. It does not fail: a file that
// will not parse, or parses and will not validate, is logged and the
// previous tuning kept, because a typo in a config file must never be able
// to stop a game that is already running. And it does not touch the world —
// game.SetTuning is an atomic publish, which is what makes this safe to
// call from the signal goroutine while the world goroutine is mid-pulse.
// Reloading world *data* is a different thing with different rules; see
// docs/design/signal-handling.md §4 for why no signal does it. And it does
// not reset a server that has no tuning file back to the defaults: a file
// that is not there means "nothing to reload", the same as it does at boot,
// rather than "revert whatever is running".
func reloadGameTuning(logger *slog.Logger, cfg *config.Config) {
	t, path, err := config.LoadGameTuningFor(cfg)
	if err != nil {
		logger.Error("SIGHUP: game tuning reload failed, keeping previous values",
			"file", cfg.GameConfigPath(), "error", err)
		return
	}
	if path == "" {
		logger.Warn("SIGHUP received but there is no game tuning file; nothing to reload",
			"file", cfg.GameConfigPath())
		return
	}
	game.SetTuning(t)
	logger.Info("SIGHUP: reloaded game tuning", "file", path)
}

func main() {
	code, err := run(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "dlmud: %v\n", err)
	}
	os.Exit(code)
}

func run(args []string) (int, error) {
	cfg, err := config.Load(args, os.LookupEnv, os.Stderr)
	if err != nil {
		// `--help` has already printed the usage the caller asked for,
		// and asking for it is not a failure.
		if errors.Is(err, flag.ErrHelp) {
			return exitOK, nil
		}
		return exitFailed, err
	}

	info := buildinfo.Get()
	if cfg.ShowVersion {
		fmt.Println(info)
		return exitOK, nil
	}

	logger, closer, err := obs.NewLogger(obs.LogOptions{
		File:      cfg.LogFile,
		Format:    cfg.LogFormat,
		Level:     cfg.LogLevel,
		AddSource: cfg.LogLevel <= -4, // debug
		// Only the JSON format uses this, and only for its `resource`
		// block; OTEL_SERVICE_NAME and OTEL_RESOURCE_ATTRIBUTES are read
		// here rather than being given flags of their own, so a deployment
		// labels these logs the same way it labels everything else.
		Resource: obs.DetectResource(os.LookupEnv, serviceName, info.Version),
	})
	if err != nil {
		return exitFailed, err
	}
	if closer != nil {
		defer func() { _ = closer.Close() }()
	}

	logger.Info("starting",
		"version", info.Version,
		"commit", info.ShortCommit(),
		"go", info.GoVersion,
		"lib_dir", cfg.LibDir,
	)
	for _, w := range cfg.Warnings() {
		logger.Warn(w)
	}

	// A legacy lib/ is refused before anything is opened, with the
	// command to convert it — rather than getting three subsystems in and
	// failing on a missing file (docs/design/yaml-only.md §3.3).
	if err := config.CheckNotLegacy(cfg.LibDir); err != nil {
		return exitFailed, err
	}

	// The yaml data format's own version stamp (docs/design/
	// data-format-versioning.md), checked once here rather than per
	// subsystem: cfg.LibDir is the one directory every format-specific
	// path (WorldPath, PlayerPath, the state/config/text subdirectories)
	// is derived from, so one file at its root versions all of them
	// together. The stamp names the release of dlctl that wrote the
	// directory; this refuses to start unless that release shares our
	// major version, and warns if it merely differs in minor. A directory
	// with no stamp — everything predating this mechanism, everything an
	// unreleased dlctl wrote, and every classic/ascii/binary-only one —
	// checks out silently; see dataversion.CheckBuild.
	if warning, err := dataversion.CheckBuild(cfg.LibDir); err != nil {
		return exitFailed, err
	} else if warning != "" {
		logger.Warn(warning)
	}

	// config.c's runtime-tunable values (game.GameTuning): the data
	// directory's own config/game.yaml (or --config's file) overlaid on the
	// archive's own defaults. Set before anything below can read it — SIGHUP
	// reloads this same path later, once the signal handling below exists to
	// carry it. No file at all is not a failure; see LoadGameTuningFor.
	tuning, tuningPath, err := config.LoadGameTuningFor(cfg)
	if err != nil {
		return exitFailed, fmt.Errorf("game tuning: %w", err)
	}
	game.SetTuning(tuning)
	if tuningPath != "" {
		logger.Info("loaded game tuning", "file", tuningPath)
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
		return exitFailed, err
	}

	// One context for the whole server, cancelled either by a signal or by
	// a god typing `shutdown`. Both run the same shutdown for the same
	// reason: one that skipped the saves would be worse than none.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Signal handling before the world loads, so a SIGTERM arriving during
	// a slow boot is honoured rather than killing the process outright.
	// Everything about what each signal does and why lives in
	// docs/design/signal-handling.md; internal/signals is the whole
	// disposition, and anything absent from this list keeps its default —
	// SIGQUIT deliberately so.
	sigs := signals.Install(logger,
		signals.Handler{Signal: syscall.SIGTERM, Does: "graceful shutdown", Run: cancel},
		signals.Handler{Signal: syscall.SIGINT, Does: "graceful shutdown", Run: cancel},
		signals.Handler{Signal: syscall.SIGHUP, Does: "reload the configuration",
			Run: func() { reloadGameTuning(logger, cfg) }},
	)
	defer sigs.Stop()

	// One directory for every piece of runtime state: the clock, the
	// boards, the mail, the bans, the houses and the reports. Under the
	// classic layout these were spread across etc/, house/ and misc/ and
	// each one derived its own path; there is one now.
	statePath := config.Dir(cfg.LibDir, config.SubsystemState)

	// Load the world before anything can connect to it. A world that will
	// not load is a boot failure, not something to serve around.
	logger.Info("loading the world", "dir", cfg.WorldPath())
	worldSrc, err := world.Open(server.DataFormat, world.Config{Dir: cfg.WorldPath(), Mini: cfg.MiniMUD})
	if err != nil {
		return exitFailed, err
	}
	defer func() { _ = worldSrc.Close() }()

	defs, err := worldSrc.Load(ctx)
	if err != nil {
		return exitFailed, err
	}
	live := game.NewLive(defs)

	// The mud clock's persisted epoch, from the state/ directory the rest
	// of the state shares (docs/design/data-format.md §9). Applied before
	// anyone can see the clock — BootReset and every command after it read
	// MudTime().
	epoch, err := clock.Load(server.DataFormat, statePath)
	if err != nil {
		return exitFailed, err
	}
	live.SetBooted(epoch)

	logger.Info("world loaded",
		"rooms", len(defs.Rooms), "mobiles", len(defs.Mobiles),
		"objects", len(defs.Objects), "zones", len(defs.Zones), "shops", len(defs.Shops))

	players, err := player.Open(server.DataFormat, player.Config{Dir: cfg.PlayerPath()})
	if err != nil {
		return exitFailed, err
	}
	defer func() { _ = players.Close() }()

	// The rent files come from the same store as the roster, because yaml
	// keeps them in the same *file* (docs/design/data-format.md §8, "one
	// player, one file").
	//
	// This used to be a type assertion with a fallback to
	// binary.NewObjectStore for a roster format that was not also an
	// ObjectStore — which meant a server running on ascii wrote its rent
	// files as 2001 struct dumps, and is why real container nesting had to
	// be format-gated as a deviation rather than simply implemented
	// (docs/design/yaml-only.md §1). There is one roster format now, it
	// implements both, and the fallback (and the only reason this command
	// imported the binary package at all) is gone. Still an assertion
	// rather than a wider interface: player.Store and player.ObjectStore
	// are a reasonable split independent of formats — a corpse-only store,
	// a read-only roster — and collapsing them would be a different
	// decision than this one.
	objects, ok := players.(player.ObjectStore)
	if !ok {
		return exitFailed, fmt.Errorf("the %s player store does not implement player.ObjectStore, "+
			"so there is nowhere to keep rent files", players.Name())
	}

	// The bulletin boards: state/boards.yaml.
	boardStore, err := boards.Open(server.DataFormat, boards.Config{Dir: statePath})
	if err != nil {
		return exitFailed, err
	}

	// The mud mail: state/mail.yaml.
	mailStore, err := mail.Open(server.DataFormat, mail.Config{Path: statePath})
	if err != nil {
		return exitFailed, err
	}

	// The houses: state/houses.yaml, control records and contents in one
	// file where classic had hcontrol plus a directory of per-room files.
	houseStore, err := houses.Open(server.DataFormat, houses.Config{ObjectDir: statePath})
	if err != nil {
		return exitFailed, err
	}

	// The site ban list: state/bans.yaml.
	banStore, err := bans.Open(server.DataFormat, bans.Config{Path: statePath})
	if err != nil {
		return exitFailed, err
	}

	// The bug/idea/typo log: state/reports.yaml.
	reportStore, err := reports.Open(server.DataFormat, reports.Config{Dir: statePath})
	if err != nil {
		return exitFailed, err
	}

	// The disallowed-name list: config/names.yaml. Missing is not an error
	// — see names.Load's doc comment — so a server with no list disallows
	// nothing, matching Valid_Name's own posture.
	disallowedNames, err := names.Load(server.DataFormat, config.Dir(cfg.LibDir, config.SubsystemNames))
	if err != nil {
		return exitFailed, err
	}

	// The greeting and the credits are licence obligations; LoadText refuses
	// to return if either is missing, which is deliberate.
	text, err := server.LoadText(cfg.LibDir)
	if err != nil {
		return exitFailed, err
	}

	eng := engine.New(engine.Options{
		World: live, Interval: cfg.PulseInterval,
		Logger: logger, Metrics: metrics,
	})

	// The generator the game rolls on. A seed of zero means the clock, which
	// is what the C server does (comm.c:406); anything else makes the run
	// reproducible, which is what comparing against the C server requires.
	seed := cfg.RNGSeed
	if seed == 0 {
		seed = uint64(time.Now().UnixNano()) //nolint:gosec // a game seed, not a secret
	}
	source, err := rng.New(cfg.RNG, seed)
	if err != nil {
		return exitFailed, err
	}
	logger.Info("random number generator", "source", source.Name(),
		"seed", seed, "reproducible", cfg.RNGSeed != 0)

	srv := server.New(server.Options{
		Engine:        eng,
		Players:       players,
		Objects:       objects,
		Boards:        boardStore,
		Mail:          mailStore,
		Houses:        houseStore,
		Bans:          banStore,
		Reports:       reportStore,
		Names:         disallowedNames,
		ClockPath:     statePath,
		WorldDir:      cfg.WorldPath(),
		WorldMini:     cfg.MiniMUD,
		Auth:          auth.Verifier{AllowLegacy: cfg.AllowLegacyPasswords},
		Text:          text,
		Logger:        logger,
		Restrict:      cfg.Restrict,
		NoSpecials:    cfg.NoSpecials,
		FreezeMobiles: cfg.FreezeMobiles,
		FreezeWeather: cfg.FreezeWeather,
		// One command per pulse, which is what game_loop allows
		// (session.Session.pace). The same interval the engine ticks on,
		// because in the C they are the same loop.
		CommandInterval: cfg.PulseInterval,
		RNG:             rng.NewRand(source),
	})
	// The engine's periodic work belongs to the server, which is the side
	// that can reach both the world and the player store. Started only now
	// that the server exists to supply it.
	// Populate the world before anyone can connect: without a reset it is
	// 2,981 rooms and nothing else. Called directly rather than through the
	// engine because the engine is not running yet — DoSync would wait for a
	// goroutine that has not been started, which is a deadlock and was one.
	// Nothing else can touch the world at this point, so this is safe.
	srv.BootReset(live)

	// update_obj_file (objsave.c:332), skipped by -q there and
	// --skip-rent-check here (db.c:456's own `if (!no_rent_check)`).
	if !cfg.SkipRentCheck {
		srv.SweepRentFiles(ctx)
	}

	eng.SetPeriodic(srv.Periodic())

	// The world goroutine gets a context of its own, deliberately not
	// derived from ctx, because it has to outlive the thing that cancels
	// ctx. Everything on the way down -- Crash_save_all, the changed
	// houses, save_mud_time -- reaches the world through engine.DoSync,
	// and DoSync is a handshake with a running loop: hand the task over,
	// wait for it to run. Cancel the loop first and there is nobody left
	// to take the task, so every one of those saves sits in its select
	// until the shutdown deadline expires and then returns "context
	// deadline exceeded". That is not a slow shutdown, it is a silent one
	// -- every character still logged in loses whatever they did since the
	// last autosave, the mud clock stops advancing across restarts, and
	// the only trace is one ERROR line at the very end. Found by
	// test/play's TestShutdownSavesEveryoneStillInTheWorld, because
	// nothing that stops short of running the real binary and sending it a
	// real SIGTERM can see it.
	worldCtx, stopWorld := context.WithCancel(context.Background())
	defer stopWorld()
	go eng.Run(worldCtx)

	go srv.RunAutosave(ctx)
	go srv.RunClockSave(ctx)

	limits := server.Limits{
		MaxPlayers: cfg.MaxPlayers,
		MaxPerHost: cfg.MaxConnsPerIP,
		LoginGrace: cfg.LoginGraceTime,
	}

	var listeners []*server.Listener
	if cfg.TelnetAddr != "" {
		ln, err := server.ListenTelnet(cfg.TelnetAddr)
		if err != nil {
			return exitFailed, err
		}
		listeners = append(listeners, ln)
	}
	if cfg.TelnetsAddr != "" {
		tlsCfg, err := tlsConfig(ctx, cfg, logger)
		if err != nil {
			return exitFailed, err
		}
		ln, err := server.ListenTLS(cfg.TelnetsAddr, tlsCfg)
		if err != nil {
			return exitFailed, err
		}
		listeners = append(listeners, ln)
	}
	// The web interface: a welcome page, a browser terminal at /play, and
	// the WebSocket upgrade at /ws that terminal speaks over. It is an
	// ordinary net/http.Server rather than a *server.Listener, because it
	// serves plain HTTP pages as well as upgrading connections — see
	// internal/server/web.go's own doc comment for how /ws still ends up
	// going through the exact same Server.serve every telnet connection
	// does.
	var webServer *http.Server
	if cfg.WSAddr != "" {
		handler, err := srv.WebHandler(ctx, server.WebOptions{
			Password:          cfg.WebPassword,
			Captcha:           cfg.WebCaptcha,
			TrustProxyHeaders: cfg.TrustProxyHeaders,
			Limits:            limits,
		})
		if err != nil {
			return exitFailed, err
		}
		ln, err := net.Listen("tcp", cfg.WSAddr)
		if err != nil {
			return exitFailed, fmt.Errorf("listening on %s: %w", cfg.WSAddr, err)
		}
		webServer = &http.Server{
			Addr:              cfg.WSAddr,
			Handler:           handler,
			ReadHeaderTimeout: 5 * time.Second,
		}
		if cfg.TLSCert != "" || cfg.TLSACMEDomain != "" {
			tlsCfg, err := tlsConfig(ctx, cfg, logger)
			if err != nil {
				return exitFailed, err
			}
			webServer.TLSConfig = tlsCfg
			logger.Info("listening", "transport", "web", "addr", ln.Addr().String(), "tls", true)
			go func() {
				// ServeTLS with empty cert/key files is correct once
				// TLSConfig already supplies certificates via
				// GetCertificate, which tlsConfig always sets up.
				if err := webServer.ServeTLS(ln, "", ""); err != nil && !errors.Is(err, http.ErrServerClosed) {
					logger.Error("listener failed", "transport", "web", "error", err)
				}
			}()
		} else {
			logger.Info("listening", "transport", "web", "addr", ln.Addr().String())
			go func() {
				if err := webServer.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
					logger.Error("listener failed", "transport", "web", "error", err)
				}
			}()
		}
	}

	if len(listeners) == 0 && webServer == nil {
		return exitFailed, fmt.Errorf("no listeners could be started")
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

	// Either a signal or a god typing `shutdown`. The second runs exactly the
	// same path as the first — the saves, the crash-saves, the waiting for
	// writes — because a shutdown that skipped those would be worse than no
	// shutdown command at all.
	select {
	case <-ctx.Done():
	case <-srv.ShutdownRequested():
		logger.Info("shutdown requested from inside the game",
			"reboot", srv.RebootWanted())
		cancel()
	}

	// Stop treating further signals specially: a second Ctrl-C should kill a
	// wedged shutdown rather than being swallowed.
	sigs.Stop()
	logger.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	health.SetReady(false)

	// Crash_save_all, as the C does on its way down (comm.c:428). Before the
	// diagnostics go, because it needs the world goroutine still turning.
	srv.SaveEverything(shutdownCtx)

	// And wait for the saves that were already in flight. A process that
	// exits with one outstanding loses it, and the whole point of pushing
	// them off the world goroutine is that nothing waits for them *during*
	// play.
	srv.WaitForWrites()

	// The web listener, same as the telnet ones: every session behind it
	// already got ctx's own cancellation (Server.serve's own ctx.Done()
	// watcher, not this call), so this is a bounded wait for those to
	// actually finish closing rather than what does the closing. It has
	// to run before stopWorld — a web session still mid-command needs the
	// world goroutine turning to finish it, the same reason nothing below
	// this line may touch the world either.
	if webServer != nil {
		if err := webServer.Shutdown(shutdownCtx); err != nil {
			logger.Error("web listener shutdown failed", "error", err)
		}
	}

	// Only now is the world finished with. Everything above this line
	// needed it turning; nothing below it does.
	stopWorld()

	if err := diag.Shutdown(shutdownCtx); err != nil {
		logger.Error("diagnostics shutdown failed", "error", err)
	}

	logger.Info("stopped")

	// The only place the exit code is anything but exitOK on a shutdown
	// that worked: `shutdown reboot` and `shutdown now` ask to come back,
	// and this is how they say so. See the exit code constants.
	if srv.RebootWanted() {
		return exitReboot, nil
	}
	return exitOK, nil
}
