// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/gerrowadat/disgracelands/internal/buildinfo"
	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/persist/convert"
	"github.com/gerrowadat/disgracelands/internal/persist/dataversion"
	"github.com/gerrowadat/disgracelands/internal/persist/help"
	"github.com/gerrowadat/disgracelands/internal/persist/messages"
	"github.com/gerrowadat/disgracelands/internal/persist/names"
	"github.com/gerrowadat/disgracelands/internal/persist/player"
	"github.com/gerrowadat/disgracelands/internal/persist/player/binary"
	playeryaml "github.com/gerrowadat/disgracelands/internal/persist/player/yaml"
	"github.com/gerrowadat/disgracelands/internal/persist/socials"
	"github.com/gerrowadat/disgracelands/internal/persist/world"
	"github.com/gerrowadat/disgracelands/internal/persist/world/classic"
	worldyaml "github.com/gerrowadat/disgracelands/internal/persist/world/yaml"
)

// importOptions is the resolved (still base, not yet subsystem-specific)
// input to every per-type importer below.
type importOptions struct {
	fromDir, toDir string
	// fromFormat is "" for "let the type pick its own default" — cmdImport
	// fills it in before calling runImport directly, but cmdImportAll (the
	// --type-less "convert everything" path) leaves it blank per type on
	// purpose, since each type's own default differs (binary for pfile,
	// classic for everything else).
	fromFormat string
	encName    string
	mini       bool // world only
	// fromHouseDir/fromMiscDir override state's own default derivation
	// (stateClassicDirs) for an archive that keeps house/ or misc/
	// somewhere unexpected.
	fromHouseDir, fromMiscDir string
	// fromObjsDir/fromAliasDir override pfile's own default derivation
	// (resolveSubdir) of plrobjs/ and plralias/ for a layout that is
	// neither this port's own (child of the roster directory) nor the C's
	// (sibling of it).
	fromObjsDir, fromAliasDir string
	// verify runs the comparison in verifyImport once the conversion is
	// done, and is on by default.
	verify bool
}

// cmdImport converts a classic (or, for pfile, binary/ascii) directory into
// yaml. With --type, it converts one subsystem; without, it converts every
// subsystem in one pass — world, pfile, state, names, messages, socials,
// help, in that order (world first: it is independent of the rest, and a
// failure there is worth knowing about before the smaller conversions
// bother running) — plus copying text/'s plain-prose files unchanged
// (internal/server/text.go reads them from the same text/<name> path
// regardless of any --*-format flag, so they are never a pluggable format)
// and config/game.yaml, the game tuning, for the same reason and a
// stronger one (see copyGameConfig), and, once everything has actually
// succeeded, stamping --to-dir with this
// build's own data-format version (docs/design/data-format-versioning.md).
// This is what used to be the separate `dlctl lib import` command; folding
// it into `import` with --type omitted means one flag surface serves both
// "convert everything" and "convert one subsystem" rather than two
// differently-shaped commands.
func cmdImport(args []string) error {
	fs := flag.NewFlagSet("import", flag.ContinueOnError)
	typeRaw := fs.String("type", "", "Subsystem to import: "+joinTypes(allTypes)+" (omit to import every subsystem at once)")
	// Both default empty rather than to "data": with no --type, a missing
	// --from-dir/--to-dir is refused outright (cmdImportAll) rather than
	// silently defaulting a whole-archive migration's source and
	// destination to the same place. With --type, each importer defaults
	// its own empty base to "data" (withDefaultBase) — that default is
	// per-subsystem-safe the way a whole-lib one is not.
	fromDir := fs.String("from-dir", "", "Source lib directory (base — e.g. its own world/, etc/, misc/, house/, text/; default \"data\" with --type)")
	toDir := fs.String("to-dir", "", "Destination lib directory (base), written fresh in yaml throughout (default \"data\" with --type)")
	fromFormat := fs.String("from-format", "", "Source format (default: classic, or binary for --type=pfile)")
	encName := fs.String("encoding", convert.DefaultEncoding, fmt.Sprintf("Source text encoding: %v", encodingNames()))
	mini := fs.Bool("mini-mud", false, "Use the reduced index.mini file list (--type=world only)")
	fromHouseDir := fs.String("from-house-dir", "", "Source (classic) directory for per-room house object files "+
		"(--type=state only; default derived from --from-dir)")
	fromMiscDir := fs.String("from-misc-dir", "", "Source (classic) directory for bug/idea/typo report files "+
		"(--type=state only; default derived from --from-dir)")
	fromObjsDir := fs.String("from-objs-dir", "", "Source plrobjs/ directory "+
		"(--type=pfile only; default: beside or inside --from-dir, whichever exists)")
	fromAliasDir := fs.String("from-alias-dir", "", "Source plralias/ directory "+
		"(--type=pfile only; default: beside or inside --from-dir, whichever exists)")
	verify := fs.Bool("verify", true, "After importing, load both directories and check they agree "+
		"(`dlctl verify --against`); --verify=false to skip it")
	if err := fs.Parse(args); err != nil {
		return err
	}

	o := importOptions{
		fromDir: *fromDir, toDir: *toDir, fromFormat: *fromFormat, encName: *encName, mini: *mini,
		fromHouseDir: *fromHouseDir, fromMiscDir: *fromMiscDir,
		fromObjsDir: *fromObjsDir, fromAliasDir: *fromAliasDir,
		verify: *verify,
	}

	if *typeRaw == "" {
		return cmdImportAll(o)
	}
	t, err := parseType(*typeRaw, allTypes)
	if err != nil {
		return err
	}
	if err := runImport(t, o); err != nil {
		return err
	}
	return verifyImport([]dirType{t}, o)
}

// verifyImport is `dlctl verify --against` run on what `import` has just
// written, and it is **on by default**.
//
// docs/proposals/yaml-only.md §3.4 asks for that, and the reason it is
// the default rather than a flag somebody remembers is the release this
// is for: after it, `dlctl import` is the only path from an archived
// lib/ to a running server, run once, by an operator who has no way to
// tell a complete conversion from a nearly-complete one. Every finding
// this checking has produced so far — a write-only escape hatch, two
// missing fields, a reversed ban list, keywords mangled into U+FFFD —
// was silent, and every one of them would have been shipped by an import
// that reported success and stopped there.
func verifyImport(types []dirType, o importOptions) error {
	if !o.verify {
		return nil
	}
	enc, ok := convert.Encodings[o.encName]
	if !ok {
		return fmt.Errorf("unknown encoding %q (have: %v)", o.encName, encodingNames())
	}
	fmt.Println("== verify ==")
	left := loadOptions{
		base: withDefaultBase(o.fromDir), format: o.fromFormat, enc: enc, mini: o.mini,
		objsDir: o.fromObjsDir, aliasDir: o.fromAliasDir,
		houseDir: o.fromHouseDir, miscDir: o.fromMiscDir,
	}
	right := loadOptions{base: withDefaultBase(o.toDir), format: "yaml", enc: enc, mini: o.mini}
	return verifyAgainst(types, left, right)
}

// withDefaultBase applies "data" as a --type-scoped import/export's own
// base-directory default — safe there in a way it is not for a whole-lib
// migration (cmdImportAll refuses an empty --from-dir/--to-dir outright
// instead).
func withDefaultBase(dir string) string {
	if dir == "" {
		return "data"
	}
	return dir
}

// runImport dispatches to one subsystem's importer.
func runImport(t dirType, o importOptions) error {
	switch t {
	case typeWorld:
		return importWorld(o)
	case typePfile:
		return importPfile(o)
	case typeState:
		return importState(o)
	case typeNames:
		return importNames(o)
	case typeMessages:
		return importMessages(o)
	case typeSocials:
		return importSocials(o)
	case typeHelp:
		return importHelp(o)
	default:
		return fmt.Errorf("import: unsupported --type %q", t)
	}
}

// cmdImportAll runs every type's importer in turn against the same --from-dir/
// --to-dir base, the seven-subsystems-in-one-pass job `dlctl lib import`
// used to be its own command for.
func cmdImportAll(o importOptions) error {
	if o.fromDir == "" || o.toDir == "" {
		return fmt.Errorf("both --from-dir and --to-dir are required")
	}

	var failed []string
	for _, t := range allTypes {
		fmt.Printf("== %s import ==\n", t)
		step := o
		step.fromFormat = "" // let each type pick its own default
		if err := runImport(t, step); err != nil {
			if !errors.Is(err, errQuiet) {
				fmt.Fprintf(os.Stderr, "%s import: %v\n", t, err)
			}
			failed = append(failed, string(t))
		}
		fmt.Println()
	}

	fmt.Println("== text ==")
	n, err := copyTextFiles(o.fromDir, o.toDir)
	switch {
	case err != nil:
		fmt.Fprintf(os.Stderr, "copying text/: %v\n", err)
		failed = append(failed, "text")
	default:
		fmt.Printf("copied %d plain text file(s) unchanged (not a pluggable format)\n", n)
	}
	fmt.Println()

	fmt.Println("== game config ==")
	switch copied, err := copyGameConfig(o.fromDir, o.toDir); {
	case err != nil:
		fmt.Fprintf(os.Stderr, "copying config/game.yaml: %v\n", err)
		failed = append(failed, "game config")
	case copied:
		fmt.Println("copied config/game.yaml unchanged (already this project's own yaml)")
	default:
		fmt.Println("no config/game.yaml to copy; the defaults are config.c's own")
	}
	fmt.Println()

	if len(failed) > 0 {
		fmt.Printf("Failed: %s. %s was not stamped with a release version — fix these and re-run.\n",
			strings.Join(failed, ", "), o.toDir)
		return errQuiet
	}

	// The stamp is this build's own release version (docs/design/data-
	// format-versioning.md): a directory records which dlctl made it, and
	// dlmud checks that against its own release before it will boot. An
	// unreleased dlctl — `go run`, `go test`, a plain `go build` — has no
	// release to name, so it writes no stamp rather than inventing one.
	// Say so: an operator who expected a stamp should find out here, from
	// the tool that did not write it, rather than from a server that
	// later checked nothing.
	if err := verifyImport(allTypes, o); err != nil {
		fmt.Printf("%s was not stamped with a release version: it does not load to the same state as %s.\n",
			o.toDir, o.fromDir)
		return err
	}
	fmt.Println()

	current, ok := dataversion.Current()
	if !ok {
		fmt.Printf("%s is a complete yaml directory. This build (%s) has no release version, so it was not stamped with one.\n",
			o.toDir, buildinfo.Get().Version)
		return nil
	}
	if err := dataversion.Write(o.toDir, current); err != nil {
		return fmt.Errorf("stamping %s: %w", o.toDir, err)
	}
	fmt.Printf("%s is a complete yaml directory, written by release %s.\n", o.toDir, current)
	return nil
}

// copyGameConfig copies fromDir/config/game.yaml across unchanged, and
// reports whether there was one.
//
// It is not converted because there is nothing to convert: the game tuning
// (internal/config's LoadGameTuning, docs/design/data-format.md §6) is this
// project's own invention rather than anything the C wrote, so it is the
// same yaml file in a classic directory as in a yaml one — the same
// reasoning that copies text/'s prose rather than importing it. Carrying it
// is the point: a lib/ that has been tuned must not silently lose its
// tuning on the way through a format conversion.
func copyGameConfig(fromDir, toDir string) (bool, error) {
	from := filepath.Join(fromDir, "config", "game.yaml")
	if _, err := os.Stat(from); os.IsNotExist(err) {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("reading %s: %w", from, err)
	}

	to := filepath.Join(toDir, "config")
	if err := os.MkdirAll(to, 0o750); err != nil {
		return false, fmt.Errorf("creating %s: %w", to, err)
	}
	if err := copyFile(from, filepath.Join(to, "game.yaml")); err != nil {
		return false, err
	}
	return true, nil
}

// copyTextFiles copies every regular file directly inside fromDir/text —
// the plain-prose canned texts (motd, credits, background, ...) — into
// toDir/text, unchanged. text/help/ is a subdirectory, not a plain file,
// so os.ReadDir's own entries already exclude it without needing a name
// check; help import is what converts it. A missing fromDir/text is not
// an error: the server treats missing canned text as "a poorer game, and
// still a game" (internal/server/text.go), and an archive with none is a
// legitimate source to import from.
func copyTextFiles(fromDir, toDir string) (int, error) {
	from := filepath.Join(fromDir, "text")
	entries, err := os.ReadDir(from)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("reading %s: %w", from, err)
	}

	to := filepath.Join(toDir, "text")
	if err := os.MkdirAll(to, 0o750); err != nil {
		return 0, fmt.Errorf("creating %s: %w", to, err)
	}

	var n int
	for _, e := range entries {
		if !e.Type().IsRegular() {
			continue
		}
		if err := copyFile(filepath.Join(from, e.Name()), filepath.Join(to, e.Name())); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

func copyFile(from, to string) error {
	src, err := os.Open(from) //nolint:gosec // operator-configured source directory
	if err != nil {
		return fmt.Errorf("reading %s: %w", from, err)
	}
	defer func() { _ = src.Close() }()

	dst, err := os.Create(to) //nolint:gosec // operator-configured destination directory
	if err != nil {
		return fmt.Errorf("writing %s: %w", to, err)
	}
	if _, err := io.Copy(dst, src); err != nil {
		_ = dst.Close()
		return fmt.Errorf("writing %s: %w", to, err)
	}
	return dst.Close()
}

// helpScreenName is HELP_PAGE_FILE's own basename.
const helpScreenName = "screen"

// copyHelpScreen copies text/help/screen from one directory to another,
// reporting whether there was one to copy.
//
// A missing screen is not an error, for the same reason a missing motd is
// not: the server treats absent canned text as a poorer game and still a
// game (internal/server/text.go). Nothing happens when the two directories
// are the same, which is import --type=help's own default.
func copyHelpScreen(fromDir, toDir string) (bool, error) {
	if fromDir == toDir {
		return false, nil
	}
	src := filepath.Join(fromDir, helpScreenName)
	if _, err := os.Stat(src); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("reading %s: %w", src, err)
	}
	// 0o750, matching copyTextFiles' own choice for the plain prose beside
	// this: canned text is not a secret, but nothing here has to be
	// world-readable to be served.
	if err := os.MkdirAll(toDir, 0o750); err != nil {
		return false, fmt.Errorf("creating %s: %w", toDir, err)
	}
	if err := copyFile(src, filepath.Join(toDir, helpScreenName)); err != nil {
		return false, err
	}
	return true, nil
}

// importWorld converts a classic world directory into yaml, per
// docs/design/data-format.md §11 step 2. It replaces nothing the C server
// reads — classic stays the parity oracle — and produces a zones.yaml
// manifest listing every zone the source loaded, enabled, plus one YAML
// file per zone.
func importWorld(o importOptions) error {
	fromFormat := o.fromFormat
	if fromFormat == "" {
		fromFormat = "classic"
	}
	if fromFormat != "classic" {
		return fmt.Errorf("import --type=world only reads a classic source (got --from-format=%q)", fromFormat)
	}
	enc, ok := convert.Encodings[o.encName]
	if !ok {
		return fmt.Errorf("unknown encoding %q (have: %v)", o.encName, encodingNames())
	}
	fromDir, err := resolveDir(typeWorld, withDefaultBase(o.fromDir), fromFormat)
	if err != nil {
		return err
	}
	toDir, err := resolveDir(typeWorld, withDefaultBase(o.toDir), "yaml")
	if err != nil {
		return err
	}

	src, err := classic.New(world.Config{Dir: fromDir, Mini: o.mini})
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()

	w, warnings, err := src.LoadWithWarnings(context.Background())
	if err != nil {
		return fmt.Errorf("loading %s: %w", fromDir, err)
	}

	out := bufio.NewWriter(os.Stdout)
	defer func() { _ = out.Flush() }()

	for _, wr := range warnings {
		_, _ = fmt.Fprintf(out, "%s\n", wr)
	}

	transcoded := transcodeWorldStrings(w, enc)

	// A world with records and no zones converts to an empty directory,
	// silently. The yaml format is organised one file per zone, and a
	// record whose vnum falls outside every zone's range is written under
	// the zone whose range begins nearest below it (writer.go's
	// fallbackZone) — which works for every real world and has nowhere to
	// put anything at all when the zone list is empty.
	//
	// Refusing is right rather than warning: the output would be a
	// complete, well-formed, empty world, and `import` would report
	// success next to it. Found by FuzzClassicRecordRoundTrip, which
	// generated exactly this shape within seconds.
	if len(w.Zones) == 0 && len(w.Rooms)+len(w.Mobiles)+len(w.Objects)+len(w.Shops) > 0 {
		return fmt.Errorf("%s has %d room(s), %d mobile(s), %d object(s) and %d shop(s) but no zones; "+
			"the yaml format is organised one file per zone and has nowhere to put them "+
			"(check %s/zon/index)",
			fromDir, len(w.Rooms), len(w.Mobiles), len(w.Objects), len(w.Shops), fromDir)
	}

	if err := os.MkdirAll(toDir, 0o755); err != nil { //nolint:gosec // world data, not secrets
		return err
	}
	entries := make([]worldyaml.ManifestEntry, 0, len(w.Zones))
	for _, z := range w.Zones {
		entries = append(entries, worldyaml.ManifestEntry{Vnum: int32(z.Vnum), Enabled: true})
	}
	if err := worldyaml.WriteManifest(toDir, entries); err != nil {
		return fmt.Errorf("writing zones.yaml: %w", err)
	}

	nsrc, err := worldyaml.New(world.Config{Dir: toDir})
	if err != nil {
		return err
	}
	defer func() { _ = nsrc.Close() }()

	for _, z := range w.Zones {
		if err := nsrc.WriteZone(context.Background(), z, w); err != nil {
			return fmt.Errorf("writing zone %d: %w", z.Vnum, err)
		}
	}

	_, _ = fmt.Fprintf(out, "\nimported %d zone(s), %d room(s), %d mobile(s), %d object(s), %d shop(s)\n",
		len(w.Zones), len(w.Rooms), len(w.Mobiles), len(w.Objects), len(w.Shops))
	if transcoded > 0 {
		_, _ = fmt.Fprintf(out, "transcoded %d string(s) from %s to UTF-8\n", transcoded, o.encName)
	}
	reportDroppedEspecs(out, w)
	return out.Flush()
}

// reportDroppedEspecs names every enhanced-mobile espec the yaml format
// has nowhere to put, because the conversion drops it.
//
// The dropping itself is correct and deliberate: §4.7 of
// docs/design/data-format.md makes `abilities` a closed, typed mapping
// over the eight keys the stock world and a snapshot of the original one
// contain between them, precisely because interpret_espec ignores
// anything else — "a typo in it currently does nothing at all", so an
// unrecognised key has never had a gameplay effect to lose. What was
// wrong was doing it in silence. This is the conversion boundary where a
// value stops existing, and docs/proposals/yaml-only.md §6 rule 2 says an
// importer names what it could not carry across.
//
// import is the only place this can arise: `fmt` reads a directory that
// is already yaml, and the yaml reader treats an unknown key inside
// `abilities` as a load error rather than a thing to preserve.
func reportDroppedEspecs(out *bufio.Writer, w *game.World) {
	var dropped []string
	for _, m := range w.Mobiles {
		if !m.Enhanced {
			continue
		}
		if _, unknown := worldyaml.AbilitiesFromEspecs(m.Especs); len(unknown) > 0 {
			dropped = append(dropped, fmt.Sprintf("mobile #%d: %s", m.Vnum, strings.Join(unknown, ", ")))
		}
	}
	if len(dropped) == 0 {
		return
	}
	_, _ = fmt.Fprintf(out, "dropped %d enhanced-mobile espec key(s) the yaml format has no field for "+
		"(the C ignores them too, see data-format.md §4.7):\n", len(dropped))
	for _, d := range dropped {
		_, _ = fmt.Fprintf(out, "  %s\n", d)
	}
}

// importPfile converts a roster into yaml, per step 5's "getting there" —
// the players counterpart of importWorld. Rent/crash and alias files are
// not pluggable the way the roster is (the C has one format for each), so
// they are read via resolveSubdir rather than --from-format: it tries
// --from-dir's own child directory first (this port's own layout, one
// directory holding a roster and everything that goes with it), then the
// C's own layout, plrobjs/plralias as a *sibling* of the roster directory
// (the C resolves LIB_PLROBJS/LIB_PLRALIAS against its own cwd, lib/, so
// an archived tree has them beside etc/ rather than inside it) —
// --from-objs-dir/--from-alias-dir override either guess.
func importPfile(o importOptions) error {
	fromFormat := o.fromFormat
	if fromFormat == "" {
		fromFormat = binary.FormatName
	}
	enc, ok := convert.Encodings[o.encName]
	if !ok {
		return fmt.Errorf("unknown encoding %q (have: %v)", o.encName, encodingNames())
	}
	fromDir, err := resolveDir(typePfile, withDefaultBase(o.fromDir), fromFormat)
	if err != nil {
		return err
	}
	toDir, err := resolveDir(typePfile, withDefaultBase(o.toDir), "yaml")
	if err != nil {
		return err
	}

	src, err := player.Open(fromFormat, player.Config{Dir: fromDir, ReadOnly: true})
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()

	objsDir, objsNote := resolveSubdir(o.fromObjsDir, fromDir, "plrobjs")
	objSrc, err := binary.NewObjectStore(player.Config{
		Dir: fromDir, ObjectsDir: objsDir, ReadOnly: true,
	})
	if err != nil {
		return err
	}

	aliasDir, aliasNote := resolveSubdir(o.fromAliasDir, fromDir, "plralias")
	aliasSrc, err := binary.NewAliasStore(player.Config{
		Dir: fromDir, AliasDir: aliasDir, ReadOnly: true,
	})
	if err != nil {
		return err
	}

	dst, err := playeryaml.New(player.Config{Dir: toDir})
	if err != nil {
		return err
	}
	defer func() { _ = dst.Close() }()

	out := bufio.NewWriter(os.Stdout)
	defer func() { _ = out.Flush() }()
	for _, note := range []string{objsNote, aliasNote} {
		if note != "" {
			_, _ = fmt.Fprintln(out, note)
		}
	}

	ctx := context.Background()
	characters, withObjects, withAliases, transcoded := 0, 0, 0, 0
	for entry, err := range src.List(ctx) {
		if err != nil {
			_, _ = fmt.Fprintf(out, "listing: %v\n", err)
			continue
		}
		rec, err := src.Load(ctx, entry.Name)
		if err != nil {
			_, _ = fmt.Fprintf(out, "%s: %v\n", entry.Name, err)
			continue
		}
		// Aliases live in their own file per character, so they are folded
		// into the record before it is saved rather than arriving with it.
		// A character with none has no file at all (write_aliases removes
		// it), which is the ordinary case and not a failure.
		switch as, aerr := aliasSrc.LoadAliases(entry.Name); {
		case aerr == nil:
			rec.Aliases = as
			withAliases++
		case errors.Is(aerr, player.ErrNotFound):
		default:
			_, _ = fmt.Fprintf(out, "%s: aliases: %v\n", entry.Name, aerr)
		}

		transcoded += transcodePlayerStrings(rec, enc)
		if err := dst.Save(ctx, rec); err != nil {
			_, _ = fmt.Fprintf(out, "%s: writing: %v\n", entry.Name, err)
			continue
		}
		characters++

		f, err := objSrc.LoadObjects(ctx, entry.Name)
		switch {
		case errors.Is(err, player.ErrNotFound):
			// No rent/crash file: every character who has never left the
			// game carrying anything.
		case err != nil:
			_, _ = fmt.Fprintf(out, "%s: reading rent file: %v\n", entry.Name, err)
		default:
			if err := dst.SaveObjects(ctx, entry.Name, f); err != nil {
				_, _ = fmt.Fprintf(out, "%s: writing rent file: %v\n", entry.Name, err)
				continue
			}
			withObjects++
		}
	}

	_, _ = fmt.Fprintf(out, "\nimported %d character(s), %d with a rent/crash file, %d with aliases\n",
		characters, withObjects, withAliases)
	if transcoded > 0 {
		_, _ = fmt.Fprintf(out, "transcoded %d string(s) from %s to UTF-8\n", transcoded, o.encName)
	}
	return out.Flush()
}

// resolveSubdir finds a per-character subdirectory (plrobjs/, plralias/)
// that goes with a roster directory, and says which one it picked.
//
// Two layouts are both real and neither is wrong. This port keeps a
// roster and the rent files that belong to it in one directory, so
// plrobjs/ is a child of fromDir. The C keeps `etc/players` and
// `plrobjs/` as siblings under lib/, because it builds both paths from
// its own cwd (db.h's PLAYER_FILE and LIB_PLROBJS) — so an archived tree
// pointed at with `--from-dir=lib/etc` has its rent files one level up.
//
// Guessing between them beats the alternative, which is what this used to
// do: look only in the first place, find nothing, and report "0 with a
// rent/crash file" — a sentence that reads like a fact about the roster
// and was actually a fact about the path. A character with no rent file
// is completely ordinary, so there was nothing here to look wrong.
//
// explicit (--from-objs-dir / --from-alias-dir) overrides, for a layout
// that is neither.
func resolveSubdir(explicit, fromDir, name string) (dir string, note string) {
	if explicit != "" {
		return explicit, ""
	}
	own := filepath.Join(fromDir, name)
	if isDir(own) {
		return own, ""
	}
	sibling := filepath.Join(filepath.Dir(filepath.Clean(fromDir)), name)
	if isDir(sibling) {
		return sibling, fmt.Sprintf("%s: reading %s (the C's lib/ layout, beside %s rather than inside it)",
			name, sibling, fromDir)
	}
	// Neither exists. Keep the port's own layout, so the message a caller
	// gets names the place they most likely meant.
	return own, ""
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// importNames converts misc/xnames into config/names.yaml, step 6b of
// docs/design/data-format.md §9.
func importNames(o importOptions) error {
	if o.fromFormat != "" && o.fromFormat != "classic" {
		return fmt.Errorf("import --type=names only reads a classic source (got --from-format=%q)", o.fromFormat)
	}
	enc, ok := convert.Encodings[o.encName]
	if !ok {
		return fmt.Errorf("unknown encoding %q (have: %v)", o.encName, encodingNames())
	}
	fromPath, err := resolveDir(typeNames, withDefaultBase(o.fromDir), "classic")
	if err != nil {
		return err
	}
	toDir, err := resolveDir(typeNames, withDefaultBase(o.toDir), "yaml")
	if err != nil {
		return err
	}

	list, err := names.Load("classic", fromPath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", fromPath, err)
	}
	transcoded := 0
	for i := range list {
		if transcodeString(&list[i], enc) {
			transcoded++
		}
	}
	if err := names.Save("yaml", toDir, list); err != nil {
		return fmt.Errorf("writing %s: %w", filepath.Join(toDir, names.YamlFile), err)
	}

	out := bufio.NewWriter(os.Stdout)
	_, _ = fmt.Fprintf(out, "names: imported %d\n", len(list))
	if transcoded > 0 {
		_, _ = fmt.Fprintf(out, "transcoded %d string(s) from %s to UTF-8\n", transcoded, o.encName)
	}
	return out.Flush()
}

// importMessages converts misc/messages into config/messages.yaml, step
// 6c of docs/design/data-format.md §9.
func importMessages(o importOptions) error {
	if o.fromFormat != "" && o.fromFormat != "classic" {
		return fmt.Errorf("import --type=messages only reads a classic source (got --from-format=%q)", o.fromFormat)
	}
	enc, ok := convert.Encodings[o.encName]
	if !ok {
		return fmt.Errorf("unknown encoding %q (have: %v)", o.encName, encodingNames())
	}
	fromPath, err := resolveDir(typeMessages, withDefaultBase(o.fromDir), "classic")
	if err != nil {
		return err
	}
	toDir, err := resolveDir(typeMessages, withDefaultBase(o.toDir), "yaml")
	if err != nil {
		return err
	}

	records, err := messages.Load("classic", fromPath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", fromPath, err)
	}
	transcoded := transcodeFightMessages(records, enc)
	if err := messages.Save("yaml", toDir, records); err != nil {
		return fmt.Errorf("writing %s: %w", filepath.Join(toDir, messages.YamlFile), err)
	}

	out := bufio.NewWriter(os.Stdout)
	_, _ = fmt.Fprintf(out, "messages: imported %d\n", len(records))
	if transcoded > 0 {
		_, _ = fmt.Fprintf(out, "transcoded %d string(s) from %s to UTF-8\n", transcoded, o.encName)
	}
	return out.Flush()
}

// importSocials converts misc/socials into config/socials.yaml, step 6c
// of docs/design/data-format.md §7.
func importSocials(o importOptions) error {
	if o.fromFormat != "" && o.fromFormat != "classic" {
		return fmt.Errorf("import --type=socials only reads a classic source (got --from-format=%q)", o.fromFormat)
	}
	enc, ok := convert.Encodings[o.encName]
	if !ok {
		return fmt.Errorf("unknown encoding %q (have: %v)", o.encName, encodingNames())
	}
	fromPath, err := resolveDir(typeSocials, withDefaultBase(o.fromDir), "classic")
	if err != nil {
		return err
	}
	toDir, err := resolveDir(typeSocials, withDefaultBase(o.toDir), "yaml")
	if err != nil {
		return err
	}

	list, err := socials.Load("classic", fromPath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", fromPath, err)
	}
	transcoded := transcodeSocials(list, enc)
	if err := socials.Save("yaml", toDir, list); err != nil {
		return fmt.Errorf("writing %s: %w", filepath.Join(toDir, socials.YamlFile), err)
	}

	out := bufio.NewWriter(os.Stdout)
	_, _ = fmt.Fprintf(out, "socials: imported %d\n", len(list))
	if transcoded > 0 {
		_, _ = fmt.Fprintf(out, "transcoded %d string(s) from %s to UTF-8\n", transcoded, o.encName)
	}
	return out.Flush()
}

// importHelp converts text/help/index plus the .hlp files it lists into
// text/help/help.yaml plus one .txt file per entry, step 6c of
// docs/design/data-format.md §7. Unlike the other six, --to-dir defaults
// to the same directory as --from-dir (both resolve to <base>/text/help):
// classic and yaml share text/help/ itself, distinguished by which files
// are present rather than by directory, so converting in place leaves
// index/*.hlp inert beside the new files rather than requiring a separate
// tree.
func importHelp(o importOptions) error {
	if o.fromFormat != "" && o.fromFormat != "classic" {
		return fmt.Errorf("import --type=help only reads a classic source (got --from-format=%q)", o.fromFormat)
	}
	enc, ok := convert.Encodings[o.encName]
	if !ok {
		return fmt.Errorf("unknown encoding %q (have: %v)", o.encName, encodingNames())
	}
	fromDir, err := resolveDir(typeHelp, withDefaultBase(o.fromDir), "classic")
	if err != nil {
		return err
	}
	toDir, err := resolveDir(typeHelp, withDefaultBase(o.toDir), "yaml")
	if err != nil {
		return err
	}

	entries, err := help.Load("classic", fromDir)
	if err != nil {
		return fmt.Errorf("reading %s: %w", fromDir, err)
	}
	transcoded := 0
	for i := range entries {
		if transcodeString(&entries[i].Body, enc) {
			transcoded++
		}
		// The keywords too. They used to be skipped, for the same
		// reason and with the same result as the world importer's own
		// keyword lists: a keyword left in CP1252 is not valid UTF-8, so
		// the yaml encoder substitutes U+FFFD for the offending byte and
		// the entry becomes unreachable by the word it was filed under.
		// They are also what game.HelpSlug names the entry's own .txt
		// file from, so a mangled keyword mangles a filename too.
		for j := range entries[i].Keywords {
			if transcodeString(&entries[i].Keywords[j], enc) {
				transcoded++
			}
		}
	}
	if err := help.Save("yaml", toDir, entries); err != nil {
		return fmt.Errorf("writing %s: %w", filepath.Join(toDir, help.YamlFile), err)
	}

	// text/help/screen is HELP_PAGE_FILE (db.h:78) -- what bare `help`
	// prints -- and it is not a help *entry*, so nothing above carries it.
	// That is right when converting in place, where it simply stays put,
	// and wrong when --to-dir is a different tree: a whole-lib `import`
	// gives every step its own destination, so a converted directory came
	// out with no screen in it at all, and bare `help` on a yaml server
	// printed the command list instead of the help screen. Copied rather
	// than converted, since internal/server/text.go reads it as plain
	// prose from the same path under either format.
	copied, err := copyHelpScreen(fromDir, toDir)
	if err != nil {
		return err
	}

	out := bufio.NewWriter(os.Stdout)
	_, _ = fmt.Fprintf(out, "help: imported %d\n", len(entries))
	if transcoded > 0 {
		_, _ = fmt.Fprintf(out, "transcoded %d help entries from %s to UTF-8\n", transcoded, o.encName)
	}
	if copied {
		_, _ = fmt.Fprintf(out, "copied %s unchanged (not a help entry)\n", helpScreenName)
	}
	return out.Flush()
}
