package pythonprogramindex

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/places"
	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/pythontarget"
)

func cumulativePythonSources(t *testing.T, names ...string) map[string]string {
	t.Helper()
	files := make(map[string]string)
	for _, name := range append([]string{"pyproject.toml", "src/fixture_app/__init__.py"}, names...) {
		data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "repositories", "python", filepath.FromSlash(name)))
		if err != nil {
			t.Fatal(err)
		}
		files[name] = string(data)
	}
	return files
}

func TestCumulativePythonRuntimeRegistrationsKeepRecipientAndLaunchEvidence(t *testing.T) {
	repository := pythonCorpus(t, cumulativePythonSources(t, "src/fixture_app/runtime_registrations.py"))
	index, err := buildOneForTest(context.Background(), repository, targetOfKind(t, repository, pythontarget.KindLibrary))
	if err != nil {
		t.Fatal(err)
	}
	graph, err := places.Build(places.Input{Revision: strings.Repeat("a", 40), Repository: repository, Targets: []places.TargetInput{{Index: index}}})
	if err != nil {
		t.Fatal(err)
	}
	symbols := make(map[string]atlas.Place)
	for _, place := range graph.Places {
		if place.Symbol != nil {
			symbols[place.Symbol.Decl.Name] = place
		}
	}
	var launched, notLaunched, scheduled, lifespan, coroutine, walletSchedule, exchangeSchedule, scheduleTime bool
	for name, place := range symbols {
		for _, binding := range place.Symbol.Bindings {
			if binding.To != name {
				continue
			}
			if strings.HasSuffix(name, "receive_prices") {
				if !strings.Contains(binding.Detail, "threading.Thread") {
					t.Fatalf("thread recipient lost: %+v", binding)
				}
				for _, evidence := range binding.Evidence {
					if evidence.Extractor == "registration_result_use" && strings.Contains(evidence.Label, "start") {
						launched = true
					}
				}
			}
			if name == "bounded_retry" && strings.Contains(binding.Detail, "threading.Thread") {
				notLaunched = true
				for _, evidence := range binding.Evidence {
					if evidence.Extractor == "registration_result_use" {
						t.Fatalf("unstarted thread gained start: %+v", binding)
					}
				}
			}
			if name == "persist_wallet" && strings.Contains(binding.Detail, "every().day.at().do") {
				scheduled = true
				for _, evidence := range binding.Evidence {
					if evidence.Extractor == "registration_receiver_call" && evidence.Label == `at("00:07")` {
						scheduleTime = true
					}
				}
			}
			if strings.HasSuffix(name, "record_wallet_state") || strings.HasSuffix(name, "ws_connection_reset") {
				if strings.Contains(binding.From, "UnknownWalletOwner") {
					t.Fatalf("an unknown same-named field borrowed Wallets identity: %+v", binding)
				}
				if strings.Contains(binding.Detail, "every().day.at().do") {
					walletSchedule = walletSchedule || strings.HasSuffix(name, "record_wallet_state")
					exchangeSchedule = exchangeSchedule || strings.HasSuffix(name, "ws_connection_reset")
				}
			}
			if name == "lifespan" && strings.Contains(binding.Detail, "argument lifespan of fastapi.FastAPI") {
				lifespan = true
			}
		}
		if name == "refresh_candles" {
			for _, caller := range place.Symbol.CalledBy {
				if strings.HasPrefix(caller.Invocation, "coroutine_result_argument:asyncio.create_task") {
					coroutine = true
				}
			}
		}
	}
	if !launched || !notLaunched || !scheduled || !lifespan || !coroutine || !walletSchedule || !exchangeSchedule || !scheduleTime {
		t.Fatalf("observations: thread=%v unstarted=%v schedule=%v lifespan=%v coroutine=%v wallet=%v exchange=%v time=%v", launched, notLaunched, scheduled, lifespan, coroutine, walletSchedule, exchangeSchedule, scheduleTime)
	}
	// Assignment identity is the structural joint: the Thread constructor and
	// the subsequent start name the same observed field, not the name "thread".
	var resultID string
	for _, relation := range index.Relations {
		for _, pattern := range relation.Patterns {
			if pattern.Selector == "Thread" && pattern.ResultID != "" {
				for _, arg := range pattern.Arguments {
					if arg.Keyword == "target" {
						for _, id := range arg.ObjectIDs {
							if objectByID(t, index, id).Name == "receive_prices" {
								resultID = pattern.ResultID
							}
						}
					}
				}
			}
		}
	}
	if resultID == "" {
		t.Fatal("thread assignment lost native result identity")
	}
	for _, relation := range index.Relations {
		for _, pattern := range relation.Patterns {
			if pattern.Selector == "start" && pattern.ReceiverID == resultID {
				return
			}
		}
	}
	t.Fatal("start disconnected from its constructor result")
}

func TestCumulativePythonRoutesRetainRouterValuesAcrossDeclarations(t *testing.T) {
	repository := pythonCorpus(t, cumulativePythonSources(t, "src/fixture_app/route_mounts.py", "src/fixture_app/django_urls.py"))
	index, err := buildOneForTest(context.Background(), repository, targetOfKind(t, repository, pythontarget.KindLibrary))
	if err != nil {
		t.Fatal(err)
	}
	var routerID string
	for _, relation := range index.Relations {
		for _, pattern := range relation.Patterns {
			if pattern.Selector == "APIRouter" {
				for _, arg := range pattern.Arguments {
					if arg.Keyword == "prefix" && arg.Value == "/v1" {
						routerID = pattern.ResultID
					}
				}
			}
		}
	}
	if routerID == "" {
		t.Fatal("router construction lost its assigned identity")
	}
	var route, mounts int
	for _, relation := range index.Relations {
		for _, pattern := range relation.Patterns {
			if relation.Kind == programindex.RelationDecorates && pattern.ReceiverID == routerID {
				route++
			}
			if pattern.Selector == "include_router" {
				for _, arg := range pattern.Arguments {
					if arg.Position == 1 && len(arg.ObjectIDs) == 1 && arg.ObjectIDs[0] == routerID {
						mounts++
					}
				}
			}
		}
	}
	if route != 1 || mounts != 3 {
		t.Fatalf("router observations route=%d mounts=%d; local lookalike stays a raw observation", route, mounts)
	}
	result, err := facts.Build(facts.Input{Revision: strings.Repeat("a", 40), Repository: repository,
		TrackedPaths: repository.VisiblePaths(), Targets: []facts.TargetInput{{Index: index}}})
	if err != nil {
		t.Fatal(err)
	}
	wanted := map[string]bool{"/api/v1/ping": false, "/alternate/v1/ping": false, "/private/ping": false, "/flask/health": false, "/django/health": false}
	for _, route := range result.OfKind(facts.KindHTTPRoute) {
		if _, ok := wanted[route.Path]; !ok {
			t.Fatalf("unexpected route or lost prefix: %+v", route)
		}
		if wanted[route.Path] {
			t.Fatalf("duplicate route: %+v", route)
		}
		if route.ObjectID == "" || route.Anchor == nil || len(route.Evidence) == 0 {
			t.Fatalf("route lost handler or mount anchors: %+v", route)
		}
		wanted[route.Path] = true
	}
	for path, found := range wanted {
		if !found {
			t.Errorf("missing mounted route %s", path)
		}
	}
}
