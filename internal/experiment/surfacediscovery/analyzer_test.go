package surfacediscovery

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestAnalyzeContextHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := AnalyzeContext(ctx, DefaultOptions(filepath.Join("testdata", "cli")))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("AnalyzeContext() error = %v, want context.Canceled", err)
	}
}

func TestAnalyzeRecordsNamedDiscoveryPhases(t *testing.T) {
	result := analyzeFixture(t, "direct")
	var names []string
	for _, phase := range result.Coverage.Phases {
		names = append(names, phase.Phase)
		if phase.Detail == "" {
			t.Fatalf("phase %q has no human-readable detail", phase.Phase)
		}
	}
	want := []string{
		"package_load", "ssa_build", "call_graph", "candidate_index",
		"architecture_anchors", "entrypoint_walk", "detached_walk",
		"catalog_finalize", "grounding_finalize",
	}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("phase names = %v, want %v", names, want)
	}
}

func TestProcessEntryArchitectureAnchorDeduplicatesSyntaxAndSSAColumns(t *testing.T) {
	symbol := Symbol{ID: "example.com/project/cmd/project.main", Package: "example.com/project/cmd/project", Name: "main"}
	a := analyzer{architectureAnchors: make(map[string]BehaviorAnchor)}

	syntaxID := a.recordArchitectureAnchorMembersWithProvenance(
		"process_entry",
		"process entry "+symbol.ID,
		Location{Path: "cmd/project/main.go", Line: 12},
		[]Symbol{symbol},
		"Exact build-selected main declaration.",
		Provenance{Provider: "gofacts", Version: "entrypoint-anchor-v1", Operation: "classify_exact_process_entry"},
	)
	ssaID := a.recordArchitectureAnchor(
		"process_entry",
		"process entry "+symbol.ID,
		Location{Path: "cmd/project/main.go", Line: 12, Column: 6},
		symbol,
		"Exact build-selected main declaration; process execution is not observed.",
	)

	if syntaxID != ssaID || len(a.architectureAnchors) != 1 {
		t.Fatalf("process anchors = %d, syntax ID = %q, SSA ID = %q", len(a.architectureAnchors), syntaxID, ssaID)
	}
	if got := a.architectureAnchors[syntaxID].Producer.Provider; got != "gofacts" {
		t.Fatalf("deduplicated process anchor provider = %q, want gofacts", got)
	}
}

func TestClassifyTerminalOwnershipKeepsDetachedRepositorySurfaceApplicationOwned(t *testing.T) {
	t.Parallel()

	scope, classification, basis := classifyTerminalOwnership(
		Location{Path: "internal/admin/routes.go", Line: 42}, nil, true,
	)
	if scope != "repository" || classification != ApplicationSurface || basis != PromotionRepositoryRegistration {
		t.Fatalf("detached repository surface = %q/%q/%q, want repository application registration", scope, classification, basis)
	}
}

func TestProcessEntryArchitectureAnchorDeduplicatesAtCollectionLimit(t *testing.T) {
	symbol := Symbol{ID: "example.com/project/cmd/project.main", Package: "example.com/project/cmd/project", Name: "main"}
	a := analyzer{architectureAnchors: make(map[string]BehaviorAnchor)}
	id := a.recordArchitectureAnchorMembersWithProvenance(
		"process_entry", "process entry "+symbol.ID,
		Location{Path: "cmd/project/main.go", Line: 12}, []Symbol{symbol}, "exact declaration",
		Provenance{Provider: "gofacts", Version: "entrypoint-anchor-v1", Operation: "classify_exact_process_entry"},
	)
	for index := len(a.architectureAnchors); index < maxCollectedArchitectureAnchors; index++ {
		dummyID := fmt.Sprintf("dummy-%04d", index)
		a.architectureAnchors[dummyID] = BehaviorAnchor{ID: dummyID}
	}

	got := a.recordArchitectureAnchor(
		"process_entry", "process entry "+symbol.ID,
		Location{Path: "cmd/project/main.go", Line: 12, Column: 6}, symbol, "typed declaration",
	)
	if got != id {
		t.Fatalf("deduplicated process anchor ID at collection limit = %q, want %q", got, id)
	}
}

func TestAnalyzeContextReturnsPromptlyWhenCanceledDuringAnalysis(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(10*time.Millisecond, cancel)
	done := make(chan error, 1)
	go func() {
		_, err := AnalyzeContext(ctx, DefaultOptions(filepath.Join("testdata", "caddy_patterns")))
		done <- err
	}()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("AnalyzeContext() error = %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("AnalyzeContext did not return promptly after cancellation")
	}
}

var fixtureResults = struct {
	sync.Mutex
	values map[string]Result
}{values: map[string]Result{}}

func TestSurfaceSemanticsSeparateDynamicRouteAndLocalServerWrapper(t *testing.T) {
	t.Parallel()

	dynamicRoute := TriggerRecord{
		Kind: "http_route", Availability: AvailabilityAvailable,
		RegistrationSite: Location{Path: "internal/routes.go", Line: 20},
		Identity:         Identity{Path: Value{Kind: "unknown", Text: "runtime route"}},
	}
	deriveSurfaceSemantics(&dynamicRoute)
	if dynamicRoute.SurfaceRole != SurfaceRoleDynamicFrontier ||
		dynamicRoute.TraceReadiness != TraceReadinessUnsupported {
		t.Fatalf("dynamic route semantics = %#v", dynamicRoute)
	}

	wrappedServer := TriggerRecord{
		Kind: "http_server", Availability: AvailabilityAvailable,
		ServerStartSite: &Location{Path: "/outside/stdlib/server.go", Line: 100},
		WrapperChain:    []Wrapper{{Callsite: Location{Path: "internal/server.go", Line: 44}}},
	}
	deriveSurfaceSemantics(&wrappedServer)
	if wrappedServer.SurfaceRole != SurfaceRoleEntrySurface ||
		wrappedServer.TraceReadiness != TraceReadinessPartial ||
		wrappedServer.Quality.RegistrationStart != SurfaceQualityPartial {
		t.Fatalf("wrapped server semantics = %#v", wrappedServer)
	}
}

func TestBoundArchitectureGroundingCapsKindsAndDropsDanglingRelationships(t *testing.T) {
	t.Parallel()

	anchors := make([]BehaviorAnchor, 0, maxArchitectureAnchorsPerKind+2)
	for index := 0; index < maxArchitectureAnchorsPerKind+2; index++ {
		anchors = append(anchors, BehaviorAnchor{
			ID: fmt.Sprintf("anchor-%02d", index), Kind: "command_dispatch",
		})
	}
	relationships := []BehaviorRelationship{
		{ID: "kept", From: "anchor-00", To: "anchor-01"},
		{ID: "dropped", From: "anchor-00", To: "anchor-17"},
	}

	gotAnchors, gotRelationships, bounded := boundArchitectureGrounding(anchors, relationships)
	if !bounded || len(gotAnchors) != maxArchitectureAnchorsPerKind ||
		len(gotRelationships) != 1 || gotRelationships[0].ID != "kept" {
		t.Fatalf("bounded grounding = %d anchors, %#v relationships, bounded=%t", len(gotAnchors), gotRelationships, bounded)
	}
}

func TestBoundArchitectureGroundingPrioritizesProcessReachableAnchors(t *testing.T) {
	t.Parallel()

	anchors := []BehaviorAnchor{{ID: "process", Kind: "process_entry"}}
	for index := 0; index < maxArchitectureAnchorsPerKind+1; index++ {
		anchors = append(anchors, BehaviorAnchor{
			ID: fmt.Sprintf("dispatch-%02d", index), Kind: "command_dispatch",
			Location: Location{Path: fmt.Sprintf("cmd/app/%02d.go", index), Line: 1},
		})
	}
	relationships := []BehaviorRelationship{{ID: "connected", From: "process", To: "dispatch-16"}}

	gotAnchors, gotRelationships, bounded := boundArchitectureGrounding(anchors, relationships)
	retained := make(map[string]bool, len(gotAnchors))
	for _, anchor := range gotAnchors {
		retained[anchor.ID] = true
	}
	if !bounded || !retained["dispatch-16"] || len(gotRelationships) != 1 || gotRelationships[0].ID != "connected" {
		t.Fatalf("reachable anchor was not retained: anchors=%#v relationships=%#v bounded=%t", gotAnchors, gotRelationships, bounded)
	}
}

func TestBoundArchitectureGroundingCapsRelationships(t *testing.T) {
	t.Parallel()

	anchors := []BehaviorAnchor{
		{ID: "process", Kind: "process_entry"},
		{ID: "dispatch", Kind: "command_dispatch"},
	}
	relationships := make([]BehaviorRelationship, 0, maxPersistedArchitectureRelationships+1)
	for index := 0; index <= maxPersistedArchitectureRelationships; index++ {
		relationships = append(relationships, BehaviorRelationship{
			ID: fmt.Sprintf("relationship-%04d", index), From: "process", To: "dispatch",
		})
	}

	_, gotRelationships, bounded := boundArchitectureGrounding(anchors, relationships)
	if !bounded || len(gotRelationships) != maxPersistedArchitectureRelationships {
		t.Fatalf("relationships = %d, bounded=%t", len(gotRelationships), bounded)
	}
}

func TestArchitectureRelationshipCapsUniqueWitnesses(t *testing.T) {
	a := analyzer{
		architectureAnchors: map[string]BehaviorAnchor{
			"process":  {ID: "process", Kind: "process_entry"},
			"dispatch": {ID: "dispatch", Kind: "command_dispatch"},
		},
		architectureRelationships: make(map[string]BehaviorRelationship),
	}
	for index := 0; index < maxArchitectureRelationshipWitnesses+5; index++ {
		a.recordArchitectureRelationship(
			"process", "dispatch", Location{Path: "cmd/app/main.go", Line: index + 1},
		)
	}
	if len(a.architectureRelationships) != 1 {
		t.Fatalf("relationships = %d, want 1", len(a.architectureRelationships))
	}
	for _, relationship := range a.architectureRelationships {
		if relationship.WitnessCount != maxArchitectureRelationshipWitnesses ||
			len(relationship.WitnessIDs) != maxArchitectureRelationshipWitnesses {
			t.Fatalf("bounded witnesses = %#v", relationship)
		}
	}
}

func TestDefaultOptionsAndImplicitOptionsUseSameBudgets(t *testing.T) {
	defaults := DefaultOptions(".")
	implicit := normalizeOptions(Options{RepoPath: "."})
	if !reflect.DeepEqual(defaults, implicit) {
		t.Fatalf("default options = %#v, implicit options = %#v", defaults, implicit)
	}
}

func TestAnalyzeDirectRoute(t *testing.T) {
	result := analyzeFixture(t, "direct")
	trigger := onlyTriggerOfKind(t, result, "http_route")
	if trigger.Identity.Path.Text != "/health" || trigger.Handler.Text != "example.com/direct.healthHandler" {
		t.Fatalf("trigger = %#v", trigger)
	}
	if trigger.FinalSeed != "net-http-servemux-handlefunc" || trigger.DiscoveryBasis != "catalog_static" {
		t.Fatalf("seed/basis = %q/%q", trigger.FinalSeed, trigger.DiscoveryBasis)
	}
	if trigger.ServerStartSite == nil {
		t.Fatal("server start site is missing")
	}
	if trigger.SurfaceRole != SurfaceRoleEntrySurface || trigger.TraceReadiness != TraceReadinessReady ||
		trigger.Quality.Identity != SurfaceQualityExact || trigger.Quality.RegistrationStart != SurfaceQualityExact ||
		trigger.Quality.HandlerCallback != SurfaceQualityExact || trigger.Quality.Reachability != SurfaceQualityStatic ||
		trigger.Quality.Traceability != TraceReadinessReady || trigger.TraceReadinessReason == "" {
		t.Fatalf("route quality/readiness = %#v", trigger)
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
			trigger := onlyTriggerOfKind(t, result, "http_route")
			if trigger.Dispatcher.Text != test.dispatcher || trigger.ServerStartSite == nil {
				t.Fatalf("dispatcher/start = %#v/%#v", trigger.Dispatcher, trigger.ServerStartSite)
			}
		})
	}
}

func TestAnalyzeRepositoryWrappersAndValues(t *testing.T) {
	result := analyzeFixture(t, "wrappers")
	trigger := onlyTriggerOfKind(t, result, "http_route")
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

func TestAnalyzeBindsCapturedClosureFreeVar(t *testing.T) {
	result := analyzeFixture(t, "captured_closure")
	routes := map[string]TriggerRecord{}
	for _, trigger := range triggersOfKind(result, "http_route") {
		if trigger.Identity.Path.Known {
			routes[trigger.Identity.Path.Text] = trigger
		}
	}
	for _, path := range []string{"/nested", "/wrapped"} {
		trigger, ok := routes[path]
		if !ok || !strings.Contains(wrapperIDs(trigger.WrapperChain), "main$") {
			t.Fatalf("captured closure route %q = %#v", path, trigger)
		}
	}
}

func TestAnalyzeCrossPackageWrapper(t *testing.T) {
	result := analyzeFixture(t, "cross")
	trigger := onlyTriggerOfKind(t, result, "http_route")
	if trigger.Identity.Path.Text != "/cross" || !strings.Contains(wrapperIDs(trigger.WrapperChain), "example.com/cross/routes.Add") {
		t.Fatalf("trigger = %#v", trigger)
	}
	if len(result.Summaries) == 0 || len(result.Summaries[0].SourceDependency) == 0 {
		t.Fatalf("summaries = %#v", result.Summaries)
	}
}

func TestAnalyzeDoesNotCrossExecutableRoots(t *testing.T) {
	result := analyzeFixture(t, "separate_mains")

	for _, trigger := range triggersOfKind(result, "http_route") {
		if trigger.ProcessEntrypoint.Package == "example.com/separate-mains/cmd/app" &&
			(trigger.Identity.Path.Text == "/helper" || strings.Contains(wrapperIDs(trigger.WrapperChain), "helper")) {
			t.Fatalf("primary executable inherited helper behavior: %#v", trigger)
		}
	}
	appRoutes := []TriggerRecord{}
	for _, trigger := range triggersOfKind(result, "http_route") {
		if trigger.ProcessEntrypoint.Package == "example.com/separate-mains/cmd/app" {
			appRoutes = append(appRoutes, trigger)
		}
	}
	if len(appRoutes) != 1 || appRoutes[0].Identity.Path.Text != "/primary" {
		t.Fatalf("application routes = %#v, want only primary route", appRoutes)
	}
	if !hasFrontier(result.Coverage.UnsupportedDispatch, "call_target_unresolved") {
		t.Fatalf("cross-executable callback was silently discarded: %#v", result.Coverage.UnsupportedDispatch)
	}
}

func TestAnalyzeInterfaceTargets(t *testing.T) {
	t.Run("single implementation", func(t *testing.T) {
		result := analyzeFixture(t, "interface_single")
		if onlyTriggerOfKind(t, result, "http_route").Resolution != "exact" {
			t.Fatalf("triggers = %#v", result.Catalog.Triggers)
		}
	})
	t.Run("multiple implementations", func(t *testing.T) {
		result := analyzeFixture(t, "interface_multiple")
		if len(result.Catalog.Triggers) < 2 {
			t.Fatalf("trigger count = %d, want at least 2", len(result.Catalog.Triggers))
		}
		for _, trigger := range triggersOfKind(result, "http_route") {
			if trigger.TraceReadiness == TraceReadinessReady || !hasFrontier(trigger.DynamicFrontier, "entrypoint_dispatch_unresolved") {
				t.Fatalf("ambiguous interface trigger was promoted past its dispatch frontier: %#v", trigger)
			}
		}
	})
}

func TestAnalyzeDynamicAndNegativeControls(t *testing.T) {
	t.Run("dynamic registration", func(t *testing.T) {
		result := analyzeFixture(t, "dynamic")
		trigger := onlyTriggerOfKind(t, result, "http_route")
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
	if worker.SurfaceRole != SurfaceRoleRuntimeActivity || worker.TraceReadiness != TraceReadinessUnsupported ||
		!strings.Contains(worker.TraceReadinessReason, "cannot independently") {
		t.Fatalf("worker trace readiness = %#v", worker)
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

func TestAnalyzeEchoDirectConvenienceAndGroupedRoutes(t *testing.T) {
	result := analyzeFixture(t, "echo")
	if len(result.Catalog.Triggers) != 4 {
		t.Fatalf("trigger count = %d, want 4: %#v", len(result.Catalog.Triggers), result.Catalog.Triggers)
	}
	want := map[string]string{
		"DELETE /direct":   "echo-v4-echo-add",
		"GET /health":      "echo-v4-echo-add",
		"POST /api/runs":   "echo-v4-group-add",
		"POST /admin/runs": "echo-v4-group-add",
	}
	for _, trigger := range result.Catalog.Triggers {
		key := trigger.Identity.Method + " " + trigger.Identity.Path.Text
		seed, ok := want[key]
		if !ok {
			t.Errorf("unexpected Echo trigger %q: %#v", key, trigger)
			continue
		}
		if trigger.Framework != "echo" || trigger.FinalSeed != seed || !trigger.Handler.Known {
			t.Errorf("Echo trigger %q = %#v", key, trigger)
		}
		delete(want, key)
	}
	if len(want) != 0 {
		t.Fatalf("missing Echo triggers: %v", want)
	}
}

func TestAnalyzeExternalFrameworkConvenienceRoutesKeepRepositoryEvidence(t *testing.T) {
	tests := []struct {
		name          string
		modulePath    string
		version       string
		application   string
		frameworkCode string
	}{
		{
			name: "Echo", modulePath: "github.com/labstack/echo/v4", version: "v4.15.4",
			application: `package main
import "github.com/labstack/echo/v4"
func handler(echo.Context) error { return nil }
func main() { e := &echo.Echo{}; e.GET("/health", handler) }
`,
			frameworkCode: `package echo
import "net/http"
type Context interface{}
type HandlerFunc func(Context) error
type MiddlewareFunc func(HandlerFunc) HandlerFunc
type Route struct{}
type Echo struct{}
func (e *Echo) Add(method, path string, handler HandlerFunc, middleware ...MiddlewareFunc) *Route { return &Route{} }
func (e *Echo) GET(path string, handler HandlerFunc, middleware ...MiddlewareFunc) *Route { return e.Add(http.MethodGet, path, handler, middleware...) }
`,
		},
		{
			name: "Gin", modulePath: "github.com/gin-gonic/gin", version: "v1.12.0",
			application: `package main
import "github.com/gin-gonic/gin"
func handler(*gin.Context) {}
func main() { r := &gin.RouterGroup{}; r.GET("/health", handler) }
`,
			frameworkCode: `package gin
import "net/http"
type Context struct{}
type HandlerFunc func(*Context)
type RouterGroup struct{}
func (g *RouterGroup) Handle(method, path string, handlers ...HandlerFunc) { _ = method; _ = path; _ = handlers }
func (g *RouterGroup) GET(path string, handlers ...HandlerFunc) { g.Handle(http.MethodGet, path, handlers...) }
`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			first := analyzeSiblingFrameworkFixture(t, test.modulePath, test.version, test.application, test.frameworkCode)
			second := analyzeSiblingFrameworkFixture(t, test.modulePath, test.version, test.application, test.frameworkCode)
			firstRoute := onlyTriggerOfKind(t, first, "http_route")
			secondRoute := onlyTriggerOfKind(t, second, "http_route")
			if firstRoute.Identity.Path.Text != "/health" || firstRoute.Identity.Method != "GET" ||
				!filepath.IsAbs(firstRoute.RegistrationSite.Path) || firstRoute.ID != secondRoute.ID {
				t.Fatalf("external %s routes = %#v / %#v", test.name, firstRoute, secondRoute)
			}
			if len(firstRoute.WrapperChain) == 0 || firstRoute.WrapperChain[0].Callsite.Path != "main.go" {
				t.Fatalf("external %s repository evidence = %#v", test.name, firstRoute.WrapperChain)
			}
		})
	}
}

func analyzeSiblingFrameworkFixture(
	t *testing.T,
	modulePath, version, application, frameworkCode string,
) Result {
	t.Helper()
	root := t.TempDir()
	appDir := filepath.Join(root, "app")
	frameworkDir := filepath.Join(root, "framework")
	for _, directory := range []string{appDir, frameworkDir} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeFixtureFile(t, filepath.Join(appDir, "go.mod"), "module example.com/externalapp\n\ngo 1.25\n\nrequire "+modulePath+" "+version+"\n\nreplace "+modulePath+" => ../framework\n")
	writeFixtureFile(t, filepath.Join(appDir, "main.go"), application)
	writeFixtureFile(t, filepath.Join(frameworkDir, "go.mod"), "module "+modulePath+"\n\ngo 1.25\n")
	writeFixtureFile(t, filepath.Join(frameworkDir, "framework.go"), frameworkCode)
	result, err := Analyze(DefaultOptions(appDir))
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func writeFixtureFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestAnalyzeCustomRouterRoutesRemainUnsupportedButServerRootIsRetained(t *testing.T) {
	result := analyzeFixture(t, "custom_router")
	if len(result.Catalog.Triggers) != 1 || result.Catalog.Triggers[0].Kind != "http_server" ||
		!hasFrontier(result.Catalog.Triggers[0].DynamicFrontier, "unresolved_dispatch_inventory") {
		t.Fatalf("custom routes were promoted or the exact server root was lost: %#v", result.Catalog.Triggers)
	}
}

func TestAnalyzeImportReachableDetachedHTTPComposition(t *testing.T) {
	result := analyzeFixture(t, "caddy_patterns")
	var staticRoutes, dynamicRoutes, servers int
	for _, trigger := range result.Catalog.Triggers {
		if !hasFrontier(trigger.DynamicFrontier, "entrypoint_dispatch_unresolved") &&
			!hasFrontier(trigger.DynamicFrontier, "call_target_limit") {
			t.Errorf("callback-dispatched trigger lacks a bounded dispatch frontier: %#v", trigger)
		}
		switch trigger.Kind {
		case "http_route":
			if trigger.Identity.Path.Known {
				staticRoutes++
				if trigger.Resolution != "exact" || trigger.ProvisionalID {
					t.Errorf("exact detached route was degraded by a reachability frontier: %#v", trigger)
				}
			} else {
				dynamicRoutes++
			}
		case "http_server":
			servers++
			if trigger.ServerStartSite == nil || trigger.Resolution == "dynamic" || trigger.ProvisionalID {
				t.Errorf("server root = %#v", trigger)
			}
		}
	}
	if staticRoutes != 2 || dynamicRoutes != 1 || servers != 2 {
		t.Fatalf(
			"surface counts = static routes %d, dynamic routes %d, servers %d; triggers=%#v",
			staticRoutes,
			dynamicRoutes,
			servers,
			result.Catalog.Triggers,
		)
	}
	if !hasFrontier(result.Coverage.UnsupportedDispatch, "call_target_unresolved") {
		t.Fatalf("unsupported dispatch = %#v", result.Coverage.UnsupportedDispatch)
	}
}

func TestAnalyzeCaddyAdminRouteProviders(t *testing.T) {
	result := analyzeFixture(t, "caddy_admin")
	want := map[string]bool{
		"/load":                    true,
		"/adapt":                   true,
		"/reverse_proxy/upstreams": true,
		"/wrapped":                 false,
		"/variable":                false,
	}
	providerCount := 0
	frontierCount := 0
	for _, trigger := range result.Catalog.Triggers {
		if trigger.Kind == "http_route_frontier" {
			frontierCount++
			if trigger.SurfaceRole != SurfaceRoleDynamicFrontier || trigger.TraceReadiness != TraceReadinessUnsupported {
				t.Errorf("route frontier semantics = %#v", trigger)
			}
			if !hasFrontier(trigger.DynamicFrontier, "configuration_assembled_route_inventory") {
				t.Errorf("Caddy route assembly frontier = %#v", trigger)
			}
			continue
		}
		providerCount++
		path := trigger.Identity.Path.Text
		handlerKnown, expected := want[path]
		if trigger.Kind != "http_route_descriptor" || trigger.Status != "confirmed_route_descriptor" || !expected ||
			trigger.Framework != "caddy-admin" || !trigger.Identity.Path.Known ||
			trigger.Handler.Known != handlerKnown || !hasFrontier(trigger.DynamicFrontier, "route_provider_dispatch_candidate") {
			t.Errorf("Caddy provider trigger = %#v", trigger)
		}
		if trigger.SurfaceRole != SurfaceRoleDescriptor || trigger.TraceReadiness != TraceReadinessPartial ||
			!strings.Contains(trigger.TraceReadinessReason, "consumer registration") {
			t.Errorf("descriptor trace readiness = %#v", trigger)
		}
		if handlerKnown && (trigger.Resolution != "exact" || trigger.ProvisionalID) {
			t.Errorf("exact provider descriptor was degraded by provider-selection uncertainty: %#v", trigger)
		}
		if !handlerKnown && (trigger.Resolution != "partial" || !trigger.ProvisionalID) {
			t.Errorf("partially resolved provider descriptor = %#v", trigger)
		}
		if !handlerKnown && !hasFrontier(trigger.DynamicFrontier, "dynamic_handler_identity") {
			t.Errorf("dynamic provider handler lacks frontier: %#v", trigger)
		}
		if len(trigger.Evidence) < 2 {
			t.Errorf("Caddy provider evidence = %#v", trigger.Evidence)
		}
		delete(want, path)
	}
	if providerCount != 5 || frontierCount != 2 {
		t.Fatalf("provider/frontier counts = %d/%d; triggers=%#v", providerCount, frontierCount, result.Catalog.Triggers)
	}
	if len(want) != 0 {
		t.Fatalf("missing Caddy provider paths: %v", want)
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
	options := DefaultOptions(filepath.Join("testdata", "caddy_admin"))
	first, err := Analyze(options)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Analyze(options)
	if err != nil {
		t.Fatal(err)
	}
	first.Coverage.ColdLatencyMillis = 0
	second.Coverage.ColdLatencyMillis = 0
	for index := range first.Coverage.Phases {
		first.Coverage.Phases[index].LatencyMillis = 0
	}
	for index := range second.Coverage.Phases {
		second.Coverage.Phases[index].LatencyMillis = 0
	}
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

func TestTriggerIdentityAndDeduplicationPreserveOwnershipAndOrder(t *testing.T) {
	t.Parallel()

	base := TriggerRecord{
		Kind: "http_route", Identity: Identity{Path: knownValue("constant", "/health")},
		ProcessEntrypoint: Symbol{ID: "example.com/app.main"},
		RegistrationSite:  Location{Path: "routes.go", Line: 10, Column: 2},
		Dispatcher:        knownValue("dispatcher", "mux"), Handler: knownValue("function", "health"),
		FinalSeed: "seed", ScenarioID: "scenario", Resolution: "exact",
	}
	otherEntrypoint := base
	otherEntrypoint.ProcessEntrypoint.ID = "example.com/tool.main"
	otherColumn := base
	otherColumn.RegistrationSite.Column = 3
	if stableTriggerID(base) == stableTriggerID(otherEntrypoint) || stableTriggerID(base) == stableTriggerID(otherColumn) {
		t.Fatal("stable trigger identity collapsed entrypoint or callsite column")
	}

	strong := base
	strong.ID = stableTriggerID(strong)
	strong.Evidence = []Evidence{{ID: "strong", Location: strong.RegistrationSite}}
	weak := strong
	weak.Resolution = "dynamic"
	weak.ProvisionalID = true
	weak.DynamicFrontier = []Frontier{{Kind: "entrypoint_dispatch_unresolved", Detail: "weak"}}
	weak.Evidence = []Evidence{{ID: "weak", Location: weak.RegistrationSite}}
	first := Result{Catalog: TriggerCatalog{Triggers: deduplicateTriggerRecords([]TriggerRecord{weak, strong})}}
	second := Result{Catalog: TriggerCatalog{Triggers: deduplicateTriggerRecords([]TriggerRecord{strong, weak})}}
	first.normalize()
	second.normalize()
	firstJSON, err := MarshalDeterministic(first.Catalog.Triggers)
	if err != nil {
		t.Fatal(err)
	}
	secondJSON, err := MarshalDeterministic(second.Catalog.Triggers)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstJSON, secondJSON) || len(first.Catalog.Triggers) != 1 ||
		first.Catalog.Triggers[0].Resolution != "exact" || len(first.Catalog.Triggers[0].DynamicFrontier) != 0 {
		t.Fatalf("deduplication is order-sensitive or retained weaker authority:\n%s\n%s", firstJSON, secondJSON)
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

func triggersOfKind(result Result, kind string) []TriggerRecord {
	triggers := []TriggerRecord{}
	for _, trigger := range result.Catalog.Triggers {
		if trigger.Kind == kind {
			triggers = append(triggers, trigger)
		}
	}
	return triggers
}

func onlyTriggerOfKind(t *testing.T, result Result, kind string) TriggerRecord {
	t.Helper()
	triggers := triggersOfKind(result, kind)
	if len(triggers) != 1 {
		t.Fatalf("%s trigger count = %d, want 1: %#v", kind, len(triggers), result.Catalog.Triggers)
	}
	return triggers[0]
}
