// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package classic

import (
	"fmt"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// parseObject reads one object record, positioned just after its "#vnum"
// line. Mirrors parse_object() in db.c.
//
// Objects are the awkward record type: they have no end-of-record marker, so
// the parser only discovers a record has ended by reading the '#' or '$' that
// starts the next one. The C code returns that line to its caller for reuse;
// here it is pushed back onto the reader, which behaves the same and reads
// better.
func (l *loader) parseObject(r *reader, vnum game.ObjVnum) (*game.ObjDef, error) {
	what := fmt.Sprintf("object #%d", vnum)
	obj := &game.ObjDef{Vnum: vnum}

	var err error
	if obj.Keywords, err = r.readString(what + " keywords"); err != nil {
		return nil, err
	}
	if obj.Keywords == "" {
		// The C loader treats a NULL name as fatal, and it is: an object
		// nobody can refer to cannot be picked up.
		return nil, fmt.Errorf("%s: has no keywords", r.where(what))
	}
	if obj.ShortDesc, err = r.readString(what + " short description"); err != nil {
		return nil, err
	}
	obj.ShortDesc = lowerLeadingArticle(obj.ShortDesc)

	if obj.Description, err = r.readString(what + " description"); err != nil {
		return nil, err
	}
	obj.Description = capitalize(obj.Description)

	if obj.ActionDesc, err = r.readString(what + " action description"); err != nil {
		return nil, err
	}

	// First numeric line: " %d %s %s %d " — type, extra flags, wear flags,
	// and a permanent-affect bitvector that the C loader reads as a plain
	// integer rather than as letter flags. A three-field line is accepted
	// with the fourth defaulting to 0.
	line, ok := r.getLine()
	if !ok {
		return nil, fmt.Errorf("%s: file ended before the first numeric line", r.where(what))
	}
	fields := splitFields(line)
	if len(fields) < 3 {
		return nil, fmt.Errorf("%s: malformed type/flags line %q, want '<type> <extra> <wear> [<perm>]'", r.where(what), line)
	}
	typ, ok := scanInt(fields[0])
	if !ok {
		return nil, fmt.Errorf("%s: item type %q is not a number", r.where(what), fields[0])
	}
	obj.Type = game.ItemType(typ)
	obj.ExtraFlags = game.SetFromRaw[game.ExtraFlag](l.parseFlagField(r, what, "extra flags", fields[1]))
	obj.WearFlags = game.SetFromRaw[game.WearFlag](l.parseFlagField(r, what, "wear flags", fields[2]))
	if len(fields) >= 4 {
		if perm, ok := scanInt(fields[3]); ok {
			obj.PermAffect = perm
		}
	}

	// Second numeric line: the four value slots. All four are required.
	line, ok = r.getLine()
	if !ok {
		return nil, fmt.Errorf("%s: file ended before the values line", r.where(what))
	}
	vals, err := requireInts(line, game.NumObjValues, "the four object values")
	if err != nil {
		return nil, fmt.Errorf("%s: %w", r.where(what), err)
	}
	copy(obj.Values[:], vals)

	// Third numeric line: " %d %d %d %d " — weight, cost, rent, level. As
	// with the first line, three fields are accepted and level defaults to 0.
	line, ok = r.getLine()
	if !ok {
		return nil, fmt.Errorf("%s: file ended before the weight/cost line", r.where(what))
	}
	wc := make([]int32, 4)
	got := scanInts(line, wc)
	if got < 3 {
		return nil, fmt.Errorf("%s: malformed weight/cost line %q, want at least '<weight> <cost> <rent>'", r.where(what), line)
	}
	obj.Weight = wc[0]
	obj.Cost = wc[1]
	obj.RentPerDay = wc[2]
	if got >= 4 {
		obj.MinLevel = wc[3]
	}

	l.applyDrinkContainerWeightFix(r, obj)

	for {
		line, ok := r.getLine()
		if !ok {
			return nil, fmt.Errorf("%s: file ended before the next record or '$'", r.where(what))
		}
		switch line[0] {
		case 'E':
			keywords, err := r.readString(what + " extra description keyword")
			if err != nil {
				return nil, err
			}
			desc, err := r.readString(what + " extra description")
			if err != nil {
				return nil, err
			}
			obj.ExtraDescs = append(obj.ExtraDescs, game.ExtraDesc{
				Keywords: keywords, Description: desc,
			})

		case 'A':
			if len(obj.Affects) >= game.MaxObjAffects {
				return nil, fmt.Errorf("%s: more than %d 'A' affect lines", r.where(what), game.MaxObjAffects)
			}
			line, ok := r.getLine()
			if !ok {
				return nil, fmt.Errorf("%s: file ended inside an 'A' affect line", r.where(what))
			}
			pair, err := requireInts(line, 2, "an affect's location and modifier")
			if err != nil {
				return nil, fmt.Errorf("%s: %w", r.where(what), err)
			}
			obj.Affects = append(obj.Affects, game.ObjAffect{
				Location: game.Apply(pair[0]), Modifier: pair[1],
			})

		case '#', '$':
			// End of this object; the line belongs to the caller.
			r.unreadLine(line)
			reverseExtras(obj.ExtraDescs)
			return obj, nil

		default:
			return nil, fmt.Errorf("%s: expected E, A, # or $, got %q", r.where(what), line)
		}
	}
}

// Item type numbers, only as far as this file needs them. The full table
// belongs with the game rules, not the parser.
const (
	itemDrinkCon = 17
	itemFountain = 23
)

// applyDrinkContainerWeightFix reproduces a load-time adjustment in
// parse_object(): a drink container or fountain whose weight is less than the
// amount of liquid it holds gets its weight raised to that amount plus five.
//
// This is a genuine mutation of world data at load time, not a validation,
// so it has to happen here for the loaded world to match the C server's.
func (l *loader) applyDrinkContainerWeightFix(r *reader, obj *game.ObjDef) {
	if obj.Type != itemDrinkCon && obj.Type != itemFountain {
		return
	}
	capacity := obj.Values[1]
	if obj.Weight >= capacity {
		return
	}
	l.infof("%s: %s weighs %d but holds %d units of liquid; the loader raises its weight to %d",
		r.where(fmt.Sprintf("object #%d", obj.Vnum)), obj.ShortDesc, obj.Weight, capacity, capacity+5)
	obj.Weight = capacity + 5
}
