// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// Text holds the server's canned files.
//
// Two of these are licence obligations rather than content: the greeting must
// name the DikuMUD and CircleMUD creators, and the credits must be shown
// intact by the `credits` command (docs/proposals/go-port-plan.md §12). They
// are loaded at boot and their absence is a startup failure, not a warning —
// a server that cannot meet the licence should not begin serving.
type Text struct {
	greeting   string
	motd       string
	imotd      string
	credits    string
	background string
	news       string
	info       string
	policies   string
	handbook   string
	wizlist    string
	immlist    string
	// socials are the entries from misc/socials. They are commands rather
	// than text, but this is the thing that reads the data directory.
	socials []game.Social
}

// Socials returns the loaded socials, for the boot step that puts them into
// the command table.
func (t *Text) Socials() []game.Social {
	if t == nil {
		return nil
	}
	return t.socials
}

// text files, relative to the data directory.
const (
	greetingFile   = "text/greetings"
	motdFile       = "text/motd"
	imotdFile      = "text/imotd"
	creditsFile    = "text/credits"
	backgroundFile = "text/background"
	newsFile       = "text/news"
	infoFile       = "text/info"
	policiesFile   = "text/policies"
	handbookFile   = "text/handbook"
	wizlistFile    = "text/wizlist"
	immlistFile    = "text/immlist"
	// socialsFile is not text/ — the C's SOCMESS_FILE is lib/misc/socials.
	socialsFile = "misc/socials"
)

// MainMenu is the C's MENU (config.c:271), verbatim.
//
// Two of its six choices do not work yet — the background story and the
// description editor do, changing a password and deleting a character do, but
// nothing here reads mail or rents. The menu is the C's and stays the C's;
// see docs/deviations.md for what differs behind it.
const MainMenu = "\r\n" +
	"Welcome to CircleMUD!\r\n" +
	"0) Exit from CircleMUD.\r\n" +
	"1) Enter the game.\r\n" +
	"2) Enter description.\r\n" +
	"3) Read the background story.\r\n" +
	"4) Change password.\r\n" +
	"5) Delete this character.\r\n" +
	"\r\n" +
	"   Make your choice: "

// WelcomeMessage and StartMessage are WELC_MESSG and START_MESSG
// (config.c:284). They are compiled-in strings in the C rather than files, so
// they are compiled in here too, verbatim.
const (
	WelcomeMessage = "\r\nWelcome to the land of CircleMUD!  May your visit here be... Interesting.\r\n\r\n"

	StartMessage = "Welcome.  This is your new CircleMUD character!  You can now earn gold,\r\n" +
		"gain experience, find weapons and equipment, and much more -- while\r\n" +
		"meeting people from around the world!\r\n"
)

// LoadText reads the canned files from a data directory.
func LoadText(dir string) (*Text, error) {
	t := &Text{}

	// Required. The licence names both of these.
	for _, f := range []struct {
		path string
		dst  *string
		why  string
	}{
		{greetingFile, &t.greeting, "the login sequence must name the DikuMUD and CircleMUD creators"},
		{creditsFile, &t.credits, "the credits must be displayed by the `credits` command"},
	} {
		b, err := os.ReadFile(filepath.Join(dir, f.path)) //nolint:gosec // operator-configured data directory
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w (required: %s — see docs/proposals/go-port-plan.md §12)",
				f.path, err, f.why)
		}
		if strings.TrimSpace(string(b)) == "" {
			return nil, fmt.Errorf("%s is empty (required: %s)", f.path, f.why)
		}
		*f.dst = string(b)
	}

	// Optional: a server with no message of the day is merely quiet.
	for _, f := range []struct {
		path string
		dst  *string
	}{
		{motdFile, &t.motd},
		{imotdFile, &t.imotd},
		{backgroundFile, &t.background},
		{newsFile, &t.news},
		{infoFile, &t.info},
		{policiesFile, &t.policies},
		{handbookFile, &t.handbook},
		{wizlistFile, &t.wizlist},
		{immlistFile, &t.immlist},
	} {
		if b, err := os.ReadFile(filepath.Join(dir, f.path)); err == nil { //nolint:gosec // as above
			*f.dst = string(b)
		}
	}

	// The socials are optional in the same way the message of the day is: a
	// server without them is a poorer game and still a game. The C exits the
	// process on a missing socials file, which is a stronger reaction than
	// this port takes to anything that is not a licence obligation.
	if f, err := os.Open(filepath.Join(dir, socialsFile)); err == nil { //nolint:gosec // operator-configured data directory
		t.socials, err = game.ParseSocials(f)
		_ = f.Close()
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", socialsFile, err)
		}
	}

	return t, nil
}

// Greeting implements session.TextFiles.
func (t *Text) Greeting() string { return t.greeting }

// MOTD implements session.TextFiles.
func (t *Text) MOTD() string { return t.motd }

// ImmortalMOTD implements session.TextFiles. The C shows this instead of the
// ordinary message of the day to anyone of immortal level
// (interpreter.c:1504); a server without one falls back to the mortal file.
func (t *Text) ImmortalMOTD() string {
	if t.imotd == "" {
		return t.motd
	}
	return t.imotd
}

// Welcome implements session.TextFiles.
func (t *Text) Welcome() string { return WelcomeMessage }

// Start implements session.TextFiles.
func (t *Text) Start() string { return StartMessage }

// Menu implements session.TextFiles.
func (t *Text) Menu() string { return MainMenu }

// Background implements session.TextFiles: menu choice 3.
func (t *Text) Background() string {
	if t.background == "" {
		return "There is no background story on file.\r\n"
	}
	return t.background
}

// Credits implements session.TextFiles.
func (t *Text) Credits() string { return t.credits }

// News, Info, Policies, Handbook, WizList and ImmList are the rest of the
// canned files, each shown by one command.
func (t *Text) News() string { return t.news }

// Info is the `info` command's text.
func (t *Text) Info() string { return t.info }

// Policies is the `policy` command's text.
func (t *Text) Policies() string { return t.policies }

// Handbook is the immortals' handbook.
func (t *Text) Handbook() string { return t.handbook }

// WizList is the list of gods.
func (t *Text) WizList() string { return t.wizlist }

// ImmList is the shorter list the `immlist` command shows.
func (t *Text) ImmList() string { return t.immlist }
