// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package main

import (
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strings"
	"time"
)

// diffValues walks two loaded states and reports every place they differ,
// as a path and the two values.
//
// Every difference, not the first: `verify --against` is run against a
// real archive by somebody deciding whether to trust a migration, and
// "the first field that disagrees" is a much worse answer than "these
// eleven fields disagree, all of them timestamps". That is also why this
// is a walk rather than a reflect.DeepEqual: DeepEqual answers the
// question with a bool, and the bool is not the useful part.
//
// The comparison is over *loaded state*, which is the claim
// docs/proposals/yaml-only.md §4.1 argues for: not "the bytes round-trip"
// (they cannot, and should not have to) but "a server running on the
// converted data behaves identically to one running on the original".
func diffValues(want, got any) []string {
	d := &differ{}
	d.walk("", reflect.ValueOf(want), reflect.ValueOf(got))
	return d.out
}

// maxDiffs caps the report. A comparison that has gone wrong structurally
// — a whole subsystem missing, an off-by-one in an index — produces one
// difference per record, and thousands of them tell you nothing the first
// fifty did not.
const maxDiffs = 200

type differ struct {
	out []string
}

func (d *differ) reportf(path, format string, args ...any) {
	if len(d.out) > maxDiffs {
		return
	}
	if len(d.out) == maxDiffs {
		d.out = append(d.out, fmt.Sprintf("... and more (stopped after %d)", maxDiffs))
		return
	}
	if path == "" {
		path = "."
	}
	d.out = append(d.out, path+": "+fmt.Sprintf(format, args...))
}

// timeType is special-cased below: a time.Time compares by instant, not by
// its unexported monotonic-clock and location fields, which two decoders
// have no reason to agree about and which say nothing about the data.
var timeType = reflect.TypeOf(time.Time{})

func (d *differ) walk(path string, a, b reflect.Value) {
	if len(d.out) > maxDiffs {
		return
	}
	if !a.IsValid() || !b.IsValid() {
		if a.IsValid() != b.IsValid() {
			d.reportf(path, "one side has no value")
		}
		return
	}
	if a.Type() != b.Type() {
		d.reportf(path, "types differ: %s vs %s", a.Type(), b.Type())
		return
	}

	if a.Type() == timeType {
		ta, tb := a.Interface().(time.Time), b.Interface().(time.Time)
		if !ta.Equal(tb) {
			d.reportf(path, "%s vs %s", ta.UTC().Format(time.RFC3339), tb.UTC().Format(time.RFC3339))
		}
		return
	}

	switch a.Kind() {
	case reflect.Pointer, reflect.Interface:
		switch {
		case a.IsNil() && b.IsNil():
		case a.IsNil() != b.IsNil():
			d.reportf(path, "%s vs %s", describeNil(a), describeNil(b))
		default:
			d.walk(path, a.Elem(), b.Elem())
		}

	case reflect.Struct:
		t := a.Type()
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if !f.IsExported() {
				continue
			}
			if isKeywordField(f) {
				d.compareKeywords(join(path, f.Name), a.Field(i).String(), b.Field(i).String())
				continue
			}
			d.walk(join(path, f.Name), a.Field(i), b.Field(i))
		}

	case reflect.Slice, reflect.Array:
		if a.Len() != b.Len() {
			d.reportf(path, "%d element(s) vs %d", a.Len(), b.Len())
			// Still walk the common prefix: a length difference with
			// identical contents up to it is a different (and much less
			// alarming) finding than one where everything also moved.
		}
		if d.walkByVnum(path, a, b) {
			return
		}
		n := min(a.Len(), b.Len())
		for i := 0; i < n; i++ {
			d.walk(fmt.Sprintf("%s[%d]", path, i), a.Index(i), b.Index(i))
		}

	case reflect.Map:
		for _, k := range sortedMapKeys(a, b) {
			av, bv := a.MapIndex(k), b.MapIndex(k)
			sub := fmt.Sprintf("%s[%v]", path, k.Interface())
			switch {
			case !av.IsValid():
				d.reportf(sub, "missing on the left, %v on the right", bv.Interface())
			case !bv.IsValid():
				d.reportf(sub, "%v on the left, missing on the right", av.Interface())
			default:
				d.walk(sub, av, bv)
			}
		}

	default:
		if !reflect.DeepEqual(a.Interface(), b.Interface()) {
			d.reportf(path, "%s vs %s", format(a), format(b))
		}
	}
}

// walkByVnum handles the one difference an element-by-element walk
// describes worst: two lists holding the same vnum-keyed records in a
// different order.
//
// Comparing those by position pairs every record with somebody else's, so
// every field of every record disagrees, and the report is hundreds of
// lines that look like total corruption rather than the one-line fact that
// two lists are out of step. That is not hypothetical: importing the
// archived Disgracelands lib/ produced exactly that, 200 lines of
// Shops[5].Messages[3] and Shops[6].Keeper, for a shop list that had every
// shop in it and had simply written five of them into the wrong zone file
// (internal/persist/world/yaml's shopHomeVnum).
//
// So when both sides are the same set of vnums, say so once and then
// compare each record against *its own* counterpart — which is what makes
// this safe to do rather than merely quieter, since a record that has both
// moved and changed still reports the change, under a path naming its vnum
// instead of an index that means nothing.
//
// Anything less tidy than that — a differing set of vnums, a repeated vnum,
// elements with no vnum at all — falls back to the positional walk, which
// is the right answer when the lists are genuinely not the same records.
func (d *differ) walkByVnum(path string, a, b reflect.Value) bool {
	if a.Len() != b.Len() || a.Len() < 2 {
		return false
	}
	ai, ok := vnumIndex(a)
	if !ok {
		return false
	}
	bi, ok := vnumIndex(b)
	if !ok {
		return false
	}

	moved, firstVnum, firstAt, firstWant := 0, int64(0), 0, 0
	for vnum, i := range ai {
		j, ok := bi[vnum]
		if !ok {
			return false // not the same records; positions are all there is
		}
		if i == j {
			continue
		}
		if moved == 0 || i < firstAt {
			firstVnum, firstAt, firstWant = vnum, i, j
		}
		moved++
	}
	if moved == 0 {
		return false // same order, so the ordinary walk says everything
	}

	d.reportf(path, "the same %d record(s) in a different order: %d of them moved "+
		"(#%d is %s on the left and %s on the right). Compared by vnum below, not by position",
		a.Len(), moved, firstVnum, ordinal(firstAt+1), ordinal(firstWant+1))

	vnums := make([]int64, 0, len(ai))
	for vnum := range ai {
		vnums = append(vnums, vnum)
	}
	slices.Sort(vnums)
	for _, vnum := range vnums {
		d.walk(fmt.Sprintf("%s[#%d]", path, vnum), a.Index(ai[vnum]), b.Index(bi[vnum]))
	}
	return true
}

// vnumIndex maps each element's Vnum to its position, or reports false if
// the elements have no integer Vnum field or two of them share one.
func vnumIndex(v reflect.Value) (map[int64]int, bool) {
	index := make(map[int64]int, v.Len())
	for i := 0; i < v.Len(); i++ {
		e := v.Index(i)
		for e.Kind() == reflect.Pointer || e.Kind() == reflect.Interface {
			if e.IsNil() {
				return nil, false
			}
			e = e.Elem()
		}
		if e.Kind() != reflect.Struct {
			return nil, false
		}
		f := e.FieldByName("Vnum")
		if !f.IsValid() || !f.CanInt() {
			return nil, false
		}
		if _, dup := index[f.Int()]; dup {
			return nil, false
		}
		index[f.Int()] = i
	}
	return index, true
}

// ordinal renders a 1-based position the way the sentence above reads it.
func ordinal(n int) string {
	suffix := "th"
	if n%100 < 11 || n%100 > 13 {
		switch n % 10 {
		case 1:
			suffix = "st"
		case 2:
			suffix = "nd"
		case 3:
			suffix = "rd"
		}
	}
	return fmt.Sprintf("%d%s", n, suffix)
}

// isKeywordField reports whether f holds a space-separated keyword list —
// an object's or mobile's name list, or an extra description's
// (internal/game: Object.Keywords, ExtraDesc.Keywords). Every one of them
// is spelled `Keywords string`, which is what makes matching on the name
// safe rather than lucky.
func isKeywordField(f reflect.StructField) bool {
	return f.Name == "Keywords" && f.Type.Kind() == reflect.String
}

// compareKeywords compares two keyword lists as *lists*, not as bytes.
//
// This is the one field where the yaml format does not promise to give
// back the bytes it was handed, and it is written down as a deviation
// ("The yaml format re-spaces a keyword list"): keywords are a YAML
// sequence, so the writer splits on whitespace (`strings.Fields`,
// internal/persist/world/yaml/writer.go) and the reader joins with one
// space. A classic namelist with a doubled space, a trailing space, or —
// the case that found this — a *newline* inside it does not come back
// byte for byte.
//
// Comparing the bytes here made `import --verify` refuse a conversion that
// had lost nothing, which matters more than it sounds: since yaml-only,
// `import` is the only path from an archive to a running server, it
// verifies itself by default, and a failed verification leaves the output
// unstamped and unbootable. Every fixture in this repo has single-spaced
// keywords, so it was latent — the same shape as the transcoding gap in
// docs/design/data-format.md §11.1, real but inert against everything
// checked in. `scripts/fuzz.sh` found it in seconds by putting a newline
// in the newbie zone's `staircase stair 606 rs`.
//
// That the difference is unobservable is not assumed. isname() is the only
// consumer of the string, and its C body (reference/tools/nameoracle.c)
// ends a keyword at any non-alphabetic character — so `\r\n` separates two
// keywords exactly as a space does, and the C answers "606", "rs",
// "stair" and "staircase" identically for both spellings. Checked against
// that oracle, not read off it.
func (d *differ) compareKeywords(path, a, b string) {
	if slices.Equal(strings.Fields(a), strings.Fields(b)) {
		return
	}
	d.reportf(path, "%q vs %q", a, b)
}

// format renders a scalar for a report, quoting strings so a difference
// that is only whitespace is visible rather than invisible — which, given
// that three of the string shapes this whole corpus exists for are a
// trailing space, a trailing newline and a bare carriage return, is not a
// nicety.
func format(v reflect.Value) string {
	if v.Kind() == reflect.String {
		return fmt.Sprintf("%q", v.String())
	}
	return fmt.Sprintf("%v", v.Interface())
}

func describeNil(v reflect.Value) string {
	if v.IsNil() {
		return "nil"
	}
	return "set"
}

func join(path, field string) string {
	if path == "" {
		return field
	}
	return path + "." + field
}

// sortedMapKeys is the union of both maps' keys, in a stable order, so two
// runs of the same comparison report the same differences in the same
// sequence.
func sortedMapKeys(a, b reflect.Value) []reflect.Value {
	seen := map[string]reflect.Value{}
	for _, m := range []reflect.Value{a, b} {
		for _, k := range m.MapKeys() {
			seen[fmt.Sprintf("%v", k.Interface())] = k
		}
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]reflect.Value, 0, len(names))
	for _, name := range names {
		out = append(out, seen[name])
	}
	return out
}

// summarise turns a difference list into the one line a caller prints when
// there is nothing to report in detail.
func summarise(diffs []string) string {
	if len(diffs) == 0 {
		return "identical"
	}
	return fmt.Sprintf("%d difference(s):\n    %s", len(diffs), strings.Join(diffs, "\n    "))
}
