/* net.c - the socket calls kvd and kvcli share. Both programs link it, but
 * only the server listens and only the client connects. */
#include <arpa/inet.h>
#include <netinet/in.h>
#include <stdlib.h>
#include <string.h>
#include <sys/socket.h>
#include <unistd.h>

#include "net.h"

/* Listens on the loopback address; KVD_BACKLOG connections may wait. */
int netListen(int port) {
    const char *backlog = getenv("KVD_BACKLOG");
    struct sockaddr_in sa;
    int on = 1, fd = socket(AF_INET, SOCK_STREAM, 0);
    if (fd == -1) return -1;
    setsockopt(fd, SOL_SOCKET, SO_REUSEADDR, &on, sizeof on);
    memset(&sa, 0, sizeof sa);
    sa.sin_family = AF_INET;
    sa.sin_port = htons((unsigned short)port);
    sa.sin_addr.s_addr = htonl(INADDR_LOOPBACK);
    if (bind(fd, (struct sockaddr *)&sa, sizeof sa) == -1 || listen(fd, backlog != NULL ? atoi(backlog) : 16) == -1) {
        close(fd);
        return -1;
    }
    return fd;
}

int netConnect(const char *host, int port) {
    struct sockaddr_in sa;
    int fd = socket(AF_INET, SOCK_STREAM, 0);
    if (fd == -1) return -1;
    memset(&sa, 0, sizeof sa);
    sa.sin_family = AF_INET;
    sa.sin_port = htons((unsigned short)port);
    if (inet_pton(AF_INET, host, &sa.sin_addr) != 1 || connect(fd, (struct sockaddr *)&sa, sizeof sa) == -1) {
        close(fd);
        return -1;
    }
    return fd;
}
