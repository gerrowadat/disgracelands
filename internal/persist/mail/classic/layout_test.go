// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package classic

import (
	"bufio"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The mail block layout, against a 32-bit build of the C.
//
// Needs `gcc -m32` (gcc-multilib on Debian). CI installs it for any change
// that can reach this; see the `ilp32` step in .github/workflows/go.yml.
//
// The block size is 100 under every data model, so nothing about the file's
// shape gives the mismatch away. What changes is how much text fits in a
// block — 79 characters in a header under ILP32, 59 under LP64 — so the wrong
// build reads every message with its middle shuffled and no error anywhere.
func TestMailLayoutMatchesA32BitBuildOfTheC(t *testing.T) {
	gcc, err := exec.LookPath("gcc")
	if err != nil {
		t.Skip("gcc not found; skipping the 32-bit mail layout check (ilp32)")
	}

	src, err := filepath.Abs(filepath.Join("..", "..", "..", "..", "reference", "tools", "maillayout.c"))
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(t.TempDir(), "maillayout")
	build := exec.Command(gcc, "-m32", "-std=gnu89", "-w", "-o", bin, src)
	if out, err := build.CombinedOutput(); err != nil {
		t.Skipf("cannot build 32-bit (install gcc-multilib to enable this check, ilp32): %v\n%s", err, out)
	}

	out, err := exec.Command(bin).Output()
	if err != nil {
		t.Fatalf("running the layout program: %v", err)
	}

	want := map[string]int{
		"block_size":      BlockSize,
		"header_size":     BlockSize,
		"data_size":       BlockSize,
		"header_datasize": HeaderTextSize,
		"data_datasize":   DataTextSize,
		"block_type":      offBlockType,
		"next_block":      offNextBlock,
		"from":            offFrom,
		"to":              offTo,
		"mail_time":       offMailTime,
		"header_txt":      offHeaderTxt,
		"data_txt":        offDataTxt,
	}

	seen := 0
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#") {
			t.Log(line)
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 {
			t.Fatalf("unexpected output %q", line)
		}
		expect, ok := want[fields[0]]
		if !ok {
			t.Errorf("the C reports %q, which this package does not know about", fields[0])
			continue
		}
		got, err := strconv.Atoi(fields[1])
		if err != nil {
			t.Fatalf("parsing %q: %v", fields[1], err)
		}
		if got != expect {
			t.Errorf("%s is %d, this package assumes %d", fields[0], got, expect)
		}
		seen++
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if seen != len(want) {
		t.Errorf("checked %d values, want %d", seen, len(want))
	}
	t.Logf("checked %d mail block values against the 32-bit C (ilp32)", seen)
}
