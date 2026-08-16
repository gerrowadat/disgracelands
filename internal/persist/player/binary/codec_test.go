// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package binary

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// generateFixture builds reference/tools/pfilegen.c for the given model and
// runs it, returning the file it wrote. It skips when the compiler cannot
// produce that model.
//
// This is the closest thing to the real data that can exist in a public
// repository. The archived database is 108 real players' password hashes,
// private mail and connection hosts, and is deliberately not here — so the
// fixture is written by a C program using the same struct and the same
// compiler, with every field set to a documented function of the record
// index. Reading it checks the decoder against arithmetic, and against C's
// idea of the layout, rather than against a blob nobody can inspect.
func generateFixture(t *testing.T, m dataModel, count int) string {
	t.Helper()

	gcc, err := exec.LookPath("gcc")
	if err != nil {
		t.Skip("gcc not found")
	}
	root := repoRoot(t)
	src := filepath.Join(root, "reference", "tools", "pfilegen.c")
	inc := filepath.Join(root, "reference", "moderncserver", "src")
	if _, err := os.Stat(filepath.Join(inc, "conf.h")); err != nil {
		t.Skip("reference/moderncserver/src/conf.h not present; run its configure first")
	}

	dir := t.TempDir()
	bin := filepath.Join(dir, "pfilegen")

	args := []string{"-std=gnu89", "-fcommon", "-w", "-I" + inc, "-o", bin, src}
	if m.ptrSize == 4 {
		args = append([]string{"-m32"}, args...)
	}
	if out, err := exec.Command(gcc, args...).CombinedOutput(); err != nil {
		if m.ptrSize == 4 {
			t.Skipf("cannot build 32-bit (install gcc-multilib to enable this check): %v\n%s", err, out)
		}
		t.Fatalf("building the fixture generator: %v\n%s", err, out)
	}

	path := filepath.Join(dir, "players")
	if out, err := exec.Command(bin, path, itoa(count)).CombinedOutput(); err != nil {
		t.Fatalf("running the fixture generator: %v\n%s", err, out)
	}
	return path
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// wantRecord is the expected decoding of fixture record i. It must stay in
// step with fill_record() in reference/tools/pfilegen.c.
func wantRecord(i int) *game.PlayerRecord {
	p := &game.PlayerRecord{
		Name:        "Test" + itoa(i),
		Title:       "the Tester of " + itoa(i),
		Description: "A test character, number " + itoa(i) + ".\n",
		Host:        "host" + itoa(i) + ".example.org",
		Credential:  game.Credential{Scheme: game.SchemeLegacyDES, Hash: "ab" + pad6(i)},

		Sex:      int32(i % 3),
		Class:    int32(i % 4),
		Level:    int32(i%34 + 1),
		Hometown: int32(3000 + i),
		Weight:   int32(100 + i%100),
		Height:   int32(150 + i%50),

		Birth:     time.Unix(int64(1000000000+i), 0).UTC(),
		LastLogon: time.Unix(int64(1100000000+i), 0).UTC(),
		Played:    time.Duration(3600*i) * time.Second,

		IDNum:       int64(1 + i),
		PlayerFlags: game.Flags(1) << uint(i%20),
		AffectFlags: game.Flags(1) << uint(i%15),
		Preferences: game.Flags(1) << uint(i%22),

		WimpLevel:     int32(i),
		FreezeLevel:   int32(i % 5),
		InvisLevel:    int32(i % 35),
		LoadRoom:      game.RoomVnum(3001 + i),
		BadPasswords:  int32(i % 4),
		SpellsToLearn: int32(i * 2),
		RemortVector:  int32(i % 16),
		SpecFlags:     int32(i * 3),
		OLCZone:       int32(30 + i),

		Skills: map[int32]int32{1: int32(50 + i%50), 200: int32(i % 100)},
	}
	if i%2 == 1 {
		p.Alignment = int32(1000 - i)
	} else {
		p.Alignment = int32(-1000 + i)
	}
	// Skill 200 is zero when i is a multiple of 100, and the decoder drops
	// zero entries rather than storing them.
	if i%100 == 0 {
		delete(p.Skills, 200)
	}

	for j := range p.SavingThrows {
		p.SavingThrows[j] = int32(i*10 + j)
	}
	p.Conditions = [3]int32{int32(i % 25), -1, int32(i % 25)}

	p.Abilities = game.Abilities{
		Strength: int32(10 + i%9), StrengthPercentile: int32(i % 101),
		Intelligence: int32(10 + i%9), Wisdom: int32(10 + i%9),
		Dexterity: int32(10 + i%9), Constitution: int32(10 + i%9),
		Charisma: int32(10 + i%9),
	}
	p.Points = game.Points{
		Mana: int32(100 + i), MaxMana: int32(100 + i),
		Hit: int32(50 + i), MaxHit: int32(50 + i),
		Move: int32(80 + i), MaxMove: int32(80 + i),
		Armor: int32(-i), Gold: int32(i * 1000), BankGold: int32(i * 2000),
		Exp: int32(i * 100000), HitRoll: int32(i % 20), DamRoll: int32(i % 20),
	}

	p.Affects = []game.Affect{{
		Type: int32(1 + i%50), Duration: int32(10 + i),
		Modifier: int32(i % 10), Location: int32(i % 20),
		Bits: game.Flags(1) << uint(i%15),
	}, {
		Type: int32(51 + i%10), Duration: int32(20 + i),
		Modifier: int32(-(i % 10)), Location: int32(i % 5),
	}}

	p.Spares.Bytes = [6]int32{int32(byte(i)), int32(byte(i + 1)), int32(byte(i + 2)),
		int32(byte(i + 3)), int32(byte(i + 4)), int32(byte(i + 5))}
	p.Spares.Ints = [7]int32{int32(i * 10), int32(i * 11), int32(i * 12),
		int32(i * 13), int32(i * 14), int32(i * 15), int32(i * 16)}
	p.Spares.Longs = [5]int64{int64(i * 17), int64(i * 18), int64(i * 19),
		int64(i * 20), int64(i * 21)}

	return p
}

func pad6(n int) string {
	s := itoa(n)
	for len(s) < 6 {
		s = "0" + s
	}
	return s
}

// TestDecodeAgainstCWrittenFixture reads a file written by a C program using
// the real struct, and checks every field.
func TestDecodeAgainstCWrittenFixture(t *testing.T) {
	const count = 12

	for _, m := range []dataModel{lp64, ilp32} {
		t.Run(m.name, func(t *testing.T) {
			path := generateFixture(t, m, count)
			data, err := os.ReadFile(path) //nolint:gosec // a path this test just created
			if err != nil {
				t.Fatal(err)
			}

			c := newCodec(m)
			if len(data) != count*c.RecordSize() {
				t.Fatalf("fixture is %d bytes; %d records of %d would be %d",
					len(data), count, c.RecordSize(), count*c.RecordSize())
			}

			for i := 0; i < count; i++ {
				got, err := c.decode(data[i*c.RecordSize() : (i+1)*c.RecordSize()])
				if err != nil {
					t.Fatalf("record %d: %v", i, err)
				}
				compareRecords(t, i, got, wantRecord(i))
			}
		})
	}
}

// TestRoundTripPreservesEverySignificantByte is the acceptance check for this
// package: decode every record, re-encode it, and require that every byte
// which carries information is unchanged.
//
// It is deliberately not a byte-for-byte comparison, because this format does
// not admit one. The C server fwrites an uninitialised stack local
// (db.c:2204) after filling it field by field with strcpy, so two things in
// every record are stack residue rather than data: the padding between
// fields, and everything after the terminating NUL of each fixed-width
// string. No reader can reconstruct those, and nothing is lost by not
// reconstructing them — but a test demanding it would fail forever while
// looking like a real defect.
//
// The fixture makes this visible on purpose: it memsets each record to 0xAB
// before filling it, so the insignificant bytes are conspicuously non-zero.
func TestRoundTripPreservesEverySignificantByte(t *testing.T) {
	const count = 12

	for _, m := range []dataModel{lp64, ilp32} {
		t.Run(m.name, func(t *testing.T) {
			path := generateFixture(t, m, count)
			data, err := os.ReadFile(path) //nolint:gosec // a path this test just created
			if err != nil {
				t.Fatal(err)
			}

			c := newCodec(m)
			size := c.RecordSize()

			for i := 0; i < count; i++ {
				orig := data[i*size : (i+1)*size]
				rec, err := c.decode(orig)
				if err != nil {
					t.Fatalf("record %d: decode: %v", i, err)
				}
				again, err := c.encode(rec)
				if err != nil {
					t.Fatalf("record %d: encode: %v", i, err)
				}

				sig := c.layout.significantBytes(orig)
				for b := range orig {
					if sig[b] && orig[b] != again[b] {
						t.Errorf("record %d: significant byte %d changed: %#x -> %#x (%s)",
							i, b, orig[b], again[b], c.fieldAt(b))
						break
					}
				}
			}
		})
	}
}

// TestSignificantBytesAreMostOfTheRecord guards the comparison above from
// becoming vacuous. If a bug ever made significantBytes return mostly false,
// the round-trip test would pass while checking nothing.
func TestSignificantBytesAreMostOfTheRecord(t *testing.T) {
	c := newCodec(ilp32)
	rec := make([]byte, c.RecordSize())
	// A record with full-length strings: nearly every byte should count.
	for i := range rec {
		rec[i] = 'x'
	}
	for _, name := range []string{"name", "title", "description", "pwd", "host"} {
		p := c.layout.at(name)
		rec[p.Offset+p.Size-1] = 0
	}

	sig := c.layout.significantBytes(rec)
	padding := 0
	for _, s := range sig {
		if !s {
			padding++
		}
	}

	// 84 of the 1288 bytes are padding, and most of it has one cause: struct
	// affected_type holds 14 bytes of members but is 16 bytes wide under
	// ilp32, because its 4-byte `bitvector` forces 4-byte alignment. Times 32
	// slots, that is 64 bytes. The remaining 20 are scattered gaps before the
	// wider fields.
	const wantPadding = 84
	if padding != wantPadding {
		t.Errorf("%d padding bytes, want %d; the mask has changed", padding, wantPadding)
	}

	affectPadding := (c.layout.at("affected").Stride - 14) * maxAffect
	if affectPadding != 64 {
		t.Errorf("the affect array contributes %d padding bytes, want 64", affectPadding)
	}
}

// TestSemanticRoundTrip checks the other direction: that re-encoding and
// re-decoding yields the same record. This is what actually matters for
// migration — the bytes are a means, the record is the thing.
func TestSemanticRoundTrip(t *testing.T) {
	const count = 12

	for _, m := range []dataModel{lp64, ilp32} {
		t.Run(m.name, func(t *testing.T) {
			path := generateFixture(t, m, count)
			data, err := os.ReadFile(path) //nolint:gosec // a path this test just created
			if err != nil {
				t.Fatal(err)
			}

			c := newCodec(m)
			size := c.RecordSize()

			for i := 0; i < count; i++ {
				first, err := c.decode(data[i*size : (i+1)*size])
				if err != nil {
					t.Fatalf("record %d: %v", i, err)
				}
				raw, err := c.encode(first)
				if err != nil {
					t.Fatalf("record %d: %v", i, err)
				}
				second, err := c.decode(raw)
				if err != nil {
					t.Fatalf("record %d: %v", i, err)
				}
				compareRecords(t, i, second, first)
			}
		})
	}
}

// fieldAt names the field containing a byte offset, so a failure says which
// field is wrong rather than which byte.
func (c *codec) fieldAt(off int) string {
	best, bestOff := "(padding)", -1
	for name, p := range c.layout.byName {
		if p.Kind == kStruct {
			continue
		}
		if off >= p.Offset && off < p.Offset+p.Size && p.Offset > bestOff {
			best, bestOff = name, p.Offset
		}
	}
	return best
}

func compareRecords(t *testing.T, i int, got, want *game.PlayerRecord) {
	t.Helper()

	check := func(field string, g, w any) {
		if g != w {
			t.Errorf("record %d: %s = %v, want %v", i, field, g, w)
		}
	}

	check("Name", got.Name, want.Name)
	check("Title", got.Title, want.Title)
	check("Description", got.Description, want.Description)
	check("Host", got.Host, want.Host)
	check("Credential", got.Credential, want.Credential)
	check("Sex", got.Sex, want.Sex)
	check("Class", got.Class, want.Class)
	check("Level", got.Level, want.Level)
	check("Hometown", got.Hometown, want.Hometown)
	check("Weight", got.Weight, want.Weight)
	check("Height", got.Height, want.Height)
	check("Birth", got.Birth, want.Birth)
	check("LastLogon", got.LastLogon, want.LastLogon)
	check("Played", got.Played, want.Played)
	check("Alignment", got.Alignment, want.Alignment)
	check("IDNum", got.IDNum, want.IDNum)
	check("PlayerFlags", got.PlayerFlags, want.PlayerFlags)
	check("AffectFlags", got.AffectFlags, want.AffectFlags)
	check("Preferences", got.Preferences, want.Preferences)
	check("WimpLevel", got.WimpLevel, want.WimpLevel)
	check("FreezeLevel", got.FreezeLevel, want.FreezeLevel)
	check("InvisLevel", got.InvisLevel, want.InvisLevel)
	check("LoadRoom", got.LoadRoom, want.LoadRoom)
	check("BadPasswords", got.BadPasswords, want.BadPasswords)
	check("SpellsToLearn", got.SpellsToLearn, want.SpellsToLearn)
	check("RemortVector", got.RemortVector, want.RemortVector)
	check("SpecFlags", got.SpecFlags, want.SpecFlags)
	check("OLCZone", got.OLCZone, want.OLCZone)
	check("Abilities", got.Abilities, want.Abilities)
	check("Points", got.Points, want.Points)
	check("SavingThrows", got.SavingThrows, want.SavingThrows)
	check("Conditions", got.Conditions, want.Conditions)
	check("Spares", got.Spares, want.Spares)

	if len(got.Skills) != len(want.Skills) {
		t.Errorf("record %d: %d skills, want %d (%v vs %v)", i, len(got.Skills), len(want.Skills), got.Skills, want.Skills)
	}
	for num, pct := range want.Skills {
		if got.Skills[num] != pct {
			t.Errorf("record %d: skill %d = %d, want %d", i, num, got.Skills[num], pct)
		}
	}

	if len(got.Affects) != len(want.Affects) {
		t.Fatalf("record %d: %d affects, want %d", i, len(got.Affects), len(want.Affects))
	}
	for j := range want.Affects {
		if got.Affects[j] != want.Affects[j] {
			t.Errorf("record %d: affect %d = %+v, want %+v", i, j, got.Affects[j], want.Affects[j])
		}
	}
}
