# The wire format kvd's values travel in, as a library and a filter. `make`
# asks for a platform; `make all` builds libwire.a and wirecat, and
# `make raw` links wirecat-raw with the escape that changes nothing.
LIB = libwire.a
CFLAGS = -O2 -Wall

default:
	@echo 'Please choose a platform: posix'

posix: all

all: $(LIB) wirecat

$(LIB): encode.o escape.o
	$(AR) rcs $@ encode.o escape.o

wirecat: wirecat.o $(LIB)
	$(CC) -o $@ wirecat.o $(LIB)

raw: wirecat.o encode.o escape_none.o
	$(CC) -o wirecat-raw wirecat.o encode.o escape_none.o
