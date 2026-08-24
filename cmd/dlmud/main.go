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
// docs/proposals/go-port-plan.md adds another layer in there — the rules core
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
	"os"
	"path/filepath"
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
	"github.com/gerrowadat/disgracelands/internal/persist/player/binary"
	"github.com/gerrowadat/disgracelands/internal/persist/reports"
	"github.com/gerrowadat/disgracelands/internal/persist/world"
	"github.com/gerrowadat/disgracelands/internal/rng"
	"github.com/gerrowadat/disgracelands/internal/server"
	"github.com/gerrowadat/disgracelands/internal/signals"

	// Register the formats the server can be configured to use.
	_ "github.com/gerrowadat/disgracelands/internal/persist/bans/classic"
	_ "github.com/gerrowadat/disgracelands/internal/persist/bans/yaml"
	_ "github.com/gerrowadat/disgracelands/internal/persist/boards/classic"
	_ "github.com/gerrowadat/disgracelands/internal/persist/boards/yaml"
	_ "github.com/gerrowadat/disgracelands/internal/persist/houses/classic"
	_ "github.com/gerrowadat/disgracelands/internal/persist/houses/yaml"
	_ "github.com/gerrowadat/disgracelands/internal/persist/mail/classic"
	_ "github.com/gerrowadat/disgracelands/internal/persist/mail/yaml"
	_ "github.com/gerrowadat/disgracelands/internal/persist/player/ascii"
	_ "github.com/gerrowadat/disgracelands/internal/persist/player/yaml"
	_ "github.com/gerrowadat/disgracelands/internal/persist/reports/classic"
	_ "github.com/gerrowadat/disgracelands/internal/persist/reports/yaml"
	_ "github.com/gerrowadat/disgracelands/internal/persist/world/classic"
	_ "github.com/gerrowadat/disgracelands/internal/persist/world/yaml"
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

// reloadGameTuning is what SIGHUP does: re-read --config's game-tuning file
// and publish it, without a restart (go-port-plan.md §9.1).
//
// Two things it deliberately does not do. It does not fail: a file that
// will not parse, or parses and will not validate, is logged and the
// previous tuning kept, because a typo in a config file must never be able
// to stop a game that is already running. And it does not touch the world —
// game.SetTuning is an atomic publish, which is what makes this safe to
// call from the signal goroutine while the world goroutine is mid-pulse.
// Reloading world *data* is a different thing with different rules; see
// docs/design/signal-handling.md §4 for why no signal does it.
func reloadGameTuning(logger *slog.Logger, cfg *config.Config) {
	if cfg.GameConfigFile == "" {
		logger.Warn("SIGHUP received but no --config is set; nothing to reload")
		return
	}
	t, err := config.LoadGameTuning(cfg.GameConfigFile)
	if err != nil {
		logger.Error("SIGHUP: game tuning reload failed, keeping previous values",
			"file", cfg.GameConfigFile, "error", err)
		return
	}
	game.SetTuning(t)
	logger.Info("SIGHUP: reloaded game tuning", "file", cfg.GameConfigFile)
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
		"player_format", cfg.PlayerFormat,
		"world_format", cfg.WorldFormat,
	)
	for _, w := range cfg.Warnings() {
		logger.Warn(w)
	}

	// The yaml data format's own version stamp (docs/design/
	// data-format-versioning.md), checked once here rather than per
	// subsystem: cfg.LibDir is the one directory every format-specific
	// path (WorldPath, PlayerPath, the state/config/text subdirectories)
	// is derived from, so one file at its root versions all of them
	// together. A directory with no stamp — everything predating this
	// mechanism, and every classic/ascii/binary-only one — checks out
	// silently; see dataversion.Check's own doc comment for what the
	// other two outcomes mean.
	if warning, err := dataversion.Check(cfg.LibDir, dataversion.Current); err != nil {
		return exitFailed, err
	} else if warning != "" {
		logger.Warn(warning)
	}

	// config.c's runtime-tunable values (game.GameTuning): --config's file
	// overlaid on the archive's own defaults. Set before anything below can
	// read it — SIGHUP reloads this same path later, once the signal
	// handling below exists to carry it.
	tuning, err := config.LoadGameTuning(cfg.GameConfigFile)
	if err != nil {
		return exitFailed, fmt.Errorf("game tuning: %w", err)
	}
	game.SetTuning(tuning)
	if cfg.GameConfigFile != "" {
		logger.Info("loaded game tuning", "file", cfg.GameConfigFile)
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

	// Load the world before anything can connect to it. A world that will
	// not load is a boot failure, not something to serve around.
	logger.Info("loading the world", "dir", cfg.WorldPath(), "format", cfg.WorldFormat)
	worldSrc, err := world.Open(cfg.WorldFormat, world.Config{Dir: cfg.WorldPath(), Mini: cfg.MiniMUD})
	if err != nil {
		return exitFailed, err
	}
	defer func() { _ = worldSrc.Close() }()

	defs, err := worldSrc.Load(ctx)
	if err != nil {
		return exitFailed, err
	}
	live := game.NewLive(defs)

	// The mud clock's persisted epoch: etc/time under classic, the same
	// state/clock.yaml directory the other four state stores share under
	// yaml (docs/design/data-format.md §9). Applied before anyone can
	// see the clock — BootReset and every command after it read MudTime().
	clockPath := filepath.Join(cfg.LibDir, "etc", "time")
	if cfg.StateFormat == "yaml" {
		clockPath = filepath.Join(cfg.LibDir, "state")
	}
	epoch, err := clock.Load(cfg.StateFormat, clockPath)
	if err != nil {
		return exitFailed, err
	}
	live.SetBooted(epoch)

	logger.Info("world loaded",
		"rooms", len(defs.Rooms), "mobiles", len(defs.Mobiles),
		"objects", len(defs.Objects), "zones", len(defs.Zones), "shops", len(defs.Shops))

	players, err := player.Open(cfg.PlayerFormat, player.Config{Dir: cfg.PlayerPath()})
	if err != nil {
		return exitFailed, err
	}
	defer func() { _ = players.Close() }()

	// The rent files are not pluggable the way the roster is, with one
	// exception: yaml folds them into the same file as the roster
	// (docs/design/data-format.md §8, "one player, one file"), so a
	// Store that is also an ObjectStore serves both — there is no separate
	// plrobjs/ to point a second store at. Every other format keeps them in
	// `plrobjs/` in the C's own file format, whatever the roster is kept as,
	// since the C has one format for them and ascii's own roster format is
	// this port's own addition.
	//
	// Under the player directory, not beside it: a served lib/ keeps a
	// roster and its rent files together, which is this port's layout and
	// not the C's. The C builds `etc/players` and `plrobjs/` from its own
	// cwd, so an *archived* tree has them as siblings — that is a
	// conversion-time concern (player.Config.ObjectsDir, and `dlctl pfile
	// import --from-objs-dir`), not one for a directory this server wrote.
	objects, ok := players.(player.ObjectStore)
	if !ok {
		objects, err = binary.NewObjectStore(player.Config{Dir: cfg.PlayerPath()})
		if err != nil {
			return exitFailed, err
		}
	}

	// The bulletin boards: beside the player data in the etc directory under
	// classic, or state/boards.yaml under yaml.
	boardDir := filepath.Join(cfg.LibDir, "etc")
	if cfg.StateFormat == "yaml" {
		boardDir = filepath.Join(cfg.LibDir, "state")
	}
	boardStore, err := boards.Open(cfg.StateFormat, boards.Config{Dir: boardDir})
	if err != nil {
		return exitFailed, err
	}

	// The mud mail file: classic's block-allocator file, or
	// state/mail.yaml under yaml.
	mailPath := filepath.Join(cfg.LibDir, "etc", "plrmail")
	if cfg.StateFormat == "yaml" {
		mailPath = filepath.Join(cfg.LibDir, "state")
	}
	mailStore, err := mail.Open(cfg.StateFormat, mail.Config{Path: mailPath})
	if err != nil {
		return exitFailed, err
	}

	// The house control file and the per-house object files: classic's two
	// separate paths, or state/houses.yaml (everything folded in) under
	// yaml.
	houseCfg := houses.Config{
		ControlPath: filepath.Join(cfg.LibDir, "etc", "hcontrol"),
		ObjectDir:   filepath.Join(cfg.LibDir, "house"),
	}
	if cfg.StateFormat == "yaml" {
		houseCfg = houses.Config{ObjectDir: filepath.Join(cfg.LibDir, "state")}
	}
	houseStore, err := houses.Open(cfg.StateFormat, houseCfg)
	if err != nil {
		return exitFailed, err
	}

	// The site ban list — the one archive file that is plain text, under
	// classic; a state/bans.yaml file under yaml.
	banPath := filepath.Join(cfg.LibDir, "etc", "badsites")
	if cfg.StateFormat == "yaml" {
		banPath = filepath.Join(cfg.LibDir, "state")
	}
	banStore, err := bans.Open(cfg.StateFormat, bans.Config{Path: banPath})
	if err != nil {
		return exitFailed, err
	}

	// The bug/idea/typo log: misc/{bugs,ideas,typos} under classic, or the
	// same state/ directory the other four state stores share under
	// yaml.
	reportsDir := filepath.Join(cfg.LibDir, "misc")
	if cfg.StateFormat == "yaml" {
		reportsDir = filepath.Join(cfg.LibDir, "state")
	}
	reportStore, err := reports.Open(cfg.StateFormat, reports.Config{Dir: reportsDir})
	if err != nil {
		return exitFailed, err
	}

	// The disallowed-name list: misc/xnames under classic, or
	// config/names.yaml under yaml. Missing is not an error — see
	// names.Load's doc comment — so a server with no list disallows
	// nothing, matching Valid_Name's own posture.
	namesPath := filepath.Join(cfg.LibDir, "misc", "xnames")
	if cfg.NamesFormat == "yaml" {
		namesPath = filepath.Join(cfg.LibDir, "config")
	}
	disallowedNames, err := names.Load(cfg.NamesFormat, namesPath)
	if err != nil {
		return exitFailed, err
	}

	// The greeting and the credits are licence obligations; LoadText refuses
	// to return if either is missing, which is deliberate.
	text, err := server.LoadText(cfg.LibDir, cfg.MessagesFormat, cfg.SocialsFormat, cfg.HelpFormat)
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
		Engine:      eng,
		Players:     players,
		Objects:     objects,
		Boards:      boardStore,
		Mail:        mailStore,
		Houses:      houseStore,
		Bans:        banStore,
		Reports:     reportStore,
		Names:       disallowedNames,
		ClockFormat: cfg.StateFormat,
		ClockPath:   clockPath,
		WorldFormat: cfg.WorldFormat,
		WorldDir:    cfg.WorldPath(),
		WorldMini:   cfg.MiniMUD,
		Auth:        auth.Verifier{AllowLegacy: cfg.AllowLegacyPasswords},
		Text:        text,
		Logger:      logger,
		Restrict:    cfg.Restrict,
		NoSpecials:  cfg.NoSpecials,
		RNG:         rng.NewRand(source),
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
		tlsCfg, err := tlsConfig(cfg)
		if err != nil {
			return exitFailed, err
		}
		ln, err := server.ListenTLS(cfg.TelnetsAddr, tlsCfg)
		if err != nil {
			return exitFailed, err
		}
		listeners = append(listeners, ln)
	}
	if len(listeners) == 0 {
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
