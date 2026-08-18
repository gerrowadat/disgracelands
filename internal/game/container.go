// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import "strings"

// Containers.
//
// A container keeps its whole state in the four object values rather than in
// fields of its own, and the C reads them through GET_OBJ_VAL(obj, N) at every
// use — so what value 1 means depends entirely on the object's type, and
// nothing checks. The names below are the whole point of this file: a
// container's capacity is value 0 and its lock state is value 1, and writing
// that down once is better than writing `Values[1]` in six places and hoping.

// Container flags, from structs.h:420. These live in value 1, not in the
// object's extra flags, which is why a container's "closed" bit and its
// "glowing" bit are stored in completely different places.
const (
	ContCloseable Flags = 1 << 0
	ContPickproof Flags = 1 << 1
	ContClosed    Flags = 1 << 2
	ContLocked    Flags = 1 << 3
)

// The value slots a container uses.
const (
	// containerCapacity is value 0: the weight it will hold. The C compares
	// the container's *total* weight against this, and a container's total
	// weight includes its own — so a bag that weighs 5 with a capacity of
	// 100 holds 95. See docs/weirdnumbers.md.
	containerCapacity = 0
	// containerFlagsValue is value 1: the ContClosed/ContLocked bitfield.
	containerFlagsValue = 1
	// containerKeyValue is value 2: the vnum of the key that opens it.
	containerKeyValue = 2
	// containerCorpseValue is value 3: -1 marks a corpse, which is why
	// corpses are containers with a strange third value.
	containerCorpseValue = 3
)

// IsContainer reports whether the object is a container.
func (o *Object) IsContainer() bool { return o != nil && o.Type == ItemContainer }

// Capacity is how much weight the container will hold.
func (o *Object) Capacity() int32 {
	if o == nil {
		return 0
	}
	return o.Values[containerCapacity]
}

// ContainerFlags are the ContClosed/ContLocked bits.
//
// The C stores these in a signed int and reads them with IS_SET, so a corpse's
// -1 in value 3 would look like every flag set if it were read from here. It
// is not; only value 1 is a bitfield.
func (o *Object) ContainerFlags() Flags {
	if o == nil {
		return 0
	}
	return Flags(uint32(o.Values[containerFlagsValue])) //nolint:gosec // a bitfield, read as written
}

// SetContainerFlag sets bits in value 1.
func (o *Object) SetContainerFlag(mask Flags) {
	if o == nil {
		return
	}
	o.Values[containerFlagsValue] = int32(o.ContainerFlags().Set(mask)) //nolint:gosec // four bits
}

// ClearContainerFlag clears bits in value 1.
func (o *Object) ClearContainerFlag(mask Flags) {
	if o == nil {
		return
	}
	o.Values[containerFlagsValue] = int32(o.ContainerFlags().Clear(mask)) //nolint:gosec // four bits
}

// ContainerClosed reports whether the container is shut.
func (o *Object) ContainerClosed() bool { return o.ContainerFlags().Has(ContClosed) }

// ContainerLocked reports whether the container is locked.
func (o *Object) ContainerLocked() bool { return o.ContainerFlags().Has(ContLocked) }

// ContainerKey is the vnum of the key that opens it, or NoObject.
func (o *Object) ContainerKey() ObjVnum {
	if o == nil || o.Values[containerKeyValue] <= 0 {
		return NoObject
	}
	return ObjVnum(o.Values[containerKeyValue])
}

// moneyDescriptions is money_desc's table (handler.c:1222): the largest pile
// each phrase covers, and the phrase.
//
// The thresholds are not round numbers and the gaps between them widen fast —
// twenty coins is "a handful", a thousand is "a pile", and a million is "an
// enormous mountain". Anything past the end of the table gets the last line,
// which exists because somebody found out you could carry more than a million
// coins.
var moneyDescriptions = []struct {
	limit int32
	desc  string
}{
	{1, "a gold coin"},
	{10, "a tiny pile of gold coins"},
	{20, "a handful of gold coins"},
	{75, "a little pile of gold coins"},
	{200, "a small pile of gold coins"},
	{1000, "a pile of gold coins"},
	{5000, "a big pile of gold coins"},
	{10000, "a large heap of gold coins"},
	{20000, "a huge mound of gold coins"},
	{75000, "an enormous mound of gold coins"},
	{150000, "a small mountain of gold coins"},
	{250000, "a mountain of gold coins"},
	{500000, "a huge mountain of gold coins"},
	{1000000, "an enormous mountain of gold coins"},
}

// MoneyDescription names a pile of coins, porting money_desc.
func MoneyDescription(amount int32) string {
	for _, entry := range moneyDescriptions {
		if amount <= entry.limit {
			return entry.desc
		}
	}
	return "an absolutely colossal mountain of gold coins"
}

// capitaliseFirst upper-cases the first letter and leaves the rest, porting
// the C's CAP macro.
func capitaliseFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
