// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

// Package auth verifies and creates player credentials.
//
// It exists to hold one policy in one place: the 2001–2008 roster's passwords
// are DES crypt(3) hashes, they must keep working so those characters can log
// in, and every successful login replaces one with something from this
// century. Nothing forces a reset — see docs/proposals/go-port-plan.md §13.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"

	"github.com/gerrowadat/disgracelands/internal/auth/descrypt"
	"github.com/gerrowadat/disgracelands/internal/game"
)

// ErrLegacyRefused is returned when a character's stored credential is a
// legacy hash and legacy verification has been turned off.
var ErrLegacyRefused = errors.New("auth: this character still has a legacy password and legacy verification is disabled")

// legacyPrefixLength is how much of a DES hash the C server ever stored, and
// therefore how much of one there is to compare.
//
// `MAX_PWD_LENGTH` is 10, so `interpreter.c:1532` keeps ten characters of a
// thirteen-character hash and `:1462` compares the same ten. Comparing all
// thirteen here would reject every correct password on the archived roster
// while reporting nothing but "wrong password" — see
// docs/proposals/go-port-plan.md §5.3.1.
const legacyPrefixLength = 10

// The fixed halves of the argon2 parameters. The salt and digest lengths are
// not a cost knob — they are how much randomness and how much digest there
// is — so unlike Cost they are the same everywhere.
const (
	argonKeyLen  = 32
	argonSaltLen = 16
)

// Cost is the argon2id work factor: what hashing a password costs, and
// therefore what a stolen player file costs to attack.
//
// It is a field on Verifier rather than a package constant because of the
// tests. The server's own suite creates and logs in several hundred
// characters, and at the real cost — about 140ms a hash — that alone was
// more than half the runtime of `go test ./internal/server`. Making the
// factor injectable lets that suite hash cheaply while the parameters that
// actually ship stay the ones a caller gets for free; internal/auth's own
// tests still run against them.
type Cost struct {
	// Time is how many passes are made over the memory.
	Time uint32
	// Memory is how much of it there is, in KiB.
	Memory uint32
	// Threads is how many lanes the passes are split across.
	Threads uint8
}

// DefaultCost is RFC 9106's second recommendation — 64 MiB and three passes
// — which takes a few tens of milliseconds. A MUD login happens a few times
// a minute at most, so there is no reason to be parsimonious, and the cost is
// what makes a stolen player file expensive.
var DefaultCost = Cost{Time: 3, Memory: 64 * 1024, Threads: 4}

// Verifier checks passwords and produces new credentials.
type Verifier struct {
	// AllowLegacy enables verification of DES hashes. Turning it off locks
	// out anyone who has not logged in since the migration, which is why it
	// defaults on and why the server counts how many accounts still depend
	// on it.
	AllowLegacy bool

	// Cost is the work factor for credentials this Verifier creates. Each
	// zero field means DefaultCost's, so a Verifier built without a thought
	// for it hashes at the real cost rather than a cheap one — the right way
	// round to be wrong.
	//
	// It has no bearing on verification: verifyArgon2id reads the parameters
	// out of the stored hash, so a credential made under one cost still
	// verifies under a Verifier configured for another. That is the same
	// property that lets the cost be raised later without locking anybody
	// out.
	Cost Cost
}

// cost is the work factor to hash at, with DefaultCost filling in whatever
// was left zero.
func (v Verifier) cost() Cost {
	c := v.Cost
	if c.Time == 0 {
		c.Time = DefaultCost.Time
	}
	if c.Memory == 0 {
		c.Memory = DefaultCost.Memory
	}
	if c.Threads == 0 {
		c.Threads = DefaultCost.Threads
	}
	return c
}

// Result describes what a verification concluded.
type Result struct {
	// OK is whether the password was correct.
	OK bool
	// Upgraded is a replacement credential to store, set when the password
	// was correct and the stored credential was a legacy one. Empty
	// otherwise.
	//
	// Returning it rather than writing it keeps this package free of any
	// notion of where players are stored: the caller saves it, on the same
	// path it already saves everything else about a successful login.
	Upgraded *game.Credential
}

// Verify checks password against a stored credential.
//
// name is the character's name, which the legacy scheme needs: DES crypt was
// salted with it. It is unused for modern schemes and callers should pass it
// regardless.
func (v Verifier) Verify(cred game.Credential, name, password string) (Result, error) {
	switch cred.Scheme {
	case game.SchemeNone:
		// No password set. This is not "any password works"; it is a
		// character that cannot be logged into until one is set.
		return Result{}, nil

	case game.SchemeLegacyDES:
		if !v.AllowLegacy {
			return Result{}, ErrLegacyRefused
		}
		ok, err := verifyLegacy(cred.Hash, name, password)
		if err != nil || !ok {
			return Result{}, err
		}
		// Correct password, obsolete hash: this is the only moment the
		// plaintext is known, so it is the only moment the upgrade can
		// happen.
		upgraded, err := v.NewCredential(password)
		if err != nil {
			return Result{}, err
		}
		return Result{OK: true, Upgraded: &upgraded}, nil

	case game.SchemeArgon2id:
		ok, err := verifyArgon2id(cred.Hash, password)
		return Result{OK: ok}, err

	default:
		return Result{}, fmt.Errorf("auth: unknown credential scheme %q", cred.Scheme)
	}
}

// NewCredential hashes a password for storage, at this Verifier's cost.
func (v Verifier) NewCredential(password string) (game.Credential, error) {
	return newCredential(password, v.cost())
}

// NewCredential hashes a password for storage at DefaultCost.
//
// It is for callers with no Verifier to hand, which means the ones that are
// not a login: `dlctl pfile passwd` sets a password offline and has no
// legacy policy to apply.
func NewCredential(password string) (game.Credential, error) {
	return newCredential(password, DefaultCost)
}

func newCredential(password string, cost Cost) (game.Credential, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return game.Credential{}, fmt.Errorf("auth: reading random salt: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, cost.Time, cost.Memory, cost.Threads, argonKeyLen)

	// The standard PHC string, so the parameters travel with the hash and a
	// later change to them does not invalidate everything stored before it.
	hash := fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, cost.Memory, cost.Time, cost.Threads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key))

	return game.Credential{Scheme: game.SchemeArgon2id, Hash: hash}, nil
}

// verifyLegacy checks a DES crypt(3) hash.
func verifyLegacy(stored, name, password string) (bool, error) {
	if stored == "" {
		return false, nil
	}

	// The salt is the first two characters of the stored hash, which are the
	// first two of the character's name — the C code sets the password using
	// the name as salt and checks it using the stored hash, and both mean the
	// same two bytes. Preferring the stored hash means a character renamed at
	// some point still verifies.
	salt := stored
	if len(salt) < 2 {
		if len(name) < 2 {
			return false, fmt.Errorf("auth: cannot derive a salt from %q", name)
		}
		salt = name
	}

	computed, err := descrypt.Crypt(password, salt)
	if err != nil {
		return false, err
	}

	// Compare only what was stored. See legacyPrefixLength.
	n := min(len(stored), legacyPrefixLength)
	if len(computed) < n {
		return false, nil
	}
	return subtle.ConstantTimeCompare([]byte(computed[:n]), []byte(stored[:n])) == 1, nil
}

// verifyArgon2id checks a PHC-format argon2id hash, taking its parameters
// from the hash rather than from this package's constants — otherwise
// changing the cost would lock out everyone hashed under the old one.
func verifyArgon2id(stored, password string) (bool, error) {
	parts := strings.Split(stored, "$")
	// "", "argon2id", "v=19", "m=...,t=...,p=...", salt, key
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, fmt.Errorf("auth: malformed argon2id hash")
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return false, fmt.Errorf("auth: malformed argon2id version: %w", err)
	}
	if version != argon2.Version {
		return false, fmt.Errorf("auth: argon2 version %d, want %d", version, argon2.Version)
	}

	memory, time, threads, err := parseArgonParams(parts[3])
	if err != nil {
		return false, err
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, fmt.Errorf("auth: malformed argon2id salt: %w", err)
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, fmt.Errorf("auth: malformed argon2id digest: %w", err)
	}

	got := argon2.IDKey([]byte(password), salt, time, memory, threads, uint32(len(want))) //nolint:gosec // length of a decoded digest
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

func parseArgonParams(s string) (memory, time uint32, threads uint8, err error) {
	for _, kv := range strings.Split(s, ",") {
		k, v, found := strings.Cut(kv, "=")
		if !found {
			return 0, 0, 0, fmt.Errorf("auth: malformed argon2id parameter %q", kv)
		}
		n, convErr := strconv.ParseUint(v, 10, 32)
		if convErr != nil {
			return 0, 0, 0, fmt.Errorf("auth: malformed argon2id parameter %q: %w", kv, convErr)
		}
		switch k {
		case "m":
			memory = uint32(n)
		case "t":
			time = uint32(n)
		case "p":
			if n > 255 {
				return 0, 0, 0, fmt.Errorf("auth: argon2id parallelism %d is out of range", n)
			}
			threads = uint8(n)
		default:
			return 0, 0, 0, fmt.Errorf("auth: unknown argon2id parameter %q", k)
		}
	}
	if memory == 0 || time == 0 || threads == 0 {
		return 0, 0, 0, fmt.Errorf("auth: incomplete argon2id parameters %q", s)
	}
	return memory, time, threads, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
