/* upper: a loadable module that uppercases a value in place. It has no main;
 * the program loading it calls upperValue. */
#include <ctype.h>

#include "strbuf.h"

void upperValue(strbuf *sb) {
    for (size_t i = 0; i < sb->len; i++)
        sb->buf[i] = (char)toupper((unsigned char)sb->buf[i]);
}
