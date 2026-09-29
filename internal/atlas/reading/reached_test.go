package reading

import (
	"slices"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
)

// The question naming what an outgoing call reaches is told where each of
// its programs reaches the call from: the first callers outside the file it
// is written in, by name alone, past the wrappers inside that file;
// a path whose callers run out inside the file only at a seed or an input's
// handler; not a helper nothing calls, a test's caller, or a caller its
// program never runs. The server's connect is reached from its replication,
// the client's from its own connecting function.
func TestAnOutgoingCallIsNamedWithWhereItsProgramsReachItFrom(t *testing.T) {
	symbol := func(id, path, name, signature string, targets []string, callers ...atlas.SymbolCaller) atlas.Place {
		return atlas.Place{ID: id, Kind: atlas.PlaceSymbol, Path: path, LineNo: 1, Parent: "file:" + path, TargetIDs: targets,
			Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{Name: name, Signature: signature}, CalledBy: callers}}
	}
	caller := func(id, path string) atlas.SymbolCaller {
		return atlas.SymbolCaller{PlaceID: id, Path: path, Kind: "calls", Resolution: "exact"}
	}
	both := []string{"server", "client"}
	places := []atlas.Place{
		{ID: "file:net.c", Kind: atlas.PlaceFile, Path: "net.c", File: &atlas.FileFacts{}},
		{ID: "file:server.c", Kind: atlas.PlaceFile, Path: "server.c", File: &atlas.FileFacts{}},
		{ID: "file:client.c", Kind: atlas.PlaceFile, Path: "client.c", File: &atlas.FileFacts{}},
		{ID: "file:net_test.c", Kind: atlas.PlaceFile, Path: "net_test.c", File: &atlas.FileFacts{Test: true}},
		symbol("generic", "net.c", "netGenericConnect", "int netGenericConnect(char *host, int port)", both, caller("connect", "net.c"), caller("nonblock", "net.c"), caller("generic", "net.c")),
		symbol("connect", "net.c", "netConnect", "int netConnect(char *host, int port)", both, caller("sync", "server.c"), caller("cli", "client.c"), caller("check", "net_test.c")),
		symbol("nonblock", "net.c", "netNonBlockConnect", "int netNonBlockConnect(char *host, int port)", both),
		symbol("sync", "server.c", "syncWithMaster", "int syncWithMaster(void)", []string{"server"}),
		symbol("cli", "client.c", "cliConnect", "static int cliConnect(void)", []string{"client"}),
		symbol("check", "net_test.c", "checkConnect", "void checkConnect(void)", both),
	}
	r := answerTestReader(t, nil, nil)
	r.places = map[string]atlas.Place{}
	r.testPaths = map[string]bool{"net_test.c": true}
	for _, place := range places {
		r.places[place.ID] = place
	}
	row := func(targets ...string) []string { return targets }
	if got := r.reachedFrom(row("server"), r.places["generic"], nil); !slices.Equal(got, []string{"syncWithMaster"}) {
		t.Fatalf("the server reaches connect from %v", got)
	}
	if got := r.reachedFrom(row("client"), r.places["generic"], nil); !slices.Equal(got, []string{"cliConnect"}) {
		t.Fatalf("the client reaches connect from %v", got)
	}
	// A wrapper inside the file whose callers run out is kept only when it
	// handles an input.
	if got := r.reachedFrom(row("server"), r.places["generic"], map[string]bool{"nonblock": true}); !slices.Equal(got, []string{"netNonBlockConnect", "syncWithMaster"}) {
		t.Fatalf("with a handler inside the file, the server reaches connect from %v", got)
	}
}
