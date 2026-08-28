// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

//go:build parity

// Package parity is the session-parity suite: the same scripts typed at the
// C server and at the Go server, with what they said compared line for line.
//
// It is the sibling of test/play and not a replacement for it, and the
// difference is what each one is evidence of. test/play asserts that the Go
// server says what *this project believes* it should say; the belief is a
// reading of the C, and a reading is what has been wrong repeatedly (all 57
// entries in docs/weirdnumbers.md, and `isname` for four phases). This suite
// asserts that the Go server says what the C server *actually says*, with no
// reading in between — the same relationship scripts/world-parity.sh has to
// the world loader, applied to what a player reads.
//
// So a test here needs no expected output written into it. The C server is
// the expectation, and where the two disagree the Go server is what is wrong
// (plan §0's fidelity rule, which still governs everything a player reads).
//
// Both servers boot on their own throwaway copy of examples/stock/binary,
// with the same fixed RNG seed and their mobiles held still, and every
// scenario is played at both. It is release-only, like test/play, and for
// stronger reasons: it needs a C toolchain, it starts two servers, and
// framing a command's output by silence makes it slow. `make session-parity`
// runs it. See docs/developer.md.
package parity

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gerrowadat/disgracelands/internal/parity"
)

// seed is the fixed RNG seed both servers are given.
//
// Fixing it is what makes a scripted fight comparable at all: both servers
// roll the C's own generator (rng.New's "circle") from the same start, so
// the same script produces the same dice on both sides — which is why the
// abilities `score` prints agree, and why a scripted kill can be compared
// round by round rather than just at the wording of its refusals.
const seed = "20080101"

// paths worked out once by TestMain.
var (
	root      string
	goBinary  string
	ctlBinary string
	cBinary   string
	// cServerErr is why there is no C server, if there is not one. Every
	// test skips on it rather than failing: a machine with no C toolchain
	// has not been told anything about the Go server by a red suite.
	cServerErr error
)

func TestMain(m *testing.M) {
	code, err := build(m)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parity: %v\n", err)
		os.Exit(1)
	}
	os.Exit(code)
}

func build(m *testing.M) (int, error) {
	root = repoRoot()

	dir, err := os.MkdirTemp("", "parity")
	if err != nil {
		return 0, err
	}
	defer func() { _ = os.RemoveAll(dir) }()

	goBinary = filepath.Join(dir, "dlmud")
	buildGo := exec.Command("go", "build", "-o", goBinary, "./cmd/dlmud") //nolint:gosec // fixed arguments
	buildGo.Dir = root
	buildGo.Stdout, buildGo.Stderr = os.Stderr, os.Stderr
	if err := buildGo.Run(); err != nil {
		return 0, fmt.Errorf("building cmd/dlmud: %w", err)
	}

	// dlctl too, because the Go side now plays on a yaml conversion of its
	// staged directory (startPair) rather than on the classic one.
	ctlBinary = filepath.Join(dir, "dlctl")
	buildCtl := exec.Command("go", "build", "-o", ctlBinary, "./cmd/dlctl") //nolint:gosec // fixed arguments
	buildCtl.Dir = root
	buildCtl.Stdout, buildCtl.Stderr = os.Stderr, os.Stderr
	if err := buildCtl.Run(); err != nil {
		return 0, fmt.Errorf("building cmd/dlctl: %w", err)
	}

	cBinary, cServerErr = ensureCServer(dir)

	return m.Run(), nil
}

// ensureCServer returns the path to a built C server, building it first if
// the binary is missing or older than the source it was built from.
//
// The staleness check is not tidiness. `-M`, which holds the mobiles still,
// is a <DoC> addition to comm.c made *for* this suite: a checkout with a
// binary built before it would boot happily, ignore the flag, wander its
// janitors around Midgaard and report every room they walked through as a
// parity failure of the Go server.
func ensureCServer(scratch string) (string, error) {
	src := filepath.Join(root, "reference", "moderncserver", "src")
	bin := filepath.Join(root, "reference", "moderncserver", "bin", "circle")

	if stale, why := outOfDate(bin, src); stale {
		if _, err := exec.LookPath("gcc"); err != nil {
			return "", fmt.Errorf("the C server needs rebuilding (%s) and there is no gcc: %w", why, err)
		}
		build := exec.Command("make", "-C", src)
		build.Stdout, build.Stderr = os.Stderr, os.Stderr
		build.Env = append(os.Environ(), "TMPDIR="+scratch)
		if err := build.Run(); err != nil {
			return "", fmt.Errorf("building the C server: %w", err)
		}
	}
	if _, err := os.Stat(bin); err != nil {
		return "", fmt.Errorf("no C server at %s: %w", bin, err)
	}
	return bin, nil
}

// outOfDate reports whether bin needs rebuilding from the sources in dir.
func outOfDate(bin, dir string) (bool, string) {
	info, err := os.Stat(bin)
	if err != nil {
		return true, "it has not been built"
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return true, "its source directory could not be read"
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".c") && !strings.HasSuffix(name, ".h") {
			continue
		}
		src, err := e.Info()
		if err != nil {
			continue
		}
		if src.ModTime().After(info.ModTime()) {
			return true, name + " is newer than the binary"
		}
	}
	return false, ""
}

// server is one running MUD and the scratch directory it runs on.
type server struct {
	name string
	addr string
	dir  string
	cmd  *exec.Cmd
	log  *os.File
}

// pair is one scenario's two servers.
type pair struct{ c, g *server }

// startPair stages a data directory for each server and starts them, for one
// scenario.
//
// A pair per scenario, not one for the suite. Sharing them would be several
// seconds faster and would make every scenario after the first a comparison
// of two worlds that had already been made to differ: an object the port
// failed to pick up is still lying in the temple when the next scenario walks
// through it, and every room description from there on is a "difference"
// that is really a consequence. Each scenario starting from the reset world
// is what makes its findings its own, and what lets one be run on its own
// with `-run` and mean the same thing it means in the suite.
func startPair(t *testing.T) *pair {
	t.Helper()

	scratch := t.TempDir()
	lib := func(name string) string {
		dir := filepath.Join(scratch, name)
		if err := parity.StageLib(dir, filepath.Join(root, "examples", "stock", "binary")); err != nil {
			t.Fatalf("staging %s: %v", name, err)
		}
		return dir
	}
	cDir, gDir := lib("clib"), lib("glib")

	// The Go side plays on a yaml conversion of its staged directory; the
	// C side keeps the classic one, because it is the only thing it reads.
	// docs/proposals/yaml-only.md §5.4 — until this, both sides booted on
	// the same classic directory, so the one Go configuration a player
	// will ever meet was the one thing this suite never exercised.
	gYaml := filepath.Join(scratch, "gyaml")
	convert := exec.Command(ctlBinary, "import", "--from-dir="+gDir, "--to-dir="+gYaml) //nolint:gosec // paths this function made
	convert.Dir = root
	if out, err := convert.CombinedOutput(); err != nil {
		t.Fatalf("converting the Go side to yaml: %v\n%s", err, out)
	}

	cPort, gPort := freePort(t), freePort(t)

	// -q skips the rent scan, -S fixes the seed, -M holds the mobiles still
	// and -W holds the weather still; the last three are <DoC> additions made
	// for this harness. -d must be absolute because the C server chdir()s
	// into it.
	c := &server{name: "circle", dir: cDir, addr: fmt.Sprintf("127.0.0.1:%d", cPort)}
	c.cmd = exec.Command(cBinary, "-q", "-M", "-W", "-S", seed, "-d", cDir, fmt.Sprint(cPort)) //nolint:gosec // ports and paths this function made

	// The Go server's own spelling of the same four, plus the C's own
	// generator so that both are rolling the same dice from the same seed.
	g := &server{name: "dlmud", dir: gYaml, addr: fmt.Sprintf("127.0.0.1:%d", gPort)}
	g.cmd = exec.Command(goBinary, //nolint:gosec // the binary this suite built
		"--lib-dir="+gYaml,
		"--world-format=yaml", "--state-format=yaml", "--names-format=yaml",
		"--messages-format=yaml", "--socials-format=yaml", "--help-format=yaml",
		"--player-format=yaml",
		"--listen-telnets=",
		"--listen-telnet="+g.addr,
		"--metrics-addr=",
		"--skip-rent-check",
		"--rng=circle",
		"--rng-seed="+seed,
		"--freeze-mobiles", "--freeze-weather",
		"--log-level=error",
	)

	p := &pair{c: c, g: g}
	t.Cleanup(p.stop)
	for _, s := range []*server{c, g} {
		if err := s.start(scratch); err != nil {
			t.Fatal(err)
		}
	}
	for _, s := range []*server{c, g} {
		if err := s.waitUntilListening(60 * time.Second); err != nil {
			t.Fatal(err)
		}
	}
	return p
}

func (p *pair) stop() {
	p.c.stop()
	p.g.stop()
}

func (s *server) start(scratch string) error {
	log, err := os.Create(filepath.Join(scratch, s.name+".log"))
	if err != nil {
		return err
	}
	s.log = log
	s.cmd.Stdout, s.cmd.Stderr = log, log
	s.cmd.Dir = root
	if err := s.cmd.Start(); err != nil {
		return fmt.Errorf("starting %s: %w", s.name, err)
	}
	return nil
}

// waitUntilListening polls the listener rather than the log, because the two
// servers say different things about being ready and the socket is the thing
// the scripts actually need.
func (s *server) waitUntilListening(within time.Duration) error {
	deadline := time.Now().Add(within)
	for {
		conn, err := net.DialTimeout("tcp", s.addr, time.Second)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%s never listened on %s: %w\n%s", s.name, s.addr, err, s.logText())
		}
		if s.cmd.ProcessState != nil {
			return fmt.Errorf("%s exited before it listened:\n%s", s.name, s.logText())
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func (s *server) stop() {
	if s == nil || s.cmd == nil || s.cmd.Process == nil {
		return
	}
	_ = s.cmd.Process.Kill()
	_, _ = s.cmd.Process.Wait()
	if s.log != nil {
		_ = s.log.Close()
	}
}

// logText is what the server said to its log, for a failure that is really a
// server falling over rather than a wording difference.
func (s *server) logText() string {
	if s == nil || s.log == nil {
		return ""
	}
	body, err := os.ReadFile(s.log.Name())
	if err != nil {
		return ""
	}
	return string(body)
}

// freePort asks the kernel for a port rather than guessing one.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port
}

// requireCServer skips a test if there is no C server to compare against.
func requireCServer(t *testing.T) {
	t.Helper()
	if cServerErr != nil {
		t.Skipf("the C server is not available, so there is nothing to compare against: %v", cServerErr)
	}
}

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
