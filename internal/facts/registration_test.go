package facts

import (
	"reflect"
	"testing"

	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/sourcevalue"
)

// A registration is recognized by its shape alone. Each case is one call as a
// language adapter records it, and the fact a reader should get from it.
func TestRegistrationsComeFromCallShapesNotFrameworkNames(t *testing.T) {
	type want struct {
		key, method, path, symbol string
		values                    []string
		resolution                Resolution
	}
	cases := []struct {
		name  string
		build func(s *synthetic)
		want  []want
	}{
		{
			name: "decorator with a path binds the decorated function",
			build: func(s *synthetic) {
				s.object("m", programindex.ObjectModule, "api", "api.py", 1, "")
				s.object("h", programindex.ObjectFunction, "list_items", "api.py", 10, "m")
				s.external("fastapi", "fastapi", "FastAPI", programindex.ExternalAuthorityPackage)
				s.relate("dec", programindex.RelationDecorates, "h", nil, loc("api.py", 9),
					pattern("p", programindex.PatternDecoratorCall, "get", loc("api.py", 9), []string{"fastapi"}, literal(1, "/items")))
			},
			want: []want{{key: "get", method: "GET", path: "/items", symbol: "list_items", values: []string{"/items"}, resolution: ResolutionExact}},
		},
		{
			name: "call with a path and a repository callable",
			build: func(s *synthetic) {
				s.object("main", programindex.ObjectFunction, "main", "main.go", 5, "")
				s.object("h", programindex.ObjectFunction, "serveItems", "main.go", 20, "")
				s.external("router", "github.com/some/router", "Handle", programindex.ExternalAuthorityPackage)
				s.relate("reg", programindex.RelationInvokesExternal, "main", []string{"router"}, loc("main.go", 7),
					pattern("p", programindex.PatternCall, "Handle", loc("main.go", 7), nil, literal(1, "GET /items"), dynamicRef(2, "h")))
				s.callback("cb", "main", "h", "reg", "p", 2)
			},
			want: []want{{key: "Handle", method: "GET", path: "/items", symbol: "serveItems", values: []string{"GET /items"}, resolution: ResolutionExact}},
		},
		{
			name: "a value of an outside type constructed with a callable in a field registers it under the literal beside it",
			build: func(s *synthetic) {
				s.object("main", programindex.ObjectFunction, "main", "main.go", 5, "")
				s.object("h", programindex.ObjectFunction, "runServe", "main.go", 30, "")
				s.external("command", "github.com/spf13/cobra", "Command", programindex.ExternalAuthorityPackage)
				s.relate("new", programindex.RelationInvokesExternal, "main", []string{"command"}, loc("main.go", 8),
					pattern("p", programindex.PatternCall, "Command", loc("main.go", 8), nil, keyword("Use", "serve"), keyword("Short", "Run the server"), dynamicKeyword("RunE", "h")))
				s.callbackKeyword("cb", "main", "h", "new", "p", "RunE")
			},
			want: []want{{key: "Command", symbol: "runServe", values: []string{"Run the server", "serve"}, resolution: ResolutionExact}},
		},
		{
			name: "a request with only a path is a registration without a handler",
			build: func(s *synthetic) {
				s.object("fn", programindex.ObjectFunction, "loadItems", "client.ts", 3, "")
				s.external("axios", "axios", "get", programindex.ExternalAuthorityPackage)
				s.relate("call", programindex.RelationInvokesExternal, "fn", []string{"axios"}, loc("client.ts", 4),
					pattern("p", programindex.PatternCall, "get", loc("client.ts", 4), nil, literal(1, "https://api.example/items")))
			},
			want: []want{{key: "get", method: "GET", path: "https://api.example/items", values: []string{"https://api.example/items"}, resolution: ResolutionExact}},
		},
		{
			name: "a templated path is possible",
			build: func(s *synthetic) {
				s.object("fn", programindex.ObjectFunction, "loadItem", "client.ts", 3, "")
				s.external("axios", "axios", "get", programindex.ExternalAuthorityPackage)
				s.relate("call", programindex.RelationInvokesExternal, "fn", []string{"axios"}, loc("client.ts", 4),
					pattern("p", programindex.PatternCall, "get", loc("client.ts", 4), nil, template(1, "/items/", "")))
			},
			want: []want{{key: "get", method: "GET", path: "/items/{param}", values: []string{"/items/{param}"}, resolution: ResolutionPossible}},
		},
		{
			name: "a value the repository built, named for a host, is a registration too",
			build: func(s *synthetic) {
				s.object("init", programindex.ObjectFunction, "init", "dns.go", 5, "")
				s.external("k6", "go.k6.io/k6/js/modules", "Register", programindex.ExternalAuthorityPackage)
				produced := dynamic(2)
				// new(DNS): a record the repository made, no field stored yet.
				produced.Origin = &sourcevalue.Value{Kind: "record", Anchor: &sourcevalue.Anchor{Path: "dns.go", Line: 6, Column: 30}}
				s.relate("reg", programindex.RelationInvokesExternal, "init", []string{"k6"}, loc("dns.go", 6),
					pattern("p", programindex.PatternCall, "Register", loc("dns.go", 6), nil, literal(1, "k6/x/dns"), produced))
			},
			want: []want{{key: "Register", path: "k6/x/dns", values: []string{"k6/x/dns"}, resolution: ResolutionExact}},
		},
		{
			name: "an unknown receiver keeps the registration possible",
			build: func(s *synthetic) {
				s.object("fn", programindex.ObjectFunction, "startServer", "server.js", 3, "")
				s.object("h", programindex.ObjectFunction, "ping", "server.js", 20, "")
				s.relate("reg", programindex.RelationCalls, "fn", nil, loc("server.js", 5),
					pattern("p", programindex.PatternCall, "get", loc("server.js", 5), nil, literal(1, "/ping"), dynamicRef(2, "h")))
			},
			want: []want{{key: "get", method: "GET", path: "/ping", symbol: "ping", values: []string{"/ping"}, resolution: ResolutionPossible}},
		},
		{
			name: "the repository's own method called get is delegation, not a registration",
			build: func(s *synthetic) {
				s.object("fn", programindex.ObjectFunction, "control", "server.js", 3, "")
				s.object("local", programindex.ObjectMethod, "lookalike.get", "server.js", 30, "")
				s.object("h", programindex.ObjectFunction, "ping", "server.js", 20, "")
				s.relate("call", programindex.RelationCalls, "fn", []string{"local"}, loc("server.js", 5),
					pattern("p", programindex.PatternCall, "get", loc("server.js", 5), nil, literal(1, "/not-a-route"), dynamicRef(2, "h")))
			},
		},
		{
			name: "a format string with a value is not a registration",
			build: func(s *synthetic) {
				s.object("fn", programindex.ObjectFunction, "describe", "main.go", 3, "")
				s.external("fmt", "fmt", "Sprintf", programindex.ExternalAuthorityPlatform)
				argument := dynamic(2)
				argument.Origin = &sourcevalue.Value{Kind: "parameter", Text: "name", Position: 1, Anchor: &sourcevalue.Anchor{Path: "main.go", Line: 3, Column: 15}, Owner: &sourcevalue.Anchor{Path: "main.go", Line: 3, Column: 1}}
				s.relate("call", programindex.RelationInvokesExternal, "fn", []string{"fmt"}, loc("main.go", 4),
					pattern("p", programindex.PatternCall, "Sprintf", loc("main.go", 4), nil, literal(1, "hello %s"), argument))
			},
		},
		{
			name: "two possible callbacks name no handler",
			build: func(s *synthetic) {
				s.object("main", programindex.ObjectFunction, "main", "server.go", 1, "")
				s.object("first", programindex.ObjectFunction, "first", "server.go", 20, "")
				s.object("second", programindex.ObjectFunction, "second", "server.go", 30, "")
				s.external("http", "net/http", "HandleFunc", programindex.ExternalAuthorityPlatform)
				argument := dynamic(2)
				argument.ObjectRefs = []string{"first", "second"}
				argument.Resolution, argument.ObjectsObserved = programindex.ResolutionAlternatives, 2
				s.relate("reg", programindex.RelationInvokesExternal, "main", []string{"http"}, loc("server.go", 3),
					pattern("p", programindex.PatternCall, "HandleFunc", loc("server.go", 3), nil, literal(1, "/ambiguous"), argument))
				for _, name := range []string{"first", "second"} {
					s.callback("cb-"+name, "main", name, "reg", "p", 2)
					callback := &s.relations[len(s.relations)-1]
					callback.ToRefs = []string{"first", "second"}
					callback.Resolution, callback.TargetsObserved = programindex.ResolutionAlternatives, 2
				}
			},
			want: []want{{key: "HandleFunc", path: "/ambiguous", values: []string{"/ambiguous"}, resolution: ResolutionExact}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newSynthetic(t, "go", "shape", "api.py", "main.go", "client.ts", "dns.go", "server.js", "server.go")
			tc.build(s)
			result := mustBuild(t, Input{Targets: []TargetInput{{Index: s.index(), Root: "."}}})
			var got []want
			for _, fact := range result.OfKind(KindRegistration) {
				got = append(got, want{key: fact.Key, method: fact.Method, path: fact.Path, symbol: fact.Symbol, values: fact.Values, resolution: fact.Resolution})
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("registrations = %+v\nwant %+v", got, tc.want)
			}
		})
	}
}

// A router handed to a mount answers under the mount's prefix; a router
// built with its own prefix composes it, marked possible because frameworks
// disagree on whether a mount replaces that prefix.
func TestRegistrationPathsFollowMounts(t *testing.T) {
	s := newSynthetic(t, "python", "api", "api.py")
	s.object("m", programindex.ObjectModule, "api", "api.py", 1, "")
	s.object("app", programindex.ObjectVariable, "app", "api.py", 3, "m")
	s.object("api", programindex.ObjectVariable, "api", "api.py", 4, "m")
	s.object("plain", programindex.ObjectVariable, "plain", "api.py", 5, "m")
	s.object("ping", programindex.ObjectFunction, "ping", "api.py", 10, "m")
	s.object("pong", programindex.ObjectFunction, "pong", "api.py", 20, "m")
	s.external("fastapi", "fastapi", "FastAPI", programindex.ExternalAuthorityPackage)
	s.external("router", "fastapi", "APIRouter", programindex.ExternalAuthorityPackage)
	s.external("include", "fastapi", "include_router", programindex.ExternalAuthorityPackage)
	prefixed := pattern("p", programindex.PatternCall, "APIRouter", loc("api.py", 4), nil, keyword("prefix", "/v1"))
	prefixed.ResultRef = "api"
	s.relate("mk-api", programindex.RelationInvokesExternal, "m", []string{"router"}, loc("api.py", 4), prefixed)
	bare := pattern("p", programindex.PatternCall, "APIRouter", loc("api.py", 5), nil)
	bare.ResultRef = "plain"
	s.relate("mk-plain", programindex.RelationInvokesExternal, "m", []string{"router"}, loc("api.py", 5), bare)
	s.relate("dec-ping", programindex.RelationDecorates, "ping", nil, loc("api.py", 9),
		receiverPattern("api", "get", loc("api.py", 9), literal(1, "/ping")))
	s.relate("dec-pong", programindex.RelationDecorates, "pong", nil, loc("api.py", 19),
		receiverPattern("plain", "get", loc("api.py", 19), literal(1, "/pong")))
	s.relate("mount-api", programindex.RelationInvokesExternal, "m", []string{"include"}, loc("api.py", 30),
		receiverPattern("app", "include_router", loc("api.py", 30), dynamicRef(1, "api"), keyword("prefix", "/api")))
	s.relate("mount-plain", programindex.RelationInvokesExternal, "m", []string{"include"}, loc("api.py", 31),
		receiverPattern("app", "include_router", loc("api.py", 31), dynamicRef(1, "plain"), keyword("prefix", "/private")))
	result := mustBuild(t, Input{Targets: []TargetInput{{Index: s.index(), Root: "."}}})
	got := map[string]Resolution{}
	for _, fact := range result.OfKind(KindRegistration) {
		if fact.Symbol != "" {
			got[fact.Symbol+" "+fact.Path] = fact.Resolution
		}
	}
	want := map[string]Resolution{"ping /api/v1/ping": ResolutionPossible, "pong /private/pong": ResolutionExact}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mounted paths = %v, want %v", got, want)
	}
}

func receiverPattern(receiver, selector string, location *programindex.Location, arguments ...programindex.PatternArgumentInput) programindex.RelationPatternInput {
	form := programindex.PatternCall
	if selector == "get" {
		form = programindex.PatternDecoratorCall
	}
	input := pattern("p", form, selector, location, nil, arguments...)
	input.ReceiverRef = receiver
	return input
}

func TestSQLStatementsNameTheirTables(t *testing.T) {
	s := newSynthetic(t, "go", "store", "store.go")
	s.object("fn", programindex.ObjectMethod, "Queries.GetUser", "store.go", 10, "")
	s.external("sql", "database/sql", "QueryRowContext", programindex.ExternalAuthorityPlatform)
	s.relate("q", programindex.RelationInvokesExternal, "fn", []string{"sql"}, loc("store.go", 12),
		pattern("p", programindex.PatternCall, "QueryRowContext", loc("store.go", 12), nil, dynamic(1),
			literal(2, "-- name: GetUser :one\nSELECT u.id, o.total FROM users u JOIN orders o ON o.user_id = u.id WHERE u.id = $1")))
	result := mustBuild(t, Input{Targets: []TargetInput{{Index: s.index(), Root: "."}}})
	queries := result.OfKind(KindSQLQuery)
	if len(queries) != 1 || queries[0].Key != "orders, users" || queries[0].Symbol != "Queries.GetUser" || queries[0].Anchor.String() != "store.go:12" {
		t.Fatalf("sql queries = %+v", queries)
	}
	if got := sqlTables("INSERT INTO audit_log (id) VALUES ($1); UPDATE users SET seen = now()"); !reflect.DeepEqual(got, []string{"audit_log", "users"}) {
		t.Fatalf("tables = %v", got)
	}
}

// Dead code is judged for the repository: a file one service reaches is not
// dead for another, and a file with nothing to run is never judged.
func TestDeadModulesAreJudgedRepositoryWide(t *testing.T) {
	api := newSynthetic(t, "go", "api", "cmd/api/main.go", "internal/shared/shared.go", "internal/model/user.go", "internal/unused/unused.go")
	api.object("main", programindex.ObjectFunction, "main", "cmd/api/main.go", 3, "")
	api.object("shared", programindex.ObjectFunction, "Shared", "internal/shared/shared.go", 3, "")
	api.object("user", programindex.ObjectType, "User", "internal/model/user.go", 3, "")
	api.object("unused", programindex.ObjectFunction, "Unused", "internal/unused/unused.go", 3, "")
	api.seed("main", programindex.SeedCallable, "cmd/api/main.go", 3)
	worker := newSynthetic(t, "go", "worker", "cmd/worker/main.go", "internal/shared/shared.go")
	worker.object("main", programindex.ObjectFunction, "main", "cmd/worker/main.go", 3, "")
	worker.object("shared", programindex.ObjectFunction, "Shared", "internal/shared/shared.go", 3, "")
	worker.seed("main", programindex.SeedCallable, "cmd/worker/main.go", 3)
	worker.relate("c", programindex.RelationCalls, "main", []string{"shared"}, loc("cmd/worker/main.go", 4))
	indexes := bindTargetSet(t, api.index(), worker.index())
	result := mustBuild(t, Input{Targets: []TargetInput{{Index: indexes[0], Root: "."}, {Index: indexes[1], Root: "."}}})
	var dead []string
	for _, fact := range result.OfKind(KindDeadModule) {
		dead = append(dead, fact.Path)
	}
	if !reflect.DeepEqual(dead, []string{"internal/unused/unused.go"}) {
		t.Fatalf("dead = %v", dead)
	}
	worker.relate("i", programindex.RelationImports, "main", []string{"shared"}, loc("cmd/worker/main.go", 2))
	result = mustBuild(t, Input{Targets: []TargetInput{{Index: worker.index(), Root: "."}}})
	if imports := result.OfKind(KindImport); len(imports) != 1 || imports[0].Path != "internal/shared/shared.go" || imports[0].Anchor.String() != "cmd/worker/main.go:2" {
		t.Fatalf("imports = %+v", imports)
	}
}

func TestDeadModulesNeedAnEntrypoint(t *testing.T) {
	s := newSynthetic(t, "python", "lib", "lib/a.py")
	s.object("a", programindex.ObjectModule, "a", "lib/a.py", 1, "")
	s.object("fn", programindex.ObjectFunction, "helper", "lib/a.py", 3, "a")
	result := mustBuild(t, Input{Targets: []TargetInput{{Index: s.index(), Root: "lib"}}})
	if dead := result.OfKind(KindDeadModule); len(dead) != 0 || !hasDiagnostic(result, "dead_module_skipped") {
		t.Fatalf("library without seeds: dead=%+v diagnostics=%+v", dead, result.Diagnostics)
	}
}
