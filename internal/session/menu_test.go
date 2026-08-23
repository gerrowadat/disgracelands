// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package session

import "testing"

// TestParseEditorRange, porting parse_action's own
// `sscanf(string, " %d - %d ", &line_low, &line_high)` (improved-edit.c:222)
// closely enough for what a person actually types at the /l prompt: no
// digits at all falls back to "the whole buffer" the same way a zero-match
// sscanf does, not an error.
func TestParseEditorRange(t *testing.T) {
	cases := []struct {
		args           string
		low, high      int
		wantErr        bool
		wantErrMessage string
	}{
		{args: "", low: 1, high: maxEditorLine},
		{args: "   ", low: 1, high: maxEditorLine},
		{args: "3", low: 3, high: 3},
		{args: " 3 ", low: 3, high: 3},
		{args: "2-5", low: 2, high: 5},
		{args: "2 - 5", low: 2, high: 5},
		{args: "5-2", wantErr: true, wantErrMessage: "That range is invalid.\r\n"},
		// Not a number at all: the C's sscanf matches zero items, the
		// same as no argument.
		{args: "abc", low: 1, high: maxEditorLine},
		// A dash with no valid second number: one match, so high==low —
		// the C's own `case 1`.
		{args: "3-abc", low: 3, high: 3},
	}

	for _, tc := range cases {
		low, high, errMsg := parseEditorRange(tc.args)
		if tc.wantErr {
			if errMsg != tc.wantErrMessage {
				t.Errorf("parseEditorRange(%q) errMsg = %q, want %q", tc.args, errMsg, tc.wantErrMessage)
			}
			continue
		}
		if errMsg != "" {
			t.Errorf("parseEditorRange(%q) errMsg = %q, want none", tc.args, errMsg)
		}
		if low != tc.low || high != tc.high {
			t.Errorf("parseEditorRange(%q) = (%d, %d), want (%d, %d)", tc.args, low, high, tc.low, tc.high)
		}
	}
}
