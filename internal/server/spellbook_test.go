// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// The spellbook: one case per entry in game's spell table, driven the way a
// player drives it — typed at a socket — and asserted on what actually
// changed in the world afterwards.
//
// Why a table rather than sixty separate tests. The individual spell tests
// elsewhere in this package are each about a *rule* — that bless refuses an
// object over five pounds per level, that summon will not move a mobile,
// that a group spell reaches the group. Those are worth having and this does
// not replace them. What was missing was the flat question underneath: does
// each spell and skill do its main job at all. Most of them had no test that
// ever cast them, and the first run found four that do not: cure blind,
// remove poison and remove curse, which share one wrong argument in
// mag_unaffects (#299), and control weather, which is not written (#300).
// It also found two that could not be *reached* from this package's test
// world rather than being broken — create food and animate dead each build a
// hard-coded vnum, and neither prototype was here — which is its own kind of
// gap and is why they are in testWorld now.
//
// TestEverySpellAndSkillIsAccountedFor is what keeps it honest. The list of
// cases is checked *against the table*, not maintained beside it, so a spell
// added to game.spellTable fails this suite until somebody either covers it
// or writes down why it cannot be — the same shape as the command table
// being sorted by interpreter.c line number rather than asserted about.
//
// Assertions prefer state to prose. A message can be right while nothing
// happened, and several of these spells say the same sentence whether they
// worked or not (mag_alter_objs sends NOEFFECT for both). Where there is no
// state — identify, locate object, detect poison — the message is all there
// is and is what is checked.
//
// What this is not about is refusals: whether a spell says no in the right
// words to the wrong target is cast_spell's business, and its own gaps are
// #301. Each case here sets up the conditions under which the spell should
// work and asks whether it did.

// spellCase is one entry.
type spellCase struct {
	// run drives the spell or skill and asserts its outcome.
	run func(t *testing.T)
	// skip says why this one is not driven at all, for the entries where a
	// player cannot reach the spell in the first place.
	skip string
	// pending names the issue a written case is currently blocked on. The
	// body still exists and still compiles — deleting this one line is the
	// whole of turning it back on once the bug is fixed, which is the
	// difference between a pending test and a note in a document.
	pending string
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// spellbookServer boots a server and puts an implementor in the board room.
//
// An implementor is the right caster for a coverage suite and not merely a
// convenient one: init_char gives level-LVL_IMPL characters every skill at
// 100 (create.go), and do_cast exempts an immortal from the mana check
// (cast.go, spell_parser.c's `GET_LEVEL(ch) < LVL_IMMORT`), so one character
// can cast every spell in the table including the six with no class level at
// all. Where being an immortal changes the answer — mag_areas skips them,
// and an immortal takes no damage — the case says so and uses a mortal.
func spellbookServer(t *testing.T) (*Server, *client) {
	t.Helper()
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "c")
	return srv, c
}

// prey puts a mobile in the room that never makes a saving throw.
//
// mag_savingthrow is `max(1, target) < number(0, 99)` (game/saving.go), and a
// mobile is read as a warrior of its own level — so the roll decides, and a
// suite of sixty casts that each depend on one is a suite that fails
// somewhere every few runs. A saving-throw bonus far above 99 makes the
// comparison false every time, which turns "did the spell land" back into a
// question about the spell.
//
// It is given real abilities for the same reason: spell_strength reads the
// victim's strength percentile and refuses at 18/00, and chill touch and
// poison both write to strength.
func prey(t *testing.T, srv *Server, room game.RoomVnum) *game.Character {
	t.Helper()

	mob := &game.Character{
		Name: "a large dog", Keywords: "dog", NPC: true,
		Position: game.PosStanding,
		MobDef:   &game.MobDef{Vnum: testDogVnum, ShortDesc: "a large dog", Keywords: "dog"},
		Record: &game.PlayerRecord{
			Name: "a large dog", Level: 5, Mobile: true,
			Points: game.Points{Hit: 500, MaxHit: 500, Mana: 100, MaxMana: 100, Move: 100, MaxMove: 100},
			Abilities: game.Abilities{
				Strength: 13, Intelligence: 13, Wisdom: 13,
				Dexterity: 13, Constitution: 13, Charisma: 13,
			},
			SavingThrows: [5]int32{200, 200, 200, 200, 200},
		},
	}
	game.SnapshotReal(mob.Record)
	if err := srv.engine.DoSync(context.Background(), func(w *game.Live) {
		if err := w.Enter(mob, room); err != nil {
			t.Errorf("placing the mobile: %v", err)
		}
		w.Track(mob)
	}); err != nil {
		t.Fatal(err)
	}
	return mob
}

// lostConcentration is do_cast's own one-in-102 failure, which happens at
// 100% skill: `number(0, 101) > GET_SKILL(ch, spellnum)` (spell_parser.c).
const lostConcentration = "You lost your concentration!"

// castOn types a cast and retries the roll above.
//
// The alternative is pinning every case to a seed that happens to succeed,
// which would make the suite fragile against any change to how often the RNG
// is drawn from — and the failure is not what any of these cases is about.
// Twelve tries makes a spurious failure a one-in-10^21 event.
func castOn(c *client, spell, target string) {
	c.t.Helper()

	line := "cast '" + spell + "'"
	if target != "" {
		line += " " + target
	}
	for try := 0; try < 12; try++ {
		before := strings.Count(c.transcript(), lostConcentration)
		c.send(line)
		// settle() rather than the prompt: a spell imposes no wait state, and
		// this is a barrier for everything the cast wrote, including the
		// lines that went to other people in the room.
		c.settle()
		if strings.Count(c.transcript(), lostConcentration) == before {
			return
		}
	}
	c.t.Fatalf("%q lost its concentration twelve times running", line)
}

// castSelf casts with no target at all, which do_cast resolves to the caster
// for any non-violent spell that can be aimed at somebody in the room.
func castSelf(c *client, spell string) {
	c.t.Helper()
	castOn(c, spell, "")
}

// useSkill types a skill and waits for the command to finish.
//
// Not settle(): kick, bash and backstab impose a wait state, and settle()'s
// own probe command is held by it right along with anything else typed next
// — see the note on waitPromptCount. The prompt is sent whatever the command
// did, so it is the barrier that works for both kinds.
func useSkill(c *client, line string) {
	c.t.Helper()
	n := waitPromptCount(c)
	c.send(line)
	waitForPrompt(c, n+1)
}

// record reads a copy of somebody's record off the world goroutine.
//
// A copy, and read inside the closure rather than asserted inside it:
// t.Fatal in an inWorld closure kills the world goroutine (CLAUDE.md), so
// every case in this file reads first and asserts afterwards.
func record(t *testing.T, srv *Server, ch *game.Character) game.PlayerRecord {
	t.Helper()
	var out game.PlayerRecord
	inWorld(t, srv, func(_ *game.Live) {
		if ch != nil && ch.Record != nil {
			out = *ch.Record
		}
	})
	return out
}

// playerRecord is record() for somebody who is only known by name.
func playerRecord(t *testing.T, srv *Server, name string) game.PlayerRecord {
	t.Helper()
	var out game.PlayerRecord
	inWorld(t, srv, func(w *game.Live) {
		if who := w.Find(name); who != nil && who.Record != nil {
			out = *who.Record
		}
	})
	return out
}

// giveTo puts an object from the test world straight into somebody's hands.
//
// Not this package's existing carry(), which drops the object and types
// `get`: several of these cases want something whose `get` line nobody has
// written an expectation for, and none of them is about picking things up.
func giveTo(t *testing.T, srv *Server, name string, vnum game.ObjVnum) *game.Object {
	t.Helper()
	var obj *game.Object
	if err := srv.engine.DoSync(context.Background(), func(w *game.Live) {
		who := w.Find(name)
		if who == nil {
			t.Errorf("%s is not in the world", name)
			return
		}
		obj = w.NewObject(vnum)
		if obj == nil {
			t.Errorf("no prototype %d in the test world", vnum)
			return
		}
		w.ObjectToChar(obj, who)
	}); err != nil {
		t.Fatal(err)
	}
	return obj
}

// alignments sets the caster's and the victim's, for the two spells whose
// whole behaviour is a comparison of them.
func alignments(t *testing.T, srv *Server, casterName string, caster int32, victim *game.Character, vic int32) {
	t.Helper()
	if err := srv.engine.DoSync(context.Background(), func(w *game.Live) {
		if who := w.Find(casterName); who != nil && who.Record != nil {
			who.Record.Alignment = caster
		}
		if victim != nil && victim.Record != nil {
			victim.Record.Alignment = vic
		}
	}); err != nil {
		t.Fatal(err)
	}
}

// hurt takes a mobile down to a fixed number of hit points, so a healing
// spell has something to restore.
func hurt(t *testing.T, srv *Server, ch *game.Character, to int32) {
	t.Helper()
	if err := srv.engine.DoSync(context.Background(), func(_ *game.Live) {
		ch.Record.Points.Hit = to
	}); err != nil {
		t.Fatal(err)
	}
}

// ---------------------------------------------------------------------------
// Case constructors for the families that behave the same way
// ---------------------------------------------------------------------------

// damages is mag_damage: the thing it is aimed at loses hit points.
func damages(spell string) spellCase {
	return spellCase{run: func(t *testing.T) {
		srv, c := spellbookServer(t)
		dog := prey(t, srv, ImmortStartRoom)

		before := record(t, srv, dog).Points.Hit
		castOn(c, spell, "dog")
		after := record(t, srv, dog).Points.Hit

		if after >= before {
			t.Errorf("%s left the dog on %d hit points, was %d", spell, after, before)
		}
	}}
}

// affectsSelf is mag_affects aimed at the caster: the flag arrives and the
// caster is told.
func affectsSelf(spell string, flag game.Flags, told string) spellCase {
	return spellCase{run: func(t *testing.T) {
		srv, c := spellbookServer(t)

		castSelf(c, spell)

		if rec := playerRecord(t, srv, "Zod"); !rec.AffectFlags.Has(flag) {
			t.Errorf("%s did not set its affect flag: have %v", spell, rec.AffectFlags)
		}
		if told != "" && !c.seen(told) {
			t.Errorf("%s did not say %q; the transcript was:\n%s", spell, told, c.transcript())
		}
	}}
}

// affectsPrey is mag_affects aimed at a mobile that cannot save.
func affectsPrey(spell string, flag game.Flags) spellCase {
	return spellCase{run: func(t *testing.T) {
		srv, c := spellbookServer(t)
		dog := prey(t, srv, ImmortStartRoom)

		castOn(c, spell, "dog")

		if rec := record(t, srv, dog); !rec.AffectFlags.Has(flag) {
			t.Errorf("%s did not set its affect flag on the dog: have %v", spell, rec.AffectFlags)
		}
	}}
}

// heals is mag_points: a hurt mobile ends up with more hit points.
func heals(spell string) spellCase {
	return spellCase{run: func(t *testing.T) {
		srv, c := spellbookServer(t)
		dog := prey(t, srv, ImmortStartRoom)
		hurt(t, srv, dog, 50)

		castOn(c, spell, "dog")

		if got := record(t, srv, dog).Points.Hit; got <= 50 {
			t.Errorf("%s left the dog on %d hit points, was 50", spell, got)
		}
	}}
}

// ---------------------------------------------------------------------------
// The table
// ---------------------------------------------------------------------------

var spellbook = map[int32]spellCase{
	// -- mag_affects, on the caster ------------------------------------

	game.SpellArmor: {run: func(t *testing.T) {
		srv, c := spellbookServer(t)
		before := playerRecord(t, srv, "Zod").Points.Armor

		castSelf(c, "armor")

		if got, want := playerRecord(t, srv, "Zod").Points.Armor, before-20; got != want {
			t.Errorf("armour class is %d, want %d", got, want)
		}
		if !c.seen("You feel someone protecting you.") {
			t.Errorf("armor said nothing; the transcript was:\n%s", c.transcript())
		}
	}},

	game.SpellBless: {run: func(t *testing.T) {
		srv, c := spellbookServer(t)
		before := playerRecord(t, srv, "Zod").Points.HitRoll

		castSelf(c, "bless")

		rec := playerRecord(t, srv, "Zod")
		if got, want := rec.Points.HitRoll, before+2; got != want {
			t.Errorf("hitroll is %d, want %d", got, want)
		}
		if !c.seen("You feel righteous.") {
			t.Errorf("bless said nothing; the transcript was:\n%s", c.transcript())
		}

		// Bless is also mag_alter_objs, and the object half is a different
		// routine reached by aiming it at something instead.
		sword := giveTo(t, srv, "Zod", testSwordVnum)
		castOn(c, "bless", "sword")
		var blessed bool
		inWorld(t, srv, func(_ *game.Live) { blessed = sword.ExtraFlags.Has(game.ItemBless) })
		if !blessed {
			t.Error("bless did not bless the sword")
		}
	}},

	game.SpellDetectAlign: affectsSelf("detect alignment", game.AffectDetectAlign, "Your eyes tingle."),
	game.SpellDetectInvis: affectsSelf("detect invisibility", game.AffectDetectInvis, "Your eyes tingle."),
	game.SpellDetectMagic: affectsSelf("detect magic", game.AffectDetectMagic, "Your eyes tingle."),
	game.SpellInfravision: affectsSelf("infravision", game.AffectInfravision, "Your eyes glow red."),
	game.SpellSenseLife:   affectsSelf("sense life", game.AffectSenseLife, "Your feel your awareness improve."),
	game.SpellProtFromEvil: affectsSelf("protection from evil", game.AffectProtectEvil,
		"You feel invulnerable!"),
	game.SpellHolyShield: affectsSelf("holy shield", game.AffectHolyShield,
		"You feel yourself protected by righteousness!"),
	game.SpellWaterwalk: affectsSelf("waterwalk", game.AffectWaterwalk,
		"You feel webbing between your toes."),
	game.SpellSanctuary: affectsSelf("sanctuary", game.AffectSanctuary,
		"A white aura momentarily surrounds you."),

	game.SpellInvisible: {run: func(t *testing.T) {
		srv, c := spellbookServer(t)
		before := playerRecord(t, srv, "Zod").Points.Armor

		castSelf(c, "invisibility")

		rec := playerRecord(t, srv, "Zod")
		if !rec.AffectFlags.Has(game.AffectInvisible) {
			t.Errorf("invisibility did not set its flag: have %v", rec.AffectFlags)
		}
		// It is forty points of armour class as well as a flag, which is the
		// part a flag check on its own would miss.
		if got, want := rec.Points.Armor, before-40; got != want {
			t.Errorf("armour class is %d, want %d", got, want)
		}
	}},

	game.SpellHolySmite: {run: func(t *testing.T) {
		srv, c := spellbookServer(t)
		before := playerRecord(t, srv, "Zod").Points

		castSelf(c, "holy smite")

		got := playerRecord(t, srv, "Zod").Points
		if got.HitRoll != before.HitRoll+10 || got.DamRoll != before.DamRoll+10 {
			t.Errorf("hitroll/damroll are %d/%d, want %d/%d",
				got.HitRoll, got.DamRoll, before.HitRoll+10, before.DamRoll+10)
		}
	}},

	game.SpellStrength: {run: func(t *testing.T) {
		// Cast at the mobile, not the caster: spell_strength refuses
		// silently for anybody already at 18/00, which every implementor is.
		srv, c := spellbookServer(t)
		dog := prey(t, srv, ImmortStartRoom)
		before := record(t, srv, dog).Abilities.Strength

		castOn(c, "strength", "dog")

		if got := record(t, srv, dog).Abilities.Strength; got <= before {
			t.Errorf("the dog's strength is %d, was %d", got, before)
		}
	}},

	// -- mag_affects, on somebody else ---------------------------------

	game.SpellBlindness: affectsPrey("blindness", game.AffectBlind),
	game.SpellCurse:     affectsPrey("curse", game.AffectCurse),
	game.SpellPoison:    affectsPrey("poison", game.AffectPoison),
	game.SpellSilence:   affectsPrey("silence", game.AffectSilence),

	game.SpellSleep: {run: func(t *testing.T) {
		srv, c := spellbookServer(t)
		dog := prey(t, srv, ImmortStartRoom)

		castOn(c, "sleep", "dog")

		var position game.Position
		var flags game.Flags
		inWorld(t, srv, func(_ *game.Live) {
			position, flags = dog.Position, dog.Record.AffectFlags
		})
		if !flags.Has(game.AffectSleep) {
			t.Errorf("sleep did not set its flag: have %v", flags)
		}
		// The flag is only half of it: mag_affects sets the position too,
		// which is what actually takes the victim out of the fight.
		if position != game.PosSleeping {
			t.Errorf("the dog is %v, want sleeping", position)
		}
	}},

	game.SpellChillTouch: {run: func(t *testing.T) {
		// Both routines at once: MAG_DAMAGE and MAG_AFFECTS.
		srv, c := spellbookServer(t)
		dog := prey(t, srv, ImmortStartRoom)
		before := record(t, srv, dog)

		castOn(c, "chill touch", "dog")

		after := record(t, srv, dog)
		if after.Points.Hit >= before.Points.Hit {
			t.Errorf("the dog is on %d hit points, was %d", after.Points.Hit, before.Points.Hit)
		}
		if after.Abilities.Strength >= before.Abilities.Strength {
			t.Errorf("the dog's strength is %d, was %d — it should wither",
				after.Abilities.Strength, before.Abilities.Strength)
		}
	}},

	// -- mag_damage ----------------------------------------------------

	game.SpellBurningHands:  damages("burning hands"),
	game.SpellCallLightning: damages("call lightning"),
	game.SpellColorSpray:    damages("color spray"),
	game.SpellEnergyDrain:   damages("energy drain"),
	game.SpellFireball:      damages("fireball"),
	game.SpellHarm:          damages("harm"),
	game.SpellLightningBolt: damages("lightning bolt"),
	game.SpellMagicMissile:  damages("magic missile"),
	game.SpellShockingGrasp: damages("shocking grasp"),
	game.SpellOuchie:        damages("ouchie"),
	game.SpellImmolate:      damages("immolate"),

	game.SpellDispelEvil: {run: func(t *testing.T) {
		// Neutral caster, evil victim: the branch that does damage. The
		// other two branches — a caster of the spell's own alignment taking
		// it themselves, and a victim of the opposite one being protected —
		// are what game.Dispel's own tests cover.
		srv, c := spellbookServer(t)
		dog := prey(t, srv, ImmortStartRoom)
		alignments(t, srv, "Zod", 0, dog, -1000)
		before := record(t, srv, dog).Points.Hit

		castOn(c, "dispel evil", "dog")

		if got := record(t, srv, dog).Points.Hit; got >= before {
			t.Errorf("the dog is on %d hit points, was %d", got, before)
		}
	}},

	game.SpellDispelGood: {run: func(t *testing.T) {
		// The mirror: a good victim is the one dispel good may hit, and an
		// evil one is protected from it.
		srv, c := spellbookServer(t)
		dog := prey(t, srv, ImmortStartRoom)
		alignments(t, srv, "Zod", 0, dog, 1000)
		before := record(t, srv, dog).Points.Hit

		castOn(c, "dispel good", "dog")

		if got := record(t, srv, dog).Points.Hit; got >= before {
			t.Errorf("the dog is on %d hit points, was %d", got, before)
		}
	}},

	game.SpellEarthquake: {run: func(t *testing.T) {
		// mag_areas: no target at all, and everybody else in the room takes
		// it. The caster is skipped, which is the only reason an immortal
		// can be the one casting.
		srv, c := spellbookServer(t)
		dog := prey(t, srv, ImmortStartRoom)
		before := record(t, srv, dog).Points.Hit

		castSelf(c, "earthquake")

		if got := record(t, srv, dog).Points.Hit; got >= before {
			t.Errorf("the dog is on %d hit points, was %d", got, before)
		}
		if !c.seen("the earth begins to shake all around you!") {
			t.Errorf("earthquake said nothing; the transcript was:\n%s", c.transcript())
		}
	}},

	// -- mag_points ----------------------------------------------------

	game.SpellCureLight:  heals("cure light"),
	game.SpellCureCritic: heals("cure critic"),
	game.SpellHeal:       heals("heal"),
	game.SpellFullHeal: {run: func(t *testing.T) {
		// The local one, and the only healing spell defined by what is
		// missing rather than by a roll — so it can be asserted exactly.
		srv, c := spellbookServer(t)
		dog := prey(t, srv, ImmortStartRoom)
		hurt(t, srv, dog, 50)

		castOn(c, "full heal", "dog")

		rec := record(t, srv, dog)
		if rec.Points.Hit != rec.Points.MaxHit {
			t.Errorf("the dog is on %d of %d hit points", rec.Points.Hit, rec.Points.MaxHit)
		}
	}},

	// -- mag_unaffects -------------------------------------------------

	// All three are the same bug: mag_unaffects is handed the cure's own
	// spell number instead of the affliction's, so it removes affects of a
	// type nothing ever applies. The C maps one to the other first
	// (magic.c:910-929).

	game.SpellCureBlind: {
		run: func(t *testing.T) {
			srv, c := spellbookServer(t)
			dog := prey(t, srv, ImmortStartRoom)

			castOn(c, "blindness", "dog")
			if !record(t, srv, dog).AffectFlags.Has(game.AffectBlind) {
				t.Fatal("the dog was not blinded to begin with")
			}

			castOn(c, "cure blind", "dog")

			if record(t, srv, dog).AffectFlags.Has(game.AffectBlind) {
				t.Error("cure blind left the dog blind")
			}
			// The caster reads the room's line, not the victim's: act's
			// to_room is the one everybody but the victim gets.
			if !c.seen("There's a momentary gleam in a large dog's eyes.") {
				t.Errorf("no room line for cure blind; the transcript was:\n%s", c.transcript())
			}
		},
	},

	game.SpellRemovePoison: {
		run: func(t *testing.T) {
			srv, c := spellbookServer(t)
			dog := prey(t, srv, ImmortStartRoom)

			castOn(c, "poison", "dog")
			if !record(t, srv, dog).AffectFlags.Has(game.AffectPoison) {
				t.Fatal("the dog was not poisoned to begin with")
			}

			castOn(c, "remove poison", "dog")

			if record(t, srv, dog).AffectFlags.Has(game.AffectPoison) {
				t.Error("remove poison left the dog poisoned")
			}
			if !c.seen("A large dog looks better.") {
				t.Errorf("no room line for remove poison; the transcript was:\n%s", c.transcript())
			}
		},
	},

	game.SpellRemoveCurse: {
		// The object half of remove curse is mag_alter_objs and works;
		// objspells_test.go covers it. This is the character half.
		run: func(t *testing.T) {
			srv, c := spellbookServer(t)
			dog := prey(t, srv, ImmortStartRoom)

			castOn(c, "curse", "dog")
			if !record(t, srv, dog).AffectFlags.Has(game.AffectCurse) {
				t.Fatal("the dog was not cursed to begin with")
			}

			castOn(c, "remove curse", "dog")

			if record(t, srv, dog).AffectFlags.Has(game.AffectCurse) {
				t.Error("remove curse left the dog cursed")
			}
		},
	},

	// -- mag_alter_objs ------------------------------------------------

	game.SpellEnchantWeapon: {run: func(t *testing.T) {
		srv, c := spellbookServer(t)
		sword := giveTo(t, srv, "Zod", testSwordVnum)

		castOn(c, "enchant weapon", "sword")

		var magic bool
		var affects int
		inWorld(t, srv, func(_ *game.Live) {
			magic = sword.ExtraFlags.Has(game.ItemMagic)
			for _, a := range sword.Affects {
				if a.Location != game.ApplyNone {
					affects++
				}
			}
		})
		if !magic {
			t.Error("the sword was not made magical")
		}
		if affects == 0 {
			t.Error("the sword gained no applies")
		}
	}},

	// -- mag_creations -------------------------------------------------

	game.SpellCreateFood: {run: func(t *testing.T) {
		srv, c := spellbookServer(t)

		castSelf(c, "create food")

		var food int
		inWorld(t, srv, func(w *game.Live) {
			if who := w.Find("Zod"); who != nil {
				for _, obj := range who.Carrying {
					if obj.Type == game.ItemFood {
						food++
					}
				}
			}
		})
		if food == 0 {
			t.Errorf("nothing edible was created; the transcript was:\n%s", c.transcript())
		}
	}},

	// -- mag_summons ---------------------------------------------------

	game.SpellClone: {run: func(t *testing.T) {
		// Clone is TAR_SELF_ONLY *and nothing else*, and do_cast's
		// empty-argument fallback only defaults to the caster for
		// TAR_CHAR_ROOM (spell_parser.c's find-the-target block). So the C
		// cannot cast it either, from either form, and the outcome under
		// test is the refusal rather than a clone.
		srv, c := spellbookServer(t)
		_ = srv

		c.send("cast 'clone'")
		c.expect("Upon who should the spell be cast?")

		c.send("cast 'clone' zod")
		c.expect("Cannot find the target of your spell!")
	}},

	game.SpellAnimateDead: {run: func(t *testing.T) {
		srv, c := spellbookServer(t)
		dog := prey(t, srv, ImmortStartRoom)

		// Something to animate. immolate is a thousand points, which is more
		// than anything in this suite survives.
		castOn(c, "immolate", "dog")
		var dead bool
		inWorld(t, srv, func(_ *game.Live) { dead = dog.Record.Points.Hit <= 0 })
		if !dead {
			t.Fatalf("the dog survived; the transcript was:\n%s", c.transcript())
		}

		// One time in ten it fails, and a failure leaves the corpse alone, so
		// the retry is safe.
		for try := 0; try < 12; try++ {
			castOn(c, "animate dead", "corpse")
			if c.seen("animates a corpse!") {
				break
			}
		}

		var zombies int
		inWorld(t, srv, func(w *game.Live) {
			for _, who := range w.Occupants(ImmortStartRoom) {
				if who.IsNPC() && who.Record != nil && who.Record.AffectFlags.Has(game.AffectCharm) {
					zombies++
				}
			}
		})
		if zombies == 0 {
			t.Errorf("no zombie was raised; the transcript was:\n%s", c.transcript())
		}
	}},

	// -- mag_groups ----------------------------------------------------

	game.SpellGroupHeal: {run: func(t *testing.T) {
		srv, c := spellbookServer(t)
		bob, _ := place(t, srv, fighterRecord("Bob", 30, 100), ImmortStartRoom)
		hurt(t, srv, bob, 10)
		inWorld(t, srv, func(w *game.Live) {
			w.AddFollower(bob, w.Find("Zod"))
			bob.SetGrouped(true)
			w.Find("Zod").SetGrouped(true)
		})

		castSelf(c, "group heal")

		if got := record(t, srv, bob).Points.Hit; got <= 10 {
			t.Errorf("Bob is on %d hit points and was not healed", got)
		}
	}},

	game.SpellGroupArmor: {run: func(t *testing.T) {
		srv, c := spellbookServer(t)
		bob, _ := place(t, srv, fighterRecord("Bob", 30, 100), ImmortStartRoom)
		inWorld(t, srv, func(w *game.Live) {
			w.AddFollower(bob, w.Find("Zod"))
			bob.SetGrouped(true)
			w.Find("Zod").SetGrouped(true)
		})
		before := record(t, srv, bob).Points.Armor

		castSelf(c, "group armor")

		if got, want := record(t, srv, bob).Points.Armor, before-20; got != want {
			t.Errorf("Bob's armour class is %d, want %d", got, want)
		}
	}},

	// -- the manual spells ---------------------------------------------

	game.SpellCharm: {run: func(t *testing.T) {
		srv, c := spellbookServer(t)
		dog := prey(t, srv, ImmortStartRoom)

		castOn(c, "charm person", "dog")

		var charmed bool
		var master string
		inWorld(t, srv, func(_ *game.Live) {
			charmed = dog.Record.AffectFlags.Has(game.AffectCharm)
			if dog.Master != nil {
				master = dog.Master.Name
			}
		})
		if !charmed {
			t.Errorf("the dog was not charmed; the transcript was:\n%s", c.transcript())
		}
		// Charm is a follow as well as a flag, which is the half that makes
		// it fight for you.
		if master != "Zod" {
			t.Errorf("the dog follows %q, want Zod", master)
		}
	}},

	game.SpellCreateWater: {run: func(t *testing.T) {
		srv, c := spellbookServer(t)
		jug := giveTo(t, srv, "Zod", testJugVnum)

		castOn(c, "create water", "jug")

		var contents, liquid int32
		inWorld(t, srv, func(_ *game.Live) {
			contents, liquid = jug.Values[1], jug.Values[2]
		})
		if contents <= 0 {
			t.Errorf("the jug holds %d, want some water", contents)
		}
		if liquid != game.LiquidWater {
			t.Errorf("the jug holds liquid %d, want water (%d)", liquid, game.LiquidWater)
		}
	}},

	game.SpellDetectPoison: {run: func(t *testing.T) {
		// No state to read: spell_detect_poison only ever reports. So the
		// outcome under test is that it reports the right thing, and it says
		// something different about you than about anybody else.
		srv, c := spellbookServer(t)
		dog := prey(t, srv, ImmortStartRoom)

		castSelf(c, "detect poison")
		c.expect("You feel healthy.")

		castOn(c, "poison", "dog")
		castOn(c, "detect poison", "dog")
		c.expect("You sense that it is poisoned.")
		_ = dog
	}},

	game.SpellIdentify: {run: func(t *testing.T) {
		// Not by casting: do_cast refuses anything above MAX_SPELLS, which
		// is 130, and identify is 201 (spell_parser.c's
		// `spellnum > MAX_SPELLS`). It reaches a player through an object —
		// a scroll, a wand, the identify specproc — so that is how it is
		// driven here.
		srv, c := spellbookServer(t)
		scroll := giveTo(t, srv, "Zod", testScrollVnum)
		inWorld(t, srv, func(_ *game.Live) {
			scroll.Values[1] = game.SpellIdentify
			scroll.Values[2] = 0
			scroll.Values[3] = 0
		})
		giveTo(t, srv, "Zod", testSwordVnum)

		c.send("recite scroll sword")
		c.settle()

		// The three lines identify always prints for a weapon.
		for _, want := range []string{"Object 'a long sword'", "Weight:", "Damage Dice"} {
			if !c.seen(want) {
				t.Errorf("identify did not print %q; the transcript was:\n%s", want, c.transcript())
			}
		}
	}},

	game.SpellLocateObject: {run: func(t *testing.T) {
		srv, c := spellbookServer(t)
		giveTo(t, srv, "Zod", testSwordVnum)

		castOn(c, "locate object", "sword")

		c.expect("A long sword is being carried by Zod.")
	}},

	game.SpellSummon: {run: func(t *testing.T) {
		// Only players may be summoned here — the local rule — so this needs
		// a second character rather than a mobile.
		srv, c := spellbookServer(t)
		bob, _ := place(t, srv, fighterRecord("Bob", 10, 200), MortalStartRoom)
		inWorld(t, srv, func(_ *game.Live) {
			// Summon protection is on by default and is a flat refusal.
			bob.Record.Preferences = bob.Record.Preferences.Set(game.PrefSummonable)
		})

		castOn(c, "summon", "bob")

		var room game.RoomVnum
		inWorld(t, srv, func(_ *game.Live) { room = bob.Room })
		if room != ImmortStartRoom {
			t.Errorf("Bob is in room %d, want %d; the transcript was:\n%s",
				room, ImmortStartRoom, c.transcript())
		}
	}},

	game.SpellWordOfRecall: {run: func(t *testing.T) {
		srv, c := spellbookServer(t)
		if got := roomOf(t, srv, "Zod"); got != ImmortStartRoom {
			t.Fatalf("Zod starts in room %d, want %d", got, ImmortStartRoom)
		}

		castSelf(c, "word of recall")

		if got := roomOf(t, srv, "Zod"); got != game.MortalStartRoom {
			t.Errorf("word of recall left Zod in room %d, want %d", got, game.MortalStartRoom)
		}
	}},

	game.SpellTeleport: {run: func(t *testing.T) {
		// Somewhere at random, so the assertion is that it moved rather than
		// where to: spell_teleport rerolls only for private rooms, death
		// traps and god rooms, and may legitimately land you back where you
		// started.
		srv, c := spellbookServer(t)
		start := roomOf(t, srv, "Zod")

		for try := 0; try < 10; try++ {
			castSelf(c, "teleport")
			if roomOf(t, srv, "Zod") != start {
				return
			}
		}
		t.Errorf("ten teleports left Zod in room %d; the transcript was:\n%s",
			start, c.transcript())
	}},

	game.SpellDispelMagic: {run: func(t *testing.T) {
		// Local, and blunt: every affect comes off, including the caster's
		// own blessings.
		srv, c := spellbookServer(t)
		castSelf(c, "armor")
		castSelf(c, "sanctuary")
		if rec := playerRecord(t, srv, "Zod"); len(rec.Affects) < 2 {
			t.Fatalf("only %d affects to dispel", len(rec.Affects))
		}

		castSelf(c, "dispel magic")

		rec := playerRecord(t, srv, "Zod")
		if len(rec.Affects) != 0 {
			t.Errorf("%d affects survived dispel magic", len(rec.Affects))
		}
		if rec.AffectFlags.Has(game.AffectSanctuary) {
			t.Error("sanctuary survived dispel magic")
		}
	}},

	game.SpellControlWeather: {
		skip: "control weather has no case in castManual and reports itself unimplemented (#300)",
	},

	// -- the skills ----------------------------------------------------

	game.SkillKick: {run: func(t *testing.T) {
		srv, c := spellbookServer(t)
		dog := prey(t, srv, ImmortStartRoom)
		before := record(t, srv, dog).Points.Hit

		useSkill(c, "kick dog")

		if got := record(t, srv, dog).Points.Hit; got >= before {
			t.Errorf("the dog is on %d hit points, was %d; the transcript was:\n%s",
				got, before, c.transcript())
		}
	}},

	game.SkillBash: {run: func(t *testing.T) {
		// Bash needs a weapon in hand, and its outcome is a position rather
		// than damage: the victim ends up sitting.
		srv, c := spellbookServer(t)
		dog := prey(t, srv, ImmortStartRoom)
		giveTo(t, srv, "Zod", testSwordVnum)
		c.send("wield sword")
		c.expect("You wield a long sword.")

		var sat bool
		for try := 0; try < 12 && !sat; try++ {
			useSkill(c, "bash dog")
			inWorld(t, srv, func(_ *game.Live) { sat = dog.Position == game.PosSitting })
		}
		if !sat {
			t.Errorf("twelve bashes never put the dog down; the transcript was:\n%s",
				c.transcript())
		}
	}},

	game.SkillBackstab: {run: func(t *testing.T) {
		srv, c := spellbookServer(t)
		dog := prey(t, srv, ImmortStartRoom)
		sword := giveTo(t, srv, "Zod", testSwordVnum)
		// The test world's sword slashes; only a piercing weapon backstabs.
		inWorld(t, srv, func(_ *game.Live) { sword.Values[3] = game.AttackPierce })
		c.send("wield sword")
		c.expect("You wield a long sword.")

		before := record(t, srv, dog).Points.Hit

		// Retried for the same reason bash and rescue are: the skill roll is
		// `number(1, 101) > GET_SKILL(...)`, so even a perfect thief misses
		// one time in 101, and a miss is not what this case is about.
		var hit bool
		for try := 0; try < 12 && !hit; try++ {
			useSkill(c, "backstab dog")
			hit = record(t, srv, dog).Points.Hit < before
		}
		if !hit {
			t.Errorf("twelve backstabs left the dog on %d hit points; the transcript was:\n%s",
				before, c.transcript())
		}
	}},

	game.SkillRescue: {run: func(t *testing.T) {
		srv, c := spellbookServer(t)
		dog := prey(t, srv, ImmortStartRoom)
		bob, _ := place(t, srv, fighterRecord("Bob", 10, 200), ImmortStartRoom)
		inWorld(t, srv, func(w *game.Live) { w.SetFighting(dog, bob) })

		var rescued bool
		for try := 0; try < 12 && !rescued; try++ {
			useSkill(c, "rescue bob")
			inWorld(t, srv, func(w *game.Live) {
				if who := w.Find("Zod"); who != nil {
					rescued = dog.Fighting == who
				}
			})
		}
		if !rescued {
			t.Errorf("the dog is still not fighting Zod; the transcript was:\n%s",
				c.transcript())
		}
	}},

	game.SkillHide: {run: func(t *testing.T) {
		srv, c := spellbookServer(t)

		var hidden bool
		for try := 0; try < 12 && !hidden; try++ {
			useSkill(c, "hide")
			hidden = playerRecord(t, srv, "Zod").AffectFlags.Has(game.AffectHide)
		}
		if !hidden {
			t.Errorf("twelve attempts never hid anybody; the transcript was:\n%s",
				c.transcript())
		}
	}},

	game.SkillSneak: {run: func(t *testing.T) {
		srv, c := spellbookServer(t)

		var sneaking bool
		for try := 0; try < 12 && !sneaking; try++ {
			useSkill(c, "sneak")
			sneaking = playerRecord(t, srv, "Zod").AffectFlags.Has(game.AffectSneak)
		}
		if !sneaking {
			t.Errorf("twelve attempts never started anybody sneaking; the transcript was:\n%s",
				c.transcript())
		}
	}},

	game.SkillPickLock: {run: func(t *testing.T) {
		// do_gen_door looks for a container before it looks for a door, and
		// the test world's chest is closed and locked.
		srv, c := spellbookServer(t)
		chest := drop(t, srv, testChestVnum, ImmortStartRoom)

		var unlocked bool
		for try := 0; try < 12 && !unlocked; try++ {
			useSkill(c, "pick chest")
			inWorld(t, srv, func(_ *game.Live) {
				unlocked = !chest.ContainerLocked()
			})
		}
		if !unlocked {
			t.Errorf("twelve attempts never picked the chest; the transcript was:\n%s",
				c.transcript())
		}
	}},

	game.SkillSteal: {run: func(t *testing.T) {
		// Asleep, deliberately: do_steal skips the roll for a victim who is
		// not awake ("A sleeping victim's purse is taken with no roll"), so
		// what is left under test is the transfer rather than a die.
		srv, c := spellbookServer(t)
		dog := prey(t, srv, ImmortStartRoom)
		inWorld(t, srv, func(_ *game.Live) {
			dog.Record.Points.Gold = 1000
			dog.Position = game.PosSleeping
		})
		before := playerRecord(t, srv, "Zod").Points.Gold

		useSkill(c, "steal coins dog")

		after := playerRecord(t, srv, "Zod").Points.Gold
		if after <= before {
			t.Errorf("Zod has %d gold, had %d; the transcript was:\n%s",
				after, before, c.transcript())
		}
		if got := record(t, srv, dog).Points.Gold; got >= 1000 {
			t.Errorf("the dog still has %d gold", got)
		}
	}},

	game.SkillTrack: {run: func(t *testing.T) {
		// Zod in the temple, the quarry one room north. The test world's
		// rooms have a single exit each, so a failed roll picks the same
		// direction the real answer is — this asserts that track finds a
		// trail and names the way, not which of the two branches ran.
		srv, c := spellbookServer(t)
		c.send("south")
		c.expect("The Temple Of Midgaard")
		place(t, srv, fighterRecord("Bob", 10, 200), ImmortStartRoom)

		useSkill(c, "track bob")

		c.expect("You sense a trail north from here!")
		if c.seen("can't sense a trail") || c.seen("something seems to be wrong") {
			t.Errorf("track could not find a path one room away; the transcript was:\n%s",
				c.transcript())
		}
	}},

	// -- not reachable by a player -------------------------------------

	game.SpellFireBreath:      {skip: "a mobile's breath weapon: no class learns it and nothing lets a player cast it"},
	game.SpellGasBreath:       {skip: "a mobile's breath weapon, as fire breath"},
	game.SpellFrostBreath:     {skip: "a mobile's breath weapon, as fire breath"},
	game.SpellAcidBreath:      {skip: "a mobile's breath weapon, as fire breath"},
	game.SpellLightningBreath: {skip: "a mobile's breath weapon, as fire breath"},
}

// TestEverySpellAndSkillIsAccountedFor checks the table above against game's
// own, in both directions.
//
// This is the part that does not go stale. A spell added to spellTable
// without an entry here fails immediately, and an entry here for a number
// that no longer exists fails too — so the suite cannot quietly stop covering
// something, which is exactly what happened to the parity suite's `known`
// list (CLAUDE.md).
func TestEverySpellAndSkillIsAccountedFor(t *testing.T) {
	// Wide enough for the skills (131-140) and the breath weapons (201-206).
	const top = 256

	var uncovered []string
	for number := int32(1); number <= top; number++ {
		info, ok := game.Spell(number)
		if !ok {
			continue
		}
		if _, covered := spellbook[number]; !covered {
			uncovered = append(uncovered, fmt.Sprintf("%d (%s)", number, info.Name))
		}
	}
	sort.Strings(uncovered)
	if len(uncovered) > 0 {
		t.Errorf("%d spells or skills have no case in the spellbook: %s\n"+
			"Add one, or an entry with a skip saying why a player cannot reach it.",
			len(uncovered), strings.Join(uncovered, ", "))
	}

	for number := range spellbook {
		if _, ok := game.Spell(number); !ok {
			t.Errorf("the spellbook has a case for %d, which is not in the spell table", number)
		}
	}
}

// TestSpellbook is the suite itself.
func TestSpellbook(t *testing.T) {
	numbers := make([]int32, 0, len(spellbook))
	for number := range spellbook {
		numbers = append(numbers, number)
	}
	sort.Slice(numbers, func(i, j int) bool { return numbers[i] < numbers[j] })

	for _, number := range numbers {
		one := spellbook[number]
		name := game.SpellName(number)
		t.Run(fmt.Sprintf("%d_%s", number, strings.ReplaceAll(name, " ", "_")), func(t *testing.T) {
			switch {
			case one.skip != "":
				t.Skip(one.skip)
			case one.pending != "":
				t.Skip(one.pending)
			}
			one.run(t)
		})
	}
}
