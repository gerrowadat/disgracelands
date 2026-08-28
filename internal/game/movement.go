// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

// What a step costs.

// movementLoss is movement_loss[] (constants.c:768), indexed by SECT_*.
//
// Re-parsed out of the C by TestMovementLossMatchesTheCSource rather than
// eyeballed, for the reason every other table here is: it is indexed by
// sector number, so one entry out of place makes the wrong terrain expensive
// and nothing else notices.
var movementLoss = []int32{
	1, // Inside
	1, // City
	2, // Field
	3, // Forest
	4, // Hills
	6, // Mountains
	4, // Swimming
	1, // Unswimable
	1, // Flying
	5, // Underwater
}

// lossFor is movement_loss[SECT(room)], with the bound the C does not have.
//
// The C indexes straight in, so a room whose sector byte is out of range
// reads past the end of the array. The world loader already refuses a sector
// it does not know, so nothing in the real data reaches this — it answers
// Inside's cost rather than panicking on a room built by hand in a test.
func lossFor(room *RoomDef) int32 {
	if room == nil {
		return movementLoss[SectorInside]
	}
	if room.SectorType < 0 || int(room.SectorType) >= len(movementLoss) {
		return movementLoss[SectorInside]
	}
	return movementLoss[room.SectorType]
}

// MovementCost is need_movement (act.movement.c:127): the average of the
// movement loss for the room being left and the room being entered.
//
//	need_movement = (movement_loss[SECT(IN_ROOM(ch))] +
//	                 movement_loss[SECT(EXIT(ch, dir)->to_room)]) / 2;
//
// **Integer division, and it truncates.** City to city is (1+1)/2 = 1, but
// city to field is (1+2)/2 = 1 as well, and field to forest is (2+3)/2 = 2.
// A step out of a city into the mountains costs (1+6)/2 = 3 and the step back
// costs the same — the average is symmetric, so walking a loop costs the same
// either way round, which is not true of a rule that charged for the
// destination alone.
//
// The cheapest step in the game therefore costs 1 and nothing is free, which
// is what makes the movement number on the prompt count down at all.
func MovementCost(from, to *RoomDef) int32 {
	return (lossFor(from) + lossFor(to)) / 2
}
