// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package session

import (
	"strconv"
	"strings"
)

// SPECIAL(bank), ported from spec_procs.c:1022.
//
// Three commands, all `do_not_here` in the table like the shop's, all taken
// by a mobile with this special on it. The bank balance itself is a field on
// the player record, so the banker is a doorway to it rather than a ledger of
// their own — which is why every bank in the game shares one balance and why
// a shopkeeper can draw on `GET_BANK_GOLD` too.

func specBank(sc *SpecialCall) bool {
	rec := sc.Actor.Record
	if rec == nil {
		return false
	}

	switch {
	case sc.Is("balance"):
		if rec.Points.BankGold > 0 {
			sc.Tell("Your current balance is %d coins.\r\n", rec.Points.BankGold)
		} else {
			sc.Tell("You currently have no money deposited.\r\n")
		}
		return true

	case sc.Is("deposit"):
		amount := bankAmount(sc.Arg)
		if amount <= 0 {
			sc.Tell("How much do you want to deposit?\r\n")
			return true
		}
		if rec.Points.Gold < amount {
			sc.Tell("You don't have that many coins!\r\n")
			return true
		}
		rec.Points.Gold -= amount
		rec.Points.BankGold += amount
		sc.Tell("You deposit %d coins.\r\n", amount)
		sc.ToRoom("%s makes a bank transaction.\r\n", sc.Actor.Name)
		return true

	case sc.Is("withdraw"):
		amount := bankAmount(sc.Arg)
		if amount <= 0 {
			sc.Tell("How much do you want to withdraw?\r\n")
			return true
		}
		if rec.Points.BankGold < amount {
			sc.Tell("You don't have that many coins deposited!\r\n")
			return true
		}
		rec.Points.Gold += amount
		rec.Points.BankGold -= amount
		sc.Tell("You withdraw %d coins.\r\n", amount)
		sc.ToRoom("%s makes a bank transaction.\r\n", sc.Actor.Name)
		return true
	}
	return false
}

// bankAmount is the C's `atoi(argument)`.
//
// atoi stops at the first character that is not part of a number and returns
// zero for anything that does not start with one, so "deposit 100 coins"
// deposits a hundred and "deposit all" deposits nothing and asks how much.
// Reproduced rather than tightened: somebody typing `deposit all` should get
// the question, not an error message the game never had.
func bankAmount(arg string) int32 {
	s := strings.TrimSpace(arg)
	end := 0
	if end < len(s) && (s[end] == '-' || s[end] == '+') {
		end++
	}
	for end < len(s) && s[end] >= '0' && s[end] <= '9' {
		end++
	}
	n, err := strconv.ParseInt(s[:end], 10, 32)
	if err != nil {
		return 0
	}
	return int32(n)
}
