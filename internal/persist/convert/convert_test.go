// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.
package convert

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

// oldDirectory builds a small stand-in for an original CircleMUD data
// directory: some plain text, some CP1252 text, and one of the binary
// formats that must not be touched.
func oldDirectory(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	write := func(rel string, data []byte) {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	write("world/wld/0.wld", []byte("#0\nThe Void~\nNothing here.\n~\n0 0 0\nS\n$\n"))
	// 0xe9 is é and 0x92 is a right single quotation mark, in CP1252.
	write("world/wld/1.wld", []byte("#1\nCaf\xe9~\nThe owner\x92s cat.\n~\n0 0 0\nS\n$\n"))
	write("text/greetings", []byte("Welcome to Disgracelands\n"))
	write("text/motd", []byte("Caf\xe9 open all hours\n"))
	// A board: binary, and containing high bytes that are not characters.
	write("etc/board.mort", []byte{0x17, 0x00, 0x00, 0x00, 0xba, 0xc0, 'h', 'i', 0x00})
	write("etc/hcontrol", []byte{0x01, 0x00, 0x00, 0x00, 0xff})
	// Ordinary files that happen to live beside binary ones.
	write("house/README", []byte("Houses go here.\n"))
	write("plrobjs/.gitkeep", nil)

	return dir
}

func run(t *testing.T, opts Options) *Report {
	t.Helper()
	if opts.To == "" {
		opts.To = filepath.Join(t.TempDir(), "out")
	}
	r, err := Run(context.Background(), opts)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return r
}

func actionFor(r *Report, path string) (Action, string, bool) {
	for _, e := range r.Entries {
		if e.Path == filepath.FromSlash(path) {
			return e.Action, e.Note, true
		}
	}
	return 0, "", false
}

func TestTranscodesTextToUTF8(t *testing.T) {
	from := oldDirectory(t)
	to := filepath.Join(t.TempDir(), "out")
	run(t, Options{From: from, To: to})

	got, err := os.ReadFile(filepath.Join(to, "world", "wld", "1.wld"))
	if err != nil {
		t.Fatal(err)
	}
	if !utf8.Valid(got) {
		t.Fatal("the converted file is still not valid UTF-8")
	}
	for _, want := range []string{"Café", "owner\u2019s"} {
		if !strings.Contains(string(got), want) {
			t.Errorf("converted file does not contain %q:\n%s", want, got)
		}
	}
}

func TestCP1252AndLatin1DifferWhereItMatters(t *testing.T) {
	// The two agree everywhere except 0x80–0x9F. 0x92 is a right single
	// quotation mark in CP1252 and an unprintable C1 control in Latin-1, and
	// the world files contain it — so the default is a real choice.
	from := oldDirectory(t)

	cp := filepath.Join(t.TempDir(), "cp")
	run(t, Options{From: from, To: cp, Encoding: Encodings["cp1252"]})
	latin := filepath.Join(t.TempDir(), "latin")
	run(t, Options{From: from, To: latin, Encoding: Encodings["latin1"]})

	cpText, _ := os.ReadFile(filepath.Join(cp, "world", "wld", "1.wld"))
	latinText, _ := os.ReadFile(filepath.Join(latin, "world", "wld", "1.wld"))

	if !strings.Contains(string(cpText), "owner\u2019s") {
		t.Errorf("cp1252 did not produce a curly apostrophe:\n%s", cpText)
	}
	if strings.Contains(string(latinText), "owner\u2019s") {
		t.Errorf("latin1 produced a curly apostrophe, which it has no mapping for:\n%s", latinText)
	}
	// Both must still be valid UTF-8; Latin-1 just produces a control
	// character rather than punctuation.
	if !utf8.Valid(latinText) {
		t.Error("the latin1 conversion produced invalid UTF-8")
	}
}

func TestAlreadyUTF8FilesAreUntouched(t *testing.T) {
	from := oldDirectory(t)
	to := filepath.Join(t.TempDir(), "out")
	r := run(t, Options{From: from, To: to})

	if a, _, ok := actionFor(r, "world/wld/0.wld"); !ok || a != Copied {
		t.Errorf("a pure-ASCII file was %v, want copied", a)
	}

	before, _ := os.ReadFile(filepath.Join(from, "world", "wld", "0.wld"))
	after, _ := os.ReadFile(filepath.Join(to, "world", "wld", "0.wld"))
	if string(before) != string(after) {
		t.Error("a file that needed no conversion was changed")
	}
}

func TestBinaryFormatsAreReportedAndLeftAlone(t *testing.T) {
	// This is the whole reason the converter is selective. A board holds
	// struct fields and length-prefixed text; transcoding it would rewrite
	// bytes that were never characters *and* invalidate the stored lengths.
	from := oldDirectory(t)
	to := filepath.Join(t.TempDir(), "out")
	r := run(t, Options{From: from, To: to})

	for _, path := range []string{"etc/board.mort", "etc/hcontrol"} {
		a, note, ok := actionFor(r, path)
		if !ok {
			t.Errorf("%s is missing from the report", path)
			continue
		}
		if a != Unsupported {
			t.Errorf("%s was %v, want unsupported", path, a)
		}
		if note == "" {
			t.Errorf("%s was reported without saying why", path)
		}

		before, _ := os.ReadFile(filepath.Join(from, path))
		after, err := os.ReadFile(filepath.Join(to, path))
		if err != nil {
			t.Errorf("%s was not carried across: %v", path, err)
			continue
		}
		if string(before) != string(after) {
			t.Errorf("%s was modified; it should have been copied byte for byte", path)
		}
	}
}

func TestOrdinaryFilesBesideBinaryOnesAreNotMisclassified(t *testing.T) {
	// house/ and plrobjs/ hold READMEs and .gitkeep files as well as data.
	// Matching on the directory alone reported those as struct dumps, which
	// is how this test came to exist.
	from := oldDirectory(t)
	r := run(t, Options{From: from})

	for _, path := range []string{"house/README", "plrobjs/.gitkeep"} {
		a, _, ok := actionFor(r, path)
		if !ok {
			t.Errorf("%s is missing from the report", path)
			continue
		}
		if a == Unsupported {
			t.Errorf("%s was reported as an unsupported binary format, but it is an ordinary file", path)
		}
	}
}

func TestDryRunWritesNothing(t *testing.T) {
	from := oldDirectory(t)
	to := filepath.Join(t.TempDir(), "out")

	r := run(t, Options{From: from, To: to, DryRun: true})
	if len(r.Entries) == 0 {
		t.Fatal("a dry run reported nothing")
	}
	if _, err := os.Stat(to); !os.IsNotExist(err) {
		t.Errorf("a dry run created %s", to)
	}
}

func TestRefusesToConvertInPlace(t *testing.T) {
	// Converting a directory into itself would leave it half-converted if
	// anything failed part way.
	dir := oldDirectory(t)
	if _, err := Run(context.Background(), Options{From: dir, To: dir}); err == nil {
		t.Error("converting a directory into itself was allowed")
	}
}

func TestRefusesADestinationInsideTheSource(t *testing.T) {
	dir := oldDirectory(t)
	_, err := Run(context.Background(), Options{From: dir, To: filepath.Join(dir, "converted")})
	if err == nil {
		t.Error("a destination inside the source was allowed")
	}
}

func TestRefusesANonEmptyDestination(t *testing.T) {
	from := oldDirectory(t)
	to := t.TempDir()
	if err := os.WriteFile(filepath.Join(to, "something"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := Run(context.Background(), Options{From: from, To: to}); err == nil {
		t.Error("a non-empty destination was accepted without --force")
	}
	if _, err := Run(context.Background(), Options{From: from, To: to, Force: true}); err != nil {
		t.Errorf("--force did not allow a non-empty destination: %v", err)
	}
}

func TestEverythingIsCarriedAcross(t *testing.T) {
	// Whatever else it does, the converter must not lose a file.
	from := oldDirectory(t)
	to := filepath.Join(t.TempDir(), "out")
	run(t, Options{From: from, To: to})

	count := func(root string) int {
		n := 0
		_ = filepath.Walk(root, func(_ string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() {
				n++
			}
			return nil
		})
		return n
	}
	if in, out := count(from), count(to); in != out {
		t.Errorf("the source has %d files and the destination %d", in, out)
	}
}

func TestConvertedOutputIsValidUTF8ExceptWhereItSaysOtherwise(t *testing.T) {
	from := oldDirectory(t)
	to := filepath.Join(t.TempDir(), "out")
	r := run(t, Options{From: from, To: to})

	unsupported := map[string]bool{}
	for _, e := range r.Entries {
		if e.Action == Unsupported {
			unsupported[e.Path] = true
		}
	}

	err := filepath.Walk(to, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(to, path)
		data, err := os.ReadFile(path) //nolint:gosec // a directory this test created
		if err != nil {
			return err
		}
		if utf8.Valid(data) {
			return nil
		}
		if !unsupported[rel] {
			t.Errorf("%s is not valid UTF-8 and was not reported as unconvertible", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
