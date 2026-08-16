// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"io"
	"log/slog"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gerrowadat/disgracelands/internal/auth"
	"github.com/gerrowadat/disgracelands/internal/engine"
	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/persist/player"
	"github.com/gerrowadat/disgracelands/internal/persist/player/ascii"
	"github.com/gerrowadat/disgracelands/internal/rng"
	"github.com/gerrowadat/disgracelands/internal/telnet"
)

// testGreeting stands in for data/text/greetings. The words the licence
// requires are checked in the real file by scripts/license-check.sh; what is
// checked here is that whatever the file says reaches the wire on every
// transport.
const testGreeting = "Welcome to the test MUD\r\n" +
	"Based on CircleMUD, created by Jeremy Elson.\r\n" +
	"Originally based on DikuMUD, created by Hans Henrik Staerfeldt,\r\n" +
	"Katja Nyboe, Tom Madsen, Michael Seifert, and Sebastian Hammer.\r\n" +
	"\r\nBy what name do you wish to be known? "

// testCredits stands in for data/text/credits. The marker line makes it
// distinguishable from the greeting, which necessarily says much the same
// thing — without it, a test waiting for "Jeremy Elson" matches the greeting
// it read minutes earlier and passes without the command having run.
const testCredits = "CREDITS-FILE\r\nCircleMUD, created by Jeremy Elson.\r\n"

func testText(t *testing.T) *Text {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "text"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		greetingFile:   testGreeting,
		creditsFile:    testCredits,
		motdFile:       "Mortal news.\r\n",
		imotdFile:      "Immortal news.\r\n",
		backgroundFile: "BACKGROUND-FILE\r\nIt is a time of conflict.\r\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	text, err := LoadText(dir)
	if err != nil {
		t.Fatal(err)
	}
	return text
}

// testWorld is the two start rooms, joined so a character can walk between
// them.
func testWorld() *game.Live {
	temple := &game.RoomDef{Vnum: MortalStartRoom, Name: "The Temple Of Midgaard", Description: "A temple.\r\n"}
	board := &game.RoomDef{Vnum: ImmortStartRoom, Name: "The Immortal Board Room", Description: "A board room.\r\n"}
	temple.Exits[game.North] = &game.ExitDef{ToRoom: ImmortStartRoom}
	board.Exits[game.South] = &game.ExitDef{ToRoom: MortalStartRoom}
	return game.NewLive(&game.World{Rooms: []*game.RoomDef{temple, board}})
}

// newTestServer builds a server on a temporary player directory and starts
// its engine.
func newTestServer(t *testing.T) (*Server, player.Store) {
	t.Helper()

	store, err := ascii.New(player.Config{Dir: filepath.Join(t.TempDir(), "pfiles")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	eng := engine.New(engine.Options{World: testWorld(), Logger: logger})

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go eng.Run(ctx)

	srv := New(Options{
		Engine:  eng,
		Players: store,
		Auth:    auth.Verifier{AllowLegacy: true},
		Text:    testText(t),
		Logger:  logger,
		RNG:     testRNG(),
	})
	return srv, store
}

// listening starts a telnet listener and its accept loop, returning the
// address to dial.
func listening(t *testing.T, srv *Server) string {
	t.Helper()
	ln, err := ListenTelnet("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	serveOn(t, srv, ln)
	return ln.Addr().String()
}

// serveOn runs an accept loop until the test ends.
func serveOn(t *testing.T, srv *Server, ln *Listener) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = srv.Accept(ctx, ln, Limits{MaxPerHost: 8, LoginGrace: time.Minute})
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
}

// TestEveryTransportSendsTheGreeting is a licence test, not a feature test.
//
// The CircleMUD licence requires the login sequence to name the DikuMUD and
// CircleMUD creators (docs/proposals/go-port-plan.md §12), and the greeting
// file is where that happens. A transport that renders its own splash screen
// and skips it would be a violation. Every transport goes through
// Server.Accept, so a new one is covered by adding a case here — and if
// someone adds a listener that bypasses Accept, this is the test that should
// have stopped them.
func TestEveryTransportSendsTheGreeting(t *testing.T) {
	srv, _ := newTestServer(t)

	for _, tc := range []struct {
		name string
		dial func(t *testing.T) *client
	}{
		{
			name: "telnet",
			dial: func(t *testing.T) *client {
				return dialClient(t, listening(t, srv))
			},
		},
		{
			name: "telnets",
			dial: func(t *testing.T) *client {
				ln, err := ListenTLS("127.0.0.1:0", &tls.Config{
					Certificates: []tls.Certificate{selfSignedCert(t)},
					MinVersion:   tls.VersionTLS12,
				})
				if err != nil {
					t.Fatal(err)
				}
				serveOn(t, srv, ln)

				conn, err := tls.Dial("tcp", ln.Addr().String(), &tls.Config{
					InsecureSkipVerify: true, //nolint:gosec // a self-signed certificate made for this test
					MinVersion:         tls.VersionTLS12,
				})
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = conn.Close() })
				return wrapClient(t, conn)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := tc.dial(t)
			got := c.expect("By what name")

			for _, required := range []string{
				"CircleMUD, created by Jeremy Elson.",
				"DikuMUD, created by Hans Henrik Staerfeldt,",
			} {
				if !strings.Contains(got, required) {
					t.Errorf("the greeting on %s does not name the creators (%q):\n%s",
						tc.name, required, got)
				}
			}
			// And it is the file, in full, not a paraphrase of it.
			if !strings.Contains(got, testGreeting) {
				t.Errorf("the greeting on %s is not the file verbatim:\n%q", tc.name, got)
			}
		})
	}
}

// TestTheGreetingIsSentBeforeAnythingIsRead: a client that sends nothing must
// still see it, because a player who connects and waits is the normal case.
func TestTheGreetingIsSentBeforeAnythingIsRead(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))

	if got := c.expect("By what name"); !strings.Contains(got, testGreeting) {
		t.Errorf("greeting not sent to a silent client:\n%q", got)
	}
}

// TestTheFirstCharacterIsAnImplementor covers init_char's first-player branch
// end to end: the level, the points it hands out directly, and the immortal
// message of the day.
func TestTheFirstCharacterIsAnImplementor(t *testing.T) {
	srv, store := newTestServer(t)
	c := dialClient(t, listening(t, srv))

	c.create("Zod", "swordfish", "m", "w")
	got := c.transcript()

	if !strings.Contains(got, "The Immortal Board Room") {
		t.Errorf("an Implementor did not start in the immortal room:\n%s", got)
	}
	// init_char's numbers, not do_start's: 500 hit points, 100 mana, 82 moves.
	// do_start must not have run at all for this character.
	if !strings.Contains(got, "500H 100M 82V > ") {
		t.Errorf("the prompt is not an Implementor's:\n%s", got)
	}
	if !strings.Contains(got, "Immortal news.") {
		t.Errorf("an Implementor was shown the mortal message of the day:\n%s", got)
	}
	if strings.Contains(got, "Mortal news.") {
		t.Errorf("an Implementor was shown the mortal message of the day:\n%s", got)
	}

	rec, err := store.Load(context.Background(), "Zod")
	if err != nil {
		t.Fatalf("the character was not saved: %v", err)
	}
	if rec.Level != game.LevelImplementor {
		t.Errorf("saved at level %d, want %d", rec.Level, game.LevelImplementor)
	}
	if len(rec.Skills) != game.MaxSkills {
		t.Errorf("knows %d skills, want all %d", len(rec.Skills), game.MaxSkills)
	}
}

// TestCreateAMortalAndLogBackIn is the phase's acceptance criterion in
// miniature: the full creation sequence, then out and back in again.
func TestCreateAMortalAndLogBackIn(t *testing.T) {
	srv, store := newTestServer(t)
	addr := listening(t, srv)

	// The first character on the roster is the Implementor, so make one to
	// get out of the way before testing an ordinary mortal.
	first := dialClient(t, addr)
	first.create("Zod", "swordfish", "m", "w")
	first.send("quit")
	first.expect("Goodbye")
	first.close()

	c := dialClient(t, addr)
	c.create("Welmar", "hunter2!", "f", "t")
	got := c.transcript()

	if !strings.Contains(got, "The Temple Of Midgaard") {
		t.Errorf("a mortal did not start in the temple:\n%s", got)
	}
	if !strings.Contains(got, "Mortal news.") {
		t.Errorf("a mortal was not shown the mortal message of the day:\n%s", got)
	}
	if !strings.Contains(got, "This is your new CircleMUD character!") {
		t.Errorf("a new character was not shown START_MESSG:\n%s", got)
	}
	if !strings.Contains(got, "Welcome to the land of CircleMUD!") {
		t.Errorf("a character entering the world was not shown WELC_MESSG:\n%s", got)
	}

	rec, err := store.Load(context.Background(), "Welmar")
	if err != nil {
		t.Fatal(err)
	}
	if rec.Level != 1 {
		t.Errorf("a new mortal is at level %d, want 1 after do_start", rec.Level)
	}
	if rec.Points.MaxMana != 100 {
		t.Errorf("mana is %d, want the 100 init_char gives and do_start leaves alone", rec.Points.MaxMana)
	}
	if rec.Title != "the Pilferess" {
		t.Errorf("title is %q, want a level-one female thief's", rec.Title)
	}
	if len(rec.Skills) != 6 {
		t.Errorf("a new thief knows %d skills, want 6", len(rec.Skills))
	}
	if rec.RemortVector == 0 {
		t.Error("a new character's remort vector was not set to their own class")
	}

	c.send("quit")
	c.expect("Goodbye")
	c.close()

	// Back in. The name is matched case-insensitively, and the start message
	// belongs only to a character entering for the first time.
	again := dialClient(t, addr)
	again.login("welmar", "hunter2!")
	got = again.transcript()

	if again.seen("This is your new CircleMUD character!") {
		t.Error("a returning character was shown the start message again")
	}
	if !strings.Contains(got, "The Temple Of Midgaard") {
		t.Errorf("a returning character did not land where they left:\n%s", got)
	}
	if !strings.Contains(got, "23H") && !strings.Contains(got, "H 100M") {
		t.Errorf("a returning character's prompt lost their points:\n%s", got)
	}
}

// TestAWrongPasswordIsRefused, and says nothing about which half was wrong.
func TestAWrongPasswordIsRefused(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	rec := &game.PlayerRecord{Name: "Welmar", Class: game.ClassThief}
	cred, err := auth.NewCredential("swordfish")
	if err != nil {
		t.Fatal(err)
	}
	game.InitChar(rec, testRNG(), false)
	rec.Credential = cred
	if err := srv.players.Save(context.Background(), rec); err != nil {
		t.Fatal(err)
	}

	c := dialClient(t, addr)
	c.expect("By what name")
	c.send("Welmar")
	c.expect("Password:")
	c.send("not-the-password")

	got := c.expect("Wrong password")
	for _, leak := range []string{"no such", "unknown", "does not exist"} {
		if strings.Contains(strings.ToLower(got), leak) {
			t.Errorf("the refusal distinguishes a bad name from a bad password:\n%s", got)
		}
	}
}

// TestEchoIsTurnedOffForPasswords: the C sends IAC WILL ECHO before a
// password prompt and IAC WONT ECHO after (comm.c's echo_off_str). A player
// left with echo off is a player typing blind, so the off and the on are both
// checked.
func TestEchoIsTurnedOffForPasswords(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))

	c.expect("By what name")
	// Nothing secret is typed at the name prompt, so echo must still be on.
	if bytes.Contains(c.wire(), telnet.Negotiate(telnet.WILL, telnet.OptEcho)) {
		t.Errorf("echo was turned off before the name prompt: % x", c.wire())
	}

	c.send("Zod")
	c.expect("Did I get that right")
	c.send("y")
	c.expect("Give me a password")

	if !bytes.Contains(c.wire(), telnet.Negotiate(telnet.WILL, telnet.OptEcho)) {
		t.Errorf("no IAC WILL ECHO before the password prompt: % x", c.wire())
	}
	if bytes.Contains(c.wire(), telnet.Negotiate(telnet.WONT, telnet.OptEcho)) {
		t.Errorf("echo was turned back on before the password was typed: % x", c.wire())
	}

	c.send("swordfish")
	c.expect("retype password")
	if bytes.Contains(c.wire(), telnet.Negotiate(telnet.WONT, telnet.OptEcho)) {
		t.Errorf("echo was turned back on while a password was still being typed: % x", c.wire())
	}

	c.send("swordfish")
	c.expect("What is your sex")
	if !bytes.Contains(c.wire(), telnet.Negotiate(telnet.WONT, telnet.OptEcho)) {
		t.Errorf("echo was not turned back on after the password: % x", c.wire())
	}
}

// TestOptionsAreOfferedOnConnect covers the negotiation the C does not do at
// all — which is why a modern client's own offers end up in its command
// stream there.
func TestOptionsAreOfferedOnConnect(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.expect("By what name")

	for _, opt := range []byte{telnet.OptSuppressGoAhead, telnet.OptCharset, telnet.OptGMCP} {
		if !bytes.Contains(c.wire(), telnet.Negotiate(telnet.WILL, opt)) {
			t.Errorf("%s was not offered: % x", telnet.OptionName(opt), c.wire())
		}
	}
}

// TestNegotiationNeverReachesTheInterpreter is the bug this whole layer
// exists to prevent: in the C, a client that offers window size has its NAWS
// bytes read as a command.
func TestNegotiationNeverReachesTheInterpreter(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.expect("By what name")

	// Offer window size, then send one, then type a name — all in the shape a
	// real client would.
	c.sendRaw(telnet.Negotiate(telnet.WILL, telnet.OptWindowSize))
	c.sendRaw(telnet.Subnegotiate(telnet.OptWindowSize, []byte{0, 80, 0, 24}))
	c.send("Zod")

	got := c.expect("Did I get that right")
	if !strings.Contains(got, "Did I get that right, Zod") {
		t.Errorf("negotiation disturbed the name that was typed:\n%s", got)
	}
	// And the server declines what it did not ask for, rather than leaving
	// the client waiting for an answer.
	if !bytes.Contains(c.wire(), telnet.Negotiate(telnet.DONT, telnet.OptWindowSize)) {
		t.Errorf("an unrequested option was neither accepted nor refused: % x", c.wire())
	}
}

// TestGMCPCarriesTheVitals: a client that turns GMCP on gets the prompt's
// numbers and the room out of band, which is what makes a browser client
// possible rather than a screen-scraper.
func TestGMCPCarriesTheVitals(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))

	c.expect("By what name")
	c.sendRaw(telnet.Negotiate(telnet.DO, telnet.OptGMCP))
	c.create("Zod", "swordfish", "m", "w")

	var sawVitals, sawRoom bool
	for _, msg := range c.gmcp() {
		switch msg.Package {
		case "Char.Vitals":
			sawVitals = true
			if !strings.Contains(string(msg.Data), `"hp":500`) {
				t.Errorf("Char.Vitals is %s, want the Implementor's 500 hit points", msg.Data)
			}
		case "Room.Info":
			sawRoom = true
			if !strings.Contains(string(msg.Data), "The Immortal Board Room") {
				t.Errorf("Room.Info is %s", msg.Data)
			}
		}
	}
	if !sawVitals {
		t.Error("no Char.Vitals was sent to a client that enabled GMCP")
	}
	if !sawRoom {
		t.Error("no Room.Info was sent to a client that enabled GMCP")
	}
}

// TestGMCPHonoursCoreSupports: a client that asked for nothing but Room gets
// nothing but Room.
func TestGMCPHonoursCoreSupports(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))

	c.expect("By what name")
	c.sendRaw(telnet.Negotiate(telnet.DO, telnet.OptGMCP))
	c.sendRaw(telnet.Subnegotiate(telnet.OptGMCP, []byte(`Core.Supports.Set ["Room 1"]`)))
	c.create("Zod", "swordfish", "m", "w")

	for _, msg := range c.gmcp() {
		if msg.Package == "Char.Vitals" {
			t.Errorf("Char.Vitals was sent to a client that asked only for Room: %s", msg.Data)
		}
	}
}

// TestGMCPIsSilentForAClientThatDidNotAskForIt.
func TestGMCPIsSilentForAClientThatDidNotAskForIt(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	if msgs := c.gmcp(); len(msgs) != 0 {
		t.Errorf("GMCP was sent to a client that never enabled it: %+v", msgs)
	}
}

// TestWalkingBetweenRooms is the rest of the acceptance criterion: a
// character can look around and move.
func TestWalkingBetweenRooms(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	first := dialClient(t, addr)
	first.create("Zod", "swordfish", "m", "w")

	first.send("south")
	if got := first.expect("The Temple Of Midgaard"); !strings.Contains(got, "The Temple Of Midgaard") {
		t.Errorf("could not walk south:\n%s", got)
	}
	first.send("north")
	first.expect("The Immortal Board Room")

	first.send("east")
	if got := first.expect("cannot go that way"); !strings.Contains(got, "cannot go that way") {
		t.Errorf("walking into a wall did not say so:\n%s", got)
	}
}

// TestCreditsAndHelpCircleMUD are licence obligations rather than features
// (docs/proposals/go-port-plan.md §12), so they are tested as such.
func TestCreditsAndHelpCircleMUD(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	c.send("credits")
	c.expect("CREDITS-FILE")
	if !strings.Contains(c.transcript(), testCredits) {
		t.Errorf("`credits` did not show the credits file intact:\n%s", c.transcript())
	}

	// The CIRCLEMUD help entry is the second half of the same obligation.
	c.send("help circlemud")
	c.expectCount("CREDITS-FILE", 2)
	if n := strings.Count(c.transcript(), "CREDITS-FILE"); n < 2 {
		t.Errorf("`help circlemud` did not show the credits (seen %d times):\n%s",
			n, c.transcript())
	}
}

// TestTooManyConnectionsFromOneAddress covers the limit the C server has no
// equivalent of.
func TestTooManyConnectionsFromOneAddress(t *testing.T) {
	srv, _ := newTestServer(t)
	ln, err := ListenTelnet("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = srv.Accept(ctx, ln, Limits{MaxPerHost: 2})
	}()
	// Cancel first, then wait: deferred calls run in reverse, so writing
	// these the other way round waits for an accept loop that has not been
	// told to stop.
	defer func() {
		cancel()
		<-done
	}()

	for i := 0; i < 2; i++ {
		held := dialClient(t, ln.Addr().String())
		held.expect("By what name")
	}

	third := dialClient(t, ln.Addr().String())
	third.expect("Too many connections")
}

// testRNG is the C server's own generator on a fixed seed: a failing test can
// be reproduced, and the numbers are the ones the C would roll.
func testRNG() *rng.Rand { return rng.NewRand(rng.NewCircle(1)) }

// selfSignedCert makes a certificate for the TLS listener test.
func selfSignedCert(t *testing.T) tls.Certificate {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}
