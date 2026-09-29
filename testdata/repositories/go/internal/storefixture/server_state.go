package storefixture

import "net/http"

// A small key-value server's state, as the C fixture's kvd and Redis keep
// theirs: one package variable whose fields functions write and read
// directly (serverState.shutdown), and entries a function reaches through a
// pointer it finds or a client it is given (stateEntry.value,
// stateClient.argv). One field's writers and readers gather by the field,
// whatever value each reaches it from.

type stateServer struct {
	dbfile   string
	db       [8]stateEntry
	dbSize   int
	dirty    int
	shutdown bool
	stats    stateStats
}

type stateEntry struct {
	key, value string
}

type stateStats struct {
	hits int
}

type stateClient struct {
	argv []string
}

var serverState stateServer

// onStateSignal asks the server to stop: the only writer of shutdown.
func onStateSignal() { serverState.shutdown = true }

// stateBeforeSleep is the only reader of shutdown.
func stateBeforeSleep() bool { return serverState.shutdown }

// loadStateConfig and StartStateServer write the snapshot file's name;
// saveStateSnapshot reads it.
func loadStateConfig(words []string) {
	if len(words) == 2 && words[0] == "dbfilename" {
		serverState.dbfile = words[1]
	}
}

func StartStateServer(config []string) string {
	serverState.dbfile = "dump.kv"
	loadStateConfig(config)
	onStateSignal()
	return saveStateSnapshot()
}

// stateFind passes the array through to each entry's key and reads the
// array where it takes an entry's address.
func stateFind(key string) *stateEntry {
	for j := 0; j < serverState.dbSize; j++ {
		if serverState.db[j].key == key {
			return &serverState.db[j]
		}
	}
	return nil
}

// setState writes an entry's value through the pointer it finds, reads the
// client's words through its parameter, and counts the change.
func setState(c *stateClient) {
	e := stateFind(c.argv[1])
	if e == nil {
		e = &serverState.db[serverState.dbSize]
		serverState.dbSize++
		e.key = c.argv[1]
	}
	e.value = c.argv[2]
	serverState.dirty++
	serverState.stats.hits += 1
}

// getState reads the value through the pointer it finds.
func getState(c *stateClient) string {
	if e := stateFind(c.argv[1]); e != nil {
		return e.value
	}
	return ""
}

// saveStateSnapshot reads every value through the package variable's array:
// serverState.db.value, the field getState reads as stateEntry.value.
func saveStateSnapshot() string {
	out := serverState.dbfile
	for j := 0; j < serverState.dbSize; j++ {
		out += " " + serverState.db[j].key + "=" + serverState.db[j].value
	}
	if stateBeforeSleep() {
		out += " (stopping)"
	}
	return out
}

// ServeStateStatus stores its status handler in the server it starts: the
// store names the field it fills, net/http.Server.Handler, declared
// net/http.Handler, as tool_cli.go's fs.Usage = c.Usage names
// flag.FlagSet.Usage.
func ServeStateStatus(addr string) error {
	mux := http.HandlerFunc(stateStatus)
	srv := &http.Server{Addr: addr}
	srv.Handler = mux
	return srv.ListenAndServe()
}

func stateStatus(w http.ResponseWriter, _ *http.Request) {
	_, _ = w.Write([]byte(saveStateSnapshot()))
}
