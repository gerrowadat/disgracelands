// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

// Package classic reads and writes misc/bugs, misc/ideas and misc/typos,
// porting do_gen_write (act.other.c:867-924).
//
// Three append-only text logs, one line per report, in a fixed sprintf
// format with no field separator beyond the format's own layout — the same
// "no struct dump, but not free-form either" shape bans' classic file has.
// Reading a line back apart is a real, if small, format-reversal exercise
// (see parseLine): a name can run to twenty characters and the C's `%-8s`
// never truncates it, so the split cannot be done by fixed column offsets.
package classic

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/persist/reports"
)

// FormatName is the name this format registers under, and the default the
// server runs on.
const FormatName = "classic"

func init() {
	reports.Register(FormatName, func(cfg reports.Config) (reports.Store, error) {
		return New(cfg)
	})
}

// fileNames are BUG_FILE/IDEA_FILE/TYPO_FILE's basenames (db.h:86-88):
// lib/misc/bugs, lib/misc/ideas, lib/misc/typos.
var fileNames = map[reports.Kind]string{
	reports.KindBug:  "bugs",
	reports.KindIdea: "ideas",
	reports.KindTypo: "typos",
}

// kindOrder fixes All()'s iteration order, so it does not depend on Go's
// randomised map order.
var kindOrder = []reports.Kind{reports.KindBug, reports.KindIdea, reports.KindTypo}

// Store is the three report files.
type Store struct {
	dir      string
	readOnly bool
	// maxFileSize is an explicit override — set only when cfg.MaxFileSize
	// was non-zero, which today is only ever the tests pinning a small size
	// to exercise the file-full refusal (act.other.c:908-911). Everywhere
	// else, Append reads game.Tuning().MaxFileSize fresh on every call, so a
	// SIGHUP reload of config/game.yaml (cmd/dlmud) takes effect on the next
	// report filed, with no plumbing back into this Store required.
	maxFileSize int64
}

// New opens the report files' directory. Missing files are not an error:
// nobody has filed one yet.
func New(cfg reports.Config) (*Store, error) {
	if cfg.Dir == "" {
		return nil, fmt.Errorf("reports: no directory configured")
	}
	return &Store{dir: cfg.Dir, readOnly: cfg.ReadOnly, maxFileSize: cfg.MaxFileSize}, nil
}

// currentMaxFileSize is the size gate Append checks: the constructor's
// explicit override if one was given, otherwise the live game-tuning value.
func (s *Store) currentMaxFileSize() int64 {
	if s.maxFileSize != 0 {
		return s.maxFileSize
	}
	return game.Tuning().MaxFileSize
}

// Name implements reports.Store.
func (s *Store) Name() string { return FormatName }

// Close implements reports.Store.
func (s *Store) Close() error { return nil }

// Append implements reports.Store (do_gen_write, act.other.c:908-921): a
// stat()-before-append size gate, then one line in the C's exact format —
// `"%-8s (%6.6s) [%5d] %s\n"`, name / a 6-character month-and-day slice of
// asctime() / the room vnum / the report text.
func (s *Store) Append(r reports.Report) (bool, error) {
	if s.readOnly {
		return false, fmt.Errorf("reports: the data directory is open read-only")
	}
	name, ok := fileNames[r.Kind]
	if !ok {
		return false, fmt.Errorf("reports: unknown kind %q", r.Kind)
	}
	path := filepath.Join(s.dir, name)

	if info, err := os.Stat(path); err == nil {
		if info.Size() >= s.currentMaxFileSize() {
			return false, nil
		}
	} else if !os.IsNotExist(err) {
		return false, fmt.Errorf("reading %s: %w", path, err)
	}

	when := r.When
	if when.IsZero() {
		when = time.Now()
	}
	line := fmt.Sprintf("%-8s (%s) [%5d] %s\n", r.Reporter, dateSlice(when), r.Room, r.Body)

	if err := os.MkdirAll(s.dir, 0o750); err != nil {
		return false, fmt.Errorf("writing %s: %w", path, err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) //nolint:gosec // operator-configured path
	if err != nil {
		return false, fmt.Errorf("writing %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.WriteString(line); err != nil {
		return false, fmt.Errorf("writing %s: %w", path, err)
	}
	return true, nil
}

// dateSlice is asctime()+4, truncated to 6 characters (act.other.c:920's
// `(tmp + 4)` fed to `%6.6s`): the month abbreviation, a space, and the day
// of the month space-padded to two digits — asctime's own "%2d" for the
// day. Go's "Jan _2" layout is exactly that slice: "_2" is a space-padded
// day, and the literal space between "Jan" and "_2" in the layout is the
// one asctime prints between the month and the day.
func dateSlice(t time.Time) string { return t.Format("Jan _2") }

// All implements reports.Store: every report in all three files, kind by
// kind, oldest first within each — the order they were appended in, since
// nothing here reorders them the way mail's free list does.
func (s *Store) All() ([]reports.Report, error) {
	var out []reports.Report
	for _, kind := range kindOrder {
		path := filepath.Join(s.dir, fileNames[kind])
		f, err := os.Open(path) //nolint:gosec // operator-configured path
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", path, err)
		}
		sc := bufio.NewScanner(f)
		lineNo := 0
		for sc.Scan() {
			lineNo++
			line := strings.TrimRight(sc.Text(), "\r")
			if line == "" {
				continue
			}
			r, ok := parseLine(line)
			if !ok {
				_ = f.Close()
				return nil, fmt.Errorf("%s:%d: does not match do_gen_write's format: %q", path, lineNo, line)
			}
			r.Kind = kind
			out = append(out, r)
		}
		err = sc.Err()
		_ = f.Close()
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", path, err)
		}
	}
	return out, nil
}

// parseLine reverses Append's fmt.Sprintf. It cannot use fixed column
// offsets: a name can run to twenty characters (maxNameLength,
// internal/session/login.go) and `%-8s` never truncates one that long, so
// the field boundary moves. Names are letters-only (invalidName), so a
// literal " (" cannot appear inside one — the first occurrence in the line
// is always the boundary between the name and the date.
func parseLine(line string) (reports.Report, bool) {
	i := strings.Index(line, " (")
	if i < 0 {
		return reports.Report{}, false
	}
	// %-8s pads a short name with trailing spaces; names are letters-only
	// (invalidName), so trimming them back off is lossless.
	name := strings.TrimRight(line[:i], " ")
	rest := line[i+2:]

	if len(rest) < 6 {
		return reports.Report{}, false
	}
	rest = rest[6:] // the date slice itself carries no structured data worth keeping (see reports.Report.When)

	rest, ok := strings.CutPrefix(rest, ") [")
	if !ok {
		return reports.Report{}, false
	}
	j := strings.Index(rest, "]")
	if j < 0 {
		return reports.Report{}, false
	}
	room, err := strconv.ParseInt(strings.TrimSpace(rest[:j]), 10, 32)
	if err != nil {
		return reports.Report{}, false
	}
	body := strings.TrimPrefix(rest[j+1:], " ")

	return reports.Report{Reporter: name, Room: int32(room), Body: body}, true //nolint:gosec // room vnums are 32-bit in this format
}
