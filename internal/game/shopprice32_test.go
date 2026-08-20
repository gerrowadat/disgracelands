// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import (
	"bufio"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Shop prices, against a 32-bit build of the C.
//
// This one needs `gcc -m32` — 32-bit libc headers, gcc-multilib on Debian —
// because the answer *depends on the width the multiplication happens at*
// and a 64-bit build gives a different one. See reference/tools/shopprice.c
// for the whole story; the short version is that `int * float` truncated to
// int is 115 with SSE and 114 with the x87, and the archived server was
// i386.
//
// CI installs the toolchain only for changes that can affect this; see the
// `ilp32` step in .github/workflows/go.yml.
func TestShopPricesMatchA32BitBuildOfTheC(t *testing.T) {
	gcc, err := exec.LookPath("gcc")
	if err != nil {
		t.Skip("gcc not found; skipping the 32-bit shop price check")
	}

	dir := t.TempDir()
	bin := filepath.Join(dir, "shopprice")
	src, err := filepath.Abs(filepath.Join("..", "..", "reference", "tools", "shopprice.c"))
	if err != nil {
		t.Fatal(err)
	}
	build := exec.Command(gcc, "-m32", "-mfpmath=387", "-std=gnu89", "-O2", "-w", "-o", bin, src)
	if out, err := build.CombinedOutput(); err != nil {
		t.Skipf("cannot build 32-bit (install gcc-multilib to enable this check): %v\n%s", err, out)
	}

	out, err := exec.Command(bin).Output()
	if err != nil {
		t.Fatalf("running the oracle: %v", err)
	}

	checked := 0
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#") {
			t.Log(line)
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 5 {
			t.Fatalf("unexpected oracle output %q", line)
		}
		buyProfit := parseFloat32(t, fields[0])
		sellProfit := parseFloat32(t, fields[1])
		cost := parseInt32(t, fields[2])
		wantBuy := parseInt32(t, fields[3])
		wantSell := parseInt32(t, fields[4])

		shop := &ShopDef{ProfitBuy: buyProfit, ProfitSell: sellProfit}
		obj := &Object{Cost: cost}

		if got := BuyPrice(shop, obj); got != wantBuy {
			t.Fatalf("BuyPrice(cost %d, profit %v) = %d, the C says %d",
				cost, buyProfit, got, wantBuy)
		}
		if got := SellPrice(shop, obj); got != wantSell {
			t.Fatalf("SellPrice(cost %d, profit %v) = %d, the C says %d",
				cost, sellProfit, got, wantSell)
		}
		checked++
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if checked == 0 {
		t.Fatal("the oracle produced no cases")
	}
	t.Logf("checked %d price pairs against the 32-bit C", checked)
}

func parseFloat32(t *testing.T, s string) float32 {
	t.Helper()
	v, err := strconv.ParseFloat(s, 32)
	if err != nil {
		t.Fatalf("parsing %q: %v", s, err)
	}
	return float32(v)
}

func parseInt32(t *testing.T, s string) int32 {
	t.Helper()
	v, err := strconv.ParseInt(s, 10, 32)
	if err != nil {
		t.Fatalf("parsing %q: %v", s, err)
	}
	return int32(v)
}
