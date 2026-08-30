// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"golang.org/x/text/encoding/charmap"

	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/persist/bans"
	"github.com/gerrowadat/disgracelands/internal/persist/boards"
	"github.com/gerrowadat/disgracelands/internal/persist/clock"
	"github.com/gerrowadat/disgracelands/internal/persist/help"
	"github.com/gerrowadat/disgracelands/internal/persist/houses"
	"github.com/gerrowadat/disgracelands/internal/persist/mail"
	"github.com/gerrowadat/disgracelands/internal/persist/messages"
	"github.com/gerrowadat/disgracelands/internal/persist/names"
	"github.com/gerrowadat/disgracelands/internal/persist/player"
	"github.com/gerrowadat/disgracelands/internal/persist/player/binary"
	"github.com/gerrowadat/disgracelands/internal/persist/reports"
	"github.com/gerrowadat/disgracelands/internal/persist/socials"
	"github.com/gerrowadat/disgracelands/internal/persist/world"
)

// This file is the "load a subsystem into a comparable Go value" half of
// `dlctl verify --against`. Every type gets one function, and the shape of
// the value it returns is chosen so that two *formats* of the same data
// produce equal values, rather than so that it reads nicely:
//
//   - Anything the C stores as an ordered file but a converter is free to
//     re-order (the roster, the houses) becomes a map keyed by identity,
//     not a slice. A conversion that changes the order of the roster has
//     changed nothing about the game, and a comparison that called that a
//     difference would produce noise for every real archive.
//   - Anything whose order *is* data (a board's messages, a mail queue, a
//     zone's reset commands) stays a slice.
//
// Getting that split wrong in either direction is the main way a
// comparison like this ends up either crying wolf or missing the thing it
// was built to catch.

// loadOptions is what every loader below needs to resolve a directory.
type loadOptions struct {
	base   string
	format string
	// enc is applied to a non-yaml source's free text, so that the two
	// sides of a comparison are in the same encoding. Without it every
	// non-ASCII string in a real archive is a spurious difference, and the
	// interesting ones are buried under them.
	enc *charmap.Charmap
	// mini selects index.mini, for a world loaded the way --mini-mud
	// loads it.
	mini bool
	// objsDir/aliasDir override where a binary/ascii roster's rent and
	// alias files live, matching import --type=pfile's own flags.
	objsDir, aliasDir string
	// houseDir/miscDir do the same for a classic state directory's other
	// two homes.
	houseDir, miscDir string
}

// loadSubsystem dispatches to the right loader.
func loadSubsystem(t dirType, o loadOptions) (any, error) {
	switch t {
	case typeWorld:
		return loadWorldState(o)
	case typePfile:
		return loadPfileState(o)
	case typeState:
		return loadStateState(o)
	case typeNames:
		return loadNamesState(o)
	case typeMessages:
		return loadMessagesState(o)
	case typeSocials:
		return loadSocialsState(o)
	case typeHelp:
		return loadHelpState(o)
	default:
		return nil, fmt.Errorf("verify: unsupported --type %q", t)
	}
}

func loadWorldState(o loadOptions) (any, error) {
	dir, err := resolveDir(typeWorld, o.base, o.format)
	if err != nil {
		return nil, err
	}
	src, err := world.Open(o.format, world.Config{Dir: dir, Mini: o.mini})
	if err != nil {
		return nil, err
	}
	defer func() { _ = src.Close() }()

	var w *game.World
	if fs, ok := src.(world.FindingSource); ok {
		w, _, err = fs.LoadWithWarnings(context.Background())
	} else {
		w, err = src.Load(context.Background())
	}
	if err != nil {
		return nil, err
	}
	if o.format != "yaml" && o.enc != nil {
		transcodeWorldStrings(w, o.enc)
	}
	// Parity mode omits the two mob fields the C loader consumes without
	// recording — the simple/enhanced distinction and the raw espec lines
	// — which is exactly the pair the yaml format folds into `abilities`
	// and does not keep separately either. Comparing with them in would
	// report a difference on every enhanced mobile in the world, all of
	// them the same known, documented, deliberate fold
	// (docs/design/data-format.md §4.7).
	return world.BuildDumpWithOptions(w, world.Options{Parity: true}), nil
}

// pfileState is a roster plus the rent files that go with it, keyed by
// lower-cased name — the same key every format already uses to find a
// character's file, and the only identity a converter is obliged to
// preserve.
type pfileState struct {
	Characters map[string]*game.PlayerRecord
	Rent       map[string]*player.RentFile
}

func loadPfileState(o loadOptions) (any, error) {
	dir, err := resolveDir(typePfile, o.base, o.format)
	if err != nil {
		return nil, err
	}
	store, err := player.Open(o.format, player.Config{Dir: dir, ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer func() { _ = store.Close() }()

	objs, err := openObjectStore(store, dir, o)
	if err != nil {
		return nil, err
	}
	aliases := openAliasStore(store, dir, o)

	state := pfileState{
		Characters: map[string]*game.PlayerRecord{},
		Rent:       map[string]*player.RentFile{},
	}
	ctx := context.Background()
	for entry, err := range store.List(ctx) {
		if err != nil {
			return nil, err
		}
		rec, err := store.Load(ctx, entry.Name)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", entry.Name, err)
		}
		if aliases != nil {
			// A character with no alias file is the ordinary case, not a
			// failure: write_aliases removes the file when the list is
			// empty.
			if as, aerr := aliases.LoadAliases(entry.Name); aerr == nil {
				rec.Aliases = as
			} else if !errors.Is(aerr, player.ErrNotFound) {
				return nil, fmt.Errorf("%s: aliases: %w", entry.Name, aerr)
			}
		}
		if o.format != "yaml" && o.enc != nil {
			transcodePlayerStrings(rec, o.enc)
		}
		key := strings.ToLower(rec.Name)
		state.Characters[key] = rec

		f, err := objs.LoadObjects(ctx, entry.Name)
		switch {
		case errors.Is(err, player.ErrNotFound):
		case err != nil:
			return nil, fmt.Errorf("%s: rent file: %w", entry.Name, err)
		default:
			state.Rent[key] = f
		}
	}
	return state, nil
}

// openObjectStore finds the rent files for a roster. A format that
// implements player.ObjectStore itself (yaml) keeps them in its own files;
// anything else uses binary's, which is what the C wrote and the only
// rent-file codec that exists.
func openObjectStore(store player.Store, dir string, o loadOptions) (player.ObjectStore, error) {
	if os, ok := store.(player.ObjectStore); ok {
		return os, nil
	}
	objsDir, _ := resolveSubdir(o.objsDir, dir, "plrobjs")
	return binary.NewObjectStore(player.Config{Dir: dir, ObjectsDir: objsDir, ReadOnly: true})
}

// openAliasStore does the same for the alias files, and returns nil for a
// format that carries aliases in the character's own record (yaml does).
func openAliasStore(store player.Store, dir string, o loadOptions) *binary.AliasStore {
	if _, ok := store.(player.ObjectStore); ok {
		return nil
	}
	aliasDir, _ := resolveSubdir(o.aliasDir, dir, "plralias")
	as, err := binary.NewAliasStore(player.Config{Dir: dir, AliasDir: aliasDir, ReadOnly: true})
	if err != nil {
		return nil
	}
	return as
}

// orphanPlayerFiles lists what one side's plrobjs/ and plralias/ hold that
// belongs to no character on its roster, and what they hold that is not a
// per-character file at all.
//
// Exactly the same judgement as orphanHouseContents above, one subsystem
// over: it is not part of pfileState, because no server reads either file
// except for a character the pfile has already loaded (nanny's CON_MENU
// case '1' calls read_aliases and then Crash_load, interpreter.c:1646 and
// :1673). Two directories that disagree about them still load to the same
// state, and making it a difference would stop `dlctl import` -- which
// verifies itself and refuses to stamp a directory that differs -- from
// converting any archive whose operator never ran plrobjs/purgeobjs. So it
// is reported beside the verdict rather than inside it (#287).
//
// A yaml side has none by construction and is not looked at: a character's
// rent file and aliases are fields of their own document
// (data-format.md §9), so a file belonging to nobody has nowhere to be.
func orphanPlayerFiles(o loadOptions) (orphans, others []string, err error) {
	dir, err := resolveDir(typePfile, o.base, o.format)
	if err != nil {
		return nil, nil, err
	}
	store, err := player.Open(o.format, player.Config{Dir: dir, ReadOnly: true})
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = store.Close() }()
	// A store that carries objects itself is one of the formats that keeps
	// everything in the character's own file; openAliasStore uses the same
	// test for the same reason.
	if _, ok := store.(player.ObjectStore); ok {
		return nil, nil, nil
	}

	roster := map[string]bool{}
	for entry, lerr := range store.List(context.Background()) {
		if lerr != nil {
			return nil, nil, lerr
		}
		roster[strings.ToLower(strings.TrimSpace(entry.Name))] = true
	}

	objsDir, _ := resolveSubdir(o.objsDir, dir, "plrobjs")
	objs, err := binary.NewObjectStore(player.Config{Dir: dir, ObjectsDir: objsDir, ReadOnly: true})
	if err != nil {
		return nil, nil, err
	}
	objFiles, err := objs.ObjectFiles()
	if err != nil {
		return nil, nil, err
	}
	aliasDir, _ := resolveSubdir(o.aliasDir, dir, "plralias")
	aliases, err := binary.NewAliasStore(player.Config{Dir: dir, AliasDir: aliasDir, ReadOnly: true})
	if err != nil {
		return nil, nil, err
	}
	aliasFiles, err := aliases.AliasFiles()
	if err != nil {
		return nil, nil, err
	}

	for _, f := range []struct {
		suffix string
		files  binary.Files
	}{{"objs", objFiles}, {"alias", aliasFiles}} {
		for _, name := range f.files.Names {
			if !roster[name] {
				orphans = append(orphans, name+"."+f.suffix)
			}
		}
		others = append(others, f.files.Others...)
	}
	sort.Strings(orphans)
	sort.Strings(others)
	return orphans, others, nil
}

// stateState is the six subsystems `--type=state` covers, in one value.
type stateState struct {
	Bans    []bans.Ban
	Boards  map[string][]boards.Message
	Mail    []mail.Message
	Houses  map[int32]houses.House
	Objects map[int32][]player.StoredObject
	Reports []reports.Report
	Clock   string
}

// stateDirs resolves the directories one side of a --type=state comparison
// lives in, and the format name to open its stores with.
//
// Classic spreads what yaml collects into one state/ directory across
// three: etc/ for the clock, boards, mail, bans and the house control
// file, house/ for house objects, misc/ for the reports.
func stateDirs(o loadOptions) (stateDir, houseDir, miscDir, format string, err error) {
	stateDir, err = resolveDir(typeState, o.base, o.format)
	if err != nil {
		return "", "", "", "", err
	}
	houseDir, miscDir = stateDir, stateDir
	if o.format != "yaml" {
		_, houseDir, miscDir = stateClassicDirs(o.base)
		if o.houseDir != "" {
			houseDir = o.houseDir
		}
		if o.miscDir != "" {
			miscDir = o.miscDir
		}
	}
	return stateDir, houseDir, miscDir, nonYamlAs(o.format, "classic"), nil
}

func loadStateState(o loadOptions) (any, error) {
	stateDir, houseDir, miscDir, format, err := stateDirs(o)
	if err != nil {
		return nil, err
	}
	yaml := format == "yaml"
	// Every classic store below is named by a file inside etc/; every yaml
	// one by the directory that holds the lot.
	inEtc := func(name string) string {
		if yaml {
			return stateDir
		}
		return filepath.Join(stateDir, name)
	}

	state := stateState{
		Boards:  map[string][]boards.Message{},
		Houses:  map[int32]houses.House{},
		Objects: map[int32][]player.StoredObject{},
	}

	banStore, err := bans.Open(format, bans.Config{Path: inEtc("badsites"), ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("bans: %w", err)
	}
	defer func() { _ = banStore.Close() }()
	state.Bans = banStore.List()

	boardStore, err := boards.Open(format, boards.Config{Dir: stateDir, ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("boards: %w", err)
	}
	defer func() { _ = boardStore.Close() }()
	for _, def := range game.Boards {
		msgs, err := boardStore.Load(def.File)
		if errors.Is(err, boards.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("boards: %s: %w", def.File, err)
		}
		state.Boards[def.File] = msgs
	}

	mailStore, err := mail.Open(format, mail.Config{Path: inEtc("plrmail"), ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("mail: %w", err)
	}
	defer func() { _ = mailStore.Close() }()
	state.Mail = mailStore.All()

	houseStore, err := houses.Open(format, houses.Config{
		ControlPath: inEtc("hcontrol"), ObjectDir: houseDir, ReadOnly: true,
	})
	if err != nil {
		return nil, fmt.Errorf("houses: %w", err)
	}
	defer func() { _ = houseStore.Close() }()
	list, err := houseStore.Load()
	if err != nil {
		return nil, fmt.Errorf("houses: %w", err)
	}
	for _, h := range list {
		state.Houses[h.Vnum] = h
		objs, err := houseStore.LoadObjects(h.Vnum)
		if err != nil {
			return nil, fmt.Errorf("house #%d: %w", h.Vnum, err)
		}
		if len(objs) > 0 {
			state.Objects[h.Vnum] = objs
		}
	}

	reportStore, err := reports.Open(format, reports.Config{Dir: miscDir, ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("reports: %w", err)
	}
	defer func() { _ = reportStore.Close() }()
	all, err := reportStore.All()
	if err != nil {
		return nil, fmt.Errorf("reports: %w", err)
	}
	state.Reports = all

	epoch, err := clock.Load(format, inEtc("time"))
	if err != nil {
		return nil, fmt.Errorf("clock: %w", err)
	}
	// Kept as a string rather than a time.Time so a difference reads as
	// the timestamp it is. The clock's own save is lossy by a documented
	// amount either way (docs/weirdnumbers.md, "Saving the clock loses up
	// to an hour, on purpose"), which is a property of the C's epoch
	// reconstruction rather than of a format.
	state.Clock = epoch.UTC().Format(time.RFC3339)

	if !yaml && o.enc != nil {
		transcodeStateStrings(&state, o.enc)
	}
	return state, nil
}

// transcodeStateStrings mirrors what importState does field by field:
// boards' headings and bodies, mail text and a report's body carry prose;
// a ban's hostname and admin name, and a house's numeric fields and
// vnum-only stored objects, do not.
func transcodeStateStrings(s *stateState, enc *charmap.Charmap) {
	for _, msgs := range s.Boards {
		for i := range msgs {
			transcodeString(&msgs[i].Heading, enc)
			transcodeString(&msgs[i].Body, enc)
		}
	}
	for i := range s.Mail {
		transcodeString(&s.Mail[i].Text, enc)
	}
	for i := range s.Reports {
		transcodeString(&s.Reports[i].Body, enc)
	}
}

// orphanHouseContents lists the rooms one directory holds house contents
// for that no control record names, with the number of objects in each.
//
// It is deliberately *not* part of stateState, and that is the whole
// judgement in this file. `verify --against`'s claim is that a server
// running on either directory behaves identically (docs/proposals/
// yaml-only.md §4.1), and no server ever reads an orphaned contents file:
// every reader — House_boot, this port's own loadHouseObjects — starts
// from the control array and asks for that house's objects by vnum. A
// file belonging to no house is bytes on a disk that nothing opens. Making
// it a *difference* would fail the comparison for every real archive that
// has one, and `dlctl import` verifies itself and refuses to stamp a
// directory that differs — so an archive with seven years of destroyed
// houses in it could not be converted at all.
//
// Silence was not right either, which is what #239 was about: the check
// built to catch a silent loss enumerated houses from the control records
// exactly like the importer it was checking, so it reported the
// conversion identical while a contents file went missing. So it is
// reported, beside the verdict rather than inside it, saying which side it
// was found on.
func orphanHouseContents(o loadOptions) ([]string, error) {
	stateDir, houseDir, _, format, err := stateDirs(o)
	if err != nil {
		return nil, err
	}
	controlPath := stateDir
	if format != "yaml" {
		controlPath = filepath.Join(stateDir, "hcontrol")
	}
	store, err := houses.Open(format, houses.Config{
		ControlPath: controlPath, ObjectDir: houseDir, ReadOnly: true,
	})
	if err != nil {
		return nil, fmt.Errorf("houses: %w", err)
	}
	defer func() { _ = store.Close() }()

	list, err := store.Load()
	if err != nil {
		return nil, fmt.Errorf("houses: %w", err)
	}
	declared := make(map[int32]bool, len(list))
	for _, h := range list {
		declared[h.Vnum] = true
	}
	withObjects, err := store.ObjectVnums()
	if err != nil {
		return nil, fmt.Errorf("houses: %w", err)
	}

	var out []string
	for _, vnum := range withObjects {
		if declared[vnum] {
			continue
		}
		objs, err := store.LoadObjects(vnum)
		if err != nil {
			return nil, fmt.Errorf("house #%d: %w", vnum, err)
		}
		out = append(out, fmt.Sprintf("#%d (%d object(s))", vnum, len(objs)))
	}
	return out, nil
}

func loadNamesState(o loadOptions) (any, error) {
	path, err := resolveDir(typeNames, o.base, o.format)
	if err != nil {
		return nil, err
	}
	list, err := names.Load(nonYamlAs(o.format, "classic"), path)
	if err != nil {
		return nil, err
	}
	if o.format != "yaml" && o.enc != nil {
		for i := range list {
			transcodeString(&list[i], o.enc)
		}
	}
	return list, nil
}

func loadMessagesState(o loadOptions) (any, error) {
	path, err := resolveDir(typeMessages, o.base, o.format)
	if err != nil {
		return nil, err
	}
	list, err := messages.Load(nonYamlAs(o.format, "classic"), path)
	if err != nil {
		return nil, err
	}
	if o.format != "yaml" && o.enc != nil {
		transcodeFightMessages(list, o.enc)
	}
	return list, nil
}

func loadSocialsState(o loadOptions) (any, error) {
	path, err := resolveDir(typeSocials, o.base, o.format)
	if err != nil {
		return nil, err
	}
	list, err := socials.Load(nonYamlAs(o.format, "classic"), path)
	if err != nil {
		return nil, err
	}
	if o.format != "yaml" && o.enc != nil {
		transcodeSocials(list, o.enc)
	}
	return list, nil
}

func loadHelpState(o loadOptions) (any, error) {
	dir, err := resolveDir(typeHelp, o.base, o.format)
	if err != nil {
		return nil, err
	}
	list, err := help.Load(nonYamlAs(o.format, "classic"), dir)
	if err != nil {
		return nil, err
	}
	if o.format != "yaml" && o.enc != nil {
		for i := range list {
			transcodeString(&list[i].Body, o.enc)
			for j := range list[i].Keywords {
				transcodeString(&list[i].Keywords[j], o.enc)
			}
		}
	}
	return list, nil
}

// nonYamlAs maps every non-yaml format name onto the one name these four
// Load functions understand for "the C's own file". binary and ascii are
// roster formats and never reach here.
func nonYamlAs(format, classic string) string {
	if format == "yaml" {
		return "yaml"
	}
	return classic
}
