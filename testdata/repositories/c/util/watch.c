/* watch: says how long kvd takes to accept a connection. */
#include <stdio.h>

#include "loop.h"
#include "net.h"

int main(void) {
    long long start = loopNowMs();
    int fd = netConnect("127.0.0.1", 7379);
    if (fd < 0) {
        fprintf(stderr, "kvd is not listening\n");
        return 1;
    }
    printf("connected in %lld ms\n", loopNowMs() - start);
    return 0;
}
