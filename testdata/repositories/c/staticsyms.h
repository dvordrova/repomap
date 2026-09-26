/* Command function addresses by name, printed by kvd --symbols to read a
 * crash report. The functions are cast to integers: nothing here calls or
 * registers them. */
static struct kvSymbol {
    const char *name;
    unsigned long pointer;
} symsTable[] = {
    {"getCommand", (unsigned long)getCommand},
    {"setCommand", (unsigned long)setCommand},
    {"delCommand", (unsigned long)delCommand},
    {NULL, 0}
};
