// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

// Package buildinfo carries version information stamped in at link time.
//
// The values are set with -ldflags at build time; when built without them
// (go run, go test, go install) they fall back to whatever the Go toolchain
// recorded in the binary's embedded VCS stamp, so a locally built binary still
// reports something truthful rather than "unknown".
package buildinfo

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"
)

// Stamped by -ldflags -X. See build/Dockerfile and .github/workflows/ci.yml.
var (
	version = ""
	commit  = ""
	date    = ""
)

// Info describes the running binary.
type Info struct {
	Version   string
	Commit    string
	Date      string
	GoVersion string
	Dirty     bool
}

// Get returns the build information, filling gaps from the embedded VCS stamp.
func Get() Info {
	i := Info{
		Version:   version,
		Commit:    commit,
		Date:      date,
		GoVersion: runtime.Version(),
	}

	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, s := range bi.Settings {
			switch s.Key {
			case "vcs.revision":
				if i.Commit == "" {
					i.Commit = s.Value
				}
			case "vcs.time":
				if i.Date == "" {
					i.Date = s.Value
				}
			case "vcs.modified":
				i.Dirty = s.Value == "true"
			}
		}
	}

	if i.Version == "" {
		i.Version = "devel"
	}
	if i.Commit == "" {
		i.Commit = "unknown"
	}
	return i
}

// ShortCommit returns the commit abbreviated to the usual seven characters.
func (i Info) ShortCommit() string {
	if len(i.Commit) > 7 {
		return i.Commit[:7]
	}
	return i.Commit
}

// String renders a one-line summary suitable for --version.
func (i Info) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "dlmud %s (%s", i.Version, i.ShortCommit())
	if i.Dirty {
		b.WriteString("-dirty")
	}
	if i.Date != "" {
		fmt.Fprintf(&b, ", %s", i.Date)
	}
	fmt.Fprintf(&b, ") %s", i.GoVersion)
	return b.String()
}
