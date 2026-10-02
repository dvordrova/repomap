#include "wire.h"

size_t wireEncode(const char *value, char *out, size_t size) {
    size_t used = 0;
    char escaped[2];
    for (; *value != '\0'; value++) {
        size_t n = wireEscape(*value, escaped);
        if (used + n + 1 > size) break;
        for (size_t i = 0; i < n; i++) out[used++] = escaped[i];
    }
    out[used] = '\0';
    return used;
}
