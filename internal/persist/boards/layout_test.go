// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package boards

import (
	"bufio"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The board file layout, against a 32-bit build of the C.
//
// Needs `gcc -m32` (gcc-multilib on Debian). CI installs it for any change
// that can reach this; see the `ilp32` step in .github/workflows/go.yml.
//
// The member that matters is the `char *heading` pointer sitting in the
// middle of the struct. Its *value* is meaningless on disk and the loader
// ignores it; its *width* moves everything after it, so a 64-bit build reads
// the level out of the pointer's second half and the lengths out of thin air.
func TestBoardLayoutMatchesA32BitBuildOfTheC(t *testing.T) {
	gcc, err := exec.LookPath("gcc")
	if err != nil {
		t.Skip("gcc not found; skipping the 32-bit board layout check (ilp32)")
	}

	src, err := filepath.Abs(filepath.Join("..", "..", "..", "reference", "tools", "boardlayout.c"))
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(t.TempDir(), "boardlayout")
	build := exec.Command(gcc, "-m32", "-std=gnu89", "-w", "-o", bin, src)
	if out, err := build.CombinedOutput(); err != nil {
		t.Skipf("cannot build 32-bit (install gcc-multilib to enable this check, ilp32): %v\n%s", err, out)
	}

	out, err := exec.Command(bin).Output()
	if err != nil {
		t.Fatalf("running the layout program: %v", err)
	}

	want := map[string]int{
		"slot_num":    offSlotNum,
		"heading":     offHeading,
		"level":       offLevel,
		"heading_len": offHeadingLen,
		"message_len": offMessageLen,
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

		switch fields[0] {
		case "sizeof":
			if got := atoi(t, fields[1]); got != msgInfoSize {
				t.Errorf("struct board_msginfo is %d bytes, this package assumes %d", got, msgInfoSize)
			}
			continue
		case "count":
			if got := atoi(t, fields[1]); got != countSize {
				t.Errorf("the leading count is %d bytes, this package assumes %d", got, countSize)
			}
			continue
		}

		offset, ok := want[fields[0]]
		if !ok {
			t.Errorf("the C has a member %q this package does not know about", fields[0])
			continue
		}
		if got := atoi(t, fields[1]); got != offset {
			t.Errorf("%s is at offset %d, this package assumes %d", fields[0], got, offset)
		}
		seen++
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if seen != len(want) {
		t.Errorf("checked %d members, want %d", seen, len(want))
	}
	t.Logf("checked %d struct members against the 32-bit C (ilp32)", seen)
}

func atoi(t *testing.T, s string) int {
	t.Helper()
	n, err := strconv.Atoi(s)
	if err != nil {
		t.Fatalf("parsing %q: %v", s, err)
	}
	return n
}
