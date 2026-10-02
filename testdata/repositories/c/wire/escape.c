#include "wire.h"
#include "wire_internal.h"

int wireNeedsEscape(char c) {
    return c == '\\' || c == '\n';
}

size_t wireEscape(char c, char *out) {
    if (wireNeedsEscape(c)) {
        out[0] = '\\';
        out[1] = c == '\n' ? 'n' : '\\';
        return 2;
    }
    out[0] = c;
    return 1;
}
