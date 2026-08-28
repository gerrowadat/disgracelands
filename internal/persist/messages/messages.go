// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

// Package messages reads and writes the skill_message/dam_message table,
// misc/messages (db.h's MESS_FILE), porting load_messages (fight.c:145-
// 193) for classic and docs/design/data-format.md §9's
// config/messages.yaml for yaml.
//
// Like internal/persist/names, this is read-only game data at runtime —
// nothing in the C ever writes misc/messages while playing — so there is
// no Store interface with a runtime-mutation method to design, only a
// list to load and (for `dlctl import --type=messages`/`fmt`) to write back out.
package messages

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/goccy/go-yaml"

	"github.com/gerrowadat/disgracelands/internal/persist/yamlenc"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// ClassicFile is misc/messages under whatever directory holds it.
const ClassicFile = "messages"

// YamlFile is config/messages.yaml under whatever directory Load/Save's
// path names, per docs/design/data-format.md §9.
const YamlFile = "messages.yaml"

// messagesSchema is the document's schema tag (data-format.md §10.1).
const messagesSchema = "dl/messages@1"

type doc struct {
	Schema   string      `yaml:"schema"`
	Messages []recordDoc `yaml:"messages,omitempty"`
}

type recordDoc struct {
	// AttackType is a symbolic name (game.AttackTypeName) where one
	// exists, "#N" otherwise — the same round-trip-losslessly convention
	// every other numbered field in this format already uses.
	AttackType string     `yaml:"attack_type"`
	Die        *msgSetDoc `yaml:"die,omitempty"`
	Miss       *msgSetDoc `yaml:"miss,omitempty"`
	Hit        *msgSetDoc `yaml:"hit,omitempty"`
	God        *msgSetDoc `yaml:"god,omitempty"`
}

// msgSetDoc is game.MsgSet. A nil *msgSetDoc is the classic format's `#`
// for all three lines at once; each field within one that classic left as
// `#` is its own omitempty rather than an empty string written out.
type msgSetDoc struct {
	Attacker string `yaml:"attacker,omitempty"`
	Victim   string `yaml:"victim,omitempty"`
	Room     string `yaml:"room,omitempty"`
}

// Load reads the fight-message table in the given format ("classic" or
// "yaml", "" meaning classic). For classic, path is the file itself
// (.../misc/messages); for yaml, path is the directory config/ lives
// under — the same asymmetry internal/persist/names already has.
//
// A missing file is not an error: load_messages' own C caller treats a
// missing MESS_FILE as fatal (exit(1)), but this port's posture
// throughout has been that a server with no optional data is a poorer
// game, not a broken one — the same choice internal/server/text.go
// already made when it first loaded this file directly, which this
// package now does on its behalf.
func Load(format, path string) ([]game.FightMessage, error) {
	switch format {
	case "", "classic":
		return loadClassic(path)
	case "yaml":
		return loadYaml(path)
	default:
		return nil, fmt.Errorf("messages: unknown format %q", format)
	}
}

// Save writes the fight-message table in the given format. Only yaml is
// implemented, for the same reason internal/persist/names.Save only
// writes yaml: the C never writes misc/messages at runtime, so a
// classic writer has nothing to serve except `dlctl`, which only ever
// needs to go classic to yaml, never back.
func Save(format, path string, records []game.FightMessage) error {
	if format != "yaml" {
		return fmt.Errorf("messages: writing %q is not supported (only yaml)", format)
	}
	return saveYaml(path, records)
}

func loadClassic(path string) ([]game.FightMessage, error) {
	f, err := os.Open(path) //nolint:gosec // operator-configured path
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	records, err := game.ParseMessagesFile(f)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return records, nil
}

func loadYaml(dir string) ([]game.FightMessage, error) {
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

	records := make([]game.FightMessage, 0, len(d.Messages))
	for _, rd := range d.Messages {
		attackType, ok := game.AttackTypeFromName(rd.AttackType)
		if !ok {
			return nil, fmt.Errorf("%s: %q: unknown attack type", path, rd.AttackType)
		}
		records = append(records, game.FightMessage{
			AttackType: attackType,
			Die:        msgSetFromDoc(rd.Die),
			Miss:       msgSetFromDoc(rd.Miss),
			Hit:        msgSetFromDoc(rd.Hit),
			God:        msgSetFromDoc(rd.God),
		})
	}
	return records, nil
}

func saveYaml(dir string, records []game.FightMessage) error {
	path := filepath.Join(dir, YamlFile)
	d := doc{Schema: messagesSchema, Messages: make([]recordDoc, 0, len(records))}
	for _, r := range records {
		d.Messages = append(d.Messages, recordDoc{
			AttackType: game.AttackTypeName(r.AttackType),
			Die:        msgSetToDoc(r.Die),
			Miss:       msgSetToDoc(r.Miss),
			Hit:        msgSetToDoc(r.Hit),
			God:        msgSetToDoc(r.God),
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

func msgSetToDoc(m game.MsgSet) *msgSetDoc {
	if m.Attacker == "" && m.Victim == "" && m.Room == "" {
		return nil
	}
	return &msgSetDoc{Attacker: m.Attacker, Victim: m.Victim, Room: m.Room}
}

func msgSetFromDoc(d *msgSetDoc) game.MsgSet {
	if d == nil {
		return game.MsgSet{}
	}
	return game.MsgSet{Attacker: d.Attacker, Victim: d.Victim, Room: d.Room}
}
