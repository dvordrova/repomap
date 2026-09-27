#ifndef NET_H
#define NET_H

/* TCP sockets on the loopback address: kvd listens, kvcli connects. */
int netListen(int port);
int netConnect(const char *host, int port);

#endif
