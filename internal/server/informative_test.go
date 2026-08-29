// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"strings"
	"testing"
	"time"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// TestConsiderTheLadder, which is the whole feature — the wording, not the
// numbers.
func TestConsiderTheLadder(t *testing.T) {
	for _, tc := range []struct {
		difference int32
		want       string
	}{
		{-20, "Now where did that chicken go?"},
		{-10, "Now where did that chicken go?"},
		{-9, "You could do it with a needle!"},
		{-5, "You could do it with a needle!"},
		{-4, "Easy."},
		{-2, "Easy."},
		{-1, "Fairly easy."},
		{0, "The perfect match!"},
		{1, "You would need some luck!"},
		{2, "You would need a lot of luck!"},
		{3, "You would need a lot of luck and great equipment!"},
		{5, "Do you feel lucky, punk?"},
		{10, "Are you mad!?"},
		{11, "You ARE mad!"},
	} {
		if got := game.ConsiderVerdict(tc.difference); !strings.HasPrefix(got, tc.want) {
			t.Errorf("a difference of %d says %q, want %q", tc.difference, got, tc.want)
		}
	}
}

// TestConsiderRefusesPlayers, with the line the C is remembered for.
func TestConsiderRefusesPlayers(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	place(t, srv, fighterRecord("Welmar", 10, 100), ImmortStartRoom)

	c.send("consider welmar")
	c.expect("Would you like to borrow a cross and a shovel?")

	c.send("consider zod")
	c.expect("Easy!  Very easy indeed!")

	c.send("consider nobody")
	c.expect("Consider killing who?")
}

// TestConsiderAMobile uses the level difference.
func TestConsiderAMobile(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	// The Implementor is level 34 and the dog is level 5.
	spawnDog(t, srv, ImmortStartRoom)
	c.send("consider dog")
	c.expect("Now where did that chicken go?")
}

// TestHealthDiagnosisBands.
func TestHealthDiagnosisBands(t *testing.T) {
	for _, tc := range []struct {
		hit, maxHit int32
		want        string
	}{
		{100, 100, "is in excellent condition"},
		{95, 100, "has a few scratches"},
		{80, 100, "has some small wounds and bruises"},
		{60, 100, "has quite a few wounds"},
		{35, 100, "has some big nasty wounds and scratches"},
		{20, 100, "looks pretty hurt"},
		{5, 100, "is in awful condition"},
		{-5, 100, "is bleeding awfully from big wounds"},
	} {
		rec := &game.PlayerRecord{Points: game.Points{Hit: tc.hit, MaxHit: tc.maxHit}}
		got := game.HealthDiagnosis("Welmar", rec)
		if !strings.Contains(got, tc.want) {
			t.Errorf("%d/%d says %q, want %q", tc.hit, tc.maxHit, got, tc.want)
		}
	}
}

// TestLookAtSomebody shows their description, their health and their gear.
func TestLookAtSomebody(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	other, otherClient := place(t, srv, fighterRecord("Welmar", 10, 100), ImmortStartRoom)
	inWorld(t, srv, func(w *game.Live) {
		other.Record.Description = "A short, angry man.\r\n"
		other.Record.Points.Hit = 50

		sword := w.NewObject(testSwordVnum)
		w.ObjectToChar(sword, other)
		w.Equip(sword, other, game.WearWield)
	})

	c.send("look welmar")
	// The equipment list is the last thing printed, so waiting for it means
	// everything above it has arrived too.
	got := c.expect("a long sword")

	for _, want := range []string{
		"A short, angry man.",
		"Welmar has quite a few wounds.",
		"a long sword",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("looking at Welmar is missing %q:\n%s", want, got)
		}
	}

	if !otherClient.said("Zod looks at you.") {
		t.Error("Welmar was not told they were being looked at")
	}
}

// TestLookAtAnObject, in the order the C searches: equipment, inventory,
// floor.
//
// "You see nothing special.." — two full stops, and no mention of what was
// looked at — is show_obj_to_char's SHOW_OBJ_ACTION default
// (act.informative.c:60), and it is the answer whether the object is on the
// floor or in hand. This test used to expect the object's *long* description
// for the floor case ("A long sword is lying here.") and its short one for
// the carried case, which is what this port did and what the C does not: the
// long description is what `look` at the room prints, not what looking at the
// object prints. Checked against the real C server before changing, with
// scripts/session-parity.sh, rather than from a reading.
// TestLookInNothingSaysTheTypoBack, which is do_look's container branch
// refusing (act.informative.c:503).
//
// The C builds the refusal out of AN() and the player's own argument, and
// AN() picks the article off the first letter alone (utils.h:133) — so it
// says the typo back, article and all, and gets "an hour" and "a onion"
// wrong on the way. `look in nothing` is "There doesn't seem to be a nothing
// here.", not a generic "You do not see that here."
//
// Untested until #263 despite being one of the differences the parity suite
// found and somebody fixed: docs/deviations.md's "Refusal wording" entry
// recorded the fix in prose and nothing pinned it.
func TestLookInNothingSaysTheTypoBack(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	for _, tc := range []struct{ arg, want string }{
		{"nothing", "There doesn't seem to be a nothing here."},
		// AN() is one test on one letter, so both of these are the C's own
		// mistakes and both are reproduced.
		{"hour", "There doesn't seem to be a hour here."},
		{"onion", "There doesn't seem to be an onion here."},
	} {
		c.send("look in " + tc.arg)
		c.expect(tc.want)
	}
}

func TestLookAtAnObject(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	drop(t, srv, testSwordVnum, ImmortStartRoom)

	c.send("look sword")
	c.expect("You see nothing special..")

	c.send("get sword")
	c.expect("You get a long sword.")
	c.send("look sword")
	// The second occurrence: the first is still sitting in the transcript
	// from the floor case above, and expect would match it and prove nothing.
	c.expectCount("You see nothing special..", 2)

	c.send("look nothing")
	c.expect("You do not see that here.")
}

// TestExamineOpensAContainer.
func TestExamineOpensAContainer(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	inWorld(t, srv, func(w *game.Live) {
		bag := w.NewObject(testRingVnum)
		bag.Keywords = "bag"
		bag.ShortDesc = "a small bag"
		bag.Type = game.ItemContainer
		w.ObjectToRoom(bag, ImmortStartRoom)

		coin := w.NewObject(testRingVnum)
		w.ObjectToObject(coin, bag)
	})

	c.send("examine bag")
	// Wait for the contents rather than the header: expectAny waits for a
	// *new* occurrence, and if the whole reply arrives in one read it has
	// already been counted.
	got := c.expect("a gold ring")
	if !strings.Contains(got, "When you look inside, you see:") {
		t.Errorf("the container was not opened up:\n%s", got)
	}

	c.send("examine")
	c.expect("Examine what?")
}

// TestTimeAndWeather.
func TestTimeAndWeather(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	c.send("time")
	got := c.expect("Day of the")
	if !strings.Contains(got, "o'clock") {
		t.Errorf("time did not print a clock:\n%s", got)
	}

	c.send("weather")
	c.expect("cloudless")

	// Indoors there is no weather to speak of.
	inWorld(t, srv, func(w *game.Live) {
		w.Room(ImmortStartRoom).Flags = w.Room(ImmortStartRoom).Flags.Set(game.RoomIndoors)
	})
	c.send("weather")
	c.expect("You have no feeling about the weather at all.")
}

// TestTheOrdinalSuffixHandlesTheTeens, which the C carries a comment about
// crediting two people with fixing.
func TestTheOrdinalSuffixHandlesTheTeens(t *testing.T) {
	for _, tc := range []struct {
		day  int32
		want string
	}{
		{1, "1st"}, {2, "2nd"}, {3, "3rd"}, {4, "4th"},
		{11, "11th"}, {12, "12th"}, {13, "13th"},
		{21, "21st"}, {22, "22nd"}, {23, "23rd"},
		{31, "31st"}, {32, "32nd"}, {33, "33rd"}, {35, "35th"},
	} {
		mt := game.MudTime{Day: tc.day - 1}
		if got := mt.Date(); !strings.Contains(got, "The "+tc.want+" Day") {
			t.Errorf("day %d rendered as %q, want %q", tc.day, got, tc.want)
		}
	}
}

// TestTheMudCalendarWraps: seventeen months of thirty-five days.
func TestTheMudCalendarWraps(t *testing.T) {
	hour := int64(game.SecondsPerMudHour)
	day := int64(game.SecondsPerMudDay)
	month := int64(game.SecondsPerMudMonth)
	year := int64(game.SecondsPerMudYear)

	for _, tc := range []struct {
		seconds int64
		want    game.MudTime
	}{
		{0, game.MudTime{}},
		{hour, game.MudTime{Hours: 1}},
		{23 * hour, game.MudTime{Hours: 23}},
		{day, game.MudTime{Day: 1}},
		{34 * day, game.MudTime{Day: 34}},
		{month, game.MudTime{Month: 1}},
		{16 * month, game.MudTime{Month: 16}},
		{year, game.MudTime{Year: 1}},
		{year + 2*month + 3*day + 4*hour, game.MudTime{Hours: 4, Day: 3, Month: 2, Year: 1}},
	} {
		if got := game.TimePassed(secondsDuration(tc.seconds)); got != tc.want {
			t.Errorf("%d seconds is %+v, want %+v", tc.seconds, got, tc.want)
		}
	}
}

func secondsDuration(seconds int64) time.Duration {
	return time.Duration(seconds) * time.Second
}
