package classic

import (
	"strings"
	"testing"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// parseOne runs a single record parser over inline text and returns the
// loader so findings can be inspected.
func newTestLoader() *loader { return &loader{dir: "testdata"} }

func TestParseRoom(t *testing.T) {
	// Room 0 of the real world, verbatim from lib/world/wld/0.wld.
	const input = `The Void~
   You don't think that you are not floating in nothing.  You can see
a strange portal located above you.
~
0 d 1
D4
The Temple of Midgaard floats above you.
~
~
0 -1 3001
S
`
	l := newTestLoader()
	r := newReader(strings.NewReader(input), "0.wld")
	room, err := l.parseRoom(r, 0)
	if err != nil {
		t.Fatalf("parseRoom: %v", err)
	}

	if room.Name != "The Void" {
		t.Errorf("Name = %q", room.Name)
	}
	if !strings.HasSuffix(room.Description, "above you.\r\n") {
		t.Errorf("Description = %q, want it to end with a CRLF", room.Description)
	}
	// "d" is bit 3.
	if want := game.Flags(1 << 3); room.Flags != want {
		t.Errorf("Flags = %#b, want %#b", room.Flags, want)
	}
	if room.SectorType != 1 {
		t.Errorf("SectorType = %d, want 1", room.SectorType)
	}

	up := room.Exits[game.Up]
	if up == nil {
		t.Fatal("no up exit parsed")
	}
	if up.ToRoom != 3001 {
		t.Errorf("up exit ToRoom = %d, want 3001", up.ToRoom)
	}
	if up.Key != -1 {
		t.Errorf("up exit Key = %d, want -1", up.Key)
	}
	if up.IsDoor() {
		t.Error("up exit reports a door, but its door flag is 0")
	}
	for dir := range room.Exits {
		if game.Direction(dir) != game.Up && room.Exits[dir] != nil {
			t.Errorf("unexpected exit in direction %s", game.Direction(dir))
		}
	}
}

func TestParseRoomExtraDescriptionsAreInReverseFileOrder(t *testing.T) {
	// The C loader prepends each extra description, so at runtime the list is
	// reversed relative to the file. Anything that stops at the first keyword
	// match therefore sees the last matching entry in the file, and that
	// behaviour has to survive the port.
	const input = `Room~
Desc~
0 0 0
E
first~
First body~
E
second~
Second body~
S
`
	l := newTestLoader()
	room, err := l.parseRoom(newReader(strings.NewReader(input), "t.wld"), 1)
	if err != nil {
		t.Fatalf("parseRoom: %v", err)
	}
	if len(room.ExtraDescs) != 2 {
		t.Fatalf("got %d extra descriptions, want 2", len(room.ExtraDescs))
	}
	if room.ExtraDescs[0].Keywords != "second" {
		t.Errorf("first entry is %q, want %q (the file's last)", room.ExtraDescs[0].Keywords, "second")
	}
}

func TestParseRoomRejectsOutOfRangeDirection(t *testing.T) {
	// The C loader indexes dir_option[] with this unchecked, which is a
	// buffer overrun. Report and drop instead.
	const input = `Room~
Desc~
0 0 0
D9
~
~
0 -1 1
S
`
	l := newTestLoader()
	room, err := l.parseRoom(newReader(strings.NewReader(input), "t.wld"), 1)
	if err != nil {
		t.Fatalf("parseRoom: %v", err)
	}
	for _, e := range room.Exits {
		if e != nil {
			t.Error("an out-of-range direction produced an exit")
		}
	}
	if !hasFinding(l, "out of range") {
		t.Errorf("no finding reported for direction D9; got %v", l.warnings)
	}
}

func TestParseMobileSimple(t *testing.T) {
	// Mob 1 of the real world, verbatim from lib/world/mob/0.mob.
	const input = `Puff dragon fractal~
Puff~
Puff the Fractal Dragon is here, contemplating a higher reality.
~
Is that some type of differential curve involving some strange, and
unknown calculus that she seems to be made out of?
~
adnopqr dkp 1000 E
26 1 -1 5d10+550 4d6+3
10000 155000
8 8 2
BareHandAttack: 12
E
`
	l := newTestLoader()
	mob, err := l.parseMobile(newReader(strings.NewReader(input), "0.mob"), 1)
	if err != nil {
		t.Fatalf("parseMobile: %v", err)
	}

	if mob.Keywords != "Puff dragon fractal" {
		t.Errorf("Keywords = %q", mob.Keywords)
	}
	if !mob.Enhanced {
		t.Error("Enhanced = false, but the type letter was E")
	}
	if mob.Level != 26 {
		t.Errorf("Level = %d, want 26", mob.Level)
	}
	// The C loader stores 20 - thac0 as the hitroll.
	if got, want := mob.HitRoll(), int32(19); got != want {
		t.Errorf("HitRoll() = %d, want %d", got, want)
	}
	// ...and ten times the file's armor class.
	if got, want := mob.ArmorClassScaled(), int32(-10); got != want {
		t.Errorf("ArmorClassScaled() = %d, want %d", got, want)
	}
	if mob.HitDice != (game.Dice{Number: 5, Size: 10, Bonus: 550}) {
		t.Errorf("HitDice = %+v, want 5d10+550", mob.HitDice)
	}
	if mob.DamageDice != (game.Dice{Number: 4, Size: 6, Bonus: 3}) {
		t.Errorf("DamageDice = %+v, want 4d6+3", mob.DamageDice)
	}
	if mob.Gold != 10000 || mob.Exp != 155000 {
		t.Errorf("Gold/Exp = %d/%d, want 10000/155000", mob.Gold, mob.Exp)
	}
	if len(mob.Especs) != 1 || mob.Especs[0].Key != "BareHandAttack" || mob.Especs[0].Value != "12" {
		t.Errorf("Especs = %+v, want one BareHandAttack: 12", mob.Especs)
	}
}

func TestParseMobileLowercasesLeadingArticle(t *testing.T) {
	// "A pelican" has to read correctly mid-sentence.
	const input = `olc pelican bird~
A pelican~
A pelican stands here.
~
Confused.
~
0 0 0 S
1 20 10 1d1+1 1d1+1
0 0
8 8 0
`
	l := newTestLoader()
	mob, err := l.parseMobile(newReader(strings.NewReader(input), "t.mob"), 2)
	if err != nil {
		t.Fatalf("parseMobile: %v", err)
	}
	if mob.ShortDesc != "a pelican" {
		t.Errorf("ShortDesc = %q, want %q", mob.ShortDesc, "a pelican")
	}
	// The long description is left alone.
	if !strings.HasPrefix(mob.LongDesc, "A pelican stands here.") {
		t.Errorf("LongDesc = %q, want its capital preserved", mob.LongDesc)
	}
	if mob.Enhanced {
		t.Error("Enhanced = true, but the type letter was S")
	}
}

func TestParseObject(t *testing.T) {
	// Object 1 from lib/world/obj/0.obj, followed by the next record's
	// header, which is how an object record actually ends.
	const input = `wings~
a pair of wings~
A pair of wings is sitting here.~
~
9 0 ae 0
6 0 0 0
5 500 10 0
A
17 2
#2
`
	l := newTestLoader()
	r := newReader(strings.NewReader(input), "0.obj")
	obj, err := l.parseObject(r, 1)
	if err != nil {
		t.Fatalf("parseObject: %v", err)
	}

	if obj.Keywords != "wings" {
		t.Errorf("Keywords = %q", obj.Keywords)
	}
	if obj.Type != 9 {
		t.Errorf("Type = %d, want 9", obj.Type)
	}
	if want := game.Flags(1<<0 | 1<<4); obj.WearFlags != want {
		t.Errorf("WearFlags = %#b, want %#b (from \"ae\")", obj.WearFlags, want)
	}
	if obj.Values != [4]int32{6, 0, 0, 0} {
		t.Errorf("Values = %v", obj.Values)
	}
	if obj.Weight != 5 || obj.Cost != 500 || obj.RentPerDay != 10 {
		t.Errorf("Weight/Cost/Rent = %d/%d/%d, want 5/500/10", obj.Weight, obj.Cost, obj.RentPerDay)
	}
	if len(obj.Affects) != 1 || obj.Affects[0] != (game.ObjAffect{Location: 17, Modifier: 2}) {
		t.Errorf("Affects = %+v, want one {17, 2}", obj.Affects)
	}

	// The record's terminating line belongs to the next record and must still
	// be readable.
	next, ok := r.getLine()
	if !ok || next != "#2" {
		t.Errorf("after parseObject, getLine() = %q, %v; want %q", next, ok, "#2")
	}
}

func TestParseObjectDrinkContainerWeightFix(t *testing.T) {
	// A load-time mutation, not a validation: the C loader rewrites the
	// weight, so the loaded world differs from the file.
	const input = `cup~
a cup~
A cup.~
~
17 0 0 0
8 8 0 0
1 10 1 0
#2
`
	l := newTestLoader()
	obj, err := l.parseObject(newReader(strings.NewReader(input), "t.obj"), 1)
	if err != nil {
		t.Fatalf("parseObject: %v", err)
	}
	if obj.Weight != 13 {
		t.Errorf("Weight = %d, want 13 (capacity 8 + 5)", obj.Weight)
	}
	if !hasFinding(l, "raises its weight") {
		t.Errorf("the weight change was not reported; got %v", l.warnings)
	}
}

func TestParseObjectAcceptsShortNumericLines(t *testing.T) {
	// The C loader accepts three fields where four are documented, defaulting
	// the fourth to zero, on both the type line and the weight line.
	const input = `thing~
a thing~
A thing.~
~
1 0 0
0 0 0 0
1 2 3
#2
`
	l := newTestLoader()
	obj, err := l.parseObject(newReader(strings.NewReader(input), "t.obj"), 1)
	if err != nil {
		t.Fatalf("parseObject: %v", err)
	}
	if obj.PermAffect != 0 || obj.MinLevel != 0 {
		t.Errorf("PermAffect/MinLevel = %d/%d, want 0/0", obj.PermAffect, obj.MinLevel)
	}
}

func TestParseZone(t *testing.T) {
	// Zone 0, verbatim from lib/world/zon/0.zon, trailing comment included:
	// the C loader's sscanf ignores it and so must this one.
	const input = `#0
Limbo - Internal~
0 99 10 2
M 0 1 1 1 	(Puff)
S
$
`
	l := newTestLoader()
	zone, err := l.parseZone(newReader(strings.NewReader(input), "0.zon"), "0.zon")
	if err != nil {
		t.Fatalf("parseZone: %v", err)
	}

	if zone.Vnum != 0 || zone.Name != "Limbo - Internal" {
		t.Errorf("Vnum/Name = %d/%q", zone.Vnum, zone.Name)
	}
	if zone.Bottom != 0 || zone.Top != 99 {
		t.Errorf("range = %d-%d, want 0-99", zone.Bottom, zone.Top)
	}
	if zone.Lifespan != 10 || zone.ResetMode != 2 {
		t.Errorf("Lifespan/ResetMode = %d/%d, want 10/2", zone.Lifespan, zone.ResetMode)
	}
	if len(zone.Commands) != 1 {
		t.Fatalf("got %d commands, want 1", len(zone.Commands))
	}
	cmd := zone.Commands[0]
	if cmd.Command != 'M' || cmd.IfFlag != 0 || cmd.Arg1 != 1 || cmd.Arg2 != 1 || cmd.Arg3 != 1 {
		t.Errorf("command = %+v, want M 0 1 1 1", cmd)
	}
}

func TestParseZoneArgumentCounts(t *testing.T) {
	// M, O, E, P and D take three arguments after the if-flag; G and R take
	// two. Getting this wrong silently shifts every argument.
	const input = `#1
Test~
100 199 30 2
M 0 100 1 101
G 1 200 1
R 0 101 200
S
`
	l := newTestLoader()
	zone, err := l.parseZone(newReader(strings.NewReader(input), "t.zon"), "t.zon")
	if err != nil {
		t.Fatalf("parseZone: %v", err)
	}
	if len(zone.Commands) != 3 {
		t.Fatalf("got %d commands, want 3", len(zone.Commands))
	}
	if got := zone.Commands[0]; got.NumArgs() != 3 || got.Arg3 != 101 {
		t.Errorf("M command = %+v, want 3 args ending in 101", got)
	}
	if got := zone.Commands[1]; got.NumArgs() != 2 || got.Arg1 != 200 || got.Arg2 != 1 {
		t.Errorf("G command = %+v, want 2 args 200, 1", got)
	}
	if got := zone.Commands[2]; got.NumArgs() != 2 || got.Arg1 != 101 || got.Arg2 != 200 {
		t.Errorf("R command = %+v, want 2 args 101, 200", got)
	}
}

func TestParseZoneSkipsIndentedComments(t *testing.T) {
	// get_line drops lines starting with '*', but an indented one reaches the
	// command loop, where the C loader skips leading space before testing.
	const input = `#1
Test~
100 199 30 2
   * an indented comment
M 0 100 1 101
S
`
	l := newTestLoader()
	zone, err := l.parseZone(newReader(strings.NewReader(input), "t.zon"), "t.zon")
	if err != nil {
		t.Fatalf("parseZone: %v", err)
	}
	if len(zone.Commands) != 1 {
		t.Errorf("got %d commands, want 1 (the indented comment is not a command)", len(zone.Commands))
	}
}

func hasFinding(l *loader, substr string) bool {
	for _, w := range l.warnings {
		if strings.Contains(w.Message, substr) {
			return true
		}
	}
	return false
}
