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

	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/persist/bans"
	bansclassic "github.com/gerrowadat/disgracelands/internal/persist/bans/classic"
	bansnative "github.com/gerrowadat/disgracelands/internal/persist/bans/native"
	"github.com/gerrowadat/disgracelands/internal/persist/boards"
	boardsclassic "github.com/gerrowadat/disgracelands/internal/persist/boards/classic"
	boardsnative "github.com/gerrowadat/disgracelands/internal/persist/boards/native"
	"github.com/gerrowadat/disgracelands/internal/persist/clock"
	"github.com/gerrowadat/disgracelands/internal/persist/houses"
	housesclassic "github.com/gerrowadat/disgracelands/internal/persist/houses/classic"
	housesnative "github.com/gerrowadat/disgracelands/internal/persist/houses/native"
	"github.com/gerrowadat/disgracelands/internal/persist/mail"
	mailclassic "github.com/gerrowadat/disgracelands/internal/persist/mail/classic"
	mailnative "github.com/gerrowadat/disgracelands/internal/persist/mail/native"
	"github.com/gerrowadat/disgracelands/internal/persist/reports"
	reportsclassic "github.com/gerrowadat/disgracelands/internal/persist/reports/classic"
	reportsnative "github.com/gerrowadat/disgracelands/internal/persist/reports/native"
)

// cmdStateImport converts bans, boards, mail, houses, the clock and the
// bug/idea/typo reports into native together, per step 6a/6b of
// docs/proposals/data-format.md §9 — one command, since they end up in one
// directory and there is no reason to convert boards without mail.
func cmdStateImport(args []string) error {
	fs := flag.NewFlagSet("state import", flag.ContinueOnError)
	fromDir := fs.String("from-dir", "data/etc", "Source (classic) directory for bans, boards, mail, the house control file and the clock")
	fromHouseDir := fs.String("from-house-dir", "data/house", "Source (classic) directory for the per-room house object files")
	fromMiscDir := fs.String("from-misc-dir", "data/misc", "Source (classic) directory for the bug/idea/typo report files")
	toDir := fs.String("to-dir", "data/state", "Destination (native) directory")
	if err := fs.Parse(args); err != nil {
		return err
	}

	out := bufio.NewWriter(os.Stdout)
	defer func() { _ = out.Flush() }()

	if err := importBans(*fromDir, *toDir, out); err != nil {
		return fmt.Errorf("bans: %w", err)
	}
	if err := importBoards(*fromDir, *toDir, out); err != nil {
		return fmt.Errorf("boards: %w", err)
	}
	if err := importMail(*fromDir, *toDir, out); err != nil {
		return fmt.Errorf("mail: %w", err)
	}
	if err := importHouses(*fromDir, *fromHouseDir, *toDir, out); err != nil {
		return fmt.Errorf("houses: %w", err)
	}
	if err := importReports(*fromMiscDir, *toDir, out); err != nil {
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
	dst, err := bansnative.New(bans.Config{Path: toDir})
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

func importBoards(fromDir, toDir string, out *bufio.Writer) error {
	src, err := boardsclassic.New(boards.Config{Dir: fromDir, ReadOnly: true})
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()
	dst, err := boardsnative.New(boards.Config{Dir: toDir})
	if err != nil {
		return err
	}
	defer func() { _ = dst.Close() }()

	boardsImported, messages := 0, 0
	for _, def := range game.Boards {
		msgs, err := src.Load(def.File)
		if err != nil {
			if err == boards.ErrNotFound { //nolint:errorlint // sentinel from this exact call, not wrapped
				continue
			}
			return fmt.Errorf("%s: %w", def.File, err)
		}
		if err := dst.Save(def.File, msgs); err != nil {
			return fmt.Errorf("%s: %w", def.File, err)
		}
		boardsImported++
		messages += len(msgs)
	}
	_, _ = fmt.Fprintf(out, "boards: imported %d board(s), %d message(s)\n", boardsImported, messages)
	return nil
}

func importMail(fromDir, toDir string, out *bufio.Writer) error {
	src, err := mailclassic.New(mail.Config{Path: filepath.Join(fromDir, "plrmail"), ReadOnly: true})
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()
	dst, err := mailnative.New(mail.Config{Path: toDir})
	if err != nil {
		return err
	}
	defer func() { _ = dst.Close() }()

	n := 0
	for _, m := range src.All() {
		if err := dst.Send(m); err != nil {
			return err
		}
		n++
	}
	_, _ = fmt.Fprintf(out, "mail: imported %d message(s)\n", n)
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
	dst, err := housesnative.New(houses.Config{ObjectDir: toDir})
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

func importReports(fromMiscDir, toDir string, out *bufio.Writer) error {
	src, err := reportsclassic.New(reports.Config{Dir: fromMiscDir, ReadOnly: true})
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()
	dst, err := reportsnative.New(reports.Config{Dir: toDir})
	if err != nil {
		return err
	}
	defer func() { _ = dst.Close() }()

	all, err := src.All()
	if err != nil {
		return err
	}
	for _, r := range all {
		if _, err := dst.Append(r); err != nil {
			return err
		}
	}
	_, _ = fmt.Fprintf(out, "reports: imported %d\n", len(all))
	return nil
}

func importClock(fromDir, toDir string, out *bufio.Writer) error {
	epoch, err := clock.Load("classic", filepath.Join(fromDir, "time"))
	if err != nil {
		return err
	}
	if err := clock.Save("native", toDir, epoch); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "clock: imported epoch %s\n", epoch.Format(time.RFC3339))
	return nil
}

// cmdStateFmt canonicalises a native state directory in place: load and
// immediately re-save bans, boards, mail and houses.
func cmdStateFmt(args []string) error {
	fs := flag.NewFlagSet("state fmt", flag.ContinueOnError)
	dir := fs.String("state-dir", "data/state", "Native state directory")
	if err := fs.Parse(args); err != nil {
		return err
	}

	out := bufio.NewWriter(os.Stdout)
	defer func() { _ = out.Flush() }()

	banStore, err := bansnative.New(bans.Config{Path: *dir})
	if err != nil {
		return fmt.Errorf("bans: %w", err)
	}
	if err := banStore.Rewrite(); err != nil {
		return fmt.Errorf("bans: %w", err)
	}

	boardStore, err := boardsnative.New(boards.Config{Dir: *dir})
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

	mailStore, err := mailnative.New(mail.Config{Path: *dir})
	if err != nil {
		return fmt.Errorf("mail: %w", err)
	}
	if err := mailStore.Rewrite(); err != nil {
		return fmt.Errorf("mail: %w", err)
	}

	houseStore, err := housesnative.New(houses.Config{ObjectDir: *dir})
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

	reportStore, err := reportsnative.New(reports.Config{Dir: *dir})
	if err != nil {
		return fmt.Errorf("reports: %w", err)
	}
	if err := reportStore.Rewrite(); err != nil {
		return fmt.Errorf("reports: %w", err)
	}

	epoch, err := clock.Load("native", *dir)
	if err != nil {
		return fmt.Errorf("clock: %w", err)
	}
	if err := clock.Save("native", *dir, epoch); err != nil {
		return fmt.Errorf("clock: %w", err)
	}

	_, _ = fmt.Fprintln(out, "formatted bans, boards, mail, houses, reports and the clock")
	return out.Flush()
}
