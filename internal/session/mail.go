// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package session

import (
	"time"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// SPECIAL(postmaster), ported from mail.c:502.
//
// Three commands — `mail`, `check`, `receive` — and, like the shop and the
// bank, all three are `do_not_here` until you are standing next to somebody
// who takes them.
//
// Mail is addressed by *id number*, not by name, and the name is looked up
// once when you write it. So mail to somebody who later renames survives, and
// mail to a deleted character is refused at the door rather than lost.

// MinMailLevel is MIN_MAIL_LEVEL (mail.h:20): level 1 characters cannot send
// mail, which is the entire anti-spam measure.
const MinMailLevel = 2

// StampPrice is STAMP_PRICE (mail.h:23).
const StampPrice int32 = 150

// MailSystem is what the postmaster needs from the rest of the server: the
// mail file, and the player index to turn names into ids and back.
//
// A seam like Violence and Rent, for the same reason.
type MailSystem interface {
	// Available reports whether the mail system is working. The C sets a
	// `no_mail` global when the file goes wrong and refuses everything.
	Available() bool
	// HasMail reports whether anything is waiting.
	HasMail(id int64) bool
	// Send stores a message.
	Send(to, from int64, sent time.Time, text string)
	// Receive takes the next message, rendered as the letter's text.
	Receive(id int64) (string, bool)
	// IDByName is get_id_by_name: the id of a registered character, or -1.
	// It reports false for a deleted one, which is mail_recip_ok.
	IDByName(name string) (int64, bool)
}

func specPostmaster(sc *SpecialCall) bool {
	if sc.Session == nil || sc.Actor.Client == nil || sc.Actor.IsNPC() {
		return false
	}
	if !sc.Is("mail") && !sc.Is("check") && !sc.Is("receive") {
		return false
	}
	if sc.Mail == nil || !sc.Mail.Available() {
		sc.Tell("Sorry, the mail system is having technical difficulties.\r\n")
		// The C returns 0 here — *not* handled — so the command falls through
		// to do_not_here and the player gets "Sorry, but you cannot do that
		// here!" after being told the mail is broken. Both messages, in that
		// order, which is almost certainly not what was meant.
		return false
	}

	switch {
	case sc.Is("mail"):
		postmasterSendMail(sc)
	case sc.Is("check"):
		postmasterCheckMail(sc)
	case sc.Is("receive"):
		postmasterReceiveMail(sc)
	}
	return true
}

// postmasterSendMail is postmaster_send_mail (mail.c:529).
func postmasterSendMail(sc *SpecialCall) {
	mailman, who := sc.Mob, sc.Actor

	if levelOf(who) < MinMailLevel {
		sc.tellFrom(mailman, "Sorry, you have to be level %d to send mail!", MinMailLevel)
		return
	}
	name, _ := oneArgument(sc.Arg)
	if name == "" {
		sc.tellFrom(mailman, "You need to specify an addressee!")
		return
	}
	if gold(who) < StampPrice {
		plural := "s"
		if StampPrice == 1 {
			plural = ""
		}
		sc.tellFrom(mailman, "A stamp costs %d coin%s.", StampPrice, plural)
		sc.tellFrom(mailman, "...which I see you can't afford.")
		return
	}
	recipient, ok := sc.Mail.IDByName(name)
	if !ok || recipient < 0 {
		sc.tellFrom(mailman, "No one by that name is registered here!")
		return
	}

	sc.ToRoom("%s starts to write some mail.\r\n", who.Name)
	sc.tellFrom(mailman, "I'll take %d coins for the stamp.", StampPrice)
	sc.tellFrom(mailman, "Write your message, use @ on a new line when done.")

	// The stamp is charged *before* the message is written, and it is not
	// refunded if you never finish. Walking away mid-letter costs you 150
	// coins, and always did.
	addGold(who, -StampPrice)

	from := int64(-1)
	if who.Record != nil {
		from = who.Record.IDNum
	}
	mail := sc.Mail
	sc.Session.beginEditor(maxMailSize, func(text string, saved bool) {
		// playing_string_cleanup's own PLR_MAILING branch (modify.c:226-231)
		// says "Mail aborted." for anything but a real save with something in
		// it — /a included — and "Message sent!" for one that goes.
		//
		// That confirmation is worth more here than it looks. The reference C
		// build used for parity testing is the one whose store_mail asserts
		// its blocks are exactly BLOCK_SIZE and core_dumps when they are not
		// on a 64-bit build (docs/deviations.md): it takes the stamp money,
		// says "Message sent!" and delivers nothing. This port actually
		// delivers, so the line is the only signal a player gets that the
		// letter went, and it was missing (#192).
		if !saved || text == "" {
			sc.Tell("Mail aborted.\r\n")
			return
		}
		mail.Send(recipient, from, time.Now(), text)
		sc.Tell("Message sent!\r\n")
	})
}

// maxMailSize is MAX_MAIL_SIZE (mail.h:26).
const maxMailSize = 4096

// postmasterCheckMail is postmaster_check_mail (mail.c:575).
func postmasterCheckMail(sc *SpecialCall) {
	if sc.Actor.Record != nil && sc.Mail.HasMail(sc.Actor.Record.IDNum) {
		sc.tellFrom(sc.Mob, "You have mail waiting.")
		return
	}
	sc.tellFrom(sc.Mob, "Sorry, you don't have any mail waiting.")
}

// postmasterReceiveMail is postmaster_receive_mail (mail.c:588).
//
// Every waiting letter at once, each as a separate object. They are notes, so
// `read letter` works — which is the only reason mail can be read at all.
func postmasterReceiveMail(sc *SpecialCall) {
	mailman, who := sc.Mob, sc.Actor
	if who.Record == nil || !sc.Mail.HasMail(who.Record.IDNum) {
		sc.tellFrom(mailman, "Sorry, you don't have any mail waiting.")
		return
	}

	for sc.Mail.HasMail(who.Record.IDNum) {
		text, ok := sc.Mail.Receive(who.Record.IDNum)
		if !ok {
			break
		}
		if text == "" {
			// "Mail system error - please report.  Error #11."
			text = "Mail system error - please report.  Error #11.\r\n"
		}

		letter := sc.World.NewBareObject()
		letter.Keywords = "mail paper letter"
		letter.ShortDesc = "a piece of mail"
		letter.Description = "Someone has left a piece of mail here."
		letter.Type = game.ItemNote
		letter.WearFlags = game.ItemWearTake | game.ItemWearHold
		letter.Weight = 1
		letter.Cost = 30
		letter.ActionDesc = text
		sc.World.ObjectToChar(letter, who)

		sc.Tell("%s gives you a piece of mail.\r\n", mailman.Name)
		for _, other := range sc.World.Occupants(who.Room) {
			if other != who && other != mailman {
				other.Tell("%s gives %s a piece of mail.\r\n", mailman.Name, who.Name)
			}
		}
	}
}
