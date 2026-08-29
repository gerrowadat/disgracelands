// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package main

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gerrowadat/disgracelands/internal/persist/convert"
)

// The compatibility suite docs/proposals/yaml-only.md §5.2 asks for:
// differential, idempotence and stability, over all three corpora.
//
// Stability — a fresh import equals the checked-in yaml, byte for byte —
// is import_test.go's TestImportMatchesTheCheckedInExamples, which already
// covered stock and mini and now covers torture too. The other two are
// here.
//
// **These are ordinary Go tests, not shell scripts**, and that is a scope
// decision rather than a style one. `go test -race ./...` already runs on
// every push and PR; a shell script would have to be added to `go.yml`,
// which CLAUDE.md explicitly forbids. Writing the suite in Go is how it
// gets day-to-day coverage without amending the workflow — the scope rule
// and the testing goal point the same way, which is worth noticing rather
// than treating as an obstacle to route around.
//
// They are also cheap to write precisely because `dlctl verify --against`
// exists as a command first: a differential test is that command's own
// comparison, called directly, asserting on the difference list instead of
// on stdout. One implementation, two callers.

// corpora is every checked-in binary/yaml pair, in the order they cost:
// mini is a tutorial zone, stock is the real 30-zone world, torture is the
// hostile one.
var corpora = libImportFixtures

// TestEveryCorpusConvertsWithNoLoss is the differential half of §5.2: for
// every subsystem of every corpus, load the legacy directory through its
// driver, load the imported yaml through its driver, and deep-compare.
//
// This is the check the whole release rests on. §4.1's chain of evidence
// has two links per side — the C loader against the Go classic loader
// (scripts/world-parity.sh, and the oracles), and the Go classic loader
// against the Go yaml loader — and transitivity gives "a server running on
// the converted data behaves identically to one running on the original".
// This test is the second link, for every subsystem rather than just the
// world.
//
// It compares the *checked-in* yaml rather than a fresh conversion on
// purpose: a fresh conversion is what the stability test covers, and
// comparing the committed bytes is what catches a checked-in corpus that
// has drifted from its source for any reason at all, including somebody
// editing it by hand.
func TestEveryCorpusConvertsWithNoLoss(t *testing.T) {
	enc := convert.Encodings[convert.DefaultEncoding]
	for _, fx := range corpora {
		t.Run(fx.name, func(t *testing.T) {
			for _, ty := range allTypes {
				t.Run(string(ty), func(t *testing.T) {
					left := loadOptions{base: fx.binaryDir, format: defaultFormat(ty, ""), enc: enc}
					right := loadOptions{base: fx.yamlDir, format: "yaml", enc: enc}
					diffs, err := compareSubsystem(ty, left, right)
					if err != nil {
						t.Fatalf("comparing: %v", err)
					}
					for _, d := range diffs {
						t.Errorf("%s", d)
					}
				})
			}
		})
	}
}

// TestFmtIsIdempotent is the idempotence half: `dlctl fmt` on an
// already-canonical directory must not change a byte.
//
// It is true of several subsystems already and asserted for none, which is
// the state a canonical-writer requirement (docs/design/data-format.md
// §10.3) should never be left in — the whole point of a canonical writer
// is that running it twice is a no-op, and "we believe it is" is not the
// same claim.
//
// Two rounds, not one: the first `fmt` is allowed to change the checked-in
// bytes (it would catch a corpus that was written by an older writer), and
// the second must not change the first's output. That is what idempotence
// actually means, and testing only the first round would instead be
// testing stability, which is a different property and already covered.
func TestFmtIsIdempotent(t *testing.T) {
	for _, fx := range corpora {
		t.Run(fx.name, func(t *testing.T) {
			for _, ty := range allTypes {
				t.Run(string(ty), func(t *testing.T) {
					dir := t.TempDir()
					copyTree(t, fx.yamlDir, dir)

					if err := run([]string{"fmt", "--type", string(ty), "--dir", dir}); err != nil {
						t.Fatalf("first fmt: %v", err)
					}
					first := treeBytes(t, dir)

					if err := run([]string{"fmt", "--type", string(ty), "--dir", dir}); err != nil {
						t.Fatalf("second fmt: %v", err)
					}
					second := treeBytes(t, dir)

					for path, want := range first {
						got, ok := second[path]
						switch {
						case !ok:
							t.Errorf("%s: the second fmt removed it", path)
						case !bytes.Equal(got, want):
							t.Errorf("%s: the second fmt changed it (%d bytes vs %d)", path, len(got), len(want))
						}
					}
					for path := range second {
						if _, ok := first[path]; !ok {
							t.Errorf("%s: the second fmt created it", path)
						}
					}
				})
			}
		})
	}
}

// TestImportVerifiesItsOwnOutput is `import --verify`, which is on by
// default: a conversion that does not load back to the same state is a
// failed conversion, not a successful one with a caveat.
func TestImportVerifiesItsOwnOutput(t *testing.T) {
	for _, fx := range corpora {
		t.Run(fx.name, func(t *testing.T) {
			to := t.TempDir()
			if err := run([]string{"import", "--from-dir", fx.binaryDir, "--to-dir", to}); err != nil {
				t.Fatalf("import with --verify on: %v", err)
			}
		})
	}
}

// TestVerifyAgainstNoticesAnEditedFile is the test the three above need to
// be worth anything: a comparison that reports "identical" for everything
// is only reassuring if it can be made to fail.
//
// It changes one byte of one player's title in a converted directory and
// requires the comparison to name it — a whole-file corruption would be
// caught by almost anything, whereas a single edited field is the shape of
// the losses this suite actually exists to find.
func TestVerifyAgainstNoticesAnEditedFile(t *testing.T) {
	const torture = "../../examples/torture"
	to := t.TempDir()
	if err := run([]string{"import", "--from-dir", torture + "/binary", "--to-dir", to}); err != nil {
		t.Fatalf("import: %v", err)
	}

	path := filepath.Join(to, "players", "t", "torturer.yaml")
	body, err := os.ReadFile(path) //nolint:gosec // a path this test just created
	if err != nil {
		t.Fatalf("reading the converted character: %v", err)
	}
	edited := bytes.Replace(body, []byte("title: xxxx"), []byte("title: yxxx"), 1)
	if bytes.Equal(edited, body) {
		t.Fatal("the title this test edits is no longer in the file; pick another field")
	}
	if err := os.WriteFile(path, edited, 0o600); err != nil {
		t.Fatalf("writing the edit: %v", err)
	}

	enc := convert.Encodings[convert.DefaultEncoding]
	diffs, err := compareSubsystem(typePfile,
		loadOptions{base: torture + "/binary", format: defaultFormat(typePfile, ""), enc: enc},
		loadOptions{base: to, format: "yaml", enc: enc})
	if err != nil {
		t.Fatalf("comparing: %v", err)
	}
	if len(diffs) != 1 || !strings.Contains(diffs[0], "Title") {
		t.Errorf("comparing an edited title reported %v, want one difference naming Title", diffs)
	}
}

// TestAnOrphanedHouseContentsFileIsNamedAndNoted covers both halves of
// #239 on the corpus built to have the case: examples/torture's
// house/5006.house has contents and etc/hcontrol does not mention 5006.
//
// Before this, import dropped it without a word and `verify --against`
// reported the conversion identical — the check built to catch a silent
// loss enumerated houses from the control records exactly like the
// importer it was checking. Both are asserted here because either alone
// would leave the other blind.
func TestAnOrphanedHouseContentsFileIsNamedAndNoted(t *testing.T) {
	const torture = "../../examples/torture"
	to := t.TempDir()

	var report bytes.Buffer
	out := bufio.NewWriter(&report)
	etcDir, houseDir, _ := stateClassicDirs(torture + "/binary")
	if err := importHouses(etcDir, houseDir, to, out); err != nil {
		t.Fatalf("importing houses: %v", err)
	}
	if err := out.Flush(); err != nil {
		t.Fatal(err)
	}
	if got := report.String(); !strings.Contains(got, "#5006: 1 object(s)") {
		t.Errorf("import did not name the dropped contents file:\n%s", got)
	}

	// And the comparison says so too — beside the verdict rather than in
	// it, because nothing ever reads an orphan and the two directories do
	// load to the same state. orphanHouseContents' doc comment has the
	// argument.
	enc := convert.Encodings[convert.DefaultEncoding]
	full := t.TempDir()
	if err := run([]string{"import", "--from-dir", torture + "/binary", "--to-dir", full}); err != nil {
		t.Fatalf("import: %v", err)
	}
	var note bytes.Buffer
	noteOut := bufio.NewWriter(&note)
	reportOrphanHouses(noteOut,
		loadOptions{base: torture + "/binary", format: defaultFormat(typeState, ""), enc: enc},
		loadOptions{base: full, format: "yaml", enc: enc})
	if err := noteOut.Flush(); err != nil {
		t.Fatal(err)
	}
	if got := note.String(); !strings.Contains(got, "#5006 (1 object(s))") {
		t.Errorf("verify said nothing about the orphaned contents file:\n%s", got)
	}
	// The converted directory has none of its own: yaml cannot hold one.
	if got := note.String(); strings.Contains(got, full) {
		t.Errorf("the yaml side was reported as holding an orphan, which it cannot:\n%s", got)
	}
}

// copyTree copies a directory tree, so a test can run `fmt` in place
// without editing the checked-in corpus.
func copyTree(t *testing.T, from, to string) {
	t.Helper()
	err := filepath.WalkDir(from, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(from, path)
		if err != nil {
			return err
		}
		dst := filepath.Join(to, rel)
		if d.IsDir() {
			return os.MkdirAll(dst, 0o750)
		}
		body, err := os.ReadFile(path) //nolint:gosec // a fixture directory in this repository
		if err != nil {
			return err
		}
		return os.WriteFile(dst, body, 0o600)
	})
	if err != nil {
		t.Fatalf("copying %s: %v", from, err)
	}
}

// treeBytes reads every regular file under dir, keyed by relative path.
func treeBytes(t *testing.T, dir string) map[string][]byte {
	t.Helper()
	out := map[string][]byte{}
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		body, err := os.ReadFile(path) //nolint:gosec // a directory this test created
		if err != nil {
			return err
		}
		out[filepath.ToSlash(rel)] = body
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", dir, err)
	}
	return out
}
