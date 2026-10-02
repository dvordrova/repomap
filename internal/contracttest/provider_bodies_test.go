package contracttest

import (
	"encoding/json"
	"slices"
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
// path, line and name, never by an ID or a host path (the orientation's
// overview and its Main flow's splits alike). Redis's orientation
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
	asked := &capturedOrientation{flowTarget: server.Target.ID}
	readCalls, readRequests := categorizer.Calls(), len(categorizer.Requests())
	if _, _, err := orientation.Run(t.Context(), llm.Executor{BatchConcurrency: 1, BatchController: &llm.BatchController{}}, asked,
		orientation.Input{RepositoryName: "kvd", Facts: layer, Claims: noClaims, Groups: indexes, Graph: graph, Categorizer: categorizer}); err != nil {
		t.Fatal(err)
	}
	preset.mu.Lock()
	bodies := append(preset.requests, categorizer.Requests()...)
	preset.mu.Unlock()
	if len(bodies) == 0 || readCalls == 0 {
		t.Fatalf("the reading sent %d bodies, %d to Jev", len(bodies), readCalls)
	}
	if len(categorizer.Requests()) == readRequests {
		t.Fatal("the Main flow's walk asked Jev nothing: the test no longer covers its splits")
	}
	orientationBodies := asked.bodies(t)
	if len(orientationBodies) != 1 {
		t.Fatalf("the orientation sent %d bodies to the provider, want the overview alone", len(orientationBodies))
	}
	bodies = append(bodies, orientationBodies...)
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

// No author prose reaches a model: a docstring or comment speaks the
// author's intent, not what the code is (owner, 2026-09-25: model inputs are
// code structure, never docstrings), and casdoor's ApiController row had
// carried 77 KB of swagger comments past the categorizer's envelope. Every
// body kvd's reading sends, to the text model and to Jev, carries none of
// the doc comments its places hold (kvd.c's "Runs every complete line of
// the query buffer as a command.").
func TestProviderBodiesCarryNoAuthorDocs(t *testing.T) {
	fixture := loadCFixture(t)
	set := buildCSet(t, fixture, "c:kvd", "c:kvcli")
	server, client := set["c:kvd"], set["c:kvcli"]
	layer, err := facts.Build(facts.Input{Repository: fixture.repository, Targets: []facts.TargetInput{{Index: server, Root: "."}, {Index: client, Root: "."}}})
	if err != nil {
		t.Fatal(err)
	}
	// Claims carry their commit's date.
	runFixtureGit(t, fixture.root, "-c", "user.email=fixture@example.test", "-c", "user.name=fixture", "-c", "commit.gpgsign=false", "commit", "--quiet", "-m", "fixture")
	quoted, err := claims.Extract(t.Context(), claims.Input{Repository: fixture.repository, RepoPath: fixture.root, Revision: "HEAD"})
	if err != nil {
		t.Fatal(err)
	}
	graph, err := places.Build(places.Input{Repository: fixture.repository, Targets: []places.TargetInput{{Index: server, Root: "."}, {Index: client, Root: "."}}, Facts: layer, Claims: quoted})
	if err != nil {
		t.Fatal(err)
	}
	var docs []string
	add := func(doc string) {
		if len(doc) >= 16 && !slices.Contains(docs, doc) {
			docs = append(docs, doc)
		}
	}
	// A directory's README first line is the authors' claim too: no
	// directory, target or boundary row carries it (casdoor's boundaries
	// had read "Supporting MCP, A2A, OAuth&nbsp;2.0, …<br>" as every
	// outgoing owner's ancestor context).
	readmeLines := 0
	for _, place := range graph.Places {
		if place.Directory != nil {
			add(place.Directory.Doc)
			if place.Directory.Readme != "" {
				add(place.Directory.Readme)
				readmeLines++
			}
		}
		if place.File != nil {
			add(place.File.Doc)
			for _, decl := range place.File.Decls {
				add(decl.Doc)
			}
		}
		if place.Symbol != nil {
			add(place.Symbol.Decl.Doc)
			for _, member := range place.Symbol.Members {
				add(member.Decl.Doc)
			}
		}
		if place.Boundary != nil {
			add(place.Boundary.CallerDoc)
		}
	}
	if !slices.ContainsFunc(docs, func(doc string) bool { return strings.Contains(doc, "Runs every complete line") }) {
		t.Fatalf("kvd's places hold none of its doc comments: the test no longer covers them (%q)", docs)
	}
	if readmeLines == 0 {
		t.Fatal("kvd's directories hold no README line: the test no longer covers them")
	}
	preset := &kvdPreset{roles: map[string]map[string]string{"sys/socket.h.connect": {"talks": "client_request"}}}
	categorizer := preset.categorizer()
	var metas []reading.TargetMeta
	for _, index := range []programindex.Index{server, client} {
		metas = append(metas, reading.TargetMeta{ID: index.Target.ID, Language: "c", Kind: "executable", Name: index.Target.Name, Root: "."})
	}
	read, err := reading.Read(t.Context(), reading.Options{
		Graph: graph, Repository: "kvd", Revision: "test", NoCaptions: false, Targets: metas,
		Executor: llm.Executor{BatchConcurrency: 1, BatchController: &llm.BatchController{}},
		Provider: preset, Categorizer: categorizer, OwnerRunDir: t.TempDir(),
		ReadSource: func(path string) ([]byte, error) { return fixture.source(t, path), nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	// The orientation is code structure too: no README line, docstring or
	// commit subject reaches its overview (Lua 5.1.5's etc library had
	// been described by etc/README as the whole directory's extras). The
	// claims stay the report's, quoted as the authors' words.
	indexes, err := groupindex.ProjectAtlas(map[string]programindex.Index{server.Target.ID: server, client.Target.ID: client}, read.Atlas)
	if err != nil {
		t.Fatal(err)
	}
	asked := &capturedOrientation{flowTarget: server.Target.ID}
	if _, _, err := orientation.Run(t.Context(), llm.Executor{BatchConcurrency: 1, BatchController: &llm.BatchController{}}, asked,
		orientation.Input{RepositoryName: "kvd", Facts: layer, Claims: quoted, Groups: indexes, Graph: graph, Categorizer: categorizer}); err != nil {
		t.Fatal(err)
	}
	readme := 0
	for _, claim := range quoted.Claims {
		add(claim.Text)
		if claim.Source == claims.SourceReadme {
			readme++
		}
	}
	if readme == 0 {
		t.Fatal("kvd's claims hold no README line: the test no longer covers them")
	}
	preset.mu.Lock()
	bodies := append(slices.Clone(preset.requests), categorizer.Requests()...)
	preset.mu.Unlock()
	overview := asked.bodies(t)
	if len(bodies) == 0 || len(overview) != 1 {
		t.Fatalf("the reading sent %d bodies and the orientation %d", len(bodies), len(overview))
	}
	// The tables a README line had reached: directories, targets and an
	// outgoing boundary's owner context (kvcli's connect, its address asked).
	for _, needle := range []string{`atlas_directories`, `atlas_targets`, `source_context`} {
		if !slices.ContainsFunc(bodies, func(body []byte) bool { return strings.Contains(string(body), needle) }) {
			t.Fatalf("no body the reading sent carries %s: the test no longer covers it", needle)
		}
	}
	bodies = append(bodies, overview...)
	if strings.Contains(string(overview[0]), `"claims"`) || strings.Contains(string(overview[0]), `"author_doc"`) {
		t.Fatal("the orientation's overview carries claims or author docs")
	}
	for _, body := range bodies {
		text := string(body)
		for _, doc := range docs {
			quoted, _ := json.Marshal(doc)
			if needle := string(quoted[1 : len(quoted)-1]); strings.Contains(text, needle) {
				at := strings.Index(text, needle)
				t.Fatalf("a request carries the author's doc %q: …%s…", doc, text[max(0, at-300):min(len(text), at+len(needle)+100)])
			}
		}
	}
}
