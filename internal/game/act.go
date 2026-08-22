// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import "strings"

// act(), ported from perform_act (comm.c:2270).
//
// Every message in the game that mentions more than one person goes through
// this. `"$n gives $p to $N."` becomes "Zod gives a sword to Welmar." for a
// bystander, and the *same string* becomes "Zod gives a sword to you." for
// the person receiving it, because the codes are resolved against whoever is
// being told.
//
// Until now the port formatted these with `%s` and wrote each one out per
// audience. That works for a message the code owns and not at all for one
// that comes from a data file — the socials are 106 of those, and they use
// the whole code set.

// ActArgs are what a message can refer to: the C's `(ch, obj, vict_obj)`,
// with the union split into the three things it is actually ever used as.
type ActArgs struct {
	// Actor is `ch`: $n, $m, $s, $e.
	Actor *Character
	// Obj is `obj`: $o, $p, $a.
	Obj *Object
	// Victim is `vict_obj` read as a character: $N, $M, $S, $E.
	Victim *Character
	// VictimObj is `vict_obj` read as an object: $O, $P, $A.
	VictimObj *Object
	// Text is `vict_obj` read as a string: $T, $F.
	Text string
}

// Act renders one message for one audience, porting perform_act.
//
// The result has its first letter capitalised and ends in CRLF, both of which
// the C does unconditionally at the end of the function — so a message that
// starts with a `$n` gets the *name* capitalised, and one that starts with a
// lower-case word gets that.
//
// It hangs off Live because the `$n`/`$N`/`$o`/`$p` codes resolve through PERS
// and OBJS, and those ask CAN_SEE — which needs the world to know whether the
// audience's room is dark. Resolving *per audience* is the whole point of the
// routine, and visibility is the sharpest case of it: the same message names
// the actor to one bystander and calls them "someone" to another.
func (l *Live) Act(format string, args ActArgs, to *Character) string {
	var b strings.Builder
	upperNext := false

	// write appends a run of text, applying a pending $U.
	write := func(s string) {
		for _, r := range s {
			if upperNext && !isSpace(r) {
				b.WriteString(strings.ToUpper(string(r)))
				upperNext = false
				continue
			}
			b.WriteRune(r)
		}
	}

	runes := []rune(format)
	for i := 0; i < len(runes); i++ {
		if runes[i] != '$' {
			write(string(runes[i]))
			continue
		}
		i++
		if i >= len(runes) {
			break
		}

		switch runes[i] {
		case 'n':
			write(l.personName(args.Actor, to))
		case 'N':
			write(l.personName(args.Victim, to))
		case 'm':
			write(args.Actor.Objective())
		case 'M':
			write(args.Victim.Objective())
		case 's':
			write(args.Actor.Possessive())
		case 'S':
			write(args.Victim.Possessive())
		case 'e':
			write(args.Actor.Subject())
		case 'E':
			write(args.Victim.Subject())
		case 'o':
			write(objectKeyword(args.Obj))
		case 'O':
			write(objectKeyword(args.VictimObj))
		case 'p':
			write(l.objectName(args.Obj, to))
		case 'P':
			write(l.objectName(args.VictimObj, to))
		case 'a':
			write(articleFor(objectKeyword(args.Obj)))
		case 'A':
			write(articleFor(objectKeyword(args.VictimObj)))
		case 'T':
			write(args.Text)
		case 'F':
			write(firstWordOf(args.Text))
		case 'u':
			// Upper-case the first letter of the word already written.
			capitaliseLastWord(&b)
		case 'U':
			upperNext = true
		case '$':
			write("$")
		default:
			// The C logs a SYSERR and substitutes nothing. A bad code in a
			// social file should not stop the message.
		}
	}

	return capitaliseFirst(b.String()) + "\r\n"
}

// personName is PERS(ch, to): their name, or "someone" if the audience cannot
// see them.
func (l *Live) personName(who, to *Character) string { return l.Pers(who, to) }

// objectName is OBJS: the short description, or "something" unseen.
func (l *Live) objectName(o *Object, to *Character) string { return l.Objs(o, to) }

// objectKeyword is OBJN: the object's *first keyword* rather than its short
// description, which is why `$o` reads as "sword" where `$p` reads as "a long
// sword".
func objectKeyword(o *Object) string {
	if o == nil {
		return ""
	}
	if w := firstWordOf(o.Keywords); w != "" {
		return w
	}
	return o.Name()
}

// articleFor is SANA/AN: "a" or "an" by the first letter, with no regard for
// how the word is actually pronounced. "an unicorn" is possible and has
// always been possible.
func articleFor(s string) string {
	if s == "" {
		return "a"
	}
	if strings.ContainsRune("aeiouAEIOU", rune(s[0])) {
		return "an"
	}
	return "a"
}

// capitaliseLastWord implements $u: upper-case the first letter of the word
// most recently written.
func capitaliseLastWord(b *strings.Builder) {
	s := b.String()
	i := len(s)
	for i > 0 && !isSpace(rune(s[i-1])) {
		i--
	}
	if i == len(s) {
		return
	}
	b.Reset()
	b.WriteString(s[:i] + strings.ToUpper(s[i:i+1]) + s[i+1:])
}

func firstWordOf(s string) string {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

func isSpace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\r'
}
