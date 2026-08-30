// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

// Package config resolves the server's runtime settings.
//
// Precedence is flags > environment > defaults, per docs/proposals/go-port-plan.md §9.1.
// Every setting is declared exactly once, in [register]; the environment
// variable name is derived from the flag name rather than written out
// separately, so the two cannot drift apart.
//
// The config-file layer (§9.1) that sits between environment and defaults is
// the game tuning ported out of the C tree's src/config.c: see GameConfigPath
// and LoadGameTuningFor. It is deliberately not part of this precedence
// chain's own settings — it configures the game, not the deployment, so it
// lives in the data directory and is resolved from it.
package config

import (
	"flag"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/gerrowadat/disgracelands/internal/rng"
)

// EnvPrefix prefixes every environment variable this package reads.
const EnvPrefix = "DL_"

// Config is the fully resolved server configuration.
type Config struct {
	// Data locations.
	LibDir    string
	PlayerDir string // empty means "derive from LibDir"
	WorldDir  string // empty means "derive from LibDir"

	// GameConfigFile overrides where the game-tuning file is read from.
	// Empty — the usual case — means <lib-dir>/config/game.yaml, which is
	// optional: a data directory that has never been tuned has no such
	// file, and no file is config.c's own behaviour exactly. See
	// GameConfigPath, LoadGameTuningFor and cmd/dlmud's SIGHUP handling.
	GameConfigFile string

	// Listeners. An empty address means the listener is disabled.
	TelnetAddr  string
	TelnetsAddr string
	// WSAddr is the web interface's own listen address: a welcome page at
	// /, a browser terminal at /play, and the WebSocket upgrade at /ws
	// that page's terminal actually speaks over — the "no browser client
	// yet" gap docs/configuration.md used to describe. See internal/server/web.go.
	WSAddr string

	// WebPassword, if set, gates the entire web interface (every route,
	// not just /play) behind HTTP Basic Auth with this one shared
	// password — there is no per-account web login, deliberately: the web
	// interface is a door into the same telnet-style login the game has
	// always had, not a second identity system.
	WebPassword string
	// WebCaptcha requires solving a simple arithmetic challenge before a
	// browser may open /ws, so that "point a script at the web port"
	// costs slightly more than "point a script at the telnet port" —
	// see internal/server/web.go's own doc comment on what this does and
	// does not defend against.
	WebCaptcha bool

	// TLS.
	TLSCert         string
	TLSKey          string
	TLSACMEDomain   string
	TLSACMECacheDir string
	// TLSReloadInterval is how often --tls-cert/--tls-key are checked for
	// changes and, if either has a newer mtime than the certificate
	// currently serving connections, reloaded — so a renewed certificate
	// (cert-manager, certbot, an ops team's own cron) takes effect on the
	// next handshake instead of needing a restart to be picked up (#147).
	// Zero disables the check: the certificate loaded at boot is used for
	// the life of the process, which was this server's only behaviour
	// before this existed.
	TLSReloadInterval time.Duration

	// Connection handling.
	TrustProxyHeaders bool
	MaxPlayers        int
	MaxConnsPerIP     int
	LoginGraceTime    time.Duration

	// Engine.
	PulseInterval time.Duration

	// RNG names the generator the game rolls on: "modern" (Go's PCG) or
	// "circle" (the C server's own, ported exactly). See internal/rng.
	RNG string
	// RNGSeed seeds it. Zero means seed from the clock, which is what the C
	// does; anything else makes a run reproducible, which is what the parity
	// harness needs.
	RNGSeed uint64

	// Behaviour carried over from the C server's single-letter options.
	MiniMUD       bool
	SkipRentCheck bool
	Restrict      bool
	NoSpecials    bool

	// FreezeMobiles holds the mobiles still: no wandering, no scavenging,
	// no mobile-activity dice. It is not a C option and not a game
	// setting — it is the session-parity harness's lever, matching the
	// `-M` added to the C server for the same purpose
	// (reference/moderncserver/src/comm.c, `freeze_mobiles`). A mobile's
	// position depends on how many pulses have elapsed since boot, so two
	// servers booted seconds apart disagree about every room a mobile
	// walks through — which is most of Midgaard. See test/parity.
	FreezeMobiles bool

	// FreezeWeather holds the barometer still: no weather_change, and none
	// of its dice. Like FreezeMobiles it is not a C option and not a game
	// setting — it matches the `-W` added to the C server for the same
	// purpose. weather_change rolls five dice a mud hour and sometimes six,
	// and the parity harness plays its script at one server and then at the
	// other, so by the same line the second server has been up longer and
	// has had a tick the first had not. See test/parity.
	FreezeWeather bool

	// Security.
	AllowLegacyPasswords bool

	// Observability.
	LogFile     string
	LogFormat   string
	LogLevel    slog.Level
	MetricsAddr string
	DebugAddr   string

	// Set when -version was requested; the caller prints and exits.
	ShowVersion bool
}

// PlayerPath returns the player-data directory, defaulting to
// LibDir/players (Dir, SubsystemPlayers).
func (c *Config) PlayerPath() string {
	if c.PlayerDir != "" {
		return c.PlayerDir
	}
	return Dir(c.LibDir, SubsystemPlayers)
}

// WorldPath returns the world-data directory, defaulting to LibDir/world.
func (c *Config) WorldPath() string {
	if c.WorldDir != "" {
		return c.WorldDir
	}
	return Dir(c.LibDir, SubsystemWorld)
}

// GameConfigPath returns the game-tuning file to read: --config if one was
// given, otherwise LibDir/config/game.yaml.
//
// The tuning is game configuration rather than deployment configuration —
// whether rent is free is a property of this game, travels with the world,
// belongs in its backup and is worth reviewing alongside it, in the way a
// listen address or a certificate path is not (docs/design/data-format.md
// §6). So it lives in the data directory, next to config/names.yaml and the
// rest, rather than in a directory of its own beside the binary. --config
// stays for the deployment that genuinely wants it elsewhere.
func (c *Config) GameConfigPath() string {
	if c.GameConfigFile != "" {
		return c.GameConfigFile
	}
	return c.LibDir + "/config/game.yaml"
}

// knownLogFormats is validated at startup so a typo fails with a useful
// message rather than deep inside logging setup.
//
// There used to be seven more lists beside it, one per pluggable data
// format. The server reads exactly one on-disk format now
// (docs/proposals/yaml-only.md §0), so there is nothing to select and
// nothing to validate. `dlctl` still reads every archived format there
// ever was and keeps its own --from-format for saying which.
var knownLogFormats = []string{"text", "json"}

// Default returns the configuration used when nothing is specified. Every
// default reproduces the C server's behaviour where one exists, with the
// documented exception of TelnetAddr (see docs/proposals/go-port-plan.md §0: plaintext
// telnet is implemented but off unless asked for).
func Default() Config {
	return Config{
		LibDir:               "examples/stock/yaml",
		TelnetAddr:           "",
		TelnetsAddr:          ":4443",
		WSAddr:               "",
		WebPassword:          "",
		WebCaptcha:           false,
		TLSACMECacheDir:      "examples/stock/yaml/.acme",
		TLSReloadInterval:    time.Minute,
		MaxPlayers:           300,
		MaxConnsPerIP:        8,
		LoginGraceTime:       60 * time.Second,
		PulseInterval:        100 * time.Millisecond,
		RNG:                  rng.Modern,
		AllowLegacyPasswords: true,
		LogFile:              "-",
		LogFormat:            "text",
		LogLevel:             slog.LevelInfo,
		MetricsAddr:          "",
		DebugAddr:            "",
	}
}

// binding ties one setting to its flag name, so the environment fallback can
// find it after parsing and apply the same parsing the flag would have.
type binding struct {
	name   string
	assign func(string) error
}

// EnvName maps a flag name to its environment variable: --lib-dir → DL_LIB_DIR.
func EnvName(flagName string) string {
	return EnvPrefix + strings.ToUpper(strings.ReplaceAll(flagName, "-", "_"))
}

// Load resolves configuration from args (excluding argv[0]) and the
// environment. lookupEnv is usually os.LookupEnv; it is a parameter so tests
// can supply their own without mutating process state.
//
// It returns [flag.ErrHelp] if help was requested, having already written the
// usage message to out.
func Load(args []string, lookupEnv func(string) (string, bool), out io.Writer) (*Config, error) {
	cfg := Default()

	fs := flag.NewFlagSet("dlmud", flag.ContinueOnError)
	fs.SetOutput(out)

	var bindings []*binding
	// bind registers a flag and remembers how to set it from a string, so the
	// same parsing logic serves both the flag and the environment variable.
	bind := func(name, usage string, assign func(string) error, register func(*flag.FlagSet)) {
		register(fs)
		bindings = append(bindings, &binding{name: name, assign: assign})
		// Document the environment variable in the usage text itself; there is
		// no separate table to fall out of date.
		f := fs.Lookup(name)
		f.Usage = fmt.Sprintf("%s [%s]", usage, EnvName(name))
	}

	str := func(name, usage string, target *string) {
		bind(name, usage,
			func(v string) error { *target = v; return nil },
			func(fs *flag.FlagSet) { fs.StringVar(target, name, *target, usage) })
	}
	boolean := func(name, usage string, target *bool) {
		bind(name, usage,
			func(v string) error {
				b, err := strconv.ParseBool(v)
				if err != nil {
					return fmt.Errorf("not a boolean: %q", v)
				}
				*target = b
				return nil
			},
			func(fs *flag.FlagSet) { fs.BoolVar(target, name, *target, usage) })
	}
	integer := func(name, usage string, target *int) {
		bind(name, usage,
			func(v string) error {
				n, err := strconv.Atoi(v)
				if err != nil {
					return fmt.Errorf("not an integer: %q", v)
				}
				*target = n
				return nil
			},
			func(fs *flag.FlagSet) { fs.IntVar(target, name, *target, usage) })
	}
	unsigned := func(name, usage string, target *uint64) {
		bind(name, usage,
			func(v string) error {
				n, err := strconv.ParseUint(v, 10, 64)
				if err != nil {
					return fmt.Errorf("not a non-negative integer: %q", v)
				}
				*target = n
				return nil
			},
			func(fs *flag.FlagSet) { fs.Uint64Var(target, name, *target, usage) })
	}
	duration := func(name, usage string, target *time.Duration) {
		bind(name, usage,
			func(v string) error {
				d, err := time.ParseDuration(v)
				if err != nil {
					return fmt.Errorf("not a duration: %q", v)
				}
				*target = d
				return nil
			},
			func(fs *flag.FlagSet) { fs.DurationVar(target, name, *target, usage) })
	}

	// Log level needs its own binding because slog.Level is not a flag type.
	logLevel := cfg.LogLevel.String()
	str("log-level", "Log level: debug, info, warn, error", &logLevel)

	str("lib-dir", "Runtime data directory (world, text, player data)", &cfg.LibDir)
	str("player-dir", "Player-data directory (default: <lib-dir>/players)", &cfg.PlayerDir)
	str("world-dir", "World-data directory (default: <lib-dir>/world)", &cfg.WorldDir)
	str("config", "Game-tuning config file (default: <lib-dir>/config/game.yaml, and optional there)", &cfg.GameConfigFile)

	str("listen-telnet", "Plaintext telnet listen address (empty = disabled)", &cfg.TelnetAddr)
	str("listen-telnets", "TLS telnet listen address (empty = disabled)", &cfg.TelnetsAddr)
	str("listen-ws", "Web interface listen address: / and /play in a browser (empty = disabled)", &cfg.WSAddr)
	str("web-password", "Password required to use the web interface, on top of the game's own login (empty = none)", &cfg.WebPassword)
	boolean("web-captcha", "Require solving a simple captcha before playing over the web interface", &cfg.WebCaptcha)

	str("tls-cert", "TLS certificate file", &cfg.TLSCert)
	str("tls-key", "TLS private key file", &cfg.TLSKey)
	str("tls-acme-domain", "Obtain a TLS certificate via ACME for this domain", &cfg.TLSACMEDomain)
	str("tls-acme-cache", "Directory for ACME certificate cache", &cfg.TLSACMECacheDir)
	duration("tls-reload-interval", "How often to check --tls-cert/--tls-key for changes and reload (0 = never)",
		&cfg.TLSReloadInterval)

	boolean("trust-proxy-headers", "Trust X-Forwarded-For from a reverse proxy", &cfg.TrustProxyHeaders)
	integer("max-players", "Maximum simultaneous players", &cfg.MaxPlayers)
	integer("max-connections-per-ip", "Maximum simultaneous connections from one address", &cfg.MaxConnsPerIP)
	duration("login-grace-time", "Time a connection may stay unauthenticated", &cfg.LoginGraceTime)

	duration("pulse-interval", "Game loop pulse interval (C server used 100ms)", &cfg.PulseInterval)

	str("rng", "Random number generator the game rolls on: "+strings.Join(rng.Names, ", ")+
		" (circle is the C server's own, for behaviour identical to it)", &cfg.RNG)
	unsigned("rng-seed", "Seed for the generator (0 = from the clock, as the C server does)", &cfg.RNGSeed)

	boolean("mini-mud", "Load a minimal world for testing (C: -m)", &cfg.MiniMUD)
	boolean("skip-rent-check", "Skip the rent scan on boot (C: -q)", &cfg.SkipRentCheck)
	boolean("restrict", "Allow no new players (C: -r)", &cfg.Restrict)
	boolean("no-specials", "Suppress special procedure assignment (C: -s)", &cfg.NoSpecials)
	boolean("freeze-mobiles", "Hold the mobiles still (parity harness; C: -M)", &cfg.FreezeMobiles)
	boolean("freeze-weather", "Hold the weather still (parity harness; C: -W)", &cfg.FreezeWeather)

	boolean("allow-legacy-passwords", "Accept pre-2008 DES crypt(3) password hashes", &cfg.AllowLegacyPasswords)

	str("log-file", "Log destination, or - for stdout (C: -o)", &cfg.LogFile)
	str("log-format", "Log format: text for a terminal, json for OpenTelemetry-shaped records", &cfg.LogFormat)
	str("metrics-addr", "Prometheus/health listen address (empty = disabled)", &cfg.MetricsAddr)
	str("debug-addr", "pprof listen address (empty = disabled; never expose)", &cfg.DebugAddr)

	boolean("version", "Print version information and exit", &cfg.ShowVersion)

	// Short aliases for the C server's single-letter options, so muscle memory
	// and old runbooks keep working. The long forms are canonical; these write
	// through to the same target rather than being separate settings.
	aliases := map[string]string{
		"d": "lib-dir",
		"o": "log-file",
		"m": "mini-mud",
		"q": "skip-rent-check",
		"r": "restrict",
		"s": "no-specials",
	}

	fs.Usage = func() {
		var b strings.Builder
		b.WriteString("dlmud - the Disgracelands server\n\n")
		b.WriteString("Usage: dlmud [options]\n\n")
		b.WriteString("Every option can also be set from the environment using the name in\n")
		b.WriteString("brackets. Precedence is flag > environment > default.\n\n")
		b.WriteString("Options:\n")
		_, _ = io.WriteString(out, b.String())

		fs.PrintDefaults()

		b.Reset()
		b.WriteString("\nShort aliases for the C server's options: ")
		for _, short := range []string{"d", "o", "m", "q", "r", "s"} {
			fmt.Fprintf(&b, "-%s=--%s ", short, aliases[short])
		}
		b.WriteString("\n")
		_, _ = io.WriteString(out, b.String())
	}

	for short, long := range aliases {
		f := fs.Lookup(long)
		fs.Var(aliasValue{f.Value}, short, "alias for --"+long)
	}

	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if fs.NArg() > 0 {
		return nil, fmt.Errorf("unexpected argument %q: the C server's bare port argument is now --listen-telnet or --listen-telnets", fs.Arg(0))
	}

	// Record which flags were given explicitly, including via an alias, so the
	// environment does not overwrite them.
	explicit := map[string]bool{}
	fs.Visit(func(f *flag.Flag) {
		name := f.Name
		if long, ok := aliases[name]; ok {
			name = long
		}
		explicit[name] = true
	})

	// A deployment that still sets a removed flag's environment variable
	// is told, by name, rather than ignored.
	//
	// internal/config derives environment names from flag names rather
	// than declaring them separately (go-port-plan.md §10), so deleting a
	// flag deletes its variable automatically — and silently. The most
	// likely failure of this release is a container that has had
	// DLMUD_WORLD_FORMAT=classic in its unit file since 2026 and now
	// quietly ignores it, boots on data it was not pointed at, and
	// produces a confusing error three layers down. docs/proposals/
	// yaml-only.md §3.1 asks for exactly this check.
	if err := rejectRemovedEnv(lookupEnv); err != nil {
		return nil, err
	}

	// Apply the environment to everything not set on the command line.
	for _, b := range bindings {
		if explicit[b.name] {
			continue
		}
		env := EnvName(b.name)
		v, ok := lookupEnv(env)
		if !ok {
			continue
		}
		if err := b.assign(v); err != nil {
			return nil, fmt.Errorf("%s: %w", env, err)
		}
	}

	if err := cfg.LogLevel.UnmarshalText([]byte(logLevel)); err != nil {
		return nil, fmt.Errorf("--log-level: %q is not one of debug, info, warn, error", logLevel)
	}

	if cfg.ShowVersion {
		return &cfg, nil
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// removedFormatFlags are the flags docs/proposals/yaml-only.md deleted,
// kept only so that their environment variables can be refused by name.
//
// Not "rejected values that are not yaml" — removed. A flag whose only
// valid value is its default is noise, and leaving it invites a future
// reader to think the seam is still live (§3.1).
var removedFormatFlags = []string{
	"player-format", "world-format", "state-format",
	"names-format", "messages-format", "socials-format", "help-format",
}

// rejectRemovedEnv fails if the environment sets a variable belonging to a
// flag this release deleted.
func rejectRemovedEnv(lookupEnv func(string) (string, bool)) error {
	for _, name := range removedFormatFlags {
		env := EnvName(name)
		v, ok := lookupEnv(env)
		if !ok {
			continue
		}
		return fmt.Errorf("%s is set to %q and --%s no longer exists: the server reads one on-disk "+
			"format now. Unset %s, and if the directory it named is a legacy one, convert it once "+
			"and point --lib-dir at the result:\n"+
			"    dlctl import --from-dir=<lib> --to-dir=<somewhere>", env, v, name, env)
	}
	return nil
}

// Validate reports configurations that cannot work, with an explanation of
// what to do instead. It is called by Load and exported for tests.
func (c *Config) Validate() error {
	if !contains(knownLogFormats, c.LogFormat) {
		return fmt.Errorf("--log-format: unknown format %q (have: %s)", c.LogFormat, strings.Join(knownLogFormats, ", "))
	}
	if c.LibDir == "" {
		return fmt.Errorf("--lib-dir: must not be empty")
	}
	if c.PulseInterval <= 0 {
		return fmt.Errorf("--pulse-interval: must be positive, got %s", c.PulseInterval)
	}
	if _, err := rng.New(c.RNG, 1); err != nil {
		return fmt.Errorf("--rng: %w", err)
	}
	if c.MaxPlayers <= 0 {
		return fmt.Errorf("--max-players: must be positive, got %d", c.MaxPlayers)
	}
	if c.MaxConnsPerIP <= 0 {
		return fmt.Errorf("--max-connections-per-ip: must be positive, got %d", c.MaxConnsPerIP)
	}

	if c.TelnetAddr == "" && c.TelnetsAddr == "" && c.WSAddr == "" {
		return fmt.Errorf("no listeners enabled: set at least one of --listen-telnet, --listen-telnets, --listen-ws")
	}

	// TLS is required for the TLS listener, and for the WebSocket listener
	// unless something in front is terminating it.
	needsTLS := c.TelnetsAddr != ""
	if needsTLS && !c.hasTLSSource() {
		return fmt.Errorf("--listen-telnets needs a certificate: set --tls-cert and --tls-key, or --tls-acme-domain")
	}
	if (c.TLSCert == "") != (c.TLSKey == "") {
		return fmt.Errorf("--tls-cert and --tls-key must be set together")
	}
	if c.TLSCert != "" && c.TLSACMEDomain != "" {
		return fmt.Errorf("--tls-cert and --tls-acme-domain are mutually exclusive")
	}
	return nil
}

func (c *Config) hasTLSSource() bool {
	return (c.TLSCert != "" && c.TLSKey != "") || c.TLSACMEDomain != ""
}

// Warnings returns non-fatal notes worth logging at startup: configurations
// that work but that someone should know about.
func (c *Config) Warnings() []string {
	var w []string
	if c.TelnetAddr != "" {
		w = append(w, "plaintext telnet is enabled on "+c.TelnetAddr+": passwords cross the network in the clear")
	}
	if c.AllowLegacyPasswords {
		w = append(w, "legacy DES crypt(3) password verification is enabled for the pre-2008 roster; "+
			"hashes upgrade on successful login, disable with --allow-legacy-passwords=false once the roster has migrated")
	}
	if c.DebugAddr != "" {
		w = append(w, "pprof is listening on "+c.DebugAddr+": do not expose this address")
	}
	if c.WSAddr != "" && !c.hasTLSSource() && !c.TrustProxyHeaders {
		w = append(w, "the web interface has no TLS and --trust-proxy-headers is off: "+
			"expect this only behind a TLS-terminating proxy")
	}
	if c.WSAddr != "" && c.WebPassword == "" {
		w = append(w, "the web interface is enabled with no --web-password: anyone who can reach "+
			c.WSAddr+" can reach the game's own login prompt")
	}
	return w
}

// aliasValue lets a short flag write through to the same target as its long
// form, so -m and --mini-mud are genuinely one setting rather than two that
// have to be reconciled.
type aliasValue struct{ flag.Value }

// IsBoolFlag makes -m work without an explicit =true, matching the C server.
func (a aliasValue) IsBoolFlag() bool {
	bf, ok := a.Value.(interface{ IsBoolFlag() bool })
	return ok && bf.IsBoolFlag()
}

// String tolerates a nil embedded Value, which is not a case any alias
// registered above can reach -- but is exactly the case flag.PrintDefaults
// manufactures. To decide whether a default is worth printing, isZeroValue
// builds a fresh zero of the flag's Value type by reflection and calls
// String on it (flag.go:538-560). The zero aliasValue embeds a nil
// flag.Value interface, so the promoted String dereferences nil; flag
// recovers, and prints "panic calling String method on zero
// config.aliasValue for flag d" -- once per alias, six lines of it, between
// the option list and the alias summary of every `dlmud --help` and every
// usage error. Nothing was broken, but the first thing a new operator runs
// printed six panics at them (#284). Same reason IsBoolFlag above is written
// to work on a zero value.
func (a aliasValue) String() string {
	if a.Value == nil {
		return ""
	}
	return a.Value.String()
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
