"""Ground-truth inventory of redis-1.3.6 for the claim audit (internal/audit).

Run from anywhere: python3 testdata/audit/redis-1.3.6/generate.py
It reads the checkout ~/git/redis-1.3.6 at the pinned revision (PINNED below)
only, refuses any other HEAD, and rewrites inventory.json beside this script. After the freeze a
change is a dated erratum with a code citation, never a silent edit.
"""
import re, json, subprocess, os
REPO = os.path.expanduser("~/git/redis-1.3.6")
PINNED = "7b7f987e9184645f64f766b7f8f7eb02a9a69552"
OUT = os.path.join(os.path.dirname(os.path.abspath(__file__)), "inventory.json")
os.chdir(REPO)
rev = subprocess.check_output(["git", "rev-parse", "HEAD"]).decode().strip()
assert rev == PINNED, f"{REPO} is at {rev}, the inventory is pinned to {PINNED}"
items = []


def add(component, inventory, kind, name, anchor, handler, must, why, **extra):
    d = {"component": component, "inventory": inventory, "kind": kind, "name": name,
         "anchor": anchor, "handler": handler or None, "must": must, "why": why}
    d.update(extra)
    items.append(d)


# ---------- redis-server requests: parsed from cmdTable ----------
desc = {
    "get": "read a string value", "set": "store a string value", "setnx": "store a string only if the key is absent",
    "append": "append to a string value", "substr": "return a substring of a string value", "del": "delete keys",
    "exists": "test whether a key exists", "incr": "increment an integer value", "decr": "decrement an integer value",
    "mget": "read several string values", "rpush": "push onto a list tail", "lpush": "push onto a list head",
    "rpop": "pop from a list tail", "lpop": "pop from a list head",
    "brpop": "blocking tail pop with timeout (client is blocked)",
    "blpop": "blocking head pop with timeout (client is blocked)", "llen": "list length",
    "lindex": "read a list element by index", "lset": "overwrite a list element", "lrange": "read a list range",
    "ltrim": "trim a list to a range", "lrem": "remove list elements equal to a value",
    "rpoplpush": "atomically move a list tail element to another list head", "sadd": "add a set member",
    "srem": "remove a set member", "smove": "move a member between sets", "sismember": "test set membership",
    "scard": "set cardinality", "spop": "pop a random set member", "srandmember": "read a random set member",
    "sinter": "intersect sets", "sinterstore": "intersect sets into a destination key", "sunion": "union sets",
    "sunionstore": "union sets into a destination key", "sdiff": "difference of sets",
    "sdiffstore": "difference of sets into a destination key",
    "smembers": "list set members; served by sinterCommand with one key",
    "zadd": "add a sorted-set member with score", "zincrby": "increment a sorted-set member score",
    "zrem": "remove a sorted-set member", "zremrangebyscore": "remove sorted-set members by score range",
    "zremrangebyrank": "remove sorted-set members by rank range",
    "zunion": "union sorted sets into a destination key (WEIGHTS/AGGREGATE)",
    "zinter": "intersect sorted sets into a destination key (WEIGHTS/AGGREGATE)",
    "zrange": "read a sorted-set range by rank", "zrangebyscore": "read a sorted-set range by score",
    "zcount": "count sorted-set members in a score range", "zrevrange": "read a sorted-set range by reverse rank",
    "zcard": "sorted-set cardinality", "zscore": "read a member score", "zrank": "rank of a member",
    "zrevrank": "reverse rank of a member", "hset": "set a hash field", "hget": "read a hash field",
    "hdel": "delete a hash field", "hlen": "hash field count", "hkeys": "list hash fields",
    "hvals": "list hash values", "hgetall": "list hash fields and values", "hexists": "test a hash field",
    "incrby": "increment an integer by an amount", "decrby": "decrement an integer by an amount",
    "getset": "set a string and return the old value", "mset": "set several strings",
    "msetnx": "set several strings only if none exists", "randomkey": "return a random key",
    "select": "switch the client's database", "move": "move a key to another database", "rename": "rename a key",
    "renamenx": "rename a key only if the target is absent", "expire": "set a key time-to-live",
    "expireat": "set a key expiry as a unix time", "keys": "list keys matching a glob pattern",
    "dbsize": "count keys in the current database", "auth": "authenticate against requirepass",
    "ping": "liveness check (+PONG)", "echo": "echo the argument",
    "save": "synchronous snapshot to dbfilename (rdbSave)",
    "bgsave": "fork a background snapshot child (rdbSaveBackground)",
    "bgrewriteaof": "fork a background append-only-file rewrite child",
    "shutdown": "save (or fsync the AOF) and exit the server", "lastsave": "unix time of the last successful save",
    "type": "type of the value at a key", "multi": "start a MULTI transaction (later commands are queued)",
    "exec": "execute queued MULTI commands", "discard": "drop queued MULTI commands",
    "sync": "replication handshake from a slave: BGSAVE, stream dump.rdb, then feed writes",
    "flushdb": "empty the current database", "flushall": "empty all databases",
    "sort": "sort a list/set/zset (BY/GET/LIMIT/STORE options)", "info": "server statistics and state as a bulk reply",
    "monitor": "turn the connection into a feed of all executed commands", "ttl": "remaining time-to-live of a key",
    "slaveof": "make this server a slave of host:port, or a master again with NO ONE",
    "debug": "debugging subcommands (SEGFAULT, RELOAD, LOADAOF, OBJECT, SWAPOUT)",
}
srvtab = {}
for i, l in enumerate(open("redis.c", encoding="latin-1"), 1):
    if 704 <= i <= 798:
        m = re.match(r'\s*\{"(\w+)",(\w+),(-?\d+),([A-Z_|]+),', l)
        name, handler, arity, flags = m.group(1), m.group(2), int(m.group(3)), m.group(4)
        srvtab[name] = i
        mode = "bulk last argument" if "BULK" in flags else "inline"
        add("redis-server", "requests", "protocol-command", name, f"redis.c:{i}", handler, True,
            f"cmdTable entry ({mode}, arity {arity}): {desc[name]}.")
assert len(srvtab) == 95, len(srvtab)
add("redis-server", "requests", "protocol-command", "quit", "redis.c:2148", "processCommand", True,
    "Not in cmdTable: processCommand special-cases QUIT and closes the connection.")
add("redis-server", "requests", "connection", "TCP accept on port (default 6379) / bind address", "redis.c:1576",
    "acceptHandler", False,
    "Listening socket (anetTcpServer, redis.c:1552) accepts clients; enforces maxclients; each client gets readQueryFromClient.")
add("redis-server", "requests", "protocol-stream", "replication stream from the master (when slave)", "redis.c:7318",
    "syncWithMaster", False,
    "After SYNC the master link becomes a client flagged REDIS_MASTER; its writes run through the same cmdTable.")

# ---------- redis-server commands ----------
add("redis-server", "commands", "arg", "[/path/to/redis.conf]", "redis.c:9130", "loadServerConfig", True,
    "Single optional positional argument; more than one prints usage and exits; without it defaults are used.")
directives = [
    ("timeout", 1646, "idle client timeout in seconds (0 disables)"), ("port", 1651, "TCP listen port"),
    ("bind", 1656, "listen address"), ("save", 1658, "save point <seconds> <changes> triggering a background save"),
    ("dir", 1665, "chdir to the working directory holding dump, AOF and temp files"),
    ("loglevel", 1671, "log verbosity"), ("logfile", 1680, "log file path or stdout"),
    ("databases", 1699, "number of databases (server.dbnum)"), ("maxclients", 1704, "maximum simultaneous clients"),
    ("maxmemory", 1706, "memory limit; DENYOOM commands are refused above it"),
    ("slaveof", 1708, "start as a slave of <masterip> <masterport>"),
    ("masterauth", 1712, "password sent with AUTH to the master"),
    ("glueoutputbuf", 1714, "glue small reply buffers before writing (yes/no)"),
    ("shareobjects", 1718, "share equal string objects (yes/no)"),
    ("rdbcompression", 1722, "LZF-compress strings in dump.rdb (yes/no)"),
    ("shareobjectspoolsize", 1726, "size of the object sharing pool"),
    ("daemonize", 1731, "fork into the background and write the pid file (yes/no)"),
    ("appendonly", 1735, "enable the append-only file (yes/no)"), ("appendfsync", 1739, "AOF fsync policy"),
    ("requirepass", 1750, "password clients must send with AUTH"),
    ("pidfile", 1752, "pid file path used when daemonized"), ("dbfilename", 1754, "snapshot file name"),
    ("vm-enabled", 1756, "enable virtual-memory swapping (yes/no)"),
    ("vm-swap-file", 1760, "swap file path (%p expands to the pid)"),
    ("vm-max-memory", 1763, "memory above which values are swapped out"),
    ("vm-page-size", 1765, "swap page size in bytes"), ("vm-pages", 1767, "number of swap pages"),
    ("vm-max-threads", 1769, "maximum VM I/O threads (0 = blocking VM)"),
    ("hash-max-zipmap-entries", 1771, "hash size limit for zipmap encoding"),
    ("hash-max-zipmap-value", 1773, "hash value length limit for zipmap encoding"),
]
for n, l, w in directives:
    add("redis-server", "commands", "setting", n, f"redis.c:{l}", "loadServerConfig", True, f"Config directive: {w}.")

# ---------- redis-server workers ----------
add("redis-server", "workers", "timer", "serverCron", "redis.c:1575", "serverCron", True,
    "1-second time event: save points, key expiry, idle timeouts, child reaping, VM swap-out, master reconnect.")
add("redis-server", "workers", "forked-child", "background save child (BGSAVE)", "redis.c:3360", "rdbSaveBackground",
    True, "fork(); the child writes the snapshot with rdbSave and exits; started by BGSAVE, SYNC and save points.")
add("redis-server", "workers", "forked-child", "background AOF rewrite child (BGREWRITEAOF)", "redis.c:7876",
    "rewriteAppendOnlyFileBackground", True,
    "fork(); the child rewrites the AOF into a temp file while the parent buffers the diff.")
add("redis-server", "workers", "thread", "VM I/O threads", "redis.c:8728", "IOThreadEntryPoint", True,
    "pthread_create in spawnIOThread (up to vm-max-threads) to load, size and write swapped values; only with vm-enabled.")
add("redis-server", "workers", "event-callback", "VM completed-job pipe handler", "redis.c:8031",
    "vmThreadedIOCompletedJob", False, "Main-thread handler woken through a pipe by I/O threads to apply finished jobs.")
add("redis-server", "workers", "event-callback", "beforeSleep", "redis.c:9152", "beforeSleep", False,
    "Runs before each event-loop wait; resumes clients whose swapped keys were loaded.")
add("redis-server", "workers", "event-callback", "sendBulkToSlave", "redis.c:7194", "sendBulkToSlave", False,
    "Writable handler that streams dump.rdb to a syncing slave after the BGSAVE finishes.")
add("redis-server", "workers", "cron-step", "save-point check", "redis.c:1322", "serverCron", False,
    "Inside serverCron: starts rdbSaveBackground when a save <seconds> <changes> point is reached.")
add("redis-server", "workers", "cron-step", "child reaping (wait3)", "redis.c:1304",
    "backgroundSaveDoneHandler / backgroundRewriteDoneHandler", False,
    "Inside serverCron: collects finished save/rewrite children and finalizes their files.")
add("redis-server", "workers", "cron-step", "active key expiry", "redis.c:1352", "serverCron", False,
    "Inside serverCron: samples keys with an expire and deletes expired ones.")
add("redis-server", "workers", "cron-step", "idle client and blocking-pop timeouts", "redis.c:1297",
    "closeTimedoutClients", False, "Inside serverCron: closes idle clients and unblocks timed-out BLPOP/BRPOP.")
add("redis-server", "workers", "cron-step", "VM swap-out", "redis.c:1368",
    "vmSwapOneObjectBlocking / vmSwapOneObjectThreaded", False,
    "Inside serverCron: swaps values out while memory exceeds vm-max-memory.")
add("redis-server", "workers", "cron-step", "master (re)connect", "redis.c:1388", "syncWithMaster", False,
    "Inside serverCron: when replstate is REDIS_REPL_CONNECT, performs the blocking sync with the master.")
add("redis-server", "workers", "signal-handler", "segvHandler", "redis.c:9233", "segvHandler", False,
    "SIGSEGV/SIGBUS/SIGFPE/SIGILL handler logging INFO and a backtrace (HAVE_BACKTRACE builds only).")

# ---------- redis-server external ----------
add("redis-server", "external", "tcp-connection", "master Redis server (slaveof host port)", "redis.c:7219",
    "syncWithMaster", True,
    "anetTcpConnect to masterhost:masterport; sends AUTH (if masterauth) and SYNC, downloads the dump, keeps the link.")
add("redis-server", "external", "dns", "host name resolution for the master host", "anet.c:145",
    "anetTcpConnect", False, "gethostbyname when masterhost is not a dotted IPv4 address; part of the master connection.")
add("redis-server", "external", "outgoing-protocol", "AUTH <masterauth> to the master", "redis.c:7230",
    "syncWithMaster", False, "Sent over the master connection only when masterauth is set.")
add("redis-server", "external", "outgoing-protocol", "SYNC to the master", "redis.c:7252", "syncWithMaster", False,
    "Requests the bulk dump from the master.")
add("redis-server", "external", "outgoing-stream", "write propagation to slaves and MONITOR clients", "redis.c:2057",
    "replicationFeedSlaves", False,
    "Executed writes are queued to slaves/monitors over their inbound connections; no connection is opened.")

# ---------- redis-server data ----------
add("redis-server", "data", "in-memory-store", "keyspace: redisDb.dict", "redis.c:282", None, True,
    "Per-database dict key -> robj; server.db[dbnum] in struct redisServer (redis.c:341), created at redis.c:1558.",
    struct="redisDb (redis.c:281)")
add("redis-server", "data", "snapshot-file", "dump.rdb", "redis.c:1493", "rdbSave / rdbLoad", True,
    "Default dbfilename: written via temp file + rename by rdbSave (redis.c:3256), loaded at startup by rdbLoad (redis.c:3618).")
add("redis-server", "data", "append-log", "appendonly.aof", "redis.c:1494", "feedAppendOnlyFile / loadAppendOnlyFile",
    True, "Append-only log of writes: opened redis.c:1580, written redis.c:7469, replayed redis.c:7528; name not configurable.")
add("redis-server", "data", "swap-file", "/tmp/redis-%p.vm", "redis.c:1503", "vmInit", True,
    "Default vm-swap-file for swapped-out values; opened and sized in vmInit (redis.c:7978) when vm-enabled.")
add("redis-server", "data", "in-memory", "expires dict (redisDb.expires)", "redis.c:283", None, False,
    "Per-database key -> expire time; saved in the snapshot.")
add("redis-server", "data", "in-memory", "blocking keys (redisDb.blockingkeys)", "redis.c:284", None, False,
    "Keys with clients blocked in BLPOP/BRPOP.")
add("redis-server", "data", "in-memory", "object sharing pool (server.sharingpool)", "redis.c:342", None, False,
    "Pool of shared string objects used when shareobjects is on.")
add("redis-server", "data", "in-memory", "client, slave and monitor lists", "redis.c:345", None, False,
    "server.clients, server.slaves and server.monitors connection registries.")
add("redis-server", "data", "pid-file", "/var/run/redis.pid", "redis.c:1492", "daemonize", False,
    "Default pidfile, written only when daemonized (redis.c:9117).")
add("redis-server", "data", "log-file", "logfile (default stdout)", "redis.c:931", "redisLog", False,
    "Log lines appended to the configured logfile, or stdout.")
add("redis-server", "data", "config-file", "redis.conf (argv[1], or '-' for stdin)", "redis.c:1621",
    "loadServerConfig", False, "Configuration file read once at startup.")
add("redis-server", "data", "temp-file", "temp-<pid>.rdb", "redis.c:3270", "rdbSave", False,
    "Temporary snapshot renamed onto dbfilename; removed if the child dies.")
add("redis-server", "data", "temp-file", "temp-rewriteaof-bg-<pid>.aof", "redis.c:7882",
    "rewriteAppendOnlyFileBackground", False,
    "Rewritten AOF from the child; the parent appends its diff (backgroundRewriteDoneHandler, redis.c:1205) and renames it onto appendonly.aof (redis.c:1221).")
add("redis-server", "data", "temp-file", "temp-rewriteaof-<pid>.aof", "redis.c:7690", "rewriteAppendOnlyFile",
    False, "Inner temp file of the rewrite, renamed to the bg temp name.")
add("redis-server", "data", "temp-file", "temp-<time>.<pid>.rdb (slave sync)", "redis.c:7275", "syncWithMaster",
    False, "Dump received from the master, renamed onto dbfilename and loaded.")
add("redis-server", "data", "system-file", "/proc/sys/vm/overcommit_memory", "redis.c:9080",
    "linuxOvercommitMemoryValue", False, "Read at startup on Linux to warn when overcommit is 0.")

# ---------- redis-cli ----------
C = "redis-cli"
for n, l, w in [("-h", 384, "server host, resolved with anetResolve; default 127.0.0.1"),
                ("-p", 394, "server port, default 6379"),
                ("-a", 403, "password; AUTH is sent only in interactive mode"),
                ("-r", 397, "repeat the command N times"),
                ("-n", 400, "database number, sent as SELECT before the command"),
                ("-i", 406, "force interactive mode")]:
    add(C, "commands", "flag", n, f"redis-cli.c:{l}", "parseOptions", True, w + ".")
add(C, "commands", "arg", "<command> [arg ...]", "redis-cli.c:311", "cliSendCommand", True,
    "Redis command word and arguments from argv or the '>> ' prompt, checked against the local cmdTable, then sent.")
add(C, "commands", "interactive-word", "quit / exit", "redis-cli.c:486", "repl", False,
    "Typed at the interactive prompt; exits locally without contacting the server.")
add(C, "commands", "stdin", "last argument from standard input", "redis-cli.c:524", "readArgFromStdin", False,
    "When exactly one argument is missing, stdin is read to EOF and used as the last argument.")
clionly = {"zmerge", "zmergeweighed", "rewriteaof"}
n = 0
for i, l in enumerate(open("redis-cli.c", encoding="latin-1"), 1):
    if 65 <= i <= 158:
        m = re.match(r'\s*\{"(\w+)",(-?\d+),([A-Z_|]+)\}', l)
        name, arity, flag = m.group(1), int(m.group(2)), m.group(3)
        n += 1
        if name in clionly:
            w = f"Client cmdTable word (arity {arity}) with no redis-server command: the server answers unknown command."
        elif name == "rpoplpush":
            w = "Client cmdTable word (arity 3) sent as bulk while the server table marks rpoplpush inline."
        elif name == "monitor":
            w = "Client cmdTable word (arity 1); the cli then prints replies forever."
        else:
            w = (f"Client cmdTable word (arity {arity}, {flag.replace('REDIS_CMD_', '').lower()}); "
                 "validated locally, forwarded to the server.")
        add(C, "commands", "client-command", name, f"redis-cli.c:{i}", "cliSendCommand", False, w)
assert n == 94
add(C, "external", "tcp-connection", "Redis server at -h/-p (default 127.0.0.1:6379)", "redis-cli.c:179",
    "cliConnect", True, "anetTcpConnect once per process; replies are parsed by cliReadReply.")
add(C, "external", "dns", "host name resolution for -h", "redis-cli.c:386", "anetResolve", False,
    "Resolves the -h argument before connecting.")
add(C, "external", "outgoing-protocol", "SELECT <db>", "redis-cli.c:299", "selectDb", False,
    "Sent before the command when -n is non-zero.")
add(C, "external", "outgoing-protocol", "AUTH <password>", "redis-cli.c:477", "repl", False,
    "Sent at the start of interactive mode when -a is given.")

# ---------- redis-benchmark ----------
B = "redis-benchmark"
for n_, l, w, m in [("-h", 428, "server host, default 127.0.0.1", True), ("-p", 436, "server port, default 6379", True),
                    ("-c", 419, "number of parallel connections, default 50", True),
                    ("-n", 422, "total number of requests, default 10000", True),
                    ("-d", 439, "SET/GET value size in bytes, clamped to 1..1MB; default 3 in code (redis-benchmark.c:496) while the usage text says 2 (redis-benchmark.c:465)", True),
                    ("-k", 425, "1 = keep alive, 0 = reconnect per request", True),
                    ("-r", 444, "key space length for random _rand keys", True),
                    ("-q", 450, "quiet: only requests per second", True),
                    ("-l", 452, "loop the whole test sequence forever", True),
                    ("-D", 454, "debug output (config.debug, used at redis-benchmark.c:189)", True),
                    ("-I", 456, "idle mode: open -c idle connections and wait forever instead of benchmarking (redis-benchmark.c:515)", True)]:
    add(B, "commands", "flag", n_, f"redis-benchmark.c:{l}", "parseOptions", m, w + ".")
add(B, "external", "tcp-connection", "Redis server at -h/-p (default 127.0.0.1:6379)", "redis-benchmark.c:343",
    "createClient", True, "anetTcpNonBlockConnect per simulated client; -c connections share one ae event loop.")
add(B, "external", "dns", "host name resolution for -h", "redis-benchmark.c:430", "anetResolve", False,
    "Resolves -h before connecting.")
add(B, "external", "outgoing-protocol",
    "fixed command mix: PING, PING (multi bulk), SET, GET, INCR, LPUSH, LPOP, SADD, SPOP, LRANGE 100/300/450/600",
    "redis-benchmark.c:531", "main", False, "Hard-coded request payloads sent in sequence; not selectable by the user.")
add(B, "workers", "event-callback", "writeHandler", "redis-benchmark.c:357", "writeHandler", False,
    "Writable file event per connection that sends the query buffer.")
add(B, "workers", "event-callback", "readHandler", "redis-benchmark.c:333", "readHandler", False,
    "Readable file event per connection that parses replies and records latency.")
add(B, "data", "in-memory", "latency histogram (config.latency)", "redis-benchmark.c:504", None, False,
    "Per-millisecond request counts used for the report.")

# ---------- redis-check-dump ----------
D = "redis-check-dump"
add(D, "commands", "arg", "<dump.rdb>", "redis-check-dump.c:627", "main", True,
    "Single positional argument: the RDB file to check; without it usage is printed.")
add(D, "data", "input-file", "RDB dump file (read-only mmap)", "redis-check-dump.c:637", "process", True,
    "Validates the REDIS0001 header and every opcode, reporting corrupt ranges; never writes.")

# ---------- scripts and build (not programs built by the Makefile) ----------
I = "utils/redis_init_script"
add(I, "commands", "subcommand", "start", "utils/redis_init_script:10", None, False,
    "Starts redis-server with the port-specific config unless the pid file exists.")
add(I, "commands", "subcommand", "stop", "utils/redis_init_script:19", None, False,
    "Sends SHUTDOWN via nc, waits, then removes the pid file.")
add(I, "external", "process-launch", "/usr/local/bin/redis-server /etc/redis/6379.conf", "utils/redis_init_script:16",
    None, False, "Launches the server binary.")
add(I, "external", "process-launch", "nc localhost 6379 (SHUTDOWN)", "utils/redis_init_script:25", None, False,
    "Pipes a SHUTDOWN request to the server with netcat.")
add(I, "data", "pid-file", "/var/run/redis_6379.pid", "utils/redis_init_script:6", None, False,
    "Pid file the script checks; differs from the server default /var/run/redis.pid.")
add(I, "data", "config-file", "/etc/redis/6379.conf", "utils/redis_init_script:7", None, False,
    "Config path passed to redis-server.")
R = "utils/redis-copy.rb"
add(R, "commands", "arg", "<srchost> <srcport> <dsthost> <dstport>", "utils/redis-copy.rb:68", "redisCopy", False,
    "Four positional arguments; waits for a key press before copying.")
add(R, "external", "client-lib-connection", "source Redis (redis gem)", "utils/redis-copy.rb:17", "redisCopy", False,
    "Reads KEYS * and values from the source.")
add(R, "external", "client-lib-connection", "destination Redis (redis gem)", "utils/redis-copy.rb:18", "redisCopy",
    False, "Writes strings, lists, sets and TTLs to the destination.")
S = "utils/redis-sha1.rb"
add(S, "commands", "arg", "[host] [port] [db]", "utils/redis-sha1.rb:48", "redisSha1", False,
    "Optional positional arguments, defaults 127.0.0.1 6379 0.")
add(S, "external", "client-lib-connection", "Redis server (redis gem)", "utils/redis-sha1.rb:17", "redisSha1", False,
    "Reads every key to compute a dataset SHA1.")
T = "utils/build-static-symbols.tcl"
add(T, "data", "input-file", "redis.c", "utils/build-static-symbols.tcl:7", None, False,
    "Scans static function names; 'make staticsymbols' redirects its output to staticsymbols.h.")
M = "Makefile"
for tgt, l, w in [("all", 27, "builds the four programs"), ("clean", 69, "removes binaries and objects"),
                  ("dep", 72, "prints header dependencies"), ("staticsymbols", 75, "regenerates staticsymbols.h"),
                  ("test", 78, "runs tclsh test-redis.tcl against a running server"),
                  ("bench", 81, "runs ./redis-benchmark"), ("log", 84, "regenerates Changelog from git log"),
                  ("32bit", 87, "builds with -m32"), ("gprof", 93, "builds with -pg"),
                  ("gcov", 96, "builds with coverage flags"), ("noopt", 99, "builds without optimization"),
                  ("32bitgprof", 102, "builds 32-bit with -pg")]:
    add(M, "commands", "make-target", tgt, f"Makefile:{l}", None, False, f"Build target: {w}.")

nots = [
    ("redis-server", "vm-max-threads (second branch)", "redis.c:1775",
     "Duplicate directive branch; unreachable because the branch at redis.c:1769 matches first."),
    ("redis-server", "appendfilename", "redis.c:1494",
     "Not a config directive in 1.3.6: appendonly.aof is a fixed default with no parser branch."),
    ("redis-server", "debug / verbose / notice / warning", "redis.c:1672", "Values of the loglevel setting, not settings."),
    ("redis-server", "no / always / everysec", "redis.c:1740", "Values of the appendfsync setting."),
    ("redis-server", "stdout", "redis.c:1684", "Value of the logfile setting meaning standard output."),
    ("redis-server", "yes / no", "redis.c:1605", "Boolean values accepted by several directives."),
    ("redis-server", "- (config path)", "redis.c:1618", "Value of the config-path argument meaning stdin, not a flag."),
    ("redis-server", "asc / desc / alpha / limit / store / by / get", "redis.c:6240",
     "SORT argument keywords; 'get' here is not the GET command."),
    ("redis-server", "weights / aggregate / sum / min / max", "redis.c:5464", "ZUNION/ZINTER argument keywords."),
    ("redis-server", "withscores", "redis.c:5595", "ZRANGE/ZREVRANGE argument keyword."),
    ("redis-server", "limit (zrangebyscore)", "redis.c:5697", "ZRANGEBYSCORE/ZCOUNT argument keyword."),
    ("redis-server", "no one", "redis.c:7326", "SLAVEOF arguments meaning become a master again."),
    ("redis-server", "segfault / reload / loadaof / object / swapout", "redis.c:8977",
     "DEBUG subcommand arguments, not separate requests."),
    ("redis-server", "DEBUG [SEGFAULT|OBJECT <key>|SWAPOUT <key>|RELOAD]", "redis.c:9063", "Words inside an error reply."),
    ("redis-server", "redis_version / role / used_memory ... (INFO fields)", "redis.c:6491", "Fields inside the INFO reply."),
    ("redis-server", "raw / int / zipmap / hashtable", "redis.c:131", "Encoding names printed in DEBUG OBJECT replies."),
    ("redis-server", "select 0 .. select 9", "redis.c:1452",
     "Shared objects written into the slave/monitor stream, not requests handled here."),
    ("redis-server", "SELECT / EXPIREAT written to the AOF", "redis.c:7427",
     "Records written to the append-only file (EXPIRE is logged as EXPIREAT)."),
    ("redis-server", "AUTH / SYNC strings in syncWithMaster", "redis.c:7252",
     "Outgoing requests to the master; the served handlers are authCommand and syncCommand in cmdTable."),
    ("redis-server", "REDIS_CMD_INLINE / REDIS_CMD_BULK / REDIS_CMD_DENYOOM", "redis.c:107",
     "Command table flags, not commands."),
    ("redis-server", "REDIS_SERVERPORT 6379", "redis.c:85", "Compiled default of the port setting, not a separate setting."),
    ("redis-server", "daemonize fork", "redis.c:9104", "The parent exits and the child is the server itself; not background work."),
    ("redis-server", "aeMain event loop", "redis.c:9153", "The main thread's loop, not a worker."),
    ("redis-server", "readQueryFromClient / sendReplyToClient file events", "redis.c:2456",
     "Per-connection request I/O plumbing, not background workers."),
    ("redis-server", "symsTable", "staticsymbols.h:1",
     "Generated function-name table for crash backtraces, not a command table."),
    ("redis-server", "design-documents/REDIS-CLUSTER", "design-documents/REDIS-CLUSTER:1",
     "Unimplemented cluster proposal; no cluster code, requests or connections exist."),
    ("redis-server", "doc/*Command.html", "doc/QuitCommand.html:22", "Documentation pages, not handlers."),
    ("redis-cli", "redis-cli cmdTable as requests", "redis-cli.c:64",
     "Client-side command table: redis-cli serves nothing, and the table does not define what the server accepts."),
    ("redis-cli", "zmerge / zmergeweighed / rewriteaof as server commands", "redis-cli.c:106",
     "Only in the client table (also redis-cli.c:107, 130); redis-server has no such commands."),
    ("redis-cli", "zunion / zinter / zremrangebyrank / sync as redis-cli commands", "redis.c:746",
     "Server commands missing from the client table; redis-cli rejects them locally as unknown."),
    ("redis-cli", "MONITOR read_forever loop", "redis-cli.c:366", "Foreground blocking read, not a worker."),
    ("redis-benchmark", "PING / SET / GET / INCR / LPUSH / LPOP / SADD / SPOP / LRANGE payloads and report titles",
     "redis-benchmark.c:531",
     "Hard-coded outgoing requests and labels; not commands the user types and not requests served."),
    ("redis-check-dump", "STRING / LIST / SET / ZSET / HASH / EXPIRETIME / SELECTDB / EOF", "redis-check-dump.c:649",
     "Type labels for the report output."),
    ("test", "test-redis.tcl", "test-redis.tcl:229",
     "Test-only suite (make test); its -h/-p/-stress/--flush/--first/--last options (test-redis.tcl:2037-2050) and 'exec leaks redis-server' are not product items."),
    ("test", "zipmap.c main (ZIPMAP_TEST_MAIN)", "zipmap.c:412",
     "Self-test main compiled only with -DZIPMAP_TEST_MAIN; no Makefile target builds it, so it is not a program or entry point."),
    ("redis-server", "/dev/null (server.devnull)", "redis.c:1539",
     "Sink stream used to measure serialized object length (redis.c:3242); not a file the server owns."),
    ("Makefile", "OPTIMIZATION / ARCH / PROF / CFLAGS / CCLINK / DEBUG", "Makefile:6",
     "Make variables (?= can be overridden from the environment at build time); no built program reads an environment variable."),
    ("test", "redis.tcl bulkarg / multibulkarg lists", "redis.tcl:22",
     "Tcl client library used by the tests; its command lists are client-side, not server requests."),
    ("utils/redis_init_script", "REDISPORT", "utils/redis_init_script:3",
     "Shell variable assigned in the script, not an environment variable read."),
    ("utils/redis-copy.rb", "'redis-sha1.rb' header comment", "utils/redis-copy.rb:1",
     "Copy-pasted header; the file is the copy tool."),
]
not_list = [{"component": c, "name": n, "anchor": a, "reason": r} for c, n, a, r in nots]

components = [
    {"name": "redis-server", "kind": "program", "built_by": "Makefile:49", "entry": "redis.c:9124 main",
     "sources": "redis.c + ae.c anet.c dict.c adlist.c sds.c zmalloc.c lzf_c.c lzf_d.c pqsort.c zipmap.c"},
    {"name": "redis-cli", "kind": "program", "built_by": "Makefile:60", "entry": "redis-cli.c:501 main",
     "sources": "redis-cli.c + anet.c sds.c adlist.c zmalloc.c"},
    {"name": "redis-benchmark", "kind": "program", "built_by": "Makefile:57", "entry": "redis-benchmark.c:483 main",
     "sources": "redis-benchmark.c + ae.c anet.c sds.c adlist.c zmalloc.c"},
    {"name": "redis-check-dump", "kind": "program", "built_by": "Makefile:63", "entry": "redis-check-dump.c:615 main",
     "sources": "redis-check-dump.c + lzf_c.c lzf_d.c"},
    {"name": "utils/redis_init_script", "kind": "script (not built)", "entry": "utils/redis_init_script:9"},
    {"name": "utils/redis-copy.rb", "kind": "script (not built)", "entry": "utils/redis-copy.rb:68"},
    {"name": "utils/redis-sha1.rb", "kind": "script (not built)", "entry": "utils/redis-sha1.rb:48"},
    {"name": "utils/build-static-symbols.tcl", "kind": "build script (not built)", "entry": "utils/build-static-symbols.tcl:7"},
    {"name": "Makefile", "kind": "build", "entry": "Makefile:27"},
]
notes = [
    "Components: the Makefile builds four programs (redis-server, redis-cli, redis-benchmark, redis-check-dump) and no library; shared objects (ae, anet, sds, adlist, zmalloc, lzf) are linked into several programs. utils/* scripts and Makefile targets are included with must=false only.",
    "All 95 redis-server cmdTable entries plus QUIT (special-cased in processCommand) are must requests; the handler is the cmdTable function (smembers is served by sinterCommand).",
    "redis-cli command words are listed as commands (kind client-command, must=false each); the single must item is '<command> [arg ...]'. The client table differs from the server: client-only zmerge, zmergeweighed, rewriteaof; server-only sync, zunion, zinter, zremrangebyrank; rpoplpush is bulk on the client but inline on the server.",
    "redis-cli -a is only honoured in interactive mode: repl() sends AUTH (redis-cli.c:472-477); one-shot mode never sends it.",
    "No environment variables are read by any C program (no getenv). Make variables (OPTIMIZATION, ARCH, PROF, CFLAGS, DEBUG) exist in the Makefile but are not listed.",
    "When a config file is given, the three default save points (redis.c:1514-1516) are cleared first (redis.c:9129); only the file's save lines apply.",
    "hash-max-zipmap-entries/value are parsed, but genRedisInfoString resets both to their compiled defaults each time INFO runs (redis.c:6486-6487).",
    "Replication to slaves is master-side output over inbound connections (replicationFeedSlaves, sendBulkToSlave); only the slave side opens an outgoing connection (syncWithMaster). Listed as external must=false / worker must=false accordingly.",
    "serverCron sub-steps are listed separately as cron-step workers with must=false; serverCron itself is the must timer.",
    "VM (swap file, I/O threads) is off by default (vm-enabled no in redis.conf) but is a real code path; its swap file and threads are marked must.",
    "The working tree also contains build outputs (*.o and the four binaries); they are untracked and git-ignored (.gitignore:2-8), not committed, and are not inventory items.",
]
review = [
    {"change": "redis-benchmark flag -D: must false -> true", "anchor": "redis-benchmark.c:454",
     "why": "Owner definition: commands are CLI flags. -D is live (config.debug read at redis-benchmark.c:189) and printed in usage; no rule exempts debug flags, and every redis-cli flag is must."},
    {"change": "redis-benchmark flag -I: must false -> true", "anchor": "redis-benchmark.c:456",
     "why": "Owner definition: commands are CLI flags. -I switches the program to idle mode (redis-benchmark.c:515) and is printed in usage."},
    {"change": "redis-benchmark flag -d: why text adds the real default", "anchor": "redis-benchmark.c:496",
     "why": "Code default is 3 bytes; the usage text claims 2 (redis-benchmark.c:465). A claim of 'default 2' quotes the usage, not behaviour."},
    {"change": "temp-rewriteaof-bg-<pid>.aof: why text cites the rename at redis.c:1221", "anchor": "redis.c:7882",
     "why": "redis.c:1205 is the temp-name snprintf in backgroundRewriteDoneHandler; the rename onto appendonly.aof is redis.c:1221."},
    {"change": "added redis-server external dns 'host name resolution for the master host' (must=false)", "anchor": "anet.c:145",
     "why": "Same shape as the redis-cli and redis-benchmark dns items: anetTcpConnect resolves masterhost with gethostbyname."},
    {"change": "added trap 'zipmap.c main (ZIPMAP_TEST_MAIN)'", "anchor": "zipmap.c:412",
     "why": "A second int main in the C sources; only a -DZIPMAP_TEST_MAIN build compiles it and the Makefile never does."},
    {"change": "added trap '/dev/null (server.devnull)'", "anchor": "redis.c:1539",
     "why": "fopen of a path at startup that is not owned data (used as a length-measuring sink, redis.c:3242)."},
    {"change": "added trap 'OPTIMIZATION / ARCH / PROF / CFLAGS / CCLINK / DEBUG'", "anchor": "Makefile:6",
     "why": "Owner definition lists env vars as commands; these are build-time make variables, not environment read by a program (no getenv anywhere). Promoted from a note to a trap."},
    {"change": "trap test-redis.tcl: reason lists --first/--last too", "anchor": "test-redis.tcl:2037",
     "why": "The option loop also accepts --first and --last."},
    {"change": "note on build outputs: 'committed' -> 'untracked and git-ignored'", "anchor": ".gitignore:2",
     "why": "git ls-files lists no *.o or binaries; .gitignore:2-8 ignores them."},
]
review_summary = ("Reviewer (read-only, code only): opened every must=true anchor (96 requests, 31 redis-server commands, 4 workers, "
                  "1 external, 4 data, 7 redis-cli commands + 1 external, 9->11 redis-benchmark flags + 1 external, "
                  "2 redis-check-dump items); all land on the named code with the right handler. Independent greps "
                  "(cmdTable, loadServerConfig branches, fork/pthread_create/aeCreateTimeEvent/aeCreateFileEvent, "
                  "fopen/open/rename, anetTcpConnect/anetTcpServer, getenv, argv parsing in the four mains) found no missing must item. "
                  "All 37 original traps verified at their anchors. Edits were made in _gen/gen_redis.py and the JSON regenerated.")
# ---------- errata (dated corrections after a skeptic pass; the audit matcher is unchanged) ----------
ERRATA_DATE = "2026-09-29"
errata = []


def erratum(item, change, reason, citation):
    errata.append({"date": ERRATA_DATE, "item": item, "change": change, "reason": reason, "citation": citation})


def find_item(component, inventory, name):
    hit = [it for it in items if it["component"] == component and it["inventory"] == inventory and it["name"] == name]
    assert len(hit) == 1, (component, inventory, name, len(hit))
    return hit[0]


FORK_WHY = ("A forked child is how BGSAVE / BGREWRITEAOF / SYNC / a save point do their work, not a way into the "
            "program; the ways in (the bgsave, bgrewriteaof and sync commands, the serverCron save-point check) and "
            "the files (dump.rdb, appendonly.aof, the temp files) stay must/may items, so the child remains findable "
            "on the bgsave/dump.rdb path.")
it = find_item("redis-server", "workers", "background save child (BGSAVE)")
assert it["must"] is True
it["must"] = False
erratum("redis-server / workers / background save child (BGSAVE) (redis.c:3360)", "must true -> false", FORK_WHY,
        "redis.c:3360 fork() in rdbSaveBackground; started from bgsaveCommand redis.c:4117 (cmdTable redis.c:782), "
        "syncCommand redis.c:7090, the serverCron save point redis.c:1322 and redis.c:7201")
it = find_item("redis-server", "workers", "background AOF rewrite child (BGREWRITEAOF)")
assert it["must"] is True
it["must"] = False
erratum("redis-server / workers / background AOF rewrite child (BGREWRITEAOF) (redis.c:7876)", "must true -> false",
        FORK_WHY, "redis.c:7876 fork() in rewriteAppendOnlyFileBackground; started only from bgrewriteaofCommand "
        "redis.c:7914 (cmdTable redis.c:783)")
tr = [t for t in not_list if t["name"] == "aeMain event loop"]
assert len(tr) == 1
tr[0]["flow"] = True
erratum("not / redis-server / aeMain event loop (redis.c:9153)", "added \"flow\": true (still a trap as a worker)",
        "Consistency with the freqtrade and litestream flow marks: the program's own foreground loop belongs to the "
        "Main flow, not to workers. Redis keeps it as a trap (listing it as a worker is wrong); the other two "
        "inventories carry their loops as must=false workers items with flow=true.",
        "redis.c:9153 aeMain(server.el) in main")
# The loop is an item of the Main flow, as in the other two inventories.
not_list.remove(tr[0])
add("redis-server", "workers", "main-loop", "aeMain event loop", "redis.c:9153", "aeMain", False,
    "main's last call: blocks in the event loop, running beforeSleep and aeProcessEvents until stop is set.",
    flow=True)
erratum("not / redis-server / aeMain event loop (redis.c:9153)",
        "trap -> item redis-server / workers / aeMain event loop: must false, \"flow\": true",
        "The program's own foreground loop is scored against the Main flow, like freqtrade's Worker.run and "
        "litestream's restore follow loop (must=false workers items with flow=true), so the three repositories "
        "score their main loops alike. A workers row on it is an extra, not a trap hit.",
        "redis.c:9153 aeMain(server.el) in main, its last call before aeDeleteEventLoop (redis.c:9154); "
        "ae.c:375-381 aeMain loops over eventLoop->beforesleep and aeProcessEvents until eventLoop->stop")

doc = {"repository": "redis-1.3.6", "path": "~/git/redis-1.3.6", "revision": rev, "components": components,
       "items": items, "not": not_list, "notes": notes, "review_summary": review_summary, "review": review,
       "errata": errata}
json.dump(doc, open(OUT, "w"), indent=1, ensure_ascii=False)
print("wrote", OUT, len(items), "items", len(not_list), "not")
