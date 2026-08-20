/*
 * shopprice.c -- what the C server actually charges.
 *
 * Part of Disgracelands. See reference/tools/README.md.
 *
 * buy_price and sell_price are one line each (shop.c:474, shop.c:632):
 *
 *     return (GET_OBJ_COST(obj) * SHOP_BUYPROFIT(shop_nr));
 *
 * An int times a float, returned as an int. The interesting part is not the
 * multiplication, it is *what width the multiplication happens at*, because
 * the result is truncated rather than rounded and the two widths disagree:
 *
 *   - Evaluated as float (FLT_EVAL_METHOD 0, which is what any x86-64 build
 *     with SSE does): 100 * 1.15f rounds to exactly 115.0f, and truncates
 *     to 115.
 *   - Evaluated in the x87's 80-bit registers (FLT_EVAL_METHOD 2, which is
 *     what a 32-bit i386 build does, and what the archived server was):
 *     100 * 1.1499999761581421 is 114.99999761581421, and truncates to 114.
 *
 * So the same source charges a different price depending on the machine, and
 * the price players actually paid is the second one. Build this -m32 and it
 * prints the archive's answer; build it native and it prints the other, which
 * is why the Go test compares against the 32-bit build specifically.
 *
 * Prints one line per case: cost buy sell
 */

#include <stdio.h>
#include <stdlib.h>

int buy_price(int cost, float profit_buy)
{
  return (cost * profit_buy);
}

int sell_price(int cost, float profit_sell)
{
  return (cost * profit_sell);
}

int main(void)
{
  /* The multipliers that actually appear in data/world/shp, plus the
   * defaults boot_the_shops uses. */
  static const float buys[]  = { 1.0f, 1.15f, 1.2f, 1.5f, 2.0f, 3.0f };
  static const float sells[] = { 0.0f, 0.15f, 0.2f, 0.5f, 0.75f, 1.0f };
  int i, cost;

  printf("# FLT_EVAL_METHOD %d\n", (int) __FLT_EVAL_METHOD__);
  for (i = 0; i < (int) (sizeof(buys) / sizeof(buys[0])); i++)
    for (cost = 0; cost <= 2000; cost++)
      printf("%.10g %.10g %d %d %d\n", (double) buys[i], (double) sells[i],
	     cost, buy_price(cost, buys[i]), sell_price(cost, sells[i]));

  return (EXIT_SUCCESS);
}
