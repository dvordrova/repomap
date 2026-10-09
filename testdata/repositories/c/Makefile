# kvd: a small key-value server and its command-line client.
# Both programs link strbuf.o, net.o and loop.o; the client never runs the
# event loop it links. LOOP_POLL asks for the portable poll(2) backend of the
# event loop on every host; without it Linux uses epoll.
CFLAGS = -std=c99 -O2 -g -Wall -D_DEFAULT_SOURCE -DLOOP_POLL

# The first fragment fixes the default goal; its prerequisites and links live
# in the included build description, relative to this invocation directory.
BUILD_RULES = build/main.mk
all:
include $(BUILD_RULES)

kvd.o: kvd.c kvd.h loop.h net.h strbuf.h staticsyms.h
kvcli.o: kvcli.c net.h strbuf.h
repl.o: repl.c net.h
loop.o: loop.c loop.h loop_poll.c loop_epoll.c
net.o: net.c net.h
strbuf.o: strbuf.c strbuf.h

.c.o:
	$(CC) -c $(CFLAGS) $<

# A smoke test: start the server, ping it, stop it.
test: kvd kvcli
	./kvd & echo $$! > kvd.pid; sleep 1; ./kvcli ping; status=$$?; kill `cat kvd.pid`; rm -f kvd.pid; exit $$status

# Build the portable poll backend with additional compiler diagnostics.
PORTABLE_CFLAGS = -std=c99 -O2 -g -Wall -Wextra -Wformat=2 -Wstrict-prototypes -Wmissing-prototypes -Wpointer-arith -Wcast-qual -D_DEFAULT_SOURCE -DLOOP_POLL
portable:
	$(MAKE) CFLAGS="$(PORTABLE_CFLAGS)" all

clean:
	rm -f kvd kvcli *.o dump.kv
