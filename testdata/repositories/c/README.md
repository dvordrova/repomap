# kvd

A small in-memory key-value server, `kvd`, and its command-line client,
`kvcli`. A client sends one command per line: `get`, `set`, `del`, `keys`,
`ping` or `bgsave`.

`make` builds both programs; `make test` starts the server, pings it and stops
it. `tools/dump.c` prints the keys of a snapshot and is built by hand:
`cc -o dump tools/dump.c strbuf.c`. Three directories have makefiles of their
own that the root `Makefile` never enters: `cd upper && make` builds
`upper.so`, a module that uppercases a value in place; `cd util && make`
lists the examples (`make ping` builds `util/ping`, `make watch` times a
connection with the event loop's clock and `make watch-replay` with the fixed
clock of `util/fixedclock.c`); and `cd wire && make all` builds `libwire.a`,
the wire format values travel in (its API is `wire.h`; `wire_internal.h`
is shared by its own files), and `wirecat`, which writes its input in
it (`make raw` links `wirecat-raw` with the escape that changes nothing;
`wire/selftest.c` is built by hand: `cc -o selftest selftest.c encode.c
escape.c`).

The server listens on `KVD_PORT` (7379 by default), lets `KVD_BACKLOG`
connections wait (16 by default) and runs the shell command
in `KVD_START_HOOK`, if set, once it is listening. `KVD_CONFIG` names a
configuration file of one directive per line: `port 7380` and
`dbfilename backup.kv`. `bgsave` writes `dump.kv`
from a forked child. The client connects to `KVD_HOST` (`127.0.0.1` by
default, optionally written `kvd://127.0.0.1`) and `KVD_PORT`; its option
`--raw` comes before the command. Without a command it reads commands from
its standard input, one per line, and sends each over a connection of its
own.
