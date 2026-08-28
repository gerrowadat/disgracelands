// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import "github.com/gerrowadat/disgracelands/internal/colour"

// send_to_all_color (comm.c:2256) and the four `<DoC>` broadcasts that use
// it. They are the loudest thing in the game: not a channel anybody joined,
// not a room anybody is standing in, but a line to every player online at
// once, and they are most of what the mud sounded like. Somebody levelling
// was an event everybody saw.

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
func (l *Live) Announce(want colour.Level, format string, args ...any) {
	for _, c := range l.Players() {
		if c.Record != nil && c.Record.PlayerFlags.Has(PlayerWriting) {
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
	l.Announce(colour.Normal,
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
		l.Announce(colour.Normal,
			"{{cyan}}A voice whispers in your ear, '%s has gained a level!'%s{{/}}", who.Name, tail)
		return
	}
	who.Tell("You rise %d levels!\r\n", levels)
	l.Announce(colour.Normal,
		"{{cyan}}A voice whispers in your ear, '%s has gained %d levels!!!'%s{{/}}",
		who.Name, levels, tail)
}
