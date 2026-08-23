// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package session

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/gerrowadat/disgracelands/internal/colour"
	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/rng"
)

// Command is one thing a player can type.
type Command struct {
	// Name is the full word. Players may type any unambiguous prefix of it.
	Name string
	// Help is the one-line description the command list shows.
	Help string
	// Run does the thing. It runs on the world goroutine, so it may touch
	// the world freely and must not block.
	Run func(cmd *Context) error
	// CLine is this command's line in `interpreter.c`, and it is what the
	// table is *sorted by*.
	//
	// The interpreter matches the first entry a typed word is a prefix of, so
	// the order of the table decides what every abbreviation means — twenty
	// years of muscle memory, resting on a list somebody wrote by hand in
	// 1993. Ordering by the C's own line number makes that derived rather
	// than maintained, and it is the only way the socials can be interleaved
	// in their right places: they are a third of the C's table and they are
	// loaded from a file at boot.
	CLine int
	// MinLevel is the level a character must be to use this command, and it
	// is checked *while matching* rather than after — see lookup.
	MinLevel int32
	// MinPosition is `cmd_info[]`'s second column: the position a character
	// must be in at least, or the interpreter refuses with a message chosen by
	// the position they are actually in (interpreter.c:636).
	//
	// Unlike MinLevel this is *not* part of matching. A command you are too
	// prone to use is still found, and still yours — you are told you cannot
	// do it now, rather than told the word means nothing.
	//
	// Filled in by commandTable from commandPositions rather than written
	// here; see that map for why.
	MinPosition game.Position
	// Social is the social this command runs, for the entries built from
	// `data/misc/socials` rather than written here.
	Social *game.Social
}

// Context is what a command is given.
type Context struct {
	Ctx       context.Context
	Session   *Session
	Character *game.Character
	World     *game.Live
	Text      TextFiles
	// RNG is the game's generator, for commands that roll — fleeing picks a
	// random direction.
	RNG *rng.Rand
	// Violence resolves damage, so that every command which can kill somebody
	// goes through the same code as the combat round.
	Violence Violence
	// Save writes a character's record back. It returns at once and does the
	// writing elsewhere: this runs on the world goroutine, and a rule that
	// waits for a disk stops the game for everybody.
	Save func(*game.Character)
	// Rent stores a character's belongings and takes them out of the world,
	// for the receptionist.
	Rent RentSaver
	// SaveBoard writes a bulletin board back to disk.
	SaveBoard BoardSaver
	// Mail is the mud mail system, for the postmaster.
	Mail MailSystem
	// Houses is the player housing system.
	Houses HouseKeeper
	// Operator is the connections and the shutdown switch, for the commands
	// that run the server rather than the game.
	Operator Operator
	// Bans is the site ban list.
	Bans BanKeeper
	// Reports is the bug/idea/typo report log, for `bug`/`idea`/`typo`.
	Reports ReportWriter
	// SetPassword replaces somebody's credential, for `set <name> passwd`.
	// A seam rather than a field write, because hashing a password is the
	// auth package's business and not the session's.
	SetPassword func(c *game.Character, password string) error
	// TextEdit reads and writes a canned text file by name, for `tedit`.
	TextEdit TextEditor
	// MobReload hot-reloads a mobile prototype from disk, for `reloadmob`.
	MobReload MobReloader
	// Arg is everything after the command word, trimmed.
	Arg string
	// Social is the social being run, for the commands that are one.
	Social *game.Social
}

// Violence is how a command hurts somebody.
//
// The rules for what follows a blow — the corpse, the experience, the
// alignment, whether the victim is now dead — belong with the combat round
// rather than with each command that can cause one. A command says what it did
// and hands over the damage; this decides what that means.
type Violence interface {
	// Damage applies damage and everything that follows from it, returning
	// what was actually taken after sanctuary and the rest.
	Damage(w *game.Live, attacker, victim *game.Character, amount int32) int32
	// SkillDamage is Damage plus skill_message's own weapon-free half
	// (fight.c's damage() dispatch, the branch every attack that is not a
	// weapon swing takes): the real message registered under skillType in
	// misc/messages, or silence if there is none — never the compiled
	// dam_message table, which only ever answers for a weapon attack.
	// amount 0 is a miss, the same "not a separate code path" rule the
	// ordinary weapon swing follows.
	SkillDamage(w *game.Live, attacker, victim *game.Character, amount, skillType int32) int32
	// Swing is one weapon attack, taken now rather than on the next round.
	Swing(w *game.Live, attacker, victim *game.Character)
}

// MobReloader hot-reloads a mobile prototype from disk, for `reloadmob`
// — new capability, not a C port; see docs/deviations.md. ok is false
// (refreshed always 0) if some current instance of vnum is fighting: the
// update mutates one prototype object every live instance shares, so it
// is all-or-nothing.
type MobReloader interface {
	ReloadMobile(w *game.Live, vnum game.MobVnum) (refreshed int, err error)
}

// ErrMobEngaged is what a MobReloader returns when some current instance
// of the vnum is fighting — see MobReloader's own doc comment for why
// that makes a reload all-or-nothing.
var ErrMobEngaged = errors.New("in combat")

// TextEditor is tedit's own seam: reading a canned text file's current
// content and writing a new one back, both by symbolic name — the same
// belongs-with-the-server shape BoardSaver already has for a board.
type TextEditor interface {
	// TextField returns a canned text file's current content, and
	// whether name is one tedit recognises at all.
	TextField(name string) (text string, ok bool)
	// SetTextField writes a new value for name, both in memory (so
	// every other command sees it immediately) and to disk, pushed off
	// the world goroutine — see Server.background's own doc comment.
	// False means name was not recognised.
	SetTextField(name, text string) bool
}

// Send writes to the player who typed the command.
//
// A Context with no session belongs to something the world is doing on its
// own — a mobile's special procedure casting a spell — and its output goes to
// the character's client, which for a mobile is nobody. That is what
// send_to_char does there too.
func (c *Context) Send(format string, args ...any) {
	if c.Session == nil {
		c.Character.Tell(format, args...)
		return
	}
	c.Session.Send(format, args...)
}

// SendAt is Send for a message whose colour is at a level other than the
// ordinary one — the combat lines are C_CMP, so somebody on "normal" colour
// sees the fight in plain text. See internal/colour.
func (c *Context) SendAt(want colour.Level, format string, args ...any) {
	if c.Session == nil {
		c.Character.Tell(format, args...)
		return
	}
	c.Session.SendAt(want, format, args...)
}

// Commands is the command table.
//
// Order matters: the C server's interpreter matches the first entry whose
// name the typed word is a prefix of, so "n" is north and not "news" purely
// because of where they sit in the table. That behaviour is player-visible
// muscle memory — someone who has typed "n" for twenty years should not find
// it means something else — so the order here is deliberate and the movement
// commands come first.
// Populated in init() rather than as a literal: doHelp lists the table, so a
// literal would be an initialisation cycle.
var Commands []Command

// staticCommands are the ones written in Go. The socials are added to them at
// boot by RegisterSocials, and the whole lot is then sorted by CLine.
var staticCommands []Command

func init() {
	staticCommands = []Command{
		{Name: "north", Help: "Move north.", Run: move(game.North), CLine: 216},
		{Name: "east", Help: "Move east.", Run: move(game.East), CLine: 217},
		{Name: "south", Help: "Move south.", Run: move(game.South), CLine: 218},
		{Name: "west", Help: "Move west.", Run: move(game.West), CLine: 219},
		{Name: "up", Help: "Move up.", Run: move(game.Up), CLine: 220},
		{Name: "down", Help: "Move down.", Run: move(game.Down), CLine: 221},

		{Name: "look", Help: "Look at the room around you.", Run: doLook, CLine: 355},
		{Name: "kill", Help: "Attack someone.", Run: doKill, CLine: 351},
		{Name: "kick", Help: "Kick someone.", Run: doKick, CLine: 352},
		// backstab before bash, which is the C's order (interpreter.c:235 and
		// :238) and therefore what `ba` means.
		{Name: "backstab", Help: "Stab someone who is not looking.", Run: doBackstab, CLine: 235},
		{Name: "bash", Help: "Knock someone over.", Run: doBash, CLine: 238},
		// alias is interpreter.c:226, ahead of at/advance (:224/:225, both
		// LVL_IMMORT and so invisible to a mortal's own abbreviation
		// matching) and ahead of assist (:229) too — so a mortal's bare `a`
		// has always meant alias in the C, not assist. Landing this command
		// is what makes that abbreviation correct rather than assist quietly
		// squatting on it for want of anything earlier being implemented.
		{Name: "alias", Help: "Define or list command aliases: alias, alias <name>, alias <name> <commands>.", Run: doAlias, CLine: 226},
		// The C has assist at interpreter.c:229, before ask — so `as` is
		// assist. Nothing ported shares its prefix.
		{Name: "assist", Help: "Join a fight on somebody's side.", Run: doAssist, CLine: 229},
		// ask and auction are :230 and :231, straight after assist — so `as`
		// is assist and asking somebody needs `ask`.
		{Name: "ask", Help: "Ask somebody something quietly.", Run: doAsk, CLine: 230},
		{Name: "auction", Help: "Announce something for sale, game-wide.", Run: doAuction, CLine: 231},
		{Name: "murder", Help: "Attack another player.", Run: doMurder, CLine: 372},
		{Name: "cast", Help: "Cast a spell: cast 'magic missile' <target>.", Run: doCast, CLine: 249},
		{Name: "flee", Help: "Run away from a fight.", Run: doFlee, CLine: 297},
		// `fill` is at interpreter.c:296, ahead of flee — but a mortal's `f`
		// reaches neither in the C: the lookup skips commands above their
		// level, so `force` (:294) is passed over and `fart` (:295) wins.
		// Socials are not ported, so no ordering here reproduces that. Flee
		// keeps `f` until they are.
		{Name: "fill", Help: "Fill a container from a fountain.", Run: doFill, CLine: 296},
		// After fill (interpreter.c:296 and :300), so `fo` is follow and `fi`
		// is fill.
		{Name: "follow", Help: "Follow somebody, or `follow self` to stop.", Run: doFollow, CLine: 300},

		{Name: "stand", Help: "Get to your feet.", Run: doStand, CLine: 490},
		// sip before sit is the C's order (interpreter.c:467 and :468), so
		// `si` takes a drink and sitting down needs the whole word.
		{Name: "sip", Help: "Take a small drink.", Run: doSip, CLine: 467},
		{Name: "sit", Help: "Sit down.", Run: doSit, CLine: 468},
		// reply is :425, immediately before rest — so `r` is reply, which is
		// twenty years of muscle memory for anybody who has ever been told
		// something.
		{Name: "reply", Help: "Answer whoever last told you something.", Run: doReply, CLine: 425},
		{Name: "rest", Help: "Rest, to recover faster.", Run: doRest, CLine: 426},
		{Name: "sleep", Help: "Sleep, to recover faster still.", Run: doSleep, CLine: 470},
		// After sleep, as in the C (interpreter.c:466 and :479), so `sl` is
		// sleep and sneaking needs `sn`.
		{Name: "sneak", Help: "Try to move without being heard.", Run: doSneak, CLine: 479},
		// After rest, as in the C (interpreter.c:426 and :441), so `res` is
		// still rest and only `resc` reaches rescue.
		{Name: "rescue", Help: "Take somebody else's fight onto yourself.", Run: doRescue, CLine: 441},
		{Name: "wake", Help: "Wake up, or wake somebody else.", Run: doWake, CLine: 536},

		// Before `pick`, `pour` and `practice`, which is the C's order
		// (interpreter.c:396, :401, :408 and :411) and therefore what a bare
		// `p` means: put.
		{Name: "noauction", Help: "Stop hearing the auction channel.", Run: toggleCommand("noauction"), CLine: 377},
		{Name: "nogossip", Help: "Stop hearing the gossip channel.", Run: toggleCommand("nogossip"), CLine: 378},
		{Name: "nograts", Help: "Stop hearing congratulations.", Run: toggleCommand("nograts"), CLine: 379},
		{Name: "norepeat", Help: "Stop having your own words repeated back.", Run: toggleCommand("norepeat"), CLine: 381},
		{Name: "noshout", Help: "Stop hearing shouts.", Run: toggleCommand("noshout"), CLine: 382},
		{Name: "nosummon", Help: "Refuse to be summoned by other players.", Run: toggleCommand("nosummon"), CLine: 383},
		{Name: "notell", Help: "Stop hearing tells.", Run: toggleCommand("notell"), CLine: 384},

		{Name: "put", Help: "Put something into a container.", Run: doPut, CLine: 396},

		{Name: "practice", Help: "Practise a spell or skill, or list what you know.", Run: doPractice, CLine: 411},

		// The four shop commands. All four are `do_not_here` in the C's
		// table (interpreter.c:246, :360, :454, :530) — what makes them work
		// is a mobile in the room with the shop_keeper special, which gets
		// first refusal and never hands them back.
		{Name: "buy", Help: "Buy something from a shopkeeper.", Run: doNotHere, CLine: 246},
		{Name: "list", Help: "List what a shopkeeper has for sale.", Run: doNotHere, CLine: 360},
		{Name: "sell", Help: "Sell something to a shopkeeper.", Run: doNotHere, CLine: 454},
		{Name: "value", Help: "Ask a shopkeeper what they would pay.", Run: doNotHere, CLine: 530},

		// `read` is do_look with SCMD_READ (interpreter.c:427) and `write` is
		// do_write (interpreter.c:557). Both are ordinary commands that a
		// bulletin board takes away from you while you are standing at one.
		{Name: "read", Help: "Read something.", Run: doRead, CLine: 427},
		{Name: "write", Help: "Write on a note, with a pen.", Run: doWrite, CLine: 557},

		// The bank and the inn. All `do_not_here` as well (interpreter.c:237,
		// :275, :550, :391, :438), and picked up by the `bank` and
		// `receptionist` specials.
		{Name: "balance", Help: "Ask a banker what you have deposited.", Run: doNotHere, CLine: 237},
		{Name: "deposit", Help: "Put money in the bank.", Run: doNotHere, CLine: 275},
		{Name: "withdraw", Help: "Take money out of the bank.", Run: doNotHere, CLine: 550},
		{Name: "offer", Help: "Ask an innkeeper what a stay would cost.", Run: doNotHere, CLine: 391},
		{Name: "rent", Help: "Store your belongings and leave the game.", Run: doNotHere, CLine: 438},

		// The post office (interpreter.c:251, :368, :436). `check` is only
		// ever a postmaster's; the other two likewise.
		{Name: "check", Help: "Ask a postmaster whether you have mail.", Run: doNotHere, CLine: 251},
		{Name: "mail", Help: "Send mail to another player.", Run: doNotHere, CLine: 368},
		{Name: "receive", Help: "Collect your mail from a postmaster.", Run: doNotHere, CLine: 436},

		// Housing (interpreter.c:330, :338). `hcontrol` is POS_DEAD and
		// LVL_GRGOD in the C's table; `house` is any mortal in their own.
		{Name: "hcontrol", Help: "Build, destroy and list houses.", Run: doHcontrol, CLine: 330},
		{Name: "house", Help: "Let somebody into your house, or list who is.", Run: doHouse, CLine: 338},

		// The last of the rules (interpreter.c:288, :358, :390, :493, :516).
		{Name: "enter", Help: "Go inside.", Run: doEnter, CLine: 288},
		{Name: "leave", Help: "Go back outside.", Run: doLeave, CLine: 358},
		{Name: "order", Help: "Tell a charmed follower what to do.", Run: doOrder, CLine: 390},
		{Name: "steal", Help: "Pick somebody's pocket.", Run: doSteal, CLine: 493},
		{Name: "track", Help: "Sense which way somebody went.", Run: doTrack, CLine: 516},

		// The wizard commands for getting about (interpreter.c:224, :313,
		// :347, :406, :407, :507, :518). Their levels are part of matching,
		// not a check afterwards — see lookupFor.
		{Name: "at", Help: "Do something somewhere else.", Run: doAt, CLine: 224, MinLevel: game.LevelImmortal},
		{Name: "goto", Help: "Go anywhere.", Run: doGoto, CLine: 313, MinLevel: game.LevelImmortal},
		{Name: "invis", Help: "Set your invisibility level.", Run: doInvis, CLine: 347, MinLevel: game.LevelImmortal},
		{Name: "poofin", Help: "Set what the room sees when you arrive.", Run: doPoofIn, CLine: 406, MinLevel: game.LevelImmortal},
		{Name: "poofout", Help: "Set what the room sees when you leave.", Run: doPoofOut, CLine: 407, MinLevel: game.LevelImmortal},
		{Name: "teleport", Help: "Send somebody somewhere.", Run: doTeleport, CLine: 507, MinLevel: game.LevelGod},
		// tedit's own field table gates each file at its own level
		// (LVL_IMPL/LVL_GRGOD); this is the command-table gate at the
		// lower of the two, matching the C's own two-stage check
		// (interpreter.c:508, tedit.c's own per-field level test).
		{Name: "tedit", Help: "Edit one of the server's canned text files.", Run: doTedit, CLine: 508, MinLevel: game.LevelGreaterGod},
		{Name: "transfer", Help: "Bring somebody to you.", Run: doTransfer, CLine: 518, MinLevel: game.LevelGod},

		// Looking at the innards (interpreter.c:494, :528, :529).
		{Name: "stat", Help: "Show everything about a room, object or character.", Run: doStat, CLine: 492, MinLevel: game.LevelImmortal},
		{Name: "vnum", Help: "List the vnums answering to a name.", Run: doVnum, CLine: 533, MinLevel: game.LevelImmortal},
		{Name: "vstat", Help: "Show everything about a prototype.", Run: doVstat, CLine: 534, MinLevel: game.LevelImmortal},

		// Changing things. LVL_FREEZE is LVL_GRGOD (structs.h:495), which is
		// why freeze and thaw sit a rank above the rest of do_wizutil.
		{Name: "advance", Help: "Set somebody's level.", Run: doAdvance, CLine: 225, MinLevel: game.LevelImplementor},
		{Name: "freeze", Help: "Stop somebody doing anything at all.", Run: doFreeze, CLine: 302, MinLevel: game.LevelGreaterGod},
		{Name: "load", Help: "Create an object or a mobile.", Run: doLoad, CLine: 363, MinLevel: game.LevelGod},
		{Name: "notitle", Help: "Stop somebody setting a title.", Run: doNoTitle, CLine: 385, MinLevel: game.LevelGod},
		{Name: "pardon", Help: "Clear somebody's killer and thief flags.", Run: doPardon, CLine: 399, MinLevel: game.LevelGod},
		{Name: "purge", Help: "Destroy something, or clean out the room.", Run: doPurge, CLine: 416, MinLevel: game.LevelGod},
		{Name: "reroll", Help: "Roll somebody's abilities again.", Run: doReroll, CLine: 440, MinLevel: game.LevelGreaterGod},

		// The local remort mechanic (interpreter.c:431, :432). `redeem` is a
		// branch of do_wizutil like the seven above it; `remort` is a command
		// of its own and needs an implementor.
		{Name: "redeem", Help: "Restore a fallen paladin.", Run: doRedeem, CLine: 431, MinLevel: game.LevelGreaterGod},
		{Name: "remort", Help: "Give somebody another class's skills.", Run: doRemort, CLine: 432, MinLevel: game.LevelImplementor},
		{Name: "restore", Help: "Heal somebody completely.", Run: doRestore, CLine: 442, MinLevel: game.LevelGod},
		// `mute`, not `squelch` — the subcommand is SCMD_SQUELCH but the word
		// players and gods type is `mute` (interpreter.c:371). Caught by the
		// command-line test, which is what it is for.
		{Name: "mute", Help: "Stop somebody shouting.", Run: doSquelch, CLine: 371, MinLevel: game.LevelGod},
		{Name: "thaw", Help: "Unfreeze somebody.", Run: doThaw, CLine: 511, MinLevel: game.LevelGreaterGod},
		{Name: "unaffect", Help: "Strip every spell from somebody.", Run: doUnaffect, CLine: 525, MinLevel: game.LevelGod},
		{Name: "zreset", Help: "Reset a zone, or the whole world.", Run: doZreset, CLine: 563, MinLevel: game.LevelGreaterGod},

		// Talking as a god, and making other people act (interpreter.c:284,
		// :285, :294, :309, :455, :499, :551, :552). `emote` is the odd one
		// out: same function, level 1, and a mortal command.
		{Name: "echo", Help: "Say something with nobody's name on it.", Run: doEcho, CLine: 284, MinLevel: game.LevelImmortal},
		{Name: "emote", Help: "Act something out.", Run: doEmote, CLine: 285},
		// `:` is emote with no space, the way `'` is say (interpreter.c:286).
		{Name: ":", Help: "Act something out.", Run: doEmote, CLine: 286},
		{Name: "force", Help: "Make somebody do something.", Run: doForce, CLine: 294, MinLevel: game.LevelGod},
		{Name: "gecho", Help: "Say something to everybody in the game.", Run: doGecho, CLine: 309, MinLevel: game.LevelGod},
		{Name: "send", Help: "Send a line to one person.", Run: doSend, CLine: 455, MinLevel: game.LevelGod},
		{Name: "syslog", Help: "Set how much of the log you see.", Run: doSyslog, CLine: 499, MinLevel: game.LevelImmortal},
		{Name: "wiznet", Help: "Talk on the gods' channel.", Run: doWiznet, CLine: 551, MinLevel: game.LevelImmortal},
		// `;` is wiznet with no space, the way `'` is say. The one-character
		// command path in split() is what makes it work.
		{Name: ";", Help: "Talk on the gods' channel.", Run: doWiznet, CLine: 552, MinLevel: game.LevelImmortal},

		// Running the place (interpreter.c:272, :274, :357, :443, :464,
		// :483, :498, :526, :555). `return` is level 0 — a mortal switched
		// into by a god needs a way of saying so.
		{Name: "date", Help: "Show the machine's clock.", Run: doDate, CLine: 272, MinLevel: game.LevelImmortal},
		{Name: "dc", Help: "Close somebody's connection.", Run: doDisconnect, CLine: 274, MinLevel: game.LevelGod},
		{Name: "last", Help: "Show when somebody was last on.", Run: doLast, CLine: 357, MinLevel: game.LevelGod},
		{Name: "return", Help: "Go back to your own body.", Run: doReturn, CLine: 443},
		{Name: "shutdow", Help: "Not enough. Type `shutdown`.", Run: shutdownCommand(false), CLine: 463, MinLevel: game.LevelGreaterGod},
		{Name: "shutdown", Help: "Stop the server.", Run: shutdownCommand(true), CLine: 464, MinLevel: game.LevelGreaterGod},
		{Name: "snoop", Help: "Watch somebody's screen.", Run: doSnoop, CLine: 483, MinLevel: game.LevelGod},
		{Name: "switch", Help: "Take over somebody else's body.", Run: doSwitch, CLine: 498, MinLevel: game.LevelGod},
		{Name: "uptime", Help: "Show how long the server has been up.", Run: doUptime, CLine: 526, MinLevel: game.LevelImmortal},
		{Name: "wizlock", Help: "Set who may log in.", Run: doWizlock, CLine: 555, MinLevel: game.LevelGreaterGod},

		// Bans and the server's view of itself (interpreter.c:236, :461,
		// :524).
		{Name: "ban", Help: "Ban a site, or list the bans.", Run: doBan, CLine: 236, MinLevel: game.LevelGreaterGod},
		{Name: "show", Help: "Show what the server knows about itself.", Run: doShow, CLine: 461, MinLevel: game.LevelImmortal},
		{Name: "set", Help: "Set a field on a character.", Run: doSet, CLine: 456, MinLevel: game.LevelGod},
		{Name: "unban", Help: "Lift a site ban.", Run: doUnban, CLine: 524, MinLevel: game.LevelGreaterGod},
		{Name: "say", Help: "Talk to the room.", Run: doSay, CLine: 449},
		{Name: "'", Help: "Talk to the room; the short form of say.", Run: doSay, CLine: 450},
		{Name: "score", Help: "Show your own statistics.", Run: doScore, CLine: 452},
		{Name: "shout", Help: "Shout to everybody in your zone.", Run: doShout, CLine: 458},
		{Name: "exits", Help: "List the ways out.", Run: doExits, CLine: 290},

		{Name: "open", Help: "Open a door.", Run: doOpen, CLine: 392},
		{Name: "close", Help: "Close a door.", Run: doClose, CLine: 255},
		{Name: "lock", Help: "Lock a door.", Run: doLock, CLine: 362},
		{Name: "unlock", Help: "Unlock a door.", Run: doUnlock, CLine: 522},
		{Name: "pick", Help: "Pick a lock.", Run: doPick, CLine: 401},

		{Name: "get", Help: "Pick something up.", Run: doGet, CLine: 307},
		// After get, as in the C (interpreter.c:307 and :310), so `g` is get
		// and giving needs `gi`.
		{Name: "give", Help: "Give something to somebody.", Run: doGive, CLine: 310},
		// drink before drop, which is the C's order (interpreter.c:279 and
		// :280) — so `dr` is drink and only `dro` is drop.
		{Name: "drink", Help: "Drink from something.", Run: doDrink, CLine: 279},
		{Name: "drop", Help: "Put something down.", Run: doDrop, CLine: 280},
		{Name: "eat", Help: "Eat something.", Run: doEat, CLine: 283},
		{Name: "inventory", Help: "List what you are carrying.", Run: doInventory, CLine: 341},
		{Name: "equipment", Help: "List what you are wearing.", Run: doEquipment, CLine: 289},
		{Name: "wear", Help: "Put something on.", Run: doWear, CLine: 538},
		{Name: "wield", Help: "Take a weapon in hand.", Run: doWield, CLine: 546},
		{Name: "remove", Help: "Take something off.", Run: doRemove, CLine: 437},
		// gossip (:315) comes before group (:316), which comes before grab
		// (:317) — so `go` is gossip, `gr` is group, and grabbing needs
		// `gra`.
		{Name: "gossip", Help: "Chat to everybody playing.", Run: doGossip, CLine: 315},
		{Name: "group", Help: "List your group, or enrol somebody in it.", Run: doGroup, CLine: 316},
		{Name: "grab", Help: "Take something in your hands.", Run: doGrab, CLine: 317},
		{Name: "grats", Help: "Congratulate somebody, game-wide.", Run: doGrats, CLine: 318},
		// gsay and gtell are the same command (:325 and :326).
		{Name: "gsay", Help: "Talk to your group.", Run: doGroupSay, CLine: 325},
		{Name: "gtell", Help: "Talk to your group.", Run: doGroupSay, CLine: 326},

		// After `exits`, `close` and `wear`, which is the C's order — so `ex`
		// is exits, `co` is close and `wea` is wear, and only the longer
		// forms reach these.
		{Name: "examine", Help: "Look closely at something.", Run: doExamine, CLine: 291},
		{Name: "consider", Help: "Size somebody up.", Run: doConsider, CLine: 257},
		// tell is the first of the t's in the C (interpreter.c:501, ahead of
		// take, taste and time), so `t` is tell.
		{Name: "tell", Help: "Tell one person something, wherever they are.", Run: doTell, CLine: 501},
		{Name: "time", Help: "Ask what time it is.", Run: doTime, CLine: 514},
		{Name: "weather", Help: "Ask what the weather is doing.", Run: doWeather, CLine: 539},
		{Name: "pour", Help: "Empty a container.", Run: doPour, CLine: 408},
		{Name: "taste", Help: "Take a small bite.", Run: doTaste, CLine: 506},
		{Name: "whisper", Help: "Whisper to somebody in the room.", Run: doWhisper, CLine: 543},
		{Name: "who", Help: "List who is playing.", Run: doWho, CLine: 540},
		{Name: "credits", Help: "Show the CircleMUD and DikuMUD credits.", Run: doCredits, CLine: 264},
		{Name: "help", Help: "Show this list, or help on a topic.", Run: doHelp, CLine: 328},
		// `hide`, `hit` and `hold` sit after help because the C has them
		// there (interpreter.c:328, :332, :333 and :334) — so `h` is help,
		// `hi` is hide, and hitting somebody needs `hit`.
		{Name: "hide", Help: "Try to hide yourself.", Run: doHide, CLine: 332},
		{Name: "hit", Help: "Attack somebody, starting now.", Run: doHit, CLine: 333},
		{Name: "hold", Help: "Take something in your hands.", Run: doGrab, CLine: 334},
		{Name: "holler", Help: "Shout across the whole game, at a cost.", Run: doHoller, CLine: 335},
		{Name: "quest", Help: "Join or leave the quest channel.", Run: toggleCommand("quest"), CLine: 420},
		// `qui` and `quit` are the same function with different subcommands
		// (interpreter.c:421, :422): the short spelling refuses, so that an
		// abbreviation of a dangerous command cannot act by accident. `q`
		// therefore reaches `qui` and tells you to type it out.
		{Name: "qui", Help: "Not enough. Type `quit`.", Run: quitCommand(false), CLine: 421},
		{Name: "quit", Help: "Leave the game.", Run: quitCommand(true), CLine: 422},
		{Name: "qsay", Help: "Talk to everybody on the quest.", Run: doQuestSay, CLine: 423},
		// Nowhere near the others: the C has it among the u's, after unlock
		// (interpreter.c:522 and :523), so `un` is unlock and ungrouping
		// needs `ung`.
		{Name: "ungroup", Help: "Disband your group, or expel somebody.", Run: doUngroup, CLine: 523},

		{Name: "autoexit", Help: "Show the exits after every move.", Run: toggleCommand("autoexit"), CLine: 232},
		{Name: "brief", Help: "Skip room descriptions you have seen.", Run: toggleCommand("brief"), CLine: 244},

		// The immortal half of do_gen_tog (interpreter.c:336, :380, :386,
		// :446). All four are LVL_IMMORT, and the level is part of matching —
		// so `ho` is `hold` for a mortal and still `hold` for a god, because
		// hold is earlier in the table.
		{Name: "holylight", Help: "See everything, everywhere.", Run: toggleCommand("holylight"), CLine: 336, MinLevel: game.LevelImmortal},
		{Name: "nohassle", Help: "Stop mobiles attacking you.", Run: toggleCommand("nohassle"), CLine: 380, MinLevel: game.LevelImmortal},
		{Name: "nowiz", Help: "Stop hearing the gods' channel.", Run: toggleCommand("nowiz"), CLine: 386, MinLevel: game.LevelImmortal},
		{Name: "roomflags", Help: "Show each room's vnum and flags.", Run: toggleCommand("roomflags"), CLine: 446, MinLevel: game.LevelImmortal},
		{Name: "clear", Help: "Clear the screen.", Run: doClearScreen, CLine: 254},
		{Name: "cls", Help: "Clear the screen.", Run: doClearScreen, CLine: 256},
		{Name: "color", Help: "Choose how much colour you see.", Run: doColour, CLine: 258},
		{Name: "commands", Help: "List the commands you can use.", Run: doCommands(listCommands), CLine: 261},
		{Name: "compact", Help: "Drop the blank line before each prompt.", Run: toggleCommand("compact"), CLine: 262},
		{Name: "diagnose", Help: "See how hurt somebody is.", Run: doDiagnose, CLine: 276},
		{Name: "display", Help: "Choose what the prompt shows.", Run: doDisplay, CLine: 277},
		{Name: "gold", Help: "Count your money.", Run: doGold, CLine: 314},
		{Name: "handbook", Help: "The immortals' handbook.", Run: cannedText("handbook", TextFiles.Handbook), CLine: 329},
		{Name: "imotd", Help: "The immortals' message of the day.", Run: cannedText("immortal message of the day", TextFiles.ImmortalMOTD), CLine: 343},
		{Name: "immlist", Help: "List the immortals.", Run: cannedText("immortal list", TextFiles.ImmList), CLine: 344},
		{Name: "info", Help: "General information about the game.", Run: cannedText("information", TextFiles.Info), CLine: 345},
		{Name: "levels", Help: "Show the experience table for your class.", Run: doLevels, CLine: 359},
		{Name: "motd", Help: "The message of the day.", Run: cannedText("message of the day", TextFiles.MOTD), CLine: 367},
		{Name: "news", Help: "Recent news.", Run: cannedText("news", TextFiles.News), CLine: 374},
		{Name: "policy", Help: "The rules.", Run: cannedText("policy", TextFiles.Policies), CLine: 404},
		{Name: "prompt", Help: "Choose what the prompt shows.", Run: doDisplay, CLine: 410},
		{Name: "report", Help: "Tell your group how you are doing.", Run: doReport, CLine: 439},
		{Name: "save", Help: "Save your character.", Run: doSave, CLine: 451},
		{Name: "socials", Help: "List the socials you can use.", Run: doCommands(listSocials), CLine: 485},
		// `take` is `get` under another name (interpreter.c:503), which is
		// why `ta` is take and not taste — take is two lines earlier.
		{Name: "take", Help: "Pick something up.", Run: doGet, CLine: 503},
		{Name: "insult", Help: "Be rude to somebody.", Run: doInsult, CLine: 346},
		{Name: "page", Help: "Send a line straight to somebody.", Run: doPage, CLine: 398, MinLevel: game.LevelGod},
		{Name: "qecho", Help: "Say something unattributed on the quest channel.", Run: doQuestEcho, CLine: 419, MinLevel: game.LevelImmortal},
		{Name: "reload", Help: "Re-read one of the canned text files.", Run: doReload, CLine: 428, MinLevel: game.LevelImplementor},
		// reloadmob is not in the C at all — new capability, not a port
		// (see docs/deviations.md, coverage_test.go's newCommands). CLine
		// is synthetic, one past the real "reload" (428), so a bare
		// "reload" keeps matching the real, ported command first and
		// only a longer typed prefix ("reloadm...") reaches this one.
		{Name: "reloadmob", Help: "Re-read a mobile's definition from disk.", Run: doReloadMob, CLine: 429, MinLevel: game.LevelGreaterGod},
		{Name: "skillset", Help: "Set somebody's skill to a number.", Run: doSkillset, CLine: 469, MinLevel: game.LevelGreaterGod},
		{Name: "trackthru", Help: "Switch tracking through closed doors.", Run: doTrackThrough, CLine: 517, MinLevel: game.LevelImplementor},
		{Name: "users", Help: "List every connection, not just the players.", Run: doUsers, CLine: 528, MinLevel: game.LevelImmortal},
		{Name: "wizhelp", Help: "List the immortal commands.", Run: doCommands(listWizhelp), CLine: 553, MinLevel: game.LevelImmortal},
		{Name: "split", Help: "Share gold with your group.", Run: doSplit, CLine: 486},
		{Name: "title", Help: "Set the title that follows your name.", Run: doTitle, CLine: 512},
		{Name: "toggle", Help: "Show every one of your settings.", Run: doToggle, CLine: 515},
		{Name: "version", Help: "Which server this is.", Run: doVersion, CLine: 531},
		{Name: "visible", Help: "Stop being invisible.", Run: doVisible, CLine: 532},
		{Name: "whoami", Help: "Say who you are.", Run: doWhoAmI, CLine: 541},
		{Name: "where", Help: "Who is in your zone, and where.", Run: doWhere, CLine: 542},
		{Name: "wimpy", Help: "Flee automatically below a hit-point level.", Run: doWimpy, CLine: 548},
		{Name: "wizlist", Help: "List the gods.", Run: cannedText("wizlist", TextFiles.WizList), CLine: 554},
		{Name: "donate", Help: "Send something to the donation room.", Run: doDonate, CLine: 278},
		{Name: "junk", Help: "Destroy something for a small reward.", Run: doJunk, CLine: 349},
		{Name: "quaff", Help: "Drink a potion.", Run: doQuaff, CLine: 418},
		{Name: "recite", Help: "Read a scroll aloud.", Run: doRecite, CLine: 435},
		{Name: "use", Help: "Use a wand or a staff you are holding.", Run: doUse, CLine: 527},

		{Name: "bug", Help: "Report a bug.", Run: doGenWrite("bug"), CLine: 247},
		{Name: "idea", Help: "Suggest an idea.", Run: doGenWrite("idea"), CLine: 342},
		{Name: "typo", Help: "Report a typo.", Run: doGenWrite("typo"), CLine: 520},
	}
	Commands = commandTable(staticCommands)
}

// RegisterSocials rebuilds the command table with the socials in it, in the
// positions `interpreter.c` gives them.
//
// A social whose name is not in the C's table is dropped with a note, which is
// what the C does too — it logs "Unknown social '%s' in social file" and
// re-uses the slot. Called once at boot, before anything can type anything.
// SocialNamed returns a registered social by name, or nil.
//
// Only the receptionist needs this: it fidgets by performing one, and the C
// reaches it through find_command() rather than through the social table
// directly.
func SocialNamed(name string) *game.Social {
	for i := range Commands {
		if Commands[i].Name == name {
			return Commands[i].Social
		}
	}
	return nil
}

func RegisterSocials(socials []game.Social) (added int, unknown []string) {
	out := append([]Command(nil), staticCommands...)

	have := make(map[string]bool, len(socials))
	for i := range socials {
		s := socials[i]
		line, ok := socialLines[s.Name]
		if !ok {
			unknown = append(unknown, s.Name)
			continue
		}
		have[s.Name] = true
		out = append(out, Command{
			Name:   s.Name,
			Help:   "(social)",
			Run:    doAction,
			CLine:  line,
			Social: &socials[i],
		})
		added++
	}

	// A social in the C's table with no entry in the file is still a command
	// there — `hop` is one — and it answers "That action is not supported."
	// Leaving it out would answer "Huh?!?", which is a different thing: one
	// says the game knows the word, the other says it does not.
	for name, line := range socialLines {
		if have[name] {
			continue
		}
		out = append(out, Command{
			Name:  name,
			Help:  "(social, with nothing in the socials file)",
			Run:   doAction,
			CLine: line,
		})
	}

	// The table is a package global and is written exactly once. Only tests
	// build more than one server in a process, and a second one writing this
	// while the first still has a goroutine reading it is a data race for no
	// gain — the socials file is the same file either way.
	socialsOnce.Do(func() { Commands = commandTable(out) })
	return added, unknown
}

var socialsOnce sync.Once

// commandTable builds the finished table: each command's minimum position
// filled in from the C's, then the whole lot in the C's order.
//
// Every path that produces a table goes through here — the static commands at
// init, and the same list with the socials interleaved once they are loaded —
// so neither step can be forgotten on one of them.
//
// The sort is stable, so two entries that somehow share a line keep the order
// they were written in.
func commandTable(in []Command) []Command {
	out := append([]Command(nil), in...)
	for i := range out {
		// A command not in the C's table is a bug the tests catch by name.
		// Leaving MinPosition at PosDead here is the lenient answer, which is
		// the right way round to be wrong: it refuses nobody.
		out[i].MinPosition = commandPositions[out[i].Name]
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CLine < out[j].CLine })
	return out
}

// Dispatcher runs commands against the world.
type Dispatcher struct {
	// Run submits a function to the world goroutine and waits for it. The
	// engine provides this; taking it as a function keeps this package from
	// importing the engine and the engine from importing this one.
	Run func(ctx context.Context, f func(*game.Live)) error
	// Text supplies the canned files.
	Text TextFiles
	// RNG is the game's generator.
	RNG *rng.Rand
	// Violence resolves damage on behalf of the commands that cause it.
	Violence Violence
	// NoSpecials suppresses special procedures, which is the C's `-s`.
	NoSpecials bool
	// Save writes a character's record back, off the world goroutine.
	Save func(*game.Character)
	// Rent stores a character's belongings, for the receptionist.
	Rent RentSaver
	// SaveBoard writes a bulletin board back to disk.
	SaveBoard BoardSaver
	// Mail is the mud mail system, for the postmaster.
	Mail MailSystem
	// Houses is the player housing system.
	Houses HouseKeeper
	// Operator is the connections and the shutdown switch.
	Operator Operator
	// Bans is the site ban list.
	Bans BanKeeper
	// Reports is the bug/idea/typo report log.
	Reports ReportWriter
	// SetPassword replaces somebody's credential.
	SetPassword func(c *game.Character, password string) error
	// TextEdit reads and writes a canned text file by name, for `tedit`.
	TextEdit TextEditor
	// MobReload hot-reloads a mobile prototype from disk, for `reloadmob`.
	MobReload MobReloader
}

// Do implements CommandHandler.
func (d *Dispatcher) Do(ctx context.Context, s *Session, line string) error {
	word, arg := split(line)
	if word == "" {
		s.Send("%s", prompt(s))
		return nil
	}

	// The level is part of the match. A command above your level is not
	// refused, it is invisible: the word carries on matching down the table
	// and, if nothing else answers to it, you get "Huh?!?" — the same answer
	// as a word that means nothing at all. A mortal typing `goto` has never
	// been told there is such a command.
	cmd := lookupFor(word, characterLevel(s))
	if cmd == nil {
		s.Send("Huh?!?\r\n%s", prompt(s))
		return nil
	}

	// A character with a wait state does not act yet. The C stops reading
	// that descriptor's input until the wait runs down, so the command is
	// *delayed* rather than refused — a player who types `kick` twice gets
	// two kicks, slowly. Sleeping here has the same effect, since this
	// goroutine is the one reading their input.
	if c := s.Character(); c != nil {
		if remaining := c.WaitRemaining(); remaining > 0 {
			select {
			case <-time.After(remaining):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}

	// The command itself runs on the world goroutine; everything it touches
	// is world state.
	return d.Run(ctx, func(w *game.Live) {
		// Typing anything at all stops you hiding, which is the first line of
		// command_interpreter and catches people out constantly.
		s.Character().SetHidden(false)

		// The three refusals command_interpreter makes between finding a
		// command and running it (interpreter.c:629-661). They come *before*
		// the special procedure gets first refusal, which is load-bearing: a
		// shopkeeper does not get to sell to somebody who is asleep.
		if refusal := refuse(s, cmd); refusal != "" {
			s.Send("%s", refusal)
			if !s.Closed() {
				s.SendVitals(s.Character())
				s.Send("%s", prompt(s))
			}
			return
		}

		c := &Context{
			Ctx: ctx, Session: s, Character: s.Character(),
			World: w, Text: d.Text, RNG: d.RNG, Violence: d.Violence, Arg: arg,
			Social: cmd.Social, Save: d.Save, Rent: d.Rent, SaveBoard: d.SaveBoard, Mail: d.Mail, Houses: d.Houses, Operator: d.Operator, Bans: d.Bans, Reports: d.Reports, SetPassword: d.SetPassword, TextEdit: d.TextEdit, MobReload: d.MobReload,
		}

		// A command that panics must not leave the player staring at a dead
		// terminal. The engine contains the panic and logs it, but it does so
		// by abandoning the rest of this function — including the prompt — so
		// the player sees nothing at all and cannot tell the difference
		// between a broken command and a hung server. Recovering here means
		// they get told, and get their prompt back.
		func() {
			defer func() {
				if r := recover(); r != nil {
					s.logger.Error("command panicked",
						"command", cmd.Name, "panic", r, "stack", string(debug.Stack()))
					s.Send("Something went wrong doing that.\r\n")
				}
			}()
			// A special procedure in reach gets first refusal, as
			// command_interpreter gives one before running the command
			// itself. One that handles the command stops it running.
			if d.NoSpecials || !c.runSpecials(cmd.Name, arg) {
				if err := cmd.Run(c); err != nil {
					s.logger.Error("command failed", "command", cmd.Name, "error", err)
					s.Send("Something went wrong doing that.\r\n")
				}
			}
		}()
		if !s.Closed() {
			// The same numbers as the prompt, out of band, so a client does
			// not have to parse them back out of it.
			s.SendVitals(s.Character())
			s.Send("%s", prompt(s))
		}
	})
}

// refuse is the `else if` ladder in command_interpreter between finding a
// command and running it (interpreter.c:629-661). It returns what to say, or
// "" to go ahead.
//
// The order is the C's and is not arbitrary. Frozen comes first, so somebody a
// god has put on ice is told that and nothing else, whatever they typed and
// whatever position they are in. The position check comes last, and in
// particular after the switched-immortal check, so a god switched into a
// sleeping rat hears about the switch rather than about the sleeping.
//
// One branch of the C's ladder is missing and costs nothing: `command_pointer
// == NULL`, the "Sorry, that command hasn't been implemented yet.". Exactly one
// row of the C's table has a null pointer — `RESERVED`, the placeholder that
// lets a specproc return command 0 — and it cannot be matched, because
// any_one_arg lowercases the typed word (interpreter.c:1028) and the name is
// upper case. So that message is unreachable in the C as well, and a command
// this port has not got answers "Huh?!?" from the lookup, which is what the C
// does for a word that is in no row at all.
func refuse(s *Session, cmd *Command) string {
	ch := s.Character()
	if ch == nil {
		return ""
	}

	// Frozen. Only a mortal — an implementor can thaw themselves out, which is
	// the point of the level test rather than an oversight.
	if !ch.IsNPC() && ch.Record != nil &&
		ch.Record.PlayerFlags.Has(game.PlayerFrozen) && ch.Record.Level < game.LevelImplementor {
		return "You try, but the mind-numbing cold prevents you...\r\n"
	}

	// A god switched into a mobile is a mobile, and the interpreter tells them
	// so rather than letting them keep their own commands. Reachable only for
	// a mob whose own level is high enough to have matched the command in the
	// first place — the level is part of the match, so a rat never gets here.
	if ch.IsNPC() && cmd.MinLevel >= game.LevelImmortal {
		return "You can't use immortal commands while switched.\r\n"
	}

	if ch.Position >= cmd.MinPosition {
		return ""
	}
	// Chosen by the position they are *in*, not the one the command wanted, so
	// the refusal describes them rather than the command.
	switch ch.Position {
	case game.PosDead:
		return "Lie still; you are DEAD!!! :-(\r\n"
	case game.PosIncapacitated, game.PosMortallyWounded:
		return "You are in a pretty bad shape, unable to do anything!\r\n"
	case game.PosStunned:
		return "All you can do right now is think about the stars!\r\n"
	case game.PosSleeping:
		return "In your dreams, or what?\r\n"
	case game.PosResting:
		return "Nah... You feel too relaxed to do that..\r\n"
	case game.PosSitting:
		return "Maybe you should get on your feet first?\r\n"
	case game.PosFighting:
		return "No way!  You're fighting for your life!\r\n"
	}
	// POS_STANDING, which cannot be below any minimum: the C's switch has no
	// default and falls through to running nothing at all, having printed
	// nothing either. Unreachable, and harmless if it ever is not.
	return ""
}

// Lookup finds the first command the word is a prefix of, which is what the C
// interpreter does — so the order of the table is player-visible muscle
// memory and worth asserting from outside this package.
func Lookup(word string) *Command { return lookup(word) }

// LookupFor is Lookup for somebody of a given level, exported so tests can
// assert what an abbreviation means on each side of the divide.
func LookupFor(word string, level int32) *Command { return lookupFor(word, level) }

// lookup finds the command a typed word means, for somebody of this level.
//
// The level is part of the *match*, not a check afterwards. The C's loop is
//
//	if (!strncmp(cmd_info[cmd].command, arg, length))
//	  if (GET_LEVEL(ch) >= cmd_info[cmd].minimum_level)
//	    break;
//
// — so a command above your level is skipped and matching carries on down the
// table (interpreter.c:623). The consequence is that **abbreviations mean
// different things to mortals and to gods**: an immortal command sitting
// earlier in the table shadows a mortal one for the people who can use it,
// and does not exist at all for everybody else. That is not a detail to
// paper over; it is twenty years of muscle memory on both sides.
func lookupFor(word string, level int32) *Command {
	word = strings.ToLower(word)
	for i := range Commands {
		if strings.HasPrefix(Commands[i].Name, word) && level >= Commands[i].MinLevel {
			return &Commands[i]
		}
	}
	return nil
}

// lookup is lookupFor at implementor level: every command is visible. Used by
// the help listing and by tests that ask what a word means in principle.
func lookup(word string) *Command { return lookupFor(word, game.LevelImplementor) }

// characterLevel is the level to match commands at. A session with no
// character yet is a mortal of level zero.
func characterLevel(s *Session) int32 {
	if s == nil {
		return 0
	}
	if c := s.Character(); c != nil && c.Record != nil {
		return c.Record.Level
	}
	return 0
}

// split separates the command word from its argument.
//
// A line starting with a non-letter is a one-character command with
// everything after it as the argument, which is what makes `'hi` and
// `;godnet test` work with no space. The C's comment credits Eric Green and
// Stefan Wasilewski with the patch and says it was "requested by many
// people".
func split(line string) (word, arg string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return "", ""
	}
	if r := rune(line[0]); !unicode.IsLetter(r) {
		return line[:1], strings.TrimSpace(line[1:])
	}
	word, arg, _ = strings.Cut(line, " ")
	return word, strings.TrimSpace(arg)
}

// prompt is what a player sees when the server is waiting for them, ported
// from make_prompt (comm.c:1043).
//
// Each part is shown only if the matching preference is set. A new character
// gets all three, because the local block at the class prompt turns them on;
// a converted one gets whatever they had turned on when they last played,
// which is the point of keeping the preference bits faithful.
func prompt(s *Session) string {
	c := s.Character()
	if c == nil || c.Record == nil {
		return "> "
	}
	rec := c.Record

	// A blank line before the prompt unless the player has asked for compact
	// mode (comm.c:1436). Another preference that was settable, listed by
	// `toggle` and saved, and read by nothing — so `compact` reported success
	// and did nothing, the same as `brief` and `autoexit` did before the light
	// work. Found by the session-parity harness, which counted the blank line.
	lead := "\r\n"
	if rec.Preferences.Has(game.PrefCompact) {
		lead = ""
	}

	var b strings.Builder
	b.WriteString(lead)
	if rec.InvisLevel > 0 {
		fmt.Fprintf(&b, "i%d ", rec.InvisLevel)
	}
	if rec.Preferences.Has(game.PrefDisplayHP) {
		fmt.Fprintf(&b, "%dH ", rec.Points.Hit)
	}
	if rec.Preferences.Has(game.PrefDisplayMana) {
		fmt.Fprintf(&b, "%dM ", rec.Points.Mana)
	}
	if rec.Preferences.Has(game.PrefDisplayMove) {
		fmt.Fprintf(&b, "%dV ", rec.Points.Move)
	}
	b.WriteString("> ")
	return b.String()
}

func doLook(c *Context) error {
	// do_look has a gate of its own (act.informative.c:662), and it is *not*
	// look_at_room's: different words, and the two tests the other way round.
	// Typing `look` while blind says "you're blind"; walking into a dark room
	// while blind says it is pitch black, because there the darkness is asked
	// first. Both are reachable and they disagree on purpose — or at least
	// they disagree, and there is no sign anybody meant them to.
	//
	// The C's first branch, `GET_POS(ch) < POS_SLEEPING` → "You can't see
	// anything but stars!", is **unreachable**: `look` and `read` are both
	// POS_RESTING in the command table (interpreter.c:355, :427), so the
	// interpreter has already refused anything below that. Not ported, and
	// recorded in docs/weirdnumbers.md with the other four of its kind.
	if isBlind(c.Character) {
		c.Send("You can't see a damned thing, you're blind!\r\n")
		return nil
	}
	if c.World.RoomIsDark(c.Character.Room) && !game.CanSeeInDark(c.Character) {
		c.Send("It is pitch black...\r\n")
		// And the one thing you *can* see in the dark. The C's comment on this
		// line is just "glowing red eyes", which is the only clue that
		// list_char_to_char has a second branch at all.
		if room := c.World.Room(c.Character.Room); room != nil {
			c.Send("%s", listCharToChar(c.World, room, c.Character))
		}
		return nil
	}

	// `look <something>` describes one thing; bare `look` describes the room.
	if arg := strings.TrimSpace(c.Arg); arg != "" {
		// `look in <container>` and `look at <thing>` are both the C's, and
		// both mean "describe that".
		arg = strings.TrimPrefix(arg, "at ")
		arg = strings.TrimPrefix(arg, "in ")
		return c.lookAtTarget(arg)
	}

	// `look` typed on purpose ignores brief mode, which is what the C's
	// ignore_brief argument is for. Everywhere else in the whole tree passes
	// 0: `do_look` at act.informative.c:680 is the only caller that passes 1.
	return lookAtRoom(c, true)
}

// lookAtRoom shows the character the room they are in.
func lookAtRoom(c *Context, ignoreBrief bool) error {
	room := c.World.Room(c.Character.Room)
	if room == nil {
		c.Send("You are nowhere at all. That should not be possible.\r\n")
		return nil
	}

	sendRoomInfo(c.Session, room)
	c.Send("%s", roomDescription(c.World, room, c.Character, ignoreBrief))
	return nil
}

// roomDescription is look_at_room (act.informative.c:413): the name, the
// description, the way out, what is lying about and who is here.
//
// It takes the viewer so it can leave them out of the list of people, consult
// their preferences and work out whether they can see at all, and it returns a
// string rather than sending, because a spell can move somebody else into a
// room and has to show it to them rather than to the caster.
//
// ignoreBrief is the C's argument of the same name: `look` typed by hand shows
// the description whatever PRF_BRIEF says, and the automatic look on arriving
// somewhere does not.
func roomDescription(w *game.Live, room *game.RoomDef, viewer *game.Character, ignoreBrief bool) string {
	// Darkness first, and blindness after it — the C's order, which decides
	// which message a blind character standing in the dark gets. They are told
	// it is pitch black, not that they are blind.
	//
	// Note this asks CAN_SEE_IN_DARK, so holylight counts directly here. It is
	// a different question from LIGHT_OK's, which takes infravision alone.
	if w.RoomIsDark(room.Vnum) && !game.CanSeeInDark(viewer) {
		return "It is pitch black...\r\n"
	}
	if isBlind(viewer) {
		return "You see nothing but infinite darkness...\r\n"
	}

	var b strings.Builder

	// An immortal with `roomflags` on gets the vnum and the flags in the
	// title line.
	// Cyan, as look_at_room does (act.informative.c:425). The colour markup
	// is resolved at the socket against the reader's preference — see
	// internal/colour — so this string is written once for everybody.
	if hasPref(viewer, game.PrefRoomFlags) {
		fmt.Fprintf(&b, "{{cyan}}[%5d] %s [ %s]{{/}}\r\n",
			room.Vnum, room.Name, game.SprintBit(room.Flags, game.RoomBitNames()))
	} else {
		fmt.Fprintf(&b, "{{cyan}}%s{{/}}\r\n", room.Name)
	}

	// Brief mode drops the description — but never in a DEATH room, because
	// that description is the only warning you get.
	if room.Description != "" &&
		(ignoreBrief || !hasPref(viewer, game.PrefBrief) || room.Flags.Has(game.RoomDeathTrap)) {
		b.WriteString(ensureNewline(room.Description))
	}

	if hasPref(viewer, game.PrefAutoExit) {
		fmt.Fprintf(&b, "{{cyan}}[ Exits: %s]{{/}}\r\n", autoExits(room))
	}

	// The two local additions, both `<DoC>` (act.informative.c:444, :452).
	if room.Flags.Has(game.RoomGoodRegen) {
		b.WriteString("You feel a soft, warm feeling in your bones.\r\n")
	}
	if room.Flags.Has(game.RoomPKill) {
		b.WriteString("You have entered a [Player Killer] room. Beware!\r\n")
	}

	// Green for what is lying about and yellow for who is here, which is the
	// C switching colour around each list rather than colouring the lines
	// themselves (act.informative.c:469-473). The reset goes after the whole
	// list, not after each line.
	// list_obj_to_char (act.informative.c:165). An object you cannot see is
	// simply not there, with no marker: the C's `show` argument produces
	// " Nothing." for an empty *inventory*, never for an empty floor.
	var objects strings.Builder
	for _, obj := range w.RoomObjects(room.Vnum) {
		if !w.CanSeeObj(viewer, obj) {
			continue
		}
		if obj.Description != "" {
			fmt.Fprintf(&objects, "%s\r\n", obj.Description)
			continue
		}
		fmt.Fprintf(&objects, "%s is lying here.\r\n", capitaliseFirst(obj.Name()))
	}
	// Unconditionally, and with one reset for both lists rather than one each.
	// The C sends the colour codes as bare writes around the two calls
	// (act.informative.c:469-473):
	//
	//	send_to_char(CCGRN(ch, C_NRM), ch);
	//	list_obj_to_char(...);
	//	send_to_char(CCYEL(ch, C_NRM), ch);
	//	list_char_to_char(...);
	//	send_to_char(CCNRM(ch, C_NRM), ch);
	//
	// So an empty room still gets a green, a yellow and a reset with nothing
	// between them — visible in a transcript, invisible on a terminal, and
	// reproduced because the session-parity harness compares transcripts.
	fmt.Fprintf(&b, "{{green}}%s{{yellow}}%s{{/}}",
		objects.String(), listCharToChar(w, room, viewer))
	return b.String()
}

// listCharToChar is list_char_to_char (act.informative.c:343).
//
// The `else if` is the interesting half and is easy to skip past: somebody you
// *cannot* see is not always silent. If the room is dark, you cannot see in the
// dark, and **they** have infravision, you get a pair of glowing red eyes
// instead of nothing. Note whose infravision it is — theirs, not yours — so it
// is the creature's own night vision that gives it away.
func listCharToChar(w *game.Live, room *game.RoomDef, viewer *game.Character) string {
	var b strings.Builder
	dark := w.RoomIsDark(room.Vnum) && !game.CanSeeInDark(viewer)

	for _, other := range w.Occupants(room.Vnum) {
		if other == viewer {
			continue
		}
		if w.CanSee(viewer, other) {
			b.WriteString(listOneChar(w, other, viewer))
			continue
		}
		if dark && other.HasAffect(game.AffectInfravision) {
			b.WriteString("You see a pair of glowing red eyes looking your way.\r\n")
		}
	}
	return b.String()
}

// charPositions are list_one_char's positions[] (act.informative.c:261),
// indexed by position. POS_FIGHTING's slot is never used — the code branches
// before reaching it — and the C's placeholder is left here for the same
// reason it is there: so the indices line up with the position constants.
var charPositions = [...]string{
	" is lying here, dead.",
	" is lying here, mortally wounded.",
	" is lying here, incapacitated.",
	" is lying here, stunned.",
	" is sleeping here.",
	" is resting here.",
	" is sitting here.",
	"!FIGHTING!",
	" is standing here.",
}

// listOneChar is list_one_char (act.informative.c:259): one line describing
// somebody the viewer can see.
//
// Two shapes, and which one you get is not about being a mobile. A mobile
// standing in its *default* position uses the long description the builder
// wrote for it; the same mobile sitting down, or fighting, or dead, falls
// through to the constructed line — which is why a corpse-to-be says "the
// cityguard is lying here, mortally wounded" rather than the long description
// that has it standing at attention.
func listOneChar(w *game.Live, who, viewer *game.Character) string {
	if who.MobDef != nil && who.MobDef.LongDesc != "" &&
		who.Position == game.Position(who.MobDef.Position) {
		var b strings.Builder
		// A `*` in front of a long description means invisible. You only ever
		// see it with detect invisible on, since otherwise the mobile is not
		// listed at all.
		if who.HasAffect(game.AffectInvisible) {
			b.WriteString("*")
		}
		b.WriteString(auraPrefix(who, viewer))
		b.WriteString(ensureNewline(who.MobDef.LongDesc))
		b.WriteString(glowLines(w, who, viewer))
		return b.String()
	}

	var b strings.Builder
	if who.IsNPC() {
		b.WriteString(capitaliseFirst(who.Name))
	} else {
		fmt.Fprintf(&b, "%s %s", who.Name, title(who))
	}

	if who.HasAffect(game.AffectInvisible) {
		b.WriteString(" (invisible)")
	}
	if who.HasAffect(game.AffectHide) {
		b.WriteString(" (hidden)")
	}
	if !who.IsNPC() && who.Client == nil {
		b.WriteString(" (linkless)")
	}
	if !who.IsNPC() && who.Record != nil && who.Record.PlayerFlags.Has(game.PlayerWriting) {
		b.WriteString(" (writing)")
	}

	if who.Position != game.PosFighting {
		b.WriteString(charPositions[who.Position])
	} else if who.Fighting == nil {
		// The C's comment is "NIL fighting pointer", and it happens: a
		// position of FIGHTING outlives the opponent by however long it takes
		// something to clear it.
		b.WriteString(" is here struggling with thin air.")
	} else {
		b.WriteString(" is here, fighting ")
		switch {
		case who.Fighting == viewer:
			b.WriteString("YOU!")
		case who.Fighting.Room != who.Room:
			b.WriteString("someone who has already left!")
		default:
			// PERS, so an invisible opponent is "someone" even here.
			fmt.Fprintf(&b, "%s!", w.Pers(who.Fighting, viewer))
		}
	}

	b.WriteString(auraSuffix(who, viewer))
	b.WriteString("\r\n")
	b.WriteString(glowLines(w, who, viewer))
	return b.String()
}

// title is the player's title, which follows their name in the room list. A
// mobile has none.
func title(who *game.Character) string {
	if who.Record == nil {
		return ""
	}
	return who.Record.Title
}

// auraPrefix and auraSuffix are the same two tests written twice in the C, in
// the two branches of list_one_char, and they differ: the long-description
// branch puts "(Red Aura) " *before* with a trailing space, the constructed one
// puts " (Red Aura)" *after*. Reproduced rather than unified, because the
// difference is visible.
func auraPrefix(who, viewer *game.Character) string {
	switch {
	case !viewer.HasAffect(game.AffectDetectAlign) || who.Record == nil:
		return ""
	case game.IsEvil(who.Record):
		return "(Red Aura) "
	case game.IsGood(who.Record):
		return "(Blue Aura) "
	}
	return ""
}

func auraSuffix(who, viewer *game.Character) string {
	switch {
	case !viewer.HasAffect(game.AffectDetectAlign) || who.Record == nil:
		return ""
	case game.IsEvil(who.Record):
		return " (Red Aura)"
	case game.IsGood(who.Record):
		return " (Blue Aura)"
	}
	return ""
}

// glowLines are the act() messages list_one_char sends after the line itself.
// Both branches send the sanctuary one; only the long-description branch sends
// the blindness one, which is the C's and looks like an oversight rather than
// a decision — a blind *player* is never reported as groping around.
func glowLines(w *game.Live, who, viewer *game.Character) string {
	var b strings.Builder
	if who.HasAffect(game.AffectSanctuary) {
		b.WriteString(w.Act("...$e glows with a bright light!", game.ActArgs{Actor: who}, viewer))
	}
	if who.MobDef != nil && who.MobDef.LongDesc != "" && who.HasAffect(game.AffectBlind) {
		b.WriteString(w.Act("...$e is groping around blindly!", game.ActArgs{Actor: who}, viewer))
	}
	return b.String()
}

// isBlind reports whether AFF_BLIND is set.
func isBlind(ch *game.Character) bool {
	return ch != nil && ch.Record != nil && ch.Record.AffectFlags.Has(game.AffectBlind)
}

// hasPref reports whether a player has a PRF_ bit set. A mobile has none: the
// C guards every one of these with !IS_NPC, because player_specials is not
// allocated for a mobile and reading it would be a null dereference.
func hasPref(ch *game.Character, flag game.Flags) bool {
	return ch != nil && !ch.IsNPC() && ch.Record != nil && ch.Record.Preferences.Has(flag)
}

// autoExits is do_auto_exits' list (act.informative.c:358).
//
// Two details that are easy to miss and both player-visible: a **closed** exit
// is not listed, so a shut door hides the way it leads; and a room with no way
// out at all says "None! " rather than nothing. Each letter is written with a
// trailing space, which is where the space before the closing bracket comes
// from.
func autoExits(room *game.RoomDef) string {
	var b strings.Builder
	for dir, e := range room.Exits {
		if e == nil || e.ToRoom == game.NoRoom || e.State.Has(game.ExitClosed) {
			continue
		}
		fmt.Fprintf(&b, "%c ", game.Direction(dir).String()[0])
	}
	if b.Len() == 0 {
		return "None! "
	}
	return b.String()
}

// exitNames lists the room's exits, truncated to width characters each.
//
// This feeds GMCP rather than the `[ Exits: ]` line, and unlike that line it
// reports closed exits too: a client drawing a map wants to know the door is
// there.
func exitNames(room *game.RoomDef, width int) []string {
	var out []string
	for dir, e := range room.Exits {
		if e == nil || e.ToRoom == game.NoRoom {
			continue
		}
		name := game.Direction(dir).String()
		if width > 0 && width < len(name) {
			name = name[:width]
		}
		out = append(out, name)
	}
	return out
}

// sendRoomInfo publishes the room out of band, so a web client can draw a map
// instead of parsing the description.
func sendRoomInfo(s *Session, room *game.RoomDef) {
	s.SendGMCP("Room.Info", RoomInfo{
		Vnum:  int32(room.Vnum),
		Name:  room.Name,
		Desc:  room.Description,
		Exits: exitNames(room, 0),
	})
}

// move returns the command for one direction.
func move(dir game.Direction) func(*Context) error {
	return func(c *Context) error {
		c.moveCharacter(c.Character, dir)
		return nil
	}
}

// moveCharacter walks one character one step, porting perform_move and
// do_simple_move.
//
// It takes the character rather than working on the session's own, because
// the last thing it does is move everybody who was following them — and each
// of those has to see the room they arrive in, told to them rather than to
// whoever gave the order. The recursion is the C's, and it terminates because
// following in loops is refused when the link is made.
func (c *Context) moveCharacter(who *game.Character, dir game.Direction) bool {
	exit := c.World.Exit(who.Room, dir)
	if exit == nil || exit.ToRoom == game.NoRoom {
		who.Tell("Alas, you cannot go that way...\r\n")
		return false
	}
	// A closed door stops a player, as it already stopped a mobile. The C
	// names the door if it has a keyword, which is how a player knows what to
	// open.
	if exit.State.Has(game.ExitClosed) {
		if name := doorName(exit); name != "door" {
			who.Tell("The %s seems to be closed.\r\n", name)
		} else {
			who.Tell("It seems to be closed.\r\n")
		}
		return false
	}
	if c.World.Room(exit.ToRoom) == nil {
		// The loader reports these as warnings rather than refusing to start,
		// so a player can still walk into one.
		who.Tell("The way is blocked by something you cannot describe.\r\n")
		return false
	}

	// House_can_enter (act.movement.c:133). Note it is guarded by the room
	// you are *leaving* being an atrium, not by the room you are entering
	// being a house — so a house reachable any other way than through its
	// atrium is not guarded at all. That is why hcontrol insists the door be
	// two-way: it is the only door.
	if from := c.World.Room(who.Room); from != nil && from.Flags.Has(game.RoomAtrium) {
		if !c.World.HouseCanEnter(who, exit.ToRoom) {
			who.Tell("That's private property -- no trespassing!\r\n")
			return false
		}
	}

	leaving := who.Room
	if err := c.World.Enter(who, exit.ToRoom); err != nil {
		who.Tell("The way is blocked by something you cannot describe.\r\n")
		return false
	}
	announce(c.World, leaving, who, "%s leaves %s.\r\n", who.Name, dir)
	announce(c.World, exit.ToRoom, who, "%s has arrived.\r\n", who.Name)

	if room := c.World.Room(exit.ToRoom); room != nil {
		if who == c.Character {
			sendRoomInfo(c.Session, room)
		}
		who.Tell("%s", roomDescription(c.World, room, who, false))
	}

	c.moveFollowers(who, leaving, dir)
	return true
}

// announce tells everyone in a room something, except the character it is
// about.
func announce(w *game.Live, room game.RoomVnum, except *game.Character, format string, args ...any) {
	for _, c := range w.Occupants(room) {
		if c != except {
			c.Tell(format, args...)
		}
	}
}

// `hit`, `murder`, `kill` and `assist` live in offensive.go.

func doWho(c *Context) error {
	// CAN_SEE, as do_who does (act.informative.c:1086) — so an `invis` god is
	// off the list for anyone below their level, and an invisible player is
	// off it for anyone without detect invisible.
	//
	// Worth knowing what else this drags in: CAN_SEE asks LIGHT_OK, and
	// LIGHT_OK asks about the *viewer's* room. **A mortal standing in an
	// unlit room sees nobody at all on the who-list.** That is the C's, it is
	// invisible in Midgaard because every room there is lit, and it is
	// startling the first time it happens in a cave. See
	// docs/weirdnumbers.md.
	var shown []*game.Character
	for _, p := range c.World.Players() {
		if c.World.CanSee(c.Character, p) {
			shown = append(shown, p)
		}
	}

	c.Send("Players\r\n-------\r\n")
	for _, p := range shown {
		title := p.Title()
		if title != "" {
			title = " " + title
		}
		c.Send("[%3d] %s%s\r\n", p.Level(), p.Name, title)
	}
	c.Send("\r\n%d character%s playing.\r\n", len(shown), plural(len(shown)))
	return nil
}

// doCredits shows the credits file.
//
// This is licence compliance, not a feature. The CircleMUD licence requires
// that the text in the credits file be preserved and displayed when the
// `credits` command is used (docs/proposals/go-port-plan.md §12). It is in
// the first set of commands implemented for exactly that reason.
func doCredits(c *Context) error {
	c.Send("%s", ensureNewline(c.Text.Credits()))
	return nil
}

// doHelp is do_help (act.informative.c:953-991): a lookup into the loaded
// help table, or the bare-`help` screen with no argument.
//
// `help circlemud` (also `help credits`, `help circle`) is a licence
// requirement (docs/proposals/go-port-plan.md §12) and needs no special
// case to satisfy it: CIRCLE CIRCLEMUD CREDITS is a real keyword in the
// real archived help data (data/text/help/info.hlp), reached by the same
// lookup as anything else, once the table is loaded. The `credits`
// command above is a different mechanism entirely — a separate file,
// text/credits — and both have to work; they are not one thing wearing
// two names.
func doHelp(c *Context) error {
	if c.Arg == "" {
		if screen := c.Text.HelpScreen(); screen != "" {
			c.Send("%s", ensureNewline(screen))
			return nil
		}
		// No help data configured at all: the command list this stub
		// always showed, so a server with none is no worse off than
		// before this landed.
		c.Send("Commands\r\n--------\r\n")
		for _, cmd := range Commands {
			c.Send("  %-10s %s\r\n", cmd.Name, cmd.Help)
		}
		return nil
	}

	entry, ok := c.Text.Help(c.Arg)
	if !ok {
		c.Send("There is no help on that word.\r\n")
		return nil
	}
	c.Send("%s", ensureNewline(entry))
	return nil
}

// quitCommand is do_quit under its two names. The C passes SCMD_QUIT for the
// full spelling and nothing for `qui`.
func quitCommand(full bool) func(*Context) error {
	return func(c *Context) error { return doQuit(c, full) }
}

// doQuit is do_quit (act.other.c:99), guards included.
//
// `quit` is POS_DEAD in the command table, so the interpreter refuses nobody
// and every check here has to be made by the command itself. Three of them:
//
//   - The `<DoC>` item count, which is a local addition and the first thing
//     the function does. A mortal carrying more than MAX_RENT items cannot
//     leave, because the rent file cannot hold them and quitting would lose
//     the excess. Note what it counts: everything carried, everything worn,
//     and the contents of any container in either — recursively, so a bag in
//     a bag is counted through.
//   - The `qui`/`quit` distinction. `qui` reaches this function with no
//     subcommand, which is the C's way of making an abbreviation of a
//     dangerous command refuse rather than act. An immortal is exempt.
//   - Position. Fighting refuses; below stunned kills you outright, which is
//     the C being literal about a character who quits while dying.
func doQuit(c *Context, full bool) error {
	ch := c.Character
	immortal := ch.Level() >= game.LevelImmortal

	if items := carriedItemCount(ch); items > game.MaxRent && !immortal {
		c.Send("You currently have too many items (%d items in total).\r\n"+
			"You must have %d items or less before leaving.\r\n", items, game.MaxRent)
		return nil
	} else {
		// Unconditional in the C, and on the *else* of the refusal — so a
		// player who is allowed to leave is always told the count.
		c.Send("Saving %d items.\r\n", items)
	}

	switch {
	case !full && !immortal:
		c.Send("You have to type quit--no less, to quit!\r\n")
	case ch.Position == game.PosFighting:
		c.Send("No way!  You're fighting for your life!\r\n")
	case ch.Position < game.PosStunned:
		// die() rather than a refusal: quitting while mortally wounded is
		// taken as giving up.
		c.Send("You die before your time...\r\n")
		c.Violence.Damage(c.World, nil, ch, ch.Record.Points.Hit+1)
	default:
		c.announce("%s has left the game.\r\n", ch.Name)
		c.Send("Goodbye, friend.. Come back soon!\r\n")
		// Quitting in a house makes it your load room, unless a god has
		// already pinned one (act.other.c:167).
		if room := c.World.Room(ch.Room); room != nil && ch.Record != nil &&
			!ch.Record.PlayerFlags.Has(game.PlayerLoadRoom) && room.Flags.Has(game.RoomHouse) {
			ch.Record.LoadRoom = ch.Room
		}
		c.Session.MarkQuit()
		c.Session.Close()
	}
	return nil
}

// carriedItemCount is count_items (act.other.c:77) over the inventory, plus
// do_quit's loop over the equipment.
//
// Each object counts one, and a container counts its contents as well as
// itself — recursively, so a pouch inside a bag inside a backpack is three
// items plus whatever is in the pouch. Worn containers are counted the same
// way; worn anything-else counts one.
func carriedItemCount(ch *game.Character) int32 {
	count := countItems(ch.Carrying)
	for _, worn := range ch.Equipment {
		if worn == nil {
			continue
		}
		if worn.Type == game.ItemContainer {
			count += countItems(worn.Contents)
		}
		count++
	}
	return count
}

func countItems(list []*game.Object) int32 {
	var count int32
	for _, o := range list {
		if o.Type == game.ItemContainer {
			count += countItems(o.Contents)
		}
		count++
	}
	return count
}

func ensureNewline(s string) string {
	if s == "" || strings.HasSuffix(s, "\n") {
		return s
	}
	return s + "\r\n"
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
