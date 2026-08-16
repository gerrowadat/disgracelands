// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

// The PLR_* and PRF_* bits, from structs.h:176 and structs.h:221.
//
// These are written into every player record, so the numbers are the file
// format and not merely an enum. They are listed in full rather than as the
// handful currently read: a partial list is an invitation to invent a value
// for the next one needed, and inventing one here would corrupt records the
// C server can still read.

// PlayerFlags: the PLR_* bits.
const (
	PlayerKiller     Flags = 1 << 0
	PlayerThief      Flags = 1 << 1
	PlayerFrozen     Flags = 1 << 2
	PlayerDontSet    Flags = 1 << 3 // the ISNPC bit; never set on a player
	PlayerWriting    Flags = 1 << 4
	PlayerMailing    Flags = 1 << 5
	PlayerCrash      Flags = 1 << 6
	PlayerSiteOK     Flags = 1 << 7
	PlayerNoShout    Flags = 1 << 8
	PlayerNoTitle    Flags = 1 << 9
	PlayerDeleted    Flags = 1 << 10
	PlayerLoadRoom   Flags = 1 << 11
	PlayerNoWizList  Flags = 1 << 12
	PlayerNoDelete   Flags = 1 << 13
	PlayerInvisStart Flags = 1 << 14
	PlayerCryo       Flags = 1 << 15
	PlayerNotDeadYet Flags = 1 << 16
	// PlayerBanned is a local addition (structs.h:194).
	PlayerBanned Flags = 1 << 17
)

// Preferences: the PRF_* bits.
const (
	PrefBrief       Flags = 1 << 0
	PrefCompact     Flags = 1 << 1
	PrefDeaf        Flags = 1 << 2
	PrefNoTell      Flags = 1 << 3
	PrefDisplayHP   Flags = 1 << 4
	PrefDisplayMana Flags = 1 << 5
	PrefDisplayMove Flags = 1 << 6
	PrefAutoExit    Flags = 1 << 7
	PrefNoHassle    Flags = 1 << 8
	PrefQuest       Flags = 1 << 9
	PrefSummonable  Flags = 1 << 10
	PrefNoRepeat    Flags = 1 << 11
	PrefHolylight   Flags = 1 << 12
	// PrefColour1 and PrefColour2 are a two-bit level, not two switches:
	// neither is off, both is full colour.
	PrefColour1   Flags = 1 << 13
	PrefColour2   Flags = 1 << 14
	PrefNoWiz     Flags = 1 << 15
	PrefLog1      Flags = 1 << 16
	PrefLog2      Flags = 1 << 17
	PrefNoAuct    Flags = 1 << 18
	PrefNoGoss    Flags = 1 << 19
	PrefNoGratz   Flags = 1 << 20
	PrefRoomFlags Flags = 1 << 21
	// PrefClearScreen is OasisOLC's.
	PrefClearScreen Flags = 1 << 22
)
