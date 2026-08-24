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
	"syscall"
	"testing"
	"time"
)

// What the signals do, tested against a real process because there is no
// other kind of test a signal has. internal/signals proves the dispatcher
// delivers to a handler; only this proves the handler is wired to something
// a player can feel.
//
// docs/design/signal-handling.md is the design. The shutdown signals live
// in shutdown_test.go, next to what they have to save.

// writeConfig writes a game-tuning file and returns its path, overwriting
// whatever was there. Called more than once in a test on purpose: the file
// changing under a running server is the thing being tested.
func writeConfig(t *testing.T, path, body string) string {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
	return path
}

// waitForLog blocks until the server has logged a message, and fails if it
// does not. A signal is asynchronous by nature -- the handler runs on the
// dispatcher's goroutine, not the test's -- so there is nothing on the
// socket to synchronize with until it has happened.
func waitForLog(t *testing.T, m *mud, msg string) {
	t.Helper()
	if !eventually(10*time.Second, func() bool {
		_, ok := m.find(msg)
		return ok
	}) {
		t.Fatalf("the server never logged %q. Its log was:\n%s", msg, m.logText())
	}
}

// TestSIGHUPReloadsTheConfiguration.
//
// SIGHUP means "re-read your configuration" here, which is the conventional
// Unix meaning and not the C's: hupsig treats SIGHUP, SIGINT and SIGTERM
// alike and exits(1) on all three, saving nobody (comm.c:2120).
//
// The effect is checked from the socket rather than from the log, because
// "the file was re-read" and "the game changed" are different claims and
// only the second one matters. level_can_shout is the one to check it with:
// it is a config.c value (config.c:61), it gates a command a mortal can
// type, and the answer is visible in one line.
func TestSIGHUPReloadsTheConfiguration(t *testing.T) {
	cfg := writeConfig(t, filepath.Join(t.TempDir(), "game.yaml"), "level_can_shout: 30\n")

	m := start(t, miniClassic, startOptions{extraFlags: []string{"--config=" + cfg}})
	c := m.dial()
	c.create("Yeller", "yellpass", "m", "w")

	contains(t, "shouting before the reload", c.do("shout hello"),
		"You must be at least level 30 before you can shout")

	writeConfig(t, cfg, "level_can_shout: 1\n")
	m.signal(syscall.SIGHUP)
	waitForLog(t, m, "SIGHUP: reloaded game tuning")

	contains(t, "shouting after the reload", c.do("shout hello"), "You shout, 'hello'")

	m.noServerErrors()
}

// TestSIGHUPKeepsTheOldConfigurationWhenTheNewOneIsBroken.
//
// The rule a reload lives by: it may fail, and failing may not take the game
// with it (docs/design/signal-handling.md §2). A typo in a file an
// operator is editing on a live server must cost them the reload and nothing
// else -- not the players who are logged in, and not the values that were
// already working.
func TestSIGHUPKeepsTheOldConfigurationWhenTheNewOneIsBroken(t *testing.T) {
	cfg := writeConfig(t, filepath.Join(t.TempDir(), "game.yaml"), "level_can_shout: 30\n")

	m := start(t, miniClassic, startOptions{extraFlags: []string{"--config=" + cfg}})
	c := m.dial()
	c.create("Yeller", "yellpass", "m", "w")

	writeConfig(t, cfg, "level_can_shout: [this is not a number]\n")
	m.signal(syscall.SIGHUP)
	waitForLog(t, m, "SIGHUP: game tuning reload failed, keeping previous values")

	// Still playing, and still on the values it booted with.
	contains(t, "the game after a failed reload", c.do("shout hello"),
		"You must be at least level 30 before you can shout")
	contains(t, "the world after a failed reload", c.do("exits"), "Obvious exits:")

	// A reload that refused a file is an ERROR, and the only one this test
	// should produce -- so noServerErrors would be wrong here, and saying
	// nothing about the log would be worse.
	errs := m.errorLines()
	if len(errs) != 1 {
		t.Errorf("expected exactly one logged error, the refused reload; got %d:\n%s",
			len(errs), strings.Join(errs, "\n"))
	}
}

// TestSIGHUPWithNoConfigFileSaysSo. There is nothing to re-read when
// --config was never set, and an operator who signals a server that has no
// configuration file has made a mistake worth telling them about rather than
// silently doing nothing.
func TestSIGHUPWithNoConfigFileSaysSo(t *testing.T) {
	m := start(t, miniClassic)

	m.signal(syscall.SIGHUP)
	waitForLog(t, m, "SIGHUP received but no --config is set; nothing to reload")

	// And it is a warning, not a failure: the server plays on.
	c := m.dial()
	c.create("Bystander", "bystandpass", "m", "w")
	contains(t, "the game after a no-op reload", c.do("exits"), "Obvious exits:")

	m.noServerErrors()
}
