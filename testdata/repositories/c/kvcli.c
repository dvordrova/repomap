/* kvcli: sends one command to kvd and prints the reply. */
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <strings.h>
#include <unistd.h>

#include "net.h"
#include "strbuf.h"

/* The interactive mode, in repl.c. */
int repl(const char *host, int port);

/* The commands the client checks before sending: the same names as the
 * server's table, which also names the functions that run them. */
static struct cliCommand {
    const char *name;
    int arity;
} cmdTable[] = {
    {"get", 2},
    {"set", 3},
    {"del", -2},
    {"keys", 1},
    {"ping", 1},
    {"bgsave", 1},
    {NULL, 0}
};

static struct cliCommand *lookupCommand(const char *name) {
    int j;
    for (j = 0; cmdTable[j].name != NULL; j++)
        if (strcasecmp(name, cmdTable[j].name) == 0) return &cmdTable[j];
    return NULL;
}

/* KVD_HOST may be written kvd://127.0.0.1; the scheme is optional. */
static const char *withoutScheme(const char *host) {
    return strncmp(host, "kvd://", strlen("kvd://")) == 0 ? host + strlen("kvd://") : host;
}

/* A short option before the command: the switch compares its letter with
 * each option's, one comparison of arg[1] for the whole switch ('h' and
 * '?' stacked on one body are one case). C compares strings by calls
 * (strcasecmp above), each its own call fact. */
static int shortOption(const char *arg) {
    switch (arg[1]) {
    case 'h':
    case '?':
        fprintf(stderr, "usage: kvcli [-h] [-V] [--raw] command [argument ...]\n");
        return 2;
    case 'V':
        puts("kvcli 1.0");
        return 0;
    default:
        return -1;
    }
}

/* bench sends its PINGs the number of times its --requests option says.
 * Only main's bench branch runs it, so --requests, which it compares its own
 * arguments with, is bench's option, not the client's. */
static int bench(int argc, char **argv) {
    int j, requests = 100;
    for (j = 1; j < argc; j++)
        if (strcasecmp(argv[j], "--requests") == 0 && j + 1 < argc) requests = atoi(argv[++j]);
    printf("bench: %d requests\n", requests);
    return 0;
}

int main(int argc, char **argv) {
    const char *host = getenv("KVD_HOST");
    const char *port = getenv("KVD_PORT");
    struct cliCommand *cmd;
    strbuf request;
    char reply[4096];
    ssize_t n;
    int fd, j, first = 1;

    /* Without a command, the client reads its commands from its input. */
    if (argc == 1) return repl(host != NULL ? withoutScheme(host) : "127.0.0.1", port != NULL ? atoi(port) : 7379);
    if (argv[1][0] == '-' && argv[1][1] != '-') return shortOption(argv[1]);
    /* bench, a subcommand of the client, runs its own code. */
    if (strcasecmp(argv[1], "bench") == 0) return bench(argc - 1, argv + 1);
    /* --raw, an option of the client, comes before the command. */
    if (argc > 2 && strcasecmp(argv[1], "--raw") == 0) first = 2;
    if (argc <= first) {
        fprintf(stderr, "usage: kvcli [--raw] command [argument ...]\n");
        return 2;
    }
    cmd = lookupCommand(argv[first]);
    if (cmd == NULL || (cmd->arity > 0 && argc - first != cmd->arity) || argc - first < -cmd->arity) {
        fprintf(stderr, "kvcli: unknown command or wrong number of arguments\n");
        return 2;
    }
    /* The same comparison of a command's own name is no option. */
    if (first == 1 && strcasecmp(cmd->name, "bgsave") == 0) fprintf(stderr, "kvcli: the save runs in the background\n");
    fd = netConnect(host != NULL ? withoutScheme(host) : "127.0.0.1", port != NULL ? atoi(port) : 7379);
    if (fd == -1) {
        perror("kvcli: connect");
        return 1;
    }
    sbInit(&request);
    for (j = first; j < argc; j++) {
        if (j > first) sbAppend(&request, " ", 1);
        sbAppend(&request, argv[j], strlen(argv[j]));
    }
    sbAppend(&request, "\n", 1);
    if (write(fd, request.buf, request.len) != (ssize_t)request.len) {
        perror("kvcli: write");
        return 1;
    }
    n = read(fd, reply, sizeof reply - 1);
    if (n <= 0) {
        perror("kvcli: read");
        return 1;
    }
    reply[n] = '\0';
    fputs(reply, stdout);
    sbFree(&request);
    close(fd);
    return 0;
}
