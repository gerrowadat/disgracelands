// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package houses

import (
	"bufio"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The house control record, against a 32-bit build of the C.
//
// Needs `gcc -m32` (gcc-multilib on Debian). CI installs it for any change
// that can reach this; see the `ilp32` step in .github/workflows/go.yml.
func TestHouseLayoutMatchesA32BitBuildOfTheC(t *testing.T) {
	gcc, err := exec.LookPath("gcc")
	if err != nil {
		t.Skip("gcc not found; skipping the 32-bit house layout check (ilp32)")
	}

	src, err := filepath.Abs(filepath.Join("..", "..", "..", "reference", "tools", "houselayout.c"))
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(t.TempDir(), "houselayout")
	build := exec.Command(gcc, "-m32", "-std=gnu89", "-w", "-o", bin, src)
	if out, err := build.CombinedOutput(); err != nil {
		t.Skipf("cannot build 32-bit (install gcc-multilib to enable this check, ilp32): %v\n%s", err, out)
	}

	out, err := exec.Command(bin).Output()
	if err != nil {
		t.Fatalf("running the layout program: %v", err)
	}

	want := map[string]int{
		"vnum":          offVnum,
		"atrium":        offAtrium,
		"exit_num":      offExitNum,
		"built_on":      offBuiltOn,
		"mode":          offMode,
		"owner":         offOwner,
		"num_of_guests": offNumGuests,
		"guests":        offGuests,
		"last_payment":  offLastPayment,
		"spare0":        offSpare0,
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

		if fields[0] == "sizeof" {
			if got := atoi(t, fields[1]); got != recordSize {
				t.Errorf("struct house_control_rec is %d bytes, this package assumes %d", got, recordSize)
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
		// The guest array's stride, checked through its total size.
		if fields[0] == "guests" {
			if got := atoi(t, fields[2]); got != MaxGuests*guestStride {
				t.Errorf("guests is %d bytes, this package assumes %d",
					got, MaxGuests*guestStride)
			}
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
