#include "wire.h"

size_t wireEscape(char c, char *out) {
    if (c == '\\' || c == '\n') {
        out[0] = '\\';
        out[1] = c == '\n' ? 'n' : '\\';
        return 2;
    }
    out[0] = c;
    return 1;
}
