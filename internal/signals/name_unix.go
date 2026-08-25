// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

//go:build unix

package signals

import (
	"os"
	"syscall"
)

// platformName spells the signals that only exist on Unix.
//
// SIGUSR1 and SIGUSR2 are the two the C traps that have no counterpart in
// Go's Windows syscall package — there is no such signal number to name,
// which is a different thing from a signal that exists and never fires.
// SIGHUP is the latter: Windows defines it, nothing ever delivers it, and
// Name spells it in the portable table either way.
//
// docs/design/signal-handling.md §3 is what these two will do when they do
// anything; today they are names and nothing else.
func platformName(sig os.Signal) (string, bool) {
	switch sig {
	case syscall.SIGUSR1:
		return "SIGUSR1", true
	case syscall.SIGUSR2:
		return "SIGUSR2", true
	}
	return "", false
}
