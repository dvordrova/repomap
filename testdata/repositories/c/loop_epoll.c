/* epoll(7) backend: Linux only. loop.c includes it when the build does not
 * ask for poll; it is not compiled on its own. */
#include <sys/epoll.h>

static int loopApiCreate(loop *l) {
    int *epfd = malloc(sizeof(int));
    if (epfd == NULL) return LOOP_ERR;
    *epfd = epoll_create(LOOP_SETSIZE);
    l->apidata = epfd;
    return *epfd == -1 ? LOOP_ERR : LOOP_OK;
}

static int loopApiAddEvent(loop *l, int fd, int mask) {
    struct epoll_event ee;
    int op = l->events[fd].mask == LOOP_NONE ? EPOLL_CTL_ADD : EPOLL_CTL_MOD;
    mask |= l->events[fd].mask;
    ee.events = (mask & LOOP_READABLE ? EPOLLIN : 0) | (mask & LOOP_WRITABLE ? EPOLLOUT : 0);
    ee.data.fd = fd;
    return epoll_ctl(*(int *)l->apidata, op, fd, &ee) == -1 ? LOOP_ERR : LOOP_OK;
}

static int loopApiPoll(loop *l) {
    struct epoll_event events[LOOP_SETSIZE];
    int j, ready = epoll_wait(*(int *)l->apidata, events, LOOP_SETSIZE, 1000);
    for (j = 0; j < ready; j++) {
        l->fired[j].fd = events[j].data.fd;
        l->fired[j].mask = (events[j].events & EPOLLIN ? LOOP_READABLE : 0) | (events[j].events & EPOLLOUT ? LOOP_WRITABLE : 0);
    }
    return ready > 0 ? ready : 0;
}
