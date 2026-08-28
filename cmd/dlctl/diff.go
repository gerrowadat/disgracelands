// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package main

import (
	"fmt"
	"reflect"
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
			d.walk(join(path, f.Name), a.Field(i), b.Field(i))
		}

	case reflect.Slice, reflect.Array:
		if a.Len() != b.Len() {
			d.reportf(path, "%d element(s) vs %d", a.Len(), b.Len())
			// Still walk the common prefix: a length difference with
			// identical contents up to it is a different (and much less
			// alarming) finding than one where everything also moved.
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
