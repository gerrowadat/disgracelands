// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

// Package rng is the game's source of random numbers.
//
// There are two of them, and which one is in use is a decision worth being
// explicit about.
//
// The C server has its own generator (random.c) — the Park-Miller minimal
// standard, a Lehmer generator from 1988 — seeded once from time(0) at boot.
// It is fully deterministic given a seed, and it is portable: the constants
// are chosen so no intermediate value overflows a 32-bit signed integer. That
// means a Go server and the C server, given the same seed, roll *the same
// numbers*. Every damage roll, every hit roll, every ability score. For a port
// whose stated rule is that the C server wins, that is the strongest
// regression net available — an off-by-one in a combat formula produces a
// different number rather than a number that happens to fall in the same
// range.
//
// So Circle is ported exactly, and the parity harness runs on it. Whether the
// live game uses it too is a configuration choice: it is a generator from 1988
// with known-weak low bits, which matters not at all for a damage roll and is
// still not something to make the default without saying so.
//
// Nothing here is for anything security-sensitive. Passwords and TLS use
// crypto/rand, and always will.
package rng

import (
	"fmt"
	"math/rand/v2"
	"strings"
	"sync"
)

// Source produces the raw values the helpers below shape.
//
// The unit is the C's: circle_random returns an unsigned long in [1, m-1],
// and number() reduces it modulo the width of the range. Reproducing that
// exactly is the point, so the interface is in those terms rather than in
// Go's.
type Source interface {
	// Name identifies the generator, for logs and the who-list.
	Name() string
	// Uint32 returns the next raw value.
	Uint32() uint32
	// Seed restarts the sequence.
	Seed(seed uint64)
}

// Known source names.
const (
	// Circle is the C server's own generator, ported exactly.
	Circle = "circle"
	// Modern is Go's PCG.
	Modern = "modern"
)

// Names lists the generators, for flag help.
var Names = []string{Modern, Circle}

// New returns a named source, seeded.
func New(name string, seed uint64) (Source, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case Circle:
		return NewCircle(seed), nil
	case Modern, "":
		return NewModern(seed), nil
	}
	return nil, fmt.Errorf("unknown random source %q (known: %s)", name, strings.Join(Names, ", "))
}

// Rand is a generator with the game's helpers on it.
//
// It is safe for concurrent use. The world runs on one goroutine and would not
// need the lock, but the login sequence rolls abilities off it, and a data
// race in a random number generator is the kind of bug that shows up as one
// impossible combat log a month later.
type Rand struct {
	mu  sync.Mutex
	src Source
}

// NewRand wraps a source.
func NewRand(src Source) *Rand { return &Rand{src: src} }

// Name identifies the underlying generator.
func (r *Rand) Name() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.src.Name()
}

// Seed restarts the sequence. Used by the parity harness, which needs both
// servers to start from the same place.
func (r *Rand) Seed(seed uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.src.Seed(seed)
}

// Number returns a value in [from, to], porting number() (utils.c:38).
//
// The C swaps the arguments if they are the wrong way round and logs a
// SYSERR. The swap is kept — a caller that gets it backwards should not also
// get a different distribution — and the complaint becomes the caller's
// problem to notice, since there is no logger here.
//
// The modulo is the C's, biased and all. A uniform reduction would be more
// correct and would produce different numbers, which is exactly what this
// package exists not to do.
func (r *Rand) Number(from, to int32) int32 {
	if from > to {
		from, to = to, from
	}
	if from == to {
		return from
	}

	// Widened deliberately. The C computes to - from + 1 in an int, so a
	// range spanning most of the integer line overflows and it rolls
	// nonsense; nothing in the game asks for one, but silently reproducing
	// undefined behaviour is not fidelity, it is a crash waiting for an
	// unusual caller.
	width := uint64(int64(to)-int64(from)) + 1 //nolint:gosec // to > from here, so the difference is positive
	if width > 1<<32 {
		width = 1 << 32
	}

	r.mu.Lock()
	v := uint64(r.src.Uint32())
	r.mu.Unlock()

	return int32(int64(from) + int64(v%width)) //nolint:gosec // v%width < width = to-from+1, so the sum is at most to
}

// Dice rolls n dice of the given number of sides, porting dice()
// (utils.c:64).
//
// The C returns 0 for a non-positive size and for a non-positive count, in
// that order, and rolls one at a time — so the number of values drawn from
// the generator is part of the behaviour, not an implementation detail.
func (r *Rand) Dice(number, size int32) int32 {
	if size <= 0 || number <= 0 {
		return 0
	}
	var sum int32
	for ; number > 0; number-- {
		sum += r.Number(1, size)
	}
	return sum
}

// Percent returns a value in [1, 100], which is what every skill check in the
// C compares against.
func (r *Rand) Percent() int32 { return r.Number(1, 100) }

// circleSource is the C server's generator (random.c).
//
//	m = 2^31 - 1   the modulus, a Mersenne prime
//	a = 16807      the multiplier, a primitive root of m
//	q = m / a      chosen so that a*(seed%q) cannot overflow
//	r = m % a
//
// The Schrage decomposition below is why this is portable at all: every
// intermediate fits in a signed 32-bit integer, so the sequence is the same
// on the VAX this was written for and on anything since.
type circleSource struct {
	seed uint32
}

const (
	circleM uint32 = 2147483647
	circleQ uint32 = 127773
	circleA uint32 = 16807
	circleR uint32 = 2836
)

// NewCircle returns the C server's generator.
func NewCircle(seed uint64) Source {
	c := &circleSource{}
	c.Seed(seed)
	return c
}

func (c *circleSource) Name() string { return Circle }

// Seed sets the state.
//
// The C assigns time(0) straight into an unsigned long and never checks it
// (comm.c:406), so a seed of zero — or of a multiple of m — sticks the
// generator at zero forever. That cannot be reproduced without also
// reproducing a server that returns the same number for the rest of its life,
// so the degenerate states are mapped away. It is the one place this
// generator is not the C's, and it is a state the C could only reach at
// 03:14:07 UTC on 19 January 2038 or by being told to.
func (c *circleSource) Seed(seed uint64) {
	s := uint32(seed % uint64(circleM)) //nolint:gosec // reduced mod m, so it fits
	if s == 0 {
		s = 1
	}
	c.seed = s
}

// Uint32 advances the generator, porting circle_random() (random.c:76).
func (c *circleSource) Uint32() uint32 {
	hi := c.seed / circleQ
	lo := c.seed % circleQ

	// The C computes this in signed ints and adds m when the result goes
	// negative. Unsigned arithmetic cannot go negative, so the comparison is
	// done before the subtraction instead of after it.
	left := circleA * lo
	right := circleR * hi
	if left > right {
		c.seed = left - right
	} else {
		c.seed = left - right + circleM
	}
	return c.seed
}

// modernSource is Go's PCG.
type modernSource struct {
	r *rand.Rand
}

// NewModern returns the modern generator.
func NewModern(seed uint64) Source {
	return &modernSource{r: rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))}
}

func (m *modernSource) Name() string { return Modern }

func (m *modernSource) Uint32() uint32 { return m.r.Uint32() }

func (m *modernSource) Seed(seed uint64) {
	m.r = rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
}
