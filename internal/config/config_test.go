// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

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
		// The seven --*-format flags are gone (docs/proposals/yaml-only.md
		// §3.1), so what used to be four "unknown format" cases is now one
		// property: passing one at all is an unknown flag.
		{"player-format no longer exists", append(minimal, "--player-format=ascii"), "not defined"},
		{"world-format no longer exists", append(minimal, "--world-format=classic"), "not defined"},
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
	if got, want := cfg.PlayerPath(), "/srv/dl/players"; got != want {
		t.Errorf("PlayerPath() = %q, want %q", got, want)
	}
	if got, want := cfg.WorldPath(), "/srv/dl/world"; got != want {
		t.Errorf("WorldPath() = %q, want %q", got, want)
	}

	override := load(t, append(minimal, "--lib-dir=/srv/dl", "--world-dir=/other/world"), nil)
	if got, want := override.WorldPath(), "/other/world"; got != want {
		t.Errorf("WorldPath() = %q, want the override %q", got, want)
	}

	// --player-dir still overrides, for a deployment that keeps its roster
	// somewhere other than under --lib-dir.
	elsewhere := load(t, append(minimal, "--lib-dir=/srv/dl", "--player-dir=/other/players"), nil)
	if got, want := elsewhere.PlayerPath(), "/other/players"; got != want {
		t.Errorf("PlayerPath() = %q, want the override %q", got, want)
	}
}

// TestRemovedFormatEnvIsRefusedByName. Deleting a flag deletes its
// environment variable too, because internal/config derives one from the
// other — and does it silently. A container with DLMUD_WORLD_FORMAT=classic
// in its unit file since 2026 quietly ignoring it is the most likely
// failure of this release (docs/proposals/yaml-only.md §3.1).
func TestRemovedFormatEnvIsRefusedByName(t *testing.T) {
	for _, name := range removedFormatFlags {
		env := EnvName(name)
		t.Run(env, func(t *testing.T) {
			_, err := Load(minimal, func(k string) (string, bool) {
				if k == env {
					return "classic", true
				}
				return "", false
			}, io.Discard)
			if err == nil {
				t.Fatalf("Load with %s set succeeded, want a refusal", env)
			}
			for _, want := range []string{env, "--" + name, "dlctl import"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("the refusal does not mention %q: %v", want, err)
				}
			}
		})
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

func TestWebFlags(t *testing.T) {
	cfg := load(t, append(minimal, "--listen-ws=:8080", "--web-password=hunter2", "--web-captcha"), nil)
	if cfg.WSAddr != ":8080" {
		t.Errorf("WSAddr = %q, want :8080", cfg.WSAddr)
	}
	if cfg.WebPassword != "hunter2" {
		t.Errorf("WebPassword = %q, want hunter2", cfg.WebPassword)
	}
	if !cfg.WebCaptcha {
		t.Error("WebCaptcha = false, want true")
	}
}

// TestWebWarnings: --listen-ws gets the same TLS caveat --debug-addr's
// neighbours already do, plus one of its own when there is no
// --web-password to gate it — but only when the web interface is actually
// enabled at all.
func TestWebWarnings(t *testing.T) {
	cfg := load(t, append(minimal, "--listen-ws=:8080"), nil)
	warnings := strings.Join(cfg.Warnings(), "\n")
	for _, want := range []string{"web interface has no TLS", "no --web-password"} {
		if !strings.Contains(warnings, want) {
			t.Errorf("Warnings() = %q, want it to mention %q", warnings, want)
		}
	}

	withPassword := load(t, append(minimal, "--listen-ws=:8080", "--web-password=hunter2"), nil)
	if strings.Contains(strings.Join(withPassword.Warnings(), "\n"), "no --web-password") {
		t.Error("Warnings() still complained about --web-password once one was set")
	}

	disabled := load(t, minimal, nil)
	if warnings := strings.Join(disabled.Warnings(), "\n"); strings.Contains(warnings, "web interface") {
		t.Errorf("Warnings() = %q, want no web-interface warning when --listen-ws is unset", warnings)
	}
}

// The usage text is printed to the writer Load is handed, and so are
// flag's own complaints about it: PrintDefaults collects any panic
// isZeroValue provoked and writes the notices to f.Output() after the
// option list (flag.go:645-650). So an assertion on this string catches
// them, and catches them for any Value type added later, not just for
// aliasValue -- which is the point, since the failure is a property of
// how a Value behaves when built by reflection rather than by us.
func TestUsagePrintsNoPanicNotices(t *testing.T) {
	var sb strings.Builder
	if _, err := Load([]string{"--help"}, noEnv, &sb); err == nil {
		t.Fatal("Load([--help]) succeeded, want flag.ErrHelp")
	}
	if out := sb.String(); strings.Contains(out, "panic calling String method") {
		t.Errorf("usage output carries flag's zero-value panic notices (#284):\n%s", out)
	}
}

// aliasValue's zero value is never registered as a flag -- but PrintDefaults
// builds one by reflection on every run, so it has to survive being used.
func TestZeroAliasValueHasAUsableString(t *testing.T) {
	var zero aliasValue
	if got := zero.String(); got != "" {
		t.Errorf("aliasValue{}.String() = %q, want \"\"", got)
	}
	if zero.IsBoolFlag() {
		t.Error("aliasValue{}.IsBoolFlag() = true, want false")
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
	for _, want := range []string{"DL_LIB_DIR", "DL_LISTEN_TELNETS", "DL_LOG_FORMAT", "Precedence is flag > environment > default"} {
		if !strings.Contains(out, want) {
			t.Errorf("usage output missing %q\n%s", want, out)
		}
	}
}
