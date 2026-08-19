// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package session

import (
	"strings"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// The special procedures themselves, ported from spec_procs.c.
//
// Each is a small program attached to a mobile, an object or a room, and
// between them they are most of what makes Midgaard feel inhabited: the
// guildmaster who teaches you, the guard who will not let you past, the dog
// that eats corpses, the janitor who tidies up after you, and Puff, who is a
// very female dragon.

// specGuild is the guildmaster, porting SPECIAL(guild).
//
// This is where `practice` actually teaches. The command itself only lists
// what you know and tells you to find your guild — which is what
// `docs/deviations.md` recorded as the largest deliberate deviation in the
// port, now resolved.
func specGuild(sc *SpecialCall) bool {
	if sc.Actor.IsNPC() || sc.Actor.Record == nil || !sc.Is("practice") {
		return false
	}

	// A Context for a command that arrived through the special rather than
	// through the dispatcher: same character, same world, no session needed
	// because Send falls back to the character's own client.
	ctx := &Context{
		Session: sc.Session, World: sc.World, Character: sc.Actor,
		RNG: sc.RNG, Violence: sc.Violence, Arg: sc.Arg,
	}

	if arg := strings.TrimSpace(sc.Arg); arg != "" {
		_ = ctx.practise(arg)
	} else {
		_ = ctx.listSkills()
	}
	return true
}

// specGuildGuard is the guard on a guild door, porting SPECIAL(guild_guard).
//
// The C's version tests `GET_CLASS(ch)`; this tree's was rewritten to test the
// remort vector, so a character who has *ever* been a thief may walk into the
// thieves' guild. That is the local remort feature working as designed, and it
// is the same rewrite `list_skills` got and `SPECIAL(guild)` did not.
func specGuildGuard(sc *SpecialCall) bool {
	dir, ok := game.ParseDirection(sc.Command)
	if !ok || sc.Mob == nil {
		return false
	}
	// A blind guard lets everybody through.
	if sc.Mob.Record != nil && sc.Mob.Record.AffectFlags.Has(game.AffectBlind) {
		return false
	}
	if sc.Actor.IsNPC() || sc.Actor.Level() >= game.LevelImmortal {
		return false
	}

	if !game.GuildBars(sc.Actor.Record, sc.Actor.Room, dir) {
		return false
	}
	sc.Tell("The guard humiliates you, and blocks your way.\r\n")
	sc.ToRoom("The guard humiliates %s, and blocks %s way.\r\n",
		sc.Actor.Name, sc.Actor.Possessive())
	return true
}

// puffSays are Puff's four lines, from SPECIAL(puff). One roll of number(0,
// 60) per pulse, so she says something about one pulse in fifteen — and the
// game's oldest in-joke is that the fourth line was added later than the
// others and by somebody else.
var puffSays = [...]string{
	"My god!  It's full of stars!",
	"How'd all those fish get up here?",
	"I'm a very female dragon.",
	"Hail to the King, Baby!",
}

// specPuff is the dragon over Midgaard.
func specPuff(sc *SpecialCall) bool {
	if !sc.Pulse() {
		return false
	}
	roll := sc.RNG.Number(0, 60)
	if roll < 0 || int(roll) >= len(puffSays) {
		return false
	}
	sc.ToRoom("%s says, '%s'\r\n", sc.Actor.Name, puffSays[roll])
	return true
}

// specFido eats corpses, porting SPECIAL(fido).
//
// What was in the corpse is left on the floor rather than eaten with it,
// which is why the scavengers that follow a fido around are worth watching.
func specFido(sc *SpecialCall) bool {
	if !sc.Pulse() || !sc.Actor.Position.Awake() {
		return false
	}

	for _, obj := range sc.World.RoomObjects(sc.Actor.Room) {
		if !game.IsCorpse(obj) {
			continue
		}
		sc.ToRoom("%s savagely devours a corpse.\r\n", sc.Actor.Name)
		for _, inside := range append([]*game.Object(nil), obj.Contents...) {
			sc.World.ObjectToRoom(inside, sc.Actor.Room)
		}
		sc.World.ExtractObject(obj)
		return true
	}
	return false
}

// specJanitor tidies up, porting SPECIAL(janitor).
//
// The test for litter is worth reading twice: anything takeable that is
// *either* a drink container *or* worth less than fifteen coins. So a janitor
// will pick up an empty bottle of any value, and will leave a fifteen-coin
// dagger where it lies.
func specJanitor(sc *SpecialCall) bool {
	if !sc.Pulse() || !sc.Actor.Position.Awake() {
		return false
	}

	for _, obj := range sc.World.RoomObjects(sc.Actor.Room) {
		if !obj.Takeable() {
			continue
		}
		if obj.Type != game.ItemDrinkCon && obj.Cost >= 15 {
			continue
		}
		sc.ToRoom("%s picks up some trash.\r\n", sc.Actor.Name)
		sc.World.ObjectToChar(obj, sc.Actor)
		return true
	}
	return false
}

// specCityguard keeps the peace, porting SPECIAL(cityguard).
//
// Three jobs in order: kill player-killers and player-thieves on sight,
// defend the innocent from whoever in the room is most evil, and — the C's
// own comment — "reward the socially inept" by spitting on the person present
// with the lowest charisma. The last of those needs the socials and is not
// here yet.
func specCityguard(sc *SpecialCall) bool {
	if !sc.Pulse() || !sc.Actor.Position.Awake() || sc.Actor.Fighting != nil {
		return false
	}

	maxEvil := int32(1000)
	var evil *game.Character

	for _, other := range sc.World.Occupants(sc.Actor.Room) {
		if other.Record == nil {
			continue
		}

		if !other.IsNPC() && other.Record.PlayerFlags.Has(game.PlayerKiller) {
			sc.ToRoom("%s screams 'HEY!!!  You're one of those PLAYER KILLERS!!!!!!'\r\n", sc.Actor.Name)
			sc.Violence.Swing(sc.World, sc.Actor, other)
			return true
		}
		if !other.IsNPC() && other.Record.PlayerFlags.Has(game.PlayerThief) {
			sc.ToRoom("%s screams 'HEY!!!  You're one of those PLAYER THIEVES!!!!!!'\r\n", sc.Actor.Name)
			sc.Violence.Swing(sc.World, sc.Actor, other)
			return true
		}

		// The most evil person in a fight, provided one side of that fight is
		// a mobile — guards do not join a duel between two players.
		if other.Fighting != nil && other.Record.Alignment < maxEvil &&
			(other.IsNPC() || other.Fighting.IsNPC()) {
			maxEvil = other.Record.Alignment
			evil = other
		}
	}

	// Only if their opponent is not evil themselves: a guard will not take
	// sides between two villains.
	if evil != nil && evil.Fighting != nil && evil.Fighting.Record != nil &&
		evil.Fighting.Record.Alignment >= 0 {
		sc.ToRoom("%s screams 'PROTECT THE INNOCENT!  BANZAI!  CHARGE!  ARARARAGGGHH!'\r\n", sc.Actor.Name)
		sc.Violence.Swing(sc.World, sc.Actor, evil)
		return true
	}
	return false
}

// specSnake poisons whoever it is fighting, porting SPECIAL(snake).
//
// `number(0, GET_LEVEL(ch)) != 0` means a level-one snake bites every round
// and a level-twenty snake bites one round in twenty-one: the *higher* the
// level, the rarer the bite.
func specSnake(sc *SpecialCall) bool {
	if !sc.Pulse() || sc.Actor.Position != game.PosFighting || sc.Actor.Fighting == nil {
		return false
	}
	victim := sc.Actor.Fighting
	if victim.Room != sc.Actor.Room || sc.RNG.Number(0, sc.Actor.Level()) != 0 {
		return false
	}

	sc.ToRoom("%s bites %s!\r\n", sc.Actor.Name, victim.Name)
	victim.Tell("%s bites you!\r\n", sc.Actor.Name)
	sc.cast(game.SpellPoison, victim, nil)
	return true
}

// specMagicUser is the mobile spellcaster, porting SPECIAL(magic_user).
//
// The spell it throws is chosen by level in bands of two, from magic missile
// at level four to fireball above seventeen, with poison, blindness and a
// drain rolled for separately on top. It is the reason a mobile mage is more
// dangerous than its hit points suggest.
func specMagicUser(sc *SpecialCall) bool {
	if !sc.Pulse() || sc.Actor.Position != game.PosFighting {
		return false
	}

	// Somebody in the room who is fighting me, chosen by rolling for each in
	// turn — so it is biased towards whoever is listed first, which the C's
	// comment calls "pseudo-randomly".
	var victim *game.Character
	for _, other := range sc.World.Occupants(sc.Actor.Room) {
		if other.Fighting == sc.Actor && sc.RNG.Number(0, 4) == 0 {
			victim = other
			break
		}
	}
	if victim == nil && sc.Actor.Fighting != nil && sc.Actor.Fighting.Room == sc.Actor.Room {
		victim = sc.Actor.Fighting
	}
	if victim == nil {
		// "Hm...didn't pick anyone...I'll wait a round."
		return true
	}

	level := sc.Actor.Level()
	if level > 13 && sc.RNG.Number(0, 10) == 0 {
		sc.cast(game.SpellPoison, victim, nil)
	}
	if level > 7 && sc.RNG.Number(0, 8) == 0 {
		sc.cast(game.SpellBlindness, victim, nil)
	}
	if level > 12 && sc.RNG.Number(0, 12) == 0 {
		switch {
		case game.IsEvil(sc.Actor.Record):
			sc.cast(game.SpellEnergyDrain, victim, nil)
		case game.IsGood(sc.Actor.Record):
			sc.cast(game.SpellDispelEvil, victim, nil)
		}
	}

	// Four rounds in five it stops here, having only maybe thrown one of the
	// three above.
	if sc.RNG.Number(0, 4) != 0 {
		return true
	}
	sc.cast(game.MobileAttackSpell(level), victim, nil)
	return true
}

// specThief picks pockets, porting SPECIAL(thief) and npc_steal.
//
// It takes gold and only gold — between one and ten per cent of what the
// victim is carrying — and being caught costs it nothing at all: the victim
// is told, and that is the end of it.
func specThief(sc *SpecialCall) bool {
	if !sc.Pulse() || sc.Actor.Position != game.PosStanding {
		return false
	}

	for _, victim := range sc.World.Occupants(sc.Actor.Room) {
		if victim.IsNPC() || victim.Level() >= game.LevelImmortal || sc.RNG.Number(0, 4) != 0 {
			continue
		}
		sc.npcSteal(victim)
		return true
	}
	return false
}

// npcSteal is the theft itself.
func (sc *SpecialCall) npcSteal(victim *game.Character) {
	if victim.Record == nil || sc.Actor.Record == nil {
		return
	}

	// Awake and lucky: they notice, and nothing is taken.
	if victim.Position.Awake() && sc.RNG.Number(0, sc.Actor.Level()) == 0 {
		victim.Tell("You discover that %s has %s hands in your wallet.\r\n",
			sc.Actor.Name, sc.Actor.Possessive())
		for _, other := range sc.World.Occupants(sc.Actor.Room) {
			if other != victim && other != sc.Actor {
				other.Tell("%s tries to steal gold from %s.\r\n", sc.Actor.Name, victim.Name)
			}
		}
		return
	}

	if gold := (victim.Record.Points.Gold * sc.RNG.Number(1, 10)) / 100; gold > 0 {
		victim.Record.Points.Gold -= gold
		sc.Actor.Record.Points.Gold += gold
	}
}

// specDump is the Midgaard dump, porting SPECIAL(dump).
//
// It is a room special, and it does two things whatever command it is given:
// everything lying in the room vanishes, on every call. Only then does it
// check whether the command was `drop`, and if so pay for what was dropped —
// a coin per ten of value, capped at fifty, and *experience* rather than gold
// for a character below level three.
func specDump(sc *SpecialCall) bool {
	room := sc.Actor.Room

	// The pre-emptive sweep, which runs even for a command the dump does not
	// handle. Anything left in the room by anybody is already gone by the
	// time the next player types anything at all.
	for _, obj := range append([]*game.Object(nil), sc.World.RoomObjects(room)...) {
		sc.ToRoom("%s vanishes in a puff of smoke!\r\n", game.Capitalise(obj.Name()))
		sc.Actor.Tell("%s vanishes in a puff of smoke!\r\n", game.Capitalise(obj.Name()))
		sc.World.ExtractObject(obj)
	}

	if !sc.Is("drop") {
		return false
	}

	// The C runs do_drop itself and then sweeps again, which is why the
	// dropped object never touches the floor for anybody else to see.
	dropped := sc.dropForDump()

	var value int32
	for _, obj := range dropped {
		sc.Actor.Tell("%s vanishes in a puff of smoke!\r\n", game.Capitalise(obj.Name()))
		sc.ToRoom("%s vanishes in a puff of smoke!\r\n", game.Capitalise(obj.Name()))
		value += max(1, min(50, obj.Cost/10))
		sc.World.ExtractObject(obj)
	}

	if value == 0 || sc.Actor.Record == nil {
		return true
	}
	sc.Actor.Tell("You are awarded for outstanding performance.\r\n")
	sc.ToRoom("%s has been awarded for being a good citizen.\r\n", sc.Actor.Name)

	if sc.Actor.Level() < 3 {
		game.GainExperience(sc.Actor.Record, value, sc.RNG)
		return true
	}
	sc.Actor.Record.Points.Gold += value
	return true
}

// dropForDump runs the drop and returns what landed on the floor, so the
// caller can price it. Simpler than the C's arrangement, which drops and then
// walks the room again — the answer is the same because the sweep above left
// the room empty.
func (sc *SpecialCall) dropForDump() []*game.Object {
	before := len(sc.World.RoomObjects(sc.Actor.Room))
	_ = before

	ctx := &Context{
		World: sc.World, Character: sc.Actor,
		RNG: sc.RNG, Violence: sc.Violence, Arg: sc.Arg,
	}
	_ = doDrop(ctx)

	return append([]*game.Object(nil), sc.World.RoomObjects(sc.Actor.Room)...)
}

// cast throws a spell from a mobile, porting cast_spell's use by the mobile
// specials — which skips do_cast's checks entirely: no mana, no skill roll,
// no position test beyond the special's own.
func (sc *SpecialCall) cast(number int32, victim *game.Character, obj *game.Object) {
	info, ok := game.Spell(number)
	if !ok {
		return
	}
	ctx := &Context{
		World: sc.World, Character: sc.Actor,
		RNG: sc.RNG, Violence: sc.Violence,
	}
	ctx.castSpell(info, number, victim, obj)
}
