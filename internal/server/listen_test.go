// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import "testing"

// TestPerHostKeyBucketsIPv6ByItsOwn64 is the property --max-connections-
// per-ip depends on: two different IPv6 addresses in the same /64 must
// count against each other, the same way two connections from one IPv4
// address already do, because a /64 — not a single address — is the unit
// an IPv6 subscriber actually owns (RFC 6177). Table-driven against real
// address shapes rather than asserted in the abstract, since this is
// exactly the kind of "would have to simulate it in your head to be sure"
// function CLAUDE.md's testing discipline distrusts reading alone for.
func TestPerHostKeyBucketsIPv6ByItsOwn64(t *testing.T) {
	tests := []struct {
		name string
		host string
		want string
	}{
		{"ipv4 counts by itself", "192.168.1.5", "192.168.1.5"},
		{"ipv4-mapped ipv6 counts as ipv4", "::ffff:192.168.1.5", "::ffff:192.168.1.5"},
		{"ipv6 masks to its /64",
			"2001:db8:1234:5678:9999:aaaa:bbbb:cccc", "2001:db8:1234:5678::"},
		{"a second address in the same /64 masks the same way",
			"2001:db8:1234:5678::1", "2001:db8:1234:5678::"},
		{"a different /64 is a different bucket",
			"2001:db8:1234:9999::1", "2001:db8:1234:9999::"},
		{"not an IP at all passes through unchanged",
			"not-an-ip", "not-an-ip"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := perHostKey(tt.host); got != tt.want {
				t.Errorf("perHostKey(%q) = %q, want %q", tt.host, got, tt.want)
			}
		})
	}

	// The property the table above is really testing: two addresses this
	// port considers "the same subscriber" must produce identical keys,
	// and two it does not must produce different ones.
	sameSubnet := []string{
		"2001:db8:1234:5678:9999:aaaa:bbbb:cccc",
		"2001:db8:1234:5678::1",
	}
	if perHostKey(sameSubnet[0]) != perHostKey(sameSubnet[1]) {
		t.Errorf("two addresses in the same /64 got different keys: %q vs %q",
			perHostKey(sameSubnet[0]), perHostKey(sameSubnet[1]))
	}
	if perHostKey("192.168.1.5") == perHostKey("192.168.1.6") {
		t.Error("two different IPv4 addresses got the same key")
	}
}
