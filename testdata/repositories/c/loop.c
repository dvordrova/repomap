#include <stdio.h>
#include <stdlib.h>

#include "loop.h"

/* The backend waits for ready descriptors. Only one file is compiled, as a
 * part of this one: epoll on Linux unless the build asks for poll, and poll
 * everywhere else. */
#if defined(__linux__) && !defined(LOOP_POLL)
#include "loop_epoll.c"
#else
#include "loop_poll.c"
#endif

/* strbuf.c has its own oom: each is static to its file. */
static void oom(const char *what) {
    fprintf(stderr, "loop: out of memory allocating %s\n", what);
    abort();
}

loop *loopCreate(void) {
    loop *l = calloc(1, sizeof(*l));
    if (l == NULL) oom("the loop");
    l->maxfd = -1;
    if (loopApiCreate(l) != LOOP_OK) oom("the backend");
    return l;
}

/* One call registers a read handler, a write handler or both: which field
 * keeps proc depends on mask. */
int loopCreateFileEvent(loop *l, int fd, int mask, loopFileProc *proc, void *data) {
    loopFileEvent *fe;
    if (fd >= LOOP_SETSIZE) return LOOP_ERR;
    if (loopApiAddEvent(l, fd, mask) != LOOP_OK) return LOOP_ERR;
    fe = &l->events[fd];
    fe->mask |= mask;
    if (mask & LOOP_READABLE) fe->rfileProc = proc;
    if (mask & LOOP_WRITABLE) fe->wfileProc = proc;
    fe->data = data;
    if (fd > l->maxfd) l->maxfd = fd;
    return LOOP_OK;
}

void loopDeleteFileEvent(loop *l, int fd, int mask) {
    if (fd >= LOOP_SETSIZE) return;
    l->events[fd].mask &= ~mask;
}

void loopSetBeforeSleep(loop *l, loopBeforeSleepProc *proc) {
    l->beforeSleep = proc;
}

int loopProcessEvents(loop *l) {
    int j, ready = loopApiPoll(l);
    for (j = 0; j < ready; j++) {
        loopFileEvent *fe = &l->events[l->fired[j].fd];
        int fd = l->fired[j].fd, mask = l->fired[j].mask;
        if (fe->mask & mask & LOOP_READABLE) fe->rfileProc(l, fd, fe->data, mask);
        if (fe->mask & mask & LOOP_WRITABLE) fe->wfileProc(l, fd, fe->data, mask);
    }
    return ready;
}

void loopMain(loop *l) {
    l->stop = 0;
    while (!l->stop) {
        if (l->beforeSleep != NULL) l->beforeSleep(l);
        loopProcessEvents(l);
    }
}

void loopStop(loop *l) {
    l->stop = 1;
}
