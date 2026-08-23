// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package session

import (
	"fmt"
	"strings"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// teditField is one row of do_tedit's own local field table (tedit.c) —
// which file a name maps to and who may edit it. Kept in the C's own
// order, since the no-argument listing walks it in that order too.
//
// MaxLength matches the C's own per-field buffer size: 2400 for the
// shorter files, 8192 for the longer ones.
type teditField struct {
	Name      string
	Level     int32
	MaxLength int
}

var teditFieldTable = []teditField{
	{"credits", game.LevelImplementor, 2400},
	{"news", game.LevelGreaterGod, 8192},
	{"motd", game.LevelGreaterGod, 2400},
	{"imotd", game.LevelImplementor, 2400},
	{"help", game.LevelGreaterGod, 2400},
	{"info", game.LevelGreaterGod, 8192},
	{"background", game.LevelImplementor, 8192},
	{"handbook", game.LevelImplementor, 8192},
	{"policies", game.LevelImplementor, 8192},
}

// teditLookup finds the first field a typed word is a prefix of,
// porting do_tedit's own `strncmp(field, fields[l].cmd, strlen(field))`
// loop — a case-sensitive prefix match against the C's own table order,
// not the case-insensitive matching most of this port's other lookups
// use, because that is what tedit.c actually does.
func teditLookup(field string) (teditField, bool) {
	for _, f := range teditFieldTable {
		if strings.HasPrefix(f.Name, field) {
			return f, true
		}
	}
	return teditField{}, false
}

// doTedit ports do_tedit (tedit.c). With no argument it lists whichever
// of the nine fields the caller's own level can reach; given a
// recognised, permitted name it shows the file's current content and
// starts the line editor seeded with it — string_write's own behaviour
// when the pointer it is handed already points at something.
//
// The C's own instructions line ("/s or @ to save, /h for more
// options.") describes the improved editor's `/`-commands, which this
// port does not implement (see docs/deviations.md's "gaps still to
// fill" — a previously-undocumented finding from this pass, affecting
// every editor in this port, not just this one). Printing that line
// here would promise commands that do not work, so this uses the C's
// own plain-editor text instead ("Type @ on a line by itself to end."),
// which is what this port's line editor actually does.
func doTedit(c *Context) error {
	field := strings.TrimSpace(c.Arg)
	level := c.Character.Level()

	if field == "" {
		return teditList(c, level)
	}

	spec, ok := teditLookup(field)
	if !ok {
		c.Send("Invalid text editor option.\r\n")
		return nil
	}
	if level < spec.Level {
		c.Send("You are not godly enough for that!\r\n")
		return nil
	}
	if c.TextEdit == nil {
		return nil
	}
	current, _ := c.TextEdit.TextField(spec.Name)

	c.Send("Instructions: Type @ on a line by itself to end.\r\n")
	c.Send("Edit file below:\r\n\r\n")
	if current != "" {
		c.Send("%s", current)
	}
	c.announce("%s begins editing a scroll.\r\n", c.Character.Name)

	name, editor := spec.Name, c.TextEdit
	c.Session.beginEditorSeeded(spec.MaxLength, current, func(text string) {
		editor.SetTextField(name, text)
		// "Saved.\r\n" is tedit_string_cleanup's own confirmation
		// (modify.c's STRINGADD_SAVE case), not the generic editor's —
		// board write/mail print nothing here because the C never did
		// for them either.
		c.Send("Saved.\r\n")
	})
	return nil
}

// teditList is do_tedit's own no-argument listing: every field the
// caller's level reaches, padded to 11 characters and wrapped seven to
// a line — porting the C's own grid, not a nicer one.
func teditList(c *Context, level int32) error {
	var names []string
	for _, f := range teditFieldTable {
		if level >= f.Level {
			names = append(names, f.Name)
		}
	}
	if len(names) == 0 {
		c.Send("Files available to be edited:\r\nNone.\r\n")
		return nil
	}

	var b strings.Builder
	b.WriteString("Files available to be edited:\r\n")
	for i, name := range names {
		fmt.Fprintf(&b, "%-11.11s", name)
		if (i+1)%7 == 0 {
			b.WriteString("\r\n")
		}
	}
	if len(names)%7 != 0 {
		b.WriteString("\r\n")
	}
	c.Send("%s", b.String())
	return nil
}
