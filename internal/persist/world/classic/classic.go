// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

// Package classic reads the CircleMUD flat-file world format: the "#vnum",
// tilde-terminated, letter-flagged files under data/world.
//
// It is written against the real files in data/world rather than against
// doc/building.txt, on the principle that where the documentation and the
// data disagree the data wins — the C parser is forgiving in ways the
// documentation does not mention and the real world exploits. Every such
// divergence is commented at the point it matters.
package classic

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/persist/world"
)

// FormatName is the name this format registers under.
const FormatName = "classic"

func init() {
	world.Register(FormatName, func(cfg world.Config) (world.Source, error) {
		return New(cfg)
	})
}

// Subdirectory names and file extensions, matching the C *_PREFIX constants.
var recordKinds = []struct {
	dir string
	ext string
}{
	{"wld", "wld"},
	{"mob", "mob"},
	{"obj", "obj"},
	{"zon", "zon"},
	{"shp", "shp"},
}

// Source reads a classic world directory.
type Source struct {
	dir  string
	mini bool
}

// New opens a classic world source. It does not touch the filesystem; a
// missing directory surfaces from Load, where it can be reported alongside
// everything else that is wrong.
func New(cfg world.Config) (*Source, error) {
	if cfg.Dir == "" {
		return nil, fmt.Errorf("classic: no world directory configured")
	}
	return &Source{dir: cfg.Dir, mini: cfg.Mini}, nil
}

// Name implements world.Source.
func (s *Source) Name() string { return FormatName }

// Close implements world.Source. Nothing is held open between calls.
func (s *Source) Close() error { return nil }

// Severity ranks a finding by what it means for someone maintaining the
// world, not by how hard it was to detect.
type Severity int

const (
	// Info: the loader did something to the data that the world file does not
	// say, and someone reading the file would not predict. Not a defect.
	Info Severity = iota
	// Warn: the world is playable but something in it does not work — an exit
	// to nowhere, a reset command referring to a mob that was deleted.
	Warn
	// Error: the C server refuses to boot on this, so it must be fixed before
	// the two servers can be compared at all.
	Error
)

// String returns the severity's lowercase name.
func (s Severity) String() string {
	switch s {
	case Info:
		return "info"
	case Warn:
		return "warn"
	case Error:
		return "error"
	}
	return "?"
}

// Warning is one finding from a load.
type Warning struct {
	Severity Severity
	Message  string
}

func (w Warning) String() string { return w.Severity.String() + ": " + w.Message }

// loader carries the state of one Load call.
type loader struct {
	dir      string
	mini     bool
	warnings []Warning
}

func (l *loader) at(sev Severity, format string, args ...any) {
	l.warnings = append(l.warnings, Warning{Severity: sev, Message: fmt.Sprintf(format, args...)})
}

func (l *loader) infof(format string, args ...any)  { l.at(Info, format, args...) }
func (l *loader) warnf(format string, args ...any)  { l.at(Warn, format, args...) }
func (l *loader) errorf(format string, args ...any) { l.at(Error, format, args...) }

// Load implements world.Source.
func (s *Source) Load(ctx context.Context) (*game.World, error) {
	w, _, err := s.LoadWithWarnings(ctx)
	return w, err
}

// LoadWithWarnings loads the world and returns everything questionable found
// along the way. `dlctl world lint` reports the warnings; the server logs
// them.
func (s *Source) LoadWithWarnings(ctx context.Context) (*game.World, []Warning, error) {
	l := &loader{dir: s.dir, mini: s.mini}
	w := &game.World{}

	for _, kind := range recordKinds {
		files, err := l.indexFiles(kind.dir)
		if err != nil {
			return nil, l.warnings, err
		}
		l.checkOrphans(kind.dir, kind.ext, files)
		for _, name := range files {
			if err := ctx.Err(); err != nil {
				return nil, l.warnings, err
			}
			if err := l.loadFile(w, kind.dir, name); err != nil {
				return nil, l.warnings, err
			}
		}
	}

	l.assignRoomZones(w)
	l.resolveShopReferences(w)
	l.checkReferences(w)

	return w, l.warnings, nil
}

// indexFiles reads a subdirectory's index file, which lists the data files to
// load in order and is terminated by a line containing '$'.
//
// Load order is significant: it determines real numbers, and the C loader
// requires rooms to arrive in ascending zone order because parse_room walks a
// zone cursor forward and never back.
func (l *loader) indexFiles(sub string) ([]string, error) {
	indexName := "index"
	if l.mini {
		indexName = "index.mini"
	}
	path := filepath.Join(l.dir, sub, indexName)

	data, err := os.ReadFile(path) //nolint:gosec // path is operator-configured
	if err != nil {
		return nil, fmt.Errorf("reading world index: %w", err)
	}

	var files []string
	for _, line := range strings.Split(string(data), "\n") {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		if name == "$" {
			return files, nil
		}
		files = append(files, name)
	}

	// The C loader reads with fscanf until it sees '$' and would run off the
	// end of the file rather than stop, so a missing terminator is a real
	// defect even though everything happens to work.
	l.warnf("%s: index has no '$' terminator", path)
	return files, nil
}

// checkOrphans reports data files sitting in a world subdirectory that no
// index lists.
//
// This is silent in the C server — files not in the index are simply never
// opened — and it is a real trap: a builder who adds a zone and forgets the
// index gets no error, just a world that quietly lacks their work. The
// Disgracelands world has four complete zones in this state.
func (l *loader) checkOrphans(sub, ext string, indexed []string) {
	entries, err := os.ReadDir(filepath.Join(l.dir, sub))
	if err != nil {
		// Not fatal: the index already loaded, so the directory is readable
		// enough for the server to work.
		l.warnf("cannot list %s/ to check for unindexed files: %v", sub, err)
		return
	}

	listed := make(map[string]bool, len(indexed))
	for _, n := range indexed {
		listed[n] = true
	}

	var orphans []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, "."+ext) || listed[name] {
			continue
		}
		orphans = append(orphans, name)
	}
	if len(orphans) == 0 {
		return
	}
	sort.Strings(orphans)
	l.warnf("%s/: %d file(s) are not listed in the index and will never be loaded: %s",
		sub, len(orphans), strings.Join(orphans, ", "))
}

// loadFile dispatches one data file to the right record parser.
func (l *loader) loadFile(w *game.World, sub, name string) error {
	path := filepath.Join(l.dir, sub, name)
	f, err := os.Open(path) //nolint:gosec // path comes from the world index
	if err != nil {
		return fmt.Errorf("opening world file: %w", err)
	}
	defer func() { _ = f.Close() }()

	r := newReader(f, path)

	switch sub {
	case "zon":
		zone, err := l.parseZone(r, path)
		if err != nil {
			return err
		}
		w.Zones = append(w.Zones, zone)
		return nil
	case "shp":
		return l.parseShopFile(w, r, path)
	}

	return l.loadRecords(w, r, sub, path)
}

// loadRecords runs the "#vnum ... $" record loop shared by the room, mob and
// object files. Mirrors discrete_load() in db.c.
func (l *loader) loadRecords(w *game.World, r *reader, sub, path string) error {
	seenAny := false

	for {
		line, ok := r.getLine()
		if !ok {
			if !seenAny {
				return fmt.Errorf("%s: file is empty", path)
			}
			return fmt.Errorf("%s: file ended without a '$' terminator", path)
		}

		if strings.HasPrefix(line, "$") {
			return nil
		}
		if !strings.HasPrefix(line, "#") {
			return fmt.Errorf("%s: expected '#<vnum>' or '$', got %q", r.where(""), line)
		}

		vnum, ok := scanInt(line[1:])
		if !ok {
			return fmt.Errorf("%s: record number %q is not a number", r.where(""), line)
		}
		seenAny = true

		// The C loader treats a vnum of 99999 or more as an end-of-file
		// marker, whatever follows it.
		if vnum >= 99999 {
			l.warnf("%s: record #%d is at or above 99999, which the loader treats as end of file; the rest of %s is ignored",
				r.where(""), vnum, path)
			return nil
		}

		// scanInt saturates rather than wrapping, so a vnum too large for 32
		// bits arrives as MaxInt32 and is caught by the 99999 test above. A
		// negative one is nonsense in any case.
		id := vnum
		if id < 0 {
			return fmt.Errorf("%s: record number %d is negative", r.where(""), id)
		}

		switch sub {
		case "wld":
			room, err := l.parseRoom(r, game.RoomVnum(id))
			if err != nil {
				return err
			}
			w.Rooms = append(w.Rooms, room)
		case "mob":
			mob, err := l.parseMobile(r, game.MobVnum(id))
			if err != nil {
				return err
			}
			w.Mobiles = append(w.Mobiles, mob)
		case "obj":
			obj, err := l.parseObject(r, game.ObjVnum(id))
			if err != nil {
				return err
			}
			w.Objects = append(w.Objects, obj)
		default:
			return fmt.Errorf("internal error: unknown record kind %q", sub)
		}
	}
}

// assignRoomZones fills in each room's owning zone from the zone vnum ranges.
//
// The C loader does this inline in parse_room with a cursor that only moves
// forward, and exits the process if a room falls outside every zone. Doing it
// as a separate pass means a room in no zone can be reported as one problem
// among many rather than killing the load.
func (l *loader) assignRoomZones(w *game.World) {
	zones := make([]*game.ZoneDef, len(w.Zones))
	copy(zones, w.Zones)
	sort.Slice(zones, func(i, j int) bool { return zones[i].Bottom < zones[j].Bottom })

	for _, room := range w.Rooms {
		found := false
		for _, z := range zones {
			if room.Vnum >= z.Bottom && room.Vnum <= z.Top {
				room.Zone = z.Vnum
				found = true
				break
			}
		}
		if !found {
			room.Zone = -1
			l.errorf("room #%d is outside every zone's vnum range; the C loader exits on this", room.Vnum)
		}
	}
}

// checkReferences reports vnums pointed at by something but never defined.
// The C loader resolves these into real numbers in renum_world() and
// renum_zone_table() and silently drops the ones that do not resolve.
func (l *loader) checkReferences(w *game.World) {
	rooms := make(map[game.RoomVnum]bool, len(w.Rooms))
	for _, r := range w.Rooms {
		if rooms[r.Vnum] {
			l.warnf("room #%d is defined more than once; the later definition is a separate room with the same vnum", r.Vnum)
		}
		rooms[r.Vnum] = true
	}
	mobs := make(map[game.MobVnum]bool, len(w.Mobiles))
	for _, m := range w.Mobiles {
		if mobs[m.Vnum] {
			l.warnf("mob #%d is defined more than once", m.Vnum)
		}
		mobs[m.Vnum] = true
	}
	objs := make(map[game.ObjVnum]bool, len(w.Objects))
	for _, o := range w.Objects {
		if objs[o.Vnum] {
			l.warnf("object #%d is defined more than once", o.Vnum)
		}
		objs[o.Vnum] = true
	}

	for _, room := range w.Rooms {
		for dir, exit := range room.Exits {
			if exit == nil {
				continue
			}
			if exit.ToRoom != game.NoRoom && !rooms[exit.ToRoom] {
				l.warnf("room #%d's %s exit leads to room #%d, which does not exist; the exit will be unusable",
					room.Vnum, game.Direction(dir), exit.ToRoom)
			}
			if exit.Key > 0 && !objs[exit.Key] {
				l.warnf("room #%d's %s exit is locked by object #%d, which does not exist",
					room.Vnum, game.Direction(dir), exit.Key)
			}
		}
	}

	for _, zone := range w.Zones {
		for _, cmd := range zone.Commands {
			l.checkResetCommand(zone, cmd, rooms, mobs, objs)
		}
	}
}

// checkResetCommand validates the vnums one reset command refers to. Which
// argument means what depends on the opcode; see reset_zone() in db.c.
func (l *loader) checkResetCommand(zone *game.ZoneDef, cmd game.ResetCommand,
	rooms map[game.RoomVnum]bool, mobs map[game.MobVnum]bool, objs map[game.ObjVnum]bool) {

	where := fmt.Sprintf("zone #%d line %d: command %q", zone.Vnum, cmd.Line, string(cmd.Command))

	switch cmd.Command {
	case 'M': // load mobile Arg1 into room Arg3, max Arg2 in the world
		if !mobs[game.MobVnum(cmd.Arg1)] {
			l.warnf("%s loads mob #%d, which does not exist", where, cmd.Arg1)
		}
		if !rooms[game.RoomVnum(cmd.Arg3)] {
			l.warnf("%s loads into room #%d, which does not exist", where, cmd.Arg3)
		}
	case 'O': // load object Arg1 into room Arg3
		if !objs[game.ObjVnum(cmd.Arg1)] {
			l.warnf("%s loads object #%d, which does not exist", where, cmd.Arg1)
		}
		if !rooms[game.RoomVnum(cmd.Arg3)] {
			l.warnf("%s loads into room #%d, which does not exist", where, cmd.Arg3)
		}
	case 'G', 'E': // give/equip object Arg1 to the last-loaded mob
		if !objs[game.ObjVnum(cmd.Arg1)] {
			l.warnf("%s gives object #%d, which does not exist", where, cmd.Arg1)
		}
	case 'P': // put object Arg1 into object Arg3
		if !objs[game.ObjVnum(cmd.Arg1)] {
			l.warnf("%s puts object #%d, which does not exist", where, cmd.Arg1)
		}
		if !objs[game.ObjVnum(cmd.Arg3)] {
			l.warnf("%s puts into container #%d, which does not exist", where, cmd.Arg3)
		}
	case 'D': // set door state in room Arg1, direction Arg2
		if !rooms[game.RoomVnum(cmd.Arg1)] {
			l.warnf("%s sets a door in room #%d, which does not exist", where, cmd.Arg1)
		}
		if _, ok := game.DirectionFromInt(cmd.Arg2); !ok {
			l.warnf("%s sets a door in direction %d, which is not a direction", where, cmd.Arg2)
		}
	case 'R': // remove object Arg2 from room Arg1
		if !rooms[game.RoomVnum(cmd.Arg1)] {
			l.warnf("%s removes from room #%d, which does not exist", where, cmd.Arg1)
		}
		if !objs[game.ObjVnum(cmd.Arg2)] {
			l.warnf("%s removes object #%d, which does not exist", where, cmd.Arg2)
		}
	default:
		l.warnf("%s is not a reset command the server understands", where)
	}
}
