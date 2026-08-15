/* ************************************************************************
*   File: mobact.c                                      Part of CircleMUD *
*  Usage: Functions for generating intelligent (?) behavior in mobiles    *
*                                                                         *
*  All rights reserved.  See license.doc for complete information.        *
*                                                                         *
*  Copyright (C) 1993, 94 by the Trustees of the Johns Hopkins University *
*  CircleMUD is based on DikuMUD, Copyright (C) 1990, 1991.               *
************************************************************************ */



#include "conf.h"
#include "sysdep.h"


#include "structs.h"
#include "utils.h"
#include "db.h"
#include "comm.h"
#include "interpreter.h"
#include "handler.h"
#include "spells.h"
#include "constants.h"

int find_first_step(room_rnum src, room_rnum target); /*Yoss - mobtrack*/
int do_simple_move(struct char_data *ch, int dir, int need_specials_check);
ACMD(do_action);



/* external globals */
extern int no_specials;

/* external functions */
ACMD(do_get);

/* local functions */
void mobile_activity(void);
void clearMemory(struct char_data *ch);
bool aggressive_mob_on_a_leash(struct char_data *slave, struct char_data *master, struct char_data *attack);

#define MOB_AGGR_TO_ALIGN (MOB_AGGR_EVIL | MOB_AGGR_NEUTRAL | MOB_AGGR_GOOD)

void mobile_activity(void)
{
  struct char_data *ch, *next_ch, *vict;
  struct obj_data *obj, *best_obj;
  int door, found, max;
  memory_rec *names;
  struct descriptor_data *d;

  for (ch = character_list; ch; ch = next_ch) 
  {
    next_ch = ch->next;

    if (!IS_MOB(ch))
      continue;

    /* Examine call for special procedure */
    if (MOB_FLAGGED(ch, MOB_SPEC) && !no_specials) 
    {
      if (mob_index[GET_MOB_RNUM(ch)].func == NULL) 
      {
	log("SYSERR: %s (#%d): Attempting to call non-existing mob function.",
		GET_NAME(ch), GET_MOB_VNUM(ch));
	REMOVE_BIT(MOB_FLAGS(ch), MOB_SPEC);
      } 
      else 
      {
        char actbuf[MAX_INPUT_LENGTH] = "";
	if ((mob_index[GET_MOB_RNUM(ch)].func) (ch, ch, 0, actbuf))
	  continue;		/* go to next char */
      }
    }

    /* If the mob has no specproc, do the default actions */
    if (FIGHTING(ch) || !AWAKE(ch))
      continue;

    /* Scavenger (picking up objects) */
    if (MOB_FLAGGED(ch, MOB_SCAVENGER))
      if (world[IN_ROOM(ch)].contents && !number(0, 10)) 
      {
	max = 1;
	best_obj = NULL;
	for (obj = world[IN_ROOM(ch)].contents; obj; obj = obj->next_content)
	  if (CAN_GET_OBJ(ch, obj) && GET_OBJ_COST(obj) > max) 
          {
	    best_obj = obj;
	    max = GET_OBJ_COST(obj);
	  }
	if (best_obj != NULL) 
        {
	  obj_from_room(best_obj);
	  obj_to_char(best_obj, ch);
	  act("$n gets $p.", FALSE, ch, best_obj, 0, TO_ROOM);
	}
      }

    /* Mob Movement */
    if (!MOB_FLAGGED(ch, MOB_SENTINEL) && (GET_POS(ch) == POS_STANDING) &&
	((door = number(0, 18)) < NUM_OF_DIRS) && CAN_GO(ch, door) &&
	!ROOM_FLAGGED(EXIT(ch, door)->to_room, ROOM_NOMOB | ROOM_DEATH) &&
	(!MOB_FLAGGED(ch, MOB_STAY_ZONE) ||
	 (world[EXIT(ch, door)->to_room].zone == world[IN_ROOM(ch)].zone))) {
      perform_move(ch, door, 1);
    }

    /* Aggressive Mobs */
    if (MOB_FLAGGED(ch, MOB_AGGRESSIVE | MOB_AGGR_TO_ALIGN)) 
    {
      found = FALSE;
      for (vict = world[IN_ROOM(ch)].people; vict && !found; vict = vict->next_in_room) {
	if ((IS_NPC(vict) || !CAN_SEE(ch, vict) || PRF_FLAGGED(vict, PRF_NOHASSLE))||((IS_EVIL(ch))&&(AFF_FLAGGED(vict,AFF_HOLY_SHIELD))))
	  continue;

	if (MOB_FLAGGED(ch, MOB_WIMPY) && AWAKE(vict))
	  continue;

        if ((GET_MOB_VNUM(ch) == 14205) && (!AFF_FLAGGED(vict, AFF_BLIND))) /*Yoss - medusa*/
        {
          send_to_char("Medusa turns her eyes to you!\r\nYou flee head over heals.\r\n", vict);
          char_from_room(vict);
          char_to_room(vict, real_room(14284));
          act("$n has arrived.", TRUE, vict, 0, 0, TO_ROOM);
          look_at_room(vict, 0);
        }



	if (MOB_FLAGGED(ch, MOB_AGGRESSIVE  ) ||
	   (MOB_FLAGGED(ch, MOB_AGGR_EVIL   ) && IS_EVIL(vict)) ||
	   (MOB_FLAGGED(ch, MOB_AGGR_NEUTRAL) && IS_NEUTRAL(vict)) ||
	   (MOB_FLAGGED(ch, MOB_AGGR_GOOD   ) && IS_GOOD(vict))) {

          /* Can a master successfully control the charmed monster? */
          if (aggressive_mob_on_a_leash(ch, ch->master, vict))
            continue;

/*humbug puts something in the code without telling doc. ExXXXXpllooooooodeeeeeeeeee*/

      if (number(0, 20) <= GET_CHA(vict)) {
        act("$n looks at $N with an indifference.",
            FALSE, ch, 0, vict, TO_NOTVICT);
        act("$N looks at you with an indifference.",
            FALSE, vict, 0, ch, TO_CHAR);
      } else {
/*humbug yep, that bit just up there*/



	  hit(ch, vict, TYPE_UNDEFINED);
	  found = TRUE;
	}
      }
    }
/*humbug*/
}
/*humbug and that bracket there, silly*/


    /* Mob Memory */

    if (MOB_FLAGGED(ch, MOB_MEMORY) && MEMORY(ch))
    {
      found = FALSE;
      for (vict = world[ch->in_room].people; vict && !found; vict = vict->next_in_room)
      {
        if (IS_NPC(vict) || !CAN_SEE(ch, vict) || PRF_FLAGGED(vict, PRF_NOHASSLE))
          continue;
        for (names = MEMORY(ch); names && !found; names = names->next)
        {
          if (names->id == GET_IDNUM(vict))
          {
            found = TRUE;
            act("'Hey!  You're the fiend that attacked me!!!', exclaims $n.", FALSE, ch, 0, 0, TO_ROOM);
            hit(ch, vict, TYPE_UNDEFINED);
          }
        }
      }

      if ( !FIGHTING(ch) && (found == FALSE) && ((GET_MOB_VNUM(ch) == 14217) || (GET_MOB_VNUM(ch) == 14203)) )
      {
        for (d = descriptor_list; d; d = d->next)
        {
          if (STATE(d) == CON_PLAYING)
          {
            if (IS_NPC(d->character) || !CAN_SEE(ch, d->character) || PRF_FLAGGED(d->character, PRF_NOHASSLE))
              continue;
            if(FIGHTING(d->character))
              continue;
            for (names = MEMORY(ch); names && !found; names = names->next)
            {
              if (names->id != GET_IDNUM(d->character))
                continue;

              if(1 != number(1, 5))
                continue;

              found = TRUE;
              if(GET_MOB_VNUM(ch) == 14217)
              {

                //If you want him to summon people regardless of if they're in a !magic room, comment out this
                //if statement
/*                if(ROOM_FLAGGED(IN_ROOM(d->character), ROOM_NOMAGIC))
                {
                  send_to_char("Ares has tried to summon you!\r\n", d->character);
                  continue;
                }
*/

                act("$n disappears suddenly.", TRUE, d->character, 0, 0, TO_ROOM);
                send_to_char("You have been summoned!\r\n", d->character);
                char_from_room(d->character);
                char_to_room(d->character, IN_ROOM(ch));
                look_at_room(d->character, 0);
                act("Ares closes his eyes and utters the words, 'gewbar'.", TRUE, d->character, 0, 0, TO_ROOM);
                act("$n arrives suddenly.", TRUE, d->character, 0, 0, TO_ROOM);

                act("'Hey!  You're the fiend that attacked me!!!', exclaims $n.", FALSE, ch, 0, 0, TO_ROOM);
                hit(ch, d->character, TYPE_UNDEFINED);
              }
              else if(GET_MOB_VNUM(ch) == 14203)
              {
                int dir = find_first_step(IN_ROOM(ch), IN_ROOM(d->character));
                if(dir > -1)
                {
                  do_simple_move(ch, dir, 1);
                }
              }
            }
          }
        }
      }
    }




    /*
     * Charmed Mob Rebellion
     *
     * In order to rebel, there need to be more charmed monsters
     * than the person can feasibly control at a time.  Then the
     * mobiles have a chance based on the charisma of their leader.
     *
     * 1-4 = 0, 5-7 = 1, 8-10 = 2, 11-13 = 3, 14-16 = 4, 17-19 = 5, etc.
     */
    if (AFF_FLAGGED(ch, AFF_CHARM) && ch->master && num_followers_charmed(ch->master) > (GET_CHA(ch->master) - 2) / 3) {
      if (!aggressive_mob_on_a_leash(ch, ch->master, ch->master)) {
        if (CAN_SEE(ch, ch->master) && !PRF_FLAGGED(ch->master, PRF_NOHASSLE))
          hit(ch, ch->master, TYPE_UNDEFINED);
        stop_follower(ch);
      }
    }

    /* Helper Mobs */
    if (MOB_FLAGGED(ch, MOB_HELPER) && !AFF_FLAGGED(ch, AFF_BLIND | AFF_CHARM)) {
      found = FALSE;
      for (vict = world[IN_ROOM(ch)].people; vict && !found; vict = vict->next_in_room) {
	if (ch == vict || !IS_NPC(vict) || !FIGHTING(vict))
	  continue;
	if (IS_NPC(FIGHTING(vict)) || ch == FIGHTING(vict))
	  continue;

	act("$n jumps to the aid of $N!", FALSE, ch, 0, vict, TO_ROOM);
	hit(ch, FIGHTING(vict), TYPE_UNDEFINED);
	found = TRUE;
      }
    }

    /* Add new mobile actions here */

  }				/* end for() */
}



/* Mob Memory Routines */

/* make ch remember victim */
void remember(struct char_data *ch, struct char_data *victim)
{
  memory_rec *tmp;
  bool present = FALSE;

  if (!IS_NPC(ch) || IS_NPC(victim) || PRF_FLAGGED(victim, PRF_NOHASSLE))
    return;

  for (tmp = MEMORY(ch); tmp && !present; tmp = tmp->next)
    if (tmp->id == GET_IDNUM(victim))
      present = TRUE;

  if (!present) {
    CREATE(tmp, memory_rec, 1);
    tmp->next = MEMORY(ch);
    tmp->id = GET_IDNUM(victim);
    MEMORY(ch) = tmp;
  }
}


/* make ch forget victim */
void forget(struct char_data *ch, struct char_data *victim)
{
  memory_rec *curr, *prev = NULL;

  if (!(curr = MEMORY(ch)))
    return;

  while (curr && curr->id != GET_IDNUM(victim)) {
    prev = curr;
    curr = curr->next;
  }

  if (!curr)
    return;			/* person wasn't there at all. */

  if (curr == MEMORY(ch))
    MEMORY(ch) = curr->next;
  else
    prev->next = curr->next;

  free(curr);
}


/* erase ch's memory */
void clearMemory(struct char_data *ch)
{
  memory_rec *curr, *next;

  curr = MEMORY(ch);

  while (curr) {
    next = curr->next;
    free(curr);
    curr = next;
  }

  MEMORY(ch) = NULL;
}


/*
 * An aggressive mobile wants to attack something.  If
 * they're under the influence of mind altering PC, then
 * see if their master can talk them out of it, eye them
 * down, or otherwise intimidate the slave.
 */
bool aggressive_mob_on_a_leash(struct char_data *slave, struct char_data *master, struct char_data *attack)
{
  static int snarl_cmd;
  int dieroll;

  if (!master || !AFF_FLAGGED(slave, AFF_CHARM))
    return (FALSE);

  if (!snarl_cmd)
    snarl_cmd = find_command("snarl");

  /* Sit. Down boy! HEEEEeeeel! */
  dieroll = number(1, 20);
  if (dieroll != 1 && (dieroll == 20 || dieroll > 10 - GET_CHA(master) + GET_INT(slave))) {
    if (snarl_cmd > 0 && attack && !number(0, 3)) {
      char victbuf[MAX_NAME_LENGTH + 1];

      strncpy(victbuf, GET_NAME(attack), sizeof(victbuf));
      victbuf[sizeof(victbuf) - 1] = '\0';

      do_action(slave, victbuf, snarl_cmd, 0);
    }

    /* Success! But for how long? Hehe. */
    return (TRUE);
  }

  /* So sorry, now you're a player killer... Tsk tsk. */
  return (FALSE);
}

