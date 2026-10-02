#ifndef WIRE_H
#define WIRE_H

#include <stddef.h>

/* One value as kvd's protocol writes it on a line: backslashes and newlines
 * escaped, so a value never ends a command early. */
size_t wireEncode(const char *value, char *out, size_t size);
size_t wireEscape(char c, char *out);

#endif
