// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import "time"

// PlayerRecord is a saved character, in a form no file format owns.
//
// This is the type every player-file format converts to and from, and keeping
// it free of any one format's idiosyncrasies is what makes the formats
// pluggable at all. The binary format's twenty-byte name field, its
// ten-byte password field, its 201-entry skill array and its 32 fixed affect
// slots are all facts about *that* format; none of them appear here. A format
// that cannot represent what is here says so through its capabilities rather
// than by silently truncating.
//
// Every numeric field has an explicit width, and times are time.Time rather
// than an integer of some era's choosing — see
// docs/design/go-port-plan.md §4.
// MaxTitleLength is MAX_TITLE_LENGTH (structs.h:536), which the C's own
// comment marks *DO*NOT*CHANGE*: it is the width of the field in char_file_u,
// so the binary format the archive is in depends on it.
const MaxTitleLength = 80

type PlayerRecord struct {
	// Name is the character's name as they typed it, with its original case.
	Name string
	// Title follows the name in the who-list ("the Apprentice of Magic").
	Title string
	// Description is what other players see on `look`.
	Description string

	Sex   Sex
	Class Class
	Race  Race
	Level int32

	// Hometown is the recall point.
	Hometown int32

	// Birth is when the character was created; LastLogon when they last
	// connected. Both are stored as 32-bit seconds in the legacy format,
	// which overflows in 2038 — see the note on Credential below for how
	// that is handled rather than inherited.
	Birth     time.Time
	LastLogon time.Time
	// Played is total time in the game.
	Played time.Duration

	// Host is the address they last connected from.
	Host string

	Credential Credential

	Weight int32
	Height int32

	// Abilities are what the character currently has; RealAbilities what they
	// rolled. The C keeps the same pair as aff_abils and real_abils, and the
	// distinction is load-bearing: affect_total recomputes the first from the
	// second every time an affect is added or removed, which is the only way
	// to get the arithmetic right when they can come and go in any order.
	Abilities     Abilities
	RealAbilities Abilities

	Points Points

	// The unaffected values the same reset restores from. An affect adds to
	// the live figure in Points; these are what it adds to.
	RealArmor        int32
	RealHitRoll      int32
	RealDamRoll      int32
	RealMaxHit       int32
	RealMaxMana      int32
	RealMaxMove      int32
	RealSavingThrows [5]int32
	// BaseAffectFlags are the flags a character has without any spell on
	// them — a mobile's from its prototype, a player's usually none.
	BaseAffectFlags AffectFlags

	// Worn points at the equipment of the character this record belongs to,
	// and is nil for a record that is not in the world. Not saved.
	//
	// affect_total reads a character's equipment, and every path that
	// recomputes — a spell landing, an affect wearing off, a shield going
	// on — has to see the same equipment or the totals disagree. The C gets
	// this for free because it works from the character; here the record
	// needs a way back.
	Worn *[NumWears]*Object
	// Mobile marks a mobile's record. The C tests IS_NPC, which is a bit in
	// the same flags field as everything else; here the distinction is
	// explicit because the two are clamped differently — see
	// RecomputeAffects.
	Mobile bool

	Alignment int32
	// LastTell is the IDNum of whoever last told them something, which is
	// what `reply` answers. Remembered by identity rather than by pointer, so
	// it survives that person logging out and back in. Not saved: the C keeps
	// it on char_special_data, which is runtime state.
	LastTell int64

	// IDNum is the character's permanent identity, referenced by mail,
	// houses and follower lists.
	IDNum int64

	// PlayerFlags are the PLR_* bits: killer, thief, frozen, deleted.
	PlayerFlags PlayerFlags
	// AffectFlags are the AFF_* bits: the spells currently on them.
	AffectFlags AffectFlags
	// Preferences are the PRF_* bits: brief mode, autoexit, colour.
	Preferences Preferences

	// SavingThrows are the five saving-throw bonuses.
	SavingThrows [5]int32

	// Skills maps a skill or spell number to its learned percentage. A map
	// rather than an array because the format's fixed 201 slots are the
	// format's business, and most characters know a handful.
	Skills map[int32]int32

	// Affects are the spells currently on the character.
	Affects []Affect

	// Aliases are the character's own `alias` command definitions
	// (interpreter.c's alias_data, do_alias). Ordered newest-first, matching
	// GET_ALIASES' prepend-on-add (interpreter.c:743) — `alias` with no
	// arguments lists them in that order, and a name can legitimately be
	// redefined, so which entry a lookup finds is meaningful. NPCs never
	// have any: IS_NPC(ch) returns before do_alias or perform_alias touch
	// GET_ALIASES at all (interpreter.c:699, :818), so this is nil for a
	// mobile's record rather than merely unused.
	Aliases []Alias

	// Conditions are drunk, full and thirsty, in that order. -1 means the
	// condition does not apply, which is how immortals are stored.
	Conditions [3]int32

	// DamageDice and DamageSize are a mobile's bare-hand attack, from the
	// mobile file. Meaningless for a player, who does number(0, 2).
	DamageDice int32
	DamageSize int32

	// WimpLevel is the hit-point threshold below which they flee.
	WimpLevel int32
	// FreezeLevel is the level of the god who froze them, or 0.
	FreezeLevel int32
	// InvisLevel is the level below which they cannot be seen.
	InvisLevel int32

	// PoofIn and PoofOut are what the room is told when a god arrives and
	// leaves. They live *outside* `player_special_data_saved` in the C
	// (structs.h:899), so they are not in the player file and do not survive
	// a reboot — every god who set one had to set it again next time the
	// server came up.
	PoofIn  string
	PoofOut string
	// LoadRoom is where they reappear, or NoRoom for the default.
	LoadRoom RoomVnum
	// BadPasswords counts consecutive failed logins.
	BadPasswords int32
	// SpellsToLearn is remaining practice sessions.
	SpellsToLearn int32
	// RemortVector is the Disgracelands multiclass bitmask: one bit per
	// class the character has remorted through. See
	// docs/investigations/non-stock-features.md — it is the headline local
	// feature and touches every IS_<CLASS> check in the game.
	RemortVector RemortClasses
	// SpecFlags and OLCZone are local additions carried in what were spare
	// slots; OLCZone is the zone a builder is permitted to edit.
	SpecFlags int32
	OLCZone   int32
}

// Abilities are the six rolled statistics, plus the exceptional-strength
// percentile that only warriors of strength 18 have.
type Abilities struct {
	Strength           int32
	StrengthPercentile int32
	Intelligence       int32
	Wisdom             int32
	Dexterity          int32
	Constitution       int32
	Charisma           int32
}

// Points are the character's current and maximum pools and their combat
// modifiers.
type Points struct {
	Mana, MaxMana int32
	Hit, MaxHit   int32
	Move, MaxMove int32

	Armor int32

	Gold     int32
	BankGold int32
	Exp      int32

	HitRoll int32
	DamRoll int32
}

// Affect is one spell or effect currently on a character.
type Affect struct {
	// Type is the spell number that caused it.
	Type int32
	// Duration is how many ticks remain.
	Duration int32
	// Modifier is added to whatever Location names.
	Modifier int32
	// Location is the APPLY_* constant the modifier applies to.
	Location Apply
	// Bits are the AFF_* flags the affect sets while it lasts.
	Bits AffectFlags
}

// Alias is one `alias` command definition — interpreter.c's alias_data,
// minus the a->type (ALIAS_SIMPLE/ALIAS_COMPLEX) field, which is not stored
// state: it is recomputed from Replacement's content every time (whether it
// contains ';' or '$'), the same way the C derives it at alias-creation time
// (interpreter.c:735-738) rather than keeping it as an independent fact that
// could disagree with the string it describes.
type Alias struct {
	// Name is the word that triggers the alias — what do_alias calls "arg".
	Name string
	// Replacement is the substituted text: a plain command for a simple
	// alias, or a template using ';' to separate multiple commands and
	// '$1'-'$9'/'$*' for positional/whole-line substitution for a complex
	// one. See internal/session's alias expansion.
	//
	// **It always begins with a space**, and that is an invariant rather
	// than an accident. do_alias builds a replacement with any_one_arg,
	// which — unlike one_argument — does not skip the whitespace it stops
	// on, so the space separating the alias's name from the rest of the
	// line is part of the value the C holds in memory. This port keeps it
	// for the same reason the C's own behaviour depends on it: $*
	// substitutes the raw untrimmed remainder. The legacy alias file
	// stores the replacement one character shorter and puts the space back
	// on the way in (alias.c:22, :55), so a value without one loses its
	// first character there — which internal/persist/player/binary now
	// refuses rather than does (#242).
	Replacement string
}

// Credential is how a character proves who they are.
//
// The legacy format stores a DES crypt(3) hash, salted with the character's
// own name and truncated to ten stored characters — which means only the
// first eight characters of a password ever mattered, and the salt was
// public. That has to be readable for the 2001-2008 roster to log in at all,
// but it must not be what the server keeps writing, so the scheme is explicit
// rather than assumed.
type Credential struct {
	// Scheme names the algorithm. An empty scheme means no password is set.
	Scheme CredentialScheme
	// Hash is the encoded hash, in whatever form Scheme implies.
	Hash string
}

// CredentialScheme identifies a password hashing algorithm.
type CredentialScheme string

const (
	// SchemeNone means no password has been set.
	SchemeNone CredentialScheme = ""
	// SchemeLegacyDES is the original crypt(3) DES hash. Verify-only: the
	// server never writes a new one.
	SchemeLegacyDES CredentialScheme = "des"
	// SchemeArgon2id is what a successful login upgrades to.
	SchemeArgon2id CredentialScheme = "argon2id"
)

// NeedsUpgrade reports whether this credential should be rehashed the next
// time the password is known — that is, on a successful login.
func (c Credential) NeedsUpgrade() bool {
	return c.Scheme == SchemeLegacyDES
}
