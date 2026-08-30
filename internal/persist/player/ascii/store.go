// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.
//
// The format implemented here is the public ascii_pfiles 2.1 patch by Alan K.
// Miles, building on an original by Chris Jacobson.

package ascii

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"iter"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/persist/player"
)

// FormatName is the name this format registers under, and the default the
// server runs on.
const FormatName = "ascii"

// IndexFile lists every character, so that `who`, `autowiz` and the login
// sequence do not have to open every file to answer a question about levels
// or names.
const IndexFile = "plr_index"

func init() {
	player.Register(FormatName, func(cfg player.Config) (player.Store, error) {
		return New(cfg)
	})
}

// Store keeps one text file per character under <dir>/<letter>/<name>.
//
// Splitting by first letter is the format's own convention and exists for
// the same reason it always does: a few thousand files in one directory is
// unpleasant on the filesystems this was designed for. It is kept because
// interoperating with the C-side tooling matters more than the layout being
// to modern taste.
type Store struct {
	dir      string
	readOnly bool

	mu sync.RWMutex
	// indexProblems are malformed plr_index lines from the last read, kept
	// so the server can report them. Guarded by mu with the rest.
	indexProblems []string
}

// New opens an ascii player store.
func New(cfg player.Config) (*Store, error) {
	if cfg.Dir == "" {
		return nil, fmt.Errorf("ascii: no player directory configured")
	}
	return &Store{dir: cfg.Dir, readOnly: cfg.ReadOnly}, nil
}

// Name implements player.Store.
func (s *Store) Name() string { return FormatName }

// Close implements player.Store.
func (s *Store) Close() error { return nil }

// Capabilities implements player.Store.
//
// The zeroes are the point: this format has no fixed-width fields, so it
// imposes no limit on names, titles, descriptions, affects or skill numbers,
// and its timestamps are decimal text rather than 32-bit seconds. That is
// most of why the server runs on it.
func (s *Store) Capabilities() player.Capabilities {
	return player.Capabilities{
		CredentialSchemes: []game.CredentialScheme{
			game.SchemeNone, game.SchemeLegacyDES, game.SchemeArgon2id,
		},
		TimestampsOverflowIn2038: false,
	}
}

// pathFor returns a character's file path. Names are lowercased for both the
// directory and the file, which is what makes lookup case-insensitive
// without a scan.
func (s *Store) pathFor(name string) (string, error) {
	clean := strings.ToLower(strings.TrimSpace(name))
	if clean == "" {
		return "", fmt.Errorf("empty character name")
	}
	// A name is a filename here *and* a whitespace-separated field in
	// plr_index, so it has to be a real player name and not merely a safe
	// path. The C's _parse_name accepts letters and nothing else
	// (interpreter.c), and that is the rule enforced here.
	//
	// This is stricter than it needs to be for safety and deliberately so. A
	// name with a space in it produced a file *and* an index line that split
	// into the wrong number of fields, which made the whole roster
	// unreadable — every login and every character creation failed. Refusing
	// at the store is the backstop that keeps a caller's mistake from
	// costing everybody their game.
	for _, r := range clean {
		if r < 'a' || r > 'z' {
			return "", fmt.Errorf("character name %q is not a valid player name", name)
		}
	}
	return filepath.Join(s.dir, clean[:1], clean), nil
}

// Load implements player.Store.
func (s *Store) Load(ctx context.Context, name string) (*game.PlayerRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	path, err := s.pathFor(name)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path) //nolint:gosec // path is derived from a validated name
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("%q: %w", name, player.ErrNotFound)
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	rec, unknown, err := Decode(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if len(unknown) > 0 {
		// Not an error: the format tolerates unknown tags and some other
		// server may have written them. Worth surfacing, though, because the
		// alternative is losing them silently on the next save.
		return rec, fmt.Errorf("%s: %d unrecognised field(s): %s",
			path, len(unknown), strings.Join(unknown, "; "))
	}
	return rec, nil
}

// Exists implements player.Store.
func (s *Store) Exists(ctx context.Context, name string) (bool, error) {
	path, err := s.pathFor(name)
	if err != nil {
		return false, err
	}
	_, err = os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// Save implements player.Store.
func (s *Store) Save(ctx context.Context, rec *game.PlayerRecord) error {
	if s.readOnly {
		return fmt.Errorf("%s is open read-only", s.dir)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	path, err := s.pathFor(rec.Name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}

	var buf strings.Builder
	if err := Encode(&buf, rec); err != nil {
		return err
	}
	// 0600: a player file holds a password hash and connection hosts.
	if err := writeFileAtomic(path, []byte(buf.String()), 0o600); err != nil {
		return err
	}
	return s.rebuildIndex()
}

// Delete implements player.Store.
func (s *Store) Delete(ctx context.Context, name string) error {
	if s.readOnly {
		return fmt.Errorf("%s is open read-only", s.dir)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	path, err := s.pathFor(name)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%q: %w", name, player.ErrNotFound)
		}
		return err
	}
	return s.rebuildIndex()
}

// List implements player.Store.
//
// It reads the index rather than every player file, which is what the index
// is for. A character present on disk but missing from the index is reported
// as a problem rather than silently included: the two disagreeing is exactly
// the kind of thing worth knowing about.
func (s *Store) List(ctx context.Context) iter.Seq2[player.IndexEntry, error] {
	return func(yield func(player.IndexEntry, error) bool) {
		s.mu.RLock()
		defer s.mu.RUnlock()

		entries, err := s.readIndex()
		if err != nil {
			yield(player.IndexEntry{}, err)
			return
		}
		for _, e := range entries {
			if err := ctx.Err(); err != nil {
				yield(player.IndexEntry{}, err)
				return
			}
			if !yield(e, nil) {
				return
			}
		}
	}
}

// readIndex parses plr_index. A missing index is an empty roster, which is
// the normal fresh-install state.
func (s *Store) readIndex() ([]player.IndexEntry, error) {
	f, err := os.Open(filepath.Join(s.dir, IndexFile)) //nolint:gosec // operator-configured path
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	// A malformed line is skipped rather than fatal.
	//
	// The index is *derived*: rebuildIndex regenerates the whole thing from
	// the player files, so a line lost here comes back the next time anybody
	// saves. Refusing to read it at all is not recoverable — it takes down
	// every login and every character creation for everybody, which is
	// exactly what a stray unparseable line once did. The skipped lines are
	// returned so the caller can say so rather than swallowing them.
	var skipped []string
	var out []player.IndexEntry
	sc := bufio.NewScanner(f)
	for line := 1; sc.Scan(); line++ {
		text := strings.TrimSpace(sc.Text())
		if text == "" || text == "~" {
			continue
		}
		// "<idnum> <name> <level> <flags> <last_logon>"
		fields := strings.Fields(text)
		if len(fields) < 5 {
			skipped = append(skipped, fmt.Sprintf("line %d: want 5 fields, got %d: %q", line, len(fields), text))
			continue
		}
		id, err := strconv.ParseInt(fields[0], 10, 64)
		if err != nil {
			skipped = append(skipped, fmt.Sprintf("line %d: id %q is not a number", line, fields[0]))
			continue
		}
		level, err := strconv.ParseInt(fields[2], 10, 32)
		if err != nil {
			skipped = append(skipped, fmt.Sprintf("line %d: level %q is not a number", line, fields[2]))
			continue
		}
		flagBits, _ := game.ParseFlagLetters(fields[3])
		flags := game.SetFromRaw[game.PlayerFlag](flagBits)
		out = append(out, player.IndexEntry{
			Name: fields[1], IDNum: id, Level: int32(level), Flags: flags,
		})
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(skipped) > 0 {
		s.noteSkipped(skipped)
	}
	return out, nil
}

// noteSkipped records malformed index lines. The store has no logger, so it
// keeps them for LastIndexProblems to report.
func (s *Store) noteSkipped(problems []string) {
	s.indexProblems = problems
}

// IndexProblems returns the malformed plr_index lines seen by the last read,
// so the server can log them once at boot rather than losing them.
func (s *Store) IndexProblems() []string { return s.indexProblems }

// rebuildIndex regenerates plr_index from the player files.
//
// The reference implementation maintains the index incrementally. Rebuilding
// it wholesale is slower and cannot drift: an index that disagrees with the
// files is a class of bug that simply does not arise. For a roster of a few
// hundred saved once a minute, the cost is not worth the risk.
//
// The caller must hold the write lock.
func (s *Store) rebuildIndex() error {
	names, err := s.scanNames()
	if err != nil {
		return err
	}
	sort.Strings(names)

	var b strings.Builder
	for _, name := range names {
		path, err := s.pathFor(name)
		if err != nil {
			continue
		}
		f, err := os.Open(path) //nolint:gosec // path derived from a scan of our own directory
		if err != nil {
			return err
		}
		rec, _, err := Decode(f)
		_ = f.Close()
		if err != nil {
			return fmt.Errorf("rebuilding %s: %s: %w", IndexFile, path, err)
		}

		// Flags are written as "0" when empty; an empty field here would
		// misalign the whitespace-separated reader.
		flags := "0"
		if !rec.PlayerFlags.Empty() {
			flags = rec.PlayerFlags.String()
		}
		fmt.Fprintf(&b, "%d %s %d %s %d\n",
			rec.IDNum, strings.ToLower(rec.Name), rec.Level, flags, unixOrZero(rec.LastLogon))
	}
	b.WriteString("~\n")

	return writeFileAtomic(filepath.Join(s.dir, IndexFile), []byte(b.String()), 0o600)
}

// scanNames finds every player file on disk.
func (s *Store) scanNames() ([]string, error) {
	var names []string

	letters, err := os.ReadDir(s.dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	for _, letter := range letters {
		if !letter.IsDir() || len(letter.Name()) != 1 {
			continue
		}
		files, err := os.ReadDir(filepath.Join(s.dir, letter.Name()))
		if err != nil {
			return nil, err
		}
		for _, f := range files {
			if f.IsDir() || strings.HasPrefix(f.Name(), ".") {
				continue
			}
			names = append(names, f.Name())
		}
	}
	return names, nil
}

// RebuildIndex regenerates plr_index from the files on disk. Exposed so a
// roster produced by conversion, or one whose index has been lost, can be
// repaired without a save.
func (s *Store) RebuildIndex() error {
	if s.readOnly {
		return fmt.Errorf("%s is open read-only", s.dir)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rebuildIndex()
}

// writeFileAtomic writes via a temporary file and a rename, so an
// interrupted write leaves the previous contents rather than half of the new
// ones. For a real player's character that is worth the extra syscalls.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+"-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(name) // a no-op once the rename has succeeded
	}()

	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(name, perm); err != nil {
		return err
	}
	return os.Rename(name, path)
}
