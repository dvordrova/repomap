# kvd

A small in-memory key-value server, `kvd`, and its command-line client,
`kvcli`. A client sends one command per line: `get`, `set`, `del`, `keys`,
`ping` or `bgsave`.

`make` builds both programs; `make test` starts the server, pings it and stops
it. `tools/dump.c` prints the keys of a snapshot and is built by hand:
`cc -o dump tools/dump.c strbuf.c`.

The server listens on `KVD_PORT` (7379 by default), lets `KVD_BACKLOG`
connections wait (16 by default) and runs the shell command
in `KVD_START_HOOK`, if set, once it is listening. `bgsave` writes `dump.kv`
from a forked child. The client connects to `KVD_HOST` (`127.0.0.1` by
default, optionally written `kvd://127.0.0.1`) and `KVD_PORT`.
