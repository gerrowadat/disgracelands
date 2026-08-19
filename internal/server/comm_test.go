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
	if !listener.said("Zod says, 'hello there'") {
		t.Error("the room did not hear it")
	}

	// `'hi` with no space, which the interpreter special-cases.
	c.send("'well then")
	c.expect("You say, 'well then'")
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

	if !nearBy.said("Zod shouts, 'is anybody here'") {
		t.Error("somebody in the same zone did not hear the shout")
	}
	if farAway.said("Zod shouts") {
		t.Error("a shout carried into another zone")
	}
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
			t.Fatal(err)
		}
	})

	c.send("gsay this way")
	c.expect("You tell the group, 'this way'")
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
