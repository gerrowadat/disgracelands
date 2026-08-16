// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package classic

import (
	"fmt"
	"math"
	"strings"
)

// The C loader reads every structural line with sscanf, and three of its
// behaviours matter for reproducing what it accepts:
//
//   - Extra trailing fields are ignored. Zone command lines in the real data
//     carry trailing comments — "M 0 1 1 1 \t(Puff)" — that no format string
//     mentions and sscanf silently drops.
//   - %d parses a leading integer prefix and stops at the first character
//     that cannot continue it, rather than requiring the whole token to be
//     numeric.
//   - A conversion that matches nothing ends the scan, and the caller decides
//     whether the count it got is acceptable. Several call sites accept a
//     short count and default the missing field.
//
// splitFields plus scanInt reproduce all three. Using strconv.Atoi over whole
// tokens instead would reject input the C loader accepts.
//
// Everything in this file format is 32 bits wide — the C loader reads it all
// with %d into `int`, or into `sh_int` for a few fields — so the scanner
// returns int32 rather than int64. That is not a detail: a scanner returning
// int64 would put a narrowing conversion at every one of the thirty-odd call
// sites, and one of those getting it wrong is precisely the failure
// docs/proposals/go-port-plan.md §4 is about. Doing the range check once, in
// the scanner, means the call sites cannot get it wrong.

// splitFields splits a line on whitespace, dropping empty fields. This is
// what sscanf's whitespace handling amounts to for these formats.
func splitFields(line string) []string {
	return strings.Fields(line)
}

// scanInt parses a leading signed 32-bit integer from s, in the manner of
// strtol. It returns the value and whether any digits were found. Trailing
// junk is ignored, so "3001)" yields 3001.
//
// A number too large for 32 bits saturates rather than wrapping. The C code
// overflows silently here; a world file with a 30-digit vnum is corrupt
// either way, and a saturated value stays obviously wrong instead of becoming
// a plausible different number. Callers that care report it.
func scanInt(s string) (int32, bool) {
	i := 0
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	neg := false
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		neg = s[i] == '-'
		i++
	}
	start := i

	var n int64
	saturated := false
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		if !saturated {
			n = n*10 + int64(s[i]-'0')
			if n > math.MaxInt32+1 {
				saturated = true
			}
		}
		i++
	}
	if i == start {
		return 0, false
	}

	if neg {
		n = -n
	}
	switch {
	case n > math.MaxInt32:
		return math.MaxInt32, true
	case n < math.MinInt32:
		return math.MinInt32, true
	}
	return int32(n), true
}

// scanInts parses up to len(out) integers from the fields of line, in order.
// It returns how many it successfully parsed; parsing stops at the first
// field that yields no digits, matching sscanf's behaviour of ending the scan
// at a failed conversion.
func scanInts(line string, out []int32) int {
	fields := splitFields(line)
	n := 0
	for n < len(out) && n < len(fields) {
		v, ok := scanInt(fields[n])
		if !ok {
			break
		}
		out[n] = v
		n++
	}
	return n
}

// requireInts parses exactly want integers from line, erroring if fewer are
// present. Extra fields are ignored, as sscanf ignores them.
func requireInts(line string, want int, what string) ([]int32, error) {
	out := make([]int32, want)
	got := scanInts(line, out)
	if got != want {
		return nil, fmt.Errorf("expected %d numbers for %s, got %d in %q", want, what, got, line)
	}
	return out, nil
}
