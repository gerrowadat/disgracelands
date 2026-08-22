// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gerrowadat/disgracelands/internal/persist/socials"
)

// End to end: LoadText(dir, ..., "native") reads config/socials.yaml, not
// misc/socials, and the real archive's "smile" entry comes back with its
// real message — proving the wiring (internal/server/text.go's own
// socials.Load call, the --socials-format flag it is fed from in
// cmd/dlmud/main.go), not just internal/persist/socials' own codec
// (already covered by its own real-archive round-trip test).
func TestNativeSocialsFormatEndToEnd(t *testing.T) {
	classic, err := socials.Load("classic", "../../data/misc/socials")
	if err != nil {
		t.Fatalf("Load(classic): %v", err)
	}

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "text"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		greetingFile: testGreeting,
		creditsFile:  testCredits,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := socials.Save("native", filepath.Join(dir, "config"), classic); err != nil {
		t.Fatalf("Save(native): %v", err)
	}

	text, err := LoadText(dir, "classic", "native", "classic")
	if err != nil {
		t.Fatalf("LoadText: %v", err)
	}

	var found bool
	for _, s := range text.Socials() {
		if s.Name != "smile" {
			continue
		}
		found = true
		if s.CharNoArg != "You smile happily." {
			t.Errorf("smile.CharNoArg = %q, want %q", s.CharNoArg, "You smile happily.")
		}
	}
	if !found {
		t.Error(`LoadText(dir, ..., "native") found no "smile" social, want the real archive's`)
	}
}
