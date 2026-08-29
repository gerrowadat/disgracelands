// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

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

// importState converts bans, boards, mail, houses, the clock and the
// bug/idea/typo reports into yaml together, per step 6a/6b of
// docs/design/data-format.md §9 — one command, since they end up in one
// directory and there is no reason to convert boards without mail.
//
// The three classic-side source directories (etc/, house/, misc/) default
// from o.fromDir via stateClassicDirs; o.fromHouseDir/o.fromMiscDir
// override them for an archive that keeps house/misc somewhere else.
func importState(o importOptions) error {
	if o.fromFormat != "" && o.fromFormat != "classic" {
		return fmt.Errorf("import --type=state only reads a classic source (got --from-format=%q)", o.fromFormat)
	}
	enc, ok := convert.Encodings[o.encName]
	if !ok {
		return fmt.Errorf("unknown encoding %q (have: %v)", o.encName, encodingNames())
	}

	etcDir, houseDir, miscDir := stateClassicDirs(withDefaultBase(o.fromDir))
	if o.fromHouseDir != "" {
		houseDir = o.fromHouseDir
	}
	if o.fromMiscDir != "" {
		miscDir = o.fromMiscDir
	}
	toDir, err := resolveDir(typeState, withDefaultBase(o.toDir), "yaml")
	if err != nil {
		return err
	}

	out := bufio.NewWriter(os.Stdout)
	defer func() { _ = out.Flush() }()

	if err := importBans(etcDir, toDir, out); err != nil {
		return fmt.Errorf("bans: %w", err)
	}
	if err := importBoards(etcDir, toDir, enc, o.encName, out); err != nil {
		return fmt.Errorf("boards: %w", err)
	}
	if err := importMail(etcDir, toDir, enc, o.encName, out); err != nil {
		return fmt.Errorf("mail: %w", err)
	}
	if err := importHouses(etcDir, houseDir, toDir, out); err != nil {
		return fmt.Errorf("houses: %w", err)
	}
	if err := importReports(miscDir, toDir, enc, o.encName, out); err != nil {
		return fmt.Errorf("reports: %w", err)
	}
	if err := importClock(etcDir, toDir, out); err != nil {
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

	// Oldest first, because Add prepends. bans.Store.List is documented
	// "newest first" and both implementations honour it by pushing onto
	// the front of the list the way ban.c's own linked list does — so
	// replaying a newest-first list through Add in the order it was
	// handed over builds the reverse of it, and the converted ban list
	// came out backwards. Nothing in the game reads the list by position,
	// but `ban` prints it in list order, so it is visible, and it is the
	// kind of difference that is obvious the moment something compares
	// the two and invisible until then.
	list := src.List()
	n := 0
	for i := len(list) - 1; i >= 0; i-- {
		if _, err := dst.Add(list[i]); err != nil {
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
	declared := make(map[int32]bool, len(list))
	var duplicates []int32
	for _, h := range list {
		// A second record for a room already seen is one the conversion
		// collapses — and one the C would never have booted either, since
		// House_boot skips a vnum that is already a house (house.c:265).
		// Counting its objects again would report contents that were
		// written once as if they had been written twice.
		if declared[h.Vnum] {
			duplicates = append(duplicates, h.Vnum)
			continue
		}
		declared[h.Vnum] = true
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

	// The count reported is the store's own, read back, rather than the
	// length of what was handed to it. Those are different numbers exactly
	// when the conversion dropped something, which is the moment a summary
	// line most needs to be true — before #240 this printed len(list) and
	// so said "imported 4 house(s)" for three that arrived.
	stored, err := dst.Load()
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "houses: imported %d house(s), %d object(s)\n", len(stored), objects)
	if len(duplicates) > 0 {
		_, _ = fmt.Fprintf(out, "houses: collapsed %d duplicate control record(s) — "+
			"state/houses.yaml keys a house by its room and the C boots the first record "+
			"for a room and skips the rest (house.c:265): %s\n",
			len(duplicates), joinVnums(duplicates))
	}

	return reportOrphanedHouseContents(src, declared, out)
}

// joinVnums renders a vnum list for a report line: "#5007, #5008".
func joinVnums(vnums []int32) string {
	parts := make([]string, len(vnums))
	for i, v := range vnums {
		parts[i] = fmt.Sprintf("#%d", v)
	}
	return strings.Join(parts, ", ")
}

// reportOrphanedHouseContents names every `<vnum>.house` file whose room
// has no control record, because the conversion drops it.
//
// The C never deletes one: House_save_control writes the control array and
// nothing else, so a house destroyed by `hcontrol destroy` — or dropped by
// the boot checks in internal/server/houses.go — leaves its contents file
// behind forever. Seven years of that and an archive has several. yaml has
// nowhere to put them: state/houses.yaml nests a house's contents inside
// its own control entry (docs/design/data-format.md §9), so contents
// belonging to no house have no entry to sit in.
//
// Dropping them is right — no server ever reads one, since every reader
// starts from the control array — but doing it in silence is not. This is
// the conversion boundary where a value stops existing, and
// docs/proposals/yaml-only.md §6 rule 2 says an importer names what it
// could not carry across. It went unnamed until #239, and `verify
// --against` could not see it either, because both sides of that
// comparison enumerated houses from the control records too.
func reportOrphanedHouseContents(src houses.Store, declared map[int32]bool, out *bufio.Writer) error {
	withObjects, err := src.ObjectVnums()
	if err != nil {
		return err
	}
	var orphans []string
	for _, vnum := range withObjects {
		if declared[vnum] {
			continue
		}
		objs, err := src.LoadObjects(vnum)
		if err != nil {
			return fmt.Errorf("house #%d: %w", vnum, err)
		}
		orphans = append(orphans, fmt.Sprintf("  #%d: %d object(s)", vnum, len(objs)))
	}
	if len(orphans) == 0 {
		return nil
	}
	_, _ = fmt.Fprintf(out, "houses: dropped %d contents file(s) belonging to no house — "+
		"the C leaves these behind when a house is destroyed and yaml has nowhere to keep "+
		"them (data-format.md §9):\n", len(orphans))
	for _, o := range orphans {
		_, _ = fmt.Fprintln(out, o)
	}
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

// fmtState canonicalises a yaml state directory in place: load and
// immediately re-save bans, boards, mail, houses, reports and the clock.
func fmtState(dir string) error {
	out := bufio.NewWriter(os.Stdout)
	defer func() { _ = out.Flush() }()

	banStore, err := bansyaml.New(bans.Config{Path: dir})
	if err != nil {
		return fmt.Errorf("bans: %w", err)
	}
	if err := banStore.Rewrite(); err != nil {
		return fmt.Errorf("bans: %w", err)
	}

	boardStore, err := boardsyaml.New(boards.Config{Dir: dir})
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

	mailStore, err := mailyaml.New(mail.Config{Path: dir})
	if err != nil {
		return fmt.Errorf("mail: %w", err)
	}
	if err := mailStore.Rewrite(); err != nil {
		return fmt.Errorf("mail: %w", err)
	}

	houseStore, err := housesyaml.New(houses.Config{ObjectDir: dir})
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

	reportStore, err := reportsyaml.New(reports.Config{Dir: dir})
	if err != nil {
		return fmt.Errorf("reports: %w", err)
	}
	if err := reportStore.Rewrite(); err != nil {
		return fmt.Errorf("reports: %w", err)
	}

	epoch, err := clock.Load("yaml", dir)
	if err != nil {
		return fmt.Errorf("clock: %w", err)
	}
	if err := clock.Save("yaml", dir, epoch); err != nil {
		return fmt.Errorf("clock: %w", err)
	}

	_, _ = fmt.Fprintln(out, "formatted bans, boards, mail, houses, reports and the clock")
	return out.Flush()
}
