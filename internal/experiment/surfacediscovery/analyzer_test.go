package surfacediscovery

import (
	"bytes"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

var fixtureResults = struct {
	sync.Mutex
	values map[string]Result
}{values: map[string]Result{}}

func TestAnalyzeDirectRoute(t *testing.T) {
	result := analyzeFixture(t, "direct")
	if len(result.Catalog.Triggers) != 1 {
		t.Fatalf("trigger count = %d, want 1", len(result.Catalog.Triggers))
	}
	trigger := result.Catalog.Triggers[0]
	if trigger.Identity.Path.Text != "/health" || trigger.Handler.Text != "example.com/direct.healthHandler" {
		t.Fatalf("trigger = %#v", trigger)
	}
	if trigger.FinalSeed != "net-http-servemux-handlefunc" || trigger.DiscoveryBasis != "catalog_static" {
		t.Fatalf("seed/basis = %q/%q", trigger.FinalSeed, trigger.DiscoveryBasis)
	}
	if trigger.ServerStartSite == nil {
		t.Fatal("server start site is missing")
	}
}

func TestAnalyzeDispatcherOwnership(t *testing.T) {
	tests := []struct {
		name       string
		fixture    string
		dispatcher string
	}{
		{name: "package default mux", fixture: "default_mux", dispatcher: "net/http.DefaultServeMux"},
		{name: "server handler field", fixture: "server_field", dispatcher: "net/http.NewServeMux@main.go:8:25"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := analyzeFixture(t, test.fixture)
			if len(result.Catalog.Triggers) != 1 {
				t.Fatalf("trigger count = %d, want 1", len(result.Catalog.Triggers))
			}
			trigger := result.Catalog.Triggers[0]
			if trigger.Dispatcher.Text != test.dispatcher || trigger.ServerStartSite == nil {
				t.Fatalf("dispatcher/start = %#v/%#v", trigger.Dispatcher, trigger.ServerStartSite)
			}
		})
	}
}

func TestAnalyzeRepositoryWrappersAndValues(t *testing.T) {
	result := analyzeFixture(t, "wrappers")
	if len(result.Catalog.Triggers) != 1 {
		t.Fatalf("trigger count = %d, want 1", len(result.Catalog.Triggers))
	}
	trigger := result.Catalog.Triggers[0]
	if trigger.Identity.Path.Text != "/v1/runs" {
		t.Fatalf("path = %#v, want /v1/runs", trigger.Identity.Path)
	}
	if len(trigger.WrapperChain) != 3 {
		t.Fatalf("wrapper chain = %#v, want registerAPI/registerAdmin/addRoute", trigger.WrapperChain)
	}
	joined := wrapperIDs(trigger.WrapperChain)
	for _, name := range []string{"registerAPI", "registerAdmin", "addRoute"} {
		if !strings.Contains(joined, name) {
			t.Fatalf("wrapper chain %q does not contain %q", joined, name)
		}
	}
	if len(trigger.Middleware) != 1 || !strings.Contains(trigger.Middleware[0].Text, ".auth") {
		t.Fatalf("middleware = %#v", trigger.Middleware)
	}
	if !strings.Contains(trigger.Handler.Text, "CreateRun") {
		t.Fatalf("handler = %#v", trigger.Handler)
	}
}

func TestAnalyzeCrossPackageWrapper(t *testing.T) {
	result := analyzeFixture(t, "cross")
	if len(result.Catalog.Triggers) != 1 {
		t.Fatalf("trigger count = %d, want 1", len(result.Catalog.Triggers))
	}
	trigger := result.Catalog.Triggers[0]
	if trigger.Identity.Path.Text != "/cross" || !strings.Contains(wrapperIDs(trigger.WrapperChain), "example.com/cross/routes.Add") {
		t.Fatalf("trigger = %#v", trigger)
	}
	if len(result.Summaries) == 0 || len(result.Summaries[0].SourceDependency) == 0 {
		t.Fatalf("summaries = %#v", result.Summaries)
	}
}

func TestAnalyzeInterfaceTargets(t *testing.T) {
	t.Run("single implementation", func(t *testing.T) {
		result := analyzeFixture(t, "interface_single")
		if len(result.Catalog.Triggers) != 1 || result.Catalog.Triggers[0].Resolution != "exact" {
			t.Fatalf("triggers = %#v", result.Catalog.Triggers)
		}
	})
	t.Run("multiple implementations", func(t *testing.T) {
		result := analyzeFixture(t, "interface_multiple")
		if len(result.Catalog.Triggers) < 2 {
			t.Fatalf("trigger count = %d, want at least 2", len(result.Catalog.Triggers))
		}
		for _, trigger := range result.Catalog.Triggers {
			if trigger.Resolution == "exact" {
				t.Fatalf("ambiguous interface trigger was marked exact: %#v", trigger)
			}
		}
	})
}

func TestAnalyzeDynamicAndNegativeControls(t *testing.T) {
	t.Run("dynamic registration", func(t *testing.T) {
		result := analyzeFixture(t, "dynamic")
		if len(result.Catalog.Triggers) != 1 {
			t.Fatalf("trigger count = %d, want 1", len(result.Catalog.Triggers))
		}
		trigger := result.Catalog.Triggers[0]
		if trigger.Status != "dynamic_unknown" || len(trigger.DynamicFrontier) < 2 {
			t.Fatalf("dynamic trigger = %#v", trigger)
		}
	})
	t.Run("negative controls", func(t *testing.T) {
		result := analyzeFixture(t, "negative")
		if len(result.Catalog.Triggers) != 0 {
			t.Fatalf("triggers = %#v, want none", result.Catalog.Triggers)
		}
	})
}

func TestAnalyzeRecursiveWrapperStops(t *testing.T) {
	result := analyzeFixture(t, "recursive")
	if len(result.Catalog.Triggers) == 0 {
		t.Fatal("partial route result was lost")
	}
	if !hasFrontier(result.Coverage.DynamicFrontiers, "recursive_wrapper") {
		t.Fatalf("frontiers = %#v", result.Coverage.DynamicFrontiers)
	}
}

func TestAnalyzeGinConvenienceAndRepositoryWrapper(t *testing.T) {
	result := analyzeFixture(t, "gin")
	if len(result.Catalog.Triggers) != 1 {
		t.Fatalf("trigger count = %d, want 1", len(result.Catalog.Triggers))
	}
	trigger := result.Catalog.Triggers[0]
	if trigger.Framework != "gin" || trigger.Identity.Method != "GET" || trigger.Identity.Path.Text != "/runs" {
		t.Fatalf("trigger = %#v", trigger)
	}
	joined := wrapperIDs(trigger.WrapperChain)
	if !strings.Contains(joined, "registerJSON") || !strings.Contains(joined, "RouterGroup).GET") {
		t.Fatalf("wrapper chain = %q", joined)
	}
}

func TestAnalyzeCustomRouterStretchRemainsUnsupported(t *testing.T) {
	result := analyzeFixture(t, "custom_router")
	if len(result.Catalog.Triggers) != 0 {
		t.Fatalf("custom registry was promoted without a configured terminal seed: %#v", result.Catalog.Triggers)
	}
}

func TestAnalyzeIsDeterministic(t *testing.T) {
	first := analyzeFixture(t, "wrappers")
	second := analyzeFixture(t, "wrappers")
	first.Coverage.ColdLatencyMillis = 0
	second.Coverage.ColdLatencyMillis = 0
	firstJSON, err := MarshalDeterministic(first)
	if err != nil {
		t.Fatal(err)
	}
	secondJSON, err := MarshalDeterministic(second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstJSON, secondJSON) {
		t.Fatalf("repeated results differ\nfirst:\n%s\nsecond:\n%s", firstJSON, secondJSON)
	}
}

func analyzeFixture(t *testing.T, name string) Result {
	t.Helper()
	fixtureResults.Lock()
	defer fixtureResults.Unlock()
	if result, ok := fixtureResults.values[name]; ok {
		return result
	}
	result, err := Analyze(DefaultOptions(filepath.Join("testdata", name)))
	if err != nil {
		t.Fatal(err)
	}
	fixtureResults.values[name] = result
	return result
}

func wrapperIDs(wrappers []Wrapper) string {
	ids := make([]string, 0, len(wrappers))
	for _, wrapper := range wrappers {
		ids = append(ids, wrapper.Symbol.ID)
	}
	return strings.Join(ids, " -> ")
}

func hasFrontier(frontiers []Frontier, kind string) bool {
	for _, frontier := range frontiers {
		if frontier.Kind == kind {
			return true
		}
	}
	return false
}
