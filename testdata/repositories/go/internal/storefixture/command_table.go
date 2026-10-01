package storefixture

import "fmt"

// A command table names each handler by a string literal. Every row is its
// own callable binding that keeps its own name and arity.
type commandRow struct {
	Name  string
	Arity int
	Run   func(args []string) string
}

func commandTable() []commandRow {
	return []commandRow{
		{Name: "get", Arity: 2, Run: getCommand},
		{Name: "set", Arity: 3, Run: setCommand},
	}
}

func getCommand(args []string) string { return args[1] }
func setCommand(args []string) string { return args[2] }

func lookupCommand(table []commandRow, name string) *commandRow {
	for i := range table {
		if table[i].Name == name {
			return &table[i]
		}
	}
	return nil
}

// DispatchCommand calls through the field of a row it looked up. Go follows
// one allocated value, and a looked-up row is not one, so the call stays
// unresolved.
func DispatchCommand(args []string) string {
	row := lookupCommand(commandTable(), args[0])
	if row == nil {
		return ""
	}
	return row.Run(args)
}

// An event loop stores a handler in one of two fields, chosen by a flag.
// The calls through those fields stay unresolved: neither field is given the
// handler registered for the other.
type eventLoop struct {
	onRead  func()
	onWrite func()
}

func (loop *eventLoop) register(readable bool, handler func()) {
	if readable {
		loop.onRead = handler
	} else {
		loop.onWrite = handler
	}
}

func (loop *eventLoop) fire() {
	loop.onRead()
	loop.onWrite()
}

func acceptClient() {}
func flushReplies() {}

func RunEventLoop() {
	loop := &eventLoop{}
	loop.register(true, acceptClient)
	loop.register(false, flushReplies)
	loop.fire()
}

// One store before the call makes the call through the field exact.
func RunSingleHandler() {
	loop := &eventLoop{onRead: acceptClient}
	loop.onRead()
}

// A store under a branch leaves the call unresolved.
func RunChosenHandler(readable bool) {
	loop := &eventLoop{}
	if readable {
		loop.onRead = acceptClient
	}
	loop.onRead()
}

// The same event loop with interface-typed fields. A handler parameter stored
// in one of two fields under a branch leaves the calls through both open;
// each handler it brings is a witness of those calls, never an alternative.
type readyHandler interface{ Handle() }

type acceptReady struct{}
type flushReady struct{}

func (acceptReady) Handle() {}
func (flushReady) Handle()  {}

type readyLoop struct {
	read  readyHandler
	write readyHandler
}

func (loop *readyLoop) register(readable bool, handler readyHandler) {
	if readable {
		loop.read = handler
	} else {
		loop.write = handler
	}
}

func (loop *readyLoop) fire() {
	loop.read.Handle()
	loop.write.Handle()
}

func RunReadyLoop() {
	loop := &readyLoop{}
	loop.register(true, acceptReady{})
	loop.register(false, flushReady{})
	loop.fire()
}

// A value stored under a branch is a value of the field: the call calls it.
func RunChosenReady(readable bool) {
	loop := &readyLoop{}
	if readable {
		loop.read = acceptReady{}
	}
	loop.read.Handle()
}

// The same choice for a field whose interface another package declares. A
// call through it is a call of fmt.Stringer.String, and the name a branch
// stored there is what it calls. Clearing a field stores nothing
// callable, so the branch that clears one leaves the name stored before it a
// possible value.
type namedLoop struct {
	chosen  fmt.Stringer
	cleared fmt.Stringer
}

type acceptName struct{}

func (acceptName) String() string { return "accept" }

func RunNamedLoop(readable bool) []string {
	loop := &namedLoop{cleared: acceptName{}}
	if readable {
		loop.chosen = acceptName{}
	} else {
		loop.cleared = nil
	}
	return []string{
		loop.chosen.String(),
		loop.cleared.String(),
	}
}

// A call written in a package-level variable's initializer runs when the
// package loads, in the synthetic initializer that declares nothing: the
// variable is its caller (GO), so namedCommands has a user.
var defaultCommands = namedCommands("get", "set")

func namedCommands(names ...string) []string { return names }

// A function calling its own parameter calls what every call into it hands
// there, as the Python and C adapters join a parameter's callers: run's two
// calls hand throttle two method values, the call's alternatives, and
// StartOnce hands runOnce one function, its exact target.
type throttle struct{}

func RunThrottle(running bool) { throttle{}.run(running) }

func (t throttle) run(running bool) {
	if running {
		t.throttle(t.processRunning, 1)
	} else {
		t.throttle(t.processStopped, 1)
	}
}

func (throttle) throttle(step func(), secs int) {
	step()
}

func (throttle) processRunning() {}

func (throttle) processStopped() {}

func runOnce(job func()) { job() }

func StartOnce() { runOnce(acceptJob) }

func acceptJob() {}

// A call handing any other value leaves the call open, the function another
// call hands its witness.
func runAny(job func()) { job() }

func StartAny(job func()) {
	runAny(flushJob)
	runAny(job)
}

func flushJob() {}

// A replica's client is chosen by a switch on its configured kind, each case
// storing what its own factory returns into one interface field, as
// litestream's NewReplicaFromConfig does: a call through the field calls each
// stored client, alternatives, never an unknown.
type objectStore interface{ Put(key string) }

type bucketStore struct{}

type diskStore struct{}

func (*bucketStore) Put(string) {}

func (*diskStore) Put(string) {}

func newBucketStore() (*bucketStore, error) { return &bucketStore{}, nil }

func newDiskStore() (*diskStore, error) { return &diskStore{}, nil }

type replica struct{ client objectStore }

func newReplica(kind string) (*replica, error) {
	r := &replica{}
	var err error
	switch kind {
	case "bucket":
		if r.client, err = newBucketStore(); err != nil {
			return nil, err
		}
	case "disk":
		if r.client, err = newDiskStore(); err != nil {
			return nil, err
		}
	}
	return r, nil
}

func (r *replica) sync(key string) { r.client.Put(key) }

func SyncReplica(kind, key string) {
	if r, err := newReplica(kind); err == nil {
		r.sync(key)
	}
}

// A hook the shared code runs is the linking program's own: cmd/worker hands
// its hook here, and cmd/app, which links this code too, runs none. The call
// through the field is the worker's wiring, no use of the worker by the app
// (etcd's server had "used" raftexample's Raft node this way).
type LinkedHook interface{ Run() }

type hookedRun struct{ hook LinkedHook }

func (h *hookedRun) fire() { h.hook.Run() }

func RunLinkedHook(hook LinkedHook) { (&hookedRun{hook: hook}).fire() }
