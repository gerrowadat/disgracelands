#include "conf.h"
#include "sysdep.h"

#include "structs.h"
#include "utils.h"


const char *race_abbrevs[] = {
        "Hum",
        "Elf",
        "Gno",
        "Dwa",
        "\n"
};

const char *pc_race_types[] = {
        "Human",
        "Elf",
        "Gnome",
        "Dwarf",
        "\n"
};

/* The menu for choosing a race in interpreter.c: */
const char *race_menu =
"\r\n"
"Select a race:\r\n"
"  [H]uman\r\n"
"  [E]lf\r\n"
"  [G]nome\r\n"
"  [D]warf\r\n";

/*
 * The code to interpret a race letter (used in interpreter.c when a
 * new character is selecting a race).
 */
int parse_race(char arg)
{
        arg = LOWER(arg);

        switch (arg) {
                case 'h': return RACE_HUMAN;
                case 'e': return RACE_ELF;
                case 'g': return RACE_GNOME;
                case 'd': return RACE_DWARF;
                default:
                        return RACE_UNDEFINED;
        }
}

bitvector_t find_race_bitvector(const char *arg)
{
        size_t rpos, ret = 0;

        for (rpos = 0; rpos < strlen(arg); rpos++)
                ret |= (1 << parse_race(arg[rpos]));

        return (ret);
}

void racial_ability_modifiers(struct char_data *ch)
{
        switch (GET_RACE(ch)) {

                default:
                case RACE_HUMAN:
                        break;

                        break;

                case RACE_ELF:
                        ch->real_abils.dex += 1;
                        ch->real_abils.con -= 1;
                        break;

                case RACE_GNOME:
                        ch->real_abils.intel += 1;
                        ch->real_abils.wis -= 1;
                        break;

                case RACE_DWARF:
                        ch->real_abils.con += 1;
                        ch->real_abils.cha -= 1;
                        break;
   }
}


void set_height_by_race(struct char_data *ch)
{
        if (GET_SEX(ch) == SEX_MALE)
        {
                if (IS_DWARF(ch))
                        GET_HEIGHT(ch) = 43 + dice(1, 10);
                else if (IS_ELF(ch))
                        GET_HEIGHT(ch) = 55 + dice(1, 10);
                else if (IS_GNOME(ch))
                        GET_HEIGHT(ch) = 38 + dice(1, 6);
                else /* if (IS_HUMAN(ch)) */
                        GET_HEIGHT(ch) = 60 + dice(2, 10);
        } else /* if (IS_FEMALE(ch)) */ {
                if (IS_DWARF(ch))
                        GET_HEIGHT(ch) = 41 + dice(1, 10);
                else if (IS_ELF(ch))
                        GET_HEIGHT(ch) = 50 + dice(1, 10);
                else if (IS_GNOME(ch))
                        GET_HEIGHT(ch) = 36 + dice(1, 6);
                else /* if (IS_HUMAN(ch)) */
                        GET_HEIGHT(ch) = 59 + dice(2, 10);
        }

        return;
}

void set_weight_by_race(struct char_data *ch)
{
        if (GET_SEX(ch) == SEX_MALE)
        {
                if (IS_DWARF(ch))
                        GET_WEIGHT(ch) = 130 + dice(4, 10);
                else if (IS_ELF(ch))
                        GET_WEIGHT(ch) = 90 + dice(3, 10);
                else if (IS_GNOME(ch))
                        GET_WEIGHT(ch) = 72 + dice(5, 4);
                else /* if (IS_HUMAN(ch)) */
                        GET_WEIGHT(ch) = 140 + dice(6, 10);
        } else /* if (IS_FEMALE(ch)) */ {
                if (IS_DWARF(ch))
                        GET_WEIGHT(ch) = 105 + dice(4, 10);
                else if (IS_ELF(ch))
                        GET_WEIGHT(ch) = 70 + dice(3, 10);
                else if (IS_GNOME(ch))
                        GET_WEIGHT(ch) = 68 + dice(5, 4);
                else /* if (IS_HUMAN(ch)) */
                        GET_WEIGHT(ch) = 100 + dice(6, 10);
        }

        return;
}


int invalid_race(struct char_data *ch, struct obj_data *obj)
{
        if (GET_LEVEL(ch) >= LVL_IMMORT)
                return FALSE;

        if (OBJ_FLAGGED(obj, ITEM_ANTI_HUMAN) && IS_HUMAN(ch))
                return (TRUE);

        if (OBJ_FLAGGED(obj, ITEM_ANTI_ELF) && IS_ELF(ch))
                return (TRUE);

        if (OBJ_FLAGGED(obj, ITEM_ANTI_DWARF) && IS_DWARF(ch))
                return (TRUE);

        if (OBJ_FLAGGED(obj, ITEM_ANTI_GNOME) && IS_GNOME(ch))
                return (TRUE);

  return (FALSE);
}

