// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import (
	"os"
	"strings"
	"testing"
)

// actPeople builds an actor and a victim with known pronouns.
func actPeople() (actor, victim *Character) {
	actor = newCharacter("Zod")
	actor.Record.Sex = SexMale
	victim = newCharacter("Welmar")
	victim.Record.Sex = SexFemale
	return actor, victim
}

// TestActSubstitutesEveryCode.
func TestActSubstitutesEveryCode(t *testing.T) {
	actor, victim := actPeople()
	sword := &Object{Keywords: "sword long", ShortDesc: "a long sword"}
	shield := &Object{Keywords: "shield", ShortDesc: "an iron shield"}

	args := ActArgs{Actor: actor, Obj: sword, Victim: victim, VictimObj: shield}

	for format, want := range map[string]string{
		"$n hits $N.": "Zod hits Welmar.\r\n",
		"$e hits $E.": "He hits she.\r\n",
		"$m and $M":   "Him and her\r\n",
		"$s and $S":   "His and her\r\n",
		"$p and $P":   "A long sword and an iron shield\r\n",
		// $o and $O are the *keyword*, not the short description: "sword",
		// not "a long sword".
		"$o and $O": "Sword and shield\r\n",
		// SANA reads the keyword list too, so `$A $O` is "a shield" even
		// though the object is "an iron shield".
		"$a $o, $A $O":      "A sword, a shield\r\n",
		"the cost is 5$$.":  "The cost is 5$.\r\n",
		"$n drops $p here.": "Zod drops a long sword here.\r\n",
	} {
		if got := Act(format, args, victim); got != want {
			t.Errorf("Act(%q) = %q, want %q", format, got, want)
		}
	}
}

// TestActCapitalisesTheWholeMessage, which the C does unconditionally at the
// end — so a message beginning with a name gets the name capitalised and one
// beginning with a word gets the word.
func TestActCapitalisesTheWholeMessage(t *testing.T) {
	actor, victim := actPeople()
	args := ActArgs{Actor: actor, Victim: victim}

	if got := Act("$e smiles.", args, victim); got != "He smiles.\r\n" {
		t.Errorf("got %q", got)
	}
	if got := Act("with a grin, $n leaves.", args, victim); !strings.HasPrefix(got, "With a grin") {
		t.Errorf("got %q", got)
	}
}

// TestActUppercaseCodes: $u takes the word already written, $U the next one.
func TestActUppercaseCodes(t *testing.T) {
	actor, victim := actPeople()
	args := ActArgs{Actor: actor, Victim: victim, Text: "some words here"}

	for format, want := range map[string]string{
		// $u upper-cases the word *already written*, and there is none — the
		// character before it is a space, so the C's scan stops immediately
		// and it does nothing at all.
		"the $u$e laughs.": "The he laughs.\r\n",
		// $U upper-cases the next character written.
		"and then $Uhello.": "And then Hello.\r\n",
		"$T and $F":         "Some words here and some\r\n",
		// $u comes *after* the word it acts on: it scans back to the start of
		// whatever has just been written and upper-cases that letter.
		"$e said hello$u": "He said Hello\r\n",
	} {
		if got := Act(format, args, victim); got != want {
			t.Errorf("Act(%q) = %q, want %q", format, got, want)
		}
	}
}

// TestActWithNothingToSubstitute. A social aimed at nobody has no victim, and
// a code for one must not panic — the C substitutes nothing and carries on.
func TestActWithNothingToSubstitute(t *testing.T) {
	actor, _ := actPeople()

	if got := Act("$n waves at $N.", ActArgs{Actor: actor}, actor); got != "Zod waves at .\r\n" {
		t.Errorf("got %q", got)
	}
	if got := Act("$n drops $p.", ActArgs{Actor: actor}, actor); got != "Zod drops .\r\n" {
		t.Errorf("got %q", got)
	}
}

// TestSocialsFileParses against the real data, which is the only copy of the
// format there is.
func TestSocialsFileParses(t *testing.T) {
	f, err := os.Open("../../data/misc/socials")
	if err != nil {
		t.Fatalf("opening the socials file: %v", err)
	}
	defer func() { _ = f.Close() }()

	list, err := ParseSocials(f)
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}
	if len(list) < 100 {
		t.Fatalf("parsed %d socials, expected about a hundred", len(list))
	}

	byName := map[string]Social{}
	for _, s := range list {
		if s.Name == "" {
			t.Error("a social with no name")
		}
		if _, dup := byName[s.Name]; dup {
			t.Errorf("%q appears twice", s.Name)
		}
		byName[s.Name] = s
	}

	// smile is the one everybody knows, and it uses every field.
	smile, ok := byName["smile"]
	if !ok {
		t.Fatal("no smile in the socials file")
	}
	for field, want := range map[string]string{
		"char_no_arg":   "You smile happily.",
		"others_no_arg": "$n smiles happily.",
		"char_found":    "You smile at $M.",
		"others_found":  "$n beams a smile at $N.",
		"vict_found":    "$n smiles at you.",
		"not_found":     "There's no one by that name around.",
		"char_auto":     "You smile at yourself.",
		"others_auto":   "$n smiles at $mself.",
	} {
		got := map[string]string{
			"char_no_arg": smile.CharNoArg, "others_no_arg": smile.OthersNoArg,
			"char_found": smile.CharFound, "others_found": smile.OthersFound,
			"vict_found": smile.VictFound, "not_found": smile.NotFound,
			"char_auto": smile.CharAuto, "others_auto": smile.OthersAuto,
		}[field]
		if got != want {
			t.Errorf("smile's %s is %q, want %q", field, got, want)
		}
	}
	if !smile.Hide {
		t.Error("smile's hide flag did not survive")
	}

	// applaud is the other shape: three lines and no target.
	applaud, ok := byName["applaud"]
	if !ok {
		t.Fatal("no applaud in the socials file")
	}
	if applaud.TakesTarget() {
		t.Error("applaud claims to take a target")
	}
	if applaud.CharNoArg != "Clap, clap, clap." {
		t.Errorf("applaud says %q", applaud.CharNoArg)
	}
}

// TestSocialsRenderThroughAct end to end: the file's codes, resolved for
// three different audiences.
func TestSocialsRenderThroughAct(t *testing.T) {
	f, err := os.Open("../../data/misc/socials")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	list, err := ParseSocials(f)
	if err != nil {
		t.Fatal(err)
	}

	var smile Social
	for _, s := range list {
		if s.Name == "smile" {
			smile = s
		}
	}

	actor, victim := actPeople()
	bystander := newCharacter("Cid")
	args := ActArgs{Actor: actor, Victim: victim}

	for _, tc := range []struct {
		format string
		to     *Character
		want   string
	}{
		{smile.CharFound, actor, "You smile at her.\r\n"},
		{smile.VictFound, victim, "Zod smiles at you.\r\n"},
		{smile.OthersFound, bystander, "Zod beams a smile at Welmar.\r\n"},
		{smile.OthersAuto, bystander, "Zod smiles at himself.\r\n"},
	} {
		if got := Act(tc.format, args, tc.to); got != tc.want {
			t.Errorf("%q = %q, want %q", tc.format, got, tc.want)
		}
	}
}
