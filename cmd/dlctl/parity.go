// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gerrowadat/disgracelands/internal/parity"
)

// cmdParitySession plays one script against both servers and diffs what they
// said.
//
// It is a `dlctl` subcommand rather than a shell script because the driving is
// the fiddly part — framing a command's output by silence, and normalising
// away the handful of things two servers can never agree on. The shell wrapper
// (scripts/session-parity.sh) boots the two servers and calls this.
func cmdParitySession(args []string) error {
	fs := flag.NewFlagSet("parity session", flag.ContinueOnError)
	script := fs.String("script", "", "session script to play (required)")
	cAddr := fs.String("c-addr", "", "host:port of the C server (required)")
	goAddr := fs.String("go-addr", "", "host:port of the Go server (required)")
	outDir := fs.String("out-dir", "", "write both transcripts here, for reading after a failure")
	quiet := fs.Duration("quiet", 400*time.Millisecond,
		"how long a server must be silent before its answer is complete")
	ignoreColour := fs.Bool("ignore-colour", false,
		"strip ANSI colour before comparing, to see past the one known systematic difference")
	fs.Usage = func() {
		_, _ = fmt.Fprint(fs.Output(),
			"Usage: dlctl parity session --script=FILE --c-addr=HOST:PORT --go-addr=HOST:PORT\n\n"+
				"Plays the script against both servers and diffs the transcripts.\n"+
				"Exits non-zero if they differ. The C server is the reference: where\n"+
				"they disagree, the Go server is what is wrong.\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *script == "" || *cAddr == "" || *goAddr == "" {
		fs.Usage()
		return fmt.Errorf("--script, --c-addr and --go-addr are all required")
	}

	body, err := os.ReadFile(*script) //nolint:gosec // a path the operator gave us
	if err != nil {
		return fmt.Errorf("reading %s: %w", *script, err)
	}
	lines := parity.ParseScript(string(body))
	if len(lines) == 0 {
		return fmt.Errorf("%s has nothing in it to type", *script)
	}

	// Sequentially rather than together: two servers sharing a machine, each
	// framing by silence, are quieter and steadier one at a time — and the
	// scripts create a character, so running both at once against one roster
	// would be a different test anyway.
	cText, err := parity.Run(lines, parity.Options{Addr: *cAddr, Quiet: *quiet})
	if err != nil {
		return fmt.Errorf("playing against the C server: %w", err)
	}
	goText, err := parity.Run(lines, parity.Options{Addr: *goAddr, Quiet: *quiet})
	if err != nil {
		return fmt.Errorf("playing against the Go server: %w", err)
	}

	cNorm, goNorm := parity.Normalise(cText), parity.Normalise(goText)
	if *ignoreColour {
		cNorm, goNorm = parity.StripColour(cNorm), parity.StripColour(goNorm)
	}

	if *outDir != "" {
		if err := os.MkdirAll(*outDir, 0o755); err != nil { //nolint:gosec // a scratch directory
			return err
		}
		for name, text := range map[string]string{
			"c.raw": cText, "go.raw": goText,
			"c.normalised": cNorm, "go.normalised": goNorm,
		} {
			if err := os.WriteFile(filepath.Join(*outDir, name), []byte(text), 0o600); err != nil {
				return err
			}
		}
	}

	if cNorm == goNorm {
		fmt.Printf("    identical (%d lines)\n", strings.Count(cNorm, "\n"))
		return nil
	}

	diff := parity.Diff(cNorm, goNorm)
	fmt.Printf("    %d differing line(s)\n\n%s\n", strings.Count(diff, "\n-")+strings.Count(diff, "\n+"), diff)
	if *outDir != "" {
		fmt.Printf("    transcripts in %s\n", *outDir)
	}
	return errQuiet
}
