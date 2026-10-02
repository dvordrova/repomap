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

/* The tick a watch prints by, its unit chosen by a flag. Each call below
   names its callee by a condition: where every branch names a function, the
   call runs one of them and the condition decides which; a branch holding a
   pointer leaves the call open. */
static long tickSeconds(long ms) { return ms / 1000; }
static long tickMillis(long ms) { return ms; }
static long tickTenths(long ms) { return ms / 100; }

long watchTick(int seconds, int tenths, long ms) {
    long (*held)(long) = tickMillis;
    long chosen = (seconds ? tickSeconds : tickMillis)(ms);
    long nested = (tenths ? (tickTenths) : seconds ? (long (*)(long))tickSeconds : &tickMillis)(ms);
    long open = (seconds ? held : tickTenths)(ms);
    return chosen + nested + open;
}

long watchTickAgain(int seconds, long ms) {
    long fixed = (1 ? tickSeconds : tickMillis)(ms);
    long same = (seconds ? tickMillis : (tickMillis))(ms);
    long star = (*(seconds ? tickSeconds : tickTenths))(ms);
    return fixed + same + star;
}
