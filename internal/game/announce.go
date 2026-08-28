// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import (
	"strings"

	"github.com/gerrowadat/disgracelands/internal/colour"
)

// send_to_all_color (comm.c:2256) and the four `<DoC>` broadcasts that use
// it. They are the loudest thing in the game: not a channel anybody joined,
// not a room anybody is standing in, but a line to every player online at
// once, and they are most of what the mud sounded like. Somebody levelling
// was an event everybody saw.

// Announcement is which of the two streams a broadcast belongs to, for
// AnnounceLevel below. Not a distinction the C draws — it has one function
// and no way to turn it down — and the split is by *frequency* rather than
// by taste, because that is the only line worth drawing here.
type Announcement int

const (
	// AnnouncementRare is the newcomer hail, the death trap and the remort.
	// Between them a handful a day on a busy server, and each one is an
	// event: somebody arrived, somebody died, somebody changed what they
	// are.
	AnnouncementRare Announcement = iota
	// AnnouncementRoutine is the level gain, and it is on its own because it
	// is the only one that fires on an ordinary kill. Everything about the
	// stream that anybody would want to turn down is this line.
	AnnouncementRoutine
)

// AnnounceLevel is how much of the broadcast stream a player hears.
//
// **A local addition. The C has no such setting** — send_to_all_color takes
// no account of the reader beyond their colour and PLR_WRITING — and it is
// recorded in docs/deviations.md. The shape is deliberately the one `color`
// and `syslog` already have: a small number in two preference bits, named,
// matched by prefix, so `announce b` is Brief exactly as `color c` is
// Complete.
type AnnounceLevel int

const (
	// AnnounceOff hears none of it.
	AnnounceOff AnnounceLevel = iota
	// AnnounceBrief hears the rare ones and not the level gains.
	AnnounceBrief
	// AnnounceAll hears everything, which is what the C does and therefore
	// what every character starts at and stays at until they say otherwise.
	AnnounceAll
)

// AnnounceLevelNames are the words `announce` matches, indexed by the level.
var AnnounceLevelNames = [...]string{"Off", "Brief", "All"}

// String names the level.
func (a AnnounceLevel) String() string {
	if a < 0 || int(a) >= len(AnnounceLevelNames) {
		return "?"
	}
	return AnnounceLevelNames[a]
}

// ParseAnnounceLevel matches a name the way `color` and `syslog` do — a
// prefix match, so `o` is Off and `a` is All.
func ParseAnnounceLevel(word string) (AnnounceLevel, bool) {
	word = strings.ToLower(strings.TrimSpace(word))
	if word == "" {
		return 0, false
	}
	for i, name := range AnnounceLevelNames {
		if strings.HasPrefix(strings.ToLower(name), word) {
			return AnnounceLevel(i), true
		}
	}
	return 0, false
}

// AnnounceLevelOf reads the level off a record.
//
// The two bits hold *suppression*, counted the way `color` counts its own
// two: low bit plus twice the high one. Zero suppression is AnnounceAll, so
// a record that has never heard of this setting — which is every record
// written before it existed — hears everything. See PrefNoAnnounce1.
//
// Three is unreachable through `announce` and is folded into Off rather than
// left as a fourth state: a hand-edited pfile should get a quiet answer, not
// a "?".
func AnnounceLevelOf(rec *PlayerRecord) AnnounceLevel {
	if rec == nil {
		return AnnounceAll
	}
	suppress := 0
	if rec.Preferences.Has(PrefNoAnnounce1) {
		suppress++
	}
	if rec.Preferences.Has(PrefNoAnnounce2) {
		suppress += 2
	}
	switch suppress {
	case 0:
		return AnnounceAll
	case 1:
		return AnnounceBrief
	default:
		return AnnounceOff
	}
}

// SetAnnounceLevel writes it back, inverting AnnounceLevelOf.
func SetAnnounceLevel(rec *PlayerRecord, level AnnounceLevel) {
	if rec == nil {
		return
	}
	suppress := 0
	switch level {
	case AnnounceBrief:
		suppress = 1
	case AnnounceOff:
		suppress = 2
	case AnnounceAll:
		suppress = 0
	}
	rec.Preferences = rec.Preferences.Clear(PrefNoAnnounce1 | PrefNoAnnounce2)
	if suppress&1 != 0 {
		rec.Preferences = rec.Preferences.Set(PrefNoAnnounce1)
	}
	if suppress&2 != 0 {
		rec.Preferences = rec.Preferences.Set(PrefNoAnnounce2)
	}
}

// Hears reports whether a record's level lets this stream through.
func (a Announcement) Hears(rec *PlayerRecord) bool {
	switch AnnounceLevelOf(rec) {
	case AnnounceOff:
		return false
	case AnnounceBrief:
		return a == AnnouncementRare
	default:
		return true
	}
}

// Announce is send_to_all_color: every playing character is told, in the
// colour given, and nobody else is.
//
// Two things it is not. It is not send_to_all (comm.c:2245), which has no
// colour and no exclusions — the C has both, and a caller that passes no
// colour is not passing KNRM. And the colour is a *threshold* on the
// reader's own COLOR_LEV rather than a decision by the sender
// (comm.c:2263): `if (COLOR_LEV(i->character) >= C_NRM)` brackets the
// message with the escape and KNRM, and somebody with `color off` gets the
// text alone. TellAt's first argument is that threshold.
//
// The exclusion is PLR_WRITING, and the C's comment for it is "Doesn't echo
// if a player is writing" — somebody halfway through a board post does not
// want the whole game shouting into their editor. That check was dead until
// #214 made the flag real; it is live here.
func (l *Live) Announce(tier Announcement, want colour.Level, format string, args ...any) {
	for _, c := range l.Players() {
		if c.Record != nil && c.Record.PlayerFlags.Has(PlayerWriting) {
			continue
		}
		// The local half: a player who has turned this stream down does not
		// get it. Everything above this line is the C's.
		if !tier.Hears(c.Record) {
			continue
		}
		c.TellAt(want, format, args...)
	}
}

// AnnounceNewPlayer is nanny's `<DoC>` hail (interpreter.c:1608-1610), sent
// the moment a character is created.
//
// The new player does not hear their own: the C walks descriptor_list for
// `STATE(i) == CON_PLAYING` and theirs is CON_RMOTD, sitting on the message
// of the day. They are not in the world here either, so Players() does not
// reach them, and the two agree without needing a special case.
//
// The C sends the newline as a *second* send_to_all_color call rather than
// putting it in the buffer, so a colour-enabled reader gets
// KCYN-text-KNRM-KCYN-CRLF-KNRM. Same text either way; one call here.
func (l *Live) AnnounceNewPlayer(name string) {
	l.Announce(AnnouncementRare, colour.Normal,
		"{{cyan}}A voice whispers in your ear, 'All hail %s, a newcomer!'{{/}}\r\n", name)
}

// AnnounceLevelGain is the world-visible tail of gain_exp's
// `if (is_altered)` block (limits.c:306-318): what the character is told,
// and what the whole game is told about them.
//
// Both halves are here because in the C they are two lines of the same block
// and every caller of gain_exp gets both. This port reaches gain_exp from
// three places — a kill (internal/server/violence.go), the cityguard's award
// (internal/session/specprocs.go) and `advance` (internal/session/
// wizchange.go, through AnnounceLevelGainRegardless below) — and before #212
// only the first of them said "You rise a level!" and none of them broadcast
// at all.
//
// The mudlog stays at the callers. internal/game has no logger and is not
// getting one; see Server.announceGain.
func (l *Live) AnnounceLevelGain(who *Character, levels int32) {
	l.announceLevelGain(who, levels, "\r\n")
}

// AnnounceLevelGainRegardless is gain_exp_regardless' copy of the same block
// (limits.c:361-370), which `advance` reaches.
//
// **The two copies are not identical, and the difference is player-visible.**
// gain_exp's whisper ends `!'\r\n` and gain_exp_regardless' ends `!'` with no
// newline at all, so on the real server an `advance` ran the whisper straight
// into whatever was written next — which, since do_advance's own "Okay." has
// already gone out and the victim's prompt comes after, is the prompt. Two
// methods rather than a boolean argument, so a call site says which of the
// C's two functions it is standing in. See docs/weirdnumbers.md.
func (l *Live) AnnounceLevelGainRegardless(who *Character, levels int32) {
	l.announceLevelGain(who, levels, "")
}

func (l *Live) announceLevelGain(who *Character, levels int32, tail string) {
	if levels <= 0 || who == nil {
		return
	}
	if levels == 1 {
		who.Tell("You rise a level!\r\n")
		l.Announce(AnnouncementRoutine, colour.Normal,
			"{{cyan}}A voice whispers in your ear, '%s has gained a level!'%s{{/}}", who.Name, tail)
		return
	}
	who.Tell("You rise %d levels!\r\n", levels)
	l.Announce(AnnouncementRoutine, colour.Normal,
		"{{cyan}}A voice whispers in your ear, '%s has gained %d levels!!!'%s{{/}}",
		who.Name, levels, tail)
}
