// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package binary

import (
	"encoding/binary"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// Byte order is little-endian, and only little-endian.
//
// The archive investigation established that this data was written by a
// FreeBSD/i386 build; docs/investigations/circlemud-archive-report.md §5 ruled
// out the SPARC/big-endian possibility explicitly by checking that the field
// values come out sane rather than byte-swapped. A big-endian file would
// decode here as enormous negative levels and nonsense timestamps rather than
// being silently accepted, which is the behaviour to want: guessing the byte
// order from the data is how you end up confidently reading a corrupt file.
var byteOrder = binary.LittleEndian

// codec reads and writes records under one data model.
type codec struct {
	layout *layout
}

// newCodec returns a codec for the given model.
func newCodec(m dataModel) *codec { return &codec{layout: computeLayout(m)} }

// RecordSize is the on-disk size of one record.
func (c *codec) RecordSize() int { return c.layout.Size }

// --- primitive readers -------------------------------------------------
//
// Every one of these takes the field by name and reads at the offset the
// layout engine computed, rather than at a constant written out here. That is
// deliberate: it means the 32-bit and 64-bit formats are read by the same
// code, and there is exactly one place — the layout — where an offset can be
// wrong.

func (c *codec) i8(rec []byte, name string) int32 {
	return widen8(rec[c.layout.at(name).Offset])
}

func (c *codec) u8(rec []byte, name string) int32 {
	return int32(rec[c.layout.at(name).Offset])
}

func (c *codec) i16(rec []byte, name string) int32 {
	p := c.layout.at(name)
	return widen16(byteOrder.Uint16(rec[p.Offset:]))
}

func (c *codec) i32(rec []byte, name string) int32 {
	p := c.layout.at(name)
	return widen32(byteOrder.Uint32(rec[p.Offset:]))
}

// varInt reads a field whose width depends on the data model — `long` and
// `time_t`. Returning int64 regardless is the point: the caller never has to
// know which model produced the file.
// varBits reads a variable-width field that is a *bitvector* rather than a
// number: `bitvector_t` is `unsigned long` (structs.h:599), so the top bit
// is a flag and not a sign.
//
// This is varInt's counterpart and exists because using varInt for these
// was wrong in a way nothing noticed for the whole port. varInt widens a
// 4-byte field through int32, so a stored 0xFFFFFFFF -- every player flag
// set, which is what a 32-bit `unsigned long` full of flags looks like --
// sign-extended to -1 and then reinterpreted as game.Flags, which is 64
// bits, giving all sixty-four bits set. flagsOf's own doc comment already
// said "a value with the top bit set is a flag, not a negative number";
// the value had been through int32 before flagsOf ever saw it.
//
// It was invisible because no fixture had a flag above bit 30 in it until
// examples/torture (docs/proposals/yaml-only.md §5.1), and because it is
// symmetric on write -- the value round-trips through this package
// unchanged -- so the package's own byte-for-byte round-trip test could
// not see it either. What sees it is comparing the decoded record against
// what another format decoded, which is `dlctl verify --against`.
func (c *codec) varBits(rec []byte, name string) game.Flags {
	p := c.layout.at(name)
	switch p.Size {
	case 4:
		return game.Flags(byteOrder.Uint32(rec[p.Offset:]))
	case 8:
		return game.Flags(byteOrder.Uint64(rec[p.Offset:]))
	}
	panic(fmt.Sprintf("binary: %s has unexpected width %d", name, p.Size))
}

func (c *codec) varInt(rec []byte, name string) int64 {
	p := c.layout.at(name)
	switch p.Size {
	case 4:
		return int64(widen32(byteOrder.Uint32(rec[p.Offset:])))
	case 8:
		return widen64(byteOrder.Uint64(rec[p.Offset:]))
	}
	panic(fmt.Sprintf("binary: %s has unexpected width %d", name, p.Size))
}

// str reads a fixed-width NUL-terminated character array.
//
// The C code writes these with strcpy into a fixed buffer, so the bytes after
// the terminator are whatever was in the struct before — often the tail of a
// previous player's name. Everything from the first NUL on is therefore
// dropped rather than trusted.
func (c *codec) str(rec []byte, name string) string {
	p := c.layout.at(name)
	b := rec[p.Offset : p.Offset+p.Size]
	if i := indexByte(b, 0); i >= 0 {
		b = b[:i]
	}
	return string(b)
}

func indexByte(b []byte, c byte) int {
	for i, x := range b {
		if x == c {
			return i
		}
	}
	return -1
}

// --- widening --------------------------------------------------------
//
// The mirror of the narrowing helpers below: on-disk values are unsigned
// bytes and words that have to be reinterpreted as the signed types the
// struct declares. Each of these is a reinterpretation, not a range change,
// and doing them in named functions keeps the reasoning in one place.

func widen8(b byte) int32 { return int32(int8(b)) } //nolint:gosec // reinterpretation, not truncation

func widen16(u uint16) int32 { return int32(int16(u)) } //nolint:gosec // reinterpretation, not truncation

func widen32(u uint32) int32 { return int32(u) } //nolint:gosec // reinterpretation, not truncation

func widen64(u uint64) int64 { return int64(u) } //nolint:gosec // reinterpretation, not truncation

// --- record decoding ---------------------------------------------------

// decode converts one on-disk record into the canonical model.
func (c *codec) decode(rec []byte) (*game.PlayerRecord, error) {
	if len(rec) != c.layout.Size {
		return nil, fmt.Errorf("record is %d bytes, want %d", len(rec), c.layout.Size)
	}

	p := &game.PlayerRecord{
		Name:        c.str(rec, "name"),
		Title:       c.str(rec, "title"),
		Description: c.str(rec, "description"),
		Host:        c.str(rec, "host"),

		Sex:      c.i8(rec, "sex"),
		Class:    c.i8(rec, "chclass"),
		Level:    c.i8(rec, "level"),
		Hometown: c.i16(rec, "hometown"),
		Weight:   c.u8(rec, "weight"),
		Height:   c.u8(rec, "height"),

		Birth:     c.timestamp(rec, "birth"),
		LastLogon: c.timestamp(rec, "last_logon"),
		Played:    time.Duration(c.i32(rec, "played")) * time.Second,

		Alignment: c.i32(rec, "cs.alignment"),
		IDNum:     c.varInt(rec, "cs.idnum"),

		PlayerFlags: c.varBits(rec, "cs.act"),
		AffectFlags: c.varBits(rec, "cs.affected_by"),
		Preferences: c.varBits(rec, "ps.pref"),

		WimpLevel:     c.i32(rec, "ps.wimp_level"),
		FreezeLevel:   c.i8(rec, "ps.freeze_level"),
		InvisLevel:    c.i16(rec, "ps.invis_level"),
		LoadRoom:      game.RoomVnum(c.i16(rec, "ps.load_room")),
		BadPasswords:  c.u8(rec, "ps.bad_pws"),
		SpellsToLearn: c.i32(rec, "ps.spells_to_learn"),
		RemortVector:  c.i32(rec, "ps.remort_vector"),
		SpecFlags:     c.i32(rec, "ps.specflags"),
		OLCZone:       c.i32(rec, "ps.olc_zone"),
	}

	// The stored password is a DES crypt(3) hash, or empty if never set.
	if pw := c.str(rec, "pwd"); pw != "" {
		p.Credential = game.Credential{Scheme: game.SchemeLegacyDES, Hash: pw}
	}

	p.Abilities = game.Abilities{
		Strength:           c.i8(rec, "ab.str"),
		StrengthPercentile: c.i8(rec, "ab.str_add"),
		Intelligence:       c.i8(rec, "ab.intel"),
		Wisdom:             c.i8(rec, "ab.wis"),
		Dexterity:          c.i8(rec, "ab.dex"),
		Constitution:       c.i8(rec, "ab.con"),
		Charisma:           c.i8(rec, "ab.cha"),
	}

	p.Points = game.Points{
		Mana: c.i16(rec, "pt.mana"), MaxMana: c.i16(rec, "pt.max_mana"),
		Hit: c.i16(rec, "pt.hit"), MaxHit: c.i16(rec, "pt.max_hit"),
		Move: c.i16(rec, "pt.move"), MaxMove: c.i16(rec, "pt.max_move"),
		Armor:    c.i16(rec, "pt.armor"),
		Gold:     c.i32(rec, "pt.gold"),
		BankGold: c.i32(rec, "pt.bank_gold"),
		Exp:      c.i32(rec, "pt.exp"),
		HitRoll:  c.i8(rec, "pt.hitroll"),
		DamRoll:  c.i8(rec, "pt.damroll"),
	}

	saves := c.layout.at("cs.apply_saving_throw")
	for i := range p.SavingThrows {
		p.SavingThrows[i] = widen16(byteOrder.Uint16(rec[saves.Offset+i*2:]))
	}

	conds := c.layout.at("ps.conditions")
	for i := range p.Conditions {
		p.Conditions[i] = widen8(rec[conds.Offset+i])
	}

	// Skills: 201 bytes, index 0 unused because spell numbers start at 1.
	// Only the non-zero entries are kept — a character who has practised
	// nothing carries an empty map rather than 200 zeroes.
	skills := c.layout.at("ps.skills")
	for i := 0; i < skills.Size; i++ {
		if v := widen8(rec[skills.Offset+i]); v != 0 {
			if p.Skills == nil {
				p.Skills = make(map[int32]int32)
			}
			p.Skills[int32(i)] = v
		}
	}

	// Affects: a fixed array of 32 slots, unused ones zeroed. A slot with no
	// spell type is empty; the C code tests the same way.
	aff := c.layout.at("affected")
	for i := 0; i < maxAffect; i++ {
		base := aff.Offset + i*aff.Stride
		typ := widen16(byteOrder.Uint16(rec[base:]))
		if typ == 0 {
			continue
		}
		bitsOff := c.layout.at("affected.bitvector").Offset - aff.Offset
		bitsSize := c.layout.at("affected.bitvector").Size
		var bits uint64
		if bitsSize == 4 {
			bits = uint64(byteOrder.Uint32(rec[base+bitsOff:]))
		} else {
			bits = byteOrder.Uint64(rec[base+bitsOff:])
		}
		p.Affects = append(p.Affects, game.Affect{
			Type:     typ,
			Duration: widen16(byteOrder.Uint16(rec[base+2:])),
			Modifier: widen8(rec[base+4]),
			Location: int32(rec[base+5]),
			Bits:     game.Flags(bits),
		})
	}

	// As ascii's Decode and yaml's recordFromDoc: char_file_u holds
	// real_abils and the unaffected points, so the decoded record is the
	// base every affect is applied to. See game.SnapshotReal.
	game.SnapshotReal(p)
	return p, nil
}

// spareFields are char_file_u's reserved slots, by layout name.
//
// The C server's own documentation tells people to use these when adding a
// field, so a value in one is not necessarily junk — Disgracelands used
// three of them, and those three (the remort vector, the spec flags and
// the OLC zone) are named fields on game.PlayerRecord. What is left is
// padding, and it is a property of *the stored record* rather than of the
// character: nothing in the game can read or set it, and the only reason
// to carry it is so that rewriting a record does not quietly discard
// whatever is in it.
//
// It used to live on game.PlayerRecord as a LegacySpares field — literally
// char_file_u's padding, in the canonical, format-neutral model whose
// entire reason for existing is that no format's idiosyncrasy leaks into
// it (docs/proposals/yaml-only.md §1). It is here now, and Store.Save
// carries the bytes across from the record it is replacing, which is both
// simpler and more honest: the spares belong to the file.
var spareFields = struct{ bytes, ints, longs []string }{
	bytes: []string{"ps.spare0", "ps.spare1", "ps.spare2", "ps.spare3", "ps.spare4", "ps.spare5"},
	ints:  []string{"ps.spare10", "ps.spare11", "ps.spare12", "ps.spare13", "ps.spare14", "ps.spare15", "ps.spare16"},
	longs: []string{"ps.spare17", "ps.spare18", "ps.spare19", "ps.spare20", "ps.spare21"},
}

// legacySpares is one record's reserved slots.
type legacySpares struct {
	Bytes [6]int32
	Ints  [7]int32
	Longs [5]int64
}

func (c *codec) decodeSpares(rec []byte) legacySpares {
	var s legacySpares
	for i, name := range spareFields.bytes {
		s.Bytes[i] = c.u8(rec, name)
	}
	for i, name := range spareFields.ints {
		s.Ints[i] = c.i32(rec, name)
	}
	for i, name := range spareFields.longs {
		s.Longs[i] = c.varInt(rec, name)
	}
	return s
}

// timestamp converts a stored time_t.
//
// A zero is "never", not 1 January 1970: the C code writes 0 into these
// fields for a character who has never logged in, and turning that into a
// 1970 date would put fictional history into the record.
func (c *codec) timestamp(rec []byte, name string) time.Time {
	v := c.varInt(rec, name)
	if v == 0 {
		return time.Time{}
	}
	return time.Unix(v, 0).UTC()
}

// --- narrowing -------------------------------------------------------
//
// The canonical model holds every numeric field wider than this format
// stores it: a level is int32 here and a single byte on disk. Converting
// between them is a deliberate truncation, and it belongs in one clearly
// named place rather than being scattered as casts through the encoder,
// where each one would be an opportunity to pick the wrong width.
//
// Values that cannot fit are the caller's problem, not these functions': the
// encoder validates names, credentials, skill numbers and affect counts
// before it gets here, and the fields these handle are ones the game itself
// keeps in range.

func narrow8(v int32) byte { return byte(int8(v)) } //nolint:gosec // truncation is the format

func narrowU8(v int32) byte { return byte(v) } //nolint:gosec // truncation is the format

func narrow16(v int32) uint16 { return uint16(int16(v)) } //nolint:gosec // truncation is the format

func narrow32(v int32) uint32 { return uint32(v) } //nolint:gosec // truncation is the format

func narrowFlags32(f game.Flags) uint32 { return uint32(f) } //nolint:gosec // truncation is the format

// storedFlags reinterprets a flag set as the signed type putVar takes.
func storedFlags(f game.Flags) int64 { return int64(f) } //nolint:gosec // reinterpretation, not truncation

func narrowVar32(v int64) uint32 { return uint32(int32(v)) } //nolint:gosec // truncation is the format

// --- record encoding ---------------------------------------------------

// encode converts a canonical record back to on-disk form.
//
// Writing this format is deliberately supported: the acceptance test for this
// package is that all the archived records survive a decode/encode round trip
// byte for byte, and that is only checkable if encoding exists.
func (c *codec) encode(p *game.PlayerRecord) ([]byte, error) {
	rec := make([]byte, c.layout.Size)

	if err := c.putStr(rec, "name", p.Name); err != nil {
		return nil, err
	}
	if err := c.putStr(rec, "title", p.Title); err != nil {
		return nil, err
	}
	if err := c.putStr(rec, "description", p.Description); err != nil {
		return nil, err
	}
	if err := c.putStr(rec, "host", p.Host); err != nil {
		return nil, err
	}
	if p.Credential.Scheme != game.SchemeNone && p.Credential.Scheme != game.SchemeLegacyDES {
		return nil, fmt.Errorf("cannot store a %s credential in the binary format, which only holds crypt(3) hashes",
			p.Credential.Scheme)
	}
	if err := c.putStr(rec, "pwd", p.Credential.Hash); err != nil {
		return nil, err
	}

	c.putI8(rec, "sex", p.Sex)
	c.putI8(rec, "chclass", p.Class)
	c.putI8(rec, "level", p.Level)
	c.putI16(rec, "hometown", p.Hometown)
	c.putU8(rec, "weight", p.Weight)
	c.putU8(rec, "height", p.Height)

	if err := c.putTime(rec, "birth", p.Birth); err != nil {
		return nil, err
	}
	if err := c.putTime(rec, "last_logon", p.LastLogon); err != nil {
		return nil, err
	}
	c.putI32(rec, "played", int32(p.Played/time.Second)) //nolint:gosec // a play time over 68 years is not a real record

	c.putI32(rec, "cs.alignment", p.Alignment)
	c.putVar(rec, "cs.idnum", p.IDNum)
	c.putVar(rec, "cs.act", storedFlags(p.PlayerFlags))
	c.putVar(rec, "cs.affected_by", storedFlags(p.AffectFlags))
	c.putVar(rec, "ps.pref", storedFlags(p.Preferences))

	c.putI32(rec, "ps.wimp_level", p.WimpLevel)
	c.putI8(rec, "ps.freeze_level", p.FreezeLevel)
	c.putI16(rec, "ps.invis_level", p.InvisLevel)
	c.putI16(rec, "ps.load_room", int32(p.LoadRoom))
	c.putU8(rec, "ps.bad_pws", p.BadPasswords)
	c.putI32(rec, "ps.spells_to_learn", p.SpellsToLearn)
	c.putI32(rec, "ps.remort_vector", p.RemortVector)
	c.putI32(rec, "ps.specflags", p.SpecFlags)
	c.putI32(rec, "ps.olc_zone", p.OLCZone)

	for name, v := range map[string]int32{
		"ab.str": p.Abilities.Strength, "ab.str_add": p.Abilities.StrengthPercentile,
		"ab.intel": p.Abilities.Intelligence, "ab.wis": p.Abilities.Wisdom,
		"ab.dex": p.Abilities.Dexterity, "ab.con": p.Abilities.Constitution,
		"ab.cha": p.Abilities.Charisma,
	} {
		c.putI8(rec, name, v)
	}

	for name, v := range map[string]int32{
		"pt.mana": p.Points.Mana, "pt.max_mana": p.Points.MaxMana,
		"pt.hit": p.Points.Hit, "pt.max_hit": p.Points.MaxHit,
		"pt.move": p.Points.Move, "pt.max_move": p.Points.MaxMove,
		"pt.armor": p.Points.Armor,
	} {
		c.putI16(rec, name, v)
	}
	c.putI32(rec, "pt.gold", p.Points.Gold)
	c.putI32(rec, "pt.bank_gold", p.Points.BankGold)
	c.putI32(rec, "pt.exp", p.Points.Exp)
	c.putI8(rec, "pt.hitroll", p.Points.HitRoll)
	c.putI8(rec, "pt.damroll", p.Points.DamRoll)

	saves := c.layout.at("cs.apply_saving_throw")
	for i, v := range p.SavingThrows {
		byteOrder.PutUint16(rec[saves.Offset+i*2:], narrow16(v))
	}

	conds := c.layout.at("ps.conditions")
	for i, v := range p.Conditions {
		rec[conds.Offset+i] = narrow8(v)
	}

	skills := c.layout.at("ps.skills")
	for num, pct := range p.Skills {
		if num < 0 || int(num) >= skills.Size {
			return nil, fmt.Errorf("skill %d is outside the %d slots this format has", num, skills.Size)
		}
		rec[skills.Offset+int(num)] = narrow8(pct)
	}

	if len(p.Affects) > maxAffect {
		return nil, fmt.Errorf("%d affects, but this format has %d slots", len(p.Affects), maxAffect)
	}
	aff := c.layout.at("affected")
	bitsOff := c.layout.at("affected.bitvector").Offset - aff.Offset
	bitsSize := c.layout.at("affected.bitvector").Size
	for i, a := range p.Affects {
		base := aff.Offset + i*aff.Stride
		byteOrder.PutUint16(rec[base:], narrow16(a.Type))
		byteOrder.PutUint16(rec[base+2:], narrow16(a.Duration))
		rec[base+4] = narrow8(a.Modifier)
		rec[base+5] = narrowU8(a.Location)
		if bitsSize == 4 {
			byteOrder.PutUint32(rec[base+bitsOff:], narrowFlags32(a.Bits))
		} else {
			byteOrder.PutUint64(rec[base+bitsOff:], uint64(a.Bits))
		}
	}

	return rec, nil
}

// encodeSpares writes reserved slots into an already-encoded record. Only
// Store.Save calls it, with the values it read out of the record it is
// replacing.
func (c *codec) encodeSpares(rec []byte, s legacySpares) {
	for i, name := range spareFields.bytes {
		c.putU8(rec, name, s.Bytes[i])
	}
	for i, name := range spareFields.ints {
		c.putI32(rec, name, s.Ints[i])
	}
	for i, name := range spareFields.longs {
		c.putVar(rec, name, s.Longs[i])
	}
}

func (c *codec) putStr(rec []byte, name, v string) error {
	p := c.layout.at(name)
	// The field must hold the string and its terminator. Silently truncating
	// a name would produce a different character; refusing is the only honest
	// option, and the capability report warns before it gets this far.
	if len(v) >= p.Size {
		return fmt.Errorf("%s is %d bytes, but this format's %s field holds %d including its terminator",
			strings.TrimSpace(v[:min(len(v), 24)]), len(v), name, p.Size)
	}
	copy(rec[p.Offset:p.Offset+p.Size], v)
	rec[p.Offset+len(v)] = 0
	return nil
}

func (c *codec) putI8(rec []byte, name string, v int32) {
	rec[c.layout.at(name).Offset] = narrow8(v)
}

func (c *codec) putU8(rec []byte, name string, v int32) {
	rec[c.layout.at(name).Offset] = narrowU8(v)
}

func (c *codec) putI16(rec []byte, name string, v int32) {
	byteOrder.PutUint16(rec[c.layout.at(name).Offset:], narrow16(v))
}

func (c *codec) putI32(rec []byte, name string, v int32) {
	byteOrder.PutUint32(rec[c.layout.at(name).Offset:], narrow32(v))
}

func (c *codec) putVar(rec []byte, name string, v int64) {
	p := c.layout.at(name)
	if p.Size == 4 {
		byteOrder.PutUint32(rec[p.Offset:], narrowVar32(v))
		return
	}
	byteOrder.PutUint64(rec[p.Offset:], widenU64(v))
}

// widenU64 reinterprets a signed value as the unsigned word the format
// stores. These fields are bitvectors and identifiers, not quantities.
func widenU64(v int64) uint64 { return uint64(v) } //nolint:gosec // reinterpretation, not truncation

// putTime writes a timestamp, refusing rather than wrapping when it does not
// fit.
//
// Under the 32-bit model these fields are four bytes, so they overflow in
// January 2038. That is a property of the format, not of this code, and it
// cannot be fixed while remaining compatible — so the only choice is between
// silently writing a date in 1901 and refusing. Refusing means a server still
// writing this format in 2038 stops rather than corrupting its roster.
func (c *codec) putTime(rec []byte, name string, t time.Time) error {
	if t.IsZero() {
		c.putVar(rec, name, 0)
		return nil
	}
	secs := t.Unix()
	if c.layout.at(name).Size == 4 && (secs > math.MaxInt32 || secs < math.MinInt32) {
		return fmt.Errorf("%s is %s, which does not fit in this format's 32-bit timestamp (it overflows on 2038-01-19)",
			name, t.Format(time.RFC3339))
	}
	c.putVar(rec, name, secs)
	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
