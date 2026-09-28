package facts

import (
	"reflect"
	"testing"

	"github.com/dvordrova/repomap/internal/programindex"
)

// net.c is linked into a server and a client; only the server runs serve.
// What serve reads, runs, queries and registers is the server's, never the
// client's, whichever target the facts meet first.
func TestFactsOfCodeATargetNeverRunsStayWithTheTargetsThatRunIt(t *testing.T) {
	source := "#include <stdlib.h>\n" +
		"int main(void) { return 0; }\n" +
		"void serve(void) {\n" +
		"    const char *backlog = getenv(\"BACKLOG\");\n" +
		"    system(\"announce\");\n" +
		"    sqlite3_exec(db, \"SELECT id FROM peers\", 0, 0, 0);\n" +
		"    signal(15, onSignal);\n" +
		"}\n" +
		"void onSignal(int sig) {}\n"
	repository := newCorpus(t, map[string]string{"net.c": source})
	build := func(name string, unreachable bool) programindex.Index {
		s := newSynthetic(t, "c", name, "net.c")
		s.object("mod", programindex.ObjectModule, "net.c", "net.c", 1, "")
		s.object("main", programindex.ObjectFunction, "main", "net.c", 2, "mod")
		s.object("serve", programindex.ObjectFunction, "serve", "net.c", 3, "mod")
		s.object("onSignal", programindex.ObjectFunction, "onSignal", "net.c", 9, "mod")
		for position := range s.objects {
			s.objects[position].Unreachable = unreachable && s.objects[position].SourceRef != "main" && s.objects[position].SourceRef != "mod"
		}
		s.seed("main", programindex.SeedCallable, "net.c", 2)
		s.external("getenv", "stdlib.h", "getenv", programindex.ExternalAuthorityPlatform)
		s.external("system", "stdlib.h", "system", programindex.ExternalAuthorityPlatform)
		s.external("sqlite", "sqlite3.h", "sqlite3_exec", programindex.ExternalAuthorityPackage)
		s.external("signal", "signal.h", "signal", programindex.ExternalAuthorityPlatform)
		s.relate("getenv", programindex.RelationInvokesExternal, "serve", []string{"getenv"}, loc("net.c", 4),
			pattern("p", programindex.PatternCall, "getenv", loc("net.c", 4), nil, literal(1, "BACKLOG")))
		s.relate("system", programindex.RelationInvokesExternal, "serve", []string{"system"}, loc("net.c", 5),
			pattern("p", programindex.PatternCall, "system", loc("net.c", 5), nil, literal(1, "announce")))
		s.relate("sqlite", programindex.RelationInvokesExternal, "serve", []string{"sqlite"}, loc("net.c", 6),
			pattern("p", programindex.PatternCall, "sqlite3_exec", loc("net.c", 6), nil, dynamic(1), literal(2, "SELECT id FROM peers"), dynamic(3), dynamic(4), dynamic(5)))
		s.relate("signal", programindex.RelationInvokesExternal, "serve", []string{"signal"}, loc("net.c", 7),
			pattern("p", programindex.PatternCall, "signal", loc("net.c", 7), nil, dynamic(1), dynamicRef(2, "onSignal")))
		s.callback("cb", "serve", "onSignal", "signal", "p", 2)
		return s.index()
	}
	indexes, err := programindex.RebindTargetSet([]programindex.Index{build("client", true), build("server", false)})
	if err != nil {
		t.Fatal(err)
	}
	client, server := indexes[0], indexes[1]
	for _, order := range [][]programindex.Index{{client, server}, {server, client}} {
		var targets []TargetInput
		for _, index := range order {
			targets = append(targets, TargetInput{Index: index, Root: "."})
		}
		result := mustBuild(t, Input{Repository: repository, Targets: targets})
		for _, kind := range []Kind{KindConfigRead, KindSQLQuery, KindRegistration} {
			var owners []string
			for _, fact := range result.OfKind(kind) {
				owners = append(owners, fact.TargetID)
			}
			if !reflect.DeepEqual(owners, []string{server.Target.ID}) {
				t.Errorf("%s facts belong to %v, want only the server %s", kind, owners, server.Target.ID)
			}
		}
	}
}
