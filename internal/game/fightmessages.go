// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/gerrowadat/disgracelands/internal/rng"
)

// misc/messages, ported from load_messages and skill_message (fight.c).
//
// No I/O lives here — internal/server reads the file and hands this
// package a reader, the same split game.ParseSocials/ParseHelpFile
// already have.

// MsgSet is msg_type (structs.h:1055-1058): the three audiences one
// situation (a death, a miss, a hit, a god-hit) can have a message for.
// An empty field is the C's '#' — no message for that audience, not an
// empty string sent.
type MsgSet struct {
	Attacker, Victim, Room string
}

// FightMessage is one 'M' record: an attack type (a spell/skill number,
// or TYPE_HIT+n for a weapon type — misc/messages mixes both) and its
// four situations.
type FightMessage struct {
	AttackType int32
	Die        MsgSet
	Miss       MsgSet
	Hit        MsgSet
	God        MsgSet
}

// ParseMessagesFile parses misc/messages, porting load_messages
// (fight.c:145-193). A record is 'M', a type number, then exactly twelve
// lines in die/miss/hit/god order, each attacker/victim/room — checked by
// first character only ('M' starts a record, '*' or a blank line between
// records is a comment), matching load_help's own single-character checks
// rather than a full-line comparison.
func ParseMessagesFile(r io.Reader) ([]FightMessage, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 4096), 1<<20)

	line, ok, err := messagesLine(sc)
	if err != nil {
		return nil, err
	}
	for ok && (line == "" || strings.HasPrefix(line, "*")) {
		line, ok, err = messagesLine(sc)
		if err != nil {
			return nil, err
		}
	}

	var records []FightMessage
	for ok && strings.HasPrefix(line, "M") {
		numLine, ok2, err := messagesLine(sc)
		if err != nil {
			return nil, err
		}
		if !ok2 {
			return nil, fmt.Errorf("fight messages: unexpected EOF reading a type number")
		}
		attackType, err := strconv.ParseInt(strings.TrimSpace(numLine), 10, 32)
		if err != nil {
			return nil, fmt.Errorf("fight messages: %q: %w", numLine, err)
		}

		sets := make([]MsgSet, 4)
		for i := range sets {
			var s MsgSet
			s.Attacker, err = messagesAction(sc)
			if err != nil {
				return nil, err
			}
			s.Victim, err = messagesAction(sc)
			if err != nil {
				return nil, err
			}
			s.Room, err = messagesAction(sc)
			if err != nil {
				return nil, err
			}
			sets[i] = s
		}
		records = append(records, FightMessage{
			AttackType: int32(attackType), //nolint:gosec // spell/skill/attack numbers are small
			Die:        sets[0], Miss: sets[1], Hit: sets[2], God: sets[3],
		})

		line, ok, err = messagesLine(sc)
		if err != nil {
			return nil, err
		}
		for ok && (line == "" || strings.HasPrefix(line, "*")) {
			line, ok, err = messagesLine(sc)
			if err != nil {
				return nil, err
			}
		}
	}
	return records, nil
}

// messagesAction reads one action line, porting fread_action
// (act.social.c:178-192): '#' alone means no message for this audience,
// anything else is the message verbatim.
func messagesAction(sc *bufio.Scanner) (string, error) {
	line, ok, err := messagesLine(sc)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("fight messages: unexpected EOF reading a message line")
	}
	if strings.HasPrefix(line, "#") {
		return "", nil
	}
	return line, nil
}

func messagesLine(sc *bufio.Scanner) (line string, ok bool, err error) {
	if !sc.Scan() {
		return "", false, sc.Err()
	}
	return strings.TrimRight(sc.Text(), "\r"), true, nil
}

// FightMessages is the loaded table, keyed by attack type for
// skill_message's lookup (fight.c:703-...).
type FightMessages struct {
	// byType holds each type's variants in the order dice(1,n) expects:
	// reversed from file order, because load_messages *prepends* each new
	// record for a type it has already seen
	// (`messages->next = fight_messages[i].msg; fight_messages[i].msg =
	// messages;`, fight.c:174-176) — so the C's own list has the
	// *last*-read record for a type first. Matching that order, not just
	// picking "a" variant, is what makes a given RNG roll land on the
	// same text the C would show for it.
	byType map[int32][]FightMessage
}

// NewFightMessages builds the lookup table from a parsed file.
func NewFightMessages(records []FightMessage) *FightMessages {
	byType := make(map[int32][]FightMessage)
	for _, rec := range records {
		// Prepend: the last record read for a type ends up first.
		byType[rec.AttackType] = append([]FightMessage{rec}, byType[rec.AttackType]...)
	}
	return &FightMessages{byType: byType}
}

// Pick chooses one of an attack type's registered variants, porting
// skill_message's own selection (fight.c:710-712): `dice(1,
// number_of_attacks)`, walked into the (reversed-into-file-order) list.
// Reports false if the type has no entries at all — skill_message's own
// "found nothing", which is how damage()'s dam_message fallback decides
// to run.
func (f *FightMessages) Pick(attackType int32, r *rng.Rand) (FightMessage, bool) {
	if f == nil {
		return FightMessage{}, false
	}
	variants := f.byType[attackType]
	if len(variants) == 0 {
		return FightMessage{}, false
	}
	n := r.Dice(1, int32(len(variants))) //nolint:gosec // variant counts are tiny
	return variants[n-1], true
}
