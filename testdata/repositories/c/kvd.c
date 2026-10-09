/* kvd: a small in-memory key-value server. A client sends one command per
 * line ("set name ada"); the server looks the first word up in its command
 * table and runs the function that row names. */
#include <fcntl.h>
#include <pthread.h>
#include <signal.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <strings.h>
#include <sys/socket.h>
#include <sys/wait.h>
#include <unistd.h>

#include "kvd.h"
#include "net.h"

/* Global state */
struct kvServer server;

/*================================ Commands ================================ */

static void getCommand(kvClient *c);
static void preloadKey(kvClient *c);
static void setCommand(kvClient *c);
static void delCommand(kvClient *c);
static void keysCommand(kvClient *c);
static void pingCommand(kvClient *c);
static void bgsaveCommand(kvClient *c);

static kvCommand cmdTable[] = {
    {"get", getCommand, 2, preloadKey},
    {"set", setCommand, 3},
    {"del", delCommand, -2},
    {"keys", keysCommand, 1},
    {"ping", pingCommand, 1},
    {"bgsave", bgsaveCommand, 1},
    {NULL, NULL, 0}
};

#include "staticsyms.h"

static void sendReplyToClient(loop *l, int fd, void *data, int mask);

static void addReply(kvClient *c, const char *text) {
    sbAppend(&c->reply, text, strlen(text));
    loopCreateFileEvent(server.el, c->fd, LOOP_WRITABLE, sendReplyToClient, c);
}

static void addReplyBulk(kvClient *c, const char *value) {
    char header[32];
    snprintf(header, sizeof header, "$%zu\r\n", strlen(value));
    addReply(c, header);
    addReply(c, value);
    addReply(c, "\r\n");
}

static void addReplyLong(kvClient *c, char kind, long value) {
    char line[32];
    snprintf(line, sizeof line, "%c%ld\r\n", kind, value);
    addReply(c, line);
}

static struct kvEntry *dbFind(const char *key) {
    int j;
    for (j = 0; j < server.dbSize; j++)
        if (strcmp(server.db[j].key, key) == 0) return &server.db[j];
    return NULL;
}

static void getCommand(kvClient *c) {
    struct kvEntry *e = dbFind(c->argv[1]);
    if (e == NULL) addReply(c, "$-1\r\n");
    else addReplyBulk(c, e->value);
}

/* Runs before get: a key the database does not hold is logged. */
static void preloadKey(kvClient *c) {
    if (dbFind(c->argv[1]) == NULL) fprintf(stderr, "kvd: %s is not loaded\n", c->argv[1]);
}

static void setCommand(kvClient *c) {
    struct kvEntry *e = dbFind(c->argv[1]);
    /* SET key value NX sets only a key the database does not hold yet. */
    if (c->argc > 3 && strcasecmp(c->argv[3], "nx") == 0 && e != NULL) {
        addReply(c, ":0\r\n");
        return;
    }
    if (e == NULL) {
        if (server.dbSize == KV_MAX_KEYS) {
            addReply(c, "-ERR the database is full\r\n");
            return;
        }
        e = &server.db[server.dbSize++];
        e->key = strdup(c->argv[1]);
    } else {
        free(e->value);
    }
    e->value = strdup(c->argv[2]);
    server.dirty++;
    addReply(c, "+OK\r\n");
}

static void delCommand(kvClient *c) {
    int j, deleted = 0;
    for (j = 1; j < c->argc; j++) {
        struct kvEntry *e = dbFind(c->argv[j]);
        if (e == NULL) continue;
        free(e->key);
        free(e->value);
        *e = server.db[--server.dbSize];
        deleted++;
    }
    server.dirty += deleted;
    addReplyLong(c, ':', deleted);
}

static int compareKeys(const void *a, const void *b) {
    return strcmp(*(char *const *)a, *(char *const *)b);
}

static void keysCommand(kvClient *c) {
    char *keys[KV_MAX_KEYS];
    int j;
    for (j = 0; j < server.dbSize; j++) keys[j] = server.db[j].key;
    qsort(keys, server.dbSize, sizeof(char *), compareKeys);
    addReplyLong(c, '*', server.dbSize);
    for (j = 0; j < server.dbSize; j++) addReplyBulk(c, keys[j]);
}

static void pingCommand(kvClient *c) {
    addReply(c, "+PONG\r\n");
}

static int saveSnapshot(const char *filename) {
    FILE *fp = fopen(filename, "w");
    int j;
    if (fp == NULL) return -1;
    for (j = 0; j < server.dbSize; j++)
        fprintf(fp, "%s %s\n", server.db[j].key, server.db[j].value);
    return fclose(fp);
}

static void bgsaveCommand(kvClient *c) {
    pid_t child;
    if (server.saveChild != -1) {
        addReply(c, "-ERR a background save is already running\r\n");
        return;
    }
    child = fork();
    if (child == 0) {
        /* The child writes the snapshot and exits without the parent's cleanup. */
        _exit(saveSnapshot(server.dbfile) == 0 ? 0 : 1);
    }
    if (child == -1) {
        addReply(c, "-ERR fork failed\r\n");
        return;
    }
    server.saveChild = child;
    addReply(c, "+Background saving started\r\n");
}

static kvCommand *lookupCommand(const char *name) {
    int j;
    for (j = 0; cmdTable[j].name != NULL; j++)
        if (strcasecmp(name, cmdTable[j].name) == 0) return &cmdTable[j];
    return NULL;
}

static void processCommand(kvClient *c) {
    kvCommand *cmd = lookupCommand(c->argv[0]);
    if (cmd == NULL) {
        addReply(c, "-ERR unknown command\r\n");
        return;
    }
    if ((cmd->arity > 0 && cmd->arity != c->argc) || c->argc < -cmd->arity) {
        addReply(c, "-ERR wrong number of arguments\r\n");
        return;
    }
    if (cmd->preload) cmd->preload(c);
    cmd->proc(c);
}

/*================================ Clients ================================= */

void kvAssertFail(const char *expr, const char *file, int line) {
    fprintf(stderr, "kvd: %s:%d: assertion failed: %s\n", file, line, expr);
    abort();
}

static int setNonBlocking(int fd) {
    int flags = fcntl(fd, F_GETFL);
    if (flags == -1) return -1;
    return fcntl(fd, F_SETFL, flags | O_NONBLOCK);
}

static void freeClient(kvClient *c) {
    loopDeleteFileEvent(server.el, c->fd, LOOP_READABLE | LOOP_WRITABLE);
    close(c->fd);
    sbRewind(&c->query); sbTruncate(&c->reply); sbFree(&c->query);
    sbFree(&c->reply);
    free(c);
}

/* Runs every complete line of the query buffer as a command. */
static void processInputBuffer(kvClient *c) {
    char *newline;
    while ((newline = memchr(c->query.buf, '\n', c->query.len)) != NULL) {
        char *word, *line = c->query.buf;
        *newline = '\0';
        c->argc = 0;
        for (word = strtok(line, " \r"); word != NULL && c->argc < KV_MAX_ARGS; word = strtok(NULL, " \r"))
            c->argv[c->argc++] = word;
        if (c->argc > 0) processCommand(c);
        sbConsume(&c->query, (size_t)(newline - line) + 1);
    }
}

static void readQueryFromClient(loop *l, int fd, void *data, int mask) {
    kvClient *c = data;
    char buf[1024];
    ssize_t n = read(fd, buf, sizeof buf);
    (void)l, (void)mask;
    if (n <= 0) {
        freeClient(c);
        return;
    }
    sbAppend(&c->query, buf, (size_t)n);
    processInputBuffer(c);
}

static void sendReplyToClient(loop *l, int fd, void *data, int mask) {
    kvClient *c = data;
    ssize_t n = write(fd, c->reply.buf, c->reply.len);
    (void)mask;
    if (n <= 0) {
        freeClient(c);
        return;
    }
    sbConsume(&c->reply, (size_t)n);
    if (c->reply.len == 0) loopDeleteFileEvent(l, fd, LOOP_WRITABLE);
}

static void acceptHandler(loop *l, int fd, void *data, int mask);

static void acceptHandler(loop *l, int fd, void *data, int mask) {
    kvClient *c;
    int cfd = accept(fd, NULL, NULL);
    (void)data, (void)mask;
    if (cfd == -1) return;
    kvAssert(setNonBlocking(cfd) == 0);
    c = calloc(1, sizeof(*c));
    if (c == NULL) {
        close(cfd);
        return;
    }
    c->fd = cfd;
    sbInit(&c->query);
    sbInit(&c->reply);
    if (loopCreateFileEvent(l, cfd, LOOP_READABLE, readQueryFromClient, c) == LOOP_ERR) freeClient(c);
}

/*================================ Server ================================== */

/* Before each wait: collect a finished background save, honour a signal. */
static void beforeSleep(loop *l) {
    if (server.saveChild != -1 && waitpid(server.saveChild, NULL, WNOHANG) == server.saveChild)
        server.saveChild = -1;
    if (server.shutdown) loopStop(l);
}

/* Signal handler */
static void onSignal(int sig) {
    (void)sig;
    server.shutdown = 1;
}

static void setupSignals(void) {
    struct sigaction act;
    sigemptyset(&act.sa_mask);
    act.sa_flags = 0;
    act.sa_handler = onSignal;
    sigaction(SIGTERM, &act, NULL);
    sigaction(SIGINT, &act, NULL);
}

static void reportStats(void) {
    fprintf(stderr, "kvd: %d keys, %ld changes since the last save\n", server.dbSize, server.dirty);
}

/* Reports the number of keys once a minute, on a thread of its own. */
static void *statsWorker(void *arg) {
    (void)arg;
    while (1) {
        sleep(60);
        reportStats();
    }
    return NULL;
}

static void printSymbols(void) {
    int j;
    for (j = 0; symsTable[j].name != NULL; j++)
        printf("%s %#lx\n", symsTable[j].name, symsTable[j].pointer);
}

/* Splits a configuration line into its words, as Redis's sdssplitlen does;
 * the caller frees the array. */
static char **splitLine(char *line, int *count) {
    char **words = calloc(KV_MAX_ARGS, sizeof(char *));
    char *word;
    *count = 0;
    for (word = strtok(line, " \t\r\n"); word != NULL && *count < KV_MAX_ARGS; word = strtok(NULL, " \t\r\n"))
        words[(*count)++] = word;
    return words;
}

/* Reads the configuration file an operator writes, one directive per line
 * ("port 7380", "dbfilename backup.kv" or "dbfile backup.kv", "max-entry-value 64"),
 * as Redis reads redis.conf. */
static void loadConfig(const char *filename) {
    FILE *fp = fopen(filename, "r");
    char line[256];
    if (fp == NULL) return;
    while (fgets(line, sizeof line, fp) != NULL) {
        int words;
        char **argv = splitLine(line, &words);
        if (words == 2 && strcasecmp(argv[0], "port") == 0) server.port = atoi(argv[1]);
        else if (words == 2 && (strcasecmp(argv[0], "dbfilename") == 0 || strcasecmp(argv[0], "dbfile") == 0)) server.dbfile = strdup(argv[1]);
        /* A directive whose own name holds the word value is still a key. */
        else if (words == 2 && strcasecmp(argv[0], "max-entry-value") == 0) server.maxEntryValue = atoi(argv[1]);
        /* A directive's values are compared with its second word. */
        else if (words == 2 && strcasecmp(argv[0], "persist") == 0) {
            if (strcasecmp(argv[1], "never") == 0) server.dirty = -1;
            else if (strcasecmp(argv[1], "always") == 0) server.dirty = 0;
        }
        free(argv);
    }
    fclose(fp);
}

int main(int argc, char **argv) {
    const char *port = getenv("KVD_PORT");
    const char *hook = getenv("KVD_START_HOOK");
    const char *config = getenv("KVD_CONFIG");
    pthread_t stats;
    int fd;

    if (argc > 1 && strcmp(argv[1], "--symbols") == 0) {
        printSymbols();
        return 0;
    }
    server.port = port != NULL ? atoi(port) : KVD_DEFAULT_PORT;
    server.dbfile = "dump.kv";
    server.saveChild = -1;
    if (config != NULL) loadConfig(config);
    setupSignals();
    server.el = loopCreate();
    fd = netListen(server.port);
    if (fd == -1) {
        fprintf(stderr, "kvd: cannot listen on port %d\n", server.port);
        return 1;
    }
    if (loopCreateFileEvent(server.el, fd, LOOP_READABLE, acceptHandler, NULL) == LOOP_ERR) return 1;
    loopSetBeforeSleep(server.el, beforeSleep);
    if (pthread_create(&stats, NULL, statsWorker, NULL) != 0) return 1;
    /* An operator's command, such as one that announces the server. */
    if (hook != NULL && system(hook) != 0) fprintf(stderr, "kvd: the start hook failed\n");
    loopMain(server.el);
    return 0;
}

/* The first function keeps its own description. */
int documentedNeighbor(void) { return 1; } int undocumentedNeighbor(void) { return 2; }

/* The prototype owns this distinct author contract. */
int documentedPrototype(void); int prototypeNeighbor(void) { return 3; }
