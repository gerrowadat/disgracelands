// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.
//
// Package convert turns an original CircleMUD data directory into one the Go
// server can run on.
//
// It is the same principle the player formats already follow: old formats are
// *inputs to a converter*, not things the live server carries. Applied to
// text, that means the server works in UTF-8 and the CP1252 that a 2002
// world was edited in gets converted once, here, rather than being decoded
// per-connection forever.
//
// What it will not do is guess. Several files in a CircleMUD data directory
// are raw struct dumps rather than text — the message boards, rent files,
// player mail — and running a byte-level transcode over one of those corrupts
// it twice: once by rewriting bytes that were never characters, and again by
// changing the length of text whose length is stored separately. Those
// formats are detected, reported, and left exactly as they are.
package convert

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"

	"github.com/gerrowadat/disgracelands/internal/persist/player"
	"github.com/gerrowadat/disgracelands/internal/persist/player/ascii"
	"github.com/gerrowadat/disgracelands/internal/persist/player/binary"
)

// Encodings a source directory might be in.
//
// CP1252 is the default because it is what these files actually are: a 2002
// world edited on and around Windows. It agrees with Latin-1 everywhere
// except 0x80–0x9F, where Latin-1 has unprintable C1 controls and CP1252 has
// the curly quotes and dashes that a word processor inserts. data/world/wld/
// 90.wld contains 0x92, which is a right single quotation mark in CP1252 and
// meaningless in Latin-1 — so the choice is not arbitrary and the default is
// not a coin toss.
var Encodings = map[string]*charmap.Charmap{
	"cp1252": charmap.Windows1252,
	"latin1": charmap.ISO8859_1,
}

// DefaultEncoding names the assumed source encoding.
const DefaultEncoding = "cp1252"

// Options controls a conversion.
type Options struct {
	// From is the original data directory; To is where the converted one is
	// written. They must differ: converting in place would leave a
	// half-converted directory behind on any failure.
	From, To string

	// Encoding is the source text encoding.
	Encoding *charmap.Charmap

	// DryRun reports what would happen without writing anything.
	DryRun bool

	// Force allows writing into a non-empty destination.
	Force bool
}

// Action is what happened to one file.
type Action int

const (
	// Copied: the bytes were already fine and were passed through.
	Copied Action = iota
	// Transcoded: the file held non-UTF-8 text, which was converted.
	Transcoded
	// Reformatted: the file was rewritten in a different format entirely,
	// which is what happens to the player database.
	Reformatted
	// Unsupported: a binary format this converter does not understand. Copied
	// verbatim and reported, because the alternative is corrupting it.
	Unsupported
)

func (a Action) String() string {
	switch a {
	case Copied:
		return "copied"
	case Transcoded:
		return "transcoded"
	case Reformatted:
		return "reformatted"
	case Unsupported:
		return "unsupported"
	}
	return "?"
}

// Entry is one file's outcome.
type Entry struct {
	Path   string
	Action Action
	// Note explains anything the action alone does not.
	Note string
}

// Report is the outcome of a conversion.
type Report struct {
	Entries  []Entry
	Problems []string
}

// Count returns how many entries had a given action.
func (r *Report) Count(a Action) int {
	n := 0
	for _, e := range r.Entries {
		if e.Action == a {
			n++
		}
	}
	return n
}

// binaryFormats are the files that are raw struct dumps rather than text.
//
// Every one of them is a `fwrite` of a struct containing pointers or
// explicit length fields, so both a byte-level transcode and a naive copy
// with re-encoded text would damage them.
//
// All five subsystems are now built and read these formats unchanged, so a
// byte-for-byte copy is the whole job as far as the *server* is concerned.
// What is still not done is the text inside them: converting that means
// decoding each record, transcoding its strings and rewriting the lengths
// that were stored beside them, which is a per-format job rather than
// anything a directory-level converter can do. They are carried across
// untouched and reported so that is visible rather than assumed.
var binaryFormats = []struct {
	// prefix matches the start of the path, suffix its end. An empty suffix
	// matches anything. Matching on both matters: plrobjs/ and house/ also
	// hold READMEs and .gitkeep files, which are ordinary text and were
	// briefly being reported as struct dumps because the directory alone was
	// enough to classify them.
	prefix, suffix, note string
}{
	{prefix: "etc/board.", note: "message board: a struct dump with length-prefixed text; read as-is, but its text is not transcoded"},
	{prefix: "etc/plrmail", note: "player mail: a block-allocated struct dump; read as-is, but its text is not transcoded"},
	{prefix: "etc/hcontrol", note: "house control: a struct dump; read as-is, and holds no text to transcode"},
	{prefix: "plrobjs/", suffix: ".objs", note: "player rent file: a struct dump; read as-is, and holds no text to transcode"},
	{prefix: "house/", suffix: ".house", note: "house contents: a struct dump; read as-is, and holds no text to transcode"},
}

// unsupportedNote returns the reason a path is left alone, or "".
func unsupportedNote(rel string) string {
	slashed := filepath.ToSlash(rel)
	for _, f := range binaryFormats {
		if !strings.HasPrefix(slashed, f.prefix) {
			continue
		}
		if f.suffix != "" && !strings.HasSuffix(slashed, f.suffix) {
			continue
		}
		return f.note
	}
	return ""
}

// Run performs a conversion.
func Run(ctx context.Context, opts Options) (*Report, error) {
	if opts.From == "" || opts.To == "" {
		return nil, errors.New("convert: both a source and a destination are required")
	}
	from, err := filepath.Abs(opts.From)
	if err != nil {
		return nil, err
	}
	to, err := filepath.Abs(opts.To)
	if err != nil {
		return nil, err
	}
	if from == to {
		return nil, errors.New("convert: the source and destination are the same directory; " +
			"converting in place would leave it half-converted if anything failed")
	}
	if strings.HasPrefix(to, from+string(filepath.Separator)) {
		return nil, fmt.Errorf("convert: the destination %s is inside the source", opts.To)
	}
	if opts.Encoding == nil {
		opts.Encoding = Encodings[DefaultEncoding]
	}

	if err := checkDestination(to, opts); err != nil {
		return nil, err
	}

	r := &Report{}

	// The player database is a whole-format change rather than a per-file
	// one, so it is handled before the walk and excluded from it.
	playerSrc, err := convertPlayers(ctx, from, to, opts, r)
	if err != nil {
		return r, err
	}

	err = filepath.Walk(from, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(from, path)
		if err != nil {
			return err
		}
		if path == playerSrc {
			return nil // already handled
		}
		return convertFile(path, filepath.Join(to, rel), rel, opts, r)
	})
	if err != nil {
		return r, err
	}

	sort.Slice(r.Entries, func(i, j int) bool { return r.Entries[i].Path < r.Entries[j].Path })
	return r, nil
}

// checkDestination refuses to write into a directory that already has
// something in it, unless told to.
func checkDestination(to string, opts Options) error {
	entries, err := os.ReadDir(to)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(entries) > 0 && !opts.Force && !opts.DryRun {
		return fmt.Errorf("convert: %s is not empty; use --force to write into it anyway", to)
	}
	return nil
}

// convertPlayers rewrites the binary player database as ascii pfiles. It
// returns the source path it consumed, so the file walk can skip it.
func convertPlayers(ctx context.Context, from, to string, opts Options, r *Report) (string, error) {
	src := filepath.Join(from, "etc", binary.FileName)
	if _, err := os.Stat(src); errors.Is(err, os.ErrNotExist) {
		// No player database is the normal state of a fresh install, and of
		// this repository, which ships none deliberately.
		return "", nil
	} else if err != nil {
		return "", err
	}

	rel := filepath.Join("etc", binary.FileName)
	if opts.DryRun {
		r.Entries = append(r.Entries, Entry{
			Path: rel, Action: Reformatted,
			Note: "binary player database -> ascii pfiles in pfiles/",
		})
		return src, nil
	}

	in, err := binary.New(player.Config{Dir: filepath.Join(from, "etc"), ReadOnly: true})
	if err != nil {
		return src, err
	}
	defer func() { _ = in.Close() }()

	out, err := ascii.New(player.Config{Dir: filepath.Join(to, "pfiles")})
	if err != nil {
		return src, err
	}
	defer func() { _ = out.Close() }()

	moved := 0
	for entry, err := range in.List(ctx) {
		if err != nil {
			r.Problems = append(r.Problems, fmt.Sprintf("%s: %v", rel, err))
			break
		}
		rec, err := in.Load(ctx, entry.Name)
		if err != nil {
			r.Problems = append(r.Problems, fmt.Sprintf("%s: %s: %v", rel, entry.Name, err))
			continue
		}
		// Player text is in the same old encoding as everything else.
		rec.Name = transcodeString(rec.Name, opts.Encoding)
		rec.Title = transcodeString(rec.Title, opts.Encoding)
		rec.Description = transcodeString(rec.Description, opts.Encoding)
		rec.Host = transcodeString(rec.Host, opts.Encoding)

		if err := out.Save(ctx, rec); err != nil {
			r.Problems = append(r.Problems, fmt.Sprintf("%s: %s: %v", rel, rec.Name, err))
			continue
		}
		moved++
	}

	r.Entries = append(r.Entries, Entry{
		Path: rel, Action: Reformatted,
		Note: fmt.Sprintf("binary player database -> %d character(s) as ascii pfiles in pfiles/", moved),
	})
	return src, nil
}

// convertFile handles one ordinary file.
func convertFile(src, dst, rel string, opts Options, r *Report) error {
	data, err := os.ReadFile(src) //nolint:gosec // walking an operator-named directory
	if err != nil {
		return err
	}

	action, out, note := classify(rel, data, opts.Encoding)

	if !opts.DryRun {
		if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
			return err
		}
		if err := writeFile(dst, out, modeFor(rel)); err != nil {
			return err
		}
	}

	r.Entries = append(r.Entries, Entry{Path: rel, Action: action, Note: note})
	return nil
}

// classify decides what to do with a file's contents.
func classify(rel string, data []byte, enc *charmap.Charmap) (Action, []byte, string) {
	if note := unsupportedNote(rel); note != "" {
		return Unsupported, data, note
	}
	if utf8.Valid(data) {
		// Already UTF-8, which every pure-ASCII file also is. Nothing to do.
		return Copied, data, ""
	}

	converted, err := enc.NewDecoder().Bytes(data)
	if err != nil {
		// The decoders here map every byte to some rune, so this should not
		// happen; report rather than silently pass the file through.
		return Copied, data, fmt.Sprintf("could not decode as %s: %v", encodingName(enc), err)
	}
	return Transcoded, converted, fmt.Sprintf("%s -> UTF-8", encodingName(enc))
}

// transcodeString converts one string if it is not already UTF-8.
func transcodeString(s string, enc *charmap.Charmap) string {
	if utf8.ValidString(s) {
		return s
	}
	out, err := enc.NewDecoder().String(s)
	if err != nil {
		return s
	}
	return out
}

func encodingName(enc *charmap.Charmap) string {
	for name, c := range Encodings {
		if c == enc {
			return name
		}
	}
	return "the source encoding"
}

// modeFor returns the permissions a converted file should have. Player data
// is the only thing here that is anyone's business but the server's.
func modeFor(rel string) os.FileMode {
	slashed := filepath.ToSlash(rel)
	switch {
	case strings.Contains(slashed, "etc/players"),
		strings.Contains(slashed, "etc/plrmail"),
		strings.HasPrefix(slashed, "pfiles/"),
		strings.HasPrefix(slashed, "plrobjs/"):
		return 0o600
	}
	return 0o640
}

// writeFile writes via a temporary file and a rename.
func writeFile(path string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+"-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(name)
	}()

	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(name, mode); err != nil {
		return err
	}
	return os.Rename(name, path)
}
