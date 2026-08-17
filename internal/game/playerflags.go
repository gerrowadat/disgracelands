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

// The ROOM_* bits, from structs.h:75.
//
// The last three are local additions, and the first of them is the reason
// this list is here at all: ROOM_GOOD_REGEN doubles every kind of
// regeneration, which is not something a port can afford to leave out.
const (
	RoomDark       Flags = 1 << 0
	RoomDeathTrap  Flags = 1 << 1
	RoomNoMob      Flags = 1 << 2
	RoomIndoors    Flags = 1 << 3
	RoomPeaceful   Flags = 1 << 4
	RoomSoundproof Flags = 1 << 5
	RoomNoTrack    Flags = 1 << 6
	RoomNoMagic    Flags = 1 << 7
	RoomTunnel     Flags = 1 << 8
	RoomPrivate    Flags = 1 << 9
	RoomGodRoom    Flags = 1 << 10
	RoomHouse      Flags = 1 << 11
	RoomHouseCrash Flags = 1 << 12
	RoomAtrium     Flags = 1 << 13
	RoomOLC        Flags = 1 << 14
	RoomBFSMark    Flags = 1 << 15

	// Local additions (structs.h:92).
	RoomGoodRegen Flags = 1 << 16
	RoomCanQuit   Flags = 1 << 17
	RoomPKill     Flags = 1 << 18
)

// The AFF_* bits, from structs.h:247.
//
// AFF_HOLY_SHIELD and AFF_SILENCE are local additions; the comment beside
// the first of them in the C still says "Room for future expansion", which is
// what the slot was before somebody used it.
const (
	AffectBlind       Flags = 1 << 0
	AffectInvisible   Flags = 1 << 1
	AffectDetectAlign Flags = 1 << 2
	AffectDetectInvis Flags = 1 << 3
	AffectDetectMagic Flags = 1 << 4
	AffectSenseLife   Flags = 1 << 5
	AffectWaterwalk   Flags = 1 << 6
	AffectSanctuary   Flags = 1 << 7
	AffectGroup       Flags = 1 << 8
	AffectCurse       Flags = 1 << 9
	AffectInfravision Flags = 1 << 10
	AffectPoison      Flags = 1 << 11
	AffectProtectEvil Flags = 1 << 12
	AffectProtectGood Flags = 1 << 13
	AffectSleep       Flags = 1 << 14
	AffectNoTrack     Flags = 1 << 15
	AffectUnused16    Flags = 1 << 16
	AffectHolyShield  Flags = 1 << 17
	AffectSneak       Flags = 1 << 18
	AffectHide        Flags = 1 << 19
	AffectSilence     Flags = 1 << 20
	AffectCharm       Flags = 1 << 21
)
