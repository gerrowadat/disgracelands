# Partial matching: five rules, not one

**Question asked:** partial keyword matching should keep working the way it
did in the C server — does it?

**Answer:** mostly, and the one place it does not is `cast`.

"Partial keyword matching" sounds like one feature. In CircleMUD it is five
different rules living in four different functions, and they disagree with
each other on purpose. Two of them are not prefix matches at all. Getting
the taxonomy right is most of the work here, because the interesting bug —
§4 — is in the rule nobody thinks of as keyword matching.

Investigated 2026-08-30, against `reference/CircleMUD3-src/src` and the
port at `044ddc2`. Findings are in §4 (a real gap, now
[#355](https://github.com/gerrowadat/disgracelands/issues/355)) and §7 (a
comment that says the opposite of what its own function does).

---

## 1. The five rules

| # | Rule | C | Prefix? | What it decides |
|---|---|---|---|---|
| 1 | `isname` | `handler.c:56` | **No — whole word** | What names a mobile, object, exit or extra description |
| 2 | The interpreter's own loop | `interpreter.c:623` | Yes | What a typed command word means |
| 3 | `is_abbrev` | `interpreter.c:1057` | Yes | Ad-hoc keyword arguments (`house build`, `at`, `in`) |
| 4 | `find_skill_num` | `spell_parser.c` | **Yes, twice, differently** | What names a spell or skill |
| 5 | `search_block` | `interpreter.c:860` | Yes | A word against a fixed table (directions, `syslog` levels, genders) |

Rules 1 and 4 are the two that surprise people, and they surprise in
opposite directions: the one everybody calls "keyword matching" does *not*
match prefixes, and the one nobody thinks about matches them two different
ways at once.

---

## 2. Rule 1 — `isname` is whole-word, and it is right here

```c
if (!*curstr && !isalpha(*curname))
  return (1);
```

The match succeeds only where the typed string runs out **and** the keyword
underneath it has ended too. `get sword` picks up a long sword; `get swo`
does not, and never did on the real server.

This is the rule with the worst history in this project and the best
current state. It was ported as a prefix match for four phases, with a
comment claiming the C matched prefixes. Then, once fixed, it was wrong a
second time for a year in a subtler way — a keyword ends at any
**non-alphabetic** byte, not at whitespace, so the C matches `6` against a
keyword of `606`, and the oracle's whole corpus was letters and spaces,
over which the right rule and the wrong one cannot disagree (#277).

**Current state: correct.** `internal/game/carry.go`'s `matchesKeywords` is
the C's own two loops transliterated rather than a rule derived from them,
and `reference/tools/nameoracle.c` checks 1,456 pairings including digits,
punctuation, doubled and trailing spaces, and a namelist wrapped by
`fread_string`. Nothing in this investigation moved it.

The subtlety worth keeping in view, because it is what makes the function
un-simulatable: the two loops disagree about what separates keywords. The
inner one breaks on a literal space; the outer one skips a run of
*alphabetic* bytes and steps one past. So where a match may begin depends
on where the previous attempt gave up.

---

## 3. Rules 2 and 3 — command abbreviation is right, including the part that looks wrong

The interpreter's loop is a prefix match against the command table, taking
the first entry that matches:

```c
for (length = strlen(arg), cmd = 0; *cmd_info[cmd].command != '\n'; cmd++)
  if (!strncmp(cmd_info[cmd].command, arg, length))
    if (GET_LEVEL(ch) >= cmd_info[cmd].minimum_level)
      break;
```

Two things about it that matter and one that does not.

**The table's order is the behaviour.** `n` means `north` because `north`
is where it sits in `cmd_info[]`, not because of anything about the word.
The port keeps `Command.CLine` and sorts by it, and
`internal/session/argument_test.go` re-parses `interpreter.c` to check both
the order and each entry's line — so abbreviation behaviour is *derived*
from the C rather than asserted about it. Correct, and already the strongest
anchoring in the tree.

**The level is part of the match, not a check after it.** A command above
your level is skipped and matching carries on down the table, so
abbreviations mean different things to mortals and to gods. `lookupFor` in
`internal/session/commands.go` reproduces this and its doc comment explains
it.

**`strncmp` looks case-sensitive and is not**, which is worth writing down
because it is the kind of thing that gets "fixed". `any_one_arg` lowercases
as it copies (`*(first_arg++) = LOWER(*argument)`), so `arg` is already
lowercase before `strncmp` sees it. The port's `strings.ToLower` agrees.
The only difference is that `LOWER()` is ASCII-only where `strings.ToLower`
is not, and it cannot bite: command names are ASCII, so a non-ASCII word
prefix-matches nothing either way.

`is_abbrev` (rule 3) is a plain case-insensitive prefix match, false for an
empty prefix. `internal/session/input.go`'s `isAbbrevOf` is a faithful
port.

---

## 4. Rule 4 — `find_skill_num` has two branches and the port has one

**This is the finding.** `cast 'mag mis'` worked on the real server. It
does not work here.

```c
for (index = 1; index <= TOP_SPELL_DEFINE; index++) {
    if (is_abbrev(name, spell_info[index].name))
      return (index);

    ok = TRUE;
    temp  = any_one_arg(strcpy(tempbuf, spell_info[index].name), first);
    temp2 = any_one_arg(name, first2);
    while (*first && *first2 && ok) {
      if (!is_abbrev(first2, first))
	ok = FALSE;
      temp  = any_one_arg(temp,  first);
      temp2 = any_one_arg(temp2, first2);
    }

    if (ok && !*first2)
      return (index);
}
```

The first branch is the obvious one: the whole typed string against the
whole spell name, so `magic mis` finds *magic missile*. It is also the only
one this port has.

The second branch walks both strings **a word at a time** and requires each
typed word to be an abbreviation of the spell-name word in the same
position. That is what makes `mag mis` work, and `b h` for *burning hands*,
and `det inv` for *detect invisibility*. It is how anybody who played this
game actually cast a spell.

`internal/game/spell.go`'s `SpellNumberByName` is `strings.HasPrefix(info.Name, name)`
— the first branch and nothing else.

### 4.1 How wrong, measured

`reference/tools/skilloracle.c` is the C's three functions verbatim, taking
its name table on stdin so it can be fed the port's own spell table. Swept
over every per-word abbreviation of all 71 spell names the port carries —
every combination of prefixes of each word, 1,549 queries:

| | |
|---|---|
| Queries | 1,549 |
| The C finds, the port does not | **1,145** |
| The port finds, the C does not | 0 |
| Both find, different spell | 0 |

The gap is a clean subset: the port never returns a *wrong* spell, it
returns nothing where the C returns something. That is the best shape this
kind of bug can have, and it means fixing it cannot break a query that
works today.

A sample of what is refused: `mag mis`, `det inv`, `b h`, `a br` (*acid
breath*), `bur han`, `cure ser`, `rem poi`, `det ali`.

### 4.2 What it affects

`SpellNumberByName` has three callers: `cast` (`internal/session/cast.go`),
`practice` and `skillset`. `cast` is the one that matters — it is the most
frequently typed command a caster has, and the abbreviation is the whole
point of the quote syntax.

### 4.3 Two things about the second branch that are not obvious

Both are reasons this belongs in an oracle rather than in a paraphrase, and
both are in `skilloracle.c`'s header:

- **The loop stops when *either* string runs out** (`*first && *first2`),
  and the verdict is `ok && !*first2`. So a query with **fewer** words than
  the spell name matches — `cure` alone reaches *cure light* through this
  branch as well as through `is_abbrev` — while a query with **more** words
  than the name does not.
- **`any_one_arg` writes through its argument**, so the C hands it a
  `strcpy`'d buffer for the spell name but passes `name` itself for the
  query. A Go port has no such hazard and no reason to reproduce it, but a
  C-shaped transliteration that missed it would be quietly destructive.

### 4.4 One difference that is *not* a bug

`SpellNumberByName` prefers an exact name outright, then the lowest-numbered
prefix match; the C takes the first match in table order with no exact-match
preference. These disagree only where one spell's full name is a strict
prefix of another's. **No such pair exists** in the 71-name table — checked
— so the difference is unreachable. Worth leaving as it is: it makes
`SpellNameOrNumber`'s round trip exact by construction, which the yaml
format depends on.

---

## 5. Rule 5 — `search_block`, and a quirk nobody reproduced

```c
if (*arg == '!')
  return (-1);
for (l = 0; *(arg + l); l++)
  *(arg + l) = LOWER(*(arg + l));      /* in place -- mutates the caller's buffer */
...
if (!l)
  l = 1;                               /* "" matches the first entry */
for (i = 0; **(list + i) != '\n'; i++)
  if (!strncmp(arg, *(list + i), l))
    return (i);
```

Three behaviours beyond the prefix match: a leading `!` refuses outright, it
lowercases the caller's buffer in place, and an **empty argument matches the
first entry in the table**.

The port has no `search_block`; each table gets its own small parser —
`ParseDirection`, `ParseAnnounceLevel`, `colour.ParseLevel`, the `genders`
lookup in `do_set`. All are prefix matches taking the first entry in table
order, which is the C's behaviour. All of them **refuse an empty word**
where the C would return entry 0.

Checked whether that is reachable: it is not. Every C caller tokenises with
`one_argument` first and branches on an empty argument before calling
`search_block` — `do_look`'s `if (!*arg)`, `do_gen_tog`'s, `do_set`'s. The
`!` guard has the same status; it exists for a convention (`\r` as a first
character) the stock tables no longer use.

**Recorded rather than changed.** Refusing is the safer shape, it is
unreachable, and a port that matched the first table entry for an empty
string would be reproducing a workaround for a problem this tree does not
have. If it is ever wanted, it belongs in `docs/deviations.md` — it is not
there now, and arguably should be.

---

## 6. Rule 0 — what reaches the matcher in the first place

Not a matching rule, but it changes what every one of them sees, and it is
easy to attribute its effects to the wrong function.

`one_argument` loops `while (fill_word(begin))`, dropping `in`, `from`,
`with`, `the`, `on`, `at`, `to` (`interpreter.c:568`). So `get the sword`
reaches `isname` as `sword`, and a player who types `look at the sign` is
matching `sign`. The port has this as `fillWords` in
`internal/session/argument.go` and it is correct.

The consequence worth knowing: an object whose keyword list is literally
`the` cannot be referred to. That is the C's behaviour and not a bug to fix.

---

## 7. A comment that says the opposite of its own function

`internal/game/object.go`:

```go
// Matches reports whether a typed word names this object, as the C's
// isname() does: any whitespace-separated keyword the word is a prefix of.
func (o *Object) Matches(word string) bool {
	...
	return matchesKeywords(o.Keywords, word)
}
```

The implementation is right — it calls `matchesKeywords`, which is the
oracle-checked whole-word port. The comment is a survivor of the version
before the fix and asserts **both** of the things §2 says the project spent
four phases and then a year disproving: that the match is a prefix, and
that keywords are separated by whitespace.

This is not cosmetic. `Object.Matches` is the function a reader reaches for
when they want to know how object keywords work, and its doc comment
currently tells them the pre-#277 story. The next person to "simplify"
`matchesKeywords` into `strings.Fields` plus `HasPrefix` will have this
comment as their justification, and both the oracle and the C would have to
be re-read to find out otherwise. Corrected in the same change as this
document.

No other stale claim of this kind was found: `live.go`'s references to the
prefix-matching era are all past-tense descriptions of the fixed bug, and
`help.go`, `announce.go`, `spell.go` and `types.go` describe genuine prefix
matches correctly.

---

## 8. Summary

| Rule | State |
|---|---|
| 1. `isname` — objects, mobiles, exits | **Correct**, oracle-checked over 1,456 pairings |
| 2. Command abbreviation | **Correct**, table order re-parsed from `interpreter.c`, level-in-match reproduced |
| 3. `is_abbrev` | **Correct** |
| 4. `find_skill_num` — spells | **Broken**: second branch missing, 1,145/1,549 abbreviations refused (#355) |
| 5. `search_block` — fixed tables | **Correct** in shape; empty-argument quirk not reproduced, and unreachable |
| 0. `fill_word` | **Correct** |

So: partial keyword matching does keep working the way it did in the C
server, for keywords. For spell names it does not, and `cast 'mag mis'` is
the shape of the loss.

---

## Related

- `reference/tools/nameoracle.c` — rule 1's oracle.
- `reference/tools/skilloracle.c` — rule 4's, written for this
  investigation.
- `docs/weirdnumbers.md` — where rule 1's history is catalogued.
- `CLAUDE.md`, "Do not read the C and transcribe it" — rules 1 and 4 are
  both instances of its "if you would have to simulate a function in your
  head to be sure of it" trigger, and rule 4 is an instance of its warning
  about corpora too. `TestSpellNumberByName`'s four cases are `magic
  missile`, `magic mis`, `armor` and `heal`: one full two-word name, one
  abbreviation of the *last* word only, and two single-word spells. The
  missing branch adds nothing to any of them — it only ever fires when the
  **first** word is abbreviated — so the four cases agree with a C they
  are not testing, which is `isname`'s letters-and-spaces corpus (#277)
  happening again in a different function.
