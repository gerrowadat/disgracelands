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

	// PrefNoAnnounce1 and PrefNoAnnounce2 are a local addition: a two-bit
	// level saying how much of the `<DoC>` broadcast stream a player wants
	// (see AnnounceLevel in announce.go). Not in the C, which has no control
	// over them at all — recorded in docs/deviations.md.
	//
	// **They count suppression, not volume, and that is deliberate.** The
	// pfile is a raw fwrite of a struct and `pref` is a `long`
	// (structs.h:858), so on the ILP32 data model the archived data uses
	// there are exactly nine spare bits here — 23 through 31 — and every
	// one of them is *clear* in every record ever written. A level where
	// zero meant "off" would therefore mute the entire roster the moment
	// this shipped. Zero means the full stream, the way PrefNoGoss and its
	// neighbours already read.
	PrefNoAnnounce1 Flags = 1 << 23
	PrefNoAnnounce2 Flags = 1 << 24
)

// RoomFlag is one of the ROOM_* bits, from structs.h:75, and RoomFlags is a
// room's set of them.
//
// The first domain to get its own type, in
// docs/proposals/idiomatic-go.md's step 1. The constants below are bit
// *indices* rather than masks — RoomDark is 0 and not 1<<0 — which is the
// only change to how they are declared and no change at all to what is
// stored: bit 0 is still bit 0 on disk, is still what `asciiflag_conv`'s
// 'a' decodes to, and is still what bitnames_test.go proves against
// room_bits[] in constants.c. §2.1.
//
// The last three are local additions, and the first of them is the reason
// this list is here at all: ROOM_GOOD_REGEN doubles every kind of
// regeneration, which is not something a port can afford to leave out.
type RoomFlag int

// RoomFlags is a set of RoomFlag.
type RoomFlags = Set[RoomFlag]

const (
	RoomDark       RoomFlag = 0
	RoomDeathTrap  RoomFlag = 1
	RoomNoMob      RoomFlag = 2
	RoomIndoors    RoomFlag = 3
	RoomPeaceful   RoomFlag = 4
	RoomSoundproof RoomFlag = 5
	RoomNoTrack    RoomFlag = 6
	RoomNoMagic    RoomFlag = 7
	RoomTunnel     RoomFlag = 8
	RoomPrivate    RoomFlag = 9
	RoomGodRoom    RoomFlag = 10
	RoomHouse      RoomFlag = 11
	RoomHouseCrash RoomFlag = 12
	RoomAtrium     RoomFlag = 13
	RoomOLC        RoomFlag = 14
	RoomBFSMark    RoomFlag = 15

	// Local additions (structs.h:92).
	RoomGoodRegen RoomFlag = 16
	RoomCanQuit   RoomFlag = 17
	RoomPKill     RoomFlag = 18
)

// The AFF_* bits, from structs.h:247.
//
// AFF_HOLY_SHIELD and AFF_SILENCE are local additions; the comment beside
// the first of them in the C still says "Room for future expansion", which is
// what the slot was before somebody used it.
//
// AffectFlag is one of them and AffectFlags is a character's or an
// object's set. Bit indices, not masks: docs/proposals/idiomatic-go.md
// §4.1, with §4.1.1 and §4.1.2 for the three ways that bites.
// affected_bits[] in constants.c is the name table.
type AffectFlag int

// AffectFlags is a set of AffectFlag.
type AffectFlags = Set[AffectFlag]

const (
	AffectBlind       AffectFlag = 0
	AffectInvisible   AffectFlag = 1
	AffectDetectAlign AffectFlag = 2
	AffectDetectInvis AffectFlag = 3
	AffectDetectMagic AffectFlag = 4
	AffectSenseLife   AffectFlag = 5
	AffectWaterwalk   AffectFlag = 6
	AffectSanctuary   AffectFlag = 7
	AffectGroup       AffectFlag = 8
	AffectCurse       AffectFlag = 9
	AffectInfravision AffectFlag = 10
	AffectPoison      AffectFlag = 11
	AffectProtectEvil AffectFlag = 12
	AffectProtectGood AffectFlag = 13
	AffectSleep       AffectFlag = 14
	AffectNoTrack     AffectFlag = 15
	AffectUnused16    AffectFlag = 16
	AffectHolyShield  AffectFlag = 17
	AffectSneak       AffectFlag = 18
	AffectHide        AffectFlag = 19
	AffectSilence     AffectFlag = 20
	AffectCharm       AffectFlag = 21
)
