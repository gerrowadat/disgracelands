package config

import (
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"
)

// noEnv is an empty environment, for tests that only exercise flags.
func noEnv(string) (string, bool) { return "", false }

// envMap turns a map into a lookup function.
func envMap(m map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) {
		v, ok := m[k]
		return v, ok
	}
}

// minimal is the smallest valid argument set: a listener with no TLS needed.
var minimal = []string{"--listen-telnets=", "--listen-telnet=:4000"}

func load(t *testing.T, args []string, env map[string]string) *Config {
	t.Helper()
	cfg, err := Load(args, envMap(env), io.Discard)
	if err != nil {
		t.Fatalf("Load(%q) = error %v, want success", args, err)
	}
	return cfg
}

func TestEnvName(t *testing.T) {
	for flagName, want := range map[string]string{
		"lib-dir":                "DL_LIB_DIR",
		"listen-telnets":         "DL_LISTEN_TELNETS",
		"max-connections-per-ip": "DL_MAX_CONNECTIONS_PER_IP",
		"version":                "DL_VERSION",
	} {
		if got := EnvName(flagName); got != want {
			t.Errorf("EnvName(%q) = %q, want %q", flagName, got, want)
		}
	}
}

func TestDefaultsAreValid(t *testing.T) {
	// The defaults enable only the TLS listener, which needs a certificate, so
	// bare defaults must be rejected with an actionable message rather than
	// silently starting a server nobody can reach.
	_, err := Load(nil, noEnv, io.Discard)
	if err == nil {
		t.Fatal("Load(nil) succeeded, want an error about the missing certificate")
	}
	if !strings.Contains(err.Error(), "--tls-cert") {
		t.Errorf("Load(nil) error = %q, want it to mention --tls-cert", err)
	}
}

func TestPlaintextTelnetIsOffByDefault(t *testing.T) {
	// docs/proposals/go-port-plan.md §0: implemented, but never on unless asked for.
	if got := Default().TelnetAddr; got != "" {
		t.Errorf("Default().TelnetAddr = %q, want empty", got)
	}
}

func TestFlagsBeatEnvironment(t *testing.T) {
	cfg := load(t, append(minimal, "--lib-dir=/from/flag"), map[string]string{
		"DL_LIB_DIR": "/from/env",
	})
	if cfg.LibDir != "/from/flag" {
		t.Errorf("LibDir = %q, want the flag value", cfg.LibDir)
	}
}

func TestEnvironmentBeatsDefault(t *testing.T) {
	cfg := load(t, minimal, map[string]string{"DL_LIB_DIR": "/from/env"})
	if cfg.LibDir != "/from/env" {
		t.Errorf("LibDir = %q, want the environment value", cfg.LibDir)
	}
}

func TestEnvironmentAppliesToEveryType(t *testing.T) {
	cfg := load(t, minimal, map[string]string{
		"DL_MINI_MUD":       "true",
		"DL_MAX_PLAYERS":    "42",
		"DL_PULSE_INTERVAL": "250ms",
		"DL_LOG_FORMAT":     "json",
		"DL_LOG_LEVEL":      "debug",
	})
	if !cfg.MiniMUD {
		t.Error("MiniMUD = false, want true from DL_MINI_MUD")
	}
	if cfg.MaxPlayers != 42 {
		t.Errorf("MaxPlayers = %d, want 42", cfg.MaxPlayers)
	}
	if cfg.PulseInterval != 250*time.Millisecond {
		t.Errorf("PulseInterval = %s, want 250ms", cfg.PulseInterval)
	}
	if cfg.LogFormat != "json" {
		t.Errorf("LogFormat = %q, want json", cfg.LogFormat)
	}
	if cfg.LogLevel != slog.LevelDebug {
		t.Errorf("LogLevel = %v, want debug", cfg.LogLevel)
	}
}

func TestBadEnvironmentValueIsReported(t *testing.T) {
	_, err := Load(minimal, envMap(map[string]string{"DL_MAX_PLAYERS": "lots"}), io.Discard)
	if err == nil {
		t.Fatal("Load succeeded with DL_MAX_PLAYERS=lots, want an error")
	}
	// The message must name the variable, not just the value: with a few dozen
	// settings, "not an integer" alone is not actionable.
	if !strings.Contains(err.Error(), "DL_MAX_PLAYERS") {
		t.Errorf("error = %q, want it to name DL_MAX_PLAYERS", err)
	}
}

func TestShortAliasesWriteThroughToLongForms(t *testing.T) {
	// The C server's options, in C style, must land in the same fields as the
	// long forms rather than being separate settings.
	cfg := load(t, []string{"-d", "/opt/dl", "-m", "-q", "-r", "-s", "--listen-telnet=:4000", "--listen-telnets="}, nil)
	if cfg.LibDir != "/opt/dl" {
		t.Errorf("LibDir = %q, want /opt/dl from -d", cfg.LibDir)
	}
	for name, got := range map[string]bool{
		"-m": cfg.MiniMUD,
		"-q": cfg.SkipRentCheck,
		"-r": cfg.Restrict,
		"-s": cfg.NoSpecials,
	} {
		if !got {
			t.Errorf("%s did not set its target", name)
		}
	}
}

func TestShortAliasBeatsEnvironment(t *testing.T) {
	// An alias is an explicit command-line choice and must win, exactly as the
	// long form does.
	cfg := load(t, append(minimal, "-d", "/from/alias"), map[string]string{"DL_LIB_DIR": "/from/env"})
	if cfg.LibDir != "/from/alias" {
		t.Errorf("LibDir = %q, want /from/alias", cfg.LibDir)
	}
}

func TestBarePortArgumentIsRejectedWithGuidance(t *testing.T) {
	_, err := Load([]string{"4000"}, noEnv, io.Discard)
	if err == nil {
		t.Fatal("Load([4000]) succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "--listen-telnet") {
		t.Errorf("error = %q, want it to point at --listen-telnet", err)
	}
}

func TestValidation(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"no listeners", []string{"--listen-telnets="}, "no listeners enabled"},
		{"unknown player format", append(minimal, "--player-format=sqlite"), "--player-format"},
		// The binary format is a real format the tooling reads and writes, but
		// the server cannot run on it: its password field is eleven bytes.
		{"binary is conversion-only", append(minimal, "--player-format=binary"), "conversion format only"},
		{"unknown world format", append(minimal, "--world-format=json"), "--world-format"},
		{"unknown log format", append(minimal, "--log-format=xml"), "--log-format"},
		{"bad log level", append(minimal, "--log-level=chatty"), "--log-level"},
		{"empty lib dir", append(minimal, "--lib-dir="), "--lib-dir"},
		{"zero pulse", append(minimal, "--pulse-interval=0"), "--pulse-interval"},
		{"negative players", append(minimal, "--max-players=-1"), "--max-players"},
		{"tls listener without cert", []string{"--listen-telnets=:4443"}, "--listen-telnets needs a certificate"},
		{"half a keypair", append(minimal, "--tls-cert=/c"), "must be set together"},
		{"cert and acme", append(minimal, "--tls-cert=/c", "--tls-key=/k", "--tls-acme-domain=d"), "mutually exclusive"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Load(tt.args, noEnv, io.Discard)
			if err == nil {
				t.Fatalf("Load(%q) succeeded, want an error containing %q", tt.args, tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %q, want it to contain %q", err, tt.want)
			}
		})
	}
}

func TestTLSListenerAcceptsACME(t *testing.T) {
	cfg := load(t, []string{"--listen-telnets=:4443", "--tls-acme-domain=mud.example.org"}, nil)
	if !cfg.hasTLSSource() {
		t.Error("hasTLSSource() = false with an ACME domain set, want true")
	}
}

func TestPathDerivation(t *testing.T) {
	cfg := load(t, append(minimal, "--lib-dir=/srv/dl"), nil)
	if got, want := cfg.PlayerPath(), "/srv/dl/pfiles"; got != want {
		t.Errorf("PlayerPath() = %q, want %q", got, want)
	}
	if got, want := cfg.WorldPath(), "/srv/dl/world"; got != want {
		t.Errorf("WorldPath() = %q, want %q", got, want)
	}

	override := load(t, append(minimal, "--lib-dir=/srv/dl", "--world-dir=/other/world"), nil)
	if got, want := override.WorldPath(), "/other/world"; got != want {
		t.Errorf("WorldPath() = %q, want the override %q", got, want)
	}
}

func TestVersionSkipsValidation(t *testing.T) {
	// --version must work on a box with no certificate configured.
	cfg, err := Load([]string{"--version"}, noEnv, io.Discard)
	if err != nil {
		t.Fatalf("Load([--version]) = error %v, want success", err)
	}
	if !cfg.ShowVersion {
		t.Error("ShowVersion = false, want true")
	}
}

func TestWarnings(t *testing.T) {
	cfg := load(t, []string{"--listen-telnet=:4000", "--listen-telnets=", "--debug-addr=:6060"}, nil)
	warnings := strings.Join(cfg.Warnings(), "\n")
	for _, want := range []string{"plaintext telnet", "legacy DES", "pprof"} {
		if !strings.Contains(warnings, want) {
			t.Errorf("Warnings() = %q, want it to mention %q", warnings, want)
		}
	}
}

func TestUsageDocumentsEnvironmentVariables(t *testing.T) {
	// The usage text is the only place the flag/env correspondence is visible
	// to an operator, so it has to actually appear there.
	var sb strings.Builder
	_, err := Load([]string{"--help"}, noEnv, &sb)
	if err == nil {
		t.Fatal("Load([--help]) succeeded, want flag.ErrHelp")
	}
	out := sb.String()
	for _, want := range []string{"DL_LIB_DIR", "DL_LISTEN_TELNETS", "DL_PLAYER_FORMAT", "Precedence is flag > environment > default"} {
		if !strings.Contains(out, want) {
			t.Errorf("usage output missing %q\n%s", want, out)
		}
	}
}
