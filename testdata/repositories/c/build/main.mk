# The server and client are outputs of the root Makefile, not build/ programs.
LINK_THREADS = -pthread
all: kvd kvcli

kvd: kvd.o loop.o net.o strbuf.o
	$(CC) -o kvd kvd.o loop.o net.o strbuf.o $(LINK_THREADS)

kvcli: kvcli.o repl.o net.o strbuf.o loop.o
	$(CC) -o kvcli kvcli.o repl.o net.o strbuf.o loop.o
