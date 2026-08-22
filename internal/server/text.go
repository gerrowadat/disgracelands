// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/persist/messages"
)

// Text holds the server's canned files.
//
// Two of these are licence obligations rather than content: the greeting must
// name the DikuMUD and CircleMUD creators, and the credits must be shown
// intact by the `credits` command (docs/proposals/go-port-plan.md §12). They
// are loaded at boot and their absence is a startup failure, not a warning —
// a server that cannot meet the licence should not begin serving.
type Text struct {
	// mu guards everything below, because `reload` replaces these while
	// sessions are reading them.
	//
	// Nothing else in the server needs a lock — the world goroutine owns the
	// world — but the canned text is read from *two* places: commands, on the
	// world goroutine, and the greeting, on the connection goroutine before a
	// session has a character at all. One implementor-only command that
	// rewrites them is enough to make that a race, so it is an RWMutex and the
	// readers are one line each.
	mu sync.RWMutex

	// dir is the data directory these were read from, so `reload` can read
	// them again. Set by LoadText and never changed.
	dir string
	// messagesFormat is misc/messages'/config/messages.yaml's format —
	// stored alongside dir for the same reason, so Reload (which does not
	// currently reload messages at all; see its own doc comment) still
	// passes LoadText the format it was originally configured with.
	messagesFormat string

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
	// help is the loaded, sorted help table (text/help/index plus each
	// file it lists). nil for a server with no help data — a poorer game,
	// not a broken one, the same posture socials already has.
	help *game.HelpIndex
	// helpScreen is text/help/screen, HELP_PAGE_FILE — what bare `help`
	// shows instead of a lookup.
	helpScreen string
	// messages is the loaded misc/messages table (skill_message's weapon-
	// type entries, consulted by the ordinary combat swing). nil for a
	// server with none — dam_message's compiled table is what a fresh
	// server always had before this existed, and stays the whole story
	// when there is nothing to prefer over it.
	messages *game.FightMessages
}

// FightMessages returns the loaded misc/messages table. Safe to call and
// to Pick from when t or the table itself is nil.
func (t *Text) FightMessages() *game.FightMessages {
	if t == nil {
		return nil
	}
	return t.messages
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

	// helpDir, helpIndexFile and helpScreenFile are text/help/, its index
	// (db.h's nameless index_filename under HLP_PREFIX) and screen
	// (HELP_PAGE_FILE, db.h:78).
	helpDir        = "text/help"
	helpIndexFile  = "text/help/index"
	helpScreenFile = "text/help/screen"

	// messagesFile is MESS_FILE (db.h:89): lib/misc/messages.
	messagesFile = "misc/messages"
	// messagesConfigDir is where config/messages.yaml lives under native
	// — the same config/ directory names.yaml already shares.
	messagesConfigDir = "config"
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
func LoadText(dir, messagesFormat string) (*Text, error) {
	t := &Text{dir: dir, messagesFormat: messagesFormat}

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
		{helpScreenFile, &t.helpScreen},
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

	// The help table, same optional posture as socials: index_boot
	// (db.c:699-817) reads text/help/index, one filename per line, then
	// load_help (db.c:1701-1734) on each in turn into one shared table,
	// sorted by hsort (db.c:1739-1747) once loading finishes.
	if f, err := os.Open(filepath.Join(dir, helpIndexFile)); err == nil { //nolint:gosec // operator-configured data directory
		files, err := game.ParseHelpIndex(f)
		_ = f.Close()
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", helpIndexFile, err)
		}
		var entries []game.HelpEntry
		for _, name := range files {
			path := filepath.Join(helpDir, name)
			hf, err := os.Open(filepath.Join(dir, path)) //nolint:gosec // as above
			if err != nil {
				return nil, fmt.Errorf("reading %s: %w", path, err)
			}
			fileEntries, err := game.ParseHelpFile(hf)
			_ = hf.Close()
			if err != nil {
				return nil, fmt.Errorf("reading %s: %w", path, err)
			}
			entries = append(entries, fileEntries...)
		}
		t.help = game.NewHelpIndex(entries)
	}

	// misc/messages, the same optional posture as help and socials —
	// porting load_messages (fight.c:145-193). classic is a file, native
	// a directory — the same asymmetry every other pluggable format in
	// this tree already has.
	messagesPath := filepath.Join(dir, messagesFile)
	if messagesFormat == "native" {
		messagesPath = filepath.Join(dir, messagesConfigDir)
	}
	records, err := messages.Load(messagesFormat, messagesPath)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", messagesPath, err)
	}
	t.messages = game.NewFightMessages(records)

	return t, nil
}

// Greeting implements session.TextFiles.
func (t *Text) Greeting() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.greeting
}

// MOTD implements session.TextFiles.
func (t *Text) MOTD() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.motd
}

// ImmortalMOTD implements session.TextFiles. The C shows this instead of the
// ordinary message of the day to anyone of immortal level
// (interpreter.c:1504); a server without one falls back to the mortal file.
func (t *Text) ImmortalMOTD() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
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
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.background == "" {
		return "There is no background story on file.\r\n"
	}
	return t.background
}

// Credits implements session.TextFiles.
func (t *Text) Credits() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.credits
}

// News, Info, Policies, Handbook, WizList and ImmList are the rest of the
// canned files, each shown by one command.
func (t *Text) News() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.news
}

// Info is the `info` command's text.
func (t *Text) Info() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.info
}

// Policies is the `policy` command's text.
func (t *Text) Policies() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.policies
}

// Handbook is the immortals' handbook.
func (t *Text) Handbook() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.handbook
}

// WizList is the list of gods.
func (t *Text) WizList() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.wizlist
}

// ImmList is the shorter list the `immlist` command shows.
func (t *Text) ImmList() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.immlist
}

// HelpScreen is HELP_PAGE_FILE (db.h:78): what bare `help` shows instead
// of a lookup.
func (t *Text) HelpScreen() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.helpScreen
}

// Help is do_help's lookup (act.informative.c:966-988), reporting whether
// anything matched. False both when nothing matches and when there is no
// help data at all — the caller cannot and does not need to tell those
// apart.
func (t *Text) Help(query string) (string, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.help == nil {
		return "", false
	}
	e, ok := t.help.Lookup(query)
	return e.Body, ok
}

// Reload re-reads one of the canned files, porting do_reboot (db.c:195).
//
// The C's body is an else-if chain over the argument, one file each, plus
// `all` (or `*`) for the lot — and this is the same chain. Two names are worth
// knowing apart because they read alike and are not: **`help` is the help
// *screen*** (HELP_PAGE_FILE, what bare `help` shows), while **`xhelp` is the
// help *database*** — the index and every file it lists.
//
// It reads on the calling goroutine, which for a command means the world
// goroutine. That is a deliberate exception to the rule that I/O runs off it:
// these are a dozen small files, it is an implementor-only command run about
// as often as the server is upgraded, and the alternative — a command whose
// effect arrives some time after it returns — is worse to use and unlike the
// C. Recorded in docs/deviations.md.
func (t *Text) Reload(what string) error {
	if t == nil {
		return errUnknownReload
	}
	what = strings.ToLower(strings.TrimSpace(what))

	// Read first, then swap: a file that has gone missing since boot leaves
	// the old text in place rather than blanking it, which is not the C's
	// behaviour — file_to_string_alloc leaves the pointer alone on failure
	// too, so the effect is the same and the reasoning is explicit.
	fresh, err := LoadText(t.dir, t.messagesFormat)
	if err != nil {
		return err
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	swap := map[string]func(){
		"wizlist":    func() { t.wizlist = fresh.wizlist },
		"immlist":    func() { t.immlist = fresh.immlist },
		"news":       func() { t.news = fresh.news },
		"credits":    func() { t.credits = fresh.credits },
		"motd":       func() { t.motd = fresh.motd },
		"imotd":      func() { t.imotd = fresh.imotd },
		"help":       func() { t.helpScreen = fresh.helpScreen },
		"info":       func() { t.info = fresh.info },
		"policy":     func() { t.policies = fresh.policies },
		"handbook":   func() { t.handbook = fresh.handbook },
		"background": func() { t.background = fresh.background },
		"greetings":  func() { t.greeting = fresh.greeting },
		"xhelp":      func() { t.help = fresh.help },
	}

	if what == "all" || what == "*" {
		// The C's `all` does not include xhelp — it lists the twelve
		// file_to_string_alloc calls and stops. Reproduced, so `reload all`
		// followed by a puzzled `reload xhelp` is the same sequence it always
		// was.
		for name, apply := range swap {
			if name != "xhelp" {
				apply()
			}
		}
		return nil
	}

	apply, ok := swap[what]
	if !ok {
		return errUnknownReload
	}
	apply()
	return nil
}

// errUnknownReload is what `reload` says for a name it does not know.
var errUnknownReload = errors.New("unknown reload option")

// ErrUnknownReload reports whether an error is that one, so the session layer
// can print the C's message without importing the reason.
func ErrUnknownReload(err error) bool { return errors.Is(err, errUnknownReload) }
