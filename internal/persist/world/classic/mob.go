package classic

import (
	"fmt"
	"strings"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// parseMobile reads one mobile record, positioned just after its "#vnum"
// line. Mirrors parse_mobile(), parse_simple_mob() and parse_enhanced_mob()
// in db.c.
func (l *loader) parseMobile(r *reader, vnum game.MobVnum) (*game.MobDef, error) {
	what := fmt.Sprintf("mob #%d", vnum)
	mob := &game.MobDef{Vnum: vnum}

	var err error
	if mob.Keywords, err = r.readString(what + " keywords"); err != nil {
		return nil, err
	}
	if mob.ShortDesc, err = r.readString(what + " short description"); err != nil {
		return nil, err
	}
	// The C loader lowercases the first letter when the short description
	// begins with an article, so "A pelican" reads correctly mid-sentence.
	mob.ShortDesc = lowerLeadingArticle(mob.ShortDesc)

	if mob.LongDesc, err = r.readString(what + " long description"); err != nil {
		return nil, err
	}
	if mob.Description, err = r.readString(what + " description"); err != nil {
		return nil, err
	}

	line, ok := r.getLine()
	if !ok {
		return nil, fmt.Errorf("%s: file ended before the flags line", r.where(what))
	}

	// "%s %s %d %c": action flags, affection flags, alignment, and the type
	// letter that selects the simple or enhanced body.
	fields := splitFields(line)
	if len(fields) < 4 {
		return nil, fmt.Errorf("%s: malformed flags line %q, want '<act> <aff> <align> {S|E}'", r.where(what), line)
	}

	mob.ActionFlags = l.parseFlagField(r, what, "action flags", fields[0])
	// parse_mobile() force-sets this on every mobile regardless of the file.
	mob.ActionFlags = mob.ActionFlags.Set(game.MobIsNPC)
	mob.AffectionFlags = l.parseFlagField(r, what, "affection flags", fields[1])

	align, ok := scanInt(fields[2])
	if !ok {
		return nil, fmt.Errorf("%s: alignment %q is not a number", r.where(what), fields[2])
	}
	mob.Alignment = align

	// The C loader reads this with %c and then upper-cases it, so a lowercase
	// 's' or a longer token beginning with 'S' both work.
	switch typ := upperASCII(fields[3][0]); typ {
	case 'S':
		if err := l.parseSimpleMob(r, mob); err != nil {
			return nil, err
		}
	case 'E':
		mob.Enhanced = true
		if err := l.parseSimpleMob(r, mob); err != nil {
			return nil, err
		}
		if err := l.parseMobEspecs(r, mob); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("%s: unsupported mob type %q (want S or E)", r.where(what), string(typ))
	}

	return mob, nil
}

// parseSimpleMob reads the three numeric lines shared by both mob formats.
func (l *loader) parseSimpleMob(r *reader, mob *game.MobDef) error {
	what := fmt.Sprintf("mob #%d", mob.Vnum)

	// " %d %d %d %dd%d+%d %dd%d+%d ": level, thac0, AC, hit dice, damage dice.
	line, ok := r.getLine()
	if !ok {
		return fmt.Errorf("%s: file ended before the level/dice line", r.where(what))
	}
	nums, err := scanDiceLine(line)
	if err != nil {
		return fmt.Errorf("%s: %w", r.where(what), err)
	}
	mob.Level = nums[0]
	mob.Thac0 = nums[1]
	mob.ArmorClass = nums[2]
	mob.HitDice = game.Dice{Number: nums[3], Size: nums[4], Bonus: nums[5]}
	mob.DamageDice = game.Dice{Number: nums[6], Size: nums[7], Bonus: nums[8]}

	// " %d %d ": gold and experience.
	line, ok = r.getLine()
	if !ok {
		return fmt.Errorf("%s: file ended before the gold/exp line", r.where(what))
	}
	pair, err := requireInts(line, 2, "gold and experience")
	if err != nil {
		return fmt.Errorf("%s: %w", r.where(what), err)
	}
	mob.Gold = pair[0]
	mob.Exp = pair[1]

	// " %d %d %d ": load position, default position, sex.
	line, ok = r.getLine()
	if !ok {
		return fmt.Errorf("%s: file ended before the position/sex line", r.where(what))
	}
	trip, err := requireInts(line, 3, "position, default position and sex")
	if err != nil {
		return fmt.Errorf("%s: %w", r.where(what), err)
	}
	mob.Position = trip[0]
	mob.DefaultPosition = trip[1]
	mob.Sex = trip[2]

	return nil
}

// parseMobEspecs reads the "Key: value" block of an enhanced mob, up to its
// 'E' terminator.
func (l *loader) parseMobEspecs(r *reader, mob *game.MobDef) error {
	what := fmt.Sprintf("mob #%d", mob.Vnum)
	for {
		line, ok := r.getLine()
		if !ok {
			return fmt.Errorf("%s: file ended inside the enhanced (E) section", r.where(what))
		}
		if line == "E" {
			return nil
		}
		if strings.HasPrefix(line, "#") {
			return fmt.Errorf("%s: unterminated E section: hit the next record at %q", r.where(what), line)
		}
		key, value, found := strings.Cut(line, ":")
		if !found {
			// interpret_espec() splits on ':' and treats a missing colon as a
			// keyword with no value rather than an error.
			key, value = line, ""
		}
		mob.Especs = append(mob.Especs, game.Espec{
			Key:   strings.TrimSpace(key),
			Value: strings.TrimSpace(value),
		})
	}
}

// scanDiceLine parses "<level> <thac0> <ac> <n>d<s>+<b> <n>d<s>+<b>" into
// nine numbers. The C code does this with a single sscanf whose format embeds
// the 'd' and '+' literals.
func scanDiceLine(line string) ([]int32, error) {
	// Replacing the separators with spaces turns the whole line into nine
	// plain fields, which is exactly what the sscanf format extracts. A '-'
	// bonus would not survive this, but the format has no way to express one
	// either: sscanf's literal '+' would fail to match.
	flat := strings.NewReplacer("d", " ", "+", " ").Replace(line)
	nums := make([]int32, 9)
	if got := scanInts(flat, nums); got != 9 {
		return nil, fmt.Errorf("expected '<level> <thac0> <ac> <n>d<s>+<b> <n>d<s>+<b>', got %d numbers in %q", got, line)
	}
	return nums, nil
}

// lowerLeadingArticle lowercases the first letter if the first word is "a",
// "an" or "the", matching parse_mobile() and parse_object().
func lowerLeadingArticle(s string) string {
	if s == "" {
		return s
	}
	first, _, _ := strings.Cut(s, " ")
	switch strings.ToLower(first) {
	case "a", "an", "the":
		if s[0] >= 'A' && s[0] <= 'Z' {
			return string(s[0]+'a'-'A') + s[1:]
		}
	}
	return s
}

// capitalize uppercases the first letter, matching the CAP() macro the object
// loader applies to long descriptions.
func capitalize(s string) string {
	if s == "" {
		return s
	}
	if s[0] >= 'a' && s[0] <= 'z' {
		return string(s[0]-'a'+'A') + s[1:]
	}
	return s
}

func upperASCII(b byte) byte {
	if b >= 'a' && b <= 'z' {
		return b - 'a' + 'A'
	}
	return b
}

// parseFlagField decodes a bitfield and reports anything questionable about
// it, so the same warnings appear wherever flags are read.
func (l *loader) parseFlagField(r *reader, what, field, raw string) game.Flags {
	flags, unknown := game.ParseFlags(raw)
	if len(unknown) > 0 {
		l.warnf("%s: %s %q contain characters that are neither letters nor digits (%q); the C loader ignores them",
			r.where(what), field, raw, string(unknown))
	}
	if flags.ExceedsCRange() {
		l.warnf("%s: %s %q use bits above %d, which the C server cannot represent",
			r.where(what), field, raw, game.CFlagLimit)
	}
	return flags
}
