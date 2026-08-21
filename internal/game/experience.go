// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import "github.com/gerrowadat/disgracelands/internal/rng"

// Experience and levelling, ported from level_exp (class.c:2101) and
// gain_exp (limits.c:265).

// expMax is EXP_MAX (class.c:2098). Immortal levels are priced just below it
// so that no amount of mortal play can reach them.
const expMax int32 = 10000000

// Experience gain and loss caps, from config.c:72. These are compile-time
// constants in the C and belong in the config file §9.1 describes; until that
// exists they live here with their provenance attached.
const (
	MaxExpGainPerKill  int32 = 100000
	MaxExpLossPerDeath int32 = 500000
)

// LevelExperience returns the experience needed to reach a level, porting
// level_exp.
//
// The tables run to level 31 — the first immortal level — and everything
// above that is priced off EXP_MAX so that adding immortal levels never
// requires touching a table. A level outside the range is a caller error in
// the C, which logs a SYSERR and returns zero; returning zero would make
// every comparison against it succeed, so this returns the maximum instead.
func LevelExperience(class, level int32) int32 {
	if level < 0 || level > LevelImplementor {
		return expMax
	}
	if level > LevelImmortal {
		return expMax - (LevelImplementor-level)*1000
	}
	table, ok := levelExperience[class]
	if !ok {
		return expMax
	}
	return table[level]
}

// ExpToLevel is how much more experience a character needs, or zero if they
// are ready to rise.
func ExpToLevel(rec *PlayerRecord) int32 {
	need := LevelExperience(rec.Class, rec.Level+1) - rec.Points.Exp
	return max(0, need)
}

// ExpGain describes what GainExperience did, so the caller can say so.
type ExpGain struct {
	// Applied is the experience actually added, after both caps.
	Applied int32
	// Capped is set when the local per-kill limit bit — the C tells the
	// player, in those words.
	Capped bool
	// Levels is how many levels were gained.
	Levels int32
}

// GainExperience awards or removes experience, porting gain_exp
// (limits.c:265).
//
// Two caps apply to a gain, in this order. The first is stock: no single kill
// awards more than max_exp_gain. The second is Disgracelands' own and is the
// more aggressive of the two — no single kill may award more than a tenth of
// the band between the current level and the next, so a low-level character
// cannot be power-levelled by being dragged along on a big kill. See
// docs/investigations/non-stock-features.md.
//
// Levelling loops rather than stepping once: enough experience for three
// levels grants three, each with its own advance_level. The C says "You rise
// 3 levels!" in that case and tells the whole game about it, which is the
// caller's job here.
func GainExperience(rec *PlayerRecord, gain int32, r *rng.Rand) ExpGain {
	// A character below level one has not started yet, and one at or above
	// the level below immortal has finished. The C's bound is
	// `>= LVL_IMMORT - 1`, so level 30 is where mortal progress stops.
	if rec.Level < 1 || rec.Level >= LevelImmortal-1 {
		return ExpGain{}
	}

	var out ExpGain

	switch {
	case gain > 0:
		gain = min(MaxExpGainPerKill, gain)

		band := LevelExperience(rec.Class, rec.Level+1) - LevelExperience(rec.Class, rec.Level)
		if limit := band / 10; gain > limit {
			gain = limit
			out.Capped = true
		}

		rec.Points.Exp += gain
		out.Applied = gain

		for rec.Level < LevelImmortal &&
			rec.Points.Exp >= LevelExperience(rec.Class, rec.Level+1) {
			rec.Level++
			out.Levels++
			AdvanceLevel(rec, r)
		}
		if out.Levels > 0 {
			rec.Title = Title(rec.Class, rec.Level, rec.Sex)
		}

	case gain < 0:
		gain = max(-MaxExpLossPerDeath, gain)
		rec.Points.Exp += gain
		if rec.Points.Exp < 0 {
			rec.Points.Exp = 0
		}
		out.Applied = gain
	}

	return out
}

var levelExperience = map[int32][]int32{
	ClassMagicUser: {
		0, 1, 2500, 5000, // 0-3
		10000, 20000, 40000, 60000, // 4-7
		90000, 135000, 250000, 375000, // 8-11
		750000, 1125000, 1500000, 1875000, // 12-15
		2250000, 2625000, 3000000, 3375000, // 16-19
		3750000, 4000000, 4300000, 4600000, // 20-23
		4900000, 5200000, 5500000, 5950000, // 24-27
		6400000, 6850000, 7400000, 8000000, // 28-31
	},
	ClassCleric: {
		0, 1, 1500, 3000, // 0-3
		6000, 13000, 27500, 55000, // 4-7
		110000, 225000, 450000, 675000, // 8-11
		900000, 1125000, 1350000, 1575000, // 12-15
		1800000, 2100000, 2400000, 2700000, // 16-19
		3000000, 3250000, 3500000, 3800000, // 20-23
		4100000, 4400000, 4800000, 5200000, // 24-27
		5600000, 6000000, 6400000, 7000000, // 28-31
	},
	ClassThief: {
		0, 1, 1250, 2500, // 0-3
		5000, 10000, 20000, 30000, // 4-7
		70000, 110000, 160000, 220000, // 8-11
		440000, 660000, 880000, 1100000, // 12-15
		1500000, 2000000, 2500000, 3000000, // 16-19
		3500000, 3650000, 3800000, 4100000, // 20-23
		4400000, 4700000, 5100000, 5500000, // 24-27
		5900000, 6300000, 6650000, 7000000, // 28-31
	},
	ClassWarrior: {
		0, 1, 2000, 4000, // 0-3
		8000, 16000, 32000, 64000, // 4-7
		125000, 250000, 500000, 750000, // 8-11
		1000000, 1250000, 1500000, 1850000, // 12-15
		2200000, 2550000, 2900000, 3250000, // 16-19
		3600000, 3900000, 4200000, 4500000, // 20-23
		4800000, 5150000, 5500000, 5950000, // 24-27
		6400000, 6850000, 7400000, 8000000, // 28-31
	},
	ClassPaladin: {
		0, 1, 2250, 4500, // 0-3
		9000, 18000, 36000, 75000, // 4-7
		150000, 300000, 600000, 900000, // 8-11
		1200000, 1500000, 1800000, 2100000, // 12-15
		2400000, 2700000, 3000000, 3300000, // 16-19
		3600000, 4000000, 4300000, 4700000, // 20-23
		5200000, 5500000, 6000000, 6700000, // 24-27
		7200000, 8000000, 9000000, 9500000, // 28-31
	},
}

// GainExperienceRegardless is gain_exp_regardless (limits.c:334): experience
// with none of the limits.
//
// No per-kill cap, no tenth-of-a-band cap, and — the part that matters — it
// levels all the way to LVL_IMPL rather than stopping at the mortal ceiling.
// It exists for `advance`, which is how somebody becomes an immortal in the
// first place, and it is the only route past level 30.
//
// Note it never levels *down*: handing it a negative number takes the
// experience away and leaves the level where it was. `advance` demoting
// somebody sets the level itself first, for exactly that reason.
func GainExperienceRegardless(rec *PlayerRecord, gain int32, r *rng.Rand) int32 {
	rec.Points.Exp += gain
	if rec.Points.Exp < 0 {
		rec.Points.Exp = 0
	}
	if rec.Mobile {
		return 0
	}

	levels := int32(0)
	for rec.Level < LevelImplementor &&
		rec.Points.Exp >= LevelExperience(rec.Class, rec.Level+1) {
		rec.Level++
		levels++
		AdvanceLevel(rec, r)
	}
	if levels > 0 {
		rec.Title = Title(rec.Class, rec.Level, rec.Sex)
	}
	return levels
}
