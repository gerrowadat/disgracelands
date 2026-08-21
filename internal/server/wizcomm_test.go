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

// Talking as a god, and making other people act.

func TestEchoAndEmote(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Narrator", "tellthetale", "m", "w")

	_, listener := place(t, srv, fighterRecord("Listener", 10, 100), ImmortStartRoom)

	c.send("echo")
	c.expect("Yes.. but what?")

	c.send("echo The sky darkens.")
	c.expect("The sky darkens.")
	c.settle()
	if !listener.said("The sky darkens.") {
		t.Error("the room did not hear the echo")
	}
	// An echo carries no name at all — that is the whole point of it.
	if listener.said("Narrator The sky darkens.") {
		t.Error("the echo was attributed")
	}

	c.send("emote shrugs.")
	c.settle()
	if !listener.said("Narrator shrugs.") {
		t.Error("the room did not hear the emote, or it was not attributed")
	}
}

// NOREPEAT swaps your own copy for a bare acknowledgement.
func TestNoRepeatOnEcho(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Quiet", "notoneself", "m", "w")

	inWorld(t, srv, func(w *game.Live) {
		if who := w.Find("Quiet"); who != nil && who.Record != nil {
			who.Record.Preferences = who.Record.Preferences.Set(game.PrefNoRepeat)
		}
	})

	c.send("echo Something happens.")
	c.expect("Okay.")
	c.settle()
	if c.seen("Something happens.") {
		t.Error("NOREPEAT still echoed the text back")
	}
}

func TestSend(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	god := dialClient(t, addr)
	god.create("Sender", "amessagefor", "m", "w")

	victim := dialClient(t, addr)
	victim.create("Target", "receivingend", "m", "w")
	setLevel(t, srv, "Target", 10)

	god.send("send")
	god.expect("Send what to who?")

	god.send("send nobodyatall hello")
	god.expect("No-one by that name here.")

	god.send("send target A voice from nowhere.")
	god.expect("You send 'A voice from nowhere.' to Target.")
	victim.expect("A voice from nowhere.")
}

func TestGecho(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	god := dialClient(t, addr)
	god.create("Herald", "tothewholeworld", "m", "w")

	// Somebody in a different room entirely.
	far := dialClient(t, addr)
	far.create("Faraway", "milesfromhere", "m", "w")
	setLevel(t, srv, "Faraway", 10)
	moveTo(t, srv, "Faraway", MageGuildRoom)

	god.send("gecho")
	god.expect("That must be a mistake...")

	god.send("gecho The world holds its breath.")
	god.expect("The world holds its breath.")
	far.expect("The world holds its breath.")
}

func TestForceOnePerson(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	god := dialClient(t, addr)
	god.create("Puppeteer", "dancemypuppet", "m", "w")

	victim := dialClient(t, addr)
	victim.create("Puppetted", "notmyownwill", "m", "w")
	setLevel(t, srv, "Puppetted", 10)
	moveTo(t, srv, "Puppetted", ImmortStartRoom)

	god.send("force")
	god.expect("Whom do you wish to force do what?")

	god.send("force nobodyatall smile")
	god.expect("No-one by that name here.")

	god.send("force puppetted smile")
	god.expect("Okay.")
	victim.expect("has forced you to 'smile'")
	victim.expect("You smile happily.")
}

// You cannot force somebody at or above your own level.
func TestYouCannotForceYourEquals(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	first := dialClient(t, addr)
	first.create("Weaker", "lessergodhere", "m", "w")

	second := dialClient(t, addr)
	second.create("Stronger", "greatergodhere", "m", "w")
	setLevel(t, srv, "Stronger", game.LevelImplementor)
	setLevel(t, srv, "Weaker", game.LevelGod)

	first.send("force stronger smile")
	first.expect("No, no, no!")
}

// `force room` reaches everybody present, and is a greater god's.
func TestForceTheRoom(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Conductor", "everyoneatonce", "m", "w")

	dog := aMobile(t, srv, "Conductor")
	if dog == nil {
		t.Fatal("no mobile")
	}

	c.send("force room smile")
	c.expect("Okay.")
	c.settle()
	if !c.seen("dog smiles happily") {
		t.Errorf("the mobile in the room was not forced:\n%s", c.transcript())
	}
}

func TestSyslog(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Watcher", "showmethelog", "m", "w")

	c.send("syslog")
	c.expect("Your syslog is currently off.")

	c.send("syslog nonsense")
	c.expect("Usage: syslog { Off | Brief | Normal | Complete }")

	// The two preference bits together make a number 0-3.
	for _, tc := range []struct {
		set  string
		want string
	}{
		{"brief", "brief"},
		{"normal", "normal"},
		{"complete", "complete"},
		{"off", "off"},
	} {
		c.send("syslog " + tc.set)
		c.expect("Your syslog is now " + tc.want + ".")
	}
}

func TestWiznet(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	first := dialClient(t, addr)
	first.create("Zeus", "kingofgods", "m", "w")

	second := dialClient(t, addr)
	second.create("Hermes", "themessenger", "m", "w")
	setLevel(t, srv, "Hermes", game.LevelImmortal)
	// A different room, to show the channel is not local.
	moveTo(t, srv, "Hermes", MageGuildRoom)

	first.send("wiznet")
	first.expect("Usage: wiznet <text>")

	first.send("wiznet anybody about?")
	first.expect("Zeus: anybody about?")
	second.expect("Zeus: anybody about?")

	// An emote is marked with the C's arrow.
	first.send("wiznet *waves")
	second.expect("Zeus: <--- waves")

	// A level restriction keeps it from the lesser gods.
	first.send("wiznet #34 for implementors only")
	first.expect("Zeus: <34> for implementors only")
	second.settle()
	if second.seen("for implementors only") {
		t.Error("a level-restricted wizline reached a lesser god")
	}

	// And you cannot shout above your own level.
	second.send("wiznet #34 let me try")
	second.expect("You can't wizline above your own level.")
}

// `;` is wiznet with no space, the way `'` is say.
func TestSemicolonIsWiznet(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Terse", "shortcutuser", "m", "w")

	c.send(";testing the shortcut")
	c.expect("Terse: testing the shortcut")
}

// NOWIZ takes you off the channel in both directions.
func TestWiznetOffline(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	first := dialClient(t, addr)
	first.create("Talker", "stillonline", "m", "w")

	second := dialClient(t, addr)
	second.create("Silent", "goneoffline", "m", "w")
	setLevel(t, srv, "Silent", game.LevelImmortal)
	inWorld(t, srv, func(w *game.Live) {
		if who := w.Find("Silent"); who != nil && who.Record != nil {
			who.Record.Preferences = who.Record.Preferences.Set(game.PrefNoWiz)
		}
	})

	second.send("wiznet hello?")
	second.expect("You are offline!")

	first.send("wiznet is anybody there")
	first.expect("Talker: is anybody there")
	second.settle()
	if second.seen("is anybody there") {
		t.Error("an offline god heard the wizline")
	}

	// `@` lists both sides.
	first.send("wiznet @")
	first.expect("Gods online:")
	first.expect("Talker")
	first.expect("Gods offline:")
	first.expect("Silent")
}

func TestMortalsCannotUseTheGodChannels(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	first := dialClient(t, addr)
	first.create("Divine", "theveryfirst", "m", "w")

	c := dialClient(t, addr)
	c.create("Ordinary", "justamortal", "m", "w")
	setLevel(t, srv, "Ordinary", 10)

	for _, command := range []string{"echo hello", "gecho hello", "send divine hi",
		"wiznet hello", "syslog", "force divine smile"} {
		c.send(command)
		c.expect("Huh?!?")
	}

	// But `emote` is a mortal command and still works.
	c.send("emote waves.")
	c.expect("Ordinary waves.")
}
