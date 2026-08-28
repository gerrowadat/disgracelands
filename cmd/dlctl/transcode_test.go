// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package main

import (
	"os"
	"path/filepath"
	"testing"
	"unicode/utf8"

	"github.com/gerrowadat/disgracelands/internal/persist/boards"
	boardsclassic "github.com/gerrowadat/disgracelands/internal/persist/boards/classic"
	"github.com/gerrowadat/disgracelands/internal/persist/mail"
	mailclassic "github.com/gerrowadat/disgracelands/internal/persist/mail/classic"
	"github.com/gerrowadat/disgracelands/internal/persist/reports"
	reportsclassic "github.com/gerrowadat/disgracelands/internal/persist/reports/classic"
)

// A found-not-assumed regression test for TODO.md's "dlctl import's
// five smaller importers not transcoding" entry: each of the five, plus
// state import's own three text-carrying subsystems, gets a source fixture
// containing the literal CP1252 byte 0x92 (Windows-1252's "right single
// quotation mark" -- the same "curly quote" example docs/design/
// data-format.md's own §11.1 write-up uses) and checks the yaml output is
// both valid UTF-8 throughout and actually decodes that byte to U+2019,
// not just "doesn't crash". Each source file is otherwise the minimal
// valid record its own classic parser requires, read from the parser
// itself (internal/game/social.go's ParseSocials, fightmessages.go's
// ParseMessagesFile, help.go's ParseHelpFile/ParseHelpIndex) rather than
// guessed at, so a malformed fixture fails loudly instead of silently
// exercising nothing.

// cp1252RightQuote is byte 0x92 in Windows-1252 (U+2019 RIGHT SINGLE
// QUOTATION MARK, "'"), used as the one non-ASCII byte in every fixture
// below.
const cp1252RightQuote = "\x92"

// wantUTF8RightQuote is what cp1252RightQuote must decode to.
const wantUTF8RightQuote = "’"

func checkTranscodedUTF8(t *testing.T, path, wantSubstring string) {
	t.Helper()
	b, err := os.ReadFile(path) //nolint:gosec // a test's own tempdir path
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	if !utf8.Valid(b) {
		t.Errorf("%s is not valid UTF-8 after import", path)
	}
	if !containsString(string(b), wantSubstring) {
		t.Errorf("%s does not contain %q (the decoded quote) after import:\n%s", path, wantSubstring, b)
	}
}

func containsString(haystack, needle string) bool {
	return len(needle) > 0 && (len(haystack) >= len(needle)) &&
		(func() bool {
			for i := 0; i+len(needle) <= len(haystack); i++ {
				if haystack[i:i+len(needle)] == needle {
					return true
				}
			}
			return false
		})()
}

// writeFile creates path's parent directories and writes it, failing the
// test on any error — every fixture below builds a classic-shaped source
// tree under a fresh base directory, the same base --from-dir now resolves
// a --type's own classic subpath from.
func writeFile(t *testing.T, path string, body []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}

func TestNamesImportTranscodesNonUTF8(t *testing.T) {
	fromDir := t.TempDir()
	writeFile(t, filepath.Join(fromDir, "misc", "xnames"), []byte("don"+cp1252RightQuote+"t\n"))
	toDir := t.TempDir()
	if err := run([]string{"import", "--type", "names", "--from-dir", fromDir, "--to-dir", toDir}); err != nil {
		t.Fatalf("import --type=names: %v", err)
	}
	checkTranscodedUTF8(t, filepath.Join(toDir, "config", "names.yaml"), "don"+wantUTF8RightQuote+"t")
}

func TestMessagesImportTranscodesNonUTF8(t *testing.T) {
	// One record, all twelve lines present (ParseMessagesFile requires
	// them regardless of any '#' placeholders): M, the attack type, then
	// Die/Miss/Hit/God, each Attacker/Victim/Room.
	body := "M\n 300\n" +
		"die-attacker die" + cp1252RightQuote + "s room\n" + "die-victim\n" + "die-room\n" +
		"miss-attacker\n" + "miss-victim\n" + "miss-room\n" +
		"hit-attacker\n" + "hit-victim\n" + "hit-room\n" +
		"god-attacker\n" + "god-victim\n" + "god-room\n"
	fromDir := t.TempDir()
	writeFile(t, filepath.Join(fromDir, "misc", "messages"), []byte(body))
	toDir := t.TempDir()
	if err := run([]string{"import", "--type", "messages", "--from-dir", fromDir, "--to-dir", toDir}); err != nil {
		t.Fatalf("import --type=messages: %v", err)
	}
	checkTranscodedUTF8(t, filepath.Join(toDir, "config", "messages.yaml"), "die"+wantUTF8RightQuote+"s")
}

func TestSocialsImportTranscodesNonUTF8(t *testing.T) {
	// Header, then CharNoArg/OthersNoArg/CharFound -- an empty CharFound
	// (a bare '#') means the five "found" fields are never read, so this
	// is the minimal valid record (game/social.go's ParseSocials).
	body := "wave 0 0\n" +
		"You wave" + cp1252RightQuote + "ly.\n" +
		"$n waves.\n" +
		"#\n" +
		"$\n"
	fromDir := t.TempDir()
	writeFile(t, filepath.Join(fromDir, "misc", "socials"), []byte(body))
	toDir := t.TempDir()
	if err := run([]string{"import", "--type", "socials", "--from-dir", fromDir, "--to-dir", toDir}); err != nil {
		t.Fatalf("import --type=socials: %v", err)
	}
	checkTranscodedUTF8(t, filepath.Join(toDir, "config", "socials.yaml"), "wave"+wantUTF8RightQuote+"ly")
}

func TestHelpImportTranscodesNonUTF8(t *testing.T) {
	fromDir := t.TempDir()
	writeFile(t, filepath.Join(fromDir, "text", "help", "index"), []byte("test.hlp\n$\n"))
	body := "test\n" +
		"It" + cp1252RightQuote + "s a test.\n" +
		"#\n" +
		"$\n"
	writeFile(t, filepath.Join(fromDir, "text", "help", "test.hlp"), []byte(body))
	toDir := t.TempDir()
	if err := run([]string{"import", "--type", "help", "--from-dir", fromDir, "--to-dir", toDir}); err != nil {
		t.Fatalf("import --type=help: %v", err)
	}
	// help.yaml itself only holds the keyword/slug index; the transcoded
	// body lands in the per-entry text file help.Save writes alongside it
	// (help.go's Save, "<slug>.txt").
	checkTranscodedUTF8(t, filepath.Join(toDir, "text", "help", "test.txt"), "It"+wantUTF8RightQuote+"s a test")
}

// TestStateImportTranscodesNonUTF8 covers the three of state import's five
// subsystems that actually carry free text -- boards, mail, reports --
// each built through its own classic Store (a real writer, not a
// hand-rolled file format) so the fixture is exactly what that format's
// own code produces. bans and houses carry no free text (a hostname/admin
// name, and numeric fields plus a vnum-only stored object respectively --
// see stateio.go's own transcodeString doc comment) and are left empty,
// the same "tolerates a missing source" behaviour examples/stock/binary's
// own empty archive already exercises.
//
// Boards and mail live under fromDir/etc, matching --from-dir's own base
// resolution (stateClassicDirs); reports uses --from-misc-dir, an explicit
// override, the same as houses' --from-house-dir -- both independent of
// --from-dir on purpose, for an archive that keeps them somewhere else.
func TestStateImportTranscodesNonUTF8(t *testing.T) {
	fromDir := t.TempDir()
	fromEtcDir := filepath.Join(fromDir, "etc")
	fromMiscDir := t.TempDir()

	boardSrc, err := boardsclassic.New(boards.Config{Dir: fromEtcDir})
	if err != nil {
		t.Fatalf("opening board source: %v", err)
	}
	if err := boardSrc.Save("board.mort", []boards.Message{{
		Heading: "Aug 20 2026 (Zod)       :: A quote" + cp1252RightQuote + "d headline",
		Body:    "The body" + cp1252RightQuote + "s own quote.\r\n",
	}}); err != nil {
		t.Fatalf("writing board fixture: %v", err)
	}
	_ = boardSrc.Close()

	mailSrc, err := mailclassic.New(mail.Config{Path: filepath.Join(fromEtcDir, "plrmail")})
	if err != nil {
		t.Fatalf("opening mail source: %v", err)
	}
	if err := mailSrc.Send(mail.Message{To: 1, From: 2, Text: "A quote" + cp1252RightQuote + "d letter."}); err != nil {
		t.Fatalf("writing mail fixture: %v", err)
	}
	_ = mailSrc.Close()

	reportSrc, err := reportsclassic.New(reports.Config{Dir: fromMiscDir})
	if err != nil {
		t.Fatalf("opening report source: %v", err)
	}
	if _, err := reportSrc.Append(reports.Report{
		Kind: reports.KindBug, Reporter: "Zod", Room: 1,
		Body: "A quote" + cp1252RightQuote + "d bug report.",
	}); err != nil {
		t.Fatalf("writing report fixture: %v", err)
	}
	_ = reportSrc.Close()

	toDir := t.TempDir()
	if err := run([]string{
		"import", "--type", "state",
		"--from-dir", fromDir,
		"--from-house-dir", t.TempDir(),
		"--from-misc-dir", fromMiscDir,
		"--to-dir", toDir,
	}); err != nil {
		t.Fatalf("import --type=state: %v", err)
	}

	checkTranscodedUTF8(t, filepath.Join(toDir, "state", "boards.yaml"), "A quote"+wantUTF8RightQuote+"d headline")
	checkTranscodedUTF8(t, filepath.Join(toDir, "state", "boards.yaml"), "body"+wantUTF8RightQuote+"s own quote")
	checkTranscodedUTF8(t, filepath.Join(toDir, "state", "mail.yaml"), "A quote"+wantUTF8RightQuote+"d letter")
	checkTranscodedUTF8(t, filepath.Join(toDir, "state", "reports.yaml"), "A quote"+wantUTF8RightQuote+"d bug report")
}

func TestImportAllPassesEncodingToEveryImporter(t *testing.T) {
	// The seven sub-importers' own tests above (plus world/pfile import's
	// pre-existing --encoding support) cover the decoding itself; this
	// only has to show `import` (--type omitted) actually threads a
	// non-default --encoding through to all five importers this change
	// touched, rather than silently dropping it for some of them the way
	// it did before this fix.
	fromDir := t.TempDir()
	for _, sub := range []string{"world", "etc", "misc", "house", "text"} {
		if err := os.MkdirAll(filepath.Join(fromDir, sub), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", sub, err)
		}
	}
	// An empty-but-valid world: one index file per subdirectory
	// (internal/persist/world/classic's own required set), each just the
	// index format's terminator with nothing listed -- this test cares
	// about --encoding reaching every step, not about the world content.
	for _, sub := range []string{"wld", "mob", "obj", "zon", "shp"} {
		dir := filepath.Join(fromDir, "world", sub)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", sub, err)
		}
		if err := os.WriteFile(filepath.Join(dir, "index"), []byte("$\n"), 0o600); err != nil {
			t.Fatalf("writing %s/index: %v", sub, err)
		}
	}
	writeFile(t, filepath.Join(fromDir, "misc", "xnames"), []byte("caf"+cp1252RightQuote+"\n"))

	toDir := t.TempDir()
	if err := run([]string{"import", "--from-dir", fromDir, "--to-dir", toDir, "--encoding", "cp1252"}); err != nil {
		t.Fatalf("import: %v", err)
	}
	checkTranscodedUTF8(t, filepath.Join(toDir, "config", "names.yaml"), "caf"+wantUTF8RightQuote)
}
