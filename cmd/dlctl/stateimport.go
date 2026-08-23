// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"

	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/persist/bans"
	bansclassic "github.com/gerrowadat/disgracelands/internal/persist/bans/classic"
	bansyaml "github.com/gerrowadat/disgracelands/internal/persist/bans/yaml"
	"github.com/gerrowadat/disgracelands/internal/persist/boards"
	boardsclassic "github.com/gerrowadat/disgracelands/internal/persist/boards/classic"
	boardsyaml "github.com/gerrowadat/disgracelands/internal/persist/boards/yaml"
	"github.com/gerrowadat/disgracelands/internal/persist/clock"
	"github.com/gerrowadat/disgracelands/internal/persist/convert"
	"github.com/gerrowadat/disgracelands/internal/persist/houses"
	housesclassic "github.com/gerrowadat/disgracelands/internal/persist/houses/classic"
	housesyaml "github.com/gerrowadat/disgracelands/internal/persist/houses/yaml"
	"github.com/gerrowadat/disgracelands/internal/persist/mail"
	mailclassic "github.com/gerrowadat/disgracelands/internal/persist/mail/classic"
	mailyaml "github.com/gerrowadat/disgracelands/internal/persist/mail/yaml"
	"github.com/gerrowadat/disgracelands/internal/persist/reports"
	reportsclassic "github.com/gerrowadat/disgracelands/internal/persist/reports/classic"
	reportsyaml "github.com/gerrowadat/disgracelands/internal/persist/reports/yaml"
)

// transcodeString converts s from enc to UTF-8, leaving it alone if it is
// empty or already valid UTF-8 — shared by importBoards/importMail/
// importReports below, the three state subsystems that carry free text.
// bans (a hostname substring and an admin's name) and houses (numeric
// fields and a StoredObject identified only by vnum, never by name or
// description) have nothing to transcode, so neither calls this.
func transcodeString(s *string, enc *charmap.Charmap) bool {
	if *s == "" || utf8.ValidString(*s) {
		return false
	}
	if out, err := enc.NewDecoder().String(*s); err == nil {
		*s = out
		return true
	}
	return false
}

// cmdStateImport converts bans, boards, mail, houses, the clock and the
// bug/idea/typo reports into yaml together, per step 6a/6b of
// docs/design/data-format.md §9 — one command, since they end up in one
// directory and there is no reason to convert boards without mail.
func cmdStateImport(args []string) error {
	fs := flag.NewFlagSet("state import", flag.ContinueOnError)
	fromDir := fs.String("from-dir", "data/etc", "Source (classic) directory for bans, boards, mail, the house control file and the clock")
	fromHouseDir := fs.String("from-house-dir", "data/house", "Source (classic) directory for the per-room house object files")
	fromMiscDir := fs.String("from-misc-dir", "data/misc", "Source (classic) directory for the bug/idea/typo report files")
	toDir := fs.String("to-dir", "data/state", "Destination (yaml) directory")
	encName := fs.String("encoding", convert.DefaultEncoding,
		fmt.Sprintf("Source text encoding: %v", encodingNames()))
	if err := fs.Parse(args); err != nil {
		return err
	}

	enc, ok := convert.Encodings[*encName]
	if !ok {
		return fmt.Errorf("unknown encoding %q (have: %v)", *encName, encodingNames())
	}

	out := bufio.NewWriter(os.Stdout)
	defer func() { _ = out.Flush() }()

	if err := importBans(*fromDir, *toDir, out); err != nil {
		return fmt.Errorf("bans: %w", err)
	}
	if err := importBoards(*fromDir, *toDir, enc, *encName, out); err != nil {
		return fmt.Errorf("boards: %w", err)
	}
	if err := importMail(*fromDir, *toDir, enc, *encName, out); err != nil {
		return fmt.Errorf("mail: %w", err)
	}
	if err := importHouses(*fromDir, *fromHouseDir, *toDir, out); err != nil {
		return fmt.Errorf("houses: %w", err)
	}
	if err := importReports(*fromMiscDir, *toDir, enc, *encName, out); err != nil {
		return fmt.Errorf("reports: %w", err)
	}
	if err := importClock(*fromDir, *toDir, out); err != nil {
		return fmt.Errorf("clock: %w", err)
	}
	return out.Flush()
}

func importBans(fromDir, toDir string, out *bufio.Writer) error {
	src, err := bansclassic.New(bans.Config{Path: filepath.Join(fromDir, "badsites"), ReadOnly: true})
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()
	dst, err := bansyaml.New(bans.Config{Path: toDir})
	if err != nil {
		return err
	}
	defer func() { _ = dst.Close() }()

	n := 0
	for _, ban := range src.List() {
		if _, err := dst.Add(ban); err != nil {
			return err
		}
		n++
	}
	_, _ = fmt.Fprintf(out, "bans: imported %d\n", n)
	return nil
}

func importBoards(fromDir, toDir string, enc *charmap.Charmap, encName string, out *bufio.Writer) error {
	src, err := boardsclassic.New(boards.Config{Dir: fromDir, ReadOnly: true})
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()
	dst, err := boardsyaml.New(boards.Config{Dir: toDir})
	if err != nil {
		return err
	}
	defer func() { _ = dst.Close() }()

	boardsImported, messages, transcoded := 0, 0, 0
	for _, def := range game.Boards {
		msgs, err := src.Load(def.File)
		if err != nil {
			if err == boards.ErrNotFound { //nolint:errorlint // sentinel from this exact call, not wrapped
				continue
			}
			return fmt.Errorf("%s: %w", def.File, err)
		}
		for i := range msgs {
			// Heading is the whole formatted line ("Aug 20 2026 (Zod)  ::
			// headline"), not just the poster's own typed headline after
			// "::" — transcoding the whole string is still correct, since
			// the date/name half is always ASCII and decodes to itself.
			if transcodeString(&msgs[i].Heading, enc) {
				transcoded++
			}
			if transcodeString(&msgs[i].Body, enc) {
				transcoded++
			}
		}
		if err := dst.Save(def.File, msgs); err != nil {
			return fmt.Errorf("%s: %w", def.File, err)
		}
		boardsImported++
		messages += len(msgs)
	}
	_, _ = fmt.Fprintf(out, "boards: imported %d board(s), %d message(s)\n", boardsImported, messages)
	if transcoded > 0 {
		_, _ = fmt.Fprintf(out, "boards: transcoded %d string(s) from %s to UTF-8\n", transcoded, encName)
	}
	return nil
}

func importMail(fromDir, toDir string, enc *charmap.Charmap, encName string, out *bufio.Writer) error {
	src, err := mailclassic.New(mail.Config{Path: filepath.Join(fromDir, "plrmail"), ReadOnly: true})
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()
	dst, err := mailyaml.New(mail.Config{Path: toDir})
	if err != nil {
		return err
	}
	defer func() { _ = dst.Close() }()

	n, transcoded := 0, 0
	for _, m := range src.All() {
		if transcodeString(&m.Text, enc) {
			transcoded++
		}
		if err := dst.Send(m); err != nil {
			return err
		}
		n++
	}
	_, _ = fmt.Fprintf(out, "mail: imported %d message(s)\n", n)
	if transcoded > 0 {
		_, _ = fmt.Fprintf(out, "mail: transcoded %d message(s) from %s to UTF-8\n", transcoded, encName)
	}
	return nil
}

func importHouses(fromDir, fromHouseDir, toDir string, out *bufio.Writer) error {
	src, err := housesclassic.New(houses.Config{
		ControlPath: filepath.Join(fromDir, "hcontrol"), ObjectDir: fromHouseDir, ReadOnly: true,
	})
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()
	dst, err := housesyaml.New(houses.Config{ObjectDir: toDir})
	if err != nil {
		return err
	}
	defer func() { _ = dst.Close() }()

	list, err := src.Load()
	if err != nil {
		return err
	}
	if err := dst.Save(list); err != nil {
		return err
	}
	objects := 0
	for _, h := range list {
		objs, err := src.LoadObjects(h.Vnum)
		if err != nil {
			return fmt.Errorf("house #%d: %w", h.Vnum, err)
		}
		if len(objs) == 0 {
			continue
		}
		if err := dst.SaveObjects(h.Vnum, objs); err != nil {
			return fmt.Errorf("house #%d: %w", h.Vnum, err)
		}
		objects += len(objs)
	}
	_, _ = fmt.Fprintf(out, "houses: imported %d house(s), %d object(s)\n", len(list), objects)
	return nil
}

func importReports(fromMiscDir, toDir string, enc *charmap.Charmap, encName string, out *bufio.Writer) error {
	src, err := reportsclassic.New(reports.Config{Dir: fromMiscDir, ReadOnly: true})
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()
	dst, err := reportsyaml.New(reports.Config{Dir: toDir})
	if err != nil {
		return err
	}
	defer func() { _ = dst.Close() }()

	all, err := src.All()
	if err != nil {
		return err
	}
	transcoded := 0
	for _, r := range all {
		if transcodeString(&r.Body, enc) {
			transcoded++
		}
		if _, err := dst.Append(r); err != nil {
			return err
		}
	}
	_, _ = fmt.Fprintf(out, "reports: imported %d\n", len(all))
	if transcoded > 0 {
		_, _ = fmt.Fprintf(out, "reports: transcoded %d from %s to UTF-8\n", transcoded, encName)
	}
	return nil
}

func importClock(fromDir, toDir string, out *bufio.Writer) error {
	epoch, err := clock.Load("classic", filepath.Join(fromDir, "time"))
	if err != nil {
		return err
	}
	if err := clock.Save("yaml", toDir, epoch); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "clock: imported epoch %s\n", epoch.Format(time.RFC3339))
	return nil
}

// cmdStateFmt canonicalises a yaml state directory in place: load and
// immediately re-save bans, boards, mail and houses.
func cmdStateFmt(args []string) error {
	fs := flag.NewFlagSet("state fmt", flag.ContinueOnError)
	dir := fs.String("state-dir", "data/state", "Yaml state directory")
	if err := fs.Parse(args); err != nil {
		return err
	}

	out := bufio.NewWriter(os.Stdout)
	defer func() { _ = out.Flush() }()

	banStore, err := bansyaml.New(bans.Config{Path: *dir})
	if err != nil {
		return fmt.Errorf("bans: %w", err)
	}
	if err := banStore.Rewrite(); err != nil {
		return fmt.Errorf("bans: %w", err)
	}

	boardStore, err := boardsyaml.New(boards.Config{Dir: *dir})
	if err != nil {
		return fmt.Errorf("boards: %w", err)
	}
	for _, def := range game.Boards {
		msgs, err := boardStore.Load(def.File)
		if err != nil {
			if err == boards.ErrNotFound { //nolint:errorlint // sentinel from this exact call, not wrapped
				continue
			}
			return fmt.Errorf("boards: %s: %w", def.File, err)
		}
		if err := boardStore.Save(def.File, msgs); err != nil {
			return fmt.Errorf("boards: %s: %w", def.File, err)
		}
	}

	mailStore, err := mailyaml.New(mail.Config{Path: *dir})
	if err != nil {
		return fmt.Errorf("mail: %w", err)
	}
	if err := mailStore.Rewrite(); err != nil {
		return fmt.Errorf("mail: %w", err)
	}

	houseStore, err := housesyaml.New(houses.Config{ObjectDir: *dir})
	if err != nil {
		return fmt.Errorf("houses: %w", err)
	}
	list, err := houseStore.Load()
	if err != nil {
		return fmt.Errorf("houses: %w", err)
	}
	if err := houseStore.Save(list); err != nil {
		return fmt.Errorf("houses: %w", err)
	}

	reportStore, err := reportsyaml.New(reports.Config{Dir: *dir})
	if err != nil {
		return fmt.Errorf("reports: %w", err)
	}
	if err := reportStore.Rewrite(); err != nil {
		return fmt.Errorf("reports: %w", err)
	}

	epoch, err := clock.Load("yaml", *dir)
	if err != nil {
		return fmt.Errorf("clock: %w", err)
	}
	if err := clock.Save("yaml", *dir, epoch); err != nil {
		return fmt.Errorf("clock: %w", err)
	}

	_, _ = fmt.Fprintln(out, "formatted bans, boards, mail, houses, reports and the clock")
	return out.Flush()
}
