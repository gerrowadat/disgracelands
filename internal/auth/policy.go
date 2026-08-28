// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package auth

import (
	"fmt"
	"strings"
)

// MinPasswordLength is what a new password must be.
//
// The C server enforced three characters. That was a reasonable floor when
// the hash truncated to eight anyway; now that a password is stored under
// argon2id and used in full, there is no reason to keep it that low. Old
// characters are unaffected: this applies only to passwords being set.
const MinPasswordLength = 6

// BadPassword reports why a password cannot be set, or "".
//
// The C refuses an empty password, one longer than ten characters, one
// shorter than three, and one equal to the character's name
// (interpreter.c:1526). Two of those four are kept, and the reasons are
// recorded in docs/deviations.md: the ten-character ceiling existed because
// the field in the binary record is ten bytes wide, and the three-character
// floor was reasonable when the hash truncated to eight characters anyway.
// Neither is true now.
//
// The rule lives here rather than beside the login state machine because the
// login state machine is no longer the only thing that sets a password:
// `dlctl passwd --type=pfile` does it offline, and a rule each caller states for
// itself is one that ends up meaning two different things. The strings are
// the C's, so they are still what a player sees at the menu.
func BadPassword(password, name string) string {
	if len(password) < MinPasswordLength {
		return fmt.Sprintf("Passwords must be at least %d characters.", MinPasswordLength)
	}
	if strings.EqualFold(password, name) {
		return "Illegal password."
	}
	return ""
}
