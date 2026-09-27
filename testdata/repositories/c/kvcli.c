/* kvcli: sends one command to kvd and prints the reply. */
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <strings.h>
#include <unistd.h>

#include "net.h"
#include "strbuf.h"

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

int main(int argc, char **argv) {
    const char *host = getenv("KVD_HOST");
    const char *port = getenv("KVD_PORT");
    struct cliCommand *cmd;
    strbuf request;
    char reply[4096];
    ssize_t n;
    int fd, j;

    if (argc < 2) {
        fprintf(stderr, "usage: kvcli command [argument ...]\n");
        return 2;
    }
    cmd = lookupCommand(argv[1]);
    if (cmd == NULL || (cmd->arity > 0 && argc - 1 != cmd->arity) || argc - 1 < -cmd->arity) {
        fprintf(stderr, "kvcli: unknown command or wrong number of arguments\n");
        return 2;
    }
    fd = netConnect(host != NULL ? withoutScheme(host) : "127.0.0.1", port != NULL ? atoi(port) : 7379);
    if (fd == -1) {
        perror("kvcli: connect");
        return 1;
    }
    sbInit(&request);
    for (j = 1; j < argc; j++) {
        if (j > 1) sbAppend(&request, " ", 1);
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
