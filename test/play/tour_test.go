// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

//go:build play

package play

import (
	"strconv"
	"strings"
	"testing"
)

// The tour is a corridor: room 3001 at the south end, one feature room per
// step north, room 3016 at the north end (examples/mini/README.md). These
// name the steps rather than the vnums, because a test that says
// `toRoom(roomArmory)` reads like the thing a player did.
const (
	roomTestingGrounds = iota
	roomHallOfMovement
	roomDoorRoom
	roomLockedVault
	roomArmory
	roomClutteredCloset
	roomDiningHall
	roomSparringRing
	roomRestingRoom
	roomGuildhall
	roomGeneralStore
	roomBank
	roomTravelersRest
	roomPostOffice
	roomNoticeBoard
	roomGraduationHall
)

// tourRoomNames is what each of those rooms calls itself, which is how a test
// checks it actually arrived.
var tourRoomNames = []string{
	"The Testing Grounds", "Hall of Movement", "The Door Room", "The Locked Vault",
	"The Armory", "The Cluttered Closet", "The Dining Hall", "The Sparring Ring",
	"The Resting Room", "Outside the Guildhall", "The General Store",
	"The Bank of Testing", "The Travelers' Rest", "The Post Office",
	"The Notice Board", "Graduation Hall",
}

// toRoom walks a character standing in the start room up the corridor to the
// n'th room of the tour, and fails the test if it does not end up there.
//
// The two doors are handled on the way, because they are the reason a test
// cannot simply send north n times: the Door Room's is closed and the Locked
// Vault's is closed *and* locked, and the key for the second is lying on the
// floor of the first. That is the tutorial's own lesson and it is also this
// helper's whole body.
func (c *client) toRoom(n int) string {
	c.t.Helper()

	var out string
	for at := roomTestingGrounds; at < n; at++ {
		switch at {
		case roomDoorRoom:
			c.do("get key")
			c.do("open door")
		case roomLockedVault:
			c.do("unlock door with key")
			c.do("open door")
		}
		out = c.do("north")
	}
	if !strings.Contains(out, tourRoomNames[n]) && n != roomTestingGrounds {
		c.t.Fatalf("walking to %s, the last move printed:\n%s", tourRoomNames[n], out)
	}
	return out
}

// arrive is the usual opening of a tour test: a fresh mortal, walked to one
// room, with what that room printed on arrival.
func arrive(t *testing.T, l lib, name string, room int) (*mud, *client, string) {
	t.Helper()

	m := start(t, l)
	c := m.dial()
	c.create(name, "tourpass", "m", "w")
	return m, c, c.toRoom(room)
}

// TestTheStartRoom. A character created on a fresh roster lands in
// game.MortalStartRoom -- 3001, hardcoded, not something a world file can
// override (internal/game/world.go), which is exactly why the tutorial
// renumbered its corridor to start there. If this fails, a new character is
// "nowhere at all" and nothing else in the suite means anything.
func TestTheStartRoom(t *testing.T) {
	for _, l := range bothFormats {
		t.Run(l.name, func(t *testing.T) {
			m := start(t, l)
			c := m.dial()
			c.create("Tourist", "tourpass", "m", "w")

			contains(t, "arriving in the world", c.do("look"),
				"The Testing Grounds",
				"Welcome to the Testing Grounds",
				"[ Exits: n ]",
			)
			m.noServerErrors()
		})
	}
}

// TestMovementAndLooking, the Hall of Movement's own lesson: the directions,
// their one-letter forms, `look`, `exits` and `score`.
func TestMovementAndLooking(t *testing.T) {
	m, c, out := arrive(t, miniClassic, "Tourist", roomHallOfMovement)

	contains(t, "walking north", out, "Hall of Movement", "[ Exits: n s ]")
	contains(t, "look", c.do("look"), "Hall of Movement")
	contains(t, "exits", c.do("exits"), "Obvious exits:", "North -", "South -")

	// The one-letter forms are the same commands: `s` back, `n` forward.
	contains(t, "s", c.do("s"), "The Testing Grounds")
	contains(t, "n", c.do("n"), "Hall of Movement")

	// A direction with nothing behind it, which is most of them here.
	contains(t, "east from a corridor", c.do("east"), "Alas, you cannot go that way...")

	contains(t, "score", c.do("score"), "You have", "hit,", "mana and", "movement points.",
		"This ranks you as Tourist", "(level 1)", "You are standing.")

	// Walking costs movement points, and the prompt is where a player sees
	// it (issue #189). Every room on the tour is SECT_INSIDE, so a step is
	// (1 + 1) / 2 = 1 — the number on the prompt goes down by one per room
	// and comes back up only on the regeneration tick.
	before := promptMovement(t, c.do("look"))
	after := promptMovement(t, c.do("south"))
	if after != before-1 {
		t.Errorf("a step south left %d movement points on the prompt, want %d", after, before-1)
	}

	m.noServerErrors()
}

// promptMovement reads the V figure off the last prompt in some output.
//
// The prompt is the only place movement points are shown as a player plays,
// which is what makes it the right thing to assert on: `score` would prove
// the field changed, and the prompt proves the player can see it.
func promptMovement(t *testing.T, out string) int {
	t.Helper()

	i := strings.LastIndex(out, promptMarker)
	if i < 0 {
		t.Fatalf("no prompt in:\n%s", out)
	}
	// Back up over the digits immediately before the "V".
	j := i
	for j > 0 && out[j-1] >= '0' && out[j-1] <= '9' {
		j--
	}
	n, err := strconv.Atoi(out[j:i])
	if err != nil {
		t.Fatalf("the prompt %q has no movement figure: %v", out[j:i+len(promptMarker)], err)
	}
	return n
}

// TestDoors, both of them: the plain closed one, and the locked one whose key
// is a room behind it.
func TestDoors(t *testing.T) {
	m, c, out := arrive(t, miniClassic, "Tourist", roomDoorRoom)

	contains(t, "the Door Room", out, "The Door Room", "A rusty key is lying on the floor here.")

	// Closed means closed, before anything else is tried.
	contains(t, "walking into a closed door", c.do("north"), "It seems to be closed.")
	contains(t, "opening it", c.do("open door"), "Okay.")
	contains(t, "closing it again", c.do("close door"), "Okay.")
	contains(t, "still closed", c.do("north"), "It seems to be closed.")

	c.do("open door")
	c.do("get key")
	contains(t, "through the first door", c.do("north"), "The Locked Vault")

	// Locked is a second state on top of closed: opening is refused until
	// the lock is undone, and undoing the lock does not open it.
	contains(t, "opening a locked door", c.do("open door"), "It seems to be locked.")
	contains(t, "unlocking", c.do("unlock door with key"), "*Click*")
	contains(t, "walking through an unlocked but shut door", c.do("north"), "It seems to be closed.")
	contains(t, "opening it now", c.do("open door"), "Okay.")
	contains(t, "through the second door", c.do("north"), "The Armory")

	m.noServerErrors()
}

// TestGettingAndWearing: the Armory, and the whole of what a character does
// with an object -- pick it up, put it on, look at what is on, take it off,
// drop it.
func TestGettingAndWearing(t *testing.T) {
	m, c, out := arrive(t, miniClassic, "Tourist", roomArmory)

	contains(t, "the Armory", out, "The Armory",
		"A plain, unadorned training sword has been left on the ground.",
		"A leather tunic is folded neatly on a rack.",
		"A small brass lantern sits here, ready to be lit.")

	contains(t, "get all", c.do("get all"),
		"You get a training sword.", "You get a leather tunic.", "You get a small brass lantern.")
	contains(t, "inventory", c.do("inventory"),
		"You are carrying:", "a training sword", "a leather tunic", "a small brass lantern")

	contains(t, "wear", c.do("wear tunic"), "You wear a leather tunic on your body.")
	contains(t, "wield", c.do("wield sword"), "You wield a training sword.")
	contains(t, "equipment", c.do("equipment"),
		"You are using:", "<worn on body>", "a leather tunic", "<wielded>", "a training sword")

	// Worn is not carried: the shopkeeper's "you don't seem to have that"
	// for a wielded weapon is this same distinction, and it surprises
	// people.
	missing(t, "inventory while wearing", c.do("inventory"), "a leather tunic")

	contains(t, "remove", c.do("remove tunic"), "You stop using a leather tunic.")
	contains(t, "inventory after removing", c.do("inventory"), "a leather tunic")
	contains(t, "drop", c.do("drop tunic"), "You drop a leather tunic.")
	contains(t, "the floor has it back", c.do("look"), "A leather tunic is folded neatly on a rack.")

	m.noServerErrors()
}

// TestContainers: the Cluttered Closet. The bag is open and the chest is
// shut, which is the point of having both.
func TestContainers(t *testing.T) {
	m, c, out := arrive(t, miniClassic, "Tourist", roomClutteredCloset)

	contains(t, "the Cluttered Closet", out, "The Cluttered Closet",
		"A leather bag lies open on the floor.",
		"A small wooden chest sits against the wall, its lid closed.")

	// `look` describes a container; it does not open it. The room text used
	// to claim otherwise, and a live playtest is what caught it
	// (examples/mini/README.md, "Two things this was wrong about"). This
	// asserts the behaviour that survived, so a change to do_look that
	// "fixed" it would fail here.
	look := c.do("look chest")
	contains(t, "look at a container", look, "A small wooden chest sits against the wall")
	missing(t, "look at a container", look, "When you look inside")

	contains(t, "opening the chest", c.do("open chest"), "Okay.")
	contains(t, "examine", c.do("examine chest"), "When you look inside, you see:", "a gold ring")

	contains(t, "getting from a container", c.do("get ring from chest"),
		"You get a gold ring from a small wooden chest.")
	c.do("get bag")
	contains(t, "putting into a container", c.do("put ring in bag"),
		"You put a gold ring in a leather bag.")
	contains(t, "examine the bag", c.do("examine bag"), "When you look inside, you see:", "a gold ring")
	contains(t, "taking it back out", c.do("get ring from bag"),
		"You get a gold ring from a leather bag.")

	m.noServerErrors()
}

// TestEatingAndDrinking: the Dining Hall.
func TestEatingAndDrinking(t *testing.T) {
	m, c, out := arrive(t, miniClassic, "Tourist", roomDiningHall)

	contains(t, "the Dining Hall", out, "The Dining Hall",
		"A loaf of bread sits on the table, still faintly warm.",
		"An empty waterskin hangs from a hook on the wall.",
		"A stone fountain bubbles quietly in the corner.")

	// do_eat wants the food in hand: eating off the floor is not a thing,
	// however much the room's own text reads as though it were.
	contains(t, "eating what is on the floor", c.do("eat bread"), "You don't seem to have a bread.")
	c.do("get bread")
	// A character is created with full conditions (do_start), so the honest
	// assertion is that the loaf in hand reached do_eat at all rather than
	// which branch of it answered. Both of these are do_eat; "You don't seem
	// to have a bread" above is not, which is the distinction that matters.
	eat := c.do("eat bread")
	if !strings.Contains(eat, "You eat a loaf of bread.") &&
		!strings.Contains(eat, "You are too full to eat more!") {
		t.Errorf("eating said something unexpected:\n%s", eat)
	}

	c.do("get waterskin")
	contains(t, "filling from a fountain", c.do("fill waterskin from fountain"),
		"You gently fill a waterskin from a stone fountain.")

	// A new character is created full (do_drink's own condition check), so
	// the honest assertion is that the command reached the drinking code at
	// all rather than which branch of it. Both answers are correct; "Huh?!?"
	// and silence are not.
	drink := c.do("drink waterskin")
	if !strings.Contains(drink, "You drink the water") &&
		!strings.Contains(drink, "Your stomach can't contain anymore!") {
		t.Errorf("drinking said something unexpected:\n%s", drink)
	}

	m.noServerErrors()
}

// TestFightingAndTheCorpse: the Sparring Ring. A whole fight, from `kill` to
// the corpse and what is in it.
func TestFightingAndTheCorpse(t *testing.T) {
	m, c, _ := arrive(t, miniClassic, "Tourist", roomArmory)

	// Armed on the way through, which is what the tutorial has just spent a
	// room teaching -- and it is also what keeps this test to a handful of
	// combat rounds instead of a few dozen. Barehanded, a level 1 warrior
	// against 2d4+4 hit points takes long enough that the fight, not the
	// server, is what the suite spends its time on.
	c.do("get sword")
	c.do("wield sword")
	out := c.north(roomSparringRing - roomArmory)

	contains(t, "the Sparring Ring", out, "The Sparring Ring",
		"A straw-stuffed training dummy stands here, patiently waiting to be hit.")

	c.do("kill dummy")
	// The rest of the fight happens on the violence pulse rather than in
	// reply to anything typed, so this waits on the socket rather than
	// sending more commands -- hitting an opponent already engaged just
	// answers "You do the best you can!" and does not hurry anything along.
	// The experience, not the R.I.P., is what is waited for: they arrive in
	// that order, so waiting for the death message would read the transcript
	// one line before the line the test is about.
	c.expect("experience points.")

	contains(t, "after the kill", c.transcript(),
		"a training dummy is dead!  R.I.P.", "You receive", "experience points.")
	contains(t, "the room now", c.do("look"), "The corpse of a training dummy is lying here.")
	contains(t, "looting", c.do("get all corpse"), "from the corpse of a training dummy")

	m.noServerErrors()
}

// TestUsingMagicItems: the loot beside the dummy. A scroll, a potion, a wand
// and a staff are four different item types with four different verbs, and
// `use` needs the thing held first.
func TestUsingMagicItems(t *testing.T) {
	m, c, _ := arrive(t, miniClassic, "Tourist", roomSparringRing)

	c.do("get all")
	contains(t, "reciting", c.do("recite scroll"), "You recite a scroll which dissolves.")
	contains(t, "quaffing", c.do("quaff potion"), "You quaff a potion.")

	// `use` is do_use, which wants the item in the hold position: this is
	// the difference between a wand and a weapon, and the message for
	// getting it wrong is its own assertion.
	contains(t, "using a wand from the pack", c.do("use wand dummy"),
		"You don't seem to be holding a wand.")
	c.do("hold wand")
	contains(t, "using a held wand", c.do("use wand dummy"), "You point a wand at a training dummy.")

	m.noServerErrors()
}

// TestPositions: the Resting Room, and the four positions a living character
// moves between.
func TestPositions(t *testing.T) {
	m, c, _ := arrive(t, miniClassic, "Tourist", roomRestingRoom)

	contains(t, "rest", c.do("rest"), "You sit down and rest your tired bones.")
	contains(t, "resting shows in score", c.do("score"), "You are resting.")
	contains(t, "sleep", c.do("sleep"), "You go to sleep.")

	// Asleep is not just a label: do_look refuses outright.
	contains(t, "looking while asleep", c.do("look"), "In your dreams, or what?")

	contains(t, "wake", c.do("wake"), "You awaken, and sit up.")
	contains(t, "stand", c.do("stand"), "You stand up.")
	contains(t, "standing shows in score", c.do("score"), "You are standing.")

	m.noServerErrors()
}

// TestPractising: the guildmaster, who teaches whatever the caller's own
// class knows rather than a fixed list.
func TestPractising(t *testing.T) {
	m, c, out := arrive(t, miniClassic, "Tourist", roomGuildhall)

	contains(t, "the Guildhall", out, "Outside the Guildhall",
		"A guildmaster stands here, ready to teach.")

	// A warrior's list. `kick` is on it and `sneak` is not, which is the
	// class table doing its job rather than one list for everybody.
	list := c.do("practice")
	contains(t, "practice with no argument", list,
		"You have 2 practice sessions remaining.", "You know of the following skills:", "kick")
	contains(t, "practising something another class knows", c.do("practice sneak"),
		"You do not know of that skill.")

	contains(t, "practising", c.do("practice kick"), "You practice for a while.")
	contains(t, "one session spent", c.do("practice"), "You have 1 practice session")

	m.noServerErrors()
}

// TestShopping: the General Store. The prices are the shop file's own markup
// applied to the objects' own costs, so they are asserted exactly -- a change
// to either is a change a player would notice.
func TestShopping(t *testing.T) {
	m, c, out := arrive(t, miniClassic, "Tourist", roomGeneralStore)

	contains(t, "the General Store", out, "The General Store",
		"A shopkeeper stands behind the counter, tallying the day's takings.")

	// 30 and 60 at the shop's 1.2 buy_profit (examples/mini/binary/world/
	// shp/1.shp). Integer-truncated, the way shop.c does it.
	contains(t, "list", c.do("list"),
		"Available", "Item", "Cost", "A shiny dagger", "36", "A warm cloak", "72")

	// Nothing in the pockets yet: the refusal is part of the shop, not an
	// error.
	contains(t, "buying with no money", c.do("buy dagger"), "You can't afford that!")

	m.noServerErrors()
}

// TestShoppingWithMoney is the same shop with a purse, which needs the coins
// the dummy is guarding two rooms south.
func TestShoppingWithMoney(t *testing.T) {
	m, c, _ := arrive(t, miniClassic, "Tourist", roomSparringRing)

	// The pouch on the floor of the Sparring Ring is 75 coins.
	contains(t, "picking up coins", c.do("get pouch"), "There were 75 coins.")
	c.north(3)

	contains(t, "buying", c.do("buy dagger"), "That'll be 36 coins, thanks.", "You now have a shiny dagger.")
	// 30 at the shop file's 0.5 sell_profit. Asserted exactly for the same
	// reason the 36 above is: it is a number a player counts.
	contains(t, "valuing", c.do("value dagger"), "I'll give you 15 gold coins for that!")
	contains(t, "selling", c.do("sell dagger"), "I'll give you 15 coins for it.", "The shopkeeper now has a dagger.")

	m.noServerErrors()
}

// TestBanking: the Bank of Testing, whose teller is an object rather than a
// mobile (examples/mini/README.md's second finding -- spec_assign.c attaches
// "bank" to two object vnums and to no mobile at all).
func TestBanking(t *testing.T) {
	m, c, _ := arrive(t, miniClassic, "Tourist", roomSparringRing)

	c.do("get pouch")
	c.north(4)
	contains(t, "arriving at the bank", c.do("look"), "The Bank of Testing",
		"An automatic teller machine is bolted to the wall here.")

	contains(t, "an empty account", c.do("balance"), "You currently have no money deposited.")
	contains(t, "depositing", c.do("deposit 20"), "You deposit 20 coins.")
	contains(t, "the balance after", c.do("balance"), "Your current balance is 20 coins.")
	contains(t, "withdrawing", c.do("withdraw 10"), "You withdraw 10 coins.")
	contains(t, "the balance after that", c.do("balance"), "Your current balance is 10 coins.")
	contains(t, "withdrawing more than there is", c.do("withdraw 1000"),
		"You don't have that many coins deposited!")

	m.noServerErrors()
}

// TestTheReceptionist: the Travelers' Rest. free_rent is on (config.c:133),
// so the whole of renting is the receptionist saying so.
func TestTheReceptionist(t *testing.T) {
	m, c, out := arrive(t, miniClassic, "Tourist", roomTravelersRest)

	contains(t, "the Travelers' Rest", out, "The Travelers' Rest",
		"The receptionist is here, ready to offer you a place to stay.")

	free := "Rent is free here.  Just quit, and your objects will be saved!"
	contains(t, "offer", c.do("offer"), free)
	contains(t, "rent", c.do("rent"), free)

	m.noServerErrors()
}

// TestTheNoticeBoard: reading an empty board, writing to it, reading it back
// and taking the message down again. The board is a real object with a real
// special (internal/game/board.go's table, vnum 3099), and writing to it goes
// through the editor.
func TestTheNoticeBoard(t *testing.T) {
	m, c, out := arrive(t, miniClassic, "Tourist", roomNoticeBoard)

	contains(t, "the Notice Board", out, "The Notice Board",
		"A bulletin board is mounted on the wall here.")
	contains(t, "an empty board", c.do("look board"),
		"This is a bulletin board.", "The board is empty.")

	// The editor: a prompt of its own, and "@" on a line by itself to save.
	c.doUntil("write A test post", "Write your message")
	c.send("This is the body of the post.")
	c.doUntil("@", promptMarker)

	listing := c.do("look board")
	contains(t, "the board after posting", listing, "1 message", "A test post", "Tourist")
	contains(t, "reading it", c.do("read 1"), "This is the body of the post.")

	contains(t, "removing it", c.do("remove 1"), "Message removed.")
	contains(t, "the board after removing", c.do("look board"), "The board is empty.")

	m.noServerErrors()
}

// TestTheEndOfTheTour: Graduation Hall, and the way back. The corridor is not
// one-way, which is a claim its own README makes and nothing else checks.
func TestTheEndOfTheTour(t *testing.T) {
	m, c, out := arrive(t, miniClassic, "Tourist", roomGraduationHall)

	contains(t, "Graduation Hall", out, "Graduation Hall", "[ Exits: s ]")

	// All the way back down, through the doors the walk up left open.
	for i := roomGraduationHall; i > roomTestingGrounds; i-- {
		if got := c.do("south"); !strings.Contains(got, tourRoomNames[i-1]) {
			t.Fatalf("walking back to %s, the move printed:\n%s", tourRoomNames[i-1], got)
		}
	}
	contains(t, "back at the start", c.do("look"), "The Testing Grounds")

	m.noServerErrors()
}

// TestTheWholeTourInOneSitting is the regression test the rest of this file
// is the diagnosis for.
//
// One character, one connection, every room in order, every command the
// tutorial's own text tells a player to type. The per-room tests above say
// what broke; this one says that something did, on the path an actual first-
// time player takes, and it runs in both data formats because the answer
// should not depend on which one the server was booted on.
func TestTheWholeTourInOneSitting(t *testing.T) {
	for _, l := range bothFormats {
		t.Run(l.name, func(t *testing.T) {
			m := start(t, l)
			c := m.dial()
			c.create("Tourist", "tourpass", "m", "w")

			for room := roomHallOfMovement; room <= roomGraduationHall; room++ {
				out := c.do("north")
				if !strings.Contains(out, tourRoomNames[room]) {
					t.Fatalf("expected to reach %s; the move printed:\n%s", tourRoomNames[room], out)
				}

				for _, cmd := range tourCommands[room] {
					got := c.do(cmd)
					if strings.Contains(got, "Huh?!?") {
						t.Errorf("in %s, %q was not understood:\n%s", tourRoomNames[room], cmd, got)
					}
				}

				// The dummy has to finish dying before the tour can move on:
				// do_simple_move refuses outright while FIGHTING, so every
				// command after `kill` would answer "No way!  You're fighting
				// for your life!" and the walk would stall in the ring. The
				// rest of the fight arrives on the violence pulse rather than
				// in reply to anything typed, so this waits on the socket.
				if room == roomSparringRing {
					c.expect("R.I.P.")
					c.do("look")
				}
			}

			m.noServerErrors()
		})
	}
}

// tourCommands is what the tutorial's own room descriptions tell a player to
// type, room by room.
//
// Kept as data rather than prose because that is what makes it a check on the
// world files as well as on the server: every one of these is a quoted
// command in examples/mini's room text, and a room that starts recommending
// something the server does not understand fails the loop above. `mail` and
// `eat` are the shape of thing that catches -- both were written into the
// tutorial in a form the server does not accept, and neither was noticed
// until something typed them.
var tourCommands = map[int][]string{
	roomDoorRoom:        {"open door", "close door", "open door", "get key"},
	roomLockedVault:     {"unlock door with key", "open door", "pick door"},
	roomArmory:          {"get all", "wear tunic", "wield sword", "inventory", "equipment", "remove tunic"},
	roomClutteredCloset: {"open chest", "examine chest", "get ring from chest", "get bag", "put ring in bag", "examine bag", "get ring from bag"},
	roomDiningHall:      {"get bread", "eat bread", "get waterskin", "fill waterskin from fountain", "drink fountain"},
	roomSparringRing:    {"get all", "hold wand", "kill dummy"},
	roomRestingRoom:     {"rest", "sleep", "wake", "stand"},
	roomGuildhall:       {"practice", "practice kick"},
	roomGeneralStore:    {"list", "buy dagger", "value dagger", "sell dagger"},
	roomBank:            {"balance", "deposit 20", "withdraw 10", "balance"},
	roomTravelersRest:   {"offer", "rent"},
	// `mail` opens the editor when it succeeds, which would leave the walk
	// waiting for a prompt that is not coming. A level 1 character is
	// refused by the postmaster before that happens (MinMailLevel), so this
	// exercises the command and the level gate without entering the editor;
	// TestMail does the whole exchange with a character who can pay.
	roomPostOffice:     {"mail Tourist", "check", "receive"},
	roomNoticeBoard:    {"look board", "read board"},
	roomGraduationHall: {"smile", "wave", "say hello", "score"},
}
