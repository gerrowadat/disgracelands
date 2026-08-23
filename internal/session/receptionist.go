// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package session

import (
	"fmt"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// gen_receptionist, ported from objsave.c:1076.
//
// The inn. `offer` prices a stay, `rent` takes it: your things go into the
// rent file, you leave the world, and you come back where you left off rather
// than in the temple. Two specials share the function — the receptionist
// charges by the day, the cryogenicist charges four days once and freezes you.
//
// On this server it is all theatre. `free_rent` is YES in config.c, so the
// receptionist says rent is free and stops. Everything below the check is
// ported anyway, because the setting is one line and the path has to be right
// if it is ever turned off.

// RentMode is which kind of stay is being sold.
type RentMode int

const (
	// RentModeDay is the receptionist: charged per day, RENT_FACTOR.
	RentModeDay RentMode = iota
	// RentModeCryo is the cryogenicist: charged once at four times the daily
	// rate, CRYO_FACTOR.
	RentModeCryo
)

// Factor is the multiplier this mode applies to every price.
func (m RentMode) Factor() int32 {
	if m == RentModeCryo {
		return game.CryoFactor
	}
	return game.RentFactor
}

// RentSaver writes a character's things into the rent file and takes them out
// of the world.
//
// A seam, like Violence and Save, and for the same reason: the object store
// belongs to the server and this package does not import it. The
// implementation is Server.RentCharacter.
type RentSaver func(w *game.Live, c *game.Character, mode RentMode, costPerDay int32)

// idleActions is gen_receptionist's action_table: what a receptionist does
// while waiting for custom.
var idleActions = []string{
	"smile", "dance", "sigh", "blush", "burp", "cough", "fart", "twiddle", "yawn",
}

func specReceptionist(sc *SpecialCall) bool { return genReceptionist(sc, RentModeDay) }
func specCryogenicist(sc *SpecialCall) bool { return genReceptionist(sc, RentModeCryo) }

func genReceptionist(sc *SpecialCall, mode RentMode) bool {
	recep, who := sc.Mob, sc.Actor
	if who.Client == nil || who.IsNPC() {
		return false
	}

	// On a pulse, a one-in-six chance of fidgeting. Note it returns FALSE
	// after doing so — a special that acts on a pulse has not "handled"
	// anything, because there was no command to handle.
	if sc.Pulse() {
		if sc.RNG != nil && sc.RNG.Number(0, 5) == 0 {
			// number(0, 8) over a nine-element table: the one place in the C
			// where a random index actually covers its array.
			sc.runSocialFor(recep, idleActions[sc.RNG.Number(0, 8)])
		}
		return false
	}

	if !sc.Is("offer") && !sc.Is("rent") {
		return false
	}
	if !recep.Position.Awake() {
		sc.Tell("%s is unable to talk to you...\r\n", capitaliseFirst(recep.Subject()))
		return true
	}

	if game.Tuning().FreeRent {
		sc.tellFrom(recep, "Rent is free here.  Just quit, and your objects will be saved!")
		return true
	}

	if sc.Is("offer") {
		offerRent(sc, recep, mode, true)
		sc.ToRoom("%s gives %s an offer.\r\n", recep.Name, who.Name)
		return true
	}

	cost, ok := offerRent(sc, recep, mode, false)
	if !ok {
		return true
	}
	if mode == RentModeCryo {
		sc.tellFrom(recep, "It will cost you %d gold coins to be frozen.", cost)
	} else {
		sc.tellFrom(recep, "Rent will cost you %d gold coins per day.", cost)
	}
	if cost > purse(who) {
		sc.tellFrom(recep, "...which I see you can't afford.")
		return true
	}
	if cost > 0 && mode == RentModeDay {
		rentDeadline(sc, recep, cost)
	}

	if mode == RentModeCryo {
		sc.tellFrom(recep, "%s stores your belongings and helps you into your private chamber.\r\n"+
			"A white mist appears in the room, chilling you to the bone...\r\n"+
			"You begin to lose consciousness...", recep.Name)
	} else {
		sc.tellFrom(recep, "%s stores your belongings and helps you into your private chamber.", recep.Name)
	}
	for _, other := range sc.World.Occupants(who.Room) {
		if other != who && other != recep {
			other.Tell("%s helps %s into %s private chamber.\r\n",
				recep.Name, who.Name, who.Possessive())
		}
	}

	// The load room is set here and nowhere else on the way out: this is what
	// makes renting bring you back to the inn rather than to the temple. A
	// plain `quit` leaves it alone, and the entry sequence clears it again
	// once it has been used (interpreter.c:1676).
	if who.Record != nil {
		who.Record.LoadRoom = who.Room
	}
	if sc.Rent != nil {
		sc.Rent(sc.World, who, mode, cost)
	}
	return true
}

// offerRent is Crash_offer_rent (objsave.c:1025): price the stay, and say so
// out loud if this is an `offer` rather than a `rent`.
//
// The second return is the C's "return 0", which means *do not proceed* and
// is not the same as a cost of zero — a free stay of something is still a
// stay. Splitting them out is the one place this reads differently from the
// C, which conflates the two and gets away with it because min_rent_cost is
// never zero.
func offerRent(sc *SpecialCall, recep *game.Character, mode RentMode, display bool) (int32, bool) {
	who := sc.Actor
	factor := mode.Factor()
	tuning := game.Tuning()

	// Anything that cannot be stored stops the whole transaction, and each
	// one is named. Note this walks *everything*, so a NORENT item at the
	// bottom of a bag is found and reported.
	unrentable := 0
	for _, obj := range who.Carrying {
		unrentable += reportUnrentables(sc, recep, obj)
	}
	for _, obj := range who.Equipment {
		unrentable += reportUnrentables(sc, recep, obj)
	}
	if unrentable > 0 {
		return 0, false
	}

	total := tuning.MinRentCost * factor
	items := 0
	for _, obj := range who.Carrying {
		reportRent(sc, recep, obj, &total, &items, display, factor)
	}
	for _, obj := range who.Equipment {
		reportRent(sc, recep, obj, &total, &items, display, factor)
	}

	if items == 0 {
		sc.tellFrom(recep, "But you are not carrying anything!  Just quit!")
		return 0, false
	}
	if items > int(tuning.MaxObjSave) {
		sc.tellFrom(recep, "Sorry, but I cannot store more than %d items.", tuning.MaxObjSave)
		return 0, false
	}

	if display {
		sc.tellFrom(recep, "Plus, my %d coin fee..", tuning.MinRentCost*factor)
		perDay := ""
		if mode == RentModeDay {
			perDay = " per day"
		}
		sc.tellFrom(recep, "For a total of %d coins%s.", total, perDay)
		if total > purse(who) {
			sc.tellFrom(recep, "...which I see you can't afford.")
			return 0, false
		}
		if mode == RentModeDay {
			rentDeadline(sc, recep, total)
		}
	}
	return total, true
}

// reportUnrentables is Crash_report_unrentables: name everything that cannot
// be stored, and count them.
func reportUnrentables(sc *SpecialCall, recep *game.Character, obj *game.Object) int {
	if obj == nil {
		return 0
	}
	found := 0
	if game.IsUnrentable(obj) {
		found = 1
		sc.tellFrom(recep, "You cannot store %s.", obj.Name())
	}
	for _, inside := range obj.Contents {
		found += reportUnrentables(sc, recep, inside)
	}
	return found
}

// reportRent is Crash_report_rent: add up the daily cost, naming each item if
// this is an `offer`.
func reportRent(sc *SpecialCall, recep *game.Character, obj *game.Object,
	cost *int32, items *int, display bool, factor int32,
) {
	if obj == nil {
		return
	}
	if !game.IsUnrentable(obj) {
		*items++
		*cost += max(0, obj.RentPerDay()*factor)
		if display {
			sc.tellFrom(recep, "%5d coins for %s..", obj.RentPerDay()*factor, obj.Name())
		}
	}
	for _, inside := range obj.Contents {
		reportRent(sc, recep, inside, cost, items, display, factor)
	}
}

// rentDeadline is Crash_rent_deadline: how long you can afford to stay.
//
// Integer division, so it rounds down, and the plural is decided by `> 1` —
// which means nought days is "0 days" and one day is "1 day", both correct by
// accident.
func rentDeadline(sc *SpecialCall, recep *game.Character, cost int32) {
	if cost == 0 {
		return
	}
	days := purse(sc.Actor) / cost
	plural := ""
	if days > 1 {
		plural = "s"
	}
	sc.tellFrom(recep, "You can rent for %d day%s with the gold you have\r\n"+
		"on hand and in the bank.", days, plural)
}

// purse is GET_GOLD(ch) + GET_BANK_GOLD(ch): everything they can pay with.
func purse(c *game.Character) int32 {
	if c == nil || c.Record == nil {
		return 0
	}
	return c.Record.Points.Gold + c.Record.Points.BankGold
}

// tellFrom is act("$n tells you, '...'", ..., TO_VICT): a mobile addressing
// one person. Every line the receptionist speaks is one of these.
func (sc *SpecialCall) tellFrom(from *game.Character, format string, args ...any) {
	sc.Actor.Tell("%s tells you, '%s'\r\n", from.Name, fmt.Sprintf(format, args...))
}

// runSocialFor makes a mobile perform a social with no target, which is what
// do_action does for a receptionist fidgeting between customers.
func (sc *SpecialCall) runSocialFor(who *game.Character, name string) {
	social := SocialNamed(name)
	if social == nil {
		return
	}
	if social.CharNoArg != "" {
		who.Tell("%s", sc.World.Act(social.CharNoArg, game.ActArgs{Actor: who}, who))
	}
	for _, other := range sc.World.Occupants(who.Room) {
		if other != who && social.OthersNoArg != "" {
			other.Tell("%s", sc.World.Act(social.OthersNoArg, game.ActArgs{Actor: who}, other))
		}
	}
}
