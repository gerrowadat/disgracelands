// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package session

import (
	"fmt"
	"strings"

	"github.com/gerrowadat/disgracelands/internal/colour"
	"github.com/gerrowadat/disgracelands/internal/game"
)

// Talking to people, ported from act.comm.c.
//
// Four ranges: the room (`say`, `whisper`, `ask`), one person anywhere
// (`tell`, `reply`), the group (`gsay`), and the whole game (`shout`,
// `gossip`, `auction`, `grats`, `holler`, `qsay`). Each channel can be
// switched off by the listener, and `noshout` is the only one that is a
// *player* flag rather than a preference — an immortal can silence somebody
// with it, which is why it lives where a player cannot reach it.

// drunkEnoughToSlur is act.comm.c's own local threshold — not config.c, and
// not reopened for tunability.
//
// LevelCanShout and HollerMoveCost (config.c:61 and :64) live in GameTuning
// (internal/game/tuning.go) now, a runtime setting rather than a constant.
const drunkEnoughToSlur int32 = 5

// doSay, porting do_say.
//
// The drunk speech is local, and it is the best thing in act.comm.c: above a
// drunkenness of five every `s` you say becomes `sh`, and one time in three
// the sentence ends "...*hic*.".
func doSay(c *Context) error {
	said := strings.TrimSpace(c.Arg)
	if said == "" {
		c.Send("Yes, but WHAT do you want to say?\r\n")
		return nil
	}

	spoken := said
	if !c.Character.IsNPC() && c.drunk() >= drunkEnoughToSlur {
		spoken = game.Slur(said, c.RNG)
	}

	c.announce("%s says, '%s'\r\n", c.Character.Name, spoken)

	if c.prefers(game.PrefNoRepeat) {
		c.Send("Okay.\r\n")
		return nil
	}
	c.Send("You say, '%s'\r\n", spoken)
	return nil
}

// doGroupSay, porting do_gsay. It reaches the whole group wherever they are,
// which is the point of being in one.
func doGroupSay(c *Context) error {
	if !c.Character.Grouped() {
		c.Send("But you are not the member of a group!\r\n")
		return nil
	}
	said := strings.TrimSpace(c.Arg)
	if said == "" {
		c.Send("Yes, but WHAT do you want to group-say?\r\n")
		return nil
	}

	leader := c.Character.GroupLeader()
	if leader != c.Character && leader.Grouped() {
		leader.Tell("%s tells the group, '%s'\r\n", c.Character.Name, said)
	}
	for _, f := range leader.Followers {
		if f != c.Character && f.Grouped() {
			f.Tell("%s tells the group, '%s'\r\n", c.Character.Name, said)
		}
	}

	if c.prefers(game.PrefNoRepeat) {
		c.Send("Okay.\r\n")
		return nil
	}
	c.Send("You tell the group, '%s'\r\n", said)
	return nil
}

// doTell, porting do_tell. It reaches anywhere in the world, and a mortal may
// only tell another *player* — an immortal may tell a mobile.
func doTell(c *Context) error {
	name, message := halfChop(c.Arg)
	if name == "" || message == "" {
		c.Send("Who do you wish to tell what??\r\n")
		return nil
	}

	victim := c.findAnywhere(name)
	if victim == nil || (victim.IsNPC() && c.Character.Level() < game.LevelImmortal) {
		c.Send("No-one by that name here.\r\n")
		return nil
	}
	if !c.tellIsOK(victim) {
		return nil
	}
	c.performTell(victim, message)
	return nil
}

// doReply, porting do_reply. The last teller is remembered by *identity*
// rather than by pointer, so it survives them logging out and back in.
func doReply(c *Context) error {
	rec := c.Character.Record
	if c.Character.IsNPC() || rec == nil {
		return nil
	}

	message := strings.TrimSpace(c.Arg)
	switch {
	case rec.LastTell == 0:
		c.Send("You have no-one to reply to!\r\n")
		return nil
	case message == "":
		c.Send("What is your reply?\r\n")
		return nil
	}

	var victim *game.Character
	for _, other := range c.World.Players() {
		if other.Record != nil && other.Record.IDNum == rec.LastTell {
			victim = other
			break
		}
	}
	if victim == nil {
		c.Send("They are no longer playing.\r\n")
		return nil
	}
	if c.tellIsOK(victim) {
		c.performTell(victim, message)
	}
	return nil
}

// tellIsOK is is_tell_ok: the six reasons a tell does not arrive, each with
// its own message so the teller can tell which.
func (c *Context) tellIsOK(victim *game.Character) bool {
	ok, refusal := tellIsOKFor(c.World, c.Character, victim)
	if !ok {
		c.Send("%s", refusal)
	}
	return ok
}

// tellIsOKFor is is_tell_ok (act.comm.c:127) with its refusal returned rather
// than sent, because the speaker is not always a player.
//
// A shopkeeper's or postmaster's lines reach the customer through do_tell
// (see keeperTells), so they run this gauntlet too — a customer with notell
// set, or standing in a soundproof room, gets no answer from the shop at all.
// The refusals themselves go to the *speaker*, and a mobile has no client to
// receive them, which is why they are a return value here.
func tellIsOKFor(w *game.Live, speaker, victim *game.Character) (bool, string) {
	soundproof := func(vnum game.RoomVnum) bool {
		room := w.Room(vnum)
		return room != nil && room.Flags.Has(game.RoomSoundproof)
	}
	switch {
	case victim == speaker:
		return false, "You try to tell yourself something.\r\n"
	case !speaker.IsNPC() && speaker.Record != nil &&
		speaker.Record.Preferences.Has(game.PrefNoTell):
		return false, "You can't tell other people while you have notell on.\r\n"
	case soundproof(speaker.Room):
		return false, "The walls seem to absorb your words.\r\n"
	case !victim.IsNPC() && victim.Client == nil:
		return false, capitaliseFirst(victim.Subject()) + "'s linkless at the moment.\r\n"
	case victim.Record != nil && victim.Record.PlayerFlags.Has(game.PlayerWriting):
		return false, capitaliseFirst(victim.Subject()) +
			"'s writing a message right now; try again later.\r\n"
	case (!victim.IsNPC() && victim.Record != nil &&
		victim.Record.Preferences.Has(game.PrefNoTell)) ||
		soundproof(victim.Room):
		return false, capitaliseFirst(victim.Subject()) + " can't hear you.\r\n"
	}
	return true, ""
}

// deliverTell is perform_tell's first half (act.comm.c:110):
// act("$n tells you, '...'", FALSE, ch, 0, vict, TO_VICT).
//
// Going through act() rather than formatting the name in is not cosmetic.
// act() resolves $n *per audience*, so a speaker the listener cannot see is
// "someone", and it capitalises the first letter of the finished line — which
// is where "The grocer tells you," gets its capital, a mobile's own name
// being lower case.
func deliverTell(w *game.Live, from, to *game.Character, message string) {
	// Red, at C_NRM — the C brackets the act() with bare writes of
	// CCRED/CCNRM (act.comm.c:109-112) rather than putting the escapes in
	// the string, which comes to the same thing here.
	to.TellAt(colour.Normal, "{{red}}%s{{/}}",
		w.Act("$n tells you, '"+message+"'", game.ActArgs{Actor: from}, to))
}

// performTell delivers one, and remembers who to reply to.
func (c *Context) performTell(victim *game.Character, message string) {
	deliverTell(c.World, c.Character, victim, message)

	if !c.Character.IsNPC() && c.prefers(game.PrefNoRepeat) {
		c.Send("Okay.\r\n")
	} else {
		// The teller's own echo is red too, but at C_CMP rather than C_NRM
		// (act.comm.c:117 against :109) — so somebody on "normal" colour
		// sees the tells they *receive* in red and their own in plain text.
		c.SendAt(colour.Complete, "{{red}}You tell %s, '%s'{{/}}\r\n", victim.Name, message)
	}

	if !victim.IsNPC() && !c.Character.IsNPC() &&
		victim.Record != nil && c.Character.Record != nil {
		victim.Record.LastTell = c.Character.Record.IDNum
	}
}

func doWhisper(c *Context) error {
	return c.specComm("whisper to", "whispers to", "whispers something to")
}
func doAsk(c *Context) error { return c.specComm("ask", "asks", "asks a question of") }

// specComm is do_spec_comm: whisper and ask, which differ only in wording.
func (c *Context) specComm(singular, plural, others string) error {
	name, message := halfChop(c.Arg)
	if name == "" || message == "" {
		c.Send("Whom do you want to %s.. and what??\r\n", singular)
		return nil
	}

	victim := c.findInRoom(name)
	if victim == nil {
		c.Send("No-one by that name here.\r\n")
		return nil
	}
	if victim == c.Character {
		c.Send("You can't get your mouth close enough to your ear...\r\n")
		return nil
	}

	victim.Tell("%s %s you, '%s'\r\n", c.Character.Name, plural, message)
	if c.prefers(game.PrefNoRepeat) {
		c.Send("Okay.\r\n")
	} else {
		c.Send("You %s %s, '%s'\r\n", singular, victim.Name, message)
	}

	for _, other := range c.World.Occupants(c.Character.Room) {
		if other != c.Character && other != victim {
			other.Tell("%s %s %s.\r\n", c.Character.Name, others, victim.Name)
		}
	}
	return nil
}

// channel is one of do_gen_comm's five, with the strings the C keeps in
// com_msgs[][4].
type channel struct {
	// verb is what the command is called in every message it prints.
	verb string
	// noshout is what somebody silenced by an immortal is told.
	noshout string
	// off is what somebody who has switched the channel off is told when
	// they try to use it.
	off string
	// mute is the preference that switches it off, or 0 for a channel that
	// cannot be switched off at all.
	mute game.Flags
	// sameZone limits it to the speaker's zone, which only shout does.
	sameZone bool
	// costsMovement is holler, and only holler.
	costsMovement bool
	// colour is com_msgs[subcmd][3] (act.comm.c:401), the fourth column of
	// the channel table: the escape each channel is written in. The speaker
	// sees it at C_CMP and everybody else at C_NRM, which is the one place
	// in the game where the same message is coloured at two different
	// thresholds depending on which end of it you are.
	colour string
}

var channels = map[string]channel{
	"holler": {
		verb: "holler", noshout: "You cannot holler!!\r\n", off: "",
		costsMovement: true, colour: "bright-green",
	},
	"shout": {
		verb: "shout", noshout: "You cannot shout!!\r\n",
		off: "Turn off your noshout flag first!\r\n", mute: game.PrefDeaf,
		sameZone: true, colour: "bright-red",
	},
	"gossip": {
		verb: "gossip", noshout: "You cannot gossip!!\r\n",
		off: "You aren't even on the channel!\r\n", mute: game.PrefNoGoss,
		colour: "yellow",
	},
	"auction": {
		verb: "auction", noshout: "You cannot auction!!\r\n",
		off: "You aren't even on the channel!\r\n", mute: game.PrefNoAuct,
		colour: "magenta",
	},
	"grats": {
		// The verb is "congrat" and the command is "grats", which is the C's
		// and reads oddly in every message the channel prints.
		verb: "congrat", noshout: "You cannot congratulate!\r\n",
		off: "You aren't even on the channel!\r\n", mute: game.PrefNoGratz,
		colour: "green",
	},
}

func doShout(c *Context) error   { return c.genComm("shout") }
func doGossip(c *Context) error  { return c.genComm("gossip") }
func doAuction(c *Context) error { return c.genComm("auction") }
func doGrats(c *Context) error   { return c.genComm("grats") }
func doHoller(c *Context) error  { return c.genComm("holler") }

// genComm is do_gen_comm: the game-wide channels.
//
// Shout is the odd one — it reaches only the speaker's own zone, and only
// people who are awake. The others reach everybody playing wherever they are
// and whatever they are doing.
func (c *Context) genComm(name string) error {
	ch, ok := channels[name]
	if !ok {
		return nil
	}
	rec := c.Character.Record

	// "to keep pets, etc from being ordered to shout".
	if c.Session == nil || rec == nil {
		return nil
	}

	tuning := game.Tuning()

	switch {
	case rec.PlayerFlags.Has(game.PlayerNoShout):
		c.Send("%s", ch.noshout)
		return nil
	case c.roomIsSoundproof(c.Character.Room):
		c.Send("The walls seem to absorb your words.\r\n")
		return nil
	case c.Character.Level() < tuning.LevelCanShout:
		c.Send("You must be at least level %d before you can %s.\r\n", tuning.LevelCanShout, ch.verb)
		return nil
	case ch.mute != 0 && rec.Preferences.Has(ch.mute):
		c.Send("%s", ch.off)
		return nil
	}

	said := strings.TrimSpace(c.Arg)
	if said == "" {
		c.Send("Yes, %s, fine, %s we must, but WHAT???\r\n", ch.verb, ch.verb)
		return nil
	}

	if ch.costsMovement {
		if rec.Points.Move < tuning.HollerMoveCost {
			c.Send("You're too exhausted to holler.\r\n")
			return nil
		}
		rec.Points.Move -= tuning.HollerMoveCost
	}

	if c.prefers(game.PrefNoRepeat) {
		c.Send("Okay.\r\n")
	} else {
		// The speaker's own echo is at C_CMP (act.comm.c:472), and the
		// listeners' at C_NRM (:488) — the same channel colour, two
		// different thresholds, which is the C and reads as a quirk until
		// you notice the speaker is the one who can turn theirs off without
		// losing everybody else's.
		c.SendAt(colour.Complete, "{{%s}}You %s, '%s'{{/}}\r\n", ch.colour, ch.verb, said)
	}

	for _, other := range c.World.Players() {
		switch {
		case other == c.Character || other.Client == nil:
			continue
		case ch.mute != 0 && other.Record != nil && other.Record.Preferences.Has(ch.mute):
			continue
		case other.Record != nil && other.Record.PlayerFlags.Has(game.PlayerWriting):
			continue
		case c.roomIsSoundproof(other.Room):
			continue
		}
		// Shout carries one zone and wakes nobody.
		if ch.sameZone && (!c.sameZone(other.Room) || !other.Position.Awake()) {
			continue
		}
		other.TellAt(colour.Normal, "{{%s}}%s{{/}}", ch.colour,
			c.World.Act("$n "+ch.verb+"s, '"+said+"'", game.ActArgs{Actor: c.Character}, other))
	}
	return nil
}

// doQuestSay and doQuestEcho are do_qcomm's two subcommands (act.comm.c:549):
// one channel, two ways of putting a line on it.
//
// `qsay` wraps what you type — "You quest-say, '...'" — and `qecho` sends the
// argument raw, with nobody's name on it. Everything else about them is the
// same function, including the refusal, the empty-argument joke (which spells
// itself out of the command's own name) and the `PRF_NOREPEAT` handling.
func doQuestSay(c *Context) error  { return questComm(c, "qsay") }
func doQuestEcho(c *Context) error { return questComm(c, "qecho") }

func questComm(c *Context, name string) error {
	if !c.prefers(game.PrefQuest) {
		c.Send("You aren't even part of the quest!\r\n")
		return nil
	}

	said := strings.TrimSpace(c.Arg)
	if said == "" {
		// CMD_NAME twice, then CAP() over the lot — so it is the command you
		// typed, capitalised once at the front.
		c.Send("%s?  Yes, fine, %s we must, but WHAT??\r\n", capitaliseFirst(name), name)
		return nil
	}

	mine, theirs := said, said
	if name == "qsay" {
		mine = fmt.Sprintf("You quest-say, '%s'", said)
		theirs = fmt.Sprintf("%s quest-says, '%s'", c.Character.Name, said)
	}

	if c.prefers(game.PrefNoRepeat) {
		c.Send("Okay.\r\n")
	} else {
		c.Send("%s\r\n", mine)
	}
	for _, other := range c.World.Players() {
		if other == c.Character || other.Record == nil {
			continue
		}
		// TO_SLEEP: the quest channel reaches you asleep, which most do not.
		if other.Record.Preferences.Has(game.PrefQuest) {
			other.Tell("%s\r\n", theirs)
		}
	}
	return nil
}

// doPage is do_page (act.comm.c:383): a line straight to one person, with two
// bells in front of it.
//
// `page all` is the interesting half — it needs *above* LVL_GOD rather than
// at it, which is the only place in the game that distinction is drawn, and
// the refusal is worth keeping for its own sake: "You will never be godly
// enough to do that!"
func doPage(c *Context) error {
	target, message := halfChop(c.Arg)

	if c.Character.IsNPC() {
		c.Send("Monsters can't page.. go away.\r\n")
		return nil
	}
	if target == "" {
		c.Send("Whom do you wish to page?\r\n")
		return nil
	}

	// \007 twice: the C really does ring the bell at you.
	line := fmt.Sprintf("\007\007*%s* %s", c.Character.Name, message)

	if strings.EqualFold(target, "all") {
		if c.Character.Level() <= game.LevelGod {
			c.Send("You will never be godly enough to do that!\r\n")
			return nil
		}
		for _, other := range c.World.Players() {
			other.Tell("%s\r\n", line)
		}
		return nil
	}

	victim := c.findAnywhere(target)
	if victim == nil {
		c.Send("There is no such person in the game!\r\n")
		return nil
	}
	victim.Tell("%s\r\n", line)
	if c.prefers(game.PrefNoRepeat) {
		c.Send("Okay.\r\n")
	} else {
		c.Send("%s\r\n", line)
	}
	return nil
}

// prefers reports whether a preference is set. False for a mobile, which has
// no preferences at all.
func (c *Context) prefers(flag game.Flags) bool {
	return c.Character.Record != nil && c.Character.Record.Preferences.Has(flag)
}

// drunk is how drunk the speaker is.
func (c *Context) drunk() int32 {
	if c.Character.Record == nil {
		return 0
	}
	return c.Character.Record.Conditions[game.CondDrunk]
}

// roomIsSoundproof reports ROOM_SOUNDPROOF.
func (c *Context) roomIsSoundproof(vnum game.RoomVnum) bool {
	room := c.World.Room(vnum)
	return room != nil && room.Flags.Has(game.RoomSoundproof)
}

// sameZone reports whether a room is in the speaker's zone, which is what
// limits a shout.
func (c *Context) sameZone(other game.RoomVnum) bool {
	return c.World.ZoneOf(c.Character.Room) == c.World.ZoneOf(other)
}

// halfChop splits off the first word and returns the rest, porting
// half_chop.
//
// It does *not* skip the filler words `one_argument` does, which is why
// `tell the bob hello` tries to tell somebody called "the". That is the C's
// behaviour and the reason `tell` is written against half_chop rather than
// one_argument in the first place: a message must survive intact.
func halfChop(s string) (first, rest string) {
	s = strings.TrimLeft(s, " \t")
	if s == "" {
		return "", ""
	}
	i := strings.IndexAny(s, " \t")
	if i < 0 {
		return strings.ToLower(s), ""
	}
	return strings.ToLower(s[:i]), strings.TrimLeft(s[i:], " \t")
}
