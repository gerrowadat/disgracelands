// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import "strings"

// DisallowedName implements session.LoginHandler: Valid_Name's xnames half
// (ban.c:255-286) — lower-case the candidate, then a substring match
// against every entry, which is itself already lower-case in the archive
// file.
//
// Valid_Name does one more thing this does not: before consulting xnames
// at all it walks the live descriptor list and refuses a name that
// matches a character already CON_PLAYING (ban.c:260-262), which is a
// narrower race-guard than the roster's own Exists check — the roster
// entry for a mid-creation name does not exist yet, so two connections
// racing to create the same name are not caught by anything here. Not
// ported: it needs a live-connection registry threaded to exactly this
// call, which nothing in this slice's scope otherwise touches. Documented
// in docs/deviations.md rather than silently assumed away.
func (s *Server) DisallowedName(name string) bool {
	if len(s.names) == 0 {
		return false
	}
	lower := strings.ToLower(name)
	for _, n := range s.names {
		if strings.Contains(lower, n) {
			return true
		}
	}
	return false
}
