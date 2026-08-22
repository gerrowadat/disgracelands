// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package session

import "github.com/gerrowadat/disgracelands/internal/game"

// The preference toggles, ported from do_gen_tog (act.other.c).
//
// One command per preference, each flipping a bit and saying which way it
// went. The messages are worth reading rather than paraphrasing: switching
// off the gossip channel is "You are now deaf to gossip", and switching
// summon protection *on* is `nosummon`, which reports "You are now safe from
// summoning" — the flag it flips is `PRF_SUMMONABLE`, so the command and the
// bit run in opposite directions.
//
// Only the toggles the communication channels need are here. The rest arrive
// with the commands they belong to.

// toggle is one preference and what to say about it.
type toggle struct {
	flag game.Flags
	// on and off are the C's tog_messages[subcmd][TOG_ON] and [TOG_OFF],
	// which are indexed by the *result* of flipping rather than by intent.
	on, off string
}

var toggles = map[string]toggle{
	"nosummon": {
		flag: game.PrefSummonable,
		on:   "You may now be summoned by other players.\r\n",
		off:  "You are now safe from summoning by other players.\r\n",
	},
	"notell": {
		flag: game.PrefNoTell,
		on:   "You are now deaf to tells.\r\n",
		off:  "You can now hear tells.\r\n",
	},
	"noauction": {
		flag: game.PrefNoAuct,
		on:   "You are now deaf to auctions.\r\n",
		off:  "You can now hear auctions.\r\n",
	},
	"noshout": {
		flag: game.PrefDeaf,
		on:   "You are now deaf to shouts.\r\n",
		off:  "You can now hear shouts.\r\n",
	},
	"nogossip": {
		flag: game.PrefNoGoss,
		on:   "You are now deaf to gossip.\r\n",
		off:  "You can now hear gossip.\r\n",
	},
	"nograts": {
		flag: game.PrefNoGratz,
		on:   "You are now deaf to the congratulation messages.\r\n",
		off:  "You can now hear the congratulation messages.\r\n",
	},
	"norepeat": {
		flag: game.PrefNoRepeat,
		on:   "You will no longer have your communication repeated.\r\n",
		off:  "You will now have your communication repeated.\r\n",
	},
	"brief": {
		flag: game.PrefBrief,
		on:   "Brief mode on.\r\n",
		off:  "Brief mode off.\r\n",
	},
	"compact": {
		flag: game.PrefCompact,
		on:   "Compact mode on.\r\n",
		off:  "Compact mode off.\r\n",
	},
	"autoexit": {
		flag: game.PrefAutoExit,
		on:   "Autoexits enabled.\r\n",
		off:  "Autoexits disabled.\r\n",
	},
	"quest": {
		flag: game.PrefQuest,
		on:   "Okay, you are part of the Quest!\r\n",
		off:  "You are no longer part of the Quest.\r\n",
	},

	// The immortal ones. Same function, same table, and the only difference is
	// the minimum level on the command-table row — so they belong here rather
	// than in `wizops.go` with the commands that need a god to do anything.
	"nohassle": {
		flag: game.PrefNoHassle,
		on:   "Nohassle enabled.\r\n",
		off:  "Nohassle disabled.\r\n",
	},
	"nowiz": {
		flag: game.PrefNoWiz,
		on:   "You are now deaf to the Wiz-channel.\r\n",
		off:  "You can now hear the Wiz-channel.\r\n",
	},
	"roomflags": {
		flag: game.PrefRoomFlags,
		on:   "You will now see the room flags.\r\n",
		off:  "You will no longer see the room flags.\r\n",
	},
	"holylight": {
		flag: game.PrefHolylight,
		on:   "HolyLight mode on.\r\n",
		off:  "HolyLight mode off.\r\n",
	},
}

// Two of do_gen_tog's seventeen are still missing, and both for the same
// reason: they flip a server-wide *global* rather than a preference
// (act.other.c:1021 and :1028). `slowns` switches reverse-DNS resolution,
// which this port does not do at all, and `trackthru` switches
// `game.TrackThroughDoors`, which the breadth-first search already reads.
//
// A global is right in the C, which is one server per process. Here the tests
// build several servers in one, each with its own world goroutine, so a
// command writing a package-level variable is a race between them rather than
// a setting. Whichever of the two lands first has to decide where the value
// lives — most likely on Live, beside the world it applies to. Recorded in
// docs/deviations.md.

// toggleCommand returns the command for one preference.
func toggleCommand(name string) func(*Context) error {
	return func(c *Context) error {
		t, ok := toggles[name]
		if !ok || c.Character.IsNPC() || c.Character.Record == nil {
			return nil
		}

		rec := c.Character.Record
		if rec.Preferences.Has(t.flag) {
			rec.Preferences = rec.Preferences.Clear(t.flag)
			c.Send("%s", t.off)
			return nil
		}
		rec.Preferences = rec.Preferences.Set(t.flag)
		c.Send("%s", t.on)
		return nil
	}
}
