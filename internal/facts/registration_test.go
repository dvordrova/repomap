package facts

import (
	"fmt"
	"reflect"
	"sort"
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
		// registrar is what the registration hands its callable or value
		// to: the outside symbol, or a repository table row's record field.
		registrar  string
		resolution Resolution
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
			want: []want{{key: "get", method: "GET", path: "/items", symbol: "list_items", values: []string{"/items"}, registrar: "fastapi.FastAPI.get", resolution: ResolutionExact}},
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
			want: []want{{key: "Handle", method: "GET", path: "/items", symbol: "serveItems", values: []string{"GET /items"}, registrar: "github.com/some/router.Handle", resolution: ResolutionExact}},
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
			want: []want{{key: "Command", symbol: "runServe", values: []string{"Run the server", "serve"}, registrar: "github.com/spf13/cobra.Command", resolution: ResolutionExact}},
		},
		{
			// The row's registrar is its record type's field, known exactly:
			// the reading asks what that field's callables become.
			name: "a row of a table the repository owns registers its callable under the literal beside it",
			build: func(s *synthetic) {
				s.object("m", programindex.ObjectModule, "server.c", "server.c", 1, "")
				s.object("command", programindex.ObjectType, "command", "server.c", 3, "m")
				s.object("table", programindex.ObjectVariable, "cmdTable", "server.c", 10, "m")
				s.object("h", programindex.ObjectFunction, "pingCommand", "server.c", 30, "m")
				s.relate("row", programindex.RelationCalls, "table", []string{"command"}, loc("server.c", 11),
					pattern("p", programindex.PatternCall, "command", loc("server.c", 11), nil, keyword("name", "ping"), dynamicKeyword("proc", "h")))
				s.relations[len(s.relations)-1].Invocation = programindex.InvocationConstruct
				s.callbackKeyword("cb", "table", "h", "row", "p", "proc")
			},
			want: []want{{key: "command", symbol: "pingCommand", values: []string{"ping"}, registrar: "server.c.command.proc", resolution: ResolutionExact}},
		},
		{
			// A command called get is a name, not an HTTP method: nothing in
			// the row is an address the verb could qualify.
			name: "a row named by a verb-shaped literal states no method",
			build: func(s *synthetic) {
				s.object("m", programindex.ObjectModule, "server.c", "server.c", 1, "")
				s.object("command", programindex.ObjectType, "command", "server.c", 3, "m")
				s.object("table", programindex.ObjectVariable, "cmdTable", "server.c", 10, "m")
				s.object("h", programindex.ObjectFunction, "getCommand", "server.c", 30, "m")
				s.relate("row", programindex.RelationCalls, "table", []string{"command"}, loc("server.c", 11),
					pattern("p", programindex.PatternCall, "command", loc("server.c", 11), nil, keyword("name", "get"), dynamicKeyword("proc", "h")))
				s.relations[len(s.relations)-1].Invocation = programindex.InvocationConstruct
				s.callbackKeyword("cb", "table", "h", "row", "p", "proc")
			},
			want: []want{{key: "command", symbol: "getCommand", values: []string{"get"}, registrar: "server.c.command.proc", resolution: ResolutionExact}},
		},
		{
			name: "a verb-shaped first literal with no address names the handler and states no method",
			build: func(s *synthetic) {
				s.object("main", programindex.ObjectFunction, "main", "main.go", 5, "")
				s.object("h", programindex.ObjectFunction, "removeItem", "main.go", 20, "")
				s.external("bus", "github.com/some/bus", "On", programindex.ExternalAuthorityPackage)
				s.relate("on", programindex.RelationInvokesExternal, "main", []string{"bus"}, loc("main.go", 7),
					pattern("p", programindex.PatternCall, "On", loc("main.go", 7), nil, literal(1, "delete"), dynamicRef(2, "h")))
				s.callback("cb", "main", "h", "on", "p", 2)
			},
			want: []want{{key: "On", symbol: "removeItem", values: []string{"delete"}, registrar: "github.com/some/bus.On", resolution: ResolutionExact}},
		},
		{
			name: "a verb literal beside an address states the method",
			build: func(s *synthetic) {
				s.object("fn", programindex.ObjectFunction, "loadItems", "main.go", 3, "")
				s.external("request", "net/http", "NewRequest", programindex.ExternalAuthorityPlatform)
				s.relate("call", programindex.RelationInvokesExternal, "fn", []string{"request"}, loc("main.go", 4),
					pattern("p", programindex.PatternCall, "NewRequest", loc("main.go", 4), nil, literal(1, "POST"), literal(2, "https://api.example/items"), dynamic(3)))
			},
			want: []want{{key: "NewRequest", method: "POST", path: "https://api.example/items", values: []string{"POST", "https://api.example/items"}, registrar: "net/http.NewRequest", resolution: ResolutionExact}},
		},
		{
			// rest.Route{Method: http.MethodGet, Path: "/users", Handler: h}:
			// the fields are keyword literals, and the path beside the verb
			// is the address it qualifies.
			name: "a route record's verb field beside its path field states the method",
			build: func(s *synthetic) {
				s.object("main", programindex.ObjectFunction, "main", "main.go", 5, "")
				s.object("h", programindex.ObjectFunction, "listUsers", "main.go", 30, "")
				s.external("route", "github.com/some/rest", "Route", programindex.ExternalAuthorityPackage)
				s.relate("new", programindex.RelationInvokesExternal, "main", []string{"route"}, loc("main.go", 8),
					pattern("p", programindex.PatternCall, "Route", loc("main.go", 8), nil, keyword("Method", "GET"), keyword("Path", "/users"), dynamicKeyword("Handler", "h")))
				s.callbackKeyword("cb", "main", "h", "new", "p", "Handler")
			},
			want: []want{{key: "Route", method: "GET", symbol: "listUsers", values: []string{"GET", "/users"}, registrar: "github.com/some/rest.Route", resolution: ResolutionExact}},
		},
		{
			// {"GET", "/health", health} in a C route table: the row's verb
			// qualifies the path beside it, unlike a command row's name.
			name: "a table row with a verb beside a path states the method",
			build: func(s *synthetic) {
				s.object("m", programindex.ObjectModule, "server.c", "server.c", 1, "")
				s.object("route", programindex.ObjectType, "route", "server.c", 3, "m")
				s.object("table", programindex.ObjectVariable, "routes", "server.c", 10, "m")
				s.object("h", programindex.ObjectFunction, "health", "server.c", 30, "m")
				s.relate("row", programindex.RelationCalls, "table", []string{"route"}, loc("server.c", 11),
					pattern("p", programindex.PatternCall, "route", loc("server.c", 11), nil, keyword("method", "GET"), keyword("path", "/health"), dynamicKeyword("handler", "h")))
				s.relations[len(s.relations)-1].Invocation = programindex.InvocationConstruct
				s.callbackKeyword("cb", "table", "h", "row", "p", "handler")
			},
			want: []want{{key: "route", method: "GET", symbol: "health", values: []string{"GET", "/health"}, registrar: "server.c.route.handler", resolution: ResolutionExact}},
		},
		{
			// net/http writes a pattern's method in capitals; a sentence with
			// a word, a space and a slash in it is a message.
			name: "prose with a slash in it is no address",
			build: func(s *synthetic) {
				s.object("fn", programindex.ObjectFunction, "openLog", "server.c", 3, "")
				s.external("log", "stdio.h", "fprintf", programindex.ExternalAuthorityPlatform)
				for line, text := range []string{"open /dev/null: %s", "failed to read the a/b pair", "get key/value"} {
					s.relate(fmt.Sprintf("log%d", line), programindex.RelationInvokesExternal, "fn", []string{"log"}, loc("server.c", 4+line),
						pattern("p", programindex.PatternCall, "fprintf", loc("server.c", 4+line), nil, dynamic(1), literal(2, text)))
				}
			},
		},
		{
			name: "a row of a table the repository owns that stores no callable is data",
			build: func(s *synthetic) {
				s.object("m", programindex.ObjectModule, "server.c", "server.c", 1, "")
				s.object("setting", programindex.ObjectType, "setting", "server.c", 3, "m")
				s.object("limits", programindex.ObjectVariable, "limits", "server.c", 5, "m")
				s.object("table", programindex.ObjectVariable, "settings", "server.c", 10, "m")
				s.relate("row", programindex.RelationCalls, "table", []string{"setting"}, loc("server.c", 11),
					pattern("p", programindex.PatternCall, "setting", loc("server.c", 11), nil, literal(1, "maxclients"), dynamicRef(2, "limits")))
				s.relations[len(s.relations)-1].Invocation = programindex.InvocationConstruct
			},
		},
		{
			name: "the same row a function builds for itself is delegation",
			build: func(s *synthetic) {
				s.object("m", programindex.ObjectModule, "server.c", "server.c", 1, "")
				s.object("command", programindex.ObjectType, "command", "server.c", 3, "m")
				s.object("fn", programindex.ObjectFunction, "once", "server.c", 20, "m")
				s.object("h", programindex.ObjectFunction, "pingCommand", "server.c", 30, "m")
				s.relate("row", programindex.RelationCalls, "fn", []string{"command"}, loc("server.c", 21),
					pattern("p", programindex.PatternCall, "command", loc("server.c", 21), nil, keyword("name", "ping"), dynamicKeyword("proc", "h")))
				s.relations[len(s.relations)-1].Invocation = programindex.InvocationConstruct
				s.callbackKeyword("cb", "fn", "h", "row", "p", "proc")
			},
		},
		{
			name: "a request with only a path is a registration without a handler",
			build: func(s *synthetic) {
				s.object("fn", programindex.ObjectFunction, "loadItems", "client.ts", 3, "")
				s.external("axios", "axios", "get", programindex.ExternalAuthorityPackage)
				s.relate("call", programindex.RelationInvokesExternal, "fn", []string{"axios"}, loc("client.ts", 4),
					pattern("p", programindex.PatternCall, "get", loc("client.ts", 4), nil, literal(1, "https://api.example/items")))
			},
			want: []want{{key: "get", method: "GET", path: "https://api.example/items", values: []string{"https://api.example/items"}, registrar: "axios.get", resolution: ResolutionExact}},
		},
		{
			name: "a templated path is possible",
			build: func(s *synthetic) {
				s.object("fn", programindex.ObjectFunction, "loadItem", "client.ts", 3, "")
				s.external("axios", "axios", "get", programindex.ExternalAuthorityPackage)
				s.relate("call", programindex.RelationInvokesExternal, "fn", []string{"axios"}, loc("client.ts", 4),
					pattern("p", programindex.PatternCall, "get", loc("client.ts", 4), nil, template(1, "/items/", "")))
			},
			want: []want{{key: "get", method: "GET", path: "/items/{param}", values: []string{"/items/{param}"}, registrar: "axios.get", resolution: ResolutionPossible}},
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
			want: []want{{key: "Register", path: "k6/x/dns", values: []string{"k6/x/dns"}, registrar: "go.k6.io/k6/js/modules.Register", resolution: ResolutionExact}},
		},
		{
			name: "a value set on the request's own context is no registration",
			build: func(s *synthetic) {
				s.object("fn", programindex.ObjectFunction, "updateContext", "main.go", 3, "")
				s.external("set", "github.com/gin-gonic/gin", "Set", programindex.ExternalAuthorityPackage)
				produced := dynamic(2)
				produced.Origin = &sourcevalue.Value{Kind: "record", Anchor: &sourcevalue.Anchor{Path: "main.go", Line: 4, Column: 9}}
				call := pattern("p", programindex.PatternCall, "Set", loc("main.go", 5), nil, literal(1, "my_user_model"), produced)
				// c is a parameter no repository caller supplies.
				call.ReceiverValue = &sourcevalue.Value{Kind: "parameter", Text: "c", Position: 1, Anchor: &sourcevalue.Anchor{Path: "main.go", Line: 3, Column: 20}, Owner: &sourcevalue.Anchor{Path: "main.go", Line: 3, Column: 1}}
				s.relate("call", programindex.RelationInvokesExternal, "fn", []string{"set"}, loc("main.go", 5), call)
			},
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
			// websocket.WebSocketApp(url, on_message=on_message,
			// on_close=on_close), (ws/websocket url {:on-message f
			// :on-close g}): the call names no one handler, so each keyword
			// entry registers its own at the address, the keyword its registrar's
			// last part and its first word.
			name: "an outside call handed two callables under keywords registers each under its keyword",
			build: func(s *synthetic) {
				s.object("main", programindex.ObjectFunction, "open_feed", "main.go", 5, "")
				s.object("message", programindex.ObjectFunction, "on_message", "main.go", 20, "")
				s.object("close", programindex.ObjectFunction, "on_close", "main.go", 30, "")
				s.external("app", "websocket", "WebSocketApp", programindex.ExternalAuthorityPackage)
				s.relate("new", programindex.RelationInvokesExternal, "main", []string{"app"}, loc("main.go", 7),
					pattern("p", programindex.PatternCall, "WebSocketApp", loc("main.go", 7), nil, literal(1, "wss://stream.example/prices"),
						dynamicKeyword("on_message", "message"), dynamicKeyword("on_close", "close")))
				s.callbackKeyword("cb-message", "main", "message", "new", "p", "on_message")
				s.callbackKeyword("cb-close", "main", "close", "new", "p", "on_close")
			},
			want: []want{
				{key: "WebSocketApp", path: "wss://stream.example/prices", symbol: "on_message", values: []string{"on_message", "wss://stream.example/prices"}, registrar: "websocket.WebSocketApp.on_message", resolution: ResolutionExact},
				{key: "WebSocketApp", path: "wss://stream.example/prices", symbol: "on_close", values: []string{"on_close", "wss://stream.example/prices"}, registrar: "websocket.WebSocketApp.on_close", resolution: ResolutionExact},
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
			want: []want{{key: "HandleFunc", path: "/ambiguous", values: []string{"/ambiguous"}, registrar: "net/http.HandleFunc", resolution: ResolutionExact}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newSynthetic(t, "go", "shape", "api.py", "main.go", "client.ts", "dns.go", "server.js", "server.go", "server.c")
			tc.build(s)
			result := mustBuild(t, Input{Targets: []TargetInput{{Index: s.index(), Root: "."}}})
			var got []want
			for _, fact := range result.OfKind(KindRegistration) {
				got = append(got, want{key: fact.Key, method: fact.Method, path: fact.Path, symbol: fact.Symbol, values: fact.Values, registrar: fact.Text, resolution: fact.Resolution})
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
	// Literals that only start with an SQL verb, handed to calls outside the
	// repository in the same function, are not statements.
	s.external("errorf", "fmt", "Errorf", programindex.ExternalAuthorityPlatform)
	s.relate("e", programindex.RelationInvokesExternal, "fn", []string{"errorf"}, loc("store.go", 14),
		pattern("p", programindex.PatternCall, "Errorf", loc("store.go", 14), nil, literal(1, "create %s dir: %w"), dynamic(2)))
	s.external("fold", "strings", "EqualFold", programindex.ExternalAuthorityPlatform)
	s.relate("f", programindex.RelationInvokesExternal, "fn", []string{"fold"}, loc("store.go", 15),
		pattern("p", programindex.PatternCall, "EqualFold", loc("store.go", 15), nil, dynamic(1), literal(2, "with")))
	// A statement whose table is a printf verb is still a statement; the table
	// is filled in at run time, so it names none. IF EXISTS is not a table.
	s.external("sprintf", "fmt", "Sprintf", programindex.ExternalAuthorityPlatform)
	s.relate("s", programindex.RelationInvokesExternal, "fn", []string{"sprintf"}, loc("store.go", 16),
		pattern("p", programindex.PatternCall, "Sprintf", loc("store.go", 16), nil, literal(1, "DROP TABLE IF EXISTS %s"), dynamic(2)))
	s.external("exec", "database/sql", "ExecContext", programindex.ExternalAuthorityPlatform)
	s.relate("x", programindex.RelationInvokesExternal, "fn", []string{"exec"}, loc("store.go", 17),
		pattern("p", programindex.PatternCall, "ExecContext", loc("store.go", 17), nil, dynamic(1), literal(2, "DROP TABLE IF EXISTS authors CASCADE")))
	result := mustBuild(t, Input{Targets: []TargetInput{{Index: s.index(), Root: "."}}})
	keys := map[string]string{}
	for _, query := range result.OfKind(KindSQLQuery) {
		keys[query.Anchor.String()] = query.Key
		if query.Anchor.String() == "store.go:12" && query.Symbol != "Queries.GetUser" {
			t.Fatalf("sql query symbol = %+v", query)
		}
	}
	if want := map[string]string{"store.go:12": "orders, users", "store.go:16": "", "store.go:17": "authors"}; !reflect.DeepEqual(keys, want) {
		t.Fatalf("sql query tables = %v, want %v", keys, want)
	}
}

// metabase: (str "SELECT 0 AS A" " UNION ALL" " SELECT 0 AS A") hands one
// statement twice, apart only in spacing. Two facts at one call read the same,
// and the atlas refused the graph for a place with two origins in one target.
func TestAStatementHandedTwiceToOneCallIsOneFact(t *testing.T) {
	s := newSynthetic(t, "go", "store", "store.go")
	s.object("fn", programindex.ObjectFunction, "zeroRows", "store.go", 10, "")
	s.external("concat", "strings", "Join", programindex.ExternalAuthorityPlatform)
	s.relate("c", programindex.RelationInvokesExternal, "fn", []string{"concat"}, loc("store.go", 12),
		pattern("p", programindex.PatternCall, "Join", loc("store.go", 12), nil,
			literal(1, "SELECT 0 AS a"), literal(2, " UNION ALL"), literal(3, " SELECT 0 AS a"), literal(4, "SELECT 1 AS b")))
	result := mustBuild(t, Input{Targets: []TargetInput{{Index: s.index(), Root: "."}}})
	var values []string
	for _, query := range result.OfKind(KindSQLQuery) {
		values = append(values, query.Value)
	}
	sort.Strings(values)
	if want := []string{"SELECT 0 AS a", "SELECT 1 AS b"}; !reflect.DeepEqual(values, want) {
		t.Fatalf("statements at one call = %v, want %v", values, want)
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

// A keyword names a prefix when its last word is prefix; a path-shaped
// literal under any other keyword is a value (statedPrefix).
func TestAPrefixKeywordEndsInTheWordPrefix(t *testing.T) {
	for keyword, want := range map[string]bool{
		"prefix": true, "url_prefix": true, "urlPrefix": true, "Prefix": true,
		"description": false, "docs_url": false, "path": false, "prefixes": false, "prefix_length": false, "suffixprefix": false,
	} {
		if got := namesPrefix(keyword); got != want {
			t.Errorf("namesPrefix(%q) = %v, want %v", keyword, got, want)
		}
	}
}
