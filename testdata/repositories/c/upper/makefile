# upper: a module kvd may load, built in its own directory with `cd upper &&
# make`; the root Makefile never enters it. -I.. finds the server's headers.
CFLAGS = -Wall -O2 -I.. -fPIC -shared

all: upper.so

upper.so: upper.c ../strbuf.h
	$(CC) $(CFLAGS) -o upper.so upper.c
