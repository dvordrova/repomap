package facts

import (
	"testing"

	"github.com/dvordrova/repomap/internal/programindex"
)

// TestCFacts reads config keys, shell commands and SQL from a C program the
// way the C adapter seals them: libc and library functions are external
// symbols named by the header the repository includes. A repository function
// that happens to be called eval or exec runs nothing it was handed.
func TestCFacts(t *testing.T) {
	source := "#include <stdlib.h>\n" +
		"#include <stdio.h>\n" +
		"int main(void) {\n" +
		"    const char *port = getenv(\"KVD_PORT\");\n" +
		"    system(\"rm -rf /tmp/kvd\");\n" +
		"    FILE *pipe = popen(command, \"r\");\n" +
		"    sqlite3_exec(db, \"SELECT id, name FROM users WHERE id = 1\", 0, 0, 0);\n" +
		"    eval(script);\n" +
		"    exec(line);\n" +
		"    run(\"ls\");\n" +
		"}\n"
	repository := newCorpus(t, map[string]string{"kvd.c": source})
	s := newSynthetic(t, "c", "kvd", "kvd.c")
	s.object("mod", programindex.ObjectModule, "kvd.c", "kvd.c", 1, "")
	s.object("main", programindex.ObjectFunction, "main", "kvd.c", 3, "mod")
	s.object("eval", programindex.ObjectFunction, "eval", "kvd.c", 3, "mod")
	s.object("exec", programindex.ObjectFunction, "exec", "kvd.c", 3, "mod")
	s.seed("main", programindex.SeedCallable, "kvd.c", 3)
	s.external("getenv", "stdlib.h", "getenv", programindex.ExternalAuthorityPlatform)
	s.external("system", "stdlib.h", "system", programindex.ExternalAuthorityPlatform)
	s.external("popen", "stdio.h", "popen", programindex.ExternalAuthorityPlatform)
	s.external("sqlite", "sqlite3.h", "sqlite3_exec", programindex.ExternalAuthorityPackage)
	s.external("run", "tasks.h", "run", programindex.ExternalAuthorityPackage)
	s.relate("getenv", programindex.RelationInvokesExternal, "main", []string{"getenv"}, loc("kvd.c", 4),
		pattern("p", programindex.PatternCall, "getenv", loc("kvd.c", 4), nil, literal(1, "KVD_PORT")))
	s.relate("system", programindex.RelationInvokesExternal, "main", []string{"system"}, loc("kvd.c", 5),
		pattern("p", programindex.PatternCall, "system", loc("kvd.c", 5), nil, literal(1, "rm -rf /tmp/kvd")))
	s.relate("popen", programindex.RelationInvokesExternal, "main", []string{"popen"}, loc("kvd.c", 6),
		pattern("p", programindex.PatternCall, "popen", loc("kvd.c", 6), nil, dynamic(1), literal(2, "r")))
	s.relate("sqlite", programindex.RelationInvokesExternal, "main", []string{"sqlite"}, loc("kvd.c", 7),
		pattern("p", programindex.PatternCall, "sqlite3_exec", loc("kvd.c", 7), nil,
			dynamic(1), literal(2, "SELECT id, name FROM users WHERE id = 1"), dynamic(3), dynamic(4), dynamic(5)))
	s.relate("eval", programindex.RelationCalls, "main", []string{"eval"}, loc("kvd.c", 8),
		pattern("p", programindex.PatternCall, "eval", loc("kvd.c", 8), nil, dynamic(1)))
	s.relate("exec", programindex.RelationCalls, "main", []string{"exec"}, loc("kvd.c", 9),
		pattern("p", programindex.PatternCall, "exec", loc("kvd.c", 9), nil, dynamic(1)))
	s.relate("run", programindex.RelationInvokesExternal, "main", []string{"run"}, loc("kvd.c", 10),
		pattern("p", programindex.PatternCall, "run", loc("kvd.c", 10), nil, literal(1, "ls")))
	result := mustBuild(t, Input{Repository: repository, Targets: []TargetInput{{Index: s.index(), Root: "."}}})

	port := requireFact(t, result, KindConfigRead, "KVD_PORT", func(fact Fact) bool { return fact.Key == "KVD_PORT" })
	if port.Anchor.String() != "kvd.c:4" || port.Symbol != "main" {
		t.Fatalf("config read %+v", port)
	}
	query := requireFact(t, result, KindSQLQuery, "users query", func(fact Fact) bool { return fact.Anchor.String() == "kvd.c:7" })
	if query.Key != "users" || query.Symbol != "main" {
		t.Fatalf("sql query %+v", query)
	}
	byAnchor := map[string]Fact{}
	for _, fact := range result.OfKind(KindDynamicExecution) {
		byAnchor[fact.Anchor.String()] = fact
	}
	if system := byAnchor["kvd.c:5"]; system.Key != "system" || system.Symbol != "main" || system.Text != `system("rm -rf /tmp/kvd");` {
		t.Fatalf("system %+v", system)
	}
	if popen := byAnchor["kvd.c:6"]; popen.Key != "popen" {
		t.Fatalf("popen %+v", popen)
	}
	if len(byAnchor) != 2 {
		t.Fatalf("dynamic execution %+v", byAnchor)
	}
}
