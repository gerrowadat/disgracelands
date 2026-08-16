package binary

import (
	"context"
	"errors"
	"fmt"
	"io"
	"iter"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/persist/player"
)

// FormatName is the name this format registers under. It is the default,
// because pointing the Go server at an existing data directory has to work
// with no migration and no flags.
const FormatName = "binary"

// FileName is the database file inside the player directory.
const FileName = "players"

func init() {
	player.Register(FormatName, func(cfg player.Config) (player.Store, error) {
		return New(cfg)
	})
}

// Store reads and writes the flat binary player database.
//
// The whole roster lives in one file of fixed-size records, indexed by
// position. That is why the C server keeps a `plr_index` in memory and why
// deleting a character marks it rather than removing it: a real deletion
// would renumber everyone after it, and other files reference players by
// position.
type Store struct {
	path     string
	readOnly bool
	codec    *codec

	// mu guards the file. Records are fixed-size and independently
	// addressable, so this could be finer-grained, but a roster of a few
	// hundred saved once a minute does not need it.
	mu sync.RWMutex
}

// New opens a binary player store.
func New(cfg player.Config) (*Store, error) {
	if cfg.Dir == "" {
		return nil, fmt.Errorf("binary: no player directory configured")
	}
	return &Store{
		path:     filepath.Join(cfg.Dir, FileName),
		readOnly: cfg.ReadOnly,
		// The archived data is ILP32 and that is what this reads. A file in
		// the LP64 layout can only have been produced by a modern rebuild of
		// the C server, which has never run here; detectModel reports that
		// case rather than misreading it.
		codec: newCodec(ilp32),
	}, nil
}

// Name implements player.Store.
func (s *Store) Name() string { return FormatName }

// Close implements player.Store. Nothing is held open between calls.
func (s *Store) Close() error { return nil }

// Capabilities implements player.Store.
func (s *Store) Capabilities() player.Capabilities {
	l := s.codec.layout
	return player.Capabilities{
		MaxNameLength:        l.at("name").Size - 1,
		MaxTitleLength:       l.at("title").Size - 1,
		MaxDescriptionLength: l.at("description").Size - 1,
		MaxAffects:           maxAffect,
		MaxSkillNumber:       l.at("ps.skills").Size - 1,
		// Only what crypt(3) produced. Storing a modern hash here is not a
		// matter of squeezing it in: the field is eleven bytes.
		CredentialSchemes:        []game.CredentialScheme{game.SchemeNone, game.SchemeLegacyDES},
		TimestampsOverflowIn2038: l.at("birth").Size == 4,
	}
}

// RecordSize returns the on-disk record size, which is how a caller can tell
// which data model a file is in.
func (s *Store) RecordSize() int { return s.codec.RecordSize() }

// readAll loads the whole file. The roster is a few hundred records of about
// 1.3KB, so this is well under a megabyte and simpler than seeking.
func (s *Store) readAll() ([]byte, error) {
	data, err := os.ReadFile(s.path) //nolint:gosec // operator-configured path
	if errors.Is(err, os.ErrNotExist) {
		// A missing file is an empty roster, not an error. The C server
		// creates it on demand and treats a blank one as the normal
		// fresh-install state — see the note in README.md about the first
		// player becoming Implementor.
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	size := s.codec.RecordSize()
	if len(data)%size != 0 {
		return nil, s.explainSize(len(data))
	}
	return data, nil
}

// explainSize turns "the file is not a whole number of records" into
// something actionable, because the most likely cause is a data model
// mismatch and the symptom otherwise looks like corruption.
func (s *Store) explainSize(n int) error {
	small, big := computeLayout(ilp32).Size, computeLayout(lp64).Size
	if n%big == 0 {
		return fmt.Errorf("%s is %d bytes, which is %d records of %d — the 64-bit layout. "+
			"This file was written by a modern rebuild of the C server, not by the original; "+
			"reading it as 32-bit would silently misread every field past the first `long`",
			s.path, n, n/big, big)
	}
	return fmt.Errorf("%s is %d bytes, which is not a whole number of %d-byte records "+
		"(%d full records plus %d bytes)", s.path, n, small, n/small, n%small)
}

// Load implements player.Store.
func (s *Store) Load(ctx context.Context, name string) (*game.PlayerRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for rec, err := range s.all(ctx) {
		if err != nil {
			return nil, err
		}
		if strings.EqualFold(rec.Name, name) {
			return rec, nil
		}
	}
	return nil, fmt.Errorf("%q: %w", name, player.ErrNotFound)
}

// Exists implements player.Store.
func (s *Store) Exists(ctx context.Context, name string) (bool, error) {
	_, err := s.Load(ctx, name)
	if errors.Is(err, player.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// all iterates every record in file order.
func (s *Store) all(ctx context.Context) iter.Seq2[*game.PlayerRecord, error] {
	return func(yield func(*game.PlayerRecord, error) bool) {
		data, err := s.readAll()
		if err != nil {
			yield(nil, err)
			return
		}
		size := s.codec.RecordSize()
		for i := 0; i*size < len(data); i++ {
			if err := ctx.Err(); err != nil {
				yield(nil, err)
				return
			}
			rec, err := s.codec.decode(data[i*size : (i+1)*size])
			if err != nil {
				if !yield(nil, fmt.Errorf("record %d: %w", i, err)) {
					return
				}
				continue
			}
			if !yield(rec, nil) {
				return
			}
		}
	}
}

// List implements player.Store.
func (s *Store) List(ctx context.Context) iter.Seq2[player.IndexEntry, error] {
	return func(yield func(player.IndexEntry, error) bool) {
		s.mu.RLock()
		defer s.mu.RUnlock()

		for rec, err := range s.all(ctx) {
			if err != nil {
				if !yield(player.IndexEntry{}, err) {
					return
				}
				continue
			}
			if !yield(player.IndexEntry{
				Name: rec.Name, IDNum: rec.IDNum,
				Level: rec.Level, Flags: rec.PlayerFlags,
			}, nil) {
				return
			}
		}
	}
}

// Save implements player.Store.
func (s *Store) Save(ctx context.Context, rec *game.PlayerRecord) error {
	if s.readOnly {
		return fmt.Errorf("%s is open read-only", s.path)
	}
	if rec.Name == "" {
		return fmt.Errorf("cannot save a character with no name")
	}

	encoded, err := s.codec.encode(rec)
	if err != nil {
		return fmt.Errorf("encoding %q: %w", rec.Name, err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := s.readAll()
	if err != nil {
		return err
	}
	size := s.codec.RecordSize()

	// Find the existing record and overwrite it in place; positions are
	// referenced elsewhere, so an update must not move anyone.
	slot := -1
	for i := 0; i*size < len(data); i++ {
		existing, err := s.codec.decode(data[i*size : (i+1)*size])
		if err != nil {
			continue
		}
		if strings.EqualFold(existing.Name, rec.Name) {
			slot = i
			break
		}
	}

	if slot >= 0 {
		copy(data[slot*size:(slot+1)*size], encoded)
	} else {
		data = append(data, encoded...)
	}

	return s.writeAll(data)
}

// Delete implements player.Store.
//
// This removes the record rather than marking it, which renumbers everyone
// after it. That is safe here and is not safe in the running server: the C
// server references players by file position from the house and mail files,
// which is why it sets PLR_DELETED instead. Nothing in the Go port stores a
// position, and the phase that adds houses and mail will have to decide the
// same question again.
func (s *Store) Delete(ctx context.Context, name string) error {
	if s.readOnly {
		return fmt.Errorf("%s is open read-only", s.path)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := s.readAll()
	if err != nil {
		return err
	}
	size := s.codec.RecordSize()

	for i := 0; i*size < len(data); i++ {
		existing, err := s.codec.decode(data[i*size : (i+1)*size])
		if err != nil {
			continue
		}
		if !strings.EqualFold(existing.Name, name) {
			continue
		}
		data = append(data[:i*size], data[(i+1)*size:]...)
		return s.writeAll(data)
	}
	return fmt.Errorf("%q: %w", name, player.ErrNotFound)
}

// writeAll replaces the database, via a temporary file and a rename.
//
// The C server writes records in place with fseek and fwrite, so an
// interrupted save leaves a half-written record and a corrupt roster. Writing
// a whole new file and renaming it means an interrupted save leaves the old
// roster intact, which for real players' characters is worth the extra I/O.
func (s *Store) writeAll(data []byte) error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, ".players-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName) // no-op once the rename has succeeded
	}()

	if _, err := tmp.Write(data); err != nil {
		return err
	}
	// The rename is only atomic with respect to a crash if the contents are
	// on disk first.
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	// 0600: this file is password hashes and connection hosts.
	if err := os.Chmod(tmpName, 0o600); err != nil {
		return err
	}
	return os.Rename(tmpName, s.path)
}

// Verify checks that every record in the file decodes, and reports what it
// found. `dlctl pfile verify` uses it; nothing else should need it.
func (s *Store) Verify(ctx context.Context) (Report, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := s.readAll()
	if err != nil {
		return Report{}, err
	}

	size := s.codec.RecordSize()
	r := Report{RecordSize: size, Bytes: len(data), Records: len(data) / size}

	for rec, err := range s.all(ctx) {
		if err != nil {
			r.Problems = append(r.Problems, err.Error())
			continue
		}
		if rec.Name == "" {
			r.Empty++
			continue
		}
		r.Named++
		if rec.Credential.Scheme == game.SchemeLegacyDES {
			r.LegacyPasswords++
		}
	}
	return r, nil
}

// Report is what Verify found.
type Report struct {
	RecordSize      int
	Bytes           int
	Records         int
	Named           int
	Empty           int
	LegacyPasswords int
	Problems        []string
}

var _ io.Closer = (*Store)(nil)
