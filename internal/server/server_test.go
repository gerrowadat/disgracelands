// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gerrowadat/disgracelands/internal/auth"
	"github.com/gerrowadat/disgracelands/internal/engine"
	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/persist/bans"
	bansyaml "github.com/gerrowadat/disgracelands/internal/persist/bans/yaml"
	"github.com/gerrowadat/disgracelands/internal/persist/boards"
	boardsyaml "github.com/gerrowadat/disgracelands/internal/persist/boards/yaml"
	"github.com/gerrowadat/disgracelands/internal/persist/houses"
	housesyaml "github.com/gerrowadat/disgracelands/internal/persist/houses/yaml"
	"github.com/gerrowadat/disgracelands/internal/persist/mail"
	mailyaml "github.com/gerrowadat/disgracelands/internal/persist/mail/yaml"
	"github.com/gerrowadat/disgracelands/internal/persist/player"
	"github.com/gerrowadat/disgracelands/internal/persist/player/ascii"
	"github.com/gerrowadat/disgracelands/internal/persist/player/binary"
	playeryaml "github.com/gerrowadat/disgracelands/internal/persist/player/yaml"
	"github.com/gerrowadat/disgracelands/internal/rng"
	"github.com/gerrowadat/disgracelands/internal/session"
	"github.com/gerrowadat/disgracelands/internal/telnet"
)

// testGreeting stands in for data/text/greetings. The words the licence
// requires are checked in the real file by scripts/license-check.sh; what is
// checked here is that whatever the file says reaches the wire on every
// transport.
const testGreeting = "Welcome to the test MUD\r\n" +
	"Based on CircleMUD, created by Jeremy Elson.\r\n" +
	"Originally based on DikuMUD, created by Hans Henrik Staerfeldt,\r\n" +
	"Katja Nyboe, Tom Madsen, Michael Seifert, and Sebastian Hammer.\r\n" +
	"\r\nBy what name do you wish to be known? "

// testCredits stands in for data/text/credits. The marker line makes it
// distinguishable from the greeting, which necessarily says much the same
// thing — without it, a test waiting for "Jeremy Elson" matches the greeting
// it read minutes earlier and passes without the command having run.
const testCredits = "CREDITS-FILE\r\nCircleMUD, created by Jeremy Elson.\r\n"

func testText(t *testing.T) *Text {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "text"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		greetingFile:   testGreeting,
		creditsFile:    testCredits,
		motdFile:       "Mortal news.\r\n",
		imotdFile:      "Immortal news.\r\n",
		backgroundFile: "BACKGROUND-FILE\r\nIt is a time of conflict.\r\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// The real socials table, because the socials are a third of the
	// command table and the tests that care about abbreviations need the
	// whole of it — and the real help data, because the tests that need
	// `help circlemud` to actually work (the licence requirement
	// go-port-plan.md §12 describes) need the real table rather than a
	// synthetic stand-in.
	//
	// Both come from examples/stock/**yaml**, not from binary/. That is
	// the change docs/design/yaml-only.md §5.4 asks for: this harness
	// is what every integration test in this package runs on, and it was
	// running on the formats the server is about to stop supporting. It
	// now proves the format the server ships on.
	//
	// The tests that load classic *deliberately*, to compare the two
	// (helpformat_test.go, socialsformat_test.go, damagemessages_test.go),
	// are untouched: they are differential tests, and this release makes
	// them more important rather than less.
	stockYaml := filepath.Join(repoRoot(t), "examples", "stock", "yaml")
	// socials.yaml specifically, not the whole of config/. That directory
	// also holds messages.yaml, and copying it registers the real
	// fight-message table for every test in this package — which is not
	// the default the harness had before and which silently defeats the
	// three tests whose subject is what happens with *nothing* registered
	// (damagemessages_test.go). Found by exactly those three failing.
	copyFile(t, filepath.Join(stockYaml, socialsConfigDir, "socials.yaml"),
		filepath.Join(dir, socialsConfigDir, "socials.yaml"))
	copyInto(t, filepath.Join(stockYaml, helpDir), filepath.Join(dir, helpDir))

	text, err := LoadText(dir)
	if err != nil {
		t.Fatal(err)
	}
	return text
}

// copyFile copies one file, creating the destination's directory.
func copyFile(t *testing.T, from, to string) {
	t.Helper()
	b, err := os.ReadFile(from) //nolint:gosec // a fixture in this repository
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil { //nolint:gosec // a test's own temp directory
		t.Fatal(err)
	}
	if err := os.WriteFile(to, b, 0o600); err != nil {
		t.Fatal(err)
	}
}

// copyInto copies every regular file in one directory into another,
// creating it. Not recursive: both directories it is used for
// (config/, text/help/) are flat.
func copyInto(t *testing.T, from, to string) {
	t.Helper()
	entries, err := os.ReadDir(from)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(to, 0o755); err != nil { //nolint:gosec // a test's own temp directory
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		b, err := os.ReadFile(filepath.Join(from, e.Name())) //nolint:gosec // a fixture in this repository
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(to, e.Name()), b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// repoRoot walks up to the directory holding go.mod.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no go.mod above the working directory")
		}
		dir = parent
	}
}

// Object prototypes the tests instantiate.
const (
	testSwordVnum    game.ObjVnum = 100
	testRingVnum     game.ObjVnum = 101
	testFountainVnum game.ObjVnum = 102
	testKeyVnum      game.ObjVnum = 103
	testBagVnum      game.ObjVnum = 104
	testChestVnum    game.ObjVnum = 105
	testPlateVnum    game.ObjVnum = 106
	testTorchVnum    game.ObjVnum = 107
	testScrollVnum   game.ObjVnum = 108
	testWandVnum     game.ObjVnum = 109
	testPotionVnum   game.ObjVnum = 110
	testStaffVnum    game.ObjVnum = 111
	testBoatVnum     game.ObjVnum = 112
	testBootsVnum    game.ObjVnum = 113
	testJugVnum      game.ObjVnum = 114
)

// testFillerVnumBase starts a run of otherwise-uninteresting, mutually
// distinct object prototypes (see the loop in testWorld below). A shop's
// `list` groups identical items into one line — shop.c:802's
// same_obj — so forcing it to paginate needs enough *distinct* items on
// the shelf, not many of a few kinds; nothing else references these.
const testFillerVnumBase game.ObjVnum = 200

// testFillerVnumCount is how many of them exist.
const testFillerVnumCount = 20

// testFillerRoomVnumBase and testFillerZoneVnumBase are the same idea for
// rooms and zones — see the loops in testWorld that use them.
const (
	testFillerRoomVnumBase  game.RoomVnum = 3030
	testFillerRoomVnumCount               = 25
	testFillerZoneVnumBase  game.ZoneVnum = 9000
	testFillerZoneVnumCount               = 25
)

// Mobile prototypes the tests instantiate.
const (
	testDogVnum         game.MobVnum = 999
	testGuildmasterVnum game.MobVnum = 998
	testShopkeeperVnum  game.MobVnum = 997
	// testZombieVnum is not this suite's choice: mag_summons hard-codes it
	// (magic.c:790), so `animate dead` needs a prototype at exactly this
	// number or it has nothing to raise.
	testZombieVnum game.MobVnum = 11
)

// testShopVnum and the shop's room. The shop buys and sells weapons and
// wands, produces the sword, and is open all day.
const (
	testShopVnum game.ShopVnum = 9001
	ShopRoom     game.RoomVnum = 3018
	// BoardRoom holds a bulletin board.
	BoardRoom game.RoomVnum = 3019
	// HouseRoom and AtriumRoom are a house and the room its door opens into.
	HouseRoom  game.RoomVnum = 3020
	AtriumRoom game.RoomVnum = 3021
	// CellarRoom is flagged DARK, for the light and visibility tests. It has
	// no exits: tests put a character in it directly, so that adding it
	// changes no other room's `[ Exits: ]` line.
	CellarRoom game.RoomVnum = 3022
	// PetShopRoom answers `list`/`buy` itself (pet_shops is a room special,
	// not a mobile's). PetShopBackRoom must be exactly one vnum higher —
	// specPetShop finds it the same blunt way the C does, by arithmetic on
	// the room the player is standing in, not a lookup.
	PetShopRoom     game.RoomVnum = 3023
	PetShopBackRoom game.RoomVnum = 3024
	// MayorLoopRoom's four horizontal exits all lead back to itself.
	// specMayor's scripted path doesn't care where each step actually
	// lands, only that a direction gets tried, so a self-loop exercises
	// that without reproducing the real archived room graph the path was
	// actually written against.
	MayorLoopRoom game.RoomVnum = 3025
)

// MageGuildRoom is guild_info's first row: the magic-user guild, whose door
// is south. The test world carries it so the guild guard has somewhere real
// to stand.
const MageGuildRoom game.RoomVnum = 3017

// testWorld is the two start rooms, joined so a character can walk between
// them, plus the mage guild, a few objects to pick up and two mobiles.
//
// Built in Go rather than read off disk, and deliberately so: every test in
// this package can then put exactly what it needs in front of a character
// without a world file to keep in step, and the whole package runs in
// seconds on every push. What that buys is precision; what it costs is that
// nothing here ever exercises the boot sequence — no world file is parsed,
// no zone reset runs, no special procedure is attached by vnum, no shop
// keeper is resolved, no text/ is read, no flag is parsed and no signal is
// handled. A world that no longer loads passes every test in this file.
//
// test/play is the other half of that trade: a real dlmud process on
// examples/mini, driven over a socket, release-only (`make play`). Neither
// replaces the other. When a change is about a *rule*, it belongs here;
// when it is about the server or its data actually working end to end, it
// belongs there.
func testWorld() *game.Live {
	temple := &game.RoomDef{Vnum: MortalStartRoom, Name: "The Temple Of Midgaard", Description: "A temple.\r\n"}
	board := &game.RoomDef{Vnum: ImmortStartRoom, Name: "The Immortal Board Room", Description: "A board room.\r\n"}
	guild := &game.RoomDef{Vnum: MageGuildRoom, Name: "The Mage Guild", Description: "A guild.\r\n"}
	// The donation room, so that `donate` has somewhere to send things.
	donation := &game.RoomDef{Vnum: 3063, Name: "The Donation Room", Description: "A donation room.\r\n"}
	temple.Exits[game.North] = &game.ExitDef{ToRoom: ImmortStartRoom}
	board.Exits[game.South] = &game.ExitDef{ToRoom: MortalStartRoom}
	guild.Exits[game.South] = &game.ExitDef{ToRoom: MortalStartRoom}

	objects := []*game.ObjDef{
		{
			Vnum: testSwordVnum, Keywords: "sword long", ShortDesc: "a long sword",
			Description: "A long sword is lying here.",
			Type:        game.ItemWeapon,
			WearFlags:   game.NewSet(game.ItemWearTake, game.ItemWearWield),
			Weight:      10,
			// A price, so the shop tests have something to charge for. 100 at
			// the shop's 1.15 markup is 114, which is the number
			// docs/weirdnumbers.md is about.
			Cost:   100,
			Values: [game.NumObjValues]int32{0, 2, 6, 3},
		},
		{
			Vnum: testRingVnum, Keywords: "ring gold", ShortDesc: "a gold ring",
			Description: "A gold ring is lying here.",
			Type:        game.ItemArmor,
			WearFlags:   game.NewSet(game.ItemWearTake, game.ItemWearFinger),
			Weight:      1,
			Cost:        50,
		},
		{
			Vnum: testKeyVnum, Keywords: "key small", ShortDesc: "a small key",
			Description: "A small key is lying here.",
			Type:        game.ItemKey,
			WearFlags:   game.NewSet(game.ItemWearTake),
			Weight:      1,
		},
		{
			// An open bag: capacity 100, no lock, and light enough that the
			// capacity is what runs out rather than the carrying weight.
			Vnum: testBagVnum, Keywords: "bag", ShortDesc: "a bag",
			Description: "A bag is lying here.",
			Type:        game.ItemContainer,
			WearFlags:   game.NewSet(game.ItemWearTake),
			Weight:      2,
			Values:      [game.NumObjValues]int32{100, 0, 0, 0},
		},
		{
			// A closeable chest, closed and locked, opened by the small key.
			Vnum: testChestVnum, Keywords: "chest", ShortDesc: "a wooden chest",
			Description: "A wooden chest sits here.",
			Type:        game.ItemContainer,
			Weight:      50,
			Values: [game.NumObjValues]int32{
				200,
				int32(game.NewSet(game.ContCloseable, game.ContClosed, game.ContLocked).Raw()),
				int32(testKeyVnum),
				0,
			},
		},
		{
			// Armour with an apply on it, so that wearing it changes two
			// different things by two different mechanisms.
			Vnum: testPlateVnum, Keywords: "plate mail", ShortDesc: "a suit of plate mail",
			Description: "A suit of plate mail is lying here.",
			Type:        game.ItemArmor,
			WearFlags:   game.NewSet(game.ItemWearTake, game.ItemWearBody),
			Weight:      100,
			Values:      [game.NumObjValues]int32{5},
			Affects:     []game.ObjAffect{{Location: game.ApplyHitRoll, Modifier: 2}},
		},
		{
			Vnum: testTorchVnum, Keywords: "torch", ShortDesc: "a torch",
			Description: "A torch is lying here.",
			Type:        game.ItemLight,
			WearFlags:   game.NewSet(game.ItemWearTake),
			Weight:      2,
			// Value 2 is how many hours of light are left.
			Values: [game.NumObjValues]int32{0, 0, 24},
		},
		{
			// A scroll of armor and bless: values 1..3 are the spells and
			// value 0 the level it casts at.
			Vnum: testScrollVnum, Keywords: "scroll", ShortDesc: "a scroll of protection",
			Description: "A scroll is lying here.",
			Type:        game.ItemScroll,
			WearFlags:   game.NewSet(game.ItemWearTake, game.ItemWearHold),
			Weight:      1,
			Values:      [game.NumObjValues]int32{20, game.SpellArmor.Number(), game.SpellBless.Number(), 0},
		},
		{
			Vnum: testWandVnum, Keywords: "wand", ShortDesc: "a wand of missiles",
			Description: "A wand is lying here.",
			Type:        game.ItemWand,
			WearFlags:   game.NewSet(game.ItemWearTake, game.ItemWearHold),
			Weight:      2,
			Cost:        200,
			// Level 20, three charges, three left, magic missile.
			Values: [game.NumObjValues]int32{20, 3, 3, game.SpellMagicMissile.Number()},
		},
		{
			Vnum: testPotionVnum, Keywords: "potion", ShortDesc: "a potion of healing",
			Description: "A potion is lying here.",
			Type:        game.ItemPotion,
			WearFlags:   game.NewSet(game.ItemWearTake, game.ItemWearHold),
			Weight:      1,
			Values:      [game.NumObjValues]int32{20, game.SpellCureLight.Number(), 0, 0},
		},
		{
			Vnum: testStaffVnum, Keywords: "staff", ShortDesc: "a staff of sleep",
			Description: "A staff is lying here.",
			Type:        game.ItemStaff,
			WearFlags:   game.NewSet(game.ItemWearTake, game.ItemWearHold),
			Weight:      5,
			Values:      [game.NumObjValues]int32{20, 5, 5, game.SpellMagicMissile.Number()},
		},
		{
			// A boat you cannot wear anywhere, which is the only kind
			// has_boat counts in an inventory: its test is
			// `find_eq_pos(ch, obj, NULL) < 0` (act.movement.c:70).
			Vnum: testBoatVnum, Keywords: "boat canoe", ShortDesc: "a small canoe",
			Description: "A small canoe is beached here.",
			Type:        game.ItemBoat,
			WearFlags:   game.NewSet(game.ItemWearTake),
			Weight:      50,
		},
		{
			// A boat that *is* wearable, so the same test can be shown from
			// the other side: carried, it does nothing; worn, it floats you.
			Vnum: testBootsVnum, Keywords: "waders boots", ShortDesc: "a pair of waders",
			Description: "A pair of waders is lying here.",
			Type:        game.ItemBoat,
			WearFlags:   game.NewSet(game.ItemWearTake, game.ItemWearFeet),
			Weight:      5,
		},
		{
			// An empty drink container, carried: create water needs
			// something in an inventory to fill, and a fountain is neither
			// takeable nor an ItemDrinkCon.
			Vnum: testJugVnum, Keywords: "jug", ShortDesc: "a clay jug",
			Description: "A clay jug is lying here.",
			Type:        game.ItemDrinkCon,
			WearFlags:   game.NewSet(game.ItemWearTake),
			Weight:      3,
			// Capacity 20, empty, and water is liquid 0.
			Values: [game.NumObjValues]int32{20, 0, game.LiquidWater, 0},
		},
		{
			// mag_creations' own object: `create food` makes this vnum and
			// nothing else (game.CreateFoodVnum, magic.c's spell_create_food),
			// and without a prototype for it the spell answers "I seem to
			// have goofed."
			Vnum: game.CreateFoodVnum, Keywords: "waybread food", ShortDesc: "a Waybread",
			Description: "A Waybread is lying here.",
			Type:        game.ItemFood,
			WearFlags:   game.NewSet(game.ItemWearTake),
			Weight:      1,
			// Value 0 is how many hours of hunger it fills.
			Values: [game.NumObjValues]int32{5},
		},
		{
			Vnum: testFountainVnum, Keywords: "fountain", ShortDesc: "a fountain",
			Description: "A fountain bubbles here.",
			Type:        game.ItemFountain,
			Weight:      500,
		},
	}

	mobiles := []*game.MobDef{
		{
			Vnum: testDogVnum, Keywords: "dog", ShortDesc: "a large dog",
			LongDesc:        "A large dog is here.\r\n",
			Level:           5,
			HitDice:         game.Dice{Number: 1, Size: 1, Bonus: 100},
			Position:        int32(game.PosStanding),
			DefaultPosition: int32(game.PosStanding),
		},
		{
			Vnum: testGuildmasterVnum, Keywords: "guildmaster",
			ShortDesc: "the guildmaster", LongDesc: "The guildmaster stands here.\r\n",
			Level:           30,
			HitDice:         game.Dice{Number: 1, Size: 1, Bonus: 500},
			Position:        int32(game.PosStanding),
			DefaultPosition: int32(game.PosStanding),
		},
	}

	mobiles = append(mobiles, &game.MobDef{
		// mag_summons' zombie (magic.c:790). `animate dead` makes this vnum
		// and nothing else, so without a prototype the spell can only ever
		// say it does not remember how.
		Vnum: testZombieVnum, Keywords: "zombie", ShortDesc: "a zombie",
		LongDesc:        "A zombie shambles here.\r\n",
		Level:           5,
		HitDice:         game.Dice{Number: 1, Size: 1, Bonus: 100},
		Position:        int32(game.PosStanding),
		DefaultPosition: int32(game.PosStanding),
	})

	mobiles = append(mobiles, &game.MobDef{
		Vnum: testShopkeeperVnum, Keywords: "shopkeeper keeper",
		ShortDesc: "the shopkeeper", LongDesc: "The shopkeeper stands here.\r\n",
		Level:           30,
		HitDice:         game.Dice{Number: 1, Size: 1, Bonus: 500},
		Position:        int32(game.PosStanding),
		DefaultPosition: int32(game.PosStanding),
		Gold:            10_000,
	})

	// The mortal bulletin board, so gen_board has an object to be attached
	// to. Vnum 3099 is board_info[0]'s.
	objects = append(objects, &game.ObjDef{
		Vnum: game.Boards[0].Vnum, Keywords: "board bulletin",
		ShortDesc:   "a bulletin board",
		Description: "A bulletin board is fastened to the wall here.",
		Type:        game.ItemOther,
		Spec:        "gen_board",
	})

	// A run of distinct, otherwise-uninteresting objects — see
	// testFillerVnumBase — for tests that need a shop shelf or a listing
	// long enough to paginate.
	for i := 0; i < testFillerVnumCount; i++ {
		vnum := testFillerVnumBase + game.ObjVnum(i)
		objects = append(objects, &game.ObjDef{
			Vnum:        vnum,
			Keywords:    fmt.Sprintf("filler filler%d", i),
			ShortDesc:   fmt.Sprintf("a filler item %d", i),
			Description: fmt.Sprintf("A filler item %d is lying here.", i),
			Type:        game.ItemTrash,
			WearFlags:   game.NewSet(game.ItemWearTake),
			Weight:      1,
			Cost:        10,
		})
	}

	// One shop. It produces the sword, so the supply is endless and `list`
	// shows "Unlimited"; it buys weapons and wands, which is enough to
	// exercise trade_with's three refusals.
	shops := []*game.ShopDef{{
		Vnum:      testShopVnum,
		Keeper:    testShopkeeperVnum,
		Producing: []game.ObjVnum{testSwordVnum},
		// 1.15 and 0.15 are the multipliers the real Midgaard magic shop
		// uses, and the ones whose truncation is checked against the C.
		ProfitBuy:  1.15,
		ProfitSell: 0.15,
		BuyTypes: []game.ShopBuyType{
			{Type: game.ItemWeapon},
			{Type: game.ItemWand},
		},
		Messages: [game.NumShopMessages]string{
			game.MsgNoSuchItem1:  "%s Sorry, I haven't got exactly that item.",
			game.MsgNoSuchItem2:  "%s You don't seem to have that.",
			game.MsgDoNotBuy:     "%s I don't buy such items.",
			game.MsgMissingCash1: "%s That is too expensive for me!",
			game.MsgMissingCash2: "%s You can't afford it!",
			game.MsgBuy:          "%s That'll be %d coins, please.",
			game.MsgSell:         "%s You'll get %d coins for it!",
		},
		Rooms:  []game.RoomVnum{ShopRoom},
		Open1:  0,
		Close1: 28, // open all day: the MUD day is 24 hours
	}}

	// Two zones, so that anything which cares about zone boundaries — a
	// shout, a reset — has one to cross. The numbers are Midgaard's own.
	zones := []*game.ZoneDef{
		{Vnum: 12, Name: "The Immortal Zone", Bottom: 1200, Top: 1299, ResetMode: 0},
		{Vnum: 30, Name: "Midgaard", Bottom: 3000, Top: 3099, ResetMode: 0},
	}

	shopRoom := &game.RoomDef{
		Vnum: ShopRoom, Name: "A Small Shop", Description: "A shop.\r\n",
	}

	boardRoom := &game.RoomDef{
		Vnum: BoardRoom, Name: "The Notice Board", Description: "A room with a board.\r\n",
	}

	// A house and its atrium, joined both ways: hcontrol insists the door be
	// two-way, and that is the only structural rule housing has.
	houseRoom := &game.RoomDef{
		Vnum: HouseRoom, Name: "A Small House", Description: "A house.\r\n",
	}
	atriumRoom := &game.RoomDef{
		Vnum: AtriumRoom, Name: "An Atrium", Description: "An atrium.\r\n",
	}
	cellarRoom := &game.RoomDef{
		Vnum: CellarRoom, Name: "A Pitch Dark Cellar", Description: "A cellar.\r\n",
		Flags: game.NewSet(game.RoomDark),
	}
	houseRoom.Exits[game.North] = &game.ExitDef{ToRoom: AtriumRoom}
	atriumRoom.Exits[game.South] = &game.ExitDef{ToRoom: HouseRoom}

	petShopRoom := &game.RoomDef{
		Vnum: PetShopRoom, Name: "The Pet Shop", Description: "A pet shop.\r\n",
		Spec: "pet_shops",
	}
	petShopBackRoom := &game.RoomDef{
		Vnum: PetShopBackRoom, Name: "Pet Shop Store", Description: "Where the pets are kept.\r\n",
	}

	mayorLoopRoom := &game.RoomDef{
		Vnum: MayorLoopRoom, Name: "A Featureless Plaza", Description: "A plaza.\r\n",
	}
	for _, dir := range []game.Direction{game.North, game.East, game.South, game.West} {
		mayorLoopRoom.Exits[dir] = &game.ExitDef{ToRoom: MayorLoopRoom}
	}

	rooms := []*game.RoomDef{
		temple, board, guild, donation, shopRoom, boardRoom, houseRoom,
		atriumRoom, cellarRoom, petShopRoom, petShopBackRoom, mayorLoopRoom,
	}
	// A run of otherwise-inert rooms — unflagged, no exits — for a test
	// that needs `show death`/`show godrooms` (act.wizard.c's shared
	// showRooms case, all through one page_string call) to actually have
	// enough matches to paginate. Nothing flags these by default, so no
	// existing "show death"/"show godrooms" expectation sees them.
	for i := 0; i < testFillerRoomVnumCount; i++ {
		vnum := testFillerRoomVnumBase + game.RoomVnum(i)
		rooms = append(rooms, &game.RoomDef{
			Vnum: vnum, Name: fmt.Sprintf("Filler Room %d", i),
			Description: fmt.Sprintf("A filler room %d.\r\n", i),
		})
	}

	// A run of otherwise-inert zones, ResetMode 0 so BootReset's absence
	// from the test world costs nothing, for a test that needs `show
	// zones` (act.wizard.c, all three branches through one page_string
	// call) to have enough rows to paginate.
	for i := 0; i < testFillerZoneVnumCount; i++ {
		vnum := testFillerZoneVnumBase + game.ZoneVnum(i)
		zones = append(zones, &game.ZoneDef{
			Vnum: vnum, Name: fmt.Sprintf("Filler Zone %d", i),
			Bottom: game.RoomVnum(vnum) * 100, Top: game.RoomVnum(vnum)*100 + 99,
			ResetMode: 0,
		})
	}

	live := game.NewLive(&game.World{
		Rooms:   rooms,
		Objects: objects,
		Mobiles: mobiles,
		Zones:   zones,
		Shops:   shops,
	})
	// assign_the_shopkeepers, which the real boot runs after AssignSpecials.
	// Done here because the test world skips BootReset.
	live.AssignShopkeepers()
	return live
}

// testAuth is the credential policy every test server runs under: legacy DES
// still accepted, and a work factor small enough to be worth paying several
// hundred times.
//
// The real factor is 64 MiB over three passes, about 140ms a hash on a
// laptop. This package creates or logs in a character in nearly every one of
// its tests, and that alone was more than half the suite's runtime — a
// profile of `go test ./internal/server` put 94% of its CPU samples inside
// argon2. Nothing here is testing the work factor; internal/auth is, both
// that DefaultCost is still the RFC 9106 recommendation and that hashes made
// under it verify.
//
// The scheme is unchanged: these are real argon2id hashes, made and verified
// by the same code the server uses, just cheap ones.
var testAuth = auth.Verifier{
	AllowLegacy: true,
	Cost:        auth.Cost{Time: 1, Memory: 8 * 1024, Threads: 4},
}

// testRoundLength is how long a combat round lasts in the tests.
//
// A wait state is real elapsed time (game.Character.Wait stores a deadline,
// and the dispatcher sleeps until it passes), so at the real two seconds a
// single `kick` costs its test six of them. A twentieth of that keeps the
// tests that assert on lag meaningful — the shortest wait anything imposes is
// one round, and 100ms is far enough clear of scheduler noise to assert
// against — while taking a dozen combat tests from two seconds each to a
// tenth.
//
// The real length is session.DefaultRoundLength, which is asserted to be
// PULSE_VIOLENCE in internal/session.
const testRoundLength = 100 * time.Millisecond

// newTestServer builds a server on a temporary player directory and starts
// its engine, on yaml — the format the server ships on.
//
// It used to build one on ascii/binary, "the server's real defaults", and
// every integration test in this package therefore proved the behaviour of
// formats the server is about to stop supporting (docs/proposals/
// yaml-only.md §5.4). The coupling is concentrated here, so the switch is
// contained and the payoff is total: every test below now runs on the
// shipping configuration.
//
// One yaml store serves as both the roster and the rent files, because
// that is what the format is — one file per character holding both (§8,
// "one player, one file"). The split-store shape newTestServerWith still
// takes is what ascii and binary need, and newLegacyTestServer is what
// still uses it.
func newTestServer(t *testing.T) (*Server, player.Store) {
	t.Helper()

	store, err := playeryaml.New(player.Config{Dir: filepath.Join(t.TempDir(), "players")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	return newTestServerWith(t, store, store, nil, nil, nil, nil)
}

// newLegacyTestServer builds one on ascii/binary instead, for the handful
// of tests whose subject *is* a legacy format's behaviour.
//
// There is exactly one such test today — TestRentingEmptiesYourBags, which
// asserts that a container's contents come back loose because
// struct obj_file_elem has no location member — and it would pass
// vacuously if it were quietly moved onto a format that does not have that
// limitation. Keeping the legacy harness available, named for what it is,
// is what stops that happening by accident.
func newLegacyTestServer(t *testing.T) (*Server, player.Store) {
	t.Helper()

	store, err := ascii.New(player.Config{Dir: filepath.Join(t.TempDir(), "pfiles")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	// The rent files live beside the roster, as they do in a real data
	// directory.
	objects, err := binary.NewObjectStore(player.Config{Dir: filepath.Join(t.TempDir(), "plrobjs-lib")})
	if err != nil {
		t.Fatal(err)
	}

	return newTestServerWith(t, store, objects, nil, nil, nil, nil)
}

// newTestServerWith is newTestServer's common tail, factored out so a test
// that needs a different player.Store/player.ObjectStore pair — yaml's
// own alias_test.go-style containment test, chiefly — does not have to
// duplicate the engine/world/board/mail/house/text wiring to get one.
//
// banStore, boardStore, mailStore and houseStore override the default yaml
// ones when non-nil — see docs/design/data-format.md §9 step 6a. Those
// defaults were classic until docs/design/yaml-only.md §5.4; they are
// yaml now, for the reason newTestServer's own comment gives.
func newTestServerWith(t *testing.T, store player.Store, objects player.ObjectStore, banStore bans.Store, boardStore boards.Store, mailStore mail.Store, houseStore houses.Store) (*Server, player.Store) {
	t.Helper()

	// Board files, in their own throwaway directory.
	if boardStore == nil {
		var err error
		boardStore, err = boardsyaml.New(boards.Config{Dir: filepath.Join(t.TempDir(), "state")})
		if err != nil {
			t.Fatal(err)
		}
	}

	if mailStore == nil {
		var err error
		mailStore, err = mailyaml.New(mail.Config{Path: filepath.Join(t.TempDir(), "state")})
		if err != nil {
			t.Fatal(err)
		}
	}

	var err error
	if banStore == nil {
		banStore, err = bansyaml.New(bans.Config{Path: filepath.Join(t.TempDir(), "state")})
		if err != nil {
			t.Fatal(err)
		}
	}

	if houseStore == nil {
		// One directory: yaml keeps a house's control record and its
		// contents in the same file, which is the point of state/houses.yaml
		// (docs/design/data-format.md §9).
		houseStore, err = housesyaml.New(houses.Config{ObjectDir: t.TempDir()})
		if err != nil {
			t.Fatal(err)
		}
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	eng := engine.New(engine.Options{World: testWorld(), Logger: logger})

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go eng.Run(ctx)

	text := testText(t)
	srv := New(Options{
		Engine:  eng,
		Players: store,
		Objects: objects,
		Boards:  boardStore,
		Mail:    mailStore,
		Houses:  houseStore,
		Bans:    banStore,
		Auth:    testAuth,
		Text:    text,
		Logger:  logger,
		RNG:     testRNG(),
		// See testRoundLength: wait states are wall-clock, and at the real
		// two seconds a round the combat tests spend most of their time
		// asleep.
		RoundLength: testRoundLength,
	})

	// Every background write must finish before the test's t.TempDir() is
	// removed. Without this the cleanup races the saves and fails with
	// "directory not empty" — in whichever *other* test the scheduler
	// happens to be running by then, which is why it looked like flakiness
	// somewhere else entirely.
	t.Cleanup(srv.WaitForWrites)

	// init_boards, which the real boot does inside BootReset. The test world
	// skips that, so the boards are loaded here.
	if err := eng.DoSync(ctx, srv.loadBoards); err != nil {
		t.Fatal(err)
	}
	if err := eng.DoSync(ctx, srv.loadHouses); err != nil {
		t.Fatal(err)
	}

	// BootReset is not called here — the test world's zones have no reset
	// commands and every test that wants something in the world puts it there
	// itself. The socials are the one part of boot that the command table
	// needs, so they are registered on their own.
	session.RegisterSocials(text.Socials())

	return srv, store
}

// listening starts a telnet listener and its accept loop, returning the
// address to dial.
func listening(t *testing.T, srv *Server) string {
	t.Helper()
	ln, err := ListenTelnet("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	serveOn(t, srv, ln)
	return ln.Addr().String()
}

// serveOn runs an accept loop until the test ends.
func serveOn(t *testing.T, srv *Server, ln *Listener) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = srv.Accept(ctx, ln, Limits{MaxPerHost: 8, LoginGrace: time.Minute})
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
}

// TestEveryTransportSendsTheGreeting is a licence test, not a feature test.
//
// The CircleMUD licence requires the login sequence to name the DikuMUD and
// CircleMUD creators (docs/design/go-port-plan.md §12), and the greeting
// file is where that happens. A transport that renders its own splash screen
// and skips it would be a violation. Every transport goes through
// Server.Accept, so a new one is covered by adding a case here — and if
// someone adds a listener that bypasses Accept, this is the test that should
// have stopped them.
func TestEveryTransportSendsTheGreeting(t *testing.T) {
	srv, _ := newTestServer(t)

	for _, tc := range []struct {
		name string
		dial func(t *testing.T) *client
	}{
		{
			name: "telnet",
			dial: func(t *testing.T) *client {
				return dialClient(t, listening(t, srv))
			},
		},
		{
			name: "telnets",
			dial: func(t *testing.T) *client {
				ln, err := ListenTLS("127.0.0.1:0", &tls.Config{
					Certificates: []tls.Certificate{selfSignedCert(t)},
					MinVersion:   tls.VersionTLS12,
				})
				if err != nil {
					t.Fatal(err)
				}
				serveOn(t, srv, ln)

				conn, err := tls.Dial("tcp", ln.Addr().String(), &tls.Config{
					InsecureSkipVerify: true, //nolint:gosec // a self-signed certificate made for this test
					MinVersion:         tls.VersionTLS12,
				})
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = conn.Close() })
				return wrapClient(t, conn)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := tc.dial(t)
			got := c.expect("By what name")

			for _, required := range []string{
				"CircleMUD, created by Jeremy Elson.",
				"DikuMUD, created by Hans Henrik Staerfeldt,",
			} {
				if !strings.Contains(got, required) {
					t.Errorf("the greeting on %s does not name the creators (%q):\n%s",
						tc.name, required, got)
				}
			}
			// And it is the file, in full, not a paraphrase of it.
			if !strings.Contains(got, testGreeting) {
				t.Errorf("the greeting on %s is not the file verbatim:\n%q", tc.name, got)
			}
		})
	}
}

// TestTheGreetingIsSentBeforeAnythingIsRead: a client that sends nothing must
// still see it, because a player who connects and waits is the normal case.
func TestTheGreetingIsSentBeforeAnythingIsRead(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))

	if got := c.expect("By what name"); !strings.Contains(got, testGreeting) {
		t.Errorf("greeting not sent to a silent client:\n%q", got)
	}
}

// TestTheFirstCharacterIsAnImplementor covers init_char's first-player branch
// end to end: the level, the points it hands out directly, and the immortal
// message of the day.
func TestTheFirstCharacterIsAnImplementor(t *testing.T) {
	srv, store := newTestServer(t)
	c := dialClient(t, listening(t, srv))

	c.create("Zod", "swordfish", "m", "w")
	got := c.transcript()

	if !strings.Contains(got, "The Immortal Board Room") {
		t.Errorf("an Implementor did not start in the immortal room:\n%s", got)
	}
	// init_char's numbers, not do_start's: 500 hit points, 100 mana, 82 moves.
	// do_start must not have run at all for this character.
	if !strings.Contains(got, "500H 100M 82V > ") {
		t.Errorf("the prompt is not an Implementor's:\n%s", got)
	}
	// The **mortal** message of the day, even though this character is a
	// level 34 implementor by the time it is sent.
	//
	// The C has two paths and only one checks the level: an existing
	// character logging in gets `imotd` if immortal (interpreter.c:1503), and
	// one who has just been created gets `motd` whatever their level (:1603),
	// one line after init_char set it to 34. So the founding implementor sees
	// the mortal file the day they are made and the immortal one every time
	// after.
	//
	// This test asserted the opposite until the session-parity harness played
	// the same script against both servers and they disagreed here.
	if !strings.Contains(got, "Mortal news.") {
		t.Errorf("a newly created character was not shown the mortal MOTD:\n%s", got)
	}
	if strings.Contains(got, "Immortal news.") {
		t.Errorf("a newly created implementor was shown the immortal MOTD; the C sends motd unconditionally at interpreter.c:1603:\n%s", got)
	}

	rec, err := store.Load(context.Background(), "Zod")
	if err != nil {
		t.Fatalf("the character was not saved: %v", err)
	}
	if rec.Level != game.LevelImplementor {
		t.Errorf("saved at level %d, want %d", rec.Level, game.LevelImplementor)
	}
	if len(rec.Skills) != game.MaxSkills {
		t.Errorf("knows %d skills, want all %d", len(rec.Skills), game.MaxSkills)
	}
}

// TestCreateAMortalAndLogBackIn is the phase's acceptance criterion in
// miniature: the full creation sequence, then out and back in again.
func TestCreateAMortalAndLogBackIn(t *testing.T) {
	srv, store := newTestServer(t)
	addr := listening(t, srv)

	// The first character on the roster is the Implementor, so make one to
	// get out of the way before testing an ordinary mortal.
	first := dialClient(t, addr)
	first.create("Zod", "swordfish", "m", "w")
	first.send("quit")
	first.expect("Goodbye")
	first.close()

	c := dialClient(t, addr)
	c.create("Welmar", "hunter2!", "f", "t")
	got := c.transcript()

	if !strings.Contains(got, "The Temple Of Midgaard") {
		t.Errorf("a mortal did not start in the temple:\n%s", got)
	}
	if !strings.Contains(got, "Mortal news.") {
		t.Errorf("a mortal was not shown the mortal message of the day:\n%s", got)
	}
	if !strings.Contains(got, "This is your new CircleMUD character!") {
		t.Errorf("a new character was not shown START_MESSG:\n%s", got)
	}
	if !strings.Contains(got, "Welcome to the land of CircleMUD!") {
		t.Errorf("a character entering the world was not shown WELC_MESSG:\n%s", got)
	}

	rec, err := store.Load(context.Background(), "Welmar")
	if err != nil {
		t.Fatal(err)
	}
	if rec.Level != 1 {
		t.Errorf("a new mortal is at level %d, want 1 after do_start", rec.Level)
	}
	if rec.Points.MaxMana != 100 {
		t.Errorf("mana is %d, want the 100 init_char gives and do_start leaves alone", rec.Points.MaxMana)
	}
	if rec.Title != "the Pilferess" {
		t.Errorf("title is %q, want a level-one female thief's", rec.Title)
	}
	if len(rec.Skills) != 6 {
		t.Errorf("a new thief knows %d skills, want 6", len(rec.Skills))
	}
	if rec.RemortVector.Empty() {
		t.Error("a new character's remort vector was not set to their own class")
	}

	c.send("quit")
	c.expect("Goodbye")
	c.close()

	// Back in. The name is matched case-insensitively, and the start message
	// belongs only to a character entering for the first time.
	again := dialClient(t, addr)
	again.login("welmar", "hunter2!")
	got = again.transcript()

	if again.seen("This is your new CircleMUD character!") {
		t.Error("a returning character was shown the start message again")
	}
	if !strings.Contains(got, "The Temple Of Midgaard") {
		t.Errorf("a returning character did not land where they left:\n%s", got)
	}
	if !strings.Contains(got, "23H") && !strings.Contains(got, "H 100M") {
		t.Errorf("a returning character's prompt lost their points:\n%s", got)
	}
}

// TestAWrongPasswordIsRefused, and says nothing about which half was wrong.
func TestAWrongPasswordIsRefused(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	rec := &game.PlayerRecord{Name: "Welmar", Class: game.ClassThief}
	cred, err := testAuth.NewCredential("swordfish")
	if err != nil {
		t.Fatal(err)
	}
	game.InitChar(rec, testRNG(), false)
	rec.Credential = cred
	if err := srv.players.Save(context.Background(), rec); err != nil {
		t.Fatal(err)
	}

	c := dialClient(t, addr)
	c.expect("By what name")
	c.send("Welmar")
	c.expect("Password:")
	c.send("not-the-password")

	got := c.expect("Wrong password")
	for _, leak := range []string{"no such", "unknown", "does not exist"} {
		if strings.Contains(strings.ToLower(got), leak) {
			t.Errorf("the refusal distinguishes a bad name from a bad password:\n%s", got)
		}
	}
}

// TestEchoIsTurnedOffForPasswords: the C sends IAC WILL ECHO before a
// password prompt and IAC WONT ECHO after (comm.c's echo_off_str). A player
// left with echo off is a player typing blind, so the off and the on are both
// checked.
func TestEchoIsTurnedOffForPasswords(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))

	c.expect("By what name")
	// Nothing secret is typed at the name prompt, so echo must still be on.
	if bytes.Contains(c.wire(), telnet.Negotiate(telnet.WILL, telnet.OptEcho)) {
		t.Errorf("echo was turned off before the name prompt: % x", c.wire())
	}

	c.send("Zod")
	c.expect("Did I get that right")
	c.send("y")
	c.expect("Give me a password")

	if !bytes.Contains(c.wire(), telnet.Negotiate(telnet.WILL, telnet.OptEcho)) {
		t.Errorf("no IAC WILL ECHO before the password prompt: % x", c.wire())
	}
	if bytes.Contains(c.wire(), telnet.Negotiate(telnet.WONT, telnet.OptEcho)) {
		t.Errorf("echo was turned back on before the password was typed: % x", c.wire())
	}

	c.send("swordfish")
	c.expect("retype password")
	if bytes.Contains(c.wire(), telnet.Negotiate(telnet.WONT, telnet.OptEcho)) {
		t.Errorf("echo was turned back on while a password was still being typed: % x", c.wire())
	}

	c.send("swordfish")
	c.expect("What is your sex")
	if !bytes.Contains(c.wire(), telnet.Negotiate(telnet.WONT, telnet.OptEcho)) {
		t.Errorf("echo was not turned back on after the password: % x", c.wire())
	}
}

// TestOptionsAreOfferedOnConnect covers the negotiation the C does not do at
// all — which is why a modern client's own offers end up in its command
// stream there.
func TestOptionsAreOfferedOnConnect(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.expect("By what name")

	for _, opt := range []byte{telnet.OptCharset, telnet.OptGMCP} {
		if !bytes.Contains(c.wire(), telnet.Negotiate(telnet.WILL, opt)) {
			t.Errorf("%s was not offered: % x", telnet.OptionName(opt), c.wire())
		}
	}
}

// TestSuppressGoAheadIsAgreedButNotOffered is about what a person sees.
//
// Offering SGA is what tips telnet(1) out of line mode and into
// character-at-a-time, where the terminal stops echoing sensibly and stops
// handling backspace, and the server is expected to do both instead. It
// doesn't, so the player gets "^M" for Enter and "^?" for backspace. The C
// server negotiates nothing and stays in line mode. A client that actually
// wants SGA is still answered.
func TestSuppressGoAheadIsAgreedButNotOffered(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.expect("By what name")

	if bytes.Contains(c.wire(), telnet.Negotiate(telnet.WILL, telnet.OptSuppressGoAhead)) {
		t.Errorf("suppress-go-ahead was volunteered: % x", c.wire())
	}

	c.sendRaw(telnet.Negotiate(telnet.DO, telnet.OptSuppressGoAhead))
	c.send("Zod")
	c.expect("Did I get that right")

	if !bytes.Contains(c.wire(), telnet.Negotiate(telnet.WILL, telnet.OptSuppressGoAhead)) {
		t.Errorf("a client asking for suppress-go-ahead was not answered: % x", c.wire())
	}
}

// TestARepeatedRequestIsNotAnsweredTwice is the loop RFC 1143 exists to
// prevent: a client that re-sends DO for an option already on must not get a
// second WILL, or two implementations doing this to each other never stop.
func TestARepeatedRequestIsNotAnsweredTwice(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.expect("By what name")

	c.sendRaw(telnet.Negotiate(telnet.DO, telnet.OptSuppressGoAhead))
	c.send("Zod")
	c.expect("Did I get that right")

	c.sendRaw(telnet.Negotiate(telnet.DO, telnet.OptSuppressGoAhead))
	c.send("y")
	c.expect("Give me a password")

	if n := bytes.Count(c.wire(), telnet.Negotiate(telnet.WILL, telnet.OptSuppressGoAhead)); n != 1 {
		t.Errorf("answered a repeated DO %d times, want 1: % x", n, c.wire())
	}
}

// TestNegotiationNeverReachesTheInterpreter is the bug this whole layer
// exists to prevent: in the C, a client that offers window size has its NAWS
// bytes read as a command.
func TestNegotiationNeverReachesTheInterpreter(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.expect("By what name")

	// Offer window size, then send one, then type a name — all in the shape a
	// real client would.
	c.sendRaw(telnet.Negotiate(telnet.WILL, telnet.OptWindowSize))
	c.sendRaw(telnet.Subnegotiate(telnet.OptWindowSize, []byte{0, 80, 0, 24}))
	c.send("Zod")

	got := c.expect("Did I get that right")
	if !strings.Contains(got, "Did I get that right, Zod") {
		t.Errorf("negotiation disturbed the name that was typed:\n%s", got)
	}
	// And the server declines what it did not ask for, rather than leaving
	// the client waiting for an answer.
	if !bytes.Contains(c.wire(), telnet.Negotiate(telnet.DONT, telnet.OptWindowSize)) {
		t.Errorf("an unrequested option was neither accepted nor refused: % x", c.wire())
	}
}

// TestGMCPCarriesTheVitals: a client that turns GMCP on gets the prompt's
// numbers and the room out of band, which is what makes a browser client
// possible rather than a screen-scraper.
func TestGMCPCarriesTheVitals(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))

	c.expect("By what name")
	c.sendRaw(telnet.Negotiate(telnet.DO, telnet.OptGMCP))
	c.create("Zod", "swordfish", "m", "w")

	var sawVitals, sawRoom bool
	for _, msg := range c.gmcp() {
		switch msg.Package {
		case "Char.Vitals":
			sawVitals = true
			if !strings.Contains(string(msg.Data), `"hp":500`) {
				t.Errorf("Char.Vitals is %s, want the Implementor's 500 hit points", msg.Data)
			}
		case "Room.Info":
			sawRoom = true
			if !strings.Contains(string(msg.Data), "The Immortal Board Room") {
				t.Errorf("Room.Info is %s", msg.Data)
			}
		}
	}
	if !sawVitals {
		t.Error("no Char.Vitals was sent to a client that enabled GMCP")
	}
	if !sawRoom {
		t.Error("no Room.Info was sent to a client that enabled GMCP")
	}
}

// TestGMCPHonoursCoreSupports: a client that asked for nothing but Room gets
// nothing but Room.
func TestGMCPHonoursCoreSupports(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))

	c.expect("By what name")
	c.sendRaw(telnet.Negotiate(telnet.DO, telnet.OptGMCP))
	c.sendRaw(telnet.Subnegotiate(telnet.OptGMCP, []byte(`Core.Supports.Set ["Room 1"]`)))
	c.create("Zod", "swordfish", "m", "w")

	for _, msg := range c.gmcp() {
		if msg.Package == "Char.Vitals" {
			t.Errorf("Char.Vitals was sent to a client that asked only for Room: %s", msg.Data)
		}
	}
}

// TestGMCPIsSilentForAClientThatDidNotAskForIt.
func TestGMCPIsSilentForAClientThatDidNotAskForIt(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	if msgs := c.gmcp(); len(msgs) != 0 {
		t.Errorf("GMCP was sent to a client that never enabled it: %+v", msgs)
	}
}

// TestWalkingBetweenRooms is the rest of the acceptance criterion: a
// character can look around and move.
func TestWalkingBetweenRooms(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	first := dialClient(t, addr)
	first.create("Zod", "swordfish", "m", "w")

	first.send("south")
	if got := first.expect("The Temple Of Midgaard"); !strings.Contains(got, "The Temple Of Midgaard") {
		t.Errorf("could not walk south:\n%s", got)
	}
	first.send("north")
	first.expect("The Immortal Board Room")

	first.send("east")
	if got := first.expect("cannot go that way"); !strings.Contains(got, "cannot go that way") {
		t.Errorf("walking into a wall did not say so:\n%s", got)
	}
}

// TestEveryFormOfEnterEndsALine covers the line endings a client may send.
//
// This is a regression test with a story. The read loop waited for LF, which
// every test here sends and no telnet(1) session does: once the server offers
// SUPPRESS-GO-AHEAD the client goes character-at-a-time and sends CR NUL for
// the Enter key, so a player typing their name at the prompt saw "^M" and a
// server that never answered. The tests all passed, because they were the
// only client that spoke LF.
func TestEveryFormOfEnterEndsALine(t *testing.T) {
	for _, tc := range []struct {
		name string
		eol  string
	}{
		{"CR NUL, what telnet(1) sends", "\r\x00"},
		{"CR LF, the NVT line ending", "\r\n"},
		{"CR alone", "\r"},
		{"LF alone, what a Unix client sends", "\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := newTestServer(t)
			c := dialClient(t, listening(t, srv))

			c.expect("By what name")
			c.sendRaw([]byte("Zod" + tc.eol))
			if got := c.expect("Did I get that right"); !strings.Contains(got, "Did I get that right") {
				t.Errorf("the name was never submitted:\n%s", got)
			}

			// And the pair must not also arrive as a second, empty line: an
			// empty name at this prompt closes the connection.
			c.sendRaw([]byte("y" + tc.eol))
			if got := c.expect("Give me a password"); !strings.Contains(got, "Give me a password") {
				t.Errorf("the confirmation was not accepted:\n%s", got)
			}
		})
	}
}

// TestBackspaceErasesRatherThanBeingRead is the C's own line discipline,
// which this port was missing until #233.
//
// process_input drops a backspace or a DEL and takes back the character
// before it (comm.c:1787). readLoop appended every byte instead, so a client
// that sends its keystrokes as they are typed — the browser terminal always,
// telnet(1) as soon as it has SUPPRESS-GO-AHEAD — had its erases read as
// data: a player who typed "Newcomerr", saw the second r and erased it, was
// answered "Names may only contain letters." for a name that looked correct
// on their screen.
func TestBackspaceErasesRatherThanBeingRead(t *testing.T) {
	for _, tc := range []struct {
		name  string
		typed string
	}{
		{"DEL, what a terminal's Backspace key sends", "Zodd\x7f"},
		{"BS, which is also Ctrl-H", "Zodd\b"},
		{"erasing the whole line and starting again", "Wrong\x7f\x7f\x7f\x7f\x7fZod"},
		{"erasing past the start of the line, which is not an error", "\x7f\x7f\x7fZod"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := newTestServer(t)
			c := dialClient(t, listening(t, srv))

			c.expect("By what name")
			c.sendRaw([]byte(tc.typed + "\r\n"))
			// The name read is the one left on the player's screen, and
			// nothing else: not the typed bytes with the erases in them
			// (which is not a legal name at all — "Names may only contain
			// letters."), and not some prefix of them either.
			got := c.expect("(Y/N)")
			if !strings.Contains(got, "Did I get that right, Zod (Y/N)?") {
				t.Errorf("after typing %q the server read something other "+
					"than Zod:\n%s", tc.typed, got)
			}
		})
	}
}

// TestASplitLineEndingIsStillOneLine checks the CR and the LF arriving in
// different reads, which is what a slow connection does and what the state
// held across the read loop exists for.
func TestASplitLineEndingIsStillOneLine(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))

	c.expect("By what name")
	c.sendRaw([]byte("Zod\r"))
	c.expect("Did I get that right")

	// The LF of the pair, late. It must be swallowed rather than read as an
	// empty answer to the confirmation prompt.
	c.sendRaw([]byte("\n"))
	c.sendRaw([]byte("y\r\n"))
	if got := c.expect("Give me a password"); !strings.Contains(got, "Give me a password") {
		t.Errorf("the late LF was read as a line of its own:\n%s", got)
	}
}

// TestCreditsAndHelpCircleMUD are licence obligations rather than features
// (docs/design/go-port-plan.md §12), so they are tested as such.
func TestCreditsAndHelpCircleMUD(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	c.send("credits")
	c.expect("CREDITS-FILE")
	if !strings.Contains(c.transcript(), testCredits) {
		t.Errorf("`credits` did not show the credits file intact:\n%s", c.transcript())
	}

	// The CIRCLEMUD help entry is the second half of the same obligation,
	// and a different mechanism from `credits`: a real keyword
	// (CIRCLE CIRCLEMUD CREDITS) in the real archived help data
	// (data/text/help/info.hlp), reached by the ordinary help lookup —
	// not the credits file, and not a special case anywhere in the code.
	// The text is identical for all three, so each occurrence has to be
	// asked for by number — expect alone would match the first reply
	// again and return immediately (CLAUDE.md's testing-traps note).
	// The entry runs long enough to page (step 6c-vii's own pager, "Nothing
	// paginates" no longer true) — "q" closes it after each query's own
	// first page, which is already past wantCredits, so the next query
	// reaches the ordinary command dispatcher rather than being read as
	// pager input.
	const wantCredits = "CircleMUD was developed from DikuMud"
	for i, query := range []string{"help circlemud", "help credits", "help circle"} {
		c.send(query)
		c.expectCount(wantCredits, i+1)
		c.expectCount("Return to continue", i+1)
		c.send("q")
	}
	if n := strings.Count(c.transcript(), wantCredits); n != 3 {
		t.Errorf("the real credits entry appeared %d times, want 3 (once per query):\n%s",
			n, c.transcript())
	}
}

// TestTooManyConnectionsFromOneAddress covers the limit the C server has no
// equivalent of.
func TestTooManyConnectionsFromOneAddress(t *testing.T) {
	srv, _ := newTestServer(t)
	ln, err := ListenTelnet("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = srv.Accept(ctx, ln, Limits{MaxPerHost: 2})
	}()
	// Cancel first, then wait: deferred calls run in reverse, so writing
	// these the other way round waits for an accept loop that has not been
	// told to stop.
	defer func() {
		cancel()
		<-done
	}()

	for i := 0; i < 2; i++ {
		held := dialClient(t, ln.Addr().String())
		held.expect("By what name")
	}

	third := dialClient(t, ln.Addr().String())
	third.expect("Too many connections")
}

// TestServerFullRefusesConnections is comm.c's own
// `sockets_connected >= max_players` (comm.c:1337) — checked before a
// connection is even given a hostname, let alone a greeting.
func TestServerFullRefusesConnections(t *testing.T) {
	srv, _ := newTestServer(t)
	ln, err := ListenTelnet("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = srv.Accept(ctx, ln, Limits{MaxPlayers: 2})
	}()
	defer func() {
		cancel()
		<-done
	}()

	held := make([]*client, 0, 2)
	for i := 0; i < 2; i++ {
		c := dialClient(t, ln.Addr().String())
		c.expect("By what name")
		held = append(held, c)
	}

	third := dialClient(t, ln.Addr().String())
	third.expect("Sorry, CircleMUD is full right now")

	// One of the two leaves, freeing a slot for the next arrival — the
	// limit counts live connections, not a lifetime total.
	held[0].close()
	if !eventually(5*time.Second, func() bool { return srv.connections.count() < 2 }) {
		t.Fatal("the closed connection was never dropped from the registry")
	}

	fourth := dialClient(t, ln.Addr().String())
	fourth.expect("By what name")
}

// testRNG is the C server's own generator on a fixed seed: a failing test can
// be reproduced, and the numbers are the ones the C would roll.
func testRNG() *rng.Rand { return rng.NewRand(rng.NewCircle(1)) }

// selfSignedCert makes a certificate for the TLS listener test.
func selfSignedCert(t *testing.T) tls.Certificate {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}
