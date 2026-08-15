// Package config resolves the server's runtime settings.
//
// Precedence is flags > environment > defaults, per docs/go-port-plan.md §9.1.
// Every setting is declared exactly once, in [register]; the environment
// variable name is derived from the flag name rather than written out
// separately, so the two cannot drift apart.
//
// A config file layer (§9.1) will sit between environment and defaults when
// the game-tuning values currently living in src/config.c are ported. The
// precedence chain here already has the slot for it; there is nothing to put
// in it yet.
package config

import (
	"flag"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"time"
)

// EnvPrefix prefixes every environment variable this package reads.
const EnvPrefix = "DL_"

// Config is the fully resolved server configuration.
type Config struct {
	// Data locations.
	LibDir    string
	PlayerDir string // empty means "derive from LibDir"
	WorldDir  string // empty means "derive from LibDir"

	// Pluggable format selection (docs/go-port-plan.md §5, §6).
	PlayerFormat string
	WorldFormat  string

	// Listeners. An empty address means the listener is disabled.
	TelnetAddr  string
	TelnetsAddr string
	WSAddr      string

	// TLS.
	TLSCert         string
	TLSKey          string
	TLSACMEDomain   string
	TLSACMECacheDir string

	// Connection handling.
	TrustProxyHeaders bool
	MaxPlayers        int
	MaxConnsPerIP     int
	LoginGraceTime    time.Duration

	// Engine.
	PulseInterval time.Duration

	// Behaviour carried over from the C server's single-letter options.
	MiniMUD       bool
	SkipRentCheck bool
	Restrict      bool
	NoSpecials    bool

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

// PlayerPath returns the player-data directory, defaulting to LibDir.
func (c *Config) PlayerPath() string {
	if c.PlayerDir != "" {
		return c.PlayerDir
	}
	return c.LibDir
}

// WorldPath returns the world-data directory, defaulting to LibDir/world.
func (c *Config) WorldPath() string {
	if c.WorldDir != "" {
		return c.WorldDir
	}
	return c.LibDir + "/world"
}

// Known format names. These are validated here so a typo fails at startup
// with a useful message rather than deep inside persistence setup. The
// authoritative list is the registry in internal/persist; until those
// packages exist, this is the list.
var (
	knownPlayerFormats = []string{"binary", "ascii"}
	knownWorldFormats  = []string{"classic"}
	knownLogFormats    = []string{"text", "json"}
)

// Default returns the configuration used when nothing is specified. Every
// default reproduces the C server's behaviour where one exists, with the
// documented exception of TelnetAddr (see docs/go-port-plan.md §0: plaintext
// telnet is implemented but off unless asked for).
func Default() Config {
	return Config{
		LibDir:               "lib",
		PlayerFormat:         "binary",
		WorldFormat:          "classic",
		TelnetAddr:           "",
		TelnetsAddr:          ":4443",
		WSAddr:               "",
		TLSACMECacheDir:      "lib/.acme",
		MaxPlayers:           300,
		MaxConnsPerIP:        8,
		LoginGraceTime:       60 * time.Second,
		PulseInterval:        100 * time.Millisecond,
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
	str("player-dir", "Player-data directory (default: --lib-dir)", &cfg.PlayerDir)
	str("world-dir", "World-data directory (default: <lib-dir>/world)", &cfg.WorldDir)

	str("player-format", "Player-file format: "+strings.Join(knownPlayerFormats, ", "), &cfg.PlayerFormat)
	str("world-format", "World-file format: "+strings.Join(knownWorldFormats, ", "), &cfg.WorldFormat)

	str("listen-telnet", "Plaintext telnet listen address (empty = disabled)", &cfg.TelnetAddr)
	str("listen-telnets", "TLS telnet listen address (empty = disabled)", &cfg.TelnetsAddr)
	str("listen-ws", "WebSocket listen address (empty = disabled)", &cfg.WSAddr)

	str("tls-cert", "TLS certificate file", &cfg.TLSCert)
	str("tls-key", "TLS private key file", &cfg.TLSKey)
	str("tls-acme-domain", "Obtain a TLS certificate via ACME for this domain", &cfg.TLSACMEDomain)
	str("tls-acme-cache", "Directory for ACME certificate cache", &cfg.TLSACMECacheDir)

	boolean("trust-proxy-headers", "Trust X-Forwarded-For from a reverse proxy", &cfg.TrustProxyHeaders)
	integer("max-players", "Maximum simultaneous players", &cfg.MaxPlayers)
	integer("max-connections-per-ip", "Maximum simultaneous connections from one address", &cfg.MaxConnsPerIP)
	duration("login-grace-time", "Time a connection may stay unauthenticated", &cfg.LoginGraceTime)

	duration("pulse-interval", "Game loop pulse interval (C server used 100ms)", &cfg.PulseInterval)

	boolean("mini-mud", "Load a minimal world for testing (C: -m)", &cfg.MiniMUD)
	boolean("skip-rent-check", "Skip the rent scan on boot (C: -q)", &cfg.SkipRentCheck)
	boolean("restrict", "Allow no new players (C: -r)", &cfg.Restrict)
	boolean("no-specials", "Suppress special procedure assignment (C: -s)", &cfg.NoSpecials)

	boolean("allow-legacy-passwords", "Accept pre-2008 DES crypt(3) password hashes", &cfg.AllowLegacyPasswords)

	str("log-file", "Log destination, or - for stdout (C: -o)", &cfg.LogFile)
	str("log-format", "Log format: "+strings.Join(knownLogFormats, ", "), &cfg.LogFormat)
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

// Validate reports configurations that cannot work, with an explanation of
// what to do instead. It is called by Load and exported for tests.
func (c *Config) Validate() error {
	if !contains(knownPlayerFormats, c.PlayerFormat) {
		return fmt.Errorf("--player-format: unknown format %q (have: %s)", c.PlayerFormat, strings.Join(knownPlayerFormats, ", "))
	}
	if !contains(knownWorldFormats, c.WorldFormat) {
		return fmt.Errorf("--world-format: unknown format %q (have: %s)", c.WorldFormat, strings.Join(knownWorldFormats, ", "))
	}
	if !contains(knownLogFormats, c.LogFormat) {
		return fmt.Errorf("--log-format: unknown format %q (have: %s)", c.LogFormat, strings.Join(knownLogFormats, ", "))
	}
	if c.LibDir == "" {
		return fmt.Errorf("--lib-dir: must not be empty")
	}
	if c.PulseInterval <= 0 {
		return fmt.Errorf("--pulse-interval: must be positive, got %s", c.PulseInterval)
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
		w = append(w, "the WebSocket listener has no TLS and --trust-proxy-headers is off: "+
			"expect this only behind a TLS-terminating proxy")
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

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
