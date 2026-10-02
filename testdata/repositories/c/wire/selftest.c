/* selftest: checks one escape, built by hand: cc -o selftest selftest.c encode.c escape.c */
#include <stdio.h>
#include <string.h>

#include "wire.h"

int main(void) {
    char out[16];
    wireEncode("a\nb", out, sizeof out);
    if (strcmp(out, "a\\nb") != 0) {
        fprintf(stderr, "selftest: got %s\n", out);
        return 1;
    }
    puts("selftest: ok");
    return 0;
}
