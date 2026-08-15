package classic

import (
	"strings"
	"testing"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// shop 3000 from lib/world/shp/30.shp, verbatim, with the file's banner.
const realShopFile = `CircleMUD v3.0 Shop File~
#3000~
3050
3051
-1
1.15
0.15
2
3
-1
%s Sorry, I haven't got exactly that item.~
%s You don't seem to have that.~
%s I don't buy such items.~
%s That is too expensive for me!~
%s You can't afford it!~
%s That'll be %d coins, please.~
%s You'll get %d coins for it!~
0
2
3000
2
3033
-1
0
28
0
0
$~
`

func TestParseShopFile(t *testing.T) {
	l := newTestLoader()
	w := &game.World{}
	if err := l.parseShopFile(w, newReader(strings.NewReader(realShopFile), "30.shp"), "30.shp"); err != nil {
		t.Fatalf("parseShopFile: %v", err)
	}
	if len(w.Shops) != 1 {
		t.Fatalf("got %d shops, want 1", len(w.Shops))
	}
	s := w.Shops[0]

	if s.Vnum != 3000 {
		t.Errorf("Vnum = %d, want 3000", s.Vnum)
	}
	if len(s.Producing) != 2 || s.Producing[0] != 3050 || s.Producing[1] != 3051 {
		t.Errorf("Producing = %v, want [3050 3051]", s.Producing)
	}
	if s.ProfitBuy != 1.15 || s.ProfitSell != 0.15 {
		t.Errorf("profits = %v/%v, want 1.15/0.15", s.ProfitBuy, s.ProfitSell)
	}
	if len(s.BuyTypes) != 2 || s.BuyTypes[0].Type != 2 || s.BuyTypes[1].Type != 3 {
		t.Errorf("BuyTypes = %+v, want types 2 and 3", s.BuyTypes)
	}
	if !strings.HasPrefix(s.Messages[game.MsgNoSuchItem1], "%s Sorry") {
		t.Errorf("first message = %q", s.Messages[game.MsgNoSuchItem1])
	}
	if !strings.Contains(s.Messages[game.MsgBuy], "%d coins") {
		t.Errorf("buy message = %q", s.Messages[game.MsgBuy])
	}
	if s.Keeper != 3000 {
		t.Errorf("Keeper = %d, want 3000", s.Keeper)
	}
	if len(s.Rooms) != 1 || s.Rooms[0] != 3033 {
		t.Errorf("Rooms = %v, want [3033]", s.Rooms)
	}
	if s.Temper != 0 || s.TradeWith != 2 {
		t.Errorf("Temper/TradeWith = %d/%d, want 0/2", s.Temper, s.TradeWith)
	}
	if s.Open1 != 0 || s.Close1 != 28 {
		t.Errorf("Open1/Close1 = %d/%d, want 0/28", s.Open1, s.Close1)
	}
}

func TestParseShopHeadersAreTildeTerminated(t *testing.T) {
	// Shop files are the only ones whose structural lines go through
	// fread_string, so "#3000~" is a record header and "#3000" alone would
	// swallow the rest of the file looking for its terminator.
	l := newTestLoader()
	w := &game.World{}
	err := l.parseShopFile(w, newReader(strings.NewReader("$~\n"), "empty.shp"), "empty.shp")
	if err != nil {
		t.Fatalf("parsing a shop file containing only its terminator: %v", err)
	}
	if len(w.Shops) != 0 {
		t.Errorf("got %d shops from an empty file, want 0", len(w.Shops))
	}
}

func TestParseShopUnterminatedFileIsAnError(t *testing.T) {
	// The C loader loops forever on this, since fread_string exits the process
	// only when fgets fails and the loop never re-checks.
	l := newTestLoader()
	w := &game.World{}
	if err := l.parseShopFile(w, newReader(strings.NewReader("CircleMUD v3.0~\n"), "t.shp"), "t.shp"); err == nil {
		t.Error("a shop file with no '$' terminator parsed successfully")
	}
}

func TestParseShopOldFormatUsesFixedLengthLists(t *testing.T) {
	// Without the v3.0 banner the produce list is exactly five entries and
	// -1 is a filler rather than a terminator, so a shop with two products
	// still consumes five lines.
	const old = `#100~
200
201
-1
-1
-1
1.0
1.0
2
-1
-1
-1
-1
a~
b~
c~
d~
e~
f~
g~
0
0
300
0
400
0
0
0
0
$~
`
	l := newTestLoader()
	w := &game.World{}
	if err := l.parseShopFile(w, newReader(strings.NewReader(old), "t.shp"), "t.shp"); err != nil {
		t.Fatalf("parseShopFile: %v", err)
	}
	if len(w.Shops) != 1 {
		t.Fatalf("got %d shops, want 1", len(w.Shops))
	}
	s := w.Shops[0]
	if len(s.Producing) != 2 {
		t.Errorf("Producing = %v, want the two real entries with the -1 fillers dropped", s.Producing)
	}
	if len(s.Rooms) != 1 || s.Rooms[0] != 400 {
		t.Errorf("Rooms = %v, want [400]", s.Rooms)
	}
}

func TestShopMessageValidation(t *testing.T) {
	// read_shop_message() rejects a message and stores NULL, which is why a
	// misconfigured keeper says nothing rather than something broken.
	tests := []struct {
		name  string
		index int
		msg   string
		want  string
	}{
		{"plain message kept", game.MsgNoSuchItem1, "%s Sorry.", "%s Sorry."},
		{"buy message may use %d after %s", game.MsgBuy, "%s That'll be %d coins.", "%s That'll be %d coins."},
		{"%d before %s is rejected", game.MsgBuy, "%d coins, %s.", ""},
		{"%d outside a buy or sell message is rejected", game.MsgNoSuchItem1, "%s %d", ""},
		{"unknown specifier is rejected", game.MsgNoSuchItem1, "%s %q", ""},
		{"two %s are rejected", game.MsgNoSuchItem1, "%s and %s", ""},
		{"escaped percent is fine", game.MsgNoSuchItem1, "100%% sure, %s", "100%% sure, %s"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := newTestLoader()
			r := newReader(strings.NewReader(""), "t.shp")
			if got := l.validateShopMessage(r, "shop #1", tt.index, tt.msg); got != tt.want {
				t.Errorf("validateShopMessage(%q) = %q, want %q", tt.msg, got, tt.want)
			}
		})
	}
}

func TestShopReferencesAreResolved(t *testing.T) {
	// add_to_list() runs real_object() over the produce list as it reads and
	// silently drops anything that does not resolve, so a shop can end up
	// producing fewer things than its file lists.
	l := newTestLoader()
	w := &game.World{
		Objects: []*game.ObjDef{{Vnum: 200}},
		Mobiles: []*game.MobDef{{Vnum: 300}},
		Rooms:   []*game.RoomDef{{Vnum: 400}},
		Shops: []*game.ShopDef{{
			Vnum:      1,
			Producing: []game.ObjVnum{200, 999},
			Keeper:    300,
			Rooms:     []game.RoomVnum{400},
		}},
	}
	l.resolveShopReferences(w)

	s := w.Shops[0]
	if len(s.Producing) != 1 || s.Producing[0] != 200 {
		t.Errorf("Producing = %v, want only the object that exists", s.Producing)
	}
	if s.Keeper != 300 {
		t.Errorf("Keeper = %d, want 300", s.Keeper)
	}
	if !hasFinding(l, "does not exist") {
		t.Error("the dropped product was not reported")
	}
}

func TestShopKeeperMissingIsCleared(t *testing.T) {
	l := newTestLoader()
	w := &game.World{Shops: []*game.ShopDef{{Vnum: 1, Keeper: 999}}}
	l.resolveShopReferences(w)

	if w.Shops[0].Keeper != game.NoMob {
		t.Errorf("Keeper = %d, want %d for a mob that does not exist", w.Shops[0].Keeper, game.NoMob)
	}
}
