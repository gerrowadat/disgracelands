/* ************************************************************************
*   File: spec_procs.c                                  Part of CircleMUD *
*  Usage: implementation of special procedures for mobiles/objects/rooms  *
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
#include "comm.h"
#include "interpreter.h"
#include "handler.h"
#include "db.h"
#include "spells.h"
#include "constants.h"

/*   external vars  */
extern struct time_info_data time_info;
extern struct spell_info_type spell_info[];
extern int guild_info[][3];

/* extern functions */
void add_follower(struct char_data *ch, struct char_data *leader);
ACMD(do_drop);
ACMD(do_gen_door);
ACMD(do_say);
ACMD(do_action);

ACMD(do_remove);
ACMD(do_give);
void perform_give(struct char_data *ch, struct char_data *vict, struct obj_data *obj);
void perform_remove(struct char_data *ch, int pos);


/* local functions */
void sort_spells(void);
int compare_spells(const void *x, const void *y);
const char *how_good(int percent);
void list_skills(struct char_data *ch);
SPECIAL(guild);
SPECIAL(dump);
SPECIAL(mayor);
void npc_steal(struct char_data *ch, struct char_data *victim);
SPECIAL(snake);
SPECIAL(moo);
SPECIAL(claws);
SPECIAL(cerberus);
SPECIAL(remmob); /*Yoss - remmob*/
SPECIAL(talkera);
SPECIAL(talkerb);
SPECIAL(talkerc);
SPECIAL(marbelsa);
SPECIAL(marbelsb);
SPECIAL(teleporter);
SPECIAL(cold);
SPECIAL(plink);
/* <DoC> */
SPECIAL(dispeller);
/* </DoC> */
SPECIAL(thief);
SPECIAL(magic_user);
SPECIAL(guild_guard);
SPECIAL(puff);
SPECIAL(fido);
SPECIAL(janitor);
SPECIAL(cityguard);
SPECIAL(pet_shops);
SPECIAL(bank);


/* ********************************************************************
*  Special procedures for mobiles                                     *
******************************************************************** */

int spell_sort_info[MAX_SKILLS + 1];

int compare_spells(const void *x, const void *y)
{
  int	a = *(const int *)x,
	b = *(const int *)y;

  return strcmp(spell_info[a].name, spell_info[b].name);
}

void sort_spells(void)
{
  int a;

  /* initialize array, avoiding reserved. */
  for (a = 1; a <= MAX_SKILLS; a++)
    spell_sort_info[a] = a;

  log("   Done Setup. Sorting Spells.");

  qsort(&spell_sort_info[1], MAX_SKILLS, sizeof(int), compare_spells);
}

const char *how_good(int percent)
{
  if (percent < 0)
    return " error)";
  if (percent == 0)
    return " (not learned)";
  if (percent <= 10)
    return " (awful)";
  if (percent <= 20)
    return " (bad)";
  if (percent <= 40)
    return " (poor)";
  if (percent <= 55)
    return " (average)";
  if (percent <= 70)
    return " (fair)";
  if (percent <= 80)
    return " (good)";
  if (percent <= 85)
    return " (very good)";

  return " (superb)";
}

const char *prac_types[] = {
  "spell",
  "skill"
};

#define LEARNED_LEVEL	0	/* % known which is considered "learned" */
#define MAX_PER_PRAC	1	/* max percent gain in skill per practice */
#define MIN_PER_PRAC	2	/* min percent gain in skill per practice */
#define PRAC_TYPE	3	/* should it say 'spell' or 'skill'?	 */

/* actual prac_params are in class.c */
extern int prac_params[4][NUM_CLASSES];

#define LEARNED(ch) (prac_params[LEARNED_LEVEL][(int)GET_CLASS(ch)])
#define MINGAIN(ch) (prac_params[MIN_PER_PRAC][(int)GET_CLASS(ch)])
#define MAXGAIN(ch) (prac_params[MAX_PER_PRAC][(int)GET_CLASS(ch)])
#define SPLSKL(ch) (prac_types[prac_params[PRAC_TYPE][(int)GET_CLASS(ch)]])

void list_skills(struct char_data *ch)
{
  int i, j, sortpos;

  if (!GET_PRACTICES(ch))
    strcpy(buf, "You have no practice sessions remaining.\r\n");
  else
    sprintf(buf, "You have %d practice session%s remaining.\r\n",
	    GET_PRACTICES(ch), (GET_PRACTICES(ch) == 1 ? "" : "s"));

  sprintf(buf + strlen(buf), "You know of the following %ss:\r\n", SPLSKL(ch));

  strcpy(buf2, buf);

  for (sortpos = 1; sortpos <= MAX_SKILLS; sortpos++) {
    i = spell_sort_info[sortpos];
    if (strlen(buf2) >= MAX_STRING_LENGTH - 32) {
      strcat(buf2, "**OVERFLOW**\r\n");
      break;
    }

        /* <DoC>
         * Replace this to list skills/spells from previous classes
    if (GET_LEVEL(ch) >= spell_info[i].min_level[(int) GET_CLASS(ch)]) {
      sprintf(buf, "%-20s %s\r\n", spell_info[i].name, how_good(GET_SKILL(ch, i)));
      strcat(buf2, buf);
    }
        */

        for(j = 0; j < NUM_CLASSES; j++)
        {
                if (((int)GET_REMORT_VECTOR(ch) & (int)pc_class_remort_masks[j]) == pc_class_remort_masks[j])
                {
                        if (GET_LEVEL(ch) >= spell_info[i].min_level[j])
                        {
                                sprintf(buf, "%-20s %s\r\n", spell_info[i].name, how_good(GET_SKILL(ch,i)));
                                strcat(buf2, buf);
                                break;
                        }
                }
        }




  }

  page_string(ch->desc, buf2, 1);
}


SPECIAL(guild)
{
  int skill_num, percent;

  if (IS_NPC(ch) || !CMD_IS("practice"))
    return (FALSE);

  skip_spaces(&argument);

  if (!*argument) {
    list_skills(ch);
    return (TRUE);
  }
  if (GET_PRACTICES(ch) <= 0) {
    send_to_char("You do not seem to be able to practice now.\r\n", ch);
    return (TRUE);
  }

  skill_num = find_skill_num(argument);

  if (skill_num < 1 ||
      GET_LEVEL(ch) < spell_info[skill_num].min_level[(int) GET_CLASS(ch)]) {
    sprintf(buf, "You do not know of that %s.\r\n", SPLSKL(ch));
    send_to_char(buf, ch);
    return (TRUE);
  }
  if (GET_SKILL(ch, skill_num) >= LEARNED(ch)) {
    send_to_char("You are already learned in that area.\r\n", ch);
    return (TRUE);
  }
  send_to_char("You practice for a while...\r\n", ch);
  GET_PRACTICES(ch)--;

  percent = GET_SKILL(ch, skill_num);
  percent += MIN(MAXGAIN(ch), MAX(MINGAIN(ch), int_app[GET_INT(ch)].learn));

  SET_SKILL(ch, skill_num, MIN(LEARNED(ch), percent));

  if (GET_SKILL(ch, skill_num) >= LEARNED(ch))
    send_to_char("You are now learned in that area.\r\n", ch);

  return (TRUE);
}



SPECIAL(dump)
{
  struct obj_data *k;
  int value = 0;

  for (k = world[IN_ROOM(ch)].contents; k; k = world[IN_ROOM(ch)].contents) {
    act("$p vanishes in a puff of smoke!", FALSE, 0, k, 0, TO_ROOM);
    extract_obj(k);
  }

  if (!CMD_IS("drop"))
    return (FALSE);

  do_drop(ch, argument, cmd, 0);

  for (k = world[IN_ROOM(ch)].contents; k; k = world[IN_ROOM(ch)].contents) {
    act("$p vanishes in a puff of smoke!", FALSE, 0, k, 0, TO_ROOM);
    value += MAX(1, MIN(50, GET_OBJ_COST(k) / 10));
    extract_obj(k);
  }

  if (value) {
    send_to_char("You are awarded for outstanding performance.\r\n", ch);
    act("$n has been awarded for being a good citizen.", TRUE, ch, 0, 0, TO_ROOM);

    if (GET_LEVEL(ch) < 3)
      gain_exp(ch, value);
    else
      GET_GOLD(ch) += value;
  }
  return (TRUE);
}


SPECIAL(mayor)
{
  char actbuf[MAX_INPUT_LENGTH];

  const char open_path[] =
	"W3a3003b33000c111d0d111e333333e22c222112212111a1S.";
  const char close_path[] =
	"W3a3003b33000c111d0d111E333333E22c222112212111a1S.";

  static const char *path = NULL;
  static int index;
  static bool move = FALSE;

  if (!move) {
    if (time_info.hours == 6) {
      move = TRUE;
      path = open_path;
      index = 0;
    } else if (time_info.hours == 20) {
      move = TRUE;
      path = close_path;
      index = 0;
    }
  }
  if (cmd || !move || (GET_POS(ch) < POS_SLEEPING) ||
      (GET_POS(ch) == POS_FIGHTING))
    return (FALSE);

  switch (path[index]) {
  case '0':
  case '1':
  case '2':
  case '3':
    perform_move(ch, path[index] - '0', 1);
    break;

  case 'W':
    GET_POS(ch) = POS_STANDING;
    act("$n awakens and groans loudly.", FALSE, ch, 0, 0, TO_ROOM);
    break;

  case 'S':
    GET_POS(ch) = POS_SLEEPING;
    act("$n lies down and instantly falls asleep.", FALSE, ch, 0, 0, TO_ROOM);
    break;

  case 'a':
    act("$n says 'Hello Honey!'", FALSE, ch, 0, 0, TO_ROOM);
    act("$n smirks.", FALSE, ch, 0, 0, TO_ROOM);
    break;

  case 'b':
    act("$n says 'What a view!  I must get something done about that dump!'",
	FALSE, ch, 0, 0, TO_ROOM);
    break;

  case 'c':
    act("$n says 'Vandals!  Youngsters nowadays have no respect for anything!'",
	FALSE, ch, 0, 0, TO_ROOM);
    break;

  case 'd':
    act("$n says 'Good day, citizens!'", FALSE, ch, 0, 0, TO_ROOM);
    break;

  case 'e':
    act("$n says 'I love this town!'", FALSE, ch, 0, 0, TO_ROOM);
    break;

  case 'E':
    act("$n says 'I hereby declare, oh, My, I forget!'", FALSE, ch, 0, 0, TO_ROOM);
    break;

  case 'O':
    do_gen_door(ch, strcpy(actbuf, "gate"), 0, SCMD_UNLOCK);
    do_gen_door(ch, strcpy(actbuf, "gate"), 0, SCMD_OPEN);
    break;

  case 'C':
    do_gen_door(ch, strcpy(actbuf, "gate"), 0, SCMD_CLOSE);
    do_gen_door(ch, strcpy(actbuf, "gate"), 0, SCMD_LOCK);
    break;

  case '.':
    move = FALSE;
    break;

  }

  index++;
  return (FALSE);
}


/* ********************************************************************
*  General special procedures for mobiles                             *
******************************************************************** */


void npc_steal(struct char_data *ch, struct char_data *victim)
{
  int gold;

  if (IS_NPC(victim))
    return;
  if (GET_LEVEL(victim) >= LVL_IMMORT)
    return;
  if (!CAN_SEE(ch, victim))
    return;

  if (AWAKE(victim) && (number(0, GET_LEVEL(ch)) == 0)) {
    act("You discover that $n has $s hands in your wallet.", FALSE, ch, 0, victim, TO_VICT);
    act("$n tries to steal gold from $N.", TRUE, ch, 0, victim, TO_NOTVICT);
  } else {
    /* Steal some gold coins */
    gold = (GET_GOLD(victim) * number(1, 10)) / 100;
    if (gold > 0) {
      GET_GOLD(ch) += gold;
      GET_GOLD(victim) -= gold;
    }
  }
}


/*
 * Quite lethal to low-level characters.
 */
SPECIAL(snake)
{
  if (cmd || GET_POS(ch) != POS_FIGHTING || !FIGHTING(ch))
    return (FALSE);

  if (IN_ROOM(FIGHTING(ch)) != IN_ROOM(ch) || number(0, GET_LEVEL(ch)) != 0)
    return (FALSE);

  act("$n bites $N!", 1, ch, 0, FIGHTING(ch), TO_NOTVICT);
  act("$n bites you!", 1, ch, 0, FIGHTING(ch), TO_VICT);
  call_magic(ch, FIGHTING(ch), 0, SPELL_POISON, GET_LEVEL(ch), CAST_SPELL);
  return (TRUE);
}

SPECIAL(talkera)
{
if(cmd) return 0;
  switch (number(0, 15)) {
  case 0:
    act("$n pats his sword in its scabbard.", FALSE, ch, 0, 0,TO_ROOM);
    do_say(ch, "You're next, you ugly rogue!", 0, 0);
    do_say(ch, "Just kidding.", 0, 0);
    act("$n pats you on your head.", FALSE, ch, 0, 0,TO_ROOM);
    return(1);
  case 1:
    do_say(ch, "I was conceived in Exile and have been integrated into society!", 0, 0);
    do_say(ch, "Muahaha!", 0, 0);
    return(1);
  case 2:
    do_say(ch, "Anyone have a butterknife I can borrow?", 0, 0);
    return(1);
  case 3:
    act("$n looks around suspiciously.", FALSE, ch, 0, 0,TO_ROOM);
    return(1);
  case 4:
   act("$n whistles a local tune.", FALSE, ch, 0, 0,TO_ROOM);
   return(1);
   default:
     return(FALSE);
     break;
  }
  return(0);
}

SPECIAL(talkerb)
{
if(cmd) return 0;
  switch (number(0, 15)) {
  case 0:
    act("$n sobs to himself..", FALSE, ch, 0, 0,TO_ROOM);
    do_say(ch, "Nobody loves me :(", 0, 0);
    do_say(ch, "*sniffle*.", 0, 0);
    act("$n looks at you sadly.", FALSE, ch, 0, 0,TO_ROOM);
    return(1);
  case 1:
    do_say(ch, "My wife left me.", 0, 0);
    do_say(ch, "And took Rover with her too.", 0, 0);
    return(1);
  case 2:
    do_say(ch, "Can you spare a copper for an old man?", 0, 0);
    return(1);
  case 3:
    act("$n takes out a handkerchief.", FALSE, ch, 0, 0,TO_ROOM);
    act("$n looks at it sadly.", FALSE, ch, 0, 0,TO_ROOM);
    act("$n blows his nose with a *sniffle*.", FALSE, ch, 0, 0, TO_ROOM);
    return(1);
  case 4:
   act("$n wipes away a tear.", FALSE, ch, 0, 0,TO_ROOM);
   return(1);
   default:
     return(FALSE);
     break;
  }
  return(0);
}

SPECIAL(talkerc)
{
if(cmd) return 0;
  switch (number(0, 15)) {
  case 0:
    act("$n smacks the leg of her leather chaps.", FALSE, ch, 0, 0,TO_ROOM);
    do_say(ch, "*giggle*", 0, 0);
    do_say(ch, "Hey, cowboy!", 0, 0);
    act("$n smiles innocently.", FALSE, ch, 0, 0,TO_ROOM);
    return(1);
  case 1:
    do_say(ch, "I've always liked horses.", 0, 0);
    do_say(ch, "I like the brown ones the best.", 0, 0);
    return(1);
  case 2:
    do_say(ch, "Do you like to ride?", 0, 0);
    return(1);
  case 3:
    act("$n suddenly whirls a lasso around.", FALSE, ch, 0, 0,TO_ROOM);
    act("$n shouts, 'Next!'", FALSE, ch, 0, 0, TO_ROOM);
    return(1);
  case 4:
   act("$n whistles a local tune.", FALSE, ch, 0, 0,TO_ROOM);
   return(1);
   default:
     return(FALSE);
     break;
  }
  return(0);
}


SPECIAL(moo)
{
  if (cmd || GET_POS(ch) != POS_FIGHTING || !FIGHTING(ch))
    return (FALSE);

  if (IN_ROOM(FIGHTING(ch)) != IN_ROOM(ch) || number(0, 1) != 0)
    return (FALSE);

  act("$n toasts $N with a gout of FIRE!", 1, ch, 0, FIGHTING(ch), TO_NOTVICT);
  act("$n toasts you with a gout of FIRE!", 1, ch, 0, FIGHTING(ch), TO_VICT);
  call_magic(ch, FIGHTING(ch), 0, SPELL_FIREBALL, GET_LEVEL(ch), CAST_SPELL);
  return (TRUE);
}

SPECIAL(claws)
{
  if (cmd || GET_POS(ch) != POS_FIGHTING || !FIGHTING(ch))
    return (FALSE);

  if (IN_ROOM(FIGHTING(ch)) != IN_ROOM(ch) || number(0, 1) != 0)
    return (FALSE);

  act("$n rakes $N with her wicked CLAWS!", 1, ch, 0, FIGHTING(ch), TO_NOTVICT);
  act("$n slashes you with her CLAWS!", 1, ch, 0, FIGHTING(ch), TO_VICT);
  call_magic(ch, FIGHTING(ch), 0, SPELL_OUCHIE, GET_LEVEL(ch), CAST_SPELL);
  return (TRUE);
}

SPECIAL(plink)
{
  if (cmd || GET_POS(ch) != POS_FIGHTING || !FIGHTING(ch))
    return (FALSE);

  if (IN_ROOM(FIGHTING(ch)) != IN_ROOM(ch) || number(0, 7) != 0)
    return (FALSE);

  act("$n zaps $N with a blast of time!", 1, ch, 0, FIGHTING(ch), TO_NOTVICT);
  act("$n DESTROYS you with a zap of pure time energy!", 1, ch, 0, FIGHTING(ch), TO_VICT);
  call_magic(ch, FIGHTING(ch), 0, SPELL_IMMOLATE, GET_LEVEL(ch), CAST_SPELL);
  return (TRUE);
}

SPECIAL(cold)
{
  if (cmd || GET_POS(ch) != POS_FIGHTING || !FIGHTING(ch))
    return (FALSE);

  if (IN_ROOM(FIGHTING(ch)) != IN_ROOM(ch) || number(0, 7) != 0)
    return (FALSE);

  act("$n freezes $N!", 1, ch, 0, FIGHTING(ch), TO_NOTVICT);
  act("$n CHILLS you to death!", 1, ch, 0, FIGHTING(ch), TO_VICT);
  call_magic(ch, FIGHTING(ch), 0, SPELL_IMMOLATE, GET_LEVEL(ch), CAST_SPELL);
  return (TRUE);
}

SPECIAL(cerberus)
{
  int i;
  struct char_data *guard = (struct char_data *)me;
  const char *buf = "Cerberus screeches, 'Those that enter may not leave the realm of the damned!'\r\n";
  const char *buf2 = "$n tries to leave up.\nCerberus blocks $n, and screeches, 'None shall leave the realm of the damned!'";

  if (!IS_MOVE(cmd) || AFF_FLAGGED(guard, AFF_BLIND))
    return (FALSE);

//If you dont want him to stop ye gods, comment these lines back in.
//  if (GET_LEVEL(ch) >= LVL_IMMORT)
//    return (FALSE);

  for (i = 0; guild_info[i][0] != -1; i++) {
    if ((IS_NPC(ch) || GET_CLASS(ch) != guild_info[i][0]) &&
        GET_ROOM_VNUM(IN_ROOM(ch)) == guild_info[i][1] &&
        cmd == guild_info[i][2]) {
      send_to_char(buf, ch);
      act(buf2, FALSE, ch, 0, 0, TO_ROOM);
      return (TRUE);
    }
  }

  return (FALSE);
}


SPECIAL(remmob) /*Yoss - remmob start*/
{

  int i;

  struct char_data *vict;
  struct obj_data *obj, *next_obj;

  if (cmd || GET_POS(ch) != POS_FIGHTING)
    return (FALSE);

  /* pseudo-randomly choose someone in the room who is fighting me */
  for (vict = world[IN_ROOM(ch)].people; vict; vict = vict->next_in_room)
    if (FIGHTING(vict) == ch)
      break;

  /* if I didn't pick any of those, then just slam the guy I'm fighting */
  if (vict == NULL && IN_ROOM(FIGHTING(ch)) == IN_ROOM(ch))
    vict = FIGHTING(ch);

  /* Hm...didn't pick anyone...I'll wait a round. */
  if (vict == NULL)
    return (TRUE);

  if (number(0, 2))
    return (TRUE);

  for (i = 0; i < NUM_WEARS; i++)
    if (GET_EQ(vict, i))
      perform_remove(vict, i);

  for (obj = vict->carrying; obj; obj = next_obj) {
    next_obj = obj->next_content;
    if (CAN_SEE_OBJ(vict, obj))
      perform_give(vict, ch, obj);
  }

  return (TRUE);

} /*Yoss - remmob end*/


SPECIAL(teleporter)
{
  if (cmd || GET_POS(ch) != POS_FIGHTING || !FIGHTING(ch))
    return (FALSE);

  if (IN_ROOM(FIGHTING(ch)) != IN_ROOM(ch) || number(0, 8) != 0)
    return (FALSE);

  act("$n dismisses $N with a gesture!", 1, ch, 0, FIGHTING(ch), TO_NOTVICT);
  act("$n dismisses you with a wave!", 1, ch, 0, FIGHTING(ch), TO_VICT);
  call_magic(ch, FIGHTING(ch), 0, SPELL_TELEPORT, GET_LEVEL(ch), CAST_SPELL);
  return (TRUE);
}

SPECIAL(thief)
{
  struct char_data *cons;

  if (cmd || GET_POS(ch) != POS_STANDING)
    return (FALSE);

  for (cons = world[IN_ROOM(ch)].people; cons; cons = cons->next_in_room)
    if (!IS_NPC(cons) && GET_LEVEL(cons) < LVL_IMMORT && !number(0, 4)) {
      npc_steal(ch, cons);
      return (TRUE);
    }

  return (FALSE);
}


SPECIAL(magic_user)
{
  struct char_data *vict;

  if (cmd || GET_POS(ch) != POS_FIGHTING)
    return (FALSE);

  /* pseudo-randomly choose someone in the room who is fighting me */
  for (vict = world[IN_ROOM(ch)].people; vict; vict = vict->next_in_room)
    if (FIGHTING(vict) == ch && !number(0, 4))
      break;

  /* if I didn't pick any of those, then just slam the guy I'm fighting */
  if (vict == NULL && IN_ROOM(FIGHTING(ch)) == IN_ROOM(ch))
    vict = FIGHTING(ch);

  /* Hm...didn't pick anyone...I'll wait a round. */
  if (vict == NULL)
    return (TRUE);

  if (GET_LEVEL(ch) > 13 && number(0, 10) == 0)
    cast_spell(ch, vict, NULL, SPELL_POISON);

  if (GET_LEVEL(ch) > 7 && number(0, 8) == 0)
    cast_spell(ch, vict, NULL, SPELL_BLINDNESS);

  if (GET_LEVEL(ch) > 12 && number(0, 12) == 0) {
    if (IS_EVIL(ch))
      cast_spell(ch, vict, NULL, SPELL_ENERGY_DRAIN);
    else if (IS_GOOD(ch))
      cast_spell(ch, vict, NULL, SPELL_DISPEL_EVIL);
  }

  if (number(0, 4))
    return (TRUE);

  switch (GET_LEVEL(ch)) {
  case 4:
  case 5:
    cast_spell(ch, vict, NULL, SPELL_MAGIC_MISSILE);
    break;
  case 6:
  case 7:
    cast_spell(ch, vict, NULL, SPELL_CHILL_TOUCH);
    break;
  case 8:
  case 9:
    cast_spell(ch, vict, NULL, SPELL_BURNING_HANDS);
    break;
  case 10:
  case 11:
    cast_spell(ch, vict, NULL, SPELL_SHOCKING_GRASP);
    break;
  case 12:
  case 13:
    cast_spell(ch, vict, NULL, SPELL_LIGHTNING_BOLT);
    break;
  case 14:
  case 15:
  case 16:
  case 17:
    cast_spell(ch, vict, NULL, SPELL_COLOR_SPRAY);
    break;
  default:
    cast_spell(ch, vict, NULL, SPELL_FIREBALL);
    break;
  }
  return (TRUE);

}


/* ********************************************************************
*  Special procedures for mobiles                                      *
******************************************************************** */

/* <DoC> */
/* Annoying cunt monster that dispels people. */
SPECIAL(dispeller)
{

	if (cmd || !FIGHTING(ch))
	{
		return(FALSE);
	}

  if (FIGHTING(ch))
  {
  			act("$n frowns disapprovingly at $N...", 1, ch, 0, FIGHTING(ch), TO_NOTVICT);
  			act("$n frowns disapprovingly at you. You shiver.", 1, ch, 0, FIGHTING(ch), TO_VICT);
  			call_magic(ch, FIGHTING(ch), 0, SPELL_DISPEL_MAGIC, GET_LEVEL(ch), CAST_SPELL);
			return(TRUE);
	}

  return(FALSE);
}
/* </DoC> */



SPECIAL(guild_guard)
{
  int i;
  struct char_data *guard = (struct char_data *)me;
  const char *buf = "The guard humiliates you, and blocks your way.\r\n";
  const char *buf2 = "The guard humiliates $n, and blocks $s way.";

  if (!IS_MOVE(cmd) || AFF_FLAGGED(guard, AFF_BLIND))
    return (FALSE);

  if (GET_LEVEL(ch) >= LVL_IMMORT)
    return (FALSE);

  
  if (IS_NPC(ch))
          return(FALSE);

  /* <DoC>
   * Now checks for each previous class. Remorted chars can access all guilds they have
 completed.
   */
 for (i = 0; guild_info[i][0] != -1; i++)
 {
          if ((((int)GET_REMORT_VECTOR(ch) & (int)pc_class_remort_masks[(int)guild_info[i][0]]) != pc_class_remort_masks[(int)guild_info[i][0]]) &&
                (GET_ROOM_VNUM(IN_ROOM(ch)) == guild_info[i][1]) &&
                cmd == guild_info[i][2])
          {
                send_to_char(buf, ch);
                act(buf2, FALSE, ch, 0, 0, TO_ROOM);
                return(TRUE);
          }
  }

  return (FALSE);
}



SPECIAL(puff)
{
  char actbuf[MAX_INPUT_LENGTH];

  if (cmd)
    return (FALSE);

  switch (number(0, 60)) {
  case 0:
    do_say(ch, strcpy(actbuf, "My god!  It's full of stars!"), 0, 0);
    return (TRUE);
  case 1:
    do_say(ch, strcpy(actbuf, "How'd all those fish get up here?"), 0, 0);
    return (TRUE);
  case 2:
    do_say(ch, strcpy(actbuf, "I'm a very female dragon."), 0, 0);
    return (TRUE);
  case 3:
    do_say(ch, strcpy(actbuf, "Hail to the King, Baby!"), 0, 0);
    return (TRUE);
  default:
    return (FALSE);
  }
}



SPECIAL(fido)
{
  struct obj_data *i, *temp, *next_obj;

  if (cmd || !AWAKE(ch))
    return (FALSE);

  for (i = world[IN_ROOM(ch)].contents; i; i = i->next_content) {
    if (!IS_CORPSE(i))
      continue;

    act("$n savagely devours a corpse.", FALSE, ch, 0, 0, TO_ROOM);
    for (temp = i->contains; temp; temp = next_obj) {
      next_obj = temp->next_content;
      obj_from_obj(temp);
      obj_to_room(temp, IN_ROOM(ch));
    }
    extract_obj(i);
    return (TRUE);
  }

  return (FALSE);
}



SPECIAL(janitor)
{
  struct obj_data *i;

  if (cmd || !AWAKE(ch))
    return (FALSE);

  for (i = world[IN_ROOM(ch)].contents; i; i = i->next_content) {
    if (!CAN_WEAR(i, ITEM_WEAR_TAKE))
      continue;
    if (GET_OBJ_TYPE(i) != ITEM_DRINKCON && GET_OBJ_COST(i) >= 15)
      continue;
    act("$n picks up some trash.", FALSE, ch, 0, 0, TO_ROOM);
    obj_from_room(i);
    obj_to_char(i, ch);
    return (TRUE);
  }

  return (FALSE);
}


SPECIAL(cityguard)
{
  struct char_data *tch, *evil, *spittle;
  int max_evil, min_cha;

  if (cmd || !AWAKE(ch) || FIGHTING(ch))
    return (FALSE);

  max_evil = 1000;
  min_cha = 6;
  spittle = evil = NULL;

  for (tch = world[IN_ROOM(ch)].people; tch; tch = tch->next_in_room) {
    if (!CAN_SEE(ch, tch))
      continue;

    if (!IS_NPC(tch) && PLR_FLAGGED(tch, PLR_KILLER)) {
      act("$n screams 'HEY!!!  You're one of those PLAYER KILLERS!!!!!!'", FALSE, ch, 0, 0, TO_ROOM);
      hit(ch, tch, TYPE_UNDEFINED);
      return (TRUE);
    }

    if (!IS_NPC(tch) && PLR_FLAGGED(tch, PLR_THIEF)) {
      act("$n screams 'HEY!!!  You're one of those PLAYER THIEVES!!!!!!'", FALSE, ch, 0, 0, TO_ROOM);
      hit(ch, tch, TYPE_UNDEFINED);
      return (TRUE);
    }

    if (FIGHTING(tch) && GET_ALIGNMENT(tch) < max_evil && (IS_NPC(tch) || IS_NPC(FIGHTING(tch)))) {
      max_evil = GET_ALIGNMENT(tch);
      evil = tch;
    }

    if (GET_CHA(tch) < min_cha) {
      spittle = tch;
      min_cha = GET_CHA(tch);
    }
  }

  if (evil && GET_ALIGNMENT(FIGHTING(evil)) >= 0) {
    act("$n screams 'PROTECT THE INNOCENT!  BANZAI!  CHARGE!  ARARARAGGGHH!'", FALSE, ch, 0, 0, TO_ROOM);
    hit(ch, evil, TYPE_UNDEFINED);
    return (TRUE);
  }

  /* Reward the socially inept. */
  if (spittle && !number(0, 1)) {
    static int spit_social;

    if (!spit_social)
      spit_social = find_command("spit");

    if (spit_social > 0) {
      char spitbuf[MAX_NAME_LENGTH + 1];

      strncpy(spitbuf, GET_NAME(spittle), sizeof(spitbuf));
      spitbuf[sizeof(spitbuf) - 1] = '\0';

      do_action(ch, spitbuf, spit_social, 0);
      return (TRUE);
    }
  }

  return (FALSE);
}


#define PET_PRICE(pet) (GET_LEVEL(pet) * 300)

SPECIAL(pet_shops)
{
  char buf[MAX_STRING_LENGTH], pet_name[256];
  room_rnum pet_room;
  struct char_data *pet;

  /* Gross. */
  pet_room = IN_ROOM(ch) + 1;

  if (CMD_IS("list")) {
    send_to_char("Available pets are:\r\n", ch);
    for (pet = world[pet_room].people; pet; pet = pet->next_in_room) {
      /* No, you can't have the Implementor as a pet if he's in there. */
      if (!IS_NPC(pet))
        continue;
      sprintf(buf, "%8d - %s\r\n", PET_PRICE(pet), GET_NAME(pet));
      send_to_char(buf, ch);
    }
    return (TRUE);
  } else if (CMD_IS("buy")) {

    two_arguments(argument, buf, pet_name);

    if (!(pet = get_char_room(buf, NULL, pet_room)) || !IS_NPC(pet)) {
      send_to_char("There is no such pet!\r\n", ch);
      return (TRUE);
    }
    if (GET_GOLD(ch) < PET_PRICE(pet)) {
      send_to_char("You don't have enough gold!\r\n", ch);
      return (TRUE);
    }
    GET_GOLD(ch) -= PET_PRICE(pet);

    pet = read_mobile(GET_MOB_RNUM(pet), REAL);
    GET_EXP(pet) = 0;
    SET_BIT(AFF_FLAGS(pet), AFF_CHARM);

    if (*pet_name) {
      sprintf(buf, "%s %s", pet->player.name, pet_name);
      /* free(pet->player.name); don't free the prototype! */
      pet->player.name = str_dup(buf);

      sprintf(buf, "%sA small sign on a chain around the neck says 'My name is %s'\r\n",
	      pet->player.description, pet_name);
      /* free(pet->player.description); don't free the prototype! */
      pet->player.description = str_dup(buf);
    }
    char_to_room(pet, IN_ROOM(ch));
    add_follower(pet, ch);

    /* Be certain that pets can't get/carry/use/wield/wear items */
    IS_CARRYING_W(pet) = 1000;
    IS_CARRYING_N(pet) = 100;

    send_to_char("May you enjoy your pet.\r\n", ch);
    act("$n buys $N as a pet.", FALSE, ch, 0, pet, TO_ROOM);

    return (TRUE);
  }

  /* All commands except list and buy */
  return (FALSE);
}



/* ********************************************************************
*  Special procedures for objects                                     *
******************************************************************** */


SPECIAL(bank)
{
  int amount;

  if (CMD_IS("balance")) {
    if (GET_BANK_GOLD(ch) > 0)
      sprintf(buf, "Your current balance is %d coins.\r\n",
	      GET_BANK_GOLD(ch));
    else
      sprintf(buf, "You currently have no money deposited.\r\n");
    send_to_char(buf, ch);
    return (TRUE);
  } else if (CMD_IS("deposit")) {
    if ((amount = atoi(argument)) <= 0) {
      send_to_char("How much do you want to deposit?\r\n", ch);
      return (TRUE);
    }
    if (GET_GOLD(ch) < amount) {
      send_to_char("You don't have that many coins!\r\n", ch);
      return (TRUE);
    }
    GET_GOLD(ch) -= amount;
    GET_BANK_GOLD(ch) += amount;
    sprintf(buf, "You deposit %d coins.\r\n", amount);
    send_to_char(buf, ch);
    act("$n makes a bank transaction.", TRUE, ch, 0, FALSE, TO_ROOM);
    return (TRUE);
  } else if (CMD_IS("withdraw")) {
    if ((amount = atoi(argument)) <= 0) {
      send_to_char("How much do you want to withdraw?\r\n", ch);
      return (TRUE);
    }
    if (GET_BANK_GOLD(ch) < amount) {
      send_to_char("You don't have that many coins deposited!\r\n", ch);
      return (TRUE);
    }
    GET_GOLD(ch) += amount;
    GET_BANK_GOLD(ch) -= amount;
    sprintf(buf, "You withdraw %d coins.\r\n", amount);
    send_to_char(buf, ch);
    act("$n makes a bank transaction.", TRUE, ch, 0, FALSE, TO_ROOM);
    return (TRUE);
  } else
    return (FALSE);
}

SPECIAL(marblesa)
{
  struct obj_data *tobj = me;

  if (tobj->in_room == NOWHERE)
    return 0;

  if (CMD_IS("north") || CMD_IS("south") || CMD_IS("east") || CMD_IS("west") ||
      CMD_IS("up") || CMD_IS("down")) {
    if (number(1, 100) + GET_DEX(ch) > 50) {
      act("You slip on $p and fall.", FALSE, ch, tobj, 0, TO_CHAR);
      act("$n slips on $p and falls.", FALSE, ch, tobj, 0, TO_ROOM);
      GET_POS(ch) = POS_SITTING;
      return 1;
    }
    else {
      act("You slip on $p, but manage to retain your balance.", FALSE, ch, tobj, 0, TO_CHAR);
      act("$n slips on $p, but manages to retain $s balance.", FALSE, ch, tobj, 0, TO_ROOM);
    }
  }
  return 0;
}

SPECIAL(marblesb)
{
  struct obj_data *tobj = me;

  if (tobj->in_room == NOWHERE)
    return 0;

  if (CMD_IS("north") || CMD_IS("south") || CMD_IS("east") || CMD_IS("west") ||
      CMD_IS("up") || CMD_IS("down")) {
    if (number(1, 100) + GET_DEX(ch) < 50) {
      act("You slip on $p and fall.", FALSE, ch, tobj, 0, TO_CHAR);
      act("$n slips on $p and falls.", FALSE, ch, tobj, 0, TO_ROOM);
      GET_POS(ch) = POS_SITTING;
      return 1;
    }
    else {
      act("You slip on $p, but manage to retain your balance.", FALSE, ch, tobj, 0, TO_CHAR);
      act("$n slips on $p, but manages to retain $s balance.", FALSE, ch, tobj, 0, TO_ROOM);
    }
  }
  return 0;
}

