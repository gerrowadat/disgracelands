// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package binary

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/persist/player"
)

// The alias files: one per character under plralias/, bucketed by first
// letter exactly like the rent files (get_filename, utils.c:518).
//
// Unlike every other file this package reads, this one is not an fwrite of a
// struct — alias.c writes it with fprintf and reads it back with
// fscanf/fgets, so there is no layout to derive and nothing for a
// reference/tools/*layout.c to check. What there is instead is a
// length-prefix convention that is easy to read past:
//
//	%d\n   strlen(alias)
//	%s\n   alias
//	%d\n   strlen(replacement) - 1
//	%s\n   replacement + 1
//	%d\n   type
//
// The two adjustments on the replacement are the whole subtlety. do_alias
// builds a replacement with `repl = any_one_arg(argument, arg)`, and
// any_one_arg — unlike one_argument — does *not* skip the whitespace it
// stops on, so the in-memory replacement always begins with the space that
// separated the alias's name from the rest of the line. write_aliases
// strips it (`replacement + 1`, and a length one shorter) and read_aliases
// puts it back (`*xbuf = ' '` before the fgets). This port keeps that
// leading space in game.Alias.Replacement too, deliberately — see
// internal/session/alias.go, where $* substitutes the raw untrimmed
// remainder — so the conversion here is exactly the C's.
//
// The `type` field is read and recomputed rather than stored. The C derives
// it from the replacement when the alias is created (ALIAS_COMPLEX if the
// text holds a ';' or a '$', interpreter.c:737) and this port derives it the
// same way at use (isComplexAlias), so there is nothing for game.Alias to
// hold: no file write_aliases produced can disagree with the derivation,
// and all 339 aliases in the surviving archive agree with it.
const (
	// aliasDir is LIB_PLRALIAS (db.h:38), and like plrobjs/ it is resolved
	// against the mud's own cwd rather than against the roster's directory.
	aliasDir = "plralias"
	// aliasSuffix is SUF_ALIAS (db.h:47).
	aliasSuffix = "alias"
)

// AliasStore reads and writes the per-character alias files.
type AliasStore struct {
	dir      string
	readOnly bool
}

// NewAliasStore opens the alias files for a configuration.
func NewAliasStore(cfg player.Config) (*AliasStore, error) {
	if cfg.Dir == "" {
		return nil, fmt.Errorf("binary: no player directory configured")
	}
	return &AliasStore{dir: AliasPath(cfg), readOnly: cfg.ReadOnly}, nil
}

// AliasPath is the plralias/ directory a configuration names: cfg.AliasDir
// when it is set, and Dir/plralias when it is not. Same split, and the same
// reason for it, as ObjectsPath.
func AliasPath(cfg player.Config) string {
	if cfg.AliasDir != "" {
		return cfg.AliasDir
	}
	return filepath.Join(cfg.Dir, aliasDir)
}

func (s *AliasStore) pathFor(name string) (string, error) {
	return bucketedPath(s.dir, name, aliasSuffix, "alias file")
}

// LoadAliases reads one character's aliases, newest first — the order the
// file has them, which is the order GET_ALIASES holds them in, which is
// prepend-on-add. game.PlayerRecord.Aliases uses the same order.
//
// A missing file is player.ErrNotFound, not an error: alias.c removes the
// file outright when a character has no aliases left (write_aliases's
// `remove(fn)` before its NULL check), so "no file" is the normal state for
// most characters.
func (s *AliasStore) LoadAliases(name string) ([]game.Alias, error) {
	path, err := s.pathFor(name)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(path) //nolint:gosec // the path is built from a validated name
	if errors.Is(err, os.ErrNotExist) {
		return nil, player.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s's alias file: %w", name, err)
	}
	return decodeAliases(b, name)
}

// SaveAliases writes one character's aliases, or removes the file when there
// are none — which is what write_aliases does, and not merely tidiness: the
// C's reader has no empty-file case at all, so leaving a zero-byte file
// behind would be leaving something it cannot read.
func (s *AliasStore) SaveAliases(name string, aliases []game.Alias) error {
	if s.readOnly {
		return fmt.Errorf("binary: the player directory is open read-only")
	}
	path, err := s.pathFor(name)
	if err != nil {
		return err
	}
	if len(aliases) == 0 {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("removing %s's alias file: %w", name, err)
		}
		return nil
	}
	body, err := encodeAliases(aliases)
	if err != nil {
		return fmt.Errorf("%s's aliases: %w", name, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	return os.WriteFile(path, body, 0o600)
}

// encodeAliases is write_aliases (alias.c:22).
//
// It refuses a replacement that does not begin with a space rather than
// writing one, which is the same posture putStr takes toward a name too
// long for its field: a truncated name is a different character, and a
// replacement missing its first character is a different command.
//
// The leading space is an invariant of game.Alias.Replacement, not an
// accident of this format — the file header above has the C citations, and
// $* substitutes the raw untrimmed remainder because of it. Nothing the
// *game* produces can violate it, since do_alias builds every replacement
// with any_one_arg. But SaveAliases is a public writer, and this stripped
// the first character of anything else unconditionally: "get all corpse"
// came back as " et all corpse", with the missing g replaced by a space on
// the way in. It went unnoticed because both sides of every comparison
// went through this same encoder — examples/torture's alias fixture was
// written that way and the corpus built to expose blind spots recorded the
// mangled form on both sides of its own conversion (#242).
func encodeAliases(aliases []game.Alias) ([]byte, error) {
	var b bytes.Buffer
	for _, a := range aliases {
		// The C writes strlen(replacement) - 1 and replacement + 1, which
		// is a buffer underrun waiting to happen for an empty replacement.
		// It cannot get one: do_alias treats an empty replacement as a
		// delete. A record that reached here with one anyway is written as
		// the empty string rather than as a negative length.
		repl := a.Replacement
		if repl != "" {
			if repl[0] != ' ' {
				return nil, fmt.Errorf(
					"alias %q: a replacement must begin with the space do_alias leaves in front of it, "+
						"and %q does not — write_aliases stores it one character shorter (alias.c:22), "+
						"so storing this would lose its first character", a.Name, a.Replacement)
			}
			repl = repl[1:]
		}
		fmt.Fprintf(&b, "%d\n%s\n%d\n%s\n%d\n",
			len(a.Name), a.Name, len(repl), repl, aliasType(a.Replacement))
	}
	return b.Bytes(), nil
}

// aliasType is the ALIAS_SIMPLE/ALIAS_COMPLEX decision (interpreter.c:737),
// recomputed on the way out rather than carried. ALIAS_SEP_CHAR is ';' and
// ALIAS_VAR_CHAR is '$' (interpreter.h:66-67).
func aliasType(replacement string) int {
	if bytes.ContainsAny([]byte(replacement), ";$") {
		return 1 // ALIAS_COMPLEX
	}
	return 0 // ALIAS_SIMPLE
}

// decodeAliases is read_aliases (alias.c:55).
//
// The C's loop is `for(;;)` with its exit test after the third field, so a
// file that ends cleanly ends the loop; this stops on any short read
// instead, which covers the same files and does not walk off a truncated
// one. A malformed file yields what was read before the damage rather than
// an error, which is the C's behaviour too — it has no validation here at
// all, and a character whose alias file was half-written should still be
// able to log in.
func decodeAliases(b []byte, name string) ([]game.Alias, error) {
	var out []game.Alias
	pos := 0

	field := func() (string, bool) {
		nl := bytes.IndexByte(b[pos:], '\n')
		if nl < 0 {
			return "", false
		}
		n, err := strconv.Atoi(string(bytes.TrimSpace(b[pos : pos+nl])))
		if err != nil || n < 0 {
			return "", false
		}
		pos += nl + 1
		if pos+n > len(b) {
			return "", false
		}
		// fgets(buf, length+1, file) reads exactly length bytes and leaves
		// the newline behind for the next fscanf's whitespace skip.
		text := string(b[pos : pos+n])
		pos += n
		if pos < len(b) && b[pos] == '\n' {
			pos++
		}
		return text, true
	}

	for pos < len(b) {
		aliasName, ok := field()
		if !ok {
			break
		}
		repl, ok := field()
		if !ok {
			break
		}
		// The type, read and discarded: see this file's header comment.
		nl := bytes.IndexByte(b[pos:], '\n')
		if nl < 0 {
			pos = len(b)
		} else {
			pos += nl + 1
		}
		if aliasName == "" {
			continue
		}
		out = append(out, game.Alias{Name: aliasName, Replacement: " " + repl})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("binary: %s's alias file has no readable entries", name)
	}
	return out, nil
}
