package storefixture

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
