// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gerrowadat/disgracelands/internal/persist/help"
)

// End to end: LoadText(dir, ..., "native") reads text/help/help.yaml plus
// one .txt file per entry, not text/help/index plus the .hlp files, and
// the real archive's CIRCLEMUD credits entry — the licence-obligation
// lookup TestCreditsAndHelpCircleMUD already proves for classic — comes
// back the same way through it, proving the wiring (internal/server/
// text.go's own help.Load call, the --help-format flag it is fed from in
// cmd/dlmud/main.go), not just internal/persist/help's own codec
// (already covered by its own real-archive round-trip test).
func TestNativeHelpFormatEndToEnd(t *testing.T) {
	classic, err := help.Load("classic", "../../data/text/help")
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
	if err := os.MkdirAll(filepath.Join(dir, helpDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := help.Save("native", filepath.Join(dir, helpDir), classic); err != nil {
		t.Fatalf("Save(native): %v", err)
	}

	text, err := LoadText(dir, "classic", "classic", "native")
	if err != nil {
		t.Fatalf("LoadText: %v", err)
	}

	const wantCredits = "CircleMUD was developed from DikuMud"
	for _, query := range []string{"circlemud", "credits", "circle"} {
		body, ok := text.Help(query)
		if !ok {
			t.Errorf("Help(%q) found nothing", query)
			continue
		}
		if !strings.Contains(body, wantCredits) {
			t.Errorf("Help(%q) = %q, want it to contain %q", query, body, wantCredits)
		}
	}
}
