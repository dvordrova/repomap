/* wirecat: writes each line of its input as kvd's protocol would carry it. */
#include <stdio.h>
#include <string.h>

#include "wire.h"

int main(void) {
    char line[512], out[1024];
    while (fgets(line, sizeof line, stdin) != NULL) {
        line[strcspn(line, "\n")] = '\0';
        wireEncode(line, out, sizeof out);
        puts(out);
    }
    return 0;
}
