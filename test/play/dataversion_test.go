// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

//go:build play

package play

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The .dlversion check (docs/design/data-format-versioning.md), exercised
// where it actually runs: in a released server binary's boot sequence,
// before any store opens.
//
// internal/persist/dataversion's own tests cover the comparison, and they
// have to do it against a hand-made version, because a test binary has no
// release of its own. What they cannot cover is that cmd/dlmud reaches the
// check at all, reaches it early enough, and turns its two outcomes into
// the two things an operator sees — a process that exits with something to
// read, or a process that starts and says so in the log. That is a boot
// sequence question, which makes it this suite's.
//
// This suite's server is built with a fixed release version (harness_test
// .go's serverVersion), so "a differing major" and "a differing minor" are
// stamps this test can write.

// stamp writes a .dlversion into a staged lib directory. The shape is
// fixed by dataversion's own writer; this deliberately writes the bytes
// rather than calling Write, so that a change to the file's shape shows up
// here as a boot failure the way it would for an operator with an old
// directory, rather than being quietly rewritten by the code under test.
func stamp(t *testing.T, dir, version string) {
	t.Helper()
	body := "schema: dl/dataversion@1\nformat: yaml\nversion: " + version + "\n"
	if err := os.WriteFile(filepath.Join(dir, ".dlversion"), []byte(body), 0o600); err != nil {
		t.Fatalf("stamping %s: %v", dir, err)
	}
}

// TestTheServerRefusesToBootOnAnotherMajorVersionsData is the consequence
// that matters: 7.3.1 will not open a directory 8.x wrote, and the operator
// finds out from a failed start with a message naming both versions, not
// from a half-loaded world.
func TestTheServerRefusesToBootOnAnotherMajorVersionsData(t *testing.T) {
	dir := stageLib(t, miniClassic)
	stamp(t, dir, "8.0.0")

	out, err := runServer(t, "--lib-dir="+dir, "--listen-telnet=127.0.0.1:0",
		"--listen-telnets=", "--metrics-addr=")
	if err == nil {
		t.Fatalf("the server booted on major-8 data; it said:\n%s", out)
	}
	for _, want := range []string{"8.0.0", "7.3.1", ".dlversion"} {
		if !strings.Contains(out, want) {
			t.Errorf("the refusal does not mention %q:\n%s", want, out)
		}
	}
	t.Logf("refused major-8 data with:\n%s", out)
}

// ... and in the other direction, which is the half that is easy to get
// wrong. A server does not get to assume that data older than itself is a
// subset of what it understands; across a major bump it is a different
// agreement about what the files mean.
func TestTheServerRefusesToBootOnAnOlderMajorVersionsData(t *testing.T) {
	dir := stageLib(t, miniClassic)
	stamp(t, dir, "6.9.9")

	out, err := runServer(t, "--lib-dir="+dir, "--listen-telnet=127.0.0.1:0",
		"--listen-telnets=", "--metrics-addr=")
	if err == nil {
		t.Fatalf("the server booted on major-6 data; it said:\n%s", out)
	}
	if !strings.Contains(out, "6.9.9") {
		t.Errorf("the refusal does not name the data's version:\n%s", out)
	}
}

// A differing minor is the other outcome, and the assertion is that it is
// *not* a refusal: the server warns and then plays. Booting it for real,
// and typing something, is the point — a check that turned a warning into
// a fatal error would be caught by the first half of this alone, but one
// that logged the warning and then left the world half-loaded would not.
func TestADifferingMinorVersionWarnsAndPlaysAnyway(t *testing.T) {
	dir := stageLib(t, miniClassic)
	stamp(t, dir, "7.9.0")

	m := startAt(t, miniClassic, dir, startOptions{})
	c := m.dial()
	c.create("Minorwarn", "tourpass", "m", "w")
	contains(t, "the world still loaded", c.do("look"), "The Testing Grounds")

	line, ok := logContaining(m, "7.9.0")
	if !ok {
		t.Fatalf("no warning naming the directory's version was logged:\n%s", m.logText())
	}
	if line.Severity != "WARN" {
		t.Errorf("the minor-version mismatch was logged at %s, want WARN: %s", line.Severity, line.raw)
	}
	// serverVersion is "vX.Y.Z"; the message names the release without it.
	if !strings.Contains(line.Msg, strings.TrimPrefix(serverVersion, "v")) {
		t.Errorf("the warning does not name the server's own version: %s", line.raw)
	}

	m.noServerErrors()
}

// The ordinary case, asserted rather than assumed: the checked-in example
// directories carry no stamp, so every other test in this suite boots
// through a check that finds nothing and says nothing. If that ever stops
// being true, the silence the rest of the suite relies on has gone.
func TestAnUnstampedDirectoryIsSilent(t *testing.T) {
	m := start(t, miniClassic)
	if _, err := os.Stat(filepath.Join(m.dir, ".dlversion")); !os.IsNotExist(err) {
		t.Fatalf("%s is stamped after all, so this test proves nothing: %v", miniClassic.dir, err)
	}
	if line, ok := logContaining(m, ".dlversion"); ok {
		t.Errorf("an unstamped directory logged something about the stamp: %s", line.raw)
	}
	m.noServerErrors()
}

// logContaining is find for a message this test only knows part of. The
// version-check messages name the directory, so they are different in every
// run and cannot be matched whole.
func logContaining(m *mud, want string) (logLine, bool) {
	for _, raw := range strings.Split(m.logText(), "\n") {
		if strings.Contains(raw, want) {
			return logLine{Severity: severityOf(raw), Msg: raw, raw: raw}, true
		}
	}
	return logLine{}, false
}

// severityOf pulls the level out of a raw JSON log line without decoding
// it, which is all logContaining's callers need. The field is
// `severity_text` rather than `level` because the server's JSON format is
// OpenTelemetry-shaped (internal/obs/otel.go), the same reason logLine
// itself names it that way.
func severityOf(raw string) string {
	for _, l := range []string{"ERROR", "WARN", "INFO", "DEBUG"} {
		if strings.Contains(raw, `"severity_text":"`+l+`"`) {
			return l
		}
	}
	return ""
}
