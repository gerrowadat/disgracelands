// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package binary

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/persist/player"
)

// The sizes gcc chooses for these two structs under each data model.
//
// Worked out by hand from the declarations and the alignment rules, and
// checked against the compiler by reference/tools/objfilelayout.c. They are
// asserted here because a wrong size is the failure that produces plausible
// nonsense rather than an error: every object after the first would be read
// from the wrong offset and come back as a different item.
func TestTheRentStructsAreTheSizeTheCompilerMakesThem(t *testing.T) {
	for _, tc := range []struct {
		model              dataModel
		wantHeader         int
		wantElem           int
		wantBitvectorAt    int
		wantAffectedAt     int
		wantAffectedStride int
	}{
		// 14 ints, and then: vnum, 4 values, flags, weight, timer, a 4-byte
		// long, and 6 two-byte affects.
		{ilp32, 56, 48, 32, 36, 2},
		// The only field that moves is the `long` bitvector, which widens to
		// 8 and drags 4 bytes of tail padding in behind the affects.
		{lp64, 56, 56, 32, 40, 2},
	} {
		t.Run(tc.model.name, func(t *testing.T) {
			c := newObjCodec(tc.model)
			if c.header.Size != tc.wantHeader {
				t.Errorf("rent_info is %d bytes, want %d", c.header.Size, tc.wantHeader)
			}
			if c.elem.Size != tc.wantElem {
				t.Errorf("obj_file_elem is %d bytes, want %d", c.elem.Size, tc.wantElem)
			}
			if got := c.elem.at("bitvector").Offset; got != tc.wantBitvectorAt {
				t.Errorf("bitvector is at %d, want %d", got, tc.wantBitvectorAt)
			}
			affects := c.elem.at("affected")
			if affects.Offset != tc.wantAffectedAt {
				t.Errorf("affected is at %d, want %d", affects.Offset, tc.wantAffectedAt)
			}
			if affects.Stride != tc.wantAffectedStride {
				t.Errorf("affected's stride is %d, want %d", affects.Stride, tc.wantAffectedStride)
			}
		})
	}
}

// sampleRentFile is a file with every field set to something distinguishable,
// so a field read from the wrong offset shows up as the wrong number rather
// than as a zero that matches by luck.
func sampleRentFile() *player.RentFile {
	f := &player.RentFile{
		Code:       player.RentRented,
		Written:    time.Unix(1_000_000_000, 0).UTC(),
		CostPerDay: 137,
		Gold:       4242,
		Bank:       999_999,
	}
	for i := range 3 {
		obj := player.StoredObject{
			Vnum:       game.ObjVnum(3000 + i),
			ExtraFlags: game.Flags(1 << uint(i)),
			Weight:     int32(10 + i),
			Timer:      int32(-1 - i),
			PermAffect: game.Flags(1 << uint(i+8)),
			Affects:    make([]game.ObjAffect, game.MaxObjAffects),
		}
		for j := range obj.Values {
			obj.Values[j] = int32(100*i + j)
		}
		for j := range obj.Affects {
			// A negative modifier in every other slot: `modifier` is an sbyte
			// and a cursed item is how that gets exercised.
			obj.Affects[j] = game.ObjAffect{Location: int32(j + 1), Modifier: int32(j * -3)}
		}
		f.Objects = append(f.Objects, obj)
	}
	return f
}

func TestARentFileSurvivesBeingWrittenAndReadBack(t *testing.T) {
	c := newObjCodec(ilp32)
	want := sampleRentFile()

	b, err := c.encode(want)
	if err != nil {
		t.Fatalf("encoding: %v", err)
	}
	if len(b) != c.header.Size+3*c.elem.Size {
		t.Errorf("encoded to %d bytes, want a %d-byte header and 3 %d-byte objects",
			len(b), c.header.Size, c.elem.Size)
	}

	got, err := c.decode(b)
	if err != nil {
		t.Fatalf("decoding: %v", err)
	}

	if got.Code != want.Code || got.CostPerDay != want.CostPerDay ||
		got.Gold != want.Gold || got.Bank != want.Bank || !got.Written.Equal(want.Written) {
		t.Errorf("header round-tripped as %+v, want %+v", *got, *want)
	}
	if len(got.Objects) != len(want.Objects) {
		t.Fatalf("read %d objects, want %d", len(got.Objects), len(want.Objects))
	}
	for i := range want.Objects {
		w, g := want.Objects[i], got.Objects[i]
		if g.Vnum != w.Vnum || g.ExtraFlags != w.ExtraFlags || g.Weight != w.Weight ||
			g.Timer != w.Timer || g.PermAffect != w.PermAffect || g.Values != w.Values {
			t.Errorf("object %d round-tripped as %+v, want %+v", i, g, w)
		}
		for j := range w.Affects {
			if g.Affects[j] != w.Affects[j] {
				t.Errorf("object %d affect %d round-tripped as %+v, want %+v",
					i, j, g.Affects[j], w.Affects[j])
			}
		}
	}
}

// A file cut short mid-object is read up to the last whole one, which is what
// the C's fread-until-feof loop does and is the right answer besides: the
// objects before the tear are still good.
func TestATruncatedRentFileKeepsTheObjectsBeforeTheTear(t *testing.T) {
	c := newObjCodec(ilp32)
	b, err := c.encode(sampleRentFile())
	if err != nil {
		t.Fatalf("encoding: %v", err)
	}

	got, err := c.decode(b[:len(b)-c.elem.Size/2])
	if err != nil {
		t.Fatalf("decoding a truncated file: %v", err)
	}
	if len(got.Objects) != 2 {
		t.Errorf("read %d objects from a file cut mid-third, want 2", len(got.Objects))
	}
}

func TestAFileTooShortForAHeaderIsAnError(t *testing.T) {
	if _, err := newObjCodec(ilp32).decode(make([]byte, 10)); err == nil {
		t.Error("decoding a 10-byte file succeeded, want an error")
	}
}

// The five buckets of get_filename (utils.c:550).
func TestRentFilesLandInTheBucketTheCWouldPutThemIn(t *testing.T) {
	s, err := NewObjectStore(player.Config{Dir: "/lib"})
	if err != nil {
		t.Fatalf("opening: %v", err)
	}

	for _, tc := range []struct{ name, want string }{
		{"Ansalon", "/lib/plrobjs/A-E/ansalon.objs"},
		{"eve", "/lib/plrobjs/A-E/eve.objs"},
		{"Fizban", "/lib/plrobjs/F-J/fizban.objs"},
		{"Krynn", "/lib/plrobjs/K-O/krynn.objs"},
		{"Tanis", "/lib/plrobjs/P-T/tanis.objs"},
		{"Zod", "/lib/plrobjs/U-Z/zod.objs"},
	} {
		got, err := s.pathFor(tc.name)
		if err != nil {
			t.Errorf("%s: %v", tc.name, err)
			continue
		}
		if got != filepath.FromSlash(tc.want) {
			t.Errorf("%s goes to %s, want %s", tc.name, got, tc.want)
		}
	}

	// The C builds this path by sprintf with no escaping at all, and gets
	// away with it only because _parse_name has already refused anything but
	// letters. This refuses rather than relying on that.
	for _, bad := range []string{"", "../etc/passwd", "zod/../../x", "a b"} {
		if _, err := s.pathFor(bad); err == nil {
			t.Errorf("pathFor(%q) succeeded, want a refusal", bad)
		}
	}
}

func TestTheStoreWritesReadsRemarksAndDeletes(t *testing.T) {
	ctx := context.Background()
	s, err := NewObjectStore(player.Config{Dir: t.TempDir()})
	if err != nil {
		t.Fatalf("opening: %v", err)
	}

	if _, err := s.LoadObjects(ctx, "zod"); !errors.Is(err, player.ErrNotFound) {
		t.Errorf("loading a character with no rent file gave %v, want ErrNotFound", err)
	}

	want := sampleRentFile()
	if err := s.SaveObjects(ctx, "zod", want); err != nil {
		t.Fatalf("saving: %v", err)
	}
	got, err := s.LoadObjects(ctx, "zod")
	if err != nil {
		t.Fatalf("loading: %v", err)
	}
	if got.Code != player.RentRented || len(got.Objects) != 3 {
		t.Errorf("read back %s with %d objects, want rented with 3", got.Code, len(got.Objects))
	}
	if !got.Code.KeepsLoadRoom() {
		t.Error("a rented file does not keep the load room, and paying for a bed should buy waking up in it")
	}

	// Crash_load turns the file it has just read into a crash file, leaving
	// the objects alone.
	at := time.Unix(1_700_000_000, 0).UTC()
	if err := s.MarkCrashed(ctx, "zod", at); err != nil {
		t.Fatalf("re-marking: %v", err)
	}
	got, err = s.LoadObjects(ctx, "zod")
	if err != nil {
		t.Fatalf("loading after re-marking: %v", err)
	}
	if got.Code != player.RentCrash {
		t.Errorf("re-marked file reads as %s, want crash", got.Code)
	}
	if !got.Written.Equal(at) {
		t.Errorf("re-marked file is dated %s, want %s", got.Written, at)
	}
	if len(got.Objects) != 3 {
		t.Errorf("re-marking lost objects: %d left, want 3", len(got.Objects))
	}
	if got.Code.KeepsLoadRoom() {
		t.Error("a crash file keeps the load room, and it should send them to the temple")
	}

	if err := s.DeleteObjects(ctx, "zod"); err != nil {
		t.Fatalf("deleting: %v", err)
	}
	if _, err := s.LoadObjects(ctx, "zod"); !errors.Is(err, player.ErrNotFound) {
		t.Errorf("loading after deleting gave %v, want ErrNotFound", err)
	}
	// Deleting what is not there is not an error: the C's unlink failure is
	// ignored on this path too.
	if err := s.DeleteObjects(ctx, "zod"); err != nil {
		t.Errorf("deleting twice: %v", err)
	}
}

func TestAReadOnlyStoreRefusesToWrite(t *testing.T) {
	ctx := context.Background()
	s, err := NewObjectStore(player.Config{Dir: t.TempDir(), ReadOnly: true})
	if err != nil {
		t.Fatalf("opening: %v", err)
	}
	if err := s.SaveObjects(ctx, "zod", sampleRentFile()); err == nil {
		t.Error("a read-only store wrote a rent file")
	}
}

// A house file is the same record with no header in front.
func TestABareObjectSequenceRoundTrips(t *testing.T) {
	want := sampleRentFile().Objects

	b, err := EncodeStoredObjects(want)
	if err != nil {
		t.Fatalf("encoding: %v", err)
	}
	if len(b) != len(want)*newObjCodec(ilp32).elem.Size {
		t.Errorf("encoded to %d bytes, want %d records with no header",
			len(b), len(want))
	}

	got, err := DecodeStoredObjects(b)
	if err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("read %d objects, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].Vnum != want[i].Vnum || got[i].Values != want[i].Values {
			t.Errorf("object %d round-tripped as %+v, want %+v", i, got[i], want[i])
		}
	}

	// A trailing partial record is ignored, as the C's read loop does.
	short, err := DecodeStoredObjects(b[:len(b)-3])
	if err != nil {
		t.Fatalf("decoding a truncated sequence: %v", err)
	}
	if len(short) != len(want)-1 {
		t.Errorf("a truncated sequence read %d objects, want %d", len(short), len(want)-1)
	}
}
