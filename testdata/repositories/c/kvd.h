#ifndef KVD_H
#define KVD_H

#include <sys/types.h>

#include "loop.h"
#include "strbuf.h"

#define KVD_DEFAULT_PORT 7379
#define KV_MAX_KEYS 1024
#define KV_MAX_ARGS 16

/* Stops the server with the expression that did not hold. */
#define kvAssert(e) ((e) ? (void)0 : kvAssertFail(#e, __FILE__, __LINE__))

/* A connected client: what it sent and what it has still to receive. */
typedef struct kvClient {
    int fd;
    strbuf query;
    strbuf reply;
    int argc;
    char *argv[KV_MAX_ARGS];
} kvClient;

typedef void kvCommandProc(kvClient *c);

/* One row of the command table: the name a client sends, the function that
 * runs it, and how many words it takes (negative: at least that many). */
typedef struct kvCommand {
    const char *name;
    kvCommandProc *proc;
    int arity;
} kvCommand;

struct kvEntry {
    char *key;
    char *value;
};

struct kvServer {
    loop *el;
    int port;
    const char *dbfile;
    struct kvEntry db[KV_MAX_KEYS];
    int dbSize;
    long dirty;
    pid_t saveChild;
    volatile int shutdown;
};

extern struct kvServer server;

void kvAssertFail(const char *expr, const char *file, int line);

#endif
