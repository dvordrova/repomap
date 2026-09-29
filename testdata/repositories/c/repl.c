/* repl.c - kvcli's interactive mode: without a command on its command
 * line, the client sends each line of its standard input as one command,
 * over a connection of its own, and prints each reply. */
#include <stdio.h>
#include <string.h>
#include <unistd.h>

#include "net.h"

int repl(const char *host, int port) {
    char line[4096], reply[4096];
    ssize_t n;
    int fd;

    while (fgets(line, sizeof line, stdin) != NULL) {
        fd = netConnect(host, port);
        if (fd == -1) return 1;
        if (write(fd, line, strlen(line)) == (ssize_t)strlen(line) && (n = read(fd, reply, sizeof reply - 1)) > 0) {
            reply[n] = '\0';
            fputs(reply, stdout);
        }
        close(fd);
    }
    return 0;
}
