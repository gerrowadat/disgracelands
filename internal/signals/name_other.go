// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

//go:build !unix

package signals

import "os"

// platformName has nothing to add off Unix: SIGUSR1 and SIGUSR2 are not
// signal numbers Windows has, so there is nothing there to name. Name's
// own fallback — os.Signal.String — covers whatever the platform does
// define. See name_unix.go.
func platformName(os.Signal) (string, bool) { return "", false }
