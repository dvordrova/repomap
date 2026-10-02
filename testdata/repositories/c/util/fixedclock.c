/* A clock that always says the same time, so `make watch-replay` prints the
 * same line on every run. */
#include "loop.h"

long long loopNowMs(void) {
    return 1000;
}
