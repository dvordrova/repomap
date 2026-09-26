/* poll(2) backend: portable, and fine for a few hundred descriptors. loop.c
 * includes it; it is not compiled on its own. */
#include <poll.h>

static int loopApiCreate(loop *l) {
    (void)l;
    return LOOP_OK;
}

static int loopApiAddEvent(loop *l, int fd, int mask) {
    (void)l, (void)fd, (void)mask;
    return LOOP_OK;
}

static int loopApiPoll(loop *l) {
    struct pollfd fds[LOOP_SETSIZE];
    int fd, j, watched = 0, fired = 0;
    for (fd = 0; fd <= l->maxfd; fd++) {
        int mask = l->events[fd].mask;
        if (mask == LOOP_NONE) continue;
        fds[watched].fd = fd;
        fds[watched].events = (mask & LOOP_READABLE ? POLLIN : 0) | (mask & LOOP_WRITABLE ? POLLOUT : 0);
        fds[watched].revents = 0;
        watched++;
    }
    if (poll(fds, watched, 1000) <= 0) return 0;
    for (j = 0; j < watched; j++) {
        int mask = LOOP_NONE;
        if (fds[j].revents & POLLIN) mask |= LOOP_READABLE;
        if (fds[j].revents & POLLOUT) mask |= LOOP_WRITABLE;
        if (mask == LOOP_NONE) continue;
        l->fired[fired].fd = fds[j].fd;
        l->fired[fired].mask = mask;
        fired++;
    }
    return fired;
}
