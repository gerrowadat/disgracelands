// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package yaml

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"iter"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/goccy/go-yaml"

	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/persist/atomicfile"
	"github.com/gerrowadat/disgracelands/internal/persist/player"
	"github.com/gerrowadat/disgracelands/internal/persist/yamlenc"
)

// FormatName is the name this format registers under.
const FormatName = "yaml"

// playerSchema is the document's schema tag, docs/design/data-format.md
// §10.1.
const playerSchema = "dl/player@1"

func init() {
	player.Register(FormatName, func(cfg player.Config) (player.Store, error) {
		return New(cfg)
	})
}

// Store keeps one file per character under <dir>/<letter>/<name>.yaml,
// folding in the roster entry and the rent/crash file both (§8: "one
// player, one file") — there is no plr_index either, matching §3's "the
// roster is built by scanning at boot": List walks the directory tree
// rather than reading an index.
//
// Store.Save and ObjectStore's four methods all read the whole file,
// change only the section that is theirs, and write the whole file back —
// the two halves are called at different times over the same document
// (rent.go's crashSave calls SaveObjects, then Save, moments apart; a
// periodic save calls only Save) and neither may clobber what the other
// last wrote.
type Store struct {
	dir      string
	readOnly bool
	mu       sync.RWMutex
}

// New opens a yaml player store.
func New(cfg player.Config) (*Store, error) {
	if cfg.Dir == "" {
		return nil, fmt.Errorf("yaml: no player directory configured")
	}
	return &Store{dir: cfg.Dir, readOnly: cfg.ReadOnly}, nil
}

// Name implements player.Store.
func (s *Store) Name() string { return FormatName }

// Close implements player.Store.
func (s *Store) Close() error { return nil }

// Capabilities implements player.Store.
//
// The zeroes match ascii's own: no fixed-width fields, so no limit on
// names, titles, descriptions, affects or skill numbers, and timestamps
// are RFC 3339 text rather than 32-bit seconds.
func (s *Store) Capabilities() player.Capabilities {
	return player.Capabilities{
		CredentialSchemes: []game.CredentialScheme{
			game.SchemeNone, game.SchemeLegacyDES, game.SchemeArgon2id,
		},
		TimestampsOverflowIn2038: false,
	}
}

// pathFor returns a character's file path, matching ascii's own
// letter-bucketed, lowercased, letters-only convention (ascii/store.go) —
// a name is a filename here too, so the same restriction applies for the
// same reason.
func (s *Store) pathFor(name string) (string, error) {
	clean := strings.ToLower(strings.TrimSpace(name))
	if clean == "" {
		return "", fmt.Errorf("empty character name")
	}
	for _, r := range clean {
		if r < 'a' || r > 'z' {
			return "", fmt.Errorf("character name %q is not a valid player name", name)
		}
	}
	return filepath.Join(s.dir, clean[:1], clean+".yaml"), nil
}

// loadDoc reads and strict-decodes one character's file. A missing file is
// reported via the returned bool rather than an error, since both Save (a
// brand new character) and the ObjectStore methods (which require an
// existing file — see readExistingDoc) need to tell "absent" apart from
// "malformed" differently.
func (s *Store) loadDoc(path string) (*playerDoc, bool, error) {
	b, err := os.ReadFile(path) //nolint:gosec // path is built from a validated name
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var doc playerDoc
	if err := yaml.UnmarshalWithOptions(b, &doc, yaml.Strict()); err != nil {
		return nil, true, fmt.Errorf("%s: %w", path, err)
	}
	return &doc, true, nil
}

// writeDoc marshals and atomically writes doc.
func writeDoc(path string, doc *playerDoc) error {
	out, err := yaml.MarshalWithOptions(doc, yamlenc.Options()...)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	// 0600: a player file holds a credential and connection hosts.
	return atomicfile.Write(path, out, 0o600)
}

// Load implements player.Store.
func (s *Store) Load(ctx context.Context, name string) (*game.PlayerRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	path, err := s.pathFor(name)
	if err != nil {
		return nil, err
	}
	doc, found, err := s.loadDoc(path)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("%q: %w", name, player.ErrNotFound)
	}
	rec, unknown, err := recordFromDoc(doc)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if len(unknown) > 0 {
		return rec, fmt.Errorf("%s: %d unrecognised name(s): %s", path, len(unknown), strings.Join(unknown, "; "))
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
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

// Save implements player.Store.
//
// The rent/crash section is preserved from whatever was already on disk —
// Save never touches it, so a periodic save cannot lose an inventory
// SaveObjects wrote moments before.
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
	existing, _, err := s.loadDoc(path)
	if err != nil {
		return err
	}

	doc := docFromRecord(rec)
	if existing != nil {
		doc.Rent = existing.Rent
		doc.Inventory = existing.Inventory
	}
	return writeDoc(path, &doc)
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
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("%q: %w", name, player.ErrNotFound)
		}
		return err
	}
	return nil
}

// List implements player.Store by walking the directory tree — no index
// to trust or rebuild, per §3.
//
// The lock is taken and released *per file* rather than held across the
// whole iteration, and that is load-bearing rather than a micro-
// optimisation. A caller ranging over List and calling Save or
// DeleteObjects on what it finds is an ordinary shape — SweepRentFiles and
// `dlctl fmt --type=pfile` both do it — and with the lock held for the
// duration it is a permanent self-deadlock: Go's RWMutex is not reentrant,
// and a writer arriving while a reader holds the lock blocks, on the same
// goroutine that would have released it.
//
// It was not a hazard while this store was only ever the *roster* and a
// separate binary.ObjectStore held the rent files, because the two had
// separate locks. It became one the moment a server ran on yaml, where one
// store is both — which is what moving internal/server's harness onto yaml
// (docs/design/yaml-only.md §5.4) turned up, as a rent sweep that hung
// the server for good the first time it had a file to delete.
//
// What the narrower locking gives up is a consistent snapshot: a character
// saved while a listing is in flight may or may not appear, and one deleted
// may be yielded and then be gone. Neither matters to any caller — the C's
// own equivalents walk a directory with no locking at all — and both are
// better than a hang.
func (s *Store) List(ctx context.Context) iter.Seq2[player.IndexEntry, error] {
	return func(yield func(player.IndexEntry, error) bool) {
		paths, err := s.listPaths()
		if err != nil {
			yield(player.IndexEntry{}, err)
			return
		}

		for _, path := range paths {
			if err := ctx.Err(); err != nil {
				yield(player.IndexEntry{}, err)
				return
			}
			doc, found, err := s.loadDocLocking(path)
			if err != nil {
				if !yield(player.IndexEntry{}, fmt.Errorf("%s: %w", path, err)) {
					return
				}
				continue
			}
			if !found {
				continue
			}
			act, _ := game.ParseBitNames(doc.Flags.Act, game.YamlPlayerFlagNames())
			act = act.Set(game.Flags(doc.Flags.ActRaw))
			// Lower-cased: see player.IndexEntry.Name.
			entry := player.IndexEntry{
				Name: strings.ToLower(doc.Name), IDNum: doc.ID,
				Level: doc.Identity.Level, Flags: act,
			}
			if !yield(entry, nil) {
				return
			}
		}
	}
}

// listPaths is every character file in the tree, sorted, read under the
// lock and returned rather than iterated with it held. See List.
func (s *Store) listPaths() ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var paths []string
	err := filepath.WalkDir(s.dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".yaml") {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	sort.Strings(paths)
	return paths, nil
}

// loadDocLocking is loadDoc with the read lock taken around it, for List's
// per-file reads. A file that has been deleted since listPaths ran is not
// an error: loadDoc reports it as not found, which List skips.
func (s *Store) loadDocLocking(path string) (*playerDoc, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.loadDoc(path)
}

// LoadObjects implements player.ObjectStore.
func (s *Store) LoadObjects(ctx context.Context, name string) (*player.RentFile, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	path, err := s.pathFor(name)
	if err != nil {
		return nil, err
	}
	doc, found, err := s.loadDoc(path)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("%q: %w", name, player.ErrNotFound)
	}
	f, unknown, ok := rentFileFromDoc(doc)
	if !ok {
		return nil, fmt.Errorf("%q: %w", name, player.ErrNotFound)
	}
	if len(unknown) > 0 {
		return f, fmt.Errorf("%s: %d unrecognised name(s): %s", path, len(unknown), strings.Join(unknown, "; "))
	}
	return f, nil
}

// SaveObjects implements player.ObjectStore.
//
// The roster half is preserved from whatever is already on disk. Unlike
// Save, a missing file is an error rather than something to create from
// nothing: a character must already have a roster entry before they can
// possibly have anything to rent or crash-save (see doc.go's package
// comment).
func (s *Store) SaveObjects(ctx context.Context, name string, f *player.RentFile) error {
	if s.readOnly {
		return fmt.Errorf("%s is open read-only", s.dir)
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	path, err := s.pathFor(name)
	if err != nil {
		return err
	}
	doc, err := s.readExistingDoc(path, name)
	if err != nil {
		return err
	}
	applyRentFile(doc, f)
	return writeDoc(path, doc)
}

// DeleteObjects implements player.ObjectStore.
//
// Clears the rent/inventory section rather than removing the file: the
// file is also the roster entry, and Crash_idlesave/objsave.c's "delete
// with nothing to store" only ever meant the *rent* file.
func (s *Store) DeleteObjects(ctx context.Context, name string) error {
	if s.readOnly {
		return fmt.Errorf("%s is open read-only", s.dir)
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	path, err := s.pathFor(name)
	if err != nil {
		return err
	}
	doc, found, err := s.loadDoc(path)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("%q: %w", name, player.ErrNotFound)
	}
	if doc.Rent == nil {
		return nil
	}
	doc.Rent = nil
	doc.Inventory = nil
	return writeDoc(path, doc)
}

// MarkCrashed implements player.ObjectStore: rewrites just the rent
// header's code and timestamp, leaving the inventory alone — Crash_load's
// own rewind-and-rewrite (objsave.c:617).
func (s *Store) MarkCrashed(ctx context.Context, name string, at time.Time) error {
	if s.readOnly {
		return fmt.Errorf("%s is open read-only", s.dir)
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	path, err := s.pathFor(name)
	if err != nil {
		return err
	}
	doc, found, err := s.loadDoc(path)
	if err != nil {
		return err
	}
	if !found || doc.Rent == nil {
		return fmt.Errorf("%q: %w", name, player.ErrNotFound)
	}
	doc.Rent.Code = player.RentCrash.String()
	doc.Rent.Written = rfc3339OrEmpty(at)
	return writeDoc(path, doc)
}

// readExistingDoc loads a character's file, refusing rather than creating
// one from nothing when it is absent — see SaveObjects' doc comment.
func (s *Store) readExistingDoc(path, name string) (*playerDoc, error) {
	doc, found, err := s.loadDoc(path)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("%q has no roster entry to attach a rent file to: %w", name, player.ErrNotFound)
	}
	return doc, nil
}
