// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package descrypt

import (
	"bufio"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Vectors produced by the system libcrypt, kept in the source so the
// implementation has something to check against even where no C toolchain or
// libcrypt exists. The exhaustive comparison is in
// TestAgainstSystemCrypt below.
var vectors = []struct{ password, salt, hash string }{
	{"password", "Zo", "ZoPAp4EhfC..M"},
	{"secret", "ab", "abNANd1rDfiNc"},
	{"", "ab", "abmF1QH4PEr.E"},
}

func TestKnownVectors(t *testing.T) {
	for _, v := range vectors {
		got, err := Crypt(v.password, v.salt)
		if err != nil {
			t.Errorf("Crypt(%q, %q): %v", v.password, v.salt, err)
			continue
		}
		if got != v.hash {
			t.Errorf("Crypt(%q, %q) = %q, want %q", v.password, v.salt, got, v.hash)
		}
	}
}

func TestOutputShape(t *testing.T) {
	got, err := Crypt("password", "Zo")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != HashLength {
		t.Errorf("hash is %d characters, want %d", len(got), HashLength)
	}
	if !strings.HasPrefix(got, "Zo") {
		t.Errorf("hash %q does not start with its salt", got)
	}
	for i, c := range got {
		if !strings.ContainsRune(Alphabet, c) {
			t.Errorf("character %d of %q is not in the crypt alphabet", i, got)
		}
	}
}

func TestOnlyTheFirstEightCharactersMatter(t *testing.T) {
	// This is a property of DES crypt, not of this implementation, and it is
	// worth asserting because it is the single most surprising thing about
	// the passwords on the archived roster: a twenty-character passphrase was
	// always eight characters of security.
	base, err := Crypt("abcdefgh", "Zo")
	if err != nil {
		t.Fatal(err)
	}
	for _, longer := range []string{"abcdefghi", "abcdefghZZZZZZZZ", "abcdefgh and then some words"} {
		got, err := Crypt(longer, "Zo")
		if err != nil {
			t.Fatal(err)
		}
		if got != base {
			t.Errorf("Crypt(%q) = %q, but Crypt(\"abcdefgh\") = %q; the ninth character should have no effect",
				longer, got, base)
		}
	}

	// And a difference within the first eight must change the hash, or the
	// test above would be vacuous.
	if differing, _ := Crypt("abcdefgX", "Zo"); differing == base {
		t.Error("changing the eighth character did not change the hash")
	}
}

func TestSaltChangesEverything(t *testing.T) {
	a, _ := Crypt("password", "aa")
	b, _ := Crypt("password", "ab")
	if a[2:] == b[2:] {
		t.Error("two salts produced the same digest; the salt is not perturbing the expansion")
	}
}

func TestAWholeHashWorksAsASalt(t *testing.T) {
	// The C code verifies by passing the stored hash as the salt argument,
	// relying on its first two characters being the original salt. If that
	// did not hold, every login would fail.
	full, err := Crypt("password", "Zo")
	if err != nil {
		t.Fatal(err)
	}
	again, err := Crypt("password", full)
	if err != nil {
		t.Fatal(err)
	}
	if again != full {
		t.Errorf("hashing under the full hash gave %q, want %q", again, full)
	}
}

func TestInvalidSalts(t *testing.T) {
	for _, salt := range []string{"", "a", "a!", "!!", "a\x00"} {
		if _, err := Crypt("password", salt); err == nil {
			t.Errorf("Crypt accepted the salt %q", salt)
		}
	}
}

// TestAgainstSystemCrypt is the check that matters.
//
// It hashes several thousand password/salt pairs with both this
// implementation and the system libcrypt, and requires every one to match. An
// implementation of a cipher that is merely "probably right" is worth very
// little; this is what makes the claim checkable rather than asserted.
//
// It skips where there is no C toolchain, and where libcrypt has been built
// without traditional DES — some modern distributions disable it, which is a
// perfectly good thing for them to do and not a failure here.
func TestAgainstSystemCrypt(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping the libcrypt comparison in short mode")
	}

	oracle := buildOracle(t)

	// A spread of shapes rather than only random noise: the empty password,
	// the boundary at eight characters, high-bit bytes, and every character
	// the salt alphabet allows.
	type pair struct{ password, salt string }
	var pairs []pair

	fixed := []string{"", "a", "ab", "abcdefg", "abcdefgh", "abcdefghi",
		"password", "Password", "12345678", "        ", "~!@#$%^&", "\x7f\x01\x02"}
	for _, p := range fixed {
		for i := 0; i < len(Alphabet); i++ {
			for j := 0; j < len(Alphabet); j += 7 {
				pairs = append(pairs, pair{p, string(Alphabet[i]) + string(Alphabet[j])})
			}
		}
	}

	rng := rand.New(rand.NewSource(1)) //nolint:gosec // reproducible test input, not a credential
	for i := 0; i < 2000; i++ {
		n := rng.Intn(12)
		b := make([]byte, n)
		for j := range b {
			// Printable ASCII: what a player could actually have typed.
			b[j] = byte(32 + rng.Intn(95))
		}
		salt := string(Alphabet[rng.Intn(len(Alphabet))]) + string(Alphabet[rng.Intn(len(Alphabet))])
		pairs = append(pairs, pair{string(b), salt})
	}

	var in strings.Builder
	for _, p := range pairs {
		in.WriteString(p.salt)
		in.WriteByte('\t')
		in.WriteString(p.password)
		in.WriteByte('\n')
	}

	cmd := exec.Command(oracle)
	cmd.Stdin = strings.NewReader(in.String())
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("running the crypt oracle: %v", err)
	}

	sc := bufio.NewScanner(strings.NewReader(string(out)))
	checked := 0
	for i := 0; sc.Scan(); i++ {
		want := sc.Text()
		if want == "UNSUPPORTED" {
			t.Skip("this libcrypt does not support traditional DES crypt; skipping the comparison")
		}
		if i >= len(pairs) {
			t.Fatalf("the oracle produced more lines than it was given inputs")
		}
		got, err := Crypt(pairs[i].password, pairs[i].salt)
		if err != nil {
			t.Fatalf("Crypt(%q, %q): %v", pairs[i].password, pairs[i].salt, err)
		}
		if got != want {
			t.Fatalf("Crypt(%q, %q) = %q, libcrypt says %q",
				pairs[i].password, pairs[i].salt, got, want)
		}
		checked++
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}

	if checked != len(pairs) {
		t.Errorf("checked %d pairs, sent %d", checked, len(pairs))
	}
	t.Logf("matched the system libcrypt on %d password/salt pairs", checked)
}

// buildOracle compiles reference/tools/cryptoracle.c, skipping if it cannot.
func buildOracle(t *testing.T) string {
	t.Helper()

	gcc, err := exec.LookPath("gcc")
	if err != nil {
		t.Skip("gcc not found; skipping the libcrypt comparison")
	}
	src := filepath.Join(repoRoot(t), "reference", "tools", "cryptoracle.c")
	if _, err := os.Stat(src); err != nil {
		t.Skipf("%s not found; skipping", src)
	}

	bin := filepath.Join(t.TempDir(), "cryptoracle")
	// -Werror=implicit-function-declaration is load-bearing: without
	// <crypt.h> the compiler assumes crypt() returns int, silently truncates
	// the pointer on a 64-bit target, and the oracle segfaults. A warning is
	// not enough when the failure looks like a crashed test binary.
	build := exec.Command(gcc, "-O2", "-Werror=implicit-function-declaration",
		"-o", bin, src, "-lcrypt")
	if out, err := build.CombinedOutput(); err != nil {
		t.Skipf("cannot build the crypt oracle (libcrypt development headers may be missing): %v\n%s", err, out)
	}
	return bin
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatal("could not find the module root")
	return ""
}

func BenchmarkCrypt(b *testing.B) {
	// Login is the only caller and it happens a few times a minute, so this
	// exists to confirm the cost is nowhere near mattering rather than to be
	// optimised against.
	for i := 0; i < b.N; i++ {
		if _, err := Crypt("password", "Zo"); err != nil {
			b.Fatal(err)
		}
	}
}
