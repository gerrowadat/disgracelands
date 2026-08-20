// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package session

import (
	"context"
	"fmt"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

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
	// Swing is one weapon attack, taken now rather than on the next round.
	Swing(w *game.Live, attacker, victim *game.Character)
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
		{Name: "transfer", Help: "Bring somebody to you.", Run: doTransfer, CLine: 518, MinLevel: game.LevelGod},
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
		{Name: "quit", Help: "Leave the game.", Run: doQuit, CLine: 422},
		{Name: "qsay", Help: "Talk to everybody on the quest.", Run: doQuestSay, CLine: 423},
		// Nowhere near the others: the C has it among the u's, after unlock
		// (interpreter.c:522 and :523), so `un` is unlock and ungrouping
		// needs `ung`.
		{Name: "ungroup", Help: "Disband your group, or expel somebody.", Run: doUngroup, CLine: 523},

		{Name: "autoexit", Help: "Show the exits after every move.", Run: toggleCommand("autoexit"), CLine: 232},
		{Name: "brief", Help: "Skip room descriptions you have seen.", Run: toggleCommand("brief"), CLine: 244},
		{Name: "clear", Help: "Clear the screen.", Run: doClearScreen, CLine: 254},
		{Name: "cls", Help: "Clear the screen.", Run: doClearScreen, CLine: 256},
		{Name: "commands", Help: "List the commands you can use.", Run: doCommands(false), CLine: 261},
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
		{Name: "socials", Help: "List the socials you can use.", Run: doCommands(true), CLine: 485},
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
	}
	Commands = sortedByCLine(staticCommands)
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
	socialsOnce.Do(func() { Commands = sortedByCLine(out) })
	return added, unknown
}

var socialsOnce sync.Once

// sortedByCLine puts the table in the C's order. Stable, so two entries that
// somehow share a line keep the order they were written in.
func sortedByCLine(in []Command) []Command {
	out := append([]Command(nil), in...)
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

		c := &Context{
			Ctx: ctx, Session: s, Character: s.Character(),
			World: w, Text: d.Text, RNG: d.RNG, Violence: d.Violence, Arg: arg,
			Social: cmd.Social, Save: d.Save, Rent: d.Rent, SaveBoard: d.SaveBoard, Mail: d.Mail, Houses: d.Houses,
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

	var b strings.Builder
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
	// `look <something>` describes one thing; bare `look` describes the room.
	if arg := strings.TrimSpace(c.Arg); arg != "" {
		// `look in <container>` and `look at <thing>` are both the C's, and
		// both mean "describe that".
		arg = strings.TrimPrefix(arg, "at ")
		arg = strings.TrimPrefix(arg, "in ")
		return c.lookAtTarget(arg)
	}

	room := c.World.Room(c.Character.Room)
	if room == nil {
		c.Send("You are nowhere at all. That should not be possible.\r\n")
		return nil
	}

	sendRoomInfo(c.Session, room)
	c.Send("%s", roomDescription(c.World, room, c.Character))
	return nil
}

// roomDescription is look_at_room's text: the name, the description, the way
// out, what is lying about and who is here.
//
// It takes the viewer so it can leave them out of the list of people, and it
// returns a string rather than sending, because a spell can move somebody
// else into a room and has to show it to them rather than to the caster.
func roomDescription(w *game.Live, room *game.RoomDef, viewer *game.Character) string {
	var b strings.Builder

	fmt.Fprintf(&b, "%s\r\n", room.Name)
	if room.Description != "" {
		b.WriteString(ensureNewline(room.Description))
	}
	if exits := exitList(room); exits != "" {
		fmt.Fprintf(&b, "[ Exits: %s ]\r\n", exits)
	}

	for _, obj := range w.RoomObjects(room.Vnum) {
		if obj.Description != "" {
			fmt.Fprintf(&b, "%s\r\n", obj.Description)
			continue
		}
		fmt.Fprintf(&b, "%s is lying here.\r\n", capitaliseFirst(obj.Name()))
	}

	for _, other := range w.Occupants(room.Vnum) {
		if other == viewer {
			continue
		}
		// A mobile has a long description written for exactly this line; a
		// player does not, so they get the generic one.
		if other.MobDef != nil && other.MobDef.LongDesc != "" {
			b.WriteString(ensureNewline(other.MobDef.LongDesc))
			continue
		}
		fmt.Fprintf(&b, "%s is standing here.\r\n", other.Name)
	}
	return b.String()
}

func exitList(room *game.RoomDef) string {
	return strings.Join(exitNames(room, 1), " ")
}

// exitNames lists the room's exits, truncated to width characters each. One
// character is what the `[ Exits: ]` line shows; GMCP wants the whole word.
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
		who.Tell("%s", roomDescription(c.World, room, who))
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
	players := c.World.Players()
	c.Send("Players\r\n-------\r\n")
	for _, p := range players {
		title := p.Title()
		if title != "" {
			title = " " + title
		}
		c.Send("[%3d] %s%s\r\n", p.Level(), p.Name, title)
	}
	c.Send("\r\n%d character%s playing.\r\n", len(players), plural(len(players)))
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

// doHelp lists the commands, and answers `help circlemud`.
//
// The CIRCLEMUD help entry is also a licence requirement and must be shown
// intact. Until the help database is loaded (a later phase), it is served
// from the credits file, which carries the same attribution and is required
// to be intact for the same reason.
func doHelp(c *Context) error {
	if strings.EqualFold(c.Arg, "circlemud") {
		c.Send("%s", ensureNewline(c.Text.Credits()))
		return nil
	}
	if c.Arg != "" {
		c.Send("There is no help on that yet.\r\n")
		return nil
	}

	c.Send("Commands\r\n--------\r\n")
	for _, cmd := range Commands {
		c.Send("  %-10s %s\r\n", cmd.Name, cmd.Help)
	}
	c.Send("\r\nType `help circlemud` for the credits.\r\n")
	return nil
}

func doQuit(c *Context) error {
	c.Send("Goodbye, friend.. Come back soon!\r\n")
	c.Session.MarkQuit()
	c.Session.Close()
	return nil
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
