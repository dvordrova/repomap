#ifndef LOOP_H
#define LOOP_H

/* A single-threaded event loop: callbacks run when a descriptor is ready. */

#define LOOP_OK 0
#define LOOP_ERR -1

#define LOOP_NONE 0
#define LOOP_READABLE 1
#define LOOP_WRITABLE 2

#define LOOP_SETSIZE 1024

struct loop;

typedef void loopFileProc(struct loop *l, int fd, void *data, int mask);
typedef void loopBeforeSleepProc(struct loop *l);

/* What to call when one descriptor is readable or writable. */
typedef struct loopFileEvent {
    int mask;
    loopFileProc *rfileProc;
    loopFileProc *wfileProc;
    void *data;
} loopFileEvent;

/* A descriptor the backend found ready. */
typedef struct {
    int fd;
    int mask;
} loopFired;

typedef struct loop {
    int maxfd;
    int stop;
    loopFileEvent events[LOOP_SETSIZE];
    loopFired fired[LOOP_SETSIZE];
    loopBeforeSleepProc *beforeSleep;
    void *apidata;
} loop;

loop *loopCreate(void);
int loopCreateFileEvent(loop *l, int fd, int mask, loopFileProc *proc, void *data);
void loopDeleteFileEvent(loop *l, int fd, int mask);
void loopSetBeforeSleep(loop *l, loopBeforeSleepProc *proc);
int loopProcessEvents(loop *l);
void loopMain(loop *l);
void loopStop(loop *l);
long long loopNowMs(void);

#endif
