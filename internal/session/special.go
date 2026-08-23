// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package session

import (
	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/rng"
)

// Special procedures — `spec_procs.c`'s `SPECIAL()` functions, and the
// dispatch in `special()` (interpreter.c) that finds them.
//
// A special is a C function attached to a mobile, object or room prototype,
// called on two occasions and told which by its `cmd` argument:
//
//   - **a command**, before the command itself runs. Returning true means
//     "handled", and the command does not run at all. This is how a
//     guildmaster intercepts `practice` and how a pet shop intercepts `list`.
//   - **a pulse**, with no command, from `mobile_activity`. This is how Puff
//     says something at random and how a janitor picks up litter.
//
// The plan's §8 describes a `Trigger` interface for scripting, with the
// built-in specials as its first consumers. This is that seam, in the shape
// the built-ins actually need: they are not events matched against patterns,
// they are functions that get first refusal on a command.

// Special is one special procedure.
//
// It returns true if it handled the call, which for a command means the
// command does not run.
type Special func(sc *SpecialCall) bool

// SpecialCall is what a special is given: the C's
// `(ch, me, cmd, argument)` with the globals it would have reached for.
type SpecialCall struct {
	World *game.Live
	// Session is the actor's session, or nil when the actor is a mobile
	// acting on a pulse. A special that hands a command on wants it.
	Session *Session
	// Actor is the C's `ch` — whoever typed the command, or the mobile
	// itself on a pulse.
	Actor *game.Character
	// Mob, Obj and Room are the C's `me`. Exactly one is set.
	Mob  *game.Character
	Obj  *game.Object
	Room *game.RoomDef
	// Command is the canonical name of the command being run, or "" on a
	// pulse. Compared with `Is`, which is the C's CMD_IS.
	Command string
	// Arg is everything after the command word.
	Arg string

	RNG      *rng.Rand
	Violence Violence
	// Rent stores a character's belongings and takes them out of the world.
	// Only the receptionist uses it.
	Rent RentSaver
	// SaveBoard writes a bulletin board back to disk.
	SaveBoard BoardSaver
	// Mail is the mud mail system, for the postmaster.
	Mail MailSystem
}

// Is reports whether the command being run is this one, porting CMD_IS.
//
// The C compares against the *table entry's* name rather than what the player
// typed, so a special written for "practice" catches "prac" too.
func (sc *SpecialCall) Is(name string) bool { return sc.Command == name }

// Pulse reports whether this is the no-command call from mobile_activity.
func (sc *SpecialCall) Pulse() bool { return sc.Command == "" }

// Tell sends to whoever is acting. A mobile on a pulse has no client and the
// message goes nowhere, which is what send_to_char does there too.
func (sc *SpecialCall) Tell(format string, args ...any) { sc.Actor.Tell(format, args...) }

// SendPaged is Tell for a text long enough to want page_string's pager
// (pager.go) — a board's message list or one message's body, a shop's
// inventory. A pulse-driven call (Session nil, a mobile acting on its
// own) has nowhere to page to, so it falls back to Tell's own posture.
func (sc *SpecialCall) SendPaged(format string, args ...any) {
	if sc.Session == nil {
		sc.Actor.Tell(format, args...)
		return
	}
	sc.Session.SendPaged(format, args...)
}

// ToRoom sends to everyone in the actor's room except the actor.
func (sc *SpecialCall) ToRoom(format string, args ...any) {
	for _, other := range sc.World.Occupants(sc.Actor.Room) {
		if other != sc.Actor {
			other.Tell(format, args...)
		}
	}
}

// specials is the registry, keyed by the name the assignment table uses.
//
// Populated in init() rather than as a literal because several of them refer
// to helpers declared later in the package, and because a missing name should
// be a startup log line rather than a compile error — the assignment table is
// ported whole, including the specials that are not written yet.
var specials = map[string]Special{}

func init() {
	specials = map[string]Special{
		"guild":        specGuild,
		"guild_guard":  specGuildGuard,
		"puff":         specPuff,
		"fido":         specFido,
		"janitor":      specJanitor,
		"cityguard":    specCityguard,
		"snake":        specSnake,
		"magic_user":   specMagicUser,
		"thief":        specThief,
		"dump":         specDump,
		"shop_keeper":  specShopKeeper,
		"bank":         specBank,
		"receptionist": specReceptionist,
		"cryogenicist": specCryogenicist,
		"gen_board":    specGenBoard,
		"postmaster":   specPostmaster,
	}
}

// SpecialNames lists the specials that are implemented, for the boot log.
func SpecialNames() []string {
	out := make([]string, 0, len(specials))
	for name := range specials {
		out = append(out, name)
	}
	return out
}

// runSpecials gives every special in reach first refusal on a command,
// porting special() (interpreter.c).
//
// The order is the C's and it is not arbitrary: the room, then the actor's
// equipment, then their inventory, then the mobiles present, then the objects
// on the floor. A room special that handles a command stops a mobile in that
// room ever seeing it.
func (c *Context) runSpecials(command, arg string) bool {
	call := func(name string, set func(*SpecialCall)) bool {
		fn, ok := specials[name]
		if !ok {
			return false
		}
		sc := &SpecialCall{
			World: c.World, Session: c.Session, Actor: c.Character,
			Command: command, Arg: arg,
			RNG: c.RNG, Violence: c.Violence, Rent: c.Rent,
			SaveBoard: c.SaveBoard, Mail: c.Mail,
		}
		set(sc)
		return fn(sc)
	}

	if room := c.World.Room(c.Character.Room); room != nil && room.Spec != "" {
		if call(room.Spec, func(sc *SpecialCall) { sc.Room = room }) {
			return true
		}
	}
	for _, obj := range c.Character.Equipment {
		if name := obj.ObjSpec(); name != "" {
			if call(name, func(sc *SpecialCall) { sc.Obj = obj }) {
				return true
			}
		}
	}
	for _, obj := range c.Character.Carrying {
		if name := obj.ObjSpec(); name != "" {
			if call(name, func(sc *SpecialCall) { sc.Obj = obj }) {
				return true
			}
		}
	}
	for _, mob := range c.World.Occupants(c.Character.Room) {
		if name := mob.MobSpec(); name != "" {
			if call(name, func(sc *SpecialCall) { sc.Mob = mob }) {
				return true
			}
		}
	}
	for _, obj := range c.World.RoomObjects(c.Character.Room) {
		if name := obj.ObjSpec(); name != "" {
			if call(name, func(sc *SpecialCall) { sc.Obj = obj }) {
				return true
			}
		}
	}
	return false
}

// PulseSpecial runs a mobile's own special with no command, porting the call
// mobile_activity makes. The mobile is both the actor and the owner, as it is
// in the C.
func PulseSpecial(w *game.Live, mob *game.Character, r *rng.Rand, v Violence) bool {
	fn, ok := specials[mob.MobSpec()]
	if !ok {
		return false
	}
	return fn(&SpecialCall{
		World: w, Actor: mob, Mob: mob,
		RNG: r, Violence: v,
	})
}
