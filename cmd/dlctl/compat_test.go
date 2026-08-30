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

// The compatibility suite docs/design/yaml-only.md §5.2 asks for:
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
					left := loadOptions{base: fx.binaryDir, format: defaultFormat(ty, "", fx.binaryDir), enc: enc}
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

// TestImportIsIdempotent is the other idempotence claim, and the one that
// was missing: `dlctl import` run twice into the same --to-dir must leave
// exactly what running it once did.
//
// §5.2's idempotence check was TestFmtIsIdempotent alone, and `import`'s
// own checks all convert into a `t.TempDir()` — a destination that is
// empty by construction. So the case where the destination already has
// content was designed out of the suite, in the same shape as
// examples/stock being pure ASCII (data-format.md §11.1) and
// examples/torture having one zone (#285): every input was one over which
// the right behaviour and the wrong one cannot disagree.
//
// They disagreed as soon as anything asked. Mail and reports were stored
// through Send and Append — the *game's* "one more has arrived" calls,
// which is all their stores offered — so a second import appended the
// whole lot again, silently, while the other five subsystems replaced
// theirs (#293). examples/torture/README.md's own "Reproducing it"
// instructions run exactly this sequence against the checked-in corpus,
// so following them committed every message and every report twice.
//
// Three rounds rather than two: doubling shows up in the second, but an
// off-by-one that only appears once a list is non-empty on entry would
// not, and the second round is the first one whose destination is not
// empty.
func TestImportIsIdempotent(t *testing.T) {
	for _, fx := range corpora {
		t.Run(fx.name, func(t *testing.T) {
			dir := t.TempDir()

			if err := run([]string{"import", "--from-dir", fx.binaryDir, "--to-dir", dir}); err != nil {
				t.Fatalf("first import: %v", err)
			}
			first := treeBytes(t, dir)

			for round := 2; round <= 3; round++ {
				// A round that is not idempotent usually fails here
				// rather than below: `import --verify` compares the
				// source against what is now in --to-dir, so a doubled
				// list is a difference it already knows how to see. It
				// reports through errQuiet, whose message is empty by
				// design (main.go) because the command has already
				// printed the detail to stdout.
				if err := run([]string{"import", "--from-dir", fx.binaryDir, "--to-dir", dir}); err != nil {
					t.Fatalf("import round %d failed; see its own output above for what differs (%v)", round, err)
				}
				again := treeBytes(t, dir)

				for path, want := range first {
					got, ok := again[path]
					switch {
					case !ok:
						t.Errorf("round %d removed %s", round, path)
					case !bytes.Equal(got, want):
						t.Errorf("round %d changed %s (%d bytes vs %d)", round, path, len(got), len(want))
					}
				}
				for path := range again {
					if _, ok := first[path]; !ok {
						t.Errorf("round %d created %s", round, path)
					}
				}
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
		loadOptions{base: torture + "/binary", format: defaultFormat(typePfile, "", torture+"/binary"), enc: enc},
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
		loadOptions{base: torture + "/binary", format: defaultFormat(typeState, "", torture+"/binary"), enc: enc},
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

// TestOrphanedRentAndAliasFilesAreNamedAndNoted is #287 on the corpus built
// to have the case: examples/torture holds plrobjs/F-J/ghost.objs and
// plralias/F-J/ghost.alias for a Ghost who is not on the roster, and a
// sixty-byte plrobjs/F-J/00 that is not a per-character file at all.
//
// Before this, import carried none of them across and said nothing: its
// loop is driven by the roster and asks for each character's files by
// name, so a file belonging to nobody was never opened, and the summary
// line counted what it found rather than what was there. `verify
// --against` was blind for the same reason, enumerating characters from
// the roster on both sides exactly like the importer it exists to check.
// Both halves are asserted here, because either alone would leave the
// other blind -- the same argument as the house-contents test above.
func TestOrphanedRentAndAliasFilesAreNamedAndNoted(t *testing.T) {
	const torture = "../../examples/torture"
	to := t.TempDir()

	var report bytes.Buffer
	if err := importPfile(importOptions{
		fromDir: torture + "/binary", toDir: to, encName: convert.DefaultEncoding,
	}, &report); err != nil {
		t.Fatalf("import: %v\n%s", err, report.String())
	}

	got := report.String()
	for _, want := range []string{
		"plrobjs: dropped 1 rent file(s)",
		"plralias: dropped 1 alias file(s)",
		"ghost",
		"F-J/00",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("import did not name what it could not carry across (%q missing):\n%s", want, got)
		}
	}

	// And the comparison says so too, beside the verdict rather than in
	// it: nothing reads an orphan, so the two directories do load to the
	// same state, and making it a difference would stop import -- which
	// verifies itself -- converting any archive that has one.
	enc := convert.Encodings[convert.DefaultEncoding]
	var note bytes.Buffer
	noteOut := bufio.NewWriter(&note)
	reportOrphanPlayerFiles(noteOut,
		loadOptions{base: torture + "/binary", format: defaultFormat(typePfile, "", torture+"/binary"), enc: enc},
		loadOptions{base: to, format: "yaml", enc: enc})
	if err := noteOut.Flush(); err != nil {
		t.Fatal(err)
	}
	if got := note.String(); !strings.Contains(got, "ghost.alias, ghost.objs") {
		t.Errorf("verify said nothing about the orphaned rent/alias files:\n%s", got)
	}
	if got := note.String(); !strings.Contains(got, "F-J/00") {
		t.Errorf("verify said nothing about the file that is not a per-character file:\n%s", got)
	}
	// The converted directory has none of its own, and cannot: a
	// character's rent file and aliases are fields of their own document.
	if got := note.String(); strings.Contains(got, to) {
		t.Errorf("the yaml side was reported as holding an orphan, which it cannot:\n%s", got)
	}
}

// TestADuplicateHouseRecordIsCollapsedAndCounted is #240: two control
// records for the same room used to collapse in silence, and the summary
// line reported the number of records *read* rather than the number
// stored, so the output actively said the wrong thing — "imported 4
// house(s)" for three that arrived.
//
// The duplicate is built here rather than committed to
// examples/torture/binary, because a corpus with one could not also be the
// corpus every other test round-trips: the collapse is exactly the case
// where a conversion is not reversible.
func TestADuplicateHouseRecordIsCollapsedAndCounted(t *testing.T) {
	const torture = "../../examples/torture"
	from := t.TempDir()
	copyTree(t, torture+"/binary", from)

	// One hcontrol record is 100 bytes in the ILP32 layout the archive
	// uses; appending a copy of the first is a duplicate of whatever room
	// it names.
	etcDir, houseDir, _ := stateClassicDirs(from)
	control := filepath.Join(etcDir, "hcontrol")
	body, err := os.ReadFile(control) //nolint:gosec // a path this test just created
	if err != nil {
		t.Fatalf("reading the control file: %v", err)
	}
	const recordSize = 100
	if len(body) < recordSize {
		t.Fatalf("the corpus's control file is %d bytes, too short to duplicate a record from", len(body))
	}
	if err := os.WriteFile(control, append(body, body[:recordSize]...), 0o600); err != nil {
		t.Fatalf("writing the duplicated control file: %v", err)
	}

	var report bytes.Buffer
	out := bufio.NewWriter(&report)
	if err := importHouses(etcDir, houseDir, t.TempDir(), out); err != nil {
		t.Fatalf("importing houses: %v", err)
	}
	if err := out.Flush(); err != nil {
		t.Fatal(err)
	}

	got := report.String()
	// Three records went in as four. The count has to be the three that
	// arrived, and the object count must not double either — the
	// duplicate's contents are the same file read twice.
	if !strings.Contains(got, "houses: imported 3 house(s), 3 object(s)") {
		t.Errorf("the summary reports the records read rather than the houses stored:\n%s", got)
	}
	if !strings.Contains(got, "collapsed 1 duplicate control record(s)") {
		t.Errorf("the collapse was not named:\n%s", got)
	}
}

// TestVerifyNoticesACopiedFileThatDidNotArrive is #241: `import` copies
// two things rather than converting them — text/'s plain prose and
// config/game.yaml, the game tuning — and the comparison looked at
// neither, so a conversion that lost either reported clean, including the
// --verify pass import runs on itself.
//
// game.yaml is the one that mattered. copyGameConfig goes out of its way
// to carry it across ("a lib/ that has been tuned must not silently lose
// its tuning on the way through a format conversion") and a directory
// that came out the other side without it is a server quietly back on
// config.c's defaults.
func TestVerifyNoticesACopiedFileThatDidNotArrive(t *testing.T) {
	const mini = "../../examples/mini"
	to := t.TempDir()
	if err := run([]string{"import", "--from-dir", mini + "/binary", "--to-dir", to}); err != nil {
		t.Fatalf("import: %v", err)
	}

	left := loadOptions{base: mini + "/binary", enc: convert.Encodings[convert.DefaultEncoding]}
	right := loadOptions{base: to, format: "yaml", enc: convert.Encodings[convert.DefaultEncoding]}

	// A faithful copy first, or the assertions below prove nothing.
	if diffs, err := compareCopiedFiles(left, right); err != nil || len(diffs) != 0 {
		t.Fatalf("a fresh import reported %v, %v; want no differences", diffs, err)
	}

	// One file gone entirely, and one emptied — `truncate -s 0` is a real
	// way to lose the tuning, and an empty file is not a missing one.
	if err := os.Remove(filepath.Join(to, "text", "motd")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(to, "config", "game.yaml"), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	diffs, err := compareCopiedFiles(left, right)
	if err != nil {
		t.Fatalf("comparing: %v", err)
	}
	if len(diffs) != 2 {
		t.Fatalf("comparing reported %v, want two differences", diffs)
	}
	joined := strings.Join(diffs, "\n")
	if !strings.Contains(joined, filepath.Join("config", "game.yaml")) {
		t.Errorf("the emptied tuning file was not named:\n%s", joined)
	}
	if !strings.Contains(joined, filepath.Join("text", "motd")) {
		t.Errorf("the missing motd was not named:\n%s", joined)
	}
}

// TestTheCopiedComparisonIgnoresTheHelpDatabase keeps `copied` and `help`
// from overlapping.
//
// text/help/ is a converted subsystem with a --type of its own, and
// copyTextFiles walks only the regular files directly inside text/ for
// exactly that reason. The one file in there that *is* copied is
// HELP_PAGE_FILE, `text/help/screen`, which is not a help entry and which
// the help comparison — which loads the help database — cannot see.
func TestTheCopiedComparisonIgnoresTheHelpDatabase(t *testing.T) {
	const mini = "../../examples/mini"
	paths, err := copiedPaths(mini + "/binary")
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	var sawScreen bool
	for _, rel := range paths {
		if rel == helpScreenPath {
			sawScreen = true
			continue
		}
		if strings.HasPrefix(rel, filepath.Join("text", "help")+string(filepath.Separator)) {
			t.Errorf("%s is part of the help database and should be compared by --type=help, not as a copied file", rel)
		}
	}
	if !sawScreen {
		t.Errorf("text/help/screen is copied by import and is not in %v", paths)
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

// TestFmtLeavesTheCheckedInCorporaAlone is the load-and-resave check
// docs/design/idiomatic-go.md §6 asks for as its step 0, and the one
// direction this repository's compatibility suite did not cover.
//
// Every other format check in the tree runs *binary → yaml*:
// TestImportMatchesTheCheckedInExamples regenerates the corpus from its
// legacy source and diffs, TestEveryCorpusConvertsWithNoLoss loads both
// formats and deep-compares, and release.yml re-runs the first of those
// against a fresh build. All three start at a legacy directory. None of
// them goes *yaml → memory → yaml*, which is the only path a change to
// what the server holds in memory can actually alter — a field that
// stopped surviving the trip through `internal/game` would be reproduced
// faithfully by an importer that never round-trips it.
//
// TestFmtIsIdempotent above walks the same code and deliberately does not
// make this claim: it allows the *first* `fmt` to change the checked-in
// bytes, because idempotence is a property of the writer ("twice equals
// once") rather than of the corpus. This is the stability half of the same
// pair, against the committed bytes, and the difference between them is
// exactly the failure a memory-model refactor produces. Kept as a separate
// test rather than tightened into that one so that a genuine writer change
// — which is allowed to reformat a corpus, once, with the corpus updated
// in the same commit — fails here with a message that says which it was.
func TestFmtLeavesTheCheckedInCorporaAlone(t *testing.T) {
	for _, fx := range corpora {
		t.Run(fx.name, func(t *testing.T) {
			for _, ty := range allTypes {
				t.Run(string(ty), func(t *testing.T) {
					dir := t.TempDir()
					copyTree(t, fx.yamlDir, dir)
					before := treeBytes(t, dir)

					if err := run([]string{"fmt", "--type", string(ty), "--dir", dir}); err != nil {
						t.Fatalf("fmt: %v", err)
					}
					after := treeBytes(t, dir)

					for path, want := range before {
						got, ok := after[path]
						switch {
						case !ok:
							t.Errorf("%s: loading and re-saving removed it", path)
						case !bytes.Equal(got, want):
							t.Errorf("%s: loading and re-saving changed it (%d bytes, was %d) — "+
								"either the writer changed, in which case regenerate the corpus in "+
								"the same commit, or a field no longer survives the trip through "+
								"memory, in which case do not", path, len(got), len(want))
						}
					}
					for path := range after {
						if _, ok := before[path]; !ok {
							t.Errorf("%s: loading and re-saving created it", path)
						}
					}
				})
			}
		})
	}
}
