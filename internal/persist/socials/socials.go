// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

// Package socials reads and writes the do_action table, misc/socials (the
// C's SOCMESS_FILE), porting boot_social_messages (act.social.c:216) for
// classic and docs/design/data-format.md §7's config/socials.yaml for
// yaml.
//
// Like internal/persist/messages and internal/persist/names, this is
// read-only game data at runtime — nothing in the C ever writes
// misc/socials while playing — so there is no Store interface with a
// runtime-mutation method to design, only a list to load and (for
// `dlctl import --type=socials`/`fmt`) to write back out.
package socials

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/goccy/go-yaml"

	"github.com/gerrowadat/disgracelands/internal/persist/yamlenc"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// ClassicFile is misc/socials under whatever directory holds it.
const ClassicFile = "socials"

// YamlFile is config/socials.yaml under whatever directory Load/Save's
// path names, per docs/design/data-format.md §7.
const YamlFile = "socials.yaml"

// socialsSchema is the document's schema tag (data-format.md §10.1).
const socialsSchema = "dl/socials@1"

type doc struct {
	Schema  string      `yaml:"schema"`
	Socials []recordDoc `yaml:"socials,omitempty"`
}

type recordDoc struct {
	Command string `yaml:"command"`
	// Hide and MinVictimPosition are always present in the classic file —
	// there is no "#" spelling for either — so neither is omitempty,
	// matching messages.go's AttackType field for the same reason.
	Hide bool `yaml:"hide"`
	// MinVictimPosition is a symbolic name (game.YamlPositionNames via
	// game.NameByValue/ValueByName), the same table the world format
	// already uses for a mobile's position/default_position.
	MinVictimPosition string `yaml:"min_victim_position"`

	// NoArg is always read from the file (the first two message lines,
	// CharNoArg/OthersNoArg), but either or both may be "#" — an empty
	// audienceDoc is nil, the same omission convention msgSetDoc uses in
	// internal/persist/messages.
	NoArg *audienceDoc `yaml:"no_arg,omitempty"`
	// Found and everything after it (NotFound, Self) are only in the file
	// at all when the social takes a target (Social.TakesTarget(), i.e.
	// CharFound != ""). When it does not, Found, NotFound and Self are
	// each already all-empty, so they omit themselves with no extra
	// presence check needed.
	Found    *foundDoc    `yaml:"found,omitempty"`
	NotFound string       `yaml:"not_found,omitempty"`
	Self     *audienceDoc `yaml:"self,omitempty"`
}

// audienceDoc is a message pair with no victim line — used for both
// no_arg (CharNoArg/OthersNoArg) and self (CharAuto/OthersAuto), which
// share the same shape in game.Social.
type audienceDoc struct {
	Char   string `yaml:"char,omitempty"`
	Others string `yaml:"others,omitempty"`
}

// foundDoc is the three found-with-a-target lines: CharFound, OthersFound,
// VictFound.
type foundDoc struct {
	Char   string `yaml:"char,omitempty"`
	Others string `yaml:"others,omitempty"`
	Victim string `yaml:"victim,omitempty"`
}

// Load reads the socials table in the given format ("classic" or
// "yaml", "" meaning classic). For classic, path is the file itself
// (.../misc/socials); for yaml, path is the directory config/ lives
// under — the same asymmetry internal/persist/messages already has.
//
// A missing file is not an error: boot_social_messages' own C caller
// treats a missing SOCMESS_FILE as fatal (exit(1)), but this port's
// posture throughout has been that a server with no optional data is a
// poorer game, not a broken one — the same choice internal/server/text.go
// already made when it first loaded this file directly, which this
// package now does on its behalf.
func Load(format, path string) ([]game.Social, error) {
	switch format {
	case "", "classic":
		return loadClassic(path)
	case "yaml":
		return loadYaml(path)
	default:
		return nil, fmt.Errorf("socials: unknown format %q", format)
	}
}

// Save writes the socials table in the given format. Only yaml is
// implemented, for the same reason internal/persist/messages.Save only
// writes yaml: the C never writes misc/socials at runtime, so a
// classic writer has nothing to serve except `dlctl`, which only ever
// needs to go classic to yaml, never back.
func Save(format, path string, list []game.Social) error {
	if format != "yaml" {
		return fmt.Errorf("socials: writing %q is not supported (only yaml)", format)
	}
	return saveYaml(path, list)
}

func loadClassic(path string) ([]game.Social, error) {
	f, err := os.Open(path) //nolint:gosec // operator-configured path
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	list, err := game.ParseSocials(f)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return list, nil
}

func loadYaml(dir string) ([]game.Social, error) {
	path := filepath.Join(dir, YamlFile)
	b, err := os.ReadFile(path) //nolint:gosec // operator-configured path
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	var d doc
	if err := yaml.UnmarshalWithOptions(b, &d, yaml.Strict()); err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	list := make([]game.Social, 0, len(d.Socials))
	for _, rd := range d.Socials {
		pos, ok := game.ValueByNameOrNumber(rd.MinVictimPosition, game.YamlPositionNames())
		if !ok {
			return nil, fmt.Errorf("%s: %q: unknown min_victim_position %q", path, rd.Command, rd.MinVictimPosition)
		}
		s := game.Social{
			Name:              rd.Command,
			Hide:              rd.Hide,
			MinVictimPosition: game.Position(pos),
			NotFound:          rd.NotFound,
		}
		if rd.NoArg != nil {
			s.CharNoArg, s.OthersNoArg = rd.NoArg.Char, rd.NoArg.Others
		}
		if rd.Found != nil {
			s.CharFound, s.OthersFound, s.VictFound = rd.Found.Char, rd.Found.Others, rd.Found.Victim
		}
		if rd.Self != nil {
			s.CharAuto, s.OthersAuto = rd.Self.Char, rd.Self.Others
		}
		list = append(list, s)
	}
	return list, nil
}

func saveYaml(dir string, list []game.Social) error {
	path := filepath.Join(dir, YamlFile)
	d := doc{Schema: socialsSchema, Socials: make([]recordDoc, 0, len(list))}
	for _, s := range list {
		d.Socials = append(d.Socials, recordDoc{
			Command:           s.Name,
			Hide:              s.Hide,
			MinVictimPosition: positionName(s.MinVictimPosition),
			NoArg:             audienceToDoc(s.CharNoArg, s.OthersNoArg),
			Found:             foundToDoc(s.CharFound, s.OthersFound, s.VictFound),
			NotFound:          s.NotFound,
			Self:              audienceToDoc(s.CharAuto, s.OthersAuto),
		})
	}

	out, err := yaml.MarshalWithOptions(d, yamlenc.Options()...)
	if err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, out, 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

// positionName names a Position for the yaml format, falling back to
// game.NameOrNumber's "#N" for a value YamlPositionNames does not cover.
//
// It used to write "unknown-N", which produced a document that would not
// load: the reader looks the name up in the same table and does not find
// that either. ParseSocials bounds-checks nothing, so an out-of-range
// position is something the classic parser really can hand over.
func positionName(pos game.Position) string {
	return game.NameOrNumber(int32(pos), game.YamlPositionNames())
}

func audienceToDoc(char, others string) *audienceDoc {
	if char == "" && others == "" {
		return nil
	}
	return &audienceDoc{Char: char, Others: others}
}

func foundToDoc(char, others, victim string) *foundDoc {
	if char == "" && others == "" && victim == "" {
		return nil
	}
	return &foundDoc{Char: char, Others: others, Victim: victim}
}
