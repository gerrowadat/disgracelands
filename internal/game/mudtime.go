// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import (
	"fmt"
	"time"
)

// The mud calendar, ported from mud_time_passed (utils.c), do_time
// (act.informative.c) and the name tables in constants.c.
//
// A mud year is seventeen months of thirty-five days, and a day is
// twenty-four hours of seventy-five real seconds — so a year passes in a
// little over eleven real days, and the weekday is computed rather than
// stored.

// weekdayNames are weekdays[] (constants.c:760). Seven of them across a
// thirty-five day month, which divides evenly — so the first of every month
// is the same weekday.
var weekdayNames = [7]string{
	"the Day of the Moon",
	"the Day of the Bull",
	"the Day of the Deception",
	"the Day of Thunder",
	"the Day of Freedom",
	"the Day of the Great Gods",
	"the Day of the Sun",
}

// monthNames are month_name[] (constants.c:772).
var monthNames = [17]string{
	"Month of Winter",
	"Month of the Winter Wolf",
	"Month of the Frost Giant",
	"Month of the Old Forces",
	"Month of the Grand Struggle",
	"Month of the Spring",
	"Month of Nature",
	"Month of Futility",
	"Month of the Dragon",
	"Month of the Sun",
	"Month of the Heat",
	"Month of the Battle",
	"Month of the Dark Shades",
	"Month of the Shadows",
	"Month of the Long Shadows",
	"Month of the Ancient Darkness",
	"Month of the Great Evil",
}

// MudTime is a moment on the mud calendar.
type MudTime struct {
	Hours int32
	// Day is zero-based, as the C stores it; do_time adds one to display it.
	Day   int32
	Month int32
	Year  int32
}

// TimePassed converts an elapsed duration into mud time, porting
// mud_time_passed (utils.c).
func TimePassed(elapsed time.Duration) MudTime {
	seconds := int64(elapsed.Seconds())
	if seconds < 0 {
		seconds = 0
	}

	var t MudTime
	t.Hours = int32((seconds / SecondsPerMudHour) % 24)
	seconds -= SecondsPerMudHour * int64(t.Hours)

	t.Day = int32((seconds / SecondsPerMudDay) % 35)
	seconds -= SecondsPerMudDay * int64(t.Day)

	t.Month = int32((seconds / SecondsPerMudMonth) % 17)
	seconds -= SecondsPerMudMonth * int64(t.Month)

	// A year is eleven real days, so this cannot overflow from any uptime a
	// server will see; clamped anyway rather than wrapping into a negative
	// year.
	years := seconds / SecondsPerMudYear
	if years > int64(^uint32(0)>>1) {
		years = int64(^uint32(0) >> 1)
	}
	t.Year = int32(years) //nolint:gosec // clamped to the int32 range above
	return t
}

// Seconds is mud_time_to_secs's summation half (utils.c:353-361): the real
// seconds this mud-time's four components represent. It is *not* generally
// the elapsed duration TimePassed computed them from — each field there
// discards its remainder (an hour's worth of seconds, a day's, and so on),
// so round-tripping a duration through TimePassed and back through Seconds
// loses up to SecondsPerMudHour-1 of it. mud_time_to_secs itself then goes
// on to subtract this from time(NULL) to produce a fresh epoch
// (utils.c:362); that half belongs where "now" is meaningful, on
// [Live.SavedEpoch].
func (t MudTime) Seconds() int64 {
	return int64(t.Year)*SecondsPerMudYear + int64(t.Month)*SecondsPerMudMonth +
		int64(t.Day)*SecondsPerMudDay + int64(t.Hours)*SecondsPerMudHour
}

// AgeOf is a character's age broken down the way age() (utils.c:366) returns
// it: years, months, days and hours on the mud calendar, with seventeen years
// added because every player starts at seventeen.
//
// `score` shows only the year, but `identify` shows all four, which is the
// only place in the game you can find out what hour of what day somebody was
// rolled up.
func AgeOf(rec *PlayerRecord, now time.Time) MudTime {
	if rec == nil || rec.Birth.IsZero() {
		return MudTime{Year: startingAge}
	}
	age := TimePassed(now.Sub(rec.Birth))
	age.Year += startingAge
	return age
}

// Weekday returns the day's name. Computed from the month and day rather than
// counted, so it never drifts.
func (t MudTime) Weekday() string {
	day := t.Day + 1
	return weekdayNames[((35*t.Month)+day)%7]
}

// MonthName returns the month's name.
func (t MudTime) MonthName() string {
	if t.Month < 0 || int(t.Month) >= len(monthNames) {
		return monthNames[0]
	}
	return monthNames[t.Month]
}

// Clock is the "It is N o'clock am/pm" line, porting the first half of
// do_time.
func (t MudTime) Clock() string {
	hour := t.Hours % 12
	if hour == 0 {
		hour = 12
	}
	meridiem := "am"
	if t.Hours >= 12 {
		meridiem = "pm"
	}
	return fmt.Sprintf("It is %d o'clock %s, on %s\r\n", hour, meridiem, t.Weekday())
}

// Date is the "The Nth Day of the ..." line.
func (t MudTime) Date() string {
	day := t.Day + 1
	return fmt.Sprintf("The %d%s Day of the %s, Year %d.\r\n",
		day, ordinalSuffix(day), t.MonthName(), t.Year)
}

// ordinalSuffix returns "st", "nd", "rd" or "th".
//
// The teens are the trap, and the C carries a comment crediting two separate
// people with fixing it: 11, 12 and 13 take "th" despite ending in 1, 2 and
// 3. The `(day % 100) / 10 != 1` test is what handles them.
func ordinalSuffix(day int32) string {
	if (day%100)/10 != 1 {
		switch day % 10 {
		case 1:
			return "st"
		case 2:
			return "nd"
		case 3:
			return "rd"
		}
	}
	return "th"
}

// Sunlight is where the sun is, from structs.h's SUN_* constants.
type Sunlight int

// The four states of the day.
const (
	SunDark Sunlight = iota
	SunRise
	SunLight
	SunSet
)

// SunlightAt returns the state of the day at an hour, porting the switch in
// weather_and_time (weather.c). The day is short: sunrise at five, sunset at
// twenty-one.
func SunlightAt(hour int32) Sunlight {
	switch {
	case hour == 5:
		return SunRise
	case hour == 6:
		return SunLight
	case hour == 20:
		return SunSet
	case hour == 21:
		return SunDark
	case hour > 5 && hour < 21:
		return SunLight
	}
	return SunDark
}

// ConsiderVerdict is what `consider` says about a level difference, porting
// the ladder in do_consider.
//
// The thresholds are uneven on purpose. Ten levels below you is a chicken;
// one above needs luck; more than ten above and the game stops being polite.
func ConsiderVerdict(difference int32) string {
	switch {
	case difference <= -10:
		return "Now where did that chicken go?\r\n"
	case difference <= -5:
		return "You could do it with a needle!\r\n"
	case difference <= -2:
		return "Easy.\r\n"
	case difference <= -1:
		return "Fairly easy.\r\n"
	case difference == 0:
		return "The perfect match!\r\n"
	case difference <= 1:
		return "You would need some luck!\r\n"
	case difference <= 2:
		return "You would need a lot of luck!\r\n"
	case difference <= 3:
		return "You would need a lot of luck and great equipment!\r\n"
	case difference <= 5:
		return "Do you feel lucky, punk?\r\n"
	case difference <= 10:
		return "Are you mad!?\r\n"
	case difference <= 100:
		return "You ARE mad!\r\n"
	}
	// Past a hundred levels the C says nothing at all, which cannot happen in
	// a game that stops at 34.
	return ""
}

// HealthDiagnosis describes how hurt somebody looks, porting
// diag_char_to_char.
//
// The percentage is integer division, so a character on 99 of 100 hit points
// is at 99% and "has a few scratches" — but one on 999 of 1000 is at 99% too.
// The bands are coarse deliberately: you are not meant to be able to read an
// opponent's exact health.
func HealthDiagnosis(name string, rec *PlayerRecord) string {
	percent := int32(-1)
	if rec != nil && rec.Points.MaxHit > 0 {
		percent = (100 * rec.Points.Hit) / rec.Points.MaxHit
	}

	switch {
	case percent >= 100:
		return name + " is in excellent condition.\r\n"
	case percent >= 90:
		return name + " has a few scratches.\r\n"
	case percent >= 75:
		return name + " has some small wounds and bruises.\r\n"
	case percent >= 50:
		return name + " has quite a few wounds.\r\n"
	case percent >= 30:
		return name + " has some big nasty wounds and scratches.\r\n"
	case percent >= 15:
		return name + " looks pretty hurt.\r\n"
	case percent >= 0:
		return name + " is in awful condition.\r\n"
	}
	return name + " is bleeding awfully from big wounds.\r\n"
}
