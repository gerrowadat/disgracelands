// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package session

import (
	"strings"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// do_alias and perform_alias/perform_complex_alias, ported from
// interpreter.c:693-845.
//
// No archived character has ever defined an alias — data/ has no plralias
// directory at all, so this is a new feature riding along with the yaml
// player format rather than a port of existing player data (see
// docs/design/go-port-plan.md's players section). The C's own behaviour
// is still the specification, byte for byte, including the two quirks that
// are easy to miss reading it once: a simple alias's replacement keeps
// whatever leading space any_one_arg left between the alias name and the
// rest of the line (see anyOneArg), and $* substitutes that same raw,
// untrimmed remainder while $1-$9 substitute space-collapsed tokens of it
// — two different views of the same text, both faithfully reproduced.

// aliasMaxTokens is NUM_TOKENS (interpreter.c:753): only $1-$9 are valid
// positional substitutions in a complex alias, and strtok stops collecting
// tokens once it has this many regardless of how much text is left.
const aliasMaxTokens = 9

// doAlias is do_alias (interpreter.c:694-745).
func doAlias(cmd *Context) error {
	if cmd.Character.IsNPC() || cmd.Character.Record == nil {
		return nil
	}
	rec := cmd.Character.Record

	name, repl := anyOneArg(cmd.Arg)

	if name == "" {
		// No argument: list what is defined, in list order — which is
		// newest-first, matching GET_ALIASES' prepend-on-add.
		cmd.Send("Currently defined aliases:\r\n")
		if len(rec.Aliases) == 0 {
			cmd.Send(" None.\r\n")
			return nil
		}
		for _, a := range rec.Aliases {
			cmd.Send("%-15s %s\r\n", a.Name, a.Replacement)
		}
		return nil
	}

	// Redefining or deleting an existing alias removes the old entry first
	// — find_alias + REMOVE_FROM_LIST (interpreter.c:717-720). a stays
	// non-nil past this point iff there was one to remove, which is what
	// tells "no replacement given" apart into "deleted" vs. "no such alias".
	_, hadExisting := findAlias(rec.Aliases, name)
	if hadExisting {
		rec.Aliases = removeAlias(rec.Aliases, name)
	}

	if repl == "" {
		if hadExisting {
			cmd.Send("Alias deleted.\r\n")
		} else {
			cmd.Send("No such alias.\r\n")
		}
		return nil
	}

	if name == "alias" {
		cmd.Send("You can't alias 'alias'.\r\n")
		return nil
	}

	// No delete_doubledollar (interpreter.c:734), and that is the whole of
	// this port's answer to the C's `$` handling. See docs/deviations.md.
	//
	// The C collapses "$$" to "$" here because process_input doubled every
	// `$` in the line on the way in off the socket, so that player text
	// could not later be read as an act() code (comm.c:1806-1810). This
	// port does not double — nothing here interpolates player text into an
	// act() format, the two places that did having been fixed in #238 —
	// and so must not collapse either. Doing one without the other is what
	// ate a character: `alias gc $$foo` stored `$foo`, where the C stored
	// `$$foo` and showed `$$foo`.
	rec.Aliases = append([]game.Alias{{Name: name, Replacement: repl}}, rec.Aliases...)
	cmd.Send("Alias added.\r\n")
	return nil
}

// findAlias is find_alias (interpreter.c:669-679): the first alias in the
// list whose name matches, walked in list order. Names are always already
// lowercased (anyOneArg lowercases the word it extracts, the same way
// any_one_arg's LOWER() does, and doAlias only ever stores what anyOneArg
// returned), so this is a plain comparison.
func findAlias(aliases []game.Alias, name string) (game.Alias, bool) {
	for _, a := range aliases {
		if a.Name == name {
			return a, true
		}
	}
	return game.Alias{}, false
}

// removeAlias drops the first alias named name — REMOVE_FROM_LIST
// (interpreter.c:718), which unlinks exactly the node find_alias found.
func removeAlias(aliases []game.Alias, name string) []game.Alias {
	for i, a := range aliases {
		if a.Name == name {
			out := make([]game.Alias, 0, len(aliases)-1)
			out = append(out, aliases[:i]...)
			return append(out, aliases[i+1:]...)
		}
	}
	return aliases
}

// isComplexAlias reports whether a replacement needs perform_complex_alias
// rather than a straight rewrite — interpreter.c:736-738: the presence of
// either ALIAS_SEP_CHAR (';') or ALIAS_VAR_CHAR ('$') decides it once, at
// definition time, rather than being re-derived from context each time the
// alias runs.
func isComplexAlias(replacement string) bool {
	return strings.ContainsAny(replacement, ";$")
}

// ExpandAlias is perform_alias (interpreter.c:815-844), minus the
// descriptor's input-queue plumbing: the C pushes a complex alias's
// commands to the *front* of the read queue and runs them one per game-loop
// pulse; this returns them as an ordered slice instead; and the caller
// (session.go's readLoop) runs each one before reading anything further off
// the socket, which comes to the same observable thing. Mobiles never reach
// this at all — see game.PlayerRecord.Aliases' doc comment — because only a
// live player's own typed line is ever offered to it.
//
// matched is false when line's first word does not name a defined alias,
// in which case the caller must run line completely unchanged, exactly as
// perform_alias returning 0 with orig untouched does. This also means an
// alias's own expansion is never re-expanded: the caller runs each returned
// command directly rather than passing it back through ExpandAlias, which is
// the same recursion guard the C's "aliased" flag provides
// (comm.c:798-803).
func ExpandAlias(aliases []game.Alias, line string) (commands []string, matched bool) {
	if len(aliases) == 0 {
		return nil, false
	}
	name, rest := anyOneArg(line)
	if name == "" {
		return nil, false
	}
	a, ok := findAlias(aliases, name)
	if !ok {
		return nil, false
	}
	if !isComplexAlias(a.Replacement) {
		return []string{a.Replacement}, true
	}
	return expandComplexAlias(rest, a.Replacement), true
}

// expandAliasedLine is where perform_alias is actually reached from
// (comm.c:797-803): only while playing, and only for a line read straight
// off the socket — never for one of a previous alias's own expanded
// commands, since session.go's readLoop is the only caller and it runs each
// returned command directly rather than feeding it back through here. That
// asymmetry is the recursion guard the C's "aliased" flag provides.
//
// A line that names no alias, or that arrives before there is a character
// or aliases to check, comes back as itself, unchanged — matching
// perform_alias leaving orig untouched and returning 0.
func (s *Session) expandAliasedLine(text string) []string {
	if s.State() != StatePlaying {
		return []string{text}
	}
	c := s.Character()
	if c == nil || c.IsNPC() || c.Record == nil || len(c.Record.Aliases) == 0 {
		return []string{text}
	}
	if commands, ok := ExpandAlias(c.Record.Aliases, text); ok {
		return commands
	}
	return []string{text}
}

// anyOneArg is any_one_arg (interpreter.c:1023-1035): the first
// whitespace-delimited word, lowercased, and the rest of the string exactly
// as it was — including whatever whitespace immediately followed the word.
// Unlike this package's split (commands.go), the remainder is *not*
// trimmed: perform_complex_alias's $* substitutes it byte for byte,
// leading space and all, which is the one place that difference is
// observable.
func anyOneArg(s string) (word, rest string) {
	i := 0
	for i < len(s) && isASCIISpace(s[i]) {
		i++
	}
	start := i
	for i < len(s) && !isASCIISpace(s[i]) {
		i++
	}
	return strings.ToLower(s[start:i]), s[i:]
}

// isASCIISpace matches isspace() under the C locale closely enough for a
// single already-CRLF-stripped input line: space, tab, and the other
// classic whitespace bytes it also accepts.
func isASCIISpace(b byte) bool {
	switch b {
	case ' ', '\t', '\n', '\v', '\f', '\r':
		return true
	}
	return false
}

// strtokSpace splits s on single spaces, dropping empty tokens exactly the
// way strtok(s, " ") does with repeated delimiters, and stops after max —
// matching perform_complex_alias's own loop, which quits calling strtok
// once num_of_tokens reaches NUM_TOKENS regardless of how much text is
// left unread (interpreter.c:764-768).
func strtokSpace(s string, max int) []string {
	var tokens []string
	for _, tok := range strings.Split(s, " ") {
		if tok == "" {
			continue
		}
		tokens = append(tokens, tok)
		if len(tokens) == max {
			break
		}
	}
	return tokens
}

// expandComplexAlias is perform_complex_alias's substitution loop
// (interpreter.c:772-799), reading replacement byte by byte:
//
//   - ';' (ALIAS_SEP_CHAR) ends the current command and starts the next.
//   - '$' followed by '1'-'9' substitutes that token of rest, if there are
//     that many; '$*' (ALIAS_GLOB_CHAR) substitutes all of rest, untrimmed.
//   - '$' followed by anything else drops the '$' and keeps the one byte
//     after it — '$$' included, which is where this parts company with the
//     C and where the whole `$` question lands. The C writes *two* dollars
//     there, and its comment says why: "redouble $ for act safety"
//     (interpreter.c:794). That is one link in an escaping chain — doubled
//     on the way in off the socket, redoubled here, halved again by act()
//     or delete_doubledollar at the far end — and this port has never had
//     the first link (docs/deviations.md, "A typed `$` is left alone").
//     Redoubling without it left the escape with nothing to unescape it:
//     `alias cash gecho $$$1` then `cash 100` printed "$$100" where the C
//     prints "$100". One dollar in, one dollar out, all the way through.
//   - A '$' as the very last byte of replacement contributes nothing: the C
//     writes a stray NUL there that its own terminator immediately
//     overwrites (interpreter.c:790, ptr walks one past the string), which
//     has no archived alias to reproduce and isn't a template anyone meant.
//
// Each ';'-delimited piece becomes one element of the returned slice, in
// the order they appear in replacement.
func expandComplexAlias(rest, replacement string) []string {
	tokens := strtokSpace(rest, aliasMaxTokens)

	var commands []string
	var buf strings.Builder
	for i := 0; i < len(replacement); {
		switch ch := replacement[i]; ch {
		case ';':
			commands = append(commands, buf.String())
			buf.Reset()
			i++
		case '$':
			if i+1 >= len(replacement) {
				i++
				continue
			}
			switch next := replacement[i+1]; {
			case next >= '1' && next <= '9' && int(next-'1') < len(tokens):
				buf.WriteString(tokens[next-'1'])
			case next == '*':
				buf.WriteString(rest)
			default:
				// '$$' falls in here too, and writes one dollar. The C
				// has the same shape — its default branch writes the
				// byte and then adds a second one *if* it was a '$' —
				// and it is that second one this port does not want.
				// See the doc comment above.
				buf.WriteByte(next)
			}
			i += 2
		default:
			buf.WriteByte(ch)
			i++
		}
	}
	commands = append(commands, buf.String())
	return commands
}
