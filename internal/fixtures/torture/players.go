// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/gerrowadat/disgracelands/internal/auth/descrypt"
	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/persist/player"
	"github.com/gerrowadat/disgracelands/internal/persist/player/binary"
)

// The roster half of the corpus: etc/players, plus the plrobjs/ and
// plralias/ trees that go with it, in the C's own sibling-of-etc layout
// (db.h's PLAYER_FILE against LIB_PLROBJS/LIB_PLRALIAS, both resolved from
// the server's own cwd) rather than this port's child-of-the-roster one.
// Both layouts are real and `dlctl import --type=pfile` guesses between
// them (resolveSubdir); exercising the archived one is the point.
//
// The last ILP32 timestamp is 2038-01-19T03:14:07Z, and that is the
// largest value any of these records can carry. A genuinely 2038-crossing
// timestamp is not written here because it *cannot* be: binary's
// putUnixSeconds refuses one rather than wrapping it (objfile.go:224), so
// the boundary value is the hostile case the format actually admits. That
// the year after it is unrepresentable is not a gap in this fixture; it is
// the argument for the release this fixture is being built for.

// lastILP32Second is 2038-01-19T03:14:07Z: INT32_MAX seconds after the
// epoch, the last instant a `time_t` of four signed bytes can name.
var lastILP32Second = time.Unix(0x7fffffff, 0).UTC()

// torturedCharacters are the roster, in the order they are written.
//
// Names are pure letters because they have to be: get_filename buckets a
// character's rent and alias files by the first letter of a lower-cased
// name and this port refuses anything else outright rather than writing a
// file called `../../etc/passwd` (binary/objfile.go's bucketedPath). So
// the length cases are the interesting ones, and both edges are here.
func torturedCharacters() []*game.PlayerRecord {
	return []*game.PlayerRecord{
		maximumCharacter(),
		nameAtTheLimit(),
		nameOneShortOfTheLimit(),
		levelZeroCharacter(),
	}
}

// maximumCharacter is the record that fills every field it can: all 32
// affect slots, every spare slot non-zero and distinct, a real DES hash,
// a title using every byte of its field, and the last timestamp the format
// can name.
func maximumCharacter() *game.PlayerRecord {
	r := &game.PlayerRecord{
		Name: "Torturer",
		// MAX_TITLE_LENGTH is 80 (structs.h:536, marked *DO*NOT*CHANGE*
		// because char_file_u's field is that wide). Every byte of it.
		Title:       strings.Repeat("x", game.MaxTitleLength),
		Description: "A character built to fill every field the format has.\r\n",
		Sex:         2,
		Class:       3,
		Level:       game.LevelImplementor,
		Hometown:    5000,
		Birth:       time.Unix(1000000000, 0).UTC(),
		LastLogon:   lastILP32Second,
		Played:      9999 * time.Hour,
		// char_file_u's host field is char[31], so thirty bytes plus a
		// terminator is the most it can hold. This is exactly thirty.
		Host:       "hosts.at.the.limit.example.org",
		Credential: legacyDESCredential("Torturer", "torture"),
		Weight:     255,
		Height:     255,
		Alignment:  -1000,
		IDNum:      1,

		PlayerFlags: game.SetFromRaw[game.PlayerFlag](0xffffffff),
		AffectFlags: game.SetFromRaw[game.AffectFlag](0xffffffff),
		Preferences: game.SetFromRaw[game.PrefFlag](0xffffffff),

		Conditions:    [3]int32{24, 24, 24},
		WimpLevel:     32000,
		FreezeLevel:   game.LevelImplementor,
		InvisLevel:    game.LevelImplementor,
		LoadRoom:      5000,
		BadPasswords:  255,
		SpellsToLearn: 32000,
		RemortVector:  0x7fffffff,
		SpecFlags:     0x7fffffff,
		OLCZone:       50,
	}

	r.Abilities = game.Abilities{
		Strength: 18, StrengthPercentile: 100, Intelligence: 18,
		Wisdom: 18, Dexterity: 18, Constitution: 18, Charisma: 18,
	}
	r.RealAbilities = r.Abilities
	r.Points = game.Points{
		Mana: 32000, MaxMana: 32000, Hit: 32000, MaxHit: 32000,
		Move: 32000, MaxMove: 32000, Armor: -100,
		Gold: 2000000000, BankGold: 2000000000, Exp: 2000000000,
		HitRoll: 127, DamRoll: 127,
	}
	for i := range r.SavingThrows {
		r.SavingThrows[i] = int32(-100 - i)
		r.RealSavingThrows[i] = r.SavingThrows[i]
	}

	// Every skill slot the format has, so nothing can quietly drop the
	// tail of the array. Slot 0 is not a skill in the C either.
	r.Skills = make(map[int32]int32, 200)
	for n := int32(1); n <= 200; n++ {
		r.Skills[n] = (n % 100) + 1
	}

	// All 32 affect slots occupied. The C reads affects until it hits an
	// empty one, so a record that fills every slot is the one that proves
	// a reader does not stop early — and the one that proves a *writer*
	// has nowhere to put a 33rd.
	for i := 0; i < 32; i++ {
		r.Affects = append(r.Affects, game.Affect{
			Type:     int32(1 + i),
			Duration: int32(100 + i),
			Modifier: int32(i - 16),
			Location: int32(i % 25),
			Bits:     game.SetFromRaw[game.AffectFlag](1 << uint(i%32)),
		})
	}

	// char_file_u's reserved padding is not set here, and cannot be: the
	// slots stopped being a field on game.PlayerRecord when they moved
	// into internal/persist/player/binary, where they belong
	// (docs/design/yaml-only.md §1). They are a property of the stored
	// record rather than of a character — nothing in the game reads or
	// sets one — and binary's Store.Save carries them across from the
	// record it is replacing, which is the behaviour worth having and the
	// one its own tests pin.
	return r
}

// nameAtTheLimit uses every byte of the name field. char_file_u's `name`
// is char[20], so nineteen letters plus the terminator is the most that
// can be stored — and MAX_NAME_LENGTH in the C is 20, which is the same
// off-by-one trap read from the other side.
func nameAtTheLimit() *game.PlayerRecord {
	return plainCharacter(strings.Repeat("A", 1)+strings.Repeat("b", 18), 2, 30)
}

// nameOneShortOfTheLimit is the same case one byte back, because an
// off-by-one in a fixed-width field is only visible with both.
func nameOneShortOfTheLimit() *game.PlayerRecord {
	return plainCharacter(strings.Repeat("C", 1)+strings.Repeat("d", 17), 3, 17)
}

// levelZeroCharacter is a record with nothing in it: level 0, no password,
// no skills, no affects, no title, zero timestamps. Every optional field
// at its zero value is as much a corner as every field at its maximum, and
// it is the one a "minimal document" test (docs/design/yaml-only.md §6)
// is about.
func levelZeroCharacter() *game.PlayerRecord {
	return &game.PlayerRecord{Name: "Nobody", IDNum: 4, Level: 0}
}

func plainCharacter(name string, idnum int64, level int32) *game.PlayerRecord {
	return &game.PlayerRecord{
		Name:          name,
		Title:         "the Character Whose Name Is " + fmt.Sprint(len(name)) + " Letters Long",
		IDNum:         idnum,
		Level:         level,
		Sex:           1,
		Class:         1,
		Hometown:      5000,
		Birth:         time.Unix(1010000000, 0).UTC(),
		LastLogon:     time.Unix(1100000000, 0).UTC(),
		Played:        100 * time.Hour,
		Host:          "example.org",
		Credential:    legacyDESCredential(name, "hunter2"),
		Weight:        150,
		Height:        170,
		Abilities:     game.Abilities{Strength: 12, Intelligence: 13, Wisdom: 14, Dexterity: 15, Constitution: 16, Charisma: 17},
		RealAbilities: game.Abilities{Strength: 12, Intelligence: 13, Wisdom: 14, Dexterity: 15, Constitution: 16, Charisma: 17},
		Points:        game.Points{Hit: 100, MaxHit: 100, Mana: 100, MaxMana: 100, Move: 100, MaxMove: 100, Gold: 1000},
		Conditions:    [3]int32{-1, -1, -1},
		// 200 is the last slot char_file_u's skills[MAX_SKILLS+1]
		// array has, MAX_SKILLS being 200.
		Skills:   map[int32]int32{1: 50, 200: 99},
		LoadRoom: game.NoRoom,
	}
}

// legacyDESCredential is a real crypt(3) DES hash, salted with the
// character's own name exactly as the C does, and then cut to the ten
// characters char_file_u's `pwd` field can actually hold.
//
// The cut is not a convenience. crypt(3) returns thirteen characters and
// MAX_PWD_LENGTH is 10, so what the archived roster contains is the first
// ten of a thirteen-character hash — every password in it is verified
// against a prefix, which is what docs/design/go-port-plan.md §5.3.1
// means by the truncation. This port's own writer refuses a thirteen-byte
// value outright rather than silently cutting it (the field is eleven
// bytes and it says so), so the truncation has to happen here, where it
// is visible, exactly as it happened in the C.
func legacyDESCredential(name, password string) game.Credential {
	hash, err := descrypt.Crypt(password, name)
	if err != nil {
		panic("torture: hashing " + name + ": " + err.Error())
	}
	// MAX_PWD_LENGTH (structs.h), the width the C's own strncpy uses.
	const maxPwdLength = 10
	if len(hash) > maxPwdLength {
		hash = hash[:maxPwdLength]
	}
	return game.Credential{Scheme: game.SchemeLegacyDES, Hash: hash}
}

// writeRoster writes etc/players and the rent, crash and alias files that
// go with it.
func writeRoster(base string) error {
	etc := filepath.Join(base, "etc")
	if err := mkdirAll(etc); err != nil {
		return err
	}

	store, err := binary.New(player.Config{Dir: etc})
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	for _, rec := range torturedCharacters() {
		if err := store.Save(ctx, rec); err != nil {
			return fmt.Errorf("saving %s: %w", rec.Name, err)
		}
	}

	objs, err := binary.NewObjectStore(player.Config{
		Dir: etc, ObjectsDir: filepath.Join(base, "plrobjs"),
	})
	if err != nil {
		return err
	}
	if err := objs.SaveObjects(ctx, "Torturer", rentedWithNestedContainers()); err != nil {
		return err
	}
	if err := objs.SaveObjects(ctx, "Nobody", crashFileLostToRent()); err != nil {
		return err
	}

	aliases, err := binary.NewAliasStore(player.Config{
		Dir: etc, AliasDir: filepath.Join(base, "plralias"),
	})
	if err != nil {
		return err
	}
	if err := aliases.SaveAliases("Torturer", torturedAliases()); err != nil {
		return err
	}

	// A rent file and an alias file for a character who is not on the
	// roster, and a file in a bucket that is not a per-character file at
	// all -- the three things a plrobjs/ or plralias/ holds that no server
	// will ever read.
	//
	// The C never removes either file when a character is deleted:
	// do_delete_char writes the roster and nothing else, so every archive
	// whose operator did not run plrobjs/purgeobjs by hand has these.
	// Disgracelands' own archive did run it, which is why all 79 of its
	// .objs and all 20 of its .alias files have roster entries -- so the
	// case that broke the importer is one no real fixture here had, which
	// is the same "corpus assembled from what the thing obviously does"
	// failure as examples/stock being pure ASCII (data-format.md §11.1).
	//
	// import used to carry these across in silence, because its loop is
	// driven by the roster and asks for each character's files by name, so
	// a file belonging to nobody was never opened and never mentioned
	// (#287). It now names them, and `verify --against` notes them beside
	// the verdict -- both halves, for the reason #239 needed both.
	if err := objs.SaveObjects(ctx, "Ghost", crashFileLostToRent()); err != nil {
		return err
	}
	if err := aliases.SaveAliases("Ghost", torturedAliases()); err != nil {
		return err
	}
	// The 60-byte `00` that sits in each bucket of the archived plrobjs/.
	// Nothing in the C writes it and nothing reads it; it is what a
	// directory this old accumulates, and it is exactly what a report of
	// "files no server reads" has to be able to say without calling it a
	// character.
	return writeFile(filepath.Join(base, "plrobjs", "F-J", "00"), strings.Repeat("\x00", 60))
}

// rentedWithNestedContainers is a rent file holding a container inside a
// container inside a container, plus an object with all four values at the
// int32 extremes.
//
// The nesting is written *through* player.FlattenStoredObjects, because
// that is what the format does: with USE_AUTOEQ 0, struct obj_file_elem
// has no location member, so Crash_save flattens every container before
// writing and everything comes back loose (docs/deviations.md, "Renting
// empties your bags and strips your body"). The tree is built here anyway
// so that the *ordering* the C's contents-then-container recursion
// produces is what lands on disk — which is the part a converter can get
// wrong, and the part a flat file still carries.
func rentedWithNestedContainers() *player.RentFile {
	inner := player.StoredObject{Vnum: 5003, Weight: 1}
	middle := player.StoredObject{Vnum: 5004, Weight: 5, Contains: []player.StoredObject{inner}}
	outer := player.StoredObject{Vnum: 5004, Weight: 5, Contains: []player.StoredObject{middle}}

	extremes := player.StoredObject{
		Vnum:       5000,
		Values:     [game.NumObjValues]int32{2147483647, -2147483648, 2147483647, -2147483648},
		ExtraFlags: game.SetFromRaw[game.ExtraFlag](0xffffffff),
		Weight:     2147483647,
		Timer:      -2147483648,
		PermAffect: game.SetFromRaw[game.AffectFlag](0xffffffff),
		Affects: []game.ObjAffect{
			{Location: 1, Modifier: 127},
			{Location: 18, Modifier: -128},
		},
	}

	return &player.RentFile{
		Code:       player.RentRented,
		Written:    lastILP32Second,
		CostPerDay: 2147483647,
		Gold:       2000000000,
		Bank:       2000000000,
		Objects:    player.FlattenStoredObjects([]player.StoredObject{outer, extremes}),
	}
}

// crashFileLostToRent is the header state a character gets when the rent
// they could not pay took everything: RENT_CRASH, no objects, and the gold
// and cost fields still filled in. The C writes this file and the loader
// has to distinguish it from "no file at all", which is a different thing
// entirely — one is a character who has never left the game carrying
// anything, the other is a character whose belongings were sold.
func crashFileLostToRent() *player.RentFile {
	return &player.RentFile{
		Code:       player.RentCrash,
		Written:    lastILP32Second,
		CostPerDay: 32000,
		Gold:       0,
		Bank:       0,
	}
}

// torturedAliases exercise both alias shapes interpreter.c distinguishes,
// which it derives from the replacement's own text rather than storing:
// ALIAS_COMPLEX for anything containing ';' or '$', ALIAS_SIMPLE
// otherwise (interpreter.c:735-738).
//
// Every replacement begins with a space, and that is the shape the C
// produces rather than a typo. do_alias builds one with any_one_arg,
// which — unlike one_argument — does not skip the whitespace it stops on,
// so the space separating the alias's name from the rest of the line is
// part of the value. write_aliases strips it on the way out and
// read_aliases puts it back (alias.c:22, :55), and $* substitutes the raw
// untrimmed remainder because of it.
//
// This fixture was written without them, and the encoder dropped the
// first character of each instead of saying so — the committed
// plralias/P-T/torturer.alias held " et all corpse", and so did the
// aliases: block in examples/torture/yaml, because both sides of every
// comparison went through the same encoder (#242). A corpus built to
// expose blind spots is not much use recording one.
func torturedAliases() []game.Alias {
	return []game.Alias{
		{Name: "gc", Replacement: " get all corpse"},
		{Name: "bs", Replacement: " backstab $1; flee"},
		{Name: "everything", Replacement: " say $*"},
		{Name: "gc", Replacement: " an earlier, shadowed definition of the same name"},
	}
}
