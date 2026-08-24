// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

//go:build play

// Package play is the play regression suite: a real dlmud process, booted on
// a real data directory, driven over a real socket by a client that types the
// same things a player would.
//
// It is deliberately the opposite of internal/server's tests, which are the
// fast ones and stay in day-to-day CI. Those build a world in Go, wire the
// server up field by field and reach into the world goroutine to assert on
// it; that is what makes them precise, and it is also what makes them blind
// to everything the boot sequence does. Nothing in internal/server loads a
// world file, runs a zone reset, attaches a special procedure by vnum,
// resolves a shop's keeper, reads text/ off disk or parses a flag. A world
// that no longer boots, a zone that no longer populates, a special that
// stopped being assigned, a format that drifted: all of those pass every
// test in internal/server and fail the first thing a player types.
//
// So this suite starts `dlmud` the way an operator would, on
// examples/mini -- the tutorial world, whose whole design is one feature per
// room (examples/mini/README.md) -- and walks the tour. Every assertion here
// is a string a player would have seen.
//
// It is behind a build tag because it is slow: a process per test, a build
// up front, and real sockets. `make play` runs it, and release.yml runs it
// before a tag is cut. See docs/developer.md.
package play

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// serverBinary is the dlmud built once by TestMain and shared by every test.
var serverBinary string

// TestMain builds the server before any test runs.
//
// Building once rather than per test is the difference between the suite
// taking seconds and taking minutes, and building at all -- rather than
// calling into internal/server -- is the point of the suite: what is under
// test is the program, flags and boot sequence included, not a struct
// assembled by a test helper.
func TestMain(m *testing.M) {
	code, err := buildAndRun(m)
	if err != nil {
		fmt.Fprintf(os.Stderr, "play: %v\n", err)
		os.Exit(1)
	}
	os.Exit(code)
}

func buildAndRun(m *testing.M) (int, error) {
	dir, err := os.MkdirTemp("", "play-bin")
	if err != nil {
		return 0, err
	}
	defer func() { _ = os.RemoveAll(dir) }()

	serverBinary = filepath.Join(dir, "dlmud")
	args := []string{"build", "-o", serverBinary}
	// The child is built with the race detector exactly when the test binary
	// was. `go test -race ./test/play` instrumenting only the driver would
	// say nothing about the server, which is where every goroutine that
	// matters lives -- and the world goroutine, the pulses and the per-
	// connection goroutines are the whole reason -race is mandatory here
	// (CLAUDE.md, "Concurrency").
	if raceEnabled {
		args = append(args, "-race")
	}
	args = append(args, "./cmd/dlmud")

	build := exec.Command("go", args...) //nolint:gosec // fixed arguments
	build.Dir = repoRoot()
	build.Stderr = os.Stderr
	build.Stdout = os.Stderr
	if err := build.Run(); err != nil {
		return 0, fmt.Errorf("building cmd/dlmud: %w", err)
	}

	return m.Run(), nil
}

// runServer runs the server to completion with the given flags and returns
// everything it printed, for the cases where booting is supposed to fail.
//
// A boot failure is a real outcome with its own exit code and its own
// message, and it is the one thing an operator sees more often than any
// other. start cannot express it: it waits for "ready", which is precisely
// what is not going to happen.
func runServer(t *testing.T, args ...string) (string, error) {
	t.Helper()

	cmd := exec.Command(serverBinary, args...) //nolint:gosec // the binary this suite built
	cmd.Dir = repoRoot()
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// repoRoot walks up from the working directory to the directory holding
// go.mod. The tests run in test/play; the example data is addressed from the
// root so that moving this package does not silently break the paths.
func repoRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			panic("no go.mod above " + dir)
		}
		dir = parent
	}
}

// lib describes one of the checked-in example data directories, and the
// flags the server needs to read it.
//
// Both of examples/mini's formats are played through, not just the classic
// one: `dlctl lib import` converting cleanly (cmd/dlctl's own tests) and the
// world dumps matching are necessary but not sufficient -- the question this
// answers is whether a server booted on the converted directory is a game
// you can play. That is a different question, and it is the one that would
// bite an operator who converted their archive.
type lib struct {
	// name is the subtest name.
	name string
	// dir is the source directory, relative to the repository root.
	dir string
	// flags are the format flags the directory needs.
	flags []string
}

var (
	miniClassic = lib{name: "classic", dir: "examples/mini/binary"}
	miniYAML    = lib{
		name: "yaml",
		dir:  "examples/mini/yaml",
		flags: []string{
			"--world-format=yaml", "--state-format=yaml", "--names-format=yaml",
			"--messages-format=yaml", "--socials-format=yaml", "--help-format=yaml",
		},
	}
)

// bothFormats is what a test ranges over when it should hold in either
// format. Tests about a rule rather than about the data use miniClassic
// alone; there is no point paying for a second boot to re-prove arithmetic.
var bothFormats = []lib{miniClassic, miniYAML}

// mud is a running server and the temporary data directory it runs on.
type mud struct {
	t    *testing.T
	addr string
	// dir is the throwaway copy of the example data. Tests that look at what
	// the server wrote -- a pfile, a rent file, a board -- read it from here.
	dir string

	cmd  *exec.Cmd
	done chan struct{}
	// stopped makes stop idempotent: a test that shuts the server down
	// itself still has the cleanup registered behind it.
	stopped bool
	// exit is what the process exited with, once it has. Only meaningful
	// after wait; see cmd/dlmud's exit code constants for what each one
	// means.
	exit int

	mu sync.Mutex
	// logs is every structured log line the server emitted, kept whole for
	// the failure dump: a play test that fails usually failed because of
	// something the server said to its log rather than to the socket.
	logs []logLine
}

// logLine is one JSON log record, decoded only as far as this suite cares.
type logLine struct {
	Severity string `json:"severity_text"`
	Msg      string `json:"_msg"`
	// The server's JSON format is OpenTelemetry-shaped (internal/obs/
	// otel.go), so a record's own attributes are nested under `attributes`
	// rather than sitting at the top level.
	Attrs struct {
		Addr string `json:"addr"`
	} `json:"attributes"`
	raw string
}

// startOptions are the knobs a test can turn before boot.
type startOptions struct {
	// extraFlags are appended after the harness's own, so a test can
	// override any of them.
	extraFlags []string
	// noFounder skips creating the implementor the harness otherwise makes
	// first. See start's comment: only a test *about* the first-player rule
	// wants this.
	noFounder bool
}

// start boots a server on a throwaway copy of l and returns it ready to play.
func start(t *testing.T, l lib, opts ...startOptions) *mud {
	t.Helper()

	var opt startOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	return startAt(t, l, stageLib(t, l), opt)
}

// startAt is start on a directory that already exists, which is how a test
// restarts a server on the data a previous one left behind.
func startAt(t *testing.T, l lib, dir string, opt startOptions) *mud {
	t.Helper()

	m := &mud{t: t, dir: dir, done: make(chan struct{})}

	args := []string{
		"--lib-dir=" + m.dir,
		"--listen-telnet=127.0.0.1:0",
		// Every other listener off: this suite drives telnet, and a TLS
		// listener with no certificate is a boot failure rather than a
		// skipped feature.
		"--listen-telnets=",
		"--metrics-addr=",
		"--log-format=json",
		"--log-file=-",
		// The C server's own generator, on a fixed seed. A play test that
		// hits a training dummy should fail because the fight changed, not
		// because the dice did (docs/weirdnumbers.md; rng.New's "circle").
		"--rng=circle",
		"--rng-seed=20010101",
	}
	args = append(args, l.flags...)
	args = append(args, opt.extraFlags...)

	m.cmd = exec.Command(serverBinary, args...) //nolint:gosec // the binary this suite built
	m.cmd.Dir = repoRoot()

	// One pipe for both streams, rather than StdoutPipe: the structured log
	// goes to stdout under --log-file=-, but a runtime panic and a race
	// report go to stderr, and interleaving them into one reader is what
	// keeps a crash visible in the same dump as the play that caused it.
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	m.cmd.Stdout = pw
	m.cmd.Stderr = pw

	ready := make(chan string, 1)
	go m.readLog(pr, ready)

	if err := m.cmd.Start(); err != nil {
		_ = pw.Close()
		t.Fatalf("starting the server: %v", err)
	}
	// The parent's copy of the write end has to go, or readLog never sees
	// EOF and m.done never closes -- which shows up as stop() timing out
	// after the server has already exited perfectly happily.
	_ = pw.Close()
	t.Cleanup(m.stop)

	select {
	case addr := <-ready:
		m.addr = addr
	case <-time.After(30 * time.Second):
		t.Fatalf("the server never became ready. Its log was:\n%s", m.logText())
	}

	// db.c's "if this is our first player --- he be God" makes the first
	// character on an empty roster an Implementor. That is a real rule with
	// its own test (TestTheFirstCharacterIsAnImplementor), but every *other*
	// test wants an ordinary mortal walking the tutorial, and on a fresh
	// data directory it cannot have one until somebody has taken that slot.
	// So the harness takes it, once, and every character a test creates
	// afterwards is a level 1 mortal like any other.
	if !opt.noFounder {
		c := m.dial()
		c.create(founderName, founderPassword, "m", "w")
		c.quit()
		c.close()
		// The *index* entry, not just the record and not just the
		// connection. Two separate waits would both be tempting and both be
		// wrong on their own: the reply to `quit` arrives before either file
		// exists, because the save is pushed off the world goroutine, and
		// the record file appears before the index line does. Everything
		// afterwards -- a second character not being the implementor, `mail
		// send Founder` finding an addressee at all (get_id_by_name reads
		// this file, internal/server/mail.go's IDByName) -- depends on the
		// index, so the index is what start waits for.
		if !eventually(10*time.Second, func() bool { return m.rosterHas(founderName) }) {
			t.Fatalf("the implementor never reached the roster index. The server log was:\n%s", m.logText())
		}
	}

	return m
}

// founderName and founderPassword are the implementor start creates. Tests
// that need a god log in as this one rather than making a second.
const (
	founderName     = "Founder"
	founderPassword = "founderpass"
)

// stageLib copies an example directory into the test's own temporary space
// and adds the writable directories a server expects.
//
// A copy, never the original: the server writes players, rent files, boards,
// mail and a clock, and a suite that let it do that in examples/ would be
// modifying checked-in data as a side effect of running.
func stageLib(t *testing.T, l lib) string {
	t.Helper()

	dir := filepath.Join(t.TempDir(), "lib")
	src := filepath.Join(repoRoot(), l.dir)
	if err := os.CopyFS(dir, os.DirFS(src)); err != nil {
		t.Fatalf("staging %s: %v", l.dir, err)
	}
	// os.CopyFS reproduces the source's read-only bits, and the checked-in
	// example data is checked in read-only often enough to matter: the
	// server has to be able to write into these.
	if err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		mode := os.FileMode(0o600)
		if d.IsDir() {
			mode = 0o700
		}
		return os.Chmod(path, mode)
	}); err != nil {
		t.Fatal(err)
	}

	// The directories a real lib/ has and an example does not, because they
	// only ever hold player state. A missing one is a first-login failure
	// rather than an empty roster -- the same reason the Makefile's own
	// scratch target recreates them.
	for _, sub := range []string{"pfiles", "plrobjs", "plralias", "house", "etc", "misc", "state"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// readLog consumes the server's structured log, remembers it, and reports the
// listening address once the server says it is ready.
func (m *mud) readLog(r io.Reader, ready chan<- string) {
	defer close(m.done)

	var addr string
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		text := scanner.Text()
		line := logLine{raw: text}
		// A line that is not JSON is still worth keeping: a runtime panic
		// or a race report arrives on this same pipe and is not structured
		// at all, and losing it would turn the most informative failure the
		// suite can produce into a silent timeout.
		_ = json.Unmarshal([]byte(text), &line)

		m.mu.Lock()
		m.logs = append(m.logs, line)
		m.mu.Unlock()

		if line.Msg == "listening" && line.Attrs.Addr != "" {
			addr = line.Attrs.Addr
		}
		if line.Msg == "ready" && addr != "" {
			select {
			case ready <- addr:
			default:
			}
		}
	}
}

// restart stops this server and boots another one on the same data
// directory, which is what a `make release`-shaped deploy does to a live
// game: everything a player expects to survive a restart has to be on disk
// by the time the first process exits.
func (m *mud) restart(l lib) *mud {
	m.t.Helper()

	m.stop()
	return startAt(m.t, l, m.dir, startOptions{noFounder: true})
}

// stop shuts the server down the way an operator would, and fails the test if
// it will not go.
//
// SIGTERM rather than Kill because the graceful path is itself under test:
// it saves every character still in the world and waits for the writes
// (cmd/dlmud's own shutdown, and Server.SaveEverything). A suite that killed
// the process would never notice that breaking, and several tests here check
// a file the shutdown is what wrote.
func (m *mud) stop() {
	if m.cmd == nil || m.cmd.Process == nil || m.stopped {
		return
	}
	_ = m.cmd.Process.Signal(syscall.SIGTERM)
	m.wait()

	if m.t.Failed() {
		m.t.Logf("server log:\n%s", m.logText())
	}
}

// wait blocks until the server process has exited and returns its exit
// status, killing it if it will not go.
//
// The exit status is part of the contract with whatever restarts the
// server: `shutdown reboot` exits 2 and asks to come back, everything else
// that worked exits 0 (cmd/dlmud's exit code constants, and
// docs/operations.md for the container settings that read them). Nothing
// short of a real process can observe that, which is why it is asserted
// here and not in internal/server.
//
// Idempotent, and it has to be: a test that stops the server itself still
// has stop registered as cleanup behind it, and os/exec's Wait may only be
// called once.
func (m *mud) wait() int {
	m.t.Helper()
	if m.stopped {
		return m.exit
	}
	m.stopped = true

	select {
	case <-m.done:
	case <-time.After(30 * time.Second):
		m.t.Errorf("the server did not shut down within 30s; killing it. Its log was:\n%s", m.logText())
		_ = m.cmd.Process.Kill()
	}

	if err := m.cmd.Wait(); err != nil {
		var exited *exec.ExitError
		if !errors.As(err, &exited) {
			m.t.Errorf("waiting for the server to exit: %v", err)
			return m.exit
		}
		m.exit = exited.ExitCode()
	}
	return m.exit
}

// signal sends the running server a signal, the way an operator does with
// `docker kill --signal=...` or `kill -HUP`. Everything the server does with
// one is in internal/signals and docs/design/signal-handling.md.
func (m *mud) signal(sig syscall.Signal) {
	m.t.Helper()
	if m.cmd == nil || m.cmd.Process == nil {
		m.t.Fatal("the server is not running")
	}
	if err := m.cmd.Process.Signal(sig); err != nil {
		m.t.Fatalf("sending the server %v: %v", sig, err)
	}
}

// logText is the whole server log, for a failure message.
func (m *mud) logText() string {
	m.mu.Lock()
	defer m.mu.Unlock()

	var b strings.Builder
	for _, l := range m.logs {
		b.WriteString(l.raw)
		b.WriteByte('\n')
	}
	return b.String()
}

// errorLines is every ERROR the server logged.
//
// This is the suite's widest net and the cheapest one. engine.Run logs "a
// task panicked and was contained" at ERROR when a command panics
// (internal/engine/engine.go:227) -- the world goroutine's recover is what
// keeps one bad command from taking the game down, and it is also what turns
// a nil dereference in a brand new feature into a player seeing nothing much
// and a test seeing nothing at all. Any test that plays through a feature
// gets that check for free by calling noServerErrors.
func (m *mud) errorLines() []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	var out []string
	for _, l := range m.logs {
		if l.Severity == "ERROR" {
			out = append(out, l.raw)
		}
	}
	return out
}

// noServerErrors fails the test if the server logged an error while it played.
//
// Call it at the end of anything that exercises a feature. It is what makes
// this a regression suite rather than a collection of transcripts: a new
// command that panics on some input the tour happens to give it fails here
// even if nobody thought to assert on what it printed.
func (m *mud) noServerErrors() {
	m.t.Helper()
	for _, line := range m.errorLines() {
		m.t.Errorf("the server logged an error while playing: %s", line)
	}
}

// path is a path inside the server's data directory.
func (m *mud) path(elem ...string) string {
	return filepath.Join(append([]string{m.dir}, elem...)...)
}

// pfile is where the ascii roster keeps one character: pfiles/<initial>/<name>,
// lowercased, no extension (internal/persist/player/ascii's Store.path).
//
// Tests wait on this file rather than on anything they can see over the
// socket, because the save is pushed off the world goroutine on purpose --
// a slow disk must not stall the game -- so the reply to `quit` arrives
// before the record exists.
func (m *mud) pfile(name string) string {
	lower := strings.ToLower(name)
	return m.path("pfiles", lower[:1], lower)
}

// rosterHas reports whether the ascii roster index lists name.
//
// plr_index is the file get_id_by_name is ported against, so this is the
// same question the postmaster and the house control ask.
func (m *mud) rosterHas(name string) bool {
	b, err := os.ReadFile(m.path("pfiles", "plr_index"))
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(b), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && strings.EqualFold(fields[1], name) {
			return true
		}
	}
	return false
}

// eventually polls until a condition holds or the deadline passes.
//
// For what happens *after* a command's reply and off the world goroutine: a
// pfile written by a background save, a rent file appearing. Waiting on the
// socket is not a barrier for those -- the same trap internal/server's own
// tests document, and it behaves identically across a process boundary.
func eventually(within time.Duration, ok func() bool) bool {
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if ok() {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return ok()
}

// fileExists is the usual eventually condition.
func fileExists(path string) func() bool {
	return func() bool {
		_, err := os.Stat(path)
		return err == nil
	}
}
