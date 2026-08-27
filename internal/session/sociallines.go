// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package session

// Where each social sits in the C's command table.
//
// A social is not a command written in Go — it is a row in `data/misc/socials`
// and a row in `interpreter.c` pointing at `do_action`. The file supplies the
// words; this supplies the *position*, which is what decides that `sm` is
// `smile` and `sn` is `snicker`.
//
// Extracted from interpreter.c and checked against it by a test, because a
// hundred and six line numbers typed by hand is a hundred and six chances to
// move somebody's abbreviation.
var socialLines = map[string]int{
	"accuse":   227,
	"applaud":  228,
	"bounce":   234,
	"beg":      239,
	"bleed":    240,
	"blush":    241,
	"bow":      242,
	"brb":      243,
	"burp":     245,
	"cackle":   250,
	"chuckle":  252,
	"clap":     253,
	"comfort":  259,
	"comb":     260,
	"cough":    263,
	"cringe":   265,
	"cry":      266,
	"cuddle":   267,
	"curse":    268,
	"curtsey":  269,
	"dance":    271,
	"daydream": 273,
	"drool":    281,
	"embrace":  287,
	"fart":     295,
	"flip":     298,
	"flirt":    299,
	"fondle":   301,
	"french":   303,
	"frown":    304,
	"fume":     305,
	"gasp":     308,
	"giggle":   311,
	"glare":    312,
	"greet":    319,
	"grin":     320,
	"groan":    321,
	"grope":    322,
	"grovel":   323,
	"growl":    324,
	"hiccup":   331,
	"hop":      337,
	"hug":      339,
	"kiss":     353,
	"laugh":    356,
	"lick":     361,
	"love":     364,
	"moan":     366,
	"massage":  369,
	"nibble":   375,
	"nod":      376,
	"nudge":    387,
	"nuzzle":   388,
	"pat":      397,
	"peer":     400,
	"point":    402,
	"poke":     403,
	"ponder":   405,
	"pout":     409,
	"pray":     412,
	"puke":     413,
	"punch":    414,
	"purr":     415,
	"roll":     445,
	"ruffle":   447,
	"scream":   453,
	"shake":    459,
	"shiver":   460,
	"shrug":    462,
	"sigh":     465,
	"sing":     466,
	"slap":     471,
	"smile":    473,
	"smirk":    474,
	"snicker":  475,
	"snap":     476,
	"snarl":    477,
	"sneeze":   478,
	"sniff":    480,
	"snore":    481,
	"snowball": 482,
	"snuggle":  484,
	"spank":    487,
	"spit":     488,
	"squeeze":  489,
	"stare":    491,
	"steam":    494,
	"stroke":   495,
	"strut":    496,
	"sulk":     497,
	"tackle":   502,
	"tango":    504,
	"taunt":    505,
	"thank":    509,
	"think":    510,
	"tickle":   513,
	"twiddle":  519,
	"wave":     537,
	"whine":    544,
	"whistle":  545,
	"wiggle":   547,
	"wink":     549,
	"worship":  556,
	"yawn":     559,
	"yodel":    560,
}

// socialLevels is the minimum level of any social that has one.
//
// Exactly one does: `snowball` is LVL_IMMORT (interpreter.c:482), which in the
// stock world is the joke it sounds like — a god throwing snow at mortals.
// Every other do_action row is 0 or 1. It is a map rather than a field on
// socialLines because one exception does not justify changing a hundred and
// six rows, and TestEveryCommandsMinimumLevelMatchesTheCSource is what would
// notice a second one appearing.
var socialLevels = map[string]int32{
	"snowball": 31, // game.LevelImmortal
}
