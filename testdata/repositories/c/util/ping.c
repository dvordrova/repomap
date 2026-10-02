/* ping: connects to kvd once and says whether it is listening. */
#include <stdio.h>

#include "net.h"

int main(void) {
    int fd = netConnect("127.0.0.1", 7379);
    if (fd < 0) {
        fprintf(stderr, "kvd is not listening\n");
        return 1;
    }
    printf("kvd is listening\n");
    return 0;
}
