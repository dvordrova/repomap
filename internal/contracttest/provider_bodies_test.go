package contracttest

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas/places"
	"github.com/dvordrova/repomap/internal/atlas/reading"
	"github.com/dvordrova/repomap/internal/claims"
	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/orientation"
	"github.com/dvordrova/repomap/internal/programindex"
)

// kvd's calls through its command table's proc field and its event loop's
// kept callables carry the stores that reach each function: local keys that
// name the function by its canonical place ID. Every body a run of kvd sends
// (the reading's tables, Jev's questions and the orientation) names code by
// path, line and name, never by an ID or a host path. Redis's orientation
// once sent "callee_id":"sym:redis.c:1398:beforeSleep".
func TestProviderBodiesCarryNoCanonicalIDsOrHostPaths(t *testing.T) {
	fixture := loadCFixture(t)
	set := buildCSet(t, fixture, "c:kvd", "c:kvcli")
	server, client := set["c:kvd"], set["c:kvcli"]
	layer, err := facts.Build(facts.Input{Repository: fixture.repository, Targets: []facts.TargetInput{{Index: server, Root: "."}, {Index: client, Root: "."}}})
	if err != nil {
		t.Fatal(err)
	}
	graph, err := places.Build(places.Input{Repository: fixture.repository, Targets: []places.TargetInput{{Index: server, Root: "."}, {Index: client, Root: "."}}, Facts: layer})
	if err != nil {
		t.Fatal(err)
	}
	stores := 0
	for _, place := range graph.Places {
		if place.Symbol != nil {
			for _, call := range place.Symbol.Calls {
				stores += len(call.Stores)
			}
		}
	}
	if stores == 0 {
		t.Fatal("no call of kvd carries a store: the test no longer covers them")
	}
	preset := &kvdPreset{roles: map[string]map[string]string{"sys/socket.h.connect": {"talks": "client_request"}}}
	categorizer := preset.categorizer()
	var metas []reading.TargetMeta
	for _, index := range []programindex.Index{server, client} {
		metas = append(metas, reading.TargetMeta{ID: index.Target.ID, Language: "c", Kind: "executable", Name: index.Target.Name, Root: "."})
	}
	read, err := reading.Read(t.Context(), reading.Options{
		Graph: graph, Repository: "kvd", Revision: "test", NoCaptions: true, Targets: metas,
		Executor: llm.Executor{BatchConcurrency: 1, BatchController: &llm.BatchController{}},
		Provider: preset, Categorizer: categorizer, OwnerRunDir: t.TempDir(),
		ReadSource: func(path string) ([]byte, error) { return fixture.source(t, path), nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	indexes, err := groupindex.ProjectAtlas(map[string]programindex.Index{server.Target.ID: server, client.Target.ID: client}, read.Atlas)
	if err != nil {
		t.Fatal(err)
	}
	noClaims, err := claims.Seal(claims.Result{Revision: "test", Claims: []claims.Claim{}})
	if err != nil {
		t.Fatal(err)
	}
	asked := &capturedOrientation{}
	if _, _, err := orientation.Run(t.Context(), llm.Executor{BatchConcurrency: 1, BatchController: &llm.BatchController{}}, asked,
		orientation.Input{RepositoryName: "kvd", Facts: layer, Claims: noClaims, Groups: indexes, Graph: graph}); err != nil {
		t.Fatal(err)
	}
	preset.mu.Lock()
	bodies := append(preset.requests, categorizer.Requests()...)
	preset.mu.Unlock()
	if len(bodies) == 0 || categorizer.Calls() == 0 {
		t.Fatalf("the reading sent %d bodies, %d to Jev", len(bodies), categorizer.Calls())
	}
	bodies = append(bodies, asked.request(t))
	assertNoLocalIdentities(t, bodies, fixture.root, repositoryRoot(t))
}

// assertNoLocalIdentities fails when a provider body names code by a local
// identity: a callee or store ID, a canonical place ID or a host-absolute
// path. Each body must be JSON, as every provider body is.
func assertNoLocalIdentities(t *testing.T, bodies [][]byte, hostPaths ...string) {
	t.Helper()
	for _, body := range bodies {
		if !json.Valid(body) {
			t.Fatalf("a provider body is not JSON: %.200s", body)
		}
		text := string(body)
		for _, local := range append([]string{`"callee_id"`, `"callee_ids"`, `"stores"`, `"place_id"`, `"object_id"`, `sym:`}, hostPaths...) {
			if at := strings.Index(text, local); at >= 0 {
				t.Fatalf("a provider body carries %q: …%s…", local, text[max(0, at-200):min(len(text), at+200)])
			}
		}
	}
}
