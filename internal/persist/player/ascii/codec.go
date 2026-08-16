// Package ascii reads and writes the ascii_pfiles player format: one
// human-readable text file per character.
//
// This is the format the server runs on. The binary format it replaces is a
// raw struct dump whose password field is eleven bytes — too small for any
// modern hash — so moving off it is a prerequisite for decent credentials
// rather than an independent improvement. The binary format remains readable
// and writable for conversion; it is simply not what a live server uses.
//
// The format itself is the public ascii_pfiles 2.1 patch. It is specified
// field by field in docs/investigations/ascii-pfile-format.md, which was
// written from the reference implementation and cross-checked against real
// files, and this package is written against that document rather than
// against the C source a second time.
package ascii

import (
	"bufio"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// Tags are exactly four characters, left-padded with spaces where the field
// name is shorter. That padding is part of the format, not decoration: the
// reference reader takes a fixed-width slice before comparing.
const tagWidth = 4

// Tags, in the order save_char() writes them. Nothing reading the format may
// depend on that order — the reference reader dispatches line by line on the
// tag and accepts them in any order, or absent — but writing them in a
// consistent order keeps a diff of two saves readable.
const (
	tagName = "Name"
	tagPass = "Pass"
	tagTitl = "Titl"
	tagDesc = "Desc"
	tagSex  = "Sex "
	tagClas = "Clas"
	tagRace = "Race"
	tagLevl = "Levl"
	tagHome = "Home"
	tagBrth = "Brth"
	tagPlyd = "Plyd"
	tagLast = "Last"
	tagHost = "Host"
	tagHite = "Hite"
	tagWate = "Wate"
	tagStr  = "Str "
	tagInt  = "Int "
	tagWis  = "Wis "
	tagDex  = "Dex "
	tagCon  = "Con "
	tagCha  = "Cha "
	tagHit  = "Hit "
	tagMana = "Mana"
	tagMove = "Move"
	tagAc   = "Ac  "
	tagGold = "Gold"
	tagBank = "Bank"
	tagExp  = "Exp "
	tagHrol = "Hrol"
	tagDrol = "Drol"
	tagAlin = "Alin"
	tagID   = "Id  "
	tagAct  = "Act "
	tagAff  = "Aff "
	tagPref = "Pref"
	tagWimp = "Wimp"
	tagFrez = "Frez"
	tagInvs = "Invs"
	tagRoom = "Room"
	tagBadp = "Badp"
	tagHung = "Hung"
	tagThir = "Thir"
	tagDrnk = "Drnk"
	tagLern = "Lern"
	tagRmrt = "Rmrt"
	tagSkil = "Skil"
	tagAffs = "Affs"
)

// savingThrowTag returns the tag for saving throw n (0-based): Thr1..Thr5.
func savingThrowTag(n int) string { return fmt.Sprintf("Thr%d", n+1) }

// Encode writes a character.
//
// Most numeric fields are omitted when they equal their default, which is
// always zero — that is what keeps a fresh character's file a dozen lines
// rather than fifty. Six fields are written unconditionally (Name, Pass,
// Brth, Plyd, Last, Id) because the reference writer does, and because a
// record missing its identity is worse than a slightly larger file.
func Encode(w io.Writer, p *game.PlayerRecord) error {
	bw := bufio.NewWriter(w)

	put := func(tag, value string) {
		_, _ = fmt.Fprintf(bw, "%-*s: %s\n", tagWidth, strings.TrimRight(tag, " "), value)
	}
	putInt := func(tag string, v int64) { put(tag, strconv.FormatInt(v, 10)) }
	// putIntIf omits the field when it holds its default.
	putIntIf := func(tag string, v int64) {
		if v != 0 {
			putInt(tag, v)
		}
	}
	putStrIf := func(tag, v string) {
		if v != "" {
			put(tag, v)
		}
	}

	// Always written.
	put(tagName, p.Name)
	put(tagPass, credentialField(p.Credential))

	putStrIf(tagTitl, p.Title)
	putIntIf(tagSex, int64(p.Sex))
	putIntIf(tagClas, int64(p.Class))
	putIntIf(tagRace, int64(p.Race))
	putIntIf(tagLevl, int64(p.Level))
	putIntIf(tagHome, int64(p.Hometown))

	// Always written.
	putInt(tagBrth, unixOrZero(p.Birth))
	putInt(tagPlyd, int64(p.Played/time.Second))
	putInt(tagLast, unixOrZero(p.LastLogon))

	putStrIf(tagHost, p.Host)
	putIntIf(tagHite, int64(p.Height))
	putIntIf(tagWate, int64(p.Weight))

	// Strength carries the exceptional-strength percentile alongside it.
	if p.Abilities.Strength != 0 || p.Abilities.StrengthPercentile != 0 {
		put(tagStr, fmt.Sprintf("%d/%d", p.Abilities.Strength, p.Abilities.StrengthPercentile))
	}
	putIntIf(tagInt, int64(p.Abilities.Intelligence))
	putIntIf(tagWis, int64(p.Abilities.Wisdom))
	putIntIf(tagDex, int64(p.Abilities.Dexterity))
	putIntIf(tagCon, int64(p.Abilities.Constitution))
	putIntIf(tagCha, int64(p.Abilities.Charisma))

	putPair := func(tag string, cur, max int32) {
		if cur != 0 || max != 0 {
			put(tag, fmt.Sprintf("%d/%d", cur, max))
		}
	}
	putPair(tagHit, p.Points.Hit, p.Points.MaxHit)
	putPair(tagMana, p.Points.Mana, p.Points.MaxMana)
	putPair(tagMove, p.Points.Move, p.Points.MaxMove)

	putIntIf(tagAc, int64(p.Points.Armor))
	putIntIf(tagGold, int64(p.Points.Gold))
	putIntIf(tagBank, int64(p.Points.BankGold))
	putIntIf(tagExp, int64(p.Points.Exp))
	putIntIf(tagHrol, int64(p.Points.HitRoll))
	putIntIf(tagDrol, int64(p.Points.DamRoll))
	putIntIf(tagAlin, int64(p.Alignment))

	// Always written.
	putInt(tagID, p.IDNum)

	// The three bitfields use the letter encoding, with "0" for empty —
	// an empty value would misalign the reader.
	if p.PlayerFlags != 0 {
		put(tagAct, p.PlayerFlags.String())
	}
	if p.AffectFlags != 0 {
		put(tagAff, p.AffectFlags.String())
	}
	if p.Preferences != 0 {
		put(tagPref, p.Preferences.String())
	}

	for i, v := range p.SavingThrows {
		putIntIf(savingThrowTag(i), int64(v))
	}

	putIntIf(tagWimp, int64(p.WimpLevel))
	putIntIf(tagFrez, int64(p.FreezeLevel))
	putIntIf(tagInvs, int64(p.InvisLevel))
	putIntIf(tagRoom, int64(p.LoadRoom))
	putIntIf(tagBadp, int64(p.BadPasswords))

	// Conditions: drunk, full, thirsty. -1 means "does not apply", which is
	// how immortals are stored, and is not the same as absent.
	putIntIf(tagDrnk, int64(p.Conditions[0]))
	putIntIf(tagHung, int64(p.Conditions[1]))
	putIntIf(tagThir, int64(p.Conditions[2]))

	putIntIf(tagLern, int64(p.SpellsToLearn))
	// Rmrt is a bitmask but is written as a plain number, matching every
	// genuine example found; see the format document.
	putIntIf(tagRmrt, int64(p.RemortVector))

	if p.Description != "" {
		_, _ = fmt.Fprintf(bw, "%s:\n%s\n~\n", tagDesc, strings.TrimRight(p.Description, "\r\n"))
	}

	if len(p.Skills) > 0 {
		_, _ = fmt.Fprintf(bw, "%s:\n", tagSkil)
		// Sorted, so two saves of the same character produce the same file.
		// Ranging a map here would make every save a spurious diff.
		nums := make([]int32, 0, len(p.Skills))
		for n := range p.Skills {
			nums = append(nums, n)
		}
		sort.Slice(nums, func(i, j int) bool { return nums[i] < nums[j] })
		for _, n := range nums {
			_, _ = fmt.Fprintf(bw, "%d %d\n", n, p.Skills[n])
		}
		_, _ = fmt.Fprint(bw, "0 0\n")
	}

	if len(p.Affects) > 0 {
		_, _ = fmt.Fprintf(bw, "%s:\n", tagAffs)
		for _, a := range p.Affects {
			_, _ = fmt.Fprintf(bw, "%d %d %d %d %d\n", a.Type, a.Duration, a.Modifier, a.Location, uint64(a.Bits))
		}
		_, _ = fmt.Fprint(bw, "0 0 0 0 0\n")
	}

	return bw.Flush()
}

// credentialField renders the password for storage.
//
// The format stores whatever crypt() produced upstream of it, so the field is
// opaque to the file. A scheme prefix is added for anything that is not the
// legacy DES hash, so a reader can tell them apart — a bare hash with no
// prefix is DES by definition, since that is all the format ever held.
func credentialField(c game.Credential) string {
	switch c.Scheme {
	case game.SchemeNone:
		return ""
	case game.SchemeLegacyDES:
		return c.Hash
	default:
		return string(c.Scheme) + ":" + c.Hash
	}
}

// parseCredential is credentialField's inverse.
func parseCredential(v string) game.Credential {
	if v == "" {
		return game.Credential{}
	}
	// A modern hash is "scheme:hash". A DES hash is 13 characters of
	// [./0-9A-Za-z] and can never contain a colon, so this cannot
	// misclassify one.
	if scheme, hash, found := strings.Cut(v, ":"); found {
		return game.Credential{Scheme: game.CredentialScheme(scheme), Hash: hash}
	}
	return game.Credential{Scheme: game.SchemeLegacyDES, Hash: v}
}

func unixOrZero(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.Unix()
}

func timeOrZero(secs int64) time.Time {
	if secs == 0 {
		return time.Time{}
	}
	return time.Unix(secs, 0).UTC()
}

// Decode reads a character file.
//
// Unknown tags are collected rather than rejected. The reference reader logs
// and skips them, and the format has no comment syntax, so a hand-annotated
// file would otherwise be unreadable — but silently discarding them would
// lose data from a file some other server wrote, so they are returned.
func Decode(r io.Reader) (*game.PlayerRecord, []string, error) {
	p := &game.PlayerRecord{}
	var unknown []string

	sc := bufio.NewScanner(r)
	// Descriptions can be long, and the default 64KB token limit is not
	// generous enough to rely on for text a player typed.
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	line := 0
	next := func() (string, bool) {
		if !sc.Scan() {
			return "", false
		}
		line++
		return sc.Text(), true
	}

	for {
		raw, ok := next()
		if !ok {
			break
		}
		if strings.TrimSpace(raw) == "" {
			continue
		}

		tag, value, found := strings.Cut(raw, ":")
		if !found {
			unknown = append(unknown, fmt.Sprintf("line %d: %q has no tag", line, raw))
			continue
		}
		tag = strings.TrimRight(tag, " ")
		value = strings.TrimSpace(value)

		if err := assign(p, tag, value, next, &unknown, line); err != nil {
			return nil, unknown, fmt.Errorf("line %d: %w", line, err)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, unknown, err
	}
	if p.Name == "" {
		return nil, unknown, fmt.Errorf("file has no Name")
	}
	return p, unknown, nil
}

// assign applies one tag. next reads a further line, for the block forms.
func assign(p *game.PlayerRecord, tag, value string, next func() (string, bool), unknown *[]string, line int) error {
	// num reads the value at full width, for the fields that need it: Id is
	// int64 and the timestamps are seconds since the epoch.
	num := func() int64 {
		n, _ := strconv.ParseInt(value, 10, 64)
		return n
	}
	// num32 reads at the width the model actually stores. Parsing at 32 bits
	// rather than narrowing afterwards means a file containing a number too
	// large for the field saturates at the limit instead of wrapping to some
	// unrelated value — this format has no width limit of its own, so nothing
	// stops a hand-edited file from carrying one.
	num32 := func() int32 {
		n, _ := strconv.ParseInt(value, 10, 32)
		return int32(n) //nolint:gosec // parsed at 32 bits, so this cannot truncate
	}

	switch tag {
	case "Name":
		p.Name = value
	case "Pass":
		p.Credential = parseCredential(value)
	case "Titl":
		p.Title = value
	case "Host":
		p.Host = value
	case "Sex":
		p.Sex = num32()
	case "Clas":
		p.Class = num32()
	case "Race":
		p.Race = num32()
	case "Levl":
		p.Level = num32()
	case "Home":
		p.Hometown = num32()
	case "Brth":
		p.Birth = timeOrZero(num())
	case "Last":
		p.LastLogon = timeOrZero(num())
	case "Plyd":
		p.Played = time.Duration(num()) * time.Second
	case "Hite":
		p.Height = num32()
	case "Wate":
		p.Weight = num32()
	case "Str":
		a, b := parsePair(value)
		p.Abilities.Strength, p.Abilities.StrengthPercentile = a, b
	case "Int":
		p.Abilities.Intelligence = num32()
	case "Wis":
		p.Abilities.Wisdom = num32()
	case "Dex":
		p.Abilities.Dexterity = num32()
	case "Con":
		p.Abilities.Constitution = num32()
	case "Cha":
		p.Abilities.Charisma = num32()
	case "Hit":
		p.Points.Hit, p.Points.MaxHit = parsePair(value)
	case "Mana":
		p.Points.Mana, p.Points.MaxMana = parsePair(value)
	case "Move":
		p.Points.Move, p.Points.MaxMove = parsePair(value)
	case "Ac":
		p.Points.Armor = num32()
	case "Gold":
		p.Points.Gold = num32()
	case "Bank":
		p.Points.BankGold = num32()
	case "Exp":
		p.Points.Exp = num32()
	case "Hrol":
		p.Points.HitRoll = num32()
	case "Drol":
		p.Points.DamRoll = num32()
	case "Alin":
		p.Alignment = num32()
	case "Id":
		p.IDNum = num()
	case "Act":
		p.PlayerFlags, _ = game.ParseFlags(value)
	case "Aff":
		p.AffectFlags, _ = game.ParseFlags(value)
	case "Pref":
		p.Preferences, _ = game.ParseFlags(value)
	case "Thr1", "Thr2", "Thr3", "Thr4", "Thr5":
		p.SavingThrows[tag[3]-'1'] = num32()
	case "Wimp":
		p.WimpLevel = num32()
	case "Frez":
		p.FreezeLevel = num32()
	case "Invs":
		p.InvisLevel = num32()
	case "Room":
		p.LoadRoom = game.RoomVnum(num32())
	case "Badp":
		p.BadPasswords = num32()
	case "Drnk":
		p.Conditions[0] = num32()
	case "Hung":
		p.Conditions[1] = num32()
	case "Thir":
		p.Conditions[2] = num32()
	case "Lern":
		p.SpellsToLearn = num32()
	case "Rmrt":
		p.RemortVector = num32()

	case "Desc":
		p.Description = readTildeBlock(next)
	case "Skil":
		p.Skills = readSkills(next)
	case "Affs":
		p.Affects = readAffects(next)

	default:
		*unknown = append(*unknown, fmt.Sprintf("line %d: unknown tag %q", line, tag))
	}
	return nil
}

// parsePair reads the "a/b" form used by Str, Hit, Mana and Move.
func parsePair(v string) (int32, int32) {
	a, b, _ := strings.Cut(v, "/")
	x, _ := strconv.ParseInt(strings.TrimSpace(a), 10, 32)
	y, _ := strconv.ParseInt(strings.TrimSpace(b), 10, 32)
	return int32(x), int32(y)
}

// readTildeBlock reads until a line containing just '~', the same terminator
// the world files use.
func readTildeBlock(next func() (string, bool)) string {
	var b strings.Builder
	for {
		line, ok := next()
		if !ok || strings.TrimRight(line, "\r") == "~" {
			return strings.TrimRight(b.String(), "\n")
		}
		b.WriteString(strings.TrimRight(line, "\r"))
		b.WriteString("\n")
	}
}

// readSkills reads "<number> <percentage>" lines until "0 0".
func readSkills(next func() (string, bool)) map[int32]int32 {
	skills := map[int32]int32{}
	for {
		line, ok := next()
		if !ok {
			return skills
		}
		var num, pct int32
		if _, err := fmt.Sscanf(strings.TrimSpace(line), "%d %d", &num, &pct); err != nil {
			return skills
		}
		if num == 0 {
			return skills
		}
		// A skill at zero percent is not known, and the binary format cannot
		// express the difference — so dropping it keeps the two formats
		// agreeing about what a character knows.
		if pct != 0 {
			skills[num] = pct
		}
	}
}

// readAffects reads five-field lines until an all-zero one.
func readAffects(next func() (string, bool)) []game.Affect {
	var affects []game.Affect
	for {
		line, ok := next()
		if !ok {
			return affects
		}
		var typ, dur, mod, loc int32
		var bits uint64
		if _, err := fmt.Sscanf(strings.TrimSpace(line), "%d %d %d %d %d", &typ, &dur, &mod, &loc, &bits); err != nil {
			return affects
		}
		if typ == 0 {
			return affects
		}
		affects = append(affects, game.Affect{
			Type: typ, Duration: dur, Modifier: mod,
			Location: loc, Bits: game.Flags(bits),
		})
	}
}
