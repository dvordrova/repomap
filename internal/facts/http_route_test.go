package facts

import (
	"testing"

	"github.com/dvordrova/repomap/internal/programindex"
)

func TestGoServeMuxPreservesNativeMethodHostAndWildcardPattern(t *testing.T) {
	for _, tc := range []struct {
		pattern, method, path string
		ok                    bool
	}{
		{"GET /health", "GET", "/health", true},
		{"PATCH\t  /tasks/{id}", "PATCH", "/tasks/{id}", true},
		{"HEAD status.example/{$}", "HEAD", "status.example/{$}", true},
		{"/files/{path...}", "ANY", "/files/{path...}", true},
		{"PURGE /cache/{key}", "PURGE", "/cache/{key}", true},
		{"custom /path", "custom", "/path", true},
		{"", "", "", false},
		{"GET nowhere", "", "", false},
		{"GET/POST /path", "", "", false},
	} {
		t.Run(tc.pattern, func(t *testing.T) {
			method, path, ok := goServeMuxMethodAndPath(tc.pattern)
			if ok != tc.ok || ok && (method != tc.method || path != tc.path) {
				t.Fatalf("%q => %q %q %v; want %q %q %v", tc.pattern, method, path, ok, tc.method, tc.path, tc.ok)
			}
		})
	}
}

func TestHTTPRouteAmbiguousCallbacksDoNotChooseFirstObservation(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		s := newSynthetic(t, "go", "route-callbacks", "server.go")
		s.object("main", programindex.ObjectFunction, "main", "server.go", 1, "")
		s.object("first", programindex.ObjectFunction, "first", "server.go", 20, "")
		s.object("second", programindex.ObjectFunction, "second", "server.go", 30, "")
		s.external("http", "net/http", "HandleFunc", programindex.ExternalAuthorityPackage)
		argument := dynamic(2)
		argument.ObjectRefs = []string{"first", "second"}
		argument.Resolution, argument.ObjectsObserved = programindex.ResolutionAlternatives, 2
		s.relate("reg", programindex.RelationInvokesExternal, "main", []string{"http"}, loc("server.go", 3),
			pattern("p", programindex.PatternCall, "HandleFunc", loc("server.go", 3), nil, literal(1, "GET /ambiguous"), argument))
		order := []string{"first", "second"}
		if reverse {
			order[0], order[1] = order[1], order[0]
		}
		for _, name := range order {
			s.callback("cb-"+name, "main", name, "reg", "p", 2)
			callback := &s.relations[len(s.relations)-1]
			callback.ToRefs = append([]string(nil), order...)
			callback.Resolution, callback.TargetsObserved = programindex.ResolutionAlternatives, 2
		}
		result := mustBuild(t, Input{Targets: []TargetInput{{Index: s.index(), Root: "."}}})
		routes := result.OfKind(KindHTTPRoute)
		if len(routes) != 1 || routes[0].Method != "GET" || routes[0].Path != "/ambiguous" || routes[0].ObjectID != "" || routes[0].Symbol != "" {
			t.Fatalf("source route was lost or chose an ambiguous callback (reverse=%v): %+v", reverse, routes)
		}
	}
}
