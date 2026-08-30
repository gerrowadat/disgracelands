// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// A domain used only by this file, so the tests below assert about Set
// itself rather than about whichever real domain happens to be converted.
type testFlag int

const (
	testZero testFlag = 0
	testOne  testFlag = 1
	testTwo  testFlag = 2
	testHigh testFlag = 40
)

func TestSetBasics(t *testing.T) {
	var s Set[testFlag]
	if !s.Empty() || s.Raw() != 0 {
		t.Fatalf("the zero value is not empty: %#v", s)
	}
	s = s.With(testOne, testHigh)
	if !s.Has(testOne) || !s.Has(testHigh) || s.Has(testTwo) {
		t.Errorf("With(one, high) gave %v", s.Members())
	}
	if got, want := s.Raw(), uint64(1<<1|1<<40); got != want {
		t.Errorf("Raw = %#x, want %#x", got, want)
	}
	if !s.HasAny(testTwo, testOne) {
		t.Error("HasAny(two, one) is false with one set")
	}
	if s.HasAll(testTwo, testOne) {
		t.Error("HasAll(two, one) is true with only one set")
	}
	if !s.HasAll() {
		t.Error("HasAll() with no arguments is false; the C's HAS_BITS against a zero mask is true")
	}
	if got := s.Without(testHigh); got != NewSet(testOne) {
		t.Errorf("Without(high) gave %v", got.Members())
	}
	if got := s.Toggle(testOne, testTwo); got != NewSet(testTwo, testHigh) {
		t.Errorf("Toggle(one, two) gave %v", got.Members())
	}
}

// TestSetIteratesInBitOrder is docs/design/idiomatic-go.md §2.2's second
// hazard, asserted rather than assumed: iteration order is player-visible
// wherever a set is printed, and the whole reason Set is a bit vector
// rather than a map is that a map's order is not an order at all.
func TestSetIteratesInBitOrder(t *testing.T) {
	s := NewSet(testHigh, testZero, testTwo)
	if got, want := s.Members(), []testFlag{testZero, testTwo, testHigh}; !slices.Equal(got, want) {
		t.Errorf("Members = %v, want %v", got, want)
	}
	// The early return from All's yield stops the loop rather than
	// running on.
	var seen []testFlag
	for v := range s.All() {
		seen = append(seen, v)
		if v == testTwo {
			break
		}
	}
	if want := []testFlag{testZero, testTwo}; !slices.Equal(seen, want) {
		t.Errorf("a broken-out-of range gave %v, want %v", seen, want)
	}
}

func TestSetOperations(t *testing.T) {
	a := NewSet(testZero, testOne)
	b := NewSet(testOne, testTwo)
	if got := a.Union(b); got != NewSet(testZero, testOne, testTwo) {
		t.Errorf("Union = %v", got.Members())
	}
	if got := a.Intersect(b); got != NewSet(testOne) {
		t.Errorf("Intersect = %v", got.Members())
	}
	if got := a.Minus(b); got != NewSet(testZero) {
		t.Errorf("Minus = %v", got.Members())
	}
	if !a.Overlaps(b) || a.Overlaps(NewSet(testHigh)) {
		t.Error("Overlaps disagrees with Intersect")
	}
}

// TestSetIgnoresAnIndexWithNoBit pins bitOf's guard. A domain constant is
// never out of range; a raw index read off disk can be, and the C's
// `1 << n` for n outside 0..63 is undefined behaviour rather than a bit.
func TestSetIgnoresAnIndexWithNoBit(t *testing.T) {
	for _, v := range []testFlag{-1, 64, 1000} {
		if s := NewSet(v); !s.Empty() {
			t.Errorf("NewSet(%d) set %#x", v, s.Raw())
		}
		if NewSet(testOne).Has(v) {
			t.Errorf("Has(%d) is true", v)
		}
	}
}

func TestSetRawRoundTrip(t *testing.T) {
	const raw = uint64(1)<<3 | uint64(1)<<40
	s := SetFromRaw[testFlag](raw)
	if s.Raw() != raw {
		t.Errorf("Raw = %#x, want %#x", s.Raw(), raw)
	}
	if s.String() != FlagLetters(raw) {
		t.Errorf("String = %q, want the letter encoding %q", s.String(), FlagLetters(raw))
	}
	if !SetFromRaw[testFlag](1<<40).ExceedsCRange() || SetFromRaw[testFlag](1<<31).ExceedsCRange() {
		t.Error("ExceedsCRange disagrees with CFlagLimit")
	}
}

// --- the OR trap -------------------------------------------------------

// TestNoFlagConstantIsCombinedWithAnOperator is the check that exists
// because this exact mistake was made twice, in the first hour of the first
// domain's conversion, and one of the two would have shipped.
//
// A domain's constants are bit *indices* now, not masks
// (docs/design/idiomatic-go.md §4.1). So the C-shaped idiom the port
// was full of —
//
//	room.Flags.HasAny(RoomNoMob | RoomDeathTrap)
//
// — still compiles, because `2 | 1` is a perfectly good `RoomFlag`. It is
// `RoomIndoors`. The variadic form (`HasAny(RoomNoMob, RoomDeathTrap)`) is
// what was meant, and nothing in the toolchain distinguishes them: the
// types are right, the arity is right, and the answer is silently about a
// different flag. `internal/game/live.go`'s "a mobile will not walk into a
// NOMOB room" was one of the two, and it was caught by a test that existed
// only because somebody had written it years earlier; `internal/session`'s
// summon check was the other, and nothing at all was watching it.
//
// This is the same shape of hazard §3.1 complains about in the *old* model
// — a mistake with no diagnostic anywhere in the toolchain — so trading one
// for the other would have been a poor bargain. The check is a source scan
// rather than a type rule because Go has no way to make it a type rule: any
// `~int` supports `|`, and the result of OR-ing two valid indices is
// another valid index.
//
// It is deliberately broad: *any* binary operator between two flag
// constants of a converted domain, not just `|`. `&`, `&^` and `^` are the
// same mistake wearing different hats, and none of them has a legitimate
// use — Set's own methods are the only thing that should be doing
// arithmetic on these values.
func TestNoFlagConstantIsCombinedWithAnOperator(t *testing.T) {
	names := typedFlagConstants(t)
	if len(names) == 0 {
		t.Fatal("found no typed flag constants at all; this check has stopped checking anything")
	}
	t.Logf("watching %d flag constants across %d converted domain(s)", len(names), countDomains(names))

	// Every Go package in the tree that can name them. reference/ is C and
	// is out of scope by rule (docs/design/idiomatic-go.md §5).
	for _, root := range []string{"..", "../../cmd", "../../test"} {
		scanForFlagArithmetic(t, root, names)
	}
}

// typedFlagConstants reads this package's own source for `const` blocks
// declaring values of a `<Domain>Flag` type, and returns the constant
// names. Derived from the declarations rather than listed, so a domain
// converted by a later step is covered the moment it lands and nobody has
// to remember to add it here.
func typedFlagConstants(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	sources, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("listing internal/game: %v", err)
	}
	fset := token.NewFileSet()
	for _, path := range sources {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		for _, decl := range f.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.CONST {
				continue
			}
			var domain string
			for _, spec := range gd.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				if id, ok := vs.Type.(*ast.Ident); ok {
					domain = id.Name
				}
				// An untyped continuation line inherits the block's
				// last stated type, iota-style.
				if !strings.HasSuffix(domain, "Flag") {
					continue
				}
				for _, n := range vs.Names {
					out[n.Name] = domain
				}
			}
		}
	}
	return out
}

func countDomains(names map[string]string) int {
	seen := map[string]bool{}
	for _, d := range names {
		seen[d] = true
	}
	return len(seen)
}

func scanForFlagArithmetic(t *testing.T, root string, names map[string]string) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "testdata" || d.Name() == "reference" {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return fmt.Errorf("parsing %s: %w", path, err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			be, ok := n.(*ast.BinaryExpr)
			if !ok {
				return true
			}
			switch be.Op {
			case token.OR, token.AND, token.AND_NOT, token.XOR:
			default:
				return true
			}
			ld, lok := flagDomainOf(be.X, names)
			rd, rok := flagDomainOf(be.Y, names)
			if lok && rok && ld == rd {
				t.Errorf("%s: %s constants combined with %q — these are bit indices, "+
					"not masks, so this is arithmetic on the wrong thing. Use Set's "+
					"variadic methods: With(a, b), Without(a, b), HasAny(a, b), HasAll(a, b).",
					fset.Position(be.Pos()), ld, be.Op)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
}

// flagDomainOf reports the domain of a bare `RoomDark` or a qualified
// `game.RoomDark`, and nothing else — a variable holding one is not a
// constant and is not what this is looking for.
func flagDomainOf(e ast.Expr, names map[string]string) (string, bool) {
	switch v := e.(type) {
	case *ast.Ident:
		d, ok := names[v.Name]
		return d, ok
	case *ast.SelectorExpr:
		if pkg, ok := v.X.(*ast.Ident); ok && pkg.Name == "game" {
			d, ok := names[v.Sel.Name]
			return d, ok
		}
	case *ast.ParenExpr:
		return flagDomainOf(v.X, names)
	}
	return "", false
}
