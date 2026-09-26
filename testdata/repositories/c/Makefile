# kvd: a small key-value server and its command-line client.
# Both programs link strbuf.o. LOOP_POLL picks the portable poll(2) backend
# of the event loop; without it loop.c would use the Linux-only epoll one.
CFLAGS = -std=c99 -O2 -g -Wall -D_DEFAULT_SOURCE -DLOOP_POLL

all: kvd kvcli

kvd: kvd.o loop.o strbuf.o
	$(CC) -o kvd kvd.o loop.o strbuf.o -pthread

kvcli: kvcli.o strbuf.o
	$(CC) -o kvcli kvcli.o strbuf.o

kvd.o: kvd.c kvd.h loop.h strbuf.h staticsyms.h
kvcli.o: kvcli.c strbuf.h
loop.o: loop.c loop.h loop_poll.c loop_epoll.c
strbuf.o: strbuf.c strbuf.h

.c.o:
	$(CC) -c $(CFLAGS) $<

# A smoke test: start the server, ping it, stop it.
test: kvd kvcli
	./kvd & echo $$! > kvd.pid; sleep 1; ./kvcli ping; status=$$?; kill `cat kvd.pid`; rm -f kvd.pid; exit $$status

clean:
	rm -f kvd kvcli *.o dump.kv
