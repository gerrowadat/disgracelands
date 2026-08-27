// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import "github.com/gerrowadat/disgracelands/internal/rng"

// The weather, ported from weather.c.
//
// It is a barometer and a sky, and the barometer does a random walk. Nothing
// in the game reads the pressure except the sky, and nothing reads the sky
// except the four messages below — the C has no rain that puts out torches
// and no lightning that hurts anybody. So this looks ignorable, and is not,
// for a reason that has nothing to do with weather:
//
// **`weather_change` rolls dice, on a timer, forever.** Five draws every mud
// hour, sometimes six — `dice(1, 4) + dice(2, 6) - dice(2, 6)`, plus a
// conditional `dice(1, 4)` in the sky switch — from `weather_and_time(1)`
// every 75 real seconds (comm.c:934). A server that does not roll them walks
// out of step with one that does, and stays out of step. That is what the
// session-parity harness is comparing when it compares anything random at
// all, and it is why an unported barometer made `flee` pick a different exit
// on the two servers.

// Sky is what the weather looks like from outdoors.
type Sky int32

// The four skies, from structs.h.
const (
	SkyCloudless Sky = 0
	SkyCloudy    Sky = 1
	SkyRaining   Sky = 2
	SkyLightning Sky = 3
)

// Weather is weather_info: a barometer, the direction it is moving, and the
// sky that follows from it.
//
// The sunlight the C also keeps here is derived from the hour instead
// (SunlightAt), since this port computes mud time from elapsed real time
// rather than incrementing a counter every pulse.
type Weather struct {
	Pressure int32
	Change   int32
	Sky      Sky
}

// InitWeather is the weather half of reset_time (db.c), run once at boot.
//
// The month decides how much of a range the barometer starts in: `dice(1, 50)`
// for months 7 to 12 and `dice(1, 80)` otherwise. One draw either way, and it
// is the first draw the C server ever takes — which makes it the one that
// offsets everything afterwards if it is missing.
func InitWeather(t MudTime, r *rng.Rand) Weather {
	w := Weather{Pressure: 960}
	if t.Month >= 7 && t.Month <= 12 {
		w.Pressure += r.Dice(1, 50)
	} else {
		w.Pressure += r.Dice(1, 80)
	}

	switch {
	case w.Pressure <= 980:
		w.Sky = SkyLightning
	case w.Pressure <= 1000:
		w.Sky = SkyRaining
	case w.Pressure <= 1020:
		w.Sky = SkyCloudy
	default:
		w.Sky = SkyCloudless
	}
	return w
}

// ChangeWeather is weather_change (weather.c:80): move the barometer, then
// see whether the sky has to follow. It returns whatever should be said to
// everybody outdoors, in order.
//
// The number of draws it takes is not fixed — five always, and a sixth when
// the sky switch reaches one of its `dice(1, 4)` tests — so a port that
// approximated this with "roll five" would drift anyway, just more slowly.
func (w *Weather) ChangeWeather(t MudTime, r *rng.Rand) []string {
	// Which way the barometer is being pushed. The month range here (9 to 16)
	// is not the same as InitWeather's (7 to 12) and there is no sign that is
	// deliberate; both are reproduced as written.
	diff := int32(2)
	if t.Month >= 9 && t.Month <= 16 {
		if w.Pressure > 985 {
			diff = -2
		}
	} else if w.Pressure > 1015 {
		diff = -2
	}

	w.Change += r.Dice(1, 4)*diff + r.Dice(2, 6) - r.Dice(2, 6)
	w.Change = min(w.Change, 12)
	w.Change = max(w.Change, -12)

	w.Pressure += w.Change
	w.Pressure = min(w.Pressure, 1040)
	w.Pressure = max(w.Pressure, 960)

	// The sky's own switch, whose cases are the C's numbered `change` values.
	// Nought means "no change", and the numbering is not ordered by anything
	// — 3 is "the clouds disappear" and 5 is "the rain stops".
	var change int
	switch w.Sky {
	case SkyCloudless:
		if w.Pressure < 990 {
			change = 1
		} else if w.Pressure < 1010 && r.Dice(1, 4) == 1 {
			change = 1
		}
	case SkyCloudy:
		switch {
		case w.Pressure < 970:
			change = 2
		case w.Pressure < 990:
			if r.Dice(1, 4) == 1 {
				change = 2
			}
		case w.Pressure > 1030:
			if r.Dice(1, 4) == 1 {
				change = 3
			}
		}
	case SkyRaining:
		switch {
		case w.Pressure < 970:
			if r.Dice(1, 4) == 1 {
				change = 4
			}
		case w.Pressure > 1030:
			change = 5
		case w.Pressure > 1010:
			if r.Dice(1, 4) == 1 {
				change = 5
			}
		}
	case SkyLightning:
		if w.Pressure > 1010 {
			change = 6
		} else if w.Pressure > 990 && r.Dice(1, 4) == 1 {
			change = 6
		}
	default:
		w.Sky = SkyCloudless
	}

	switch change {
	case 1:
		w.Sky = SkyCloudy
		return []string{"The sky starts to get cloudy.\r\n"}
	case 2:
		w.Sky = SkyRaining
		return []string{"It starts to rain.\r\n"}
	case 3:
		w.Sky = SkyCloudless
		return []string{"The clouds disappear.\r\n"}
	case 4:
		w.Sky = SkyLightning
		return []string{"Lightning starts to show in the sky.\r\n"}
	case 5:
		w.Sky = SkyCloudy
		return []string{"The rain stops.\r\n"}
	case 6:
		w.Sky = SkyRaining
		return []string{"The lightning stops.\r\n"}
	}
	return nil
}

// SunriseMessage is another_hour's own half of the same pulse
// (weather.c:38): four announcements, at four specific hours, to everybody
// outdoors.
//
// It rolls nothing, so it is not what the generators care about — but it is
// output the C produces every mud day and this port produced never, which
// would show up in any scenario long enough to cross one of these hours.
func SunriseMessage(hour int32) string {
	switch hour {
	case 5:
		return "The sun rises in the east.\r\n"
	case 6:
		return "The day has begun.\r\n"
	case 21:
		return "The sun slowly disappears in the west.\r\n"
	case 22:
		return "The night has begun.\r\n"
	}
	return ""
}
