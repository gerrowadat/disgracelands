// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package rng

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestCircleMatchesTheCGenerator is the test the whole numeric-parity idea
// rests on. If this passes, a Go server and the C server given the same seed
// roll the same numbers; if it fails, everything built on that assumption is
// worthless, so it fails rather than skips wherever a compiler exists.
func TestCircleMatchesTheCGenerator(t *testing.T) {
	oracle := buildOracle(t)

	for _, seed := range []uint64{1, 2, 42, 1024, 123456789, 2147483646} {
		want := runOracle(t, oracle, strconv.FormatUint(seed, 10), "5000")

		src := NewCircle(seed)
		for i, w := range want {
			if got := uint64(src.Uint32()); got != w {
				t.Fatalf("seed %d, draw %d: Go gave %d, the C gave %d", seed, i, got, w)
			}
		}
		t.Logf("seed %d: %d values identical to the C generator", seed, len(want))
	}
}

// TestNumberMatchesTheC covers the reduction as well as the generator: the
// modulo in number() is biased, and reproducing the bias is the point.
func TestNumberMatchesTheC(t *testing.T) {
	oracle := buildOracle(t)

	for _, tc := range []struct{ lo, hi int32 }{
		{1, 100},  // every skill check
		{1, 6},    // a die
		{3, 8},    // a magic-user's hit points
		{0, 2},    // a magic-user's movement
		{1, 1},    // the degenerate case: number(1, 1)
		{-5, 5},   // negative ranges, which the C allows
		{10, 3},   // arguments the wrong way round, which the C swaps
		{0, 1000}, // wide enough for the modulo bias to matter
	} {
		want := runOracle(t, oracle, "42", "2000",
			strconv.Itoa(int(tc.lo)), strconv.Itoa(int(tc.hi)))

		r := NewRand(NewCircle(42))
		for i, w := range want {
			if got := r.Number(tc.lo, tc.hi); int64(got) != int64(w) {
				t.Fatalf("number(%d, %d) draw %d: Go gave %d, the C gave %d",
					tc.lo, tc.hi, i, got, w)
			}
		}
	}
}

// TestAZeroWidthRangeStillDraws is the test the single-range oracle above
// cannot be: it compares draw *consumption*, not just values.
//
// number(1, 1) can only answer 1, so a port that returns 1 without touching
// the generator agrees with TestNumberMatchesTheC perfectly — and is one draw
// behind the C from then on. This port had exactly that early return, and it
// cost 288 draws during a single boot of the stock world (every d1 in every
// mobile's hit dice), which put the session-parity harness's two servers 288
// values apart before a player had typed a word. `flee` picking a different
// exit on the two servers was the visible end of it.
//
// Interleaving the degenerate range with a real one is what makes the missing
// draw observable: the 1s all match either way, and the second column does
// not.
func TestAZeroWidthRangeStillDraws(t *testing.T) {
	oracle := buildOracle(t)

	for _, tc := range []struct{ lo, hi int32 }{
		{1, 1},   // a d1, which is what mobile hit dice are full of
		{0, 0},   // and the same thing at zero
		{-3, -3}, // and negative, since number() allows it
	} {
		want := runOracle(t, oracle, "42", "500",
			strconv.Itoa(int(tc.lo)), strconv.Itoa(int(tc.hi)), "1", "100")

		r := NewRand(NewCircle(42))
		for i := 0; i < len(want); i += 2 {
			if got := r.Number(tc.lo, tc.hi); int64(got) != int64(want[i]) {
				t.Fatalf("number(%d, %d) draw %d: Go gave %d, the C gave %d",
					tc.lo, tc.hi, i/2, got, want[i])
			}
			if got := r.Number(1, 100); int64(got) != int64(want[i+1]) {
				t.Fatalf("number(1, 100) after number(%d, %d), draw %d: Go gave %d, the C gave %d"+
					" — the degenerate range did not consume a value",
					tc.lo, tc.hi, i/2, got, want[i+1])
			}
		}
	}
}

// TestSeedingIsReproducible, which is what the parity harness needs.
func TestSeedingIsReproducible(t *testing.T) {
	for _, name := range Names {
		first, err := New(name, 99)
		if err != nil {
			t.Fatal(err)
		}
		second, err := New(name, 99)
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 1000; i++ {
			if a, b := first.Uint32(), second.Uint32(); a != b {
				t.Fatalf("%s: two generators seeded alike diverged at draw %d: %d vs %d", name, i, a, b)
			}
		}

		// And re-seeding restarts it.
		want := make([]uint32, 100)
		first.Seed(7)
		for i := range want {
			want[i] = first.Uint32()
		}
		first.Seed(7)
		for i, w := range want {
			if got := first.Uint32(); got != w {
				t.Fatalf("%s: re-seeding did not restart the sequence at draw %d", name, i)
			}
		}
	}
}

// TestADegenerateSeedIsMappedAway. The C stores time(0) straight into the
// seed, and a seed of zero sticks the generator at zero for the life of the
// process. That is the one place the port deliberately differs.
func TestADegenerateSeedIsMappedAway(t *testing.T) {
	for _, seed := range []uint64{0, uint64(circleM), uint64(circleM) * 2} {
		src := NewCircle(seed)
		var sawNonZero bool
		for i := 0; i < 10; i++ {
			if src.Uint32() != 0 {
				sawNonZero = true
			}
		}
		if !sawNonZero {
			t.Errorf("seed %d leaves the generator stuck at zero", seed)
		}
	}
}

// TestCircleStaysInRange over a long run: the state must never leave
// [1, m-1], which is the property that makes the generator full-period.
func TestCircleStaysInRange(t *testing.T) {
	src := NewCircle(1)
	for i := 0; i < 2_000_000; i++ {
		if v := src.Uint32(); v == 0 || v >= circleM {
			t.Fatalf("draw %d is %d, outside [1, %d)", i, v, circleM)
		}
	}
}

func TestDice(t *testing.T) {
	r := NewRand(NewCircle(5))

	// The C returns zero for a non-positive size or count, before rolling.
	for _, tc := range []struct{ number, size int32 }{{0, 6}, {-1, 6}, {3, 0}, {3, -1}} {
		if got := r.Dice(tc.number, tc.size); got != 0 {
			t.Errorf("Dice(%d, %d) = %d, want 0", tc.number, tc.size, got)
		}
	}

	for i := 0; i < 1000; i++ {
		if got := r.Dice(3, 6); got < 3 || got > 18 {
			t.Fatalf("Dice(3, 6) = %d, want 3..18", got)
		}
	}
}

// TestDiceDrawsOncePerDie: the number of values taken from the generator is
// part of the behaviour, because everything after it in the sequence shifts.
func TestDiceDrawsOncePerDie(t *testing.T) {
	counting := NewCircle(11)
	r := NewRand(counting)
	r.Dice(4, 6)

	reference := NewCircle(11)
	for i := 0; i < 4; i++ {
		reference.Uint32()
	}

	if a, b := counting.Uint32(), reference.Uint32(); a != b {
		t.Errorf("Dice(4, 6) consumed a different number of draws: %d vs %d", a, b)
	}
}

func TestPercent(t *testing.T) {
	r := NewRand(NewCircle(3))
	seen := map[int32]bool{}
	for i := 0; i < 20000; i++ {
		v := r.Percent()
		if v < 1 || v > 100 {
			t.Fatalf("Percent() = %d, want 1..100", v)
		}
		seen[v] = true
	}
	if len(seen) != 100 {
		t.Errorf("saw %d of the 100 possible values", len(seen))
	}
}

func TestNewRejectsAnUnknownName(t *testing.T) {
	if _, err := New("mersenne", 1); err == nil {
		t.Error("New accepted an unknown generator")
	}
	// An empty name is the default rather than an error, so a zero-value
	// configuration still starts.
	src, err := New("", 1)
	if err != nil || src.Name() != Modern {
		t.Errorf("New(\"\") = %v, %v; want the modern generator", src, err)
	}
}

// buildOracle compiles reference/tools/randoracle.c.
//
// Unlike the libcrypt oracle this does not skip when there is no compiler in
// CI: the whole numeric-parity argument rests on this comparison, and a check
// that quietly does not run is worse than no check. It skips only where a
// compiler was never expected.
func buildOracle(t *testing.T) string {
	t.Helper()

	gcc, err := exec.LookPath("gcc")
	if err != nil {
		if os.Getenv("CI") != "" {
			t.Fatal("gcc not found in CI; the C generator comparison must run")
		}
		t.Skip("gcc not found; skipping the C generator comparison")
	}

	src := filepath.Join(repoRoot(t), "reference", "tools", "randoracle.c")
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("%s not found: %v", src, err)
	}

	bin := filepath.Join(t.TempDir(), "randoracle")
	build := exec.Command(gcc, "-O2", "-Wall", "-Werror", "-o", bin, src)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("compiling the oracle: %v\n%s", err, out)
	}
	return bin
}

func runOracle(t *testing.T, bin string, args ...string) []uint64 {
	t.Helper()

	out, err := exec.Command(bin, args...).Output()
	if err != nil {
		t.Fatalf("running the oracle with %v: %v", args, err)
	}

	var values []uint64
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		// Negative values come back from number() with a negative range.
		v, err := strconv.ParseInt(line, 10, 64)
		if err != nil {
			t.Fatalf("unparseable oracle output %q: %v", line, err)
		}
		values = append(values, uint64(v))
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if len(values) == 0 {
		t.Fatalf("the oracle produced nothing for %v", args)
	}
	return values
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	return dir
}
