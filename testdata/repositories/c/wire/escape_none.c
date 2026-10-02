/* The raw build's escape: every character as it is, for values known to hold
 * no backslash or newline (`make raw`). */
#include "wire.h"

size_t wireEscape(char c, char *out) {
    out[0] = c;
    return 1;
}
