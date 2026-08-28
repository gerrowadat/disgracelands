// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package session

import (
	"fmt"
	"strings"
	"time"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// do_ban, do_unban and do_show, ported from ban.c:128 and act.wizard.c:2155.
//
// `ban` keeps people out; `show` is everything the server knows about itself,
// behind one dispatcher with a per-field level.

// BanKeeper is the ban list, as the commands need it. A seam like the others.
type BanKeeper interface {
	// Bans lists them, newest first.
	Bans() []BanEntry
	// Ban records one, reporting false when the site is already listed.
	Ban(site, kind, by string) (bool, error)
	// Unban removes one, reporting the type it was.
	Unban(site string) (string, bool, error)
	// ValidBanType reports whether a word names a ban type.
	ValidBanType(kind string) bool
}

// BanEntry is one line of the ban list.
type BanEntry struct {
	Site string
	Type string
	When time.Time
	By   string
}

// banListFormat is BAN_LIST_FORMAT (ban.c:128). The `.25`, `.8`, `.10` and
// `.16` precisions truncate as well as pad, which is why a long hostname is
// cut off in the listing rather than pushing the columns out.
const banListFormat = "%-25.25s  %-8.8s  %-10.10s  %-16.16s\r\n"

// doBan is do_ban (ban.c:129).
func doBan(c *Context) error {
	if c.Bans == nil {
		return nil
	}

	if strings.TrimSpace(c.Arg) == "" {
		list := c.Bans.Bans()
		if len(list) == 0 {
			c.Send("No sites are banned.\r\n")
			return nil
		}
		var b strings.Builder
		fmt.Fprintf(&b, banListFormat, "Banned Site Name", "Ban Type", "Banned On", "Banned By")
		rule := strings.Repeat("-", 33)
		fmt.Fprintf(&b, banListFormat, rule, rule, rule, rule)
		for _, ban := range list {
			fmt.Fprintf(&b, banListFormat, ban.Site, ban.Type,
				houseDate(ban.When, "Unknown"), ban.By)
		}
		c.Send("%s", b.String())
		return nil
	}

	kind, site, _ := twoArguments(c.Arg)
	if site == "" || kind == "" {
		c.Send("Usage: ban {all | select | new} site_name\r\n")
		return nil
	}
	// `no` is a valid ban type in the file and *not* one you may set: the C
	// checks the three by name rather than looping the table.
	if !c.Bans.ValidBanType(kind) || strings.EqualFold(kind, "no") {
		c.Send("Flag must be ALL, SELECT, or NEW.\r\n")
		return nil
	}

	added, err := c.Bans.Ban(site, kind, c.Character.Name)
	if err != nil {
		c.Send("The ban list could not be written.\r\n")
		return nil
	}
	if !added {
		c.Send("That site has already been banned -- unban it to change the ban type.\r\n")
		return nil
	}
	c.Send("Site banned.\r\n")
	return nil
}

// doUnban is do_unban (ban.c:210).
func doUnban(c *Context) error {
	if c.Bans == nil {
		return nil
	}
	site, _ := oneArgument(c.Arg)
	if site == "" {
		c.Send("A site to unban might help.\r\n")
		return nil
	}

	_, found, err := c.Bans.Unban(site)
	if err != nil {
		c.Send("The ban list could not be written.\r\n")
		return nil
	}
	if !found {
		c.Send("That site is not currently banned.\r\n")
		return nil
	}
	c.Send("Site unbanned.\r\n")
	return nil
}

// showFields is do_show's table (act.wizard.c:2166): what may be shown, and
// who may see it.
var showFields = []struct {
	name  string
	level int32
}{
	{"zones", game.LevelImmortal},
	{"player", game.LevelGod},
	{"rent", game.LevelGod},
	{"stats", game.LevelImmortal},
	{"errors", game.LevelImplementor},
	{"death", game.LevelGod},
	{"godrooms", game.LevelGod},
	{"shops", game.LevelImmortal},
	{"houses", game.LevelGod},
	{"snoop", game.LevelGreaterGod},
}

// doShow is do_show (act.wizard.c:2155).
func doShow(c *Context) error {
	if strings.TrimSpace(c.Arg) == "" {
		var b strings.Builder
		b.WriteString("Show options:\r\n")
		shown := 0
		for _, field := range showFields {
			if field.level > levelOf(c.Character) {
				continue
			}
			shown++
			fmt.Fprintf(&b, "%-15s", field.name)
			if shown%5 == 0 {
				b.WriteString("\r\n")
			}
		}
		b.WriteString("\r\n")
		c.Send("%s", b.String())
		return nil
	}

	field, value, _ := twoArguments(c.Arg)

	chosen := -1
	for i, candidate := range showFields {
		if strings.HasPrefix(candidate.name, strings.ToLower(field)) {
			chosen = i
			break
		}
	}
	if chosen < 0 {
		c.Send("Sorry, I don't understand that.\r\n")
		return nil
	}
	if levelOf(c.Character) < showFields[chosen].level {
		c.Send("You are not godly enough for that!\r\n")
		return nil
	}

	// `.` means "the one I am standing in", and only `show zones` uses it.
	self := value == "."

	switch showFields[chosen].name {
	case "zones":
		c.showZones(value, self)
	case "player":
		c.showPlayer(value)
	case "stats":
		c.showStats()
	case "errors":
		c.showRooms("Errant Rooms\r\n------------\r\n", func(room *game.RoomDef) bool {
			// A room with an exit *to room zero* is almost always a builder's
			// mistake: zero is the first room in the file, not "nowhere".
			for _, exit := range room.Exits {
				if exit != nil && exit.ToRoom == 0 {
					return true
				}
			}
			return false
		})
	case "death":
		c.showRooms("Death Traps\r\n-----------\r\n", func(room *game.RoomDef) bool {
			return room.Flags.Has(game.RoomDeathTrap)
		})
	case "godrooms":
		c.showRooms("Godrooms\r\n--------------------------\r\n", func(room *game.RoomDef) bool {
			return room.Flags.Has(game.RoomGodRoom)
		})
	case "snoop":
		c.showSnooping()
	case "rent":
		c.showRent(value)
	case "shops":
		c.showShops(value, self)
	case "houses":
		// hcontrol_list_houses, which `hcontrol show` also reaches
		// (act.wizard.c:2321). `show` is LVL_GOD and `hcontrol` is LVL_GRGOD
		// (interpreter.c:330), so this is the listing a god can see without
		// having the command that prints it.
		if c.Houses == nil {
			c.Send("Houses are not enabled on this server.\r\n")
			return nil
		}
		hcontrolShow(c)
	}
	return nil
}

// showZones prints one zone, the current one, or all of them.
func (c *Context) showZones(value string, self bool) {
	// The zone's *age* — minutes since its last reset — is the server's, not
	// the world's: the C keeps it in the zone table and this port keeps the
	// ageing state beside the pulse that drives it.
	age := func(zone *game.ZoneDef) int32 {
		if c.Operator == nil {
			return 0
		}
		return c.Operator.ZoneAge(zone.Vnum)
	}
	line := func(zone *game.ZoneDef) string {
		return fmt.Sprintf("%3d %-30.30s Age: %3d; Reset: %3d (%1d); Range: %5d-%5d\r\n",
			zone.Vnum, zone.Name, age(zone), zone.Lifespan,
			zone.ResetMode, zone.Bottom, zone.Top)
	}

	if self {
		if zone := c.World.ZoneOf(c.Character.Room); zone != nil {
			c.SendPaged("%s", line(zone))
		}
		return
	}
	if value != "" && isNumber(value) {
		want := game.ZoneVnum(atoi(value))
		for _, zone := range c.World.Zones() {
			if zone.Vnum == want {
				c.SendPaged("%s", line(zone))
				return
			}
		}
		c.Send("That is not a valid zone.\r\n")
		return
	}

	var b strings.Builder
	for _, zone := range c.World.Zones() {
		b.WriteString(line(zone))
	}
	c.SendPaged("%s", b.String())
}

// showPlayer reads somebody out of the roster, so it works for a character
// who is not logged in.
func (c *Context) showPlayer(name string) {
	if name == "" {
		c.Send("A name would help.\r\n")
		return
	}
	if c.Operator == nil {
		c.Send("There is no such player.\r\n")
		return
	}
	entry, ok := c.Operator.ShowPlayer(name)
	if !ok {
		c.Send("There is no such player.\r\n")
		return
	}

	c.Send("Player: %-12s (%s) [%2d %s]\r\n", entry.Name,
		game.SprintType(entry.Sex, game.GenderNames()), entry.Level,
		game.ClassAbbrevs[entry.Class])
	c.Send("Au: %-8d  Bal: %-8d  Exp: %-8d  Align: %-5d  Lessons: %-3d\r\n",
		entry.Gold, entry.Bank, entry.Exp, entry.Alignment, entry.Lessons)
	c.Send("Started: %-20.16s  Last: %-20.16s  Played: %3dh %2dm\r\n",
		clockTime(entry.Born), clockTime(entry.LastLogon),
		int64(entry.Played/time.Hour), int64(entry.Played/time.Minute)%60)
}

// showRent is Crash_listrent (objsave.c:342): what a name's rent file holds,
// read without loading it into the world. Not paginated — the C sends it
// with one plain send_to_char, unlike show's other listings.
func (c *Context) showRent(name string) {
	if name == "" {
		c.Send("A name would help.\r\n")
		return
	}
	if c.Operator == nil {
		return
	}
	listing, ok := c.Operator.ShowRent(name)
	if !ok {
		c.Send("%s has no rent file.\r\n", name)
		return
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s\r\n", listing.Code)
	for _, vnum := range listing.Vnums {
		def := c.World.ObjectDef(vnum)
		if def == nil {
			// real_object(object.item_number) != NOTHING: the prototype is
			// gone, and read_object would have nothing to read.
			continue
		}
		fmt.Fprintf(&b, " [%5d] (%5dau) %-20s\r\n", vnum, def.RentPerDay, def.ShortDesc)
	}
	c.Send("%s", b.String())
}

// showStats is the counts, which is what a god types when somebody asks how
// big the game is.
func (c *Context) showStats() {
	players, connected := 0, 0
	for _, who := range c.World.Players() {
		players++
		if who.Client != nil {
			connected++
		}
	}
	mobiles := len(c.World.Mobiles())

	var b strings.Builder
	b.WriteString("Current stats:\r\n")
	fmt.Fprintf(&b, "  %5d players in game  %5d connected\r\n", players, connected)
	fmt.Fprintf(&b, "  %5d mobiles          %5d prototypes\r\n",
		mobiles, len(c.World.MobileDefs()))
	fmt.Fprintf(&b, "  %5d objects          %5d prototypes\r\n",
		len(c.World.Objects()), len(c.World.ObjectDefs()))
	fmt.Fprintf(&b, "  %5d rooms            %5d zones\r\n",
		c.World.RoomCount(), len(c.World.Zones()))
	c.Send("%s", b.String())
}

// showRooms lists every room matching a test, numbered.
func (c *Context) showRooms(heading string, matches func(*game.RoomDef) bool) {
	var b strings.Builder
	b.WriteString(heading)

	found := 0
	for i := 0; i < c.World.RoomCount(); i++ {
		room := c.World.RoomAt(i)
		if room == nil || !matches(room) {
			continue
		}
		found++
		fmt.Fprintf(&b, "%2d: [%5d] %s\r\n", found, room.Vnum, room.Name)
	}
	c.SendPaged("%s", b.String())
}

// showSnooping is `show snoop`, and it is the only way to find out that
// somebody is watching you — which is to say a god can find out and you
// cannot.
func (c *Context) showSnooping() {
	c.Send("People currently snooping:\r\n")
	c.Send("--------------------------\r\n")

	if c.Operator == nil {
		c.Send("No one is currently snooping.\r\n")
		return
	}

	var b strings.Builder
	for _, sess := range c.Operator.Sessions() {
		watched := sess.Snooping()
		snooper := sess.Character()
		if watched == nil || snooper == nil {
			continue
		}
		// You cannot see a snoop by somebody above you.
		if levelOf(c.Character) < levelOf(snooper) {
			continue
		}
		if victim := watched.Character(); victim != nil {
			fmt.Fprintf(&b, "%-10s - snooped by %s.\r\n", victim.Name, snooper.Name)
		}
	}
	if b.Len() == 0 {
		c.Send("No one is currently snooping.\r\n")
		return
	}
	c.Send("%s", b.String())
}
