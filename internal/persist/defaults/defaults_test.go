// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

// Package defaults_test is the "minimal document" suite
// docs/design/yaml-only.md §6 asks for: one document per subsystem
// holding only its required fields, loaded, with every optional field
// asserted against the value it is supposed to default to.
//
// It exists because of a rule that has to be true before the first new
// field arrives rather than after it. `yaml.Strict()` makes an unknown
// field an error, so a format only grows by adding fields that are
// *optional* — and an optional field's default in Go is whatever the zero
// value happens to be, which is right often enough to be a trap. The first
// field whose sensible default is `true`, or `-1`, or where "unset" and
// "zero" mean different things, is silently wrong for every directory
// written before it existed.
//
// The three rules §6 states, of which this is the third:
//
//  1. A new optional field's default is declared explicitly, never
//     inherited from Go's zero value.
//  2. A new field no legacy format can source is named in the importer's
//     own output, so an operator converting a real archive learns which
//     values are this port's choice rather than their data. (`dlctl
//     import` already does this for the enhanced-mobile espec keys it
//     drops.)
//  3. A minimal-document test per subsystem — this file. It is what stops
//     a default drifting when a struct is edited, and it is cheap.
//
// A test rather than a table of declared defaults in the code: there is
// nothing in the format today whose sensible default is not its zero
// value, so a defaults table would be an empty framework asserting
// nothing. What the rule actually needs is for the current answers to be
// *written down and checked*, which is this, and for the next field to
// have somewhere obvious to declare itself, which is the case it will
// fail here first.
//
// A separate package rather than one test per format package, because the
// property is about the formats *together*: the interesting failure is one
// subsystem quietly disagreeing with the others about what an absent field
// means.
package defaults_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/persist/bans"
	bansyaml "github.com/gerrowadat/disgracelands/internal/persist/bans/yaml"
	"github.com/gerrowadat/disgracelands/internal/persist/boards"
	boardsyaml "github.com/gerrowadat/disgracelands/internal/persist/boards/yaml"
	"github.com/gerrowadat/disgracelands/internal/persist/help"
	"github.com/gerrowadat/disgracelands/internal/persist/houses"
	housesyaml "github.com/gerrowadat/disgracelands/internal/persist/houses/yaml"
	"github.com/gerrowadat/disgracelands/internal/persist/mail"
	mailyaml "github.com/gerrowadat/disgracelands/internal/persist/mail/yaml"
	"github.com/gerrowadat/disgracelands/internal/persist/messages"
	"github.com/gerrowadat/disgracelands/internal/persist/names"
	"github.com/gerrowadat/disgracelands/internal/persist/player"
	playeryaml "github.com/gerrowadat/disgracelands/internal/persist/player/yaml"
	"github.com/gerrowadat/disgracelands/internal/persist/reports"
	reportsyaml "github.com/gerrowadat/disgracelands/internal/persist/reports/yaml"
	"github.com/gerrowadat/disgracelands/internal/persist/socials"
	"github.com/gerrowadat/disgracelands/internal/persist/world"
	worldyaml "github.com/gerrowadat/disgracelands/internal/persist/world/yaml"
)

// writeFile puts a document where a store will find it.
func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestMinimalWorld. A zone with one room, one mobile and one object, each
// carrying only what the schema requires.
func TestMinimalWorld(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "zones.yaml"), "schema: dl/zones@1\nzones:\n  - 1\n")
	writeFile(t, filepath.Join(dir, "1-minimal.yaml"), `schema: dl/world@1
zone:
  vnum: 1
  name: Minimal
  range: [0, 99]
  lifespan: 30
  reset: always
rooms:
  - vnum: 1
    name: A Room
    desc: ""
    sector: inside
mobiles:
  - vnum: 1
    keywords: [thing]
    short: a thing
    long: A thing is here.
    desc: ""
    alignment: 0
    level: 1
    thac0: 20
    ac: 10
    hp: 1d1
    damage: 1d1
    gold: 0
    exp: 0
    position: standing
    default_position: standing
    sex: neutral
objects:
  - vnum: 1
    keywords: [thing]
    short: a thing
    desc: A thing lies here.
    type: other
    weight: 0
    cost: 0
    rent: 0
`)

	src, err := worldyaml.New(world.Config{Dir: dir})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = src.Close() }()
	w, err := src.Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(w.Rooms) != 1 || len(w.Mobiles) != 1 || len(w.Objects) != 1 {
		t.Fatalf("loaded %d rooms, %d mobiles, %d objects; want one of each",
			len(w.Rooms), len(w.Mobiles), len(w.Objects))
	}
	room := w.Rooms[0]
	check(t, "room.Flags", room.Flags, game.RoomFlags{})
	check(t, "room.SectorType", room.SectorType, game.SectorInside)
	check(t, "room.ExtraDescs", len(room.ExtraDescs), 0)
	for dir, exit := range room.Exits {
		if exit != nil {
			t.Errorf("room.Exits[%v] is set in a room that declares none", dir)
		}
	}

	mob := w.Mobiles[0]
	// MobIsNPC is force-set by the loader on every mobile, in both
	// formats, exactly as parse_mobile does — so the default here is not
	// zero, and that is the point of asserting it.
	check(t, "mob.ActionFlags", mob.ActionFlags, game.NewSet(game.MobIsNPC))
	check(t, "mob.AffectionFlags", mob.AffectionFlags, game.AffectFlags{})
	check(t, "mob.Enhanced", mob.Enhanced, false)
	check(t, "mob.Especs", len(mob.Especs), 0)

	obj := w.Objects[0]
	check(t, "obj.ExtraFlags", obj.ExtraFlags, game.ExtraFlagSet{})
	check(t, "obj.WearFlags", obj.WearFlags, game.WearFlagSet{})
	check(t, "obj.PermAffect", obj.PermAffect, int32(0))
	check(t, "obj.MinLevel", obj.MinLevel, int32(0))
	check(t, "obj.Values", obj.Values, [game.NumObjValues]int32{})
	check(t, "obj.Affects", len(obj.Affects), 0)
	check(t, "obj.ExtraDescs", len(obj.ExtraDescs), 0)
}

// TestMinimalPlayer. A character file with nothing in it but an identity.
func TestMinimalPlayer(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "n", "nobody.yaml"), `schema: dl/player@1
id: 7
name: Nobody
identity: {}
times: {}
body: {}
pools:
  hit: {}
  mana: {}
  move: {}
combat:
  saves: {}
wealth: {}
conditions:
  hunger: 0
  thirst: 0
  drunk: 0
flags: {}
`)

	store, err := playeryaml.New(player.Config{Dir: dir})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = store.Close() }()

	rec, err := store.Load(context.Background(), "Nobody")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	check(t, "Name", rec.Name, "Nobody")
	check(t, "IDNum", rec.IDNum, int64(7))
	check(t, "Credential.Scheme", rec.Credential.Scheme, game.SchemeNone)
	check(t, "Level", rec.Level, int32(0))
	check(t, "PlayerFlags", rec.PlayerFlags, game.PlayerFlags{})
	check(t, "AffectFlags", rec.AffectFlags, game.AffectFlags{})
	check(t, "Preferences", rec.Preferences, game.Preferences{})
	check(t, "Skills", len(rec.Skills), 0)
	check(t, "Affects", len(rec.Affects), 0)
	check(t, "Aliases", len(rec.Aliases), 0)
	check(t, "SpecFlags", rec.SpecFlags, int32(0))
	check(t, "OLCZone", rec.OLCZone, int32(0))
	check(t, "RemortVector", rec.RemortVector, game.RemortClasses{})
	if !rec.Birth.IsZero() || !rec.LastLogon.IsZero() {
		t.Errorf("an absent timestamp loaded as %v/%v, want the zero time — not 1970",
			rec.Birth, rec.LastLogon)
	}
	// LoadRoom's absent value is 0 and *means* 0, which the server then
	// treats as "no load room set" alongside game.NoRoom
	// (internal/server/server.go's own check tests both). Asserted here
	// because it is exactly the shape §6 warns about: a field where zero
	// and unset are not obviously the same thing.
	check(t, "LoadRoom", rec.LoadRoom, game.RoomVnum(0))
}

// TestMinimalRentFile. A character with an empty rent block: the file
// exists, and it holds nothing.
func TestMinimalRentFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "n", "nobody.yaml"), `schema: dl/player@1
id: 7
name: Nobody
identity: {}
times: {}
body: {}
pools:
  hit: {}
  mana: {}
  move: {}
combat:
  saves: {}
wealth: {}
conditions:
  hunger: 0
  thirst: 0
  drunk: 0
flags: {}
rent:
  code: crash
`)

	store, err := playeryaml.New(player.Config{Dir: dir})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = store.Close() }()

	f, err := store.LoadObjects(context.Background(), "Nobody")
	if err != nil {
		t.Fatalf("LoadObjects: %v", err)
	}
	check(t, "Code", f.Code, player.RentCrash)
	check(t, "CostPerDay", f.CostPerDay, int32(0))
	check(t, "Gold", f.Gold, int32(0))
	check(t, "Bank", f.Bank, int32(0))
	check(t, "Objects", len(f.Objects), 0)
	if !f.Written.IsZero() {
		t.Errorf("an absent `written` loaded as %v, want the zero time", f.Written)
	}
}

// TestMinimalState covers the five state subsystems whose documents can be
// reduced to a single record, plus the empty case for each.
func TestMinimalState(t *testing.T) {
	t.Run("bans", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "bans.yaml"), "schema: dl/bans@1\nbans:\n  - site: example.org\n    type: all\n")
		store, err := bansyaml.New(bans.Config{Path: dir})
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		defer func() { _ = store.Close() }()
		list := store.List()
		if len(list) != 1 {
			t.Fatalf("loaded %d bans, want 1", len(list))
		}
		check(t, "By", list[0].By, "")
		if !list[0].When.IsZero() {
			t.Errorf("an absent `when` loaded as %v, want the zero time", list[0].When)
		}
	})

	t.Run("boards", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "boards.yaml"),
			"schema: dl/boards@1\nboards:\n  board.mort:\n    - heading: a heading\n")
		store, err := boardsyaml.New(boards.Config{Dir: dir})
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		defer func() { _ = store.Close() }()
		msgs, err := store.Load("board.mort")
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if len(msgs) != 1 {
			t.Fatalf("loaded %d messages, want 1", len(msgs))
		}
		check(t, "Level", msgs[0].Level, int32(0))
		check(t, "Body", msgs[0].Body, "")
	})

	t.Run("mail", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "mail.yaml"),
			"schema: dl/mail@1\nmail:\n  - to: 1\n    from: 2\n    text: hello\n")
		store, err := mailyaml.New(mail.Config{Path: dir})
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		defer func() { _ = store.Close() }()
		all := store.All()
		if len(all) != 1 {
			t.Fatalf("loaded %d messages, want 1", len(all))
		}
		if !all[0].Sent.IsZero() {
			t.Errorf("an absent `sent` loaded as %v, want the zero time", all[0].Sent)
		}
	})

	t.Run("houses", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "houses.yaml"),
			"schema: dl/houses@1\nhouses:\n  - vnum: 1\n    atrium: 2\n    exit: north\n    owner: 3\n")
		store, err := housesyaml.New(houses.Config{ObjectDir: dir})
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		defer func() { _ = store.Close() }()
		list, err := store.Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if len(list) != 1 {
			t.Fatalf("loaded %d houses, want 1", len(list))
		}
		check(t, "Mode", list[0].Mode, int32(0))
		check(t, "Guests", len(list[0].Guests), 0)
		if !list[0].BuiltOn.IsZero() || !list[0].LastPayment.IsZero() {
			t.Errorf("absent timestamps loaded as %v/%v, want the zero time",
				list[0].BuiltOn, list[0].LastPayment)
		}
		objs, err := store.LoadObjects(1)
		if err != nil {
			t.Fatalf("LoadObjects: %v", err)
		}
		check(t, "contents", len(objs), 0)
	})

	t.Run("reports", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "reports.yaml"),
			"schema: dl/reports@1\nreports:\n  - kind: bug\n    body: it broke\n")
		store, err := reportsyaml.New(reports.Config{Dir: dir})
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		defer func() { _ = store.Close() }()
		all, err := store.All()
		if err != nil {
			t.Fatalf("All: %v", err)
		}
		if len(all) != 1 {
			t.Fatalf("loaded %d reports, want 1", len(all))
		}
		check(t, "Reporter", all[0].Reporter, "")
		check(t, "Room", all[0].Room, int32(0))
		if !all[0].When.IsZero() {
			t.Errorf("an absent `when` loaded as %v, want the zero time", all[0].When)
		}
	})
}

// TestMinimalConfig covers the three config/ tables and the help database.
func TestMinimalConfig(t *testing.T) {
	t.Run("names", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "names.yaml"), "schema: dl/names@1\ndisallowed:\n  - nope\n")
		list, err := names.Load("yaml", dir)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		check(t, "names", len(list), 1)
	})

	t.Run("messages", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "messages.yaml"),
			"schema: dl/messages@1\nmessages:\n  - attack_type: kick\n")
		list, err := messages.Load("yaml", dir)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if len(list) != 1 {
			t.Fatalf("loaded %d records, want 1", len(list))
		}
		// Every one of the twelve message slots absent means every one
		// empty, which is what a '#' in each of misc/messages' twelve
		// lines means too.
		for name, set := range map[string]game.MsgSet{
			"die": list[0].Die, "miss": list[0].Miss,
			"hit": list[0].Hit, "god": list[0].God,
		} {
			check(t, name+".Attacker", set.Attacker, "")
			check(t, name+".Victim", set.Victim, "")
			check(t, name+".Room", set.Room, "")
		}
	})

	t.Run("socials", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "socials.yaml"),
			"schema: dl/socials@1\nsocials:\n  - command: ponder\n    hide: false\n    min_victim_position: dead\n")
		list, err := socials.Load("yaml", dir)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if len(list) != 1 {
			t.Fatalf("loaded %d socials, want 1", len(list))
		}
		s := list[0]
		check(t, "CharNoArg", s.CharNoArg, "")
		check(t, "CharFound", s.CharFound, "")
		check(t, "TakesTarget", s.TakesTarget(), false)
	})

	t.Run("help", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "help.yaml"),
			"schema: dl/help@1\nentries:\n  - keywords: [thing]\n    file: thing.txt\n")
		writeFile(t, filepath.Join(dir, "thing.txt"), "THING\n\nA thing.\n")
		list, err := help.Load("yaml", dir)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if len(list) != 1 {
			t.Fatalf("loaded %d entries, want 1", len(list))
		}
		check(t, "keywords", len(list[0].Keywords), 1)
	})
}

// check compares one loaded value against the default it is supposed to
// have, naming the field.
func check[T comparable](t *testing.T, what string, got, want T) {
	t.Helper()
	if got != want {
		t.Errorf("%s defaulted to %v, want %v", what, got, want)
	}
}
