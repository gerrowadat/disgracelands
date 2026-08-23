// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"testing"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// TestSayReachesTheRoom, and the one-character form of it.
func TestSayReachesTheRoom(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	_, listener := place(t, srv, fighterRecord("Bob", 10, 100), ImmortStartRoom)

	c.send("say")
	c.expect("Yes, but WHAT do you want to say?")

	c.send("say hello there")
	c.expect("You say, 'hello there'")
	// See the note on settle() in TestWhisperAndAsk: seeing your own
	// reply does not mean the rest of the room has been written to yet.
	c.settle()
	if !listener.said("Zod says, 'hello there'") {
		t.Error("the room did not hear it")
	}

	// `'hi` with no space, which the interpreter special-cases.
	c.send("'well then")
	c.expect("You say, 'well then'")
	// See the note on settle() in TestWhisperAndAsk: seeing your own
	// reply does not mean the rest of the room has been written to yet.
	c.settle()
	if !listener.said("Zod says, 'well then'") {
		t.Error("the room did not hear the short form")
	}
}

// TestDrunkSpeech is local to this tree and is the best thing in act.comm.c.
func TestDrunkSpeech(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	// Sober first.
	c.send("say pass the salt")
	c.expect("You say, 'pass the salt'")

	inWorld(t, srv, func(w *game.Live) {
		w.Find("Zod").Record.Conditions[game.CondDrunk] = 10
	})

	c.send("say pass the salt")
	c.expect("You say, 'pashsh the shalt")
}

// TestTellAndReply, across rooms, and the memory of who told you.
func TestTellAndReply(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	first := dialClient(t, addr)
	first.create("Zod", "swordfish", "m", "w")
	first.send("quit")
	first.expect("Goodbye")
	first.close()

	c := dialClient(t, addr)
	c.create("Welmar", "hunter2!", "m", "w")

	// Somewhere else entirely: a tell reaches the whole world.
	bob, listener := place(t, srv, fighterRecord("Bob", 10, 100), MortalStartRoom)
	inWorld(t, srv, func(_ *game.Live) { bob.Record.IDNum = 4242 })

	c.send("reply hello")
	c.expect("You have no-one to reply to!")

	c.send("tell")
	c.expect("Who do you wish to tell what??")

	c.send("tell nobody hello")
	c.expect("No-one by that name here.")

	c.send("tell bob are you there")
	c.expect("You tell Bob, 'are you there'")
	// See the note on settle() in TestWhisperAndAsk: seeing your own
	// reply does not mean the rest of the room has been written to yet.
	c.settle()
	if !listener.said("Welmar tells you, 'are you there'") {
		t.Error("Bob was not told")
	}

	// Bob's record now remembers who to reply to.
	inWorld(t, srv, func(w *game.Live) {
		welmar := w.Find("Welmar")
		if bob.Record.LastTell != welmar.Record.IDNum {
			t.Errorf("Bob's last teller is %d, want Welmar's %d",
				bob.Record.LastTell, welmar.Record.IDNum)
		}
		// And now the other way, so the reply has somebody to find.
		welmar.Record.LastTell = bob.Record.IDNum
	})

	c.send("reply yes")
	c.expect("You tell Bob, 'yes'")
}

// TestTellingYourselfAndTheDeaf.
func TestTellingYourselfAndTheDeaf(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	bob, _ := place(t, srv, fighterRecord("Bob", 10, 100), ImmortStartRoom)
	bob.Record.Sex = game.SexMale
	// place() gives them a client; take it away to make them linkless.
	inWorld(t, srv, func(_ *game.Live) { bob.Client = nil })

	c.send("tell zod hello")
	c.expect("You try to tell yourself something.")

	// A character with no client is linkless.
	c.send("tell bob hello")
	c.expect("He's linkless at the moment.")

	// notell on the listener.
	inWorld(t, srv, func(_ *game.Live) {
		bob.Client = &recorder{}
		bob.Record.Preferences = bob.Record.Preferences.Set(game.PrefNoTell)
	})
	c.send("tell bob hello")
	c.expect("He can't hear you.")

	// notell on the teller.
	inWorld(t, srv, func(w *game.Live) {
		bob.Record.Preferences = bob.Record.Preferences.Clear(game.PrefNoTell)
		w.Find("Zod").Record.Preferences =
			w.Find("Zod").Record.Preferences.Set(game.PrefNoTell)
	})
	c.send("tell bob hello")
	c.expect("You can't tell other people while you have notell on.")
}

// TestChannelsReachEverybodyPlaying, and the toggles that switch them off.
func TestChannelsReachEverybodyPlaying(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	// In another room entirely: gossip does not care where you are.
	_, listener := place(t, srv, fighterRecord("Bob", 10, 100), MortalStartRoom)

	c.send("gossip")
	c.expect("Yes, gossip, fine, gossip we must, but WHAT???")

	c.send("gossip anybody about?")
	c.expect("You gossip, 'anybody about?'")
	// See the note on settle() in TestWhisperAndAsk: seeing your own
	// reply does not mean the rest of the room has been written to yet.
	c.settle()
	if !listener.said("Zod gossips, 'anybody about?'") {
		t.Error("the gossip did not carry")
	}

	// The congratulation channel calls itself "congrat" in every message,
	// though the command is `grats`.
	c.send("grats well done")
	c.expect("You congrat, 'well done'")
}

// TestSwitchingAChannelOff.
func TestSwitchingAChannelOff(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	c.send("nogossip")
	c.expect("You are now deaf to gossip.")

	// Off the channel means you cannot use it either.
	c.send("gossip hello")
	c.expect("You aren't even on the channel!")

	c.send("nogossip")
	c.expect("You can now hear gossip.")

	// nosummon runs backwards, and a new character starts *not* summonable —
	// so the first press of the command called "nosummon" makes you
	// summonable. That is the C's, and it is as confusing as it looks.
	c.send("nosummon")
	c.expect("You may now be summoned by other players.")
	inWorld(t, srv, func(w *game.Live) {
		if !w.Find("Zod").Record.Preferences.Has(game.PrefSummonable) {
			t.Error("the summonable bit was not set")
		}
	})

	c.send("nosummon")
	c.expect("You are now safe from summoning by other players.")
}

// TestNoRepeatSwallowsTheEcho.
func TestNoRepeatSwallowsTheEcho(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	_, listener := place(t, srv, fighterRecord("Bob", 10, 100), ImmortStartRoom)

	c.send("norepeat")
	c.expect("You will no longer have your communication repeated.")

	c.send("say hello")
	c.expect("Okay.")
	// See the note on settle() in TestWhisperAndAsk: seeing your own
	// reply does not mean the rest of the room has been written to yet.
	c.settle()
	if !listener.said("Zod says, 'hello'") {
		t.Error("norepeat silenced the room as well as the echo")
	}
}

// TestShoutCarriesOneZone, which is what makes it different from gossip.
func TestShoutCarriesOneZone(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	// The test world's two start rooms are in different zones: 1204 and
	// 3001. A shout from one must not reach the other.
	_, farAway := place(t, srv, fighterRecord("Bob", 10, 100), MortalStartRoom)
	_, nearBy := place(t, srv, fighterRecord("Cid", 10, 100), ImmortStartRoom)

	c.send("shout is anybody here")
	c.expect("You shout, 'is anybody here'")

	// See the note on settle() in TestWhisperAndAsk: seeing your own
	// reply does not mean the rest of the room has been written to yet.
	c.settle()
	if !nearBy.said("Zod shouts, 'is anybody here'") {
		t.Error("somebody in the same zone did not hear the shout")
	}
	if farAway.said("Zod shouts") {
		t.Error("a shout carried into another zone")
	}
}

// TestLevelCanShoutIsTunable: level_can_shout (config.c:61) is now a runtime
// setting (internal/game/tuning.go), not a constant — raise it and a brand
// new, level-one character must be refused.
func TestLevelCanShoutIsTunable(t *testing.T) {
	orig := game.Tuning()
	t.Cleanup(func() { game.SetTuning(orig) })
	tuning := orig
	tuning.LevelCanShout = 5
	game.SetTuning(tuning)

	srv, _ := newTestServer(t)
	// The first player on the roster is level 34 by init_char
	// (light_test.go's own note on the same rule), which would sail past
	// any level_can_shout this test sets. A second character is an
	// ordinary mortal.
	dialClient(t, listening(t, srv)).create("Filler", "swordfish", "m", "w")
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish2", "m", "w")

	c.send("shout is anybody here")
	c.expect("You must be at least level 5 before you can shout.")
}

// TestHollerMoveCostIsTunable: holler_move_cost (config.c:64) is now a
// runtime setting too — raise it above what a fresh character has, and
// holler must refuse for exhaustion rather than deduct movement it does
// not have.
func TestHollerMoveCostIsTunable(t *testing.T) {
	orig := game.Tuning()
	t.Cleanup(func() { game.SetTuning(orig) })
	tuning := orig
	tuning.HollerMoveCost = 1_000_000
	game.SetTuning(tuning)

	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	c.send("holler is anybody here")
	c.expect("You're too exhausted to holler.")
}

// TestWhisperAndAsk, and what the rest of the room sees.
func TestWhisperAndAsk(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	_, bob := place(t, srv, fighterRecord("Bob", 10, 100), ImmortStartRoom)
	_, cid := place(t, srv, fighterRecord("Cid", 10, 100), ImmortStartRoom)

	c.send("whisper")
	c.expect("Whom do you want to whisper to.. and what??")

	c.send("whisper bob meet me later")
	c.expect("You whisper to Bob, 'meet me later'")

	// A barrier before reading anybody else's buffer. `expect` waits for a
	// write to *this* client's socket, and the messages to the rest of the
	// room are separate writes on the world goroutine — seeing your own
	// reply does not mean theirs have happened yet. settle() sends a command
	// and waits for its answer, so everything the previous one queued is
	// done. Without it this test passes locally and fails on a busier
	// machine, which is exactly how it was found.
	c.settle()

	if !bob.said("Zod whispers to you, 'meet me later'") {
		t.Error("Bob did not hear the whisper")
	}
	// The room sees that something was said, not what.
	if !cid.said("Zod whispers something to Bob.") {
		t.Error("the room was not told a whisper happened")
	}
	if cid.said("meet me later") {
		t.Error("the room heard the whisper itself")
	}

	c.send("ask bob what time is it")
	c.expect("You ask Bob, 'what time is it'")
	c.settle()
	if !cid.said("Zod asks a question of Bob.") {
		t.Error("the room was not told about the question")
	}
}

// TestGroupSayReachesTheGroupAnywhere.
func TestGroupSayReachesTheGroupAnywhere(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	member, listener := place(t, srv, fighterRecord("Bob", 30, 100), ImmortStartRoom)

	c.send("gsay hello")
	c.expect("But you are not the member of a group!")

	inWorld(t, srv, func(w *game.Live) {
		zod := w.Find("Zod")
		w.AddFollower(member, zod)
		zod.SetGrouped(true)
		member.SetGrouped(true)
		// Somewhere else: a group-say does not care where they are.
		if err := w.Enter(member, MortalStartRoom); err != nil {
			t.Error(err)
		}
	})

	c.send("gsay this way")
	c.expect("You tell the group, 'this way'")
	// See the note on settle() in TestWhisperAndAsk: seeing your own
	// reply does not mean the rest of the room has been written to yet.
	c.settle()
	if !listener.said("Zod tells the group, 'this way'") {
		t.Error("the group member did not hear it")
	}
}

// TestQuestChannel.
func TestQuestChannel(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	c.send("qsay hello")
	c.expect("You aren't even part of the quest!")

	c.send("quest")
	c.expect("Okay, you are part of the Quest!")

	c.send("qsay hello")
	c.expect("You quest-say, 'hello'")
}

// TestSocialsWork end to end: the file's messages, resolved for the actor,
// the victim and a bystander.
func TestSocialsWork(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	victim, victimClient := place(t, srv, fighterRecord("Bob", 10, 100), ImmortStartRoom)
	victim.Record.Sex = game.SexMale
	_, bystander := place(t, srv, fighterRecord("Cid", 10, 100), ImmortStartRoom)

	// With no argument.
	c.send("smile")
	c.expect("You smile happily.")
	// See the note on settle() in TestWhisperAndAsk: seeing your own
	// reply does not mean the rest of the room has been written to yet.
	c.settle()
	if !bystander.said("Zod smiles happily.") {
		t.Error("the room did not see the smile")
	}

	// Aimed at somebody.
	c.send("smile bob")
	c.expect("You smile at him.")
	// See the note on settle() in TestWhisperAndAsk: seeing your own
	// reply does not mean the rest of the room has been written to yet.
	c.settle()
	if !victimClient.said("Zod smiles at you.") {
		t.Error("Bob was not smiled at")
	}
	if !bystander.said("Zod beams a smile at Bob.") {
		t.Error("the room did not see it")
	}

	// Aimed at yourself.
	c.send("smile zod")
	c.expect("You smile at yourself.")
	// See the note on settle() in TestWhisperAndAsk: seeing your own
	// reply does not mean the rest of the room has been written to yet.
	c.settle()
	if !bystander.said("Zod smiles at himself.") {
		t.Error("the room did not see the self-smile")
	}

	// Aimed at nobody.
	c.send("smile nobody")
	c.expect("There's no one by that name around.")
}

// TestASocialThatTakesNoTarget ignores its argument, as do_action does when
// the file gives it no char_found.
func TestASocialThatTakesNoTarget(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	place(t, srv, fighterRecord("Bob", 10, 100), ImmortStartRoom)

	c.send("applaud bob")
	c.expect("Clap, clap, clap.")
}

// TestASocialWontReachSomebodyLyingDown. Every social carries a minimum
// position for its victim, and below it there is one message for all of them.
func TestASocialWontReachSomebodyLyingDown(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	victim, _ := place(t, srv, fighterRecord("Bob", 10, 100), ImmortStartRoom)
	inWorld(t, srv, func(_ *game.Live) { victim.Position = game.PosSleeping })

	// accuse wants its victim at least resting.
	c.send("accuse bob")
	c.expect("Bob is not in a proper position for that.")
}

// TestASocialWithNothingInTheFile. `hop` is in the C's command table and not
// in the socials file, so it is a command that knows it cannot do anything —
// which is not the same as a word the game has never heard of.
func TestASocialWithNothingInTheFile(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	c.send("hop")
	c.expect("That action is not supported.")

	c.send("frobnicate")
	c.expect("Huh?!?")
}
