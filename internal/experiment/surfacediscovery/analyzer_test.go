package surfacediscovery

import (
	"bytes"
	"path/filepath"
	"reflect"
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
		if !hasLoopSignal(result.Coverage.LoopSignals, "registration_loop") {
			t.Fatalf("loop signals = %#v", result.Coverage.LoopSignals)
		}
	})
	t.Run("negative controls", func(t *testing.T) {
		result := analyzeFixture(t, "negative")
		if len(result.Catalog.Triggers) != 0 {
			t.Fatalf("triggers = %#v, want none", result.Catalog.Triggers)
		}
	})
}

func TestAnalyzeWorkerRequiresTerminalStartAndLoopEvidence(t *testing.T) {
	result := analyzeFixture(t, "workers")
	if len(result.Catalog.Triggers) != 3 {
		t.Fatalf("trigger count = %d, want worker and two finite async tasks", len(result.Catalog.Triggers))
	}
	var worker, finite, scannerTask *TriggerRecord
	for index := range result.Catalog.Triggers {
		trigger := &result.Catalog.Triggers[index]
		switch {
		case trigger.Kind == "worker":
			worker = trigger
		case trigger.Kind == "async_task" && strings.Contains(trigger.Handler.Text, "oneShot"):
			finite = trigger
		case trigger.Kind == "async_task" && strings.Contains(trigger.Handler.Text, "registerTasks$"):
			scannerTask = trigger
		}
	}
	if worker == nil || worker.Status != "confirmed_worker_registration" ||
		!strings.Contains(worker.Handler.Text, "runWorker") {
		t.Fatalf("worker = %#v", worker)
	}
	if finite == nil || finite.Status != "confirmed_async_task_start" ||
		!strings.Contains(finite.Handler.Text, "oneShot") {
		t.Fatalf("finite task = %#v", finite)
	}
	if scannerTask == nil || scannerTask.Status != "confirmed_async_task_start" {
		t.Fatalf("captured method task = %#v; triggers = %#v", scannerTask, result.Catalog.Triggers)
	}
	if result.Coverage.Workers != 1 || result.Coverage.AsyncTasks != 2 ||
		!hasLoopSignal(result.Coverage.LoopSignals, "channel_receive_loop") ||
		!hasLoopSignal(result.Coverage.LoopSignals, "control_flow_loop") {
		t.Fatalf("coverage = %#v", result.Coverage)
	}
	seenEvidence := map[string]struct{}{}
	for _, evidence := range worker.Evidence {
		if _, duplicate := seenEvidence[evidence.ID]; duplicate {
			t.Fatalf("worker evidence repeats %q: %#v", evidence.ID, worker.Evidence)
		}
		seenEvidence[evidence.ID] = struct{}{}
	}
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

func TestAnalyzeGroundsModularServerFromEntrypointComposition(t *testing.T) {
	result := analyzeFixture(t, "modular")
	if result.Grounding.RepositoryArchetype.Selected != "modular_platform_server" {
		t.Fatalf("archetype = %#v", result.Grounding.RepositoryArchetype)
	}
	if result.Grounding.GroundingMode != "behavior_grounded" {
		t.Fatalf("grounding mode = %q", result.Grounding.GroundingMode)
	}
	if result.Coverage.FunctionsInspected <= 1 {
		t.Fatalf("functions inspected = %d, want composition traversal beyond main", result.Coverage.FunctionsInspected)
	}
	kinds := make(map[string]bool)
	for _, anchor := range result.Grounding.Anchors {
		kinds[anchor.Kind] = true
		if anchor.Location.Path == "" || anchor.Location.Line == 0 || len(anchor.AssociatedMembers) == 0 || len(anchor.Limitations) == 0 {
			t.Fatalf("incomplete anchor = %#v", anchor)
		}
	}
	for _, kind := range []string{
		"process_entry", "command_dispatch", "config_ingress", "registry_write",
		"registry_lookup", "lifecycle_start", "admin_control_plane",
		"request_dispatch_root", "tls_or_security_boundary", "extension_family",
	} {
		if !kinds[kind] {
			t.Errorf("missing behavior anchor kind %q: %#v", kind, result.Grounding.Anchors)
		}
	}
	if len(result.Grounding.Relationships) == 0 {
		t.Fatal("entrypoint composition produced no exact behavior handoffs")
	}
	registrationRelations := 0
	for _, relationship := range result.Grounding.Relationships {
		if relationship.Kind != "registers_extension_family" {
			continue
		}
		registrationRelations++
		if relationship.WitnessCount != 2 || len(relationship.WitnessIDs) != 2 {
			t.Fatalf("aggregated registration = %#v, want two exact witnesses", relationship)
		}
	}
	if registrationRelations != 1 {
		t.Fatalf("registration relationships = %d, want one aggregate", registrationRelations)
	}
	for _, anchor := range result.Grounding.Anchors {
		if anchor.Kind != "config_apply" {
			continue
		}
		for _, member := range anchor.AssociatedMembers {
			if strings.Contains(member.Location.Path, "/headers/") || member.Name == "ApplyToRequest" {
				t.Fatalf("request mutator classified as config_apply: %#v", member)
			}
		}
	}
}

func TestDeduplicateArchitectureSymbolsNormalizesReceiverWrappers(t *testing.T) {
	t.Parallel()

	location := Location{Path: "context.go", Line: 188, Column: 20}
	symbols := []Symbol{
		{ID: "example.(*Context).LoadModule", Package: "example", Name: "LoadModule", Location: location},
		{ID: "example.(Context).LoadModule", Package: "example", Name: "LoadModule", Location: location},
		{ID: "example.(*Context).LoadModule", Package: "example", Name: "LoadModule", Location: location},
	}
	first := deduplicateArchitectureSymbols(symbols)
	second := deduplicateArchitectureSymbols([]Symbol{symbols[2], symbols[1], symbols[0]})
	if len(first) != 1 || !reflect.DeepEqual(first, second) {
		t.Fatalf("deduplicated symbols are unstable: first=%#v second=%#v", first, second)
	}
	if first[0].ID != "example.(*Context).LoadModule" || !reflect.DeepEqual(
		first[0].EquivalentIDs,
		[]string{"example.(*Context).LoadModule", "example.(Context).LoadModule"},
	) {
		t.Fatalf("canonical declaration = %#v", first[0])
	}
}

func TestConfigurationPackageClassificationDoesNotUseReceiverName(t *testing.T) {
	t.Parallel()

	if !isConfigurationPackage("example/internal/config") || !isConfigurationPackage("example/caddyconfig/httpcaddyfile") {
		t.Fatal("configuration packages were not recognized")
	}
	if isConfigurationPackage("example/modules/headers") || isConfigurationPackage("example/modules/request") {
		t.Fatal("request packages were classified from unrelated Apply-style receiver names")
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

func hasLoopSignal(signals []LoopSignal, kind string) bool {
	for _, signal := range signals {
		if signal.Kind == kind {
			return true
		}
	}
	return false
}
