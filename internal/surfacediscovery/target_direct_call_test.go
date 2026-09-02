package surfacediscovery

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/godynamichandoff"
)

func TestAnalyzeContextWithInputRequiresResolvedGoTarget(t *testing.T) {
	options := defaultHostOptions(t.TempDir())
	options.GoTarget = ""
	_, err := AnalyzeContextWithInput(context.Background(), options, Input{})
	if err == nil || !strings.Contains(err.Error(), "resolved Go target is required") {
		t.Fatalf("missing target error = %v", err)
	}
}

func TestAnalyzeContextWithInputRejectsNegativeGraphControls(t *testing.T) {
	for name, mutate := range map[string]func(*Options){
		"depth": func(options *Options) { options.DirectCallDepth = -1 },
		"edges": func(options *Options) { options.DirectCallEdgeLimit = -1 },
	} {
		t.Run(name, func(t *testing.T) {
			options := defaultHostOptions(t.TempDir())
			mutate(&options)
			if _, err := AnalyzeContextWithInput(context.Background(), options, Input{}); err == nil ||
				!strings.Contains(err.Error(), "must be non-negative") {
				t.Fatalf("negative graph control error = %v", err)
			}
		})
	}
}

func TestTargetDirectCallCancellationDuringTraversalIsTerminal(t *testing.T) {
	repository := t.TempDir()
	writeTargetScopeFile(t, repository, "go.mod", "module example.com/cancelled\n\ngo 1.24\n")
	writeTargetScopeFile(t, repository, "main.go", `package main

func leaf() {}
func main() { leaf() }
`)
	input := targetDirectCallExecutableInput("example.com/cancelled", "main.go", 4)
	options := defaultHostOptions(repository)
	workspace, err := PrepareWorkspace(context.Background(), options, []Input{input})
	if err != nil {
		t.Fatal(err)
	}
	binding := workspace.bindings[input.AnalysisTarget.TargetRef]
	a, err := workspace.analyzerFor(
		context.Background(), options, binding.input, workspace.universes[binding.universeKey],
	)
	if err != nil {
		t.Fatal(err)
	}
	a.dynamicHandoffCapture = &dynamicHandoffCapture{}
	a.prepareTargetProgram()
	if roots := a.targetDirectCallRoots(); len(roots) != 1 {
		t.Fatalf("direct-call roots = %d, want one", len(roots))
	}

	// The first poll admits traversal setup; the second cancels at the first BFS
	// item. Returning nil here used to let analyzePreparedTarget seal the
	// declaration-only builder as a successful, incomplete DirectCallIndex.
	a.ctx = &cancelOnErrPollContext{Context: context.Background(), cancelAt: 2}
	if err := a.recordTargetDirectCallEdges(); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled direct-call traversal error = %v, want context.Canceled", err)
	}
}

type cancelOnErrPollContext struct {
	context.Context
	calls    int
	cancelAt int
}

func (ctx *cancelOnErrPollContext) Err() error {
	ctx.calls++
	if ctx.calls >= ctx.cancelAt {
		return context.Canceled
	}
	return nil
}

func TestTargetDirectCallRejectsMissingExactExecutableRoot(t *testing.T) {
	repository := t.TempDir()
	writeTargetScopeFile(t, repository, "go.mod", "module example.com/root\n\ngo 1.24\n")
	writeTargetScopeFile(t, repository, "main.go", `package main

func helper() {}
func main() { helper() }
`)
	input := targetDirectCallExecutableInput("example.com/root", "main.go", 3)
	_, err := analyzeForTest(defaultHostOptions(repository), input)
	var targetErr *AnalysisTargetSSAUnavailableError
	if !errors.As(err, &targetErr) || targetErr.Reason != AnalysisTargetExactRootsUnavailable ||
		targetErr.ExpectedRoots != 1 || targetErr.ResolvedRoots != 0 {
		t.Fatalf("missing exact root error = %#v / %v", targetErr, err)
	}
}

func TestTargetDirectCallDepthBoundsEdgesButRetainsCompleteDeclarations(t *testing.T) {
	repository := t.TempDir()
	writeTargetScopeFile(t, repository, "go.mod", "module example.com/graph\n\ngo 1.24\n")
	writeTargetScopeFile(t, repository, "main.go", `package main

func main() { first() }
func first() { second() }
func second() { third() }
func third() {}
func unrelated() { leaf() }
func leaf() {}
`)
	input := targetDirectCallExecutableInput("example.com/graph", "main.go", 3)

	options := defaultHostOptions(repository)
	options.DirectCallDepth = 2
	result, err := analyzeForTest(options, input)
	if err != nil {
		t.Fatal(err)
	}
	index := result.DirectCallIndex
	if index == nil || index.State != DirectCallIndexReady {
		t.Fatalf("index = %#v", index)
	}
	if err := index.Validate(); err != nil {
		t.Fatalf("Validate depth-2 index: %v", err)
	}
	if index.Scope.MaxDepth != 2 || index.Scope.EdgeLimit != DefaultDirectCallEdgeLimit ||
		index.Coverage.DepthBoundRepositoryCallsExcluded != 1 {
		t.Fatalf("depth-2 authority = scope %#v coverage %#v", index.Scope, index.Coverage)
	}
	second := targetDirectCallNodeBySymbol(t, index, "second")
	var frontier DirectCallNodeFrontier
	ok := false
	for _, candidate := range index.Frontiers {
		if candidate.CallerID == second.ID {
			frontier = candidate
			ok = true
			break
		}
	}
	if !ok || frontier.DepthBoundRepositoryCallsExcluded != 1 {
		t.Fatalf("second depth frontier = %+v/%v, want one exact omitted call", frontier, ok)
	}
	for _, symbol := range []string{"main", "first", "second", "third", "unrelated", "leaf"} {
		if !targetDirectCallHasNode(index, symbol) {
			t.Fatalf("complete declaration catalog lost %q: %#v", symbol, index.Nodes)
		}
	}
	want := map[string]bool{"main->first": true, "first->second": true}
	if got := targetDirectCallEdgeNames(index); !reflect.DeepEqual(got, want) {
		t.Fatalf("depth-2 edges = %v, want %v", got, want)
	}

	options.DirectCallDepth = 3
	deeper, err := analyzeForTest(options, input)
	if err != nil {
		t.Fatal(err)
	}
	want["second->third"] = true
	if got := targetDirectCallEdgeNames(deeper.DirectCallIndex); !reflect.DeepEqual(got, want) {
		t.Fatalf("depth-3 edges = %v, want %v", got, want)
	}
	if err := deeper.DirectCallIndex.Validate(); err != nil {
		t.Fatalf("Validate depth-3 index: %v", err)
	}
	if deeper.DirectCallIndex.Coverage.DepthBoundRepositoryCallsExcluded != 0 {
		t.Fatalf("exhausted graph retained depth frontier: %#v", deeper.DirectCallIndex.Coverage)
	}

	options.DirectCallDepth = 4
	exhausted, err := analyzeForTest(options, input)
	if err != nil {
		t.Fatal(err)
	}
	if got := targetDirectCallEdgeNames(exhausted.DirectCallIndex); !reflect.DeepEqual(got, want) {
		t.Fatalf("depth-4 exhausted edges = %v, want %v", got, want)
	}
	if exhausted.DirectCallIndex.SHA256 == deeper.DirectCallIndex.SHA256 {
		t.Fatal("configured depth was not bound into DirectCallIndex identity")
	}
}

func TestTargetDirectCallDefaultRetainsCallsBeyondFormerDepth(t *testing.T) {
	repository := t.TempDir()
	writeTargetScopeFile(t, repository, "go.mod", "module example.com/deep\n\ngo 1.24\n")
	var source strings.Builder
	source.WriteString("package main\n\nfunc main() { step0() }\n")
	for depth := 0; depth < AdvisoryDirectCallMaxDepth+2; depth++ {
		if depth == AdvisoryDirectCallMaxDepth+1 {
			fmt.Fprintf(&source, "func step%d() {}\n", depth)
			continue
		}
		fmt.Fprintf(&source, "func step%d() { step%d() }\n", depth, depth+1)
	}
	writeTargetScopeFile(t, repository, "main.go", source.String())

	result, err := analyzeForTest(
		defaultHostOptions(repository),
		targetDirectCallExecutableInput("example.com/deep", "main.go", 3),
	)
	if err != nil {
		t.Fatal(err)
	}
	index := result.DirectCallIndex
	if index == nil || index.State != DirectCallIndexReady ||
		index.Scope.MaxDepth != 0 || index.Scope.EdgeLimit != 0 {
		t.Fatalf("unbounded default index = %#v", index)
	}
	wantLastEdge := fmt.Sprintf("step%d->step%d", AdvisoryDirectCallMaxDepth, AdvisoryDirectCallMaxDepth+1)
	if !targetDirectCallEdgeNames(index)[wantLastEdge] {
		t.Fatalf("default graph lost edge beyond former depth %d: %v", AdvisoryDirectCallMaxDepth, targetDirectCallEdgeNames(index))
	}
	if index.Coverage.TraversalDepthReached <= AdvisoryDirectCallMaxDepth ||
		index.Coverage.DepthBoundRepositoryCallsExcluded != 0 {
		t.Fatalf("default traversal coverage = %#v", index.Coverage)
	}
	warnings := DirectCallScaleWarnings(*index)
	if len(warnings) != 1 || warnings[0].Kind != DirectCallScaleWarningDepth ||
		warnings[0].Retained != index.Coverage.TraversalDepthReached {
		t.Fatalf("deep graph warnings = %#v", warnings)
	}
}

func TestTargetDirectCallEdgeLimitClosesBeforeProviderGraphCanBeUsed(t *testing.T) {
	repository := t.TempDir()
	writeTargetScopeFile(t, repository, "go.mod", "module example.com/limit\n\ngo 1.24\n")
	writeTargetScopeFile(t, repository, "main.go", `package main

func main() { first() }
func first() { second() }
func second() {}
`)
	input := targetDirectCallExecutableInput("example.com/limit", "main.go", 3)
	options := defaultHostOptions(repository)
	options.DirectCallDepth = 2
	options.DirectCallEdgeLimit = 1

	limited, err := analyzeForTest(options, input)
	if err != nil {
		t.Fatal(err)
	}
	if limited.DirectCallIndex == nil ||
		limited.DirectCallIndex.State != DirectCallIndexUnavailable ||
		limited.DirectCallIndex.ClosedReason != DirectCallIndexClosedEdgeLimit ||
		len(limited.DirectCallIndex.Nodes) != 0 || len(limited.DirectCallIndex.Edges) != 0 {
		t.Fatalf("limited index = %#v", limited.DirectCallIndex)
	}
	if limited.DirectCallIndex.Coverage.EdgeLimitSafeDepth != 1 {
		t.Fatalf("edge-limit safe depth = %d, want 1", limited.DirectCallIndex.Coverage.EdgeLimitSafeDepth)
	}
	if err := limited.DirectCallIndex.Validate(); err != nil {
		t.Fatalf("Validate limited index: %v", err)
	}

	// The first remedy printed to the user is causal: reducing depth removes
	// the second edge instead of merely changing error prose.
	options.DirectCallDepth = 1
	recovered, err := analyzeForTest(options, input)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.DirectCallIndex == nil || recovered.DirectCallIndex.State != DirectCallIndexReady ||
		len(recovered.DirectCallIndex.Edges) != 1 ||
		recovered.DirectCallIndex.Coverage.DepthBoundRepositoryCallsExcluded != 1 {
		t.Fatalf("lower-depth recovery = %#v", recovered.DirectCallIndex)
	}
	if err := recovered.DirectCallIndex.Validate(); err != nil {
		t.Fatalf("Validate lower-depth recovery: %v", err)
	}

	options.DirectCallDepth = 2
	options.DirectCallEdgeLimit = 2
	admitted, err := analyzeForTest(options, input)
	if err != nil {
		t.Fatal(err)
	}
	if admitted.DirectCallIndex == nil || admitted.DirectCallIndex.State != DirectCallIndexReady ||
		len(admitted.DirectCallIndex.Edges) != 2 {
		t.Fatalf("admitted index = %#v", admitted.DirectCallIndex)
	}
}

func TestTargetDirectCallLibraryUsesEveryExactExportedAPIRoot(t *testing.T) {
	repository := t.TempDir()
	writeTargetScopeFile(t, repository, "go.mod", "module example.com/library\n\ngo 1.24\n")
	writeTargetScopeFile(t, repository, "library.go", `package library

type Service struct{}
type hiddenService struct{}

func New() *Service { newHelper(); return &Service{} }
func (Service) Start() { startHelper() }
func (*hiddenService) Exported() { hiddenHelper() }
func (*hiddenService) private() {}
func newHelper() { leaf() }
func startHelper() { leaf() }
func hiddenHelper() { leaf() }
func hidden() { leaf() }
func leaf() {}
`)
	input := Input{
		ModuleDirs: []string{"."},
		Packages:   []PackageInput{{Path: "example.com/library", ModuleDir: "."}},
		AnalysisTarget: &AnalysisTargetInput{
			TargetRef: "target-library", Kind: AnalysisTargetModuleLibrary,
			ModuleID: "module-library", ModulePath: "example.com/library", ModuleDir: ".",
			TargetPackages: []string{"example.com/library"},
		},
	}
	options := defaultHostOptions(repository)
	options.DirectCallDepth = 1
	result, err := analyzeForTest(options, input)
	if err != nil {
		t.Fatal(err)
	}
	index := result.DirectCallIndex
	if index == nil || index.State != DirectCallIndexReady {
		t.Fatalf("index = %#v", index)
	}
	if err := index.Validate(); err != nil {
		t.Fatalf("Validate library index: %v", err)
	}
	if index.Scope.TargetRef != "target-library" || index.Scope.TargetKind != AnalysisTargetModuleLibrary ||
		index.Scope.TargetModuleID != "module-library" || index.Scope.TargetModulePath != "example.com/library" ||
		index.Scope.TargetModuleDir != "." || index.Scope.TargetPackage != "" ||
		!reflect.DeepEqual(index.Scope.TargetPackages, []string{"example.com/library"}) || index.Scope.MaxDepth != 1 ||
		index.Scope.EdgeLimit != DefaultDirectCallEdgeLimit {
		t.Fatalf("library scope = %#v", index.Scope)
	}
	for _, symbol := range []string{"New", "Start", "Exported", "newHelper", "startHelper", "hiddenHelper", "hidden", "leaf"} {
		if !targetDirectCallHasNode(index, symbol) {
			t.Fatalf("complete declaration catalog lost %q: %#v", symbol, index.Nodes)
		}
	}
	nodes := make(map[string]DirectCallNode, len(index.Nodes))
	for _, node := range index.Nodes {
		nodes[node.ID] = node
	}
	if len(index.Edges) != 3 {
		t.Fatalf("library root edges = %#v, want exactly New, Start, and unused Exported calls", index.Edges)
	}
	callers := make(map[string]bool)
	unusedHiddenMethodWitnesses := 0
	for _, edge := range index.Edges {
		caller := nodes[edge.CallerID]
		if !caller.Exported || caller.Package != "example.com/library" {
			t.Fatalf("non-exported function became library root: %#v", caller)
		}
		callers[caller.Symbol.Name] = true
		if caller.Symbol.Name == "Exported" && nodes[edge.CalleeID].Symbol.Name == "hiddenHelper" {
			unusedHiddenMethodWitnesses += edge.WitnessCount
		}
	}
	if !callers["New"] || !callers["Start"] || !callers["Exported"] ||
		unusedHiddenMethodWitnesses != 1 {
		t.Fatalf("library exported root callers = %v", callers)
	}
}

func TestTargetDirectCallLibraryGenericMethodRootUsesOneOriginWitness(t *testing.T) {
	repository := t.TempDir()
	writeTargetScopeFile(t, repository, "go.mod", "module example.com/generic\n\ngo 1.24\n")
	writeTargetScopeFile(t, repository, "generic.go", `package generic

type hiddenGeneric[T any] struct{}
func (*hiddenGeneric[T]) Convert() { helper() }
func use() { var value hiddenGeneric[int]; value.Convert() }
func helper() {}
`)
	input := Input{
		ModuleDirs: []string{"."},
		Packages:   []PackageInput{{Path: "example.com/generic", ModuleDir: "."}},
		AnalysisTarget: &AnalysisTargetInput{
			TargetRef: "target-generic", Kind: AnalysisTargetModuleLibrary,
			ModuleID: "module-generic", ModulePath: "example.com/generic", ModuleDir: ".",
			TargetPackages: []string{"example.com/generic"},
		},
	}
	result, err := analyzeForTest(defaultHostOptions(repository), input)
	if err != nil {
		t.Fatal(err)
	}
	index := result.DirectCallIndex
	if index == nil || index.State != DirectCallIndexReady || len(index.Edges) != 1 {
		t.Fatalf("generic library index = %#v", index)
	}
	nodes := make(map[string]DirectCallNode, len(index.Nodes))
	for _, node := range index.Nodes {
		nodes[node.ID] = node
	}
	edge := index.Edges[0]
	if nodes[edge.CallerID].Symbol.Name != "Convert" ||
		nodes[edge.CalleeID].Symbol.Name != "helper" || edge.WitnessCount != 1 {
		t.Fatalf("generic method root edge = %#v nodes=%#v", edge, nodes)
	}
}

func TestTargetDirectCallContinuesThroughExactFieldCallableBinding(t *testing.T) {
	repository := t.TempDir()
	source := `package main

type Handler interface { Serve() }
type HandlerFunc func()
func (function HandlerFunc) Serve() { function() }
type Server struct { Handler Handler }
type Box struct { Value any }

func leaf() {}
func handle() {
	_ = &Server{Handler: HandlerFunc(handle)}
	leaf()
}
func main() {
	_ = &Server{Handler: HandlerFunc(handle)}
	_ = &Box{Value: HandlerFunc(handle)}
	println("ready")
}
`
	writeTargetScopeFile(t, repository, "go.mod", "module example.com/binding\n\ngo 1.24\n")
	writeTargetScopeFile(t, repository, "main.go", source)
	mainLine := strings.Count(source[:strings.Index(source, "func main")], "\n") + 1
	input := targetDirectCallExecutableInput("example.com/binding", "main.go", mainLine)

	options := defaultHostOptions(repository)
	options.CaptureDynamicHandoffIndex = true
	options.DirectCallDepth = 1
	shallow, err := analyzeForTest(options, input)
	if err != nil {
		t.Fatal(err)
	}
	if err := shallow.DirectCallIndex.Validate(); err != nil {
		t.Fatal(err)
	}
	if shallow.DirectCallIndex.Coverage.BuiltinCallsExcluded != 1 ||
		shallow.DirectCallIndex.Coverage.NonStaticCallsExcluded != 0 {
		t.Fatalf("builtin coverage = %#v", shallow.DirectCallIndex.Coverage)
	}
	if got := targetDirectCallEdgeNames(shallow.DirectCallIndex); len(got) != 0 {
		t.Fatalf("depth-1 exact edges = %v: binding must not invent main->handle and handle->leaf is behind the depth frontier", got)
	}
	if shallow.DirectCallIndex.Coverage.DepthBoundRepositoryCallsExcluded != 1 {
		t.Fatalf("depth-1 frontier = %#v", shallow.DirectCallIndex.Coverage)
	}
	if shallow.DynamicHandoffIndex == nil || shallow.DynamicHandoffIndex.Coverage.CallableBindings != 2 {
		t.Fatalf("callable binding overlay = %#v", shallow.DynamicHandoffIndex)
	}
	mainNode := targetDirectCallNodeBySymbol(t, shallow.DirectCallIndex, "main")
	handleNode := targetDirectCallNodeBySymbol(t, shallow.DirectCallIndex, "handle")
	mainBinding := false
	for _, handoff := range shallow.DynamicHandoffIndex.Handoffs {
		if handoff.Kind == godynamichandoff.CallableBinding && handoff.Slot.Field == "Value" {
			t.Fatalf("empty any field became a callable binding: %#v", handoff)
		}
		if handoff.Kind == godynamichandoff.CallableBinding && handoff.CallerID == mainNode.ID &&
			handoff.Resolution == godynamichandoff.ResolutionExact && len(handoff.Candidates) == 1 &&
			handoff.Candidates[0].FunctionID == handleNode.ID && handoff.Slot.Field == "Handler" {
			mainBinding = handoff.Invocation == godynamichandoff.InvocationBinding
		}
	}
	if !mainBinding {
		t.Fatalf("main callable binding is absent: %#v", shallow.DynamicHandoffIndex.Handoffs)
	}

	options.DirectCallDepth = 2
	deep, err := analyzeForTest(options, input)
	if err != nil {
		t.Fatal(err)
	}
	if err := deep.DirectCallIndex.Validate(); err != nil {
		t.Fatal(err)
	}
	if got, want := targetDirectCallEdgeNames(deep.DirectCallIndex), map[string]bool{"handle->leaf": true}; !reflect.DeepEqual(got, want) {
		t.Fatalf("depth-2 exact edges = %v, want %v", got, want)
	}
	if deep.DirectCallIndex.Coverage.DepthBoundRepositoryCallsExcluded != 0 {
		t.Fatalf("cycle-safe traversal retained false frontier: %#v", deep.DirectCallIndex.Coverage)
	}

	// The structural snapshot is captured independently of the optional
	// dynamic-handoff output. Disabling that output must not change exact BFS.
	options.CaptureDynamicHandoffIndex = false
	withoutOverlay, err := analyzeForTest(options, input)
	if err != nil {
		t.Fatal(err)
	}
	if withoutOverlay.DynamicHandoffIndex != nil {
		t.Fatalf("disabled dynamic overlay was published: %#v", withoutOverlay.DynamicHandoffIndex)
	}
	if got, want := targetDirectCallEdgeNames(withoutOverlay.DirectCallIndex), targetDirectCallEdgeNames(deep.DirectCallIndex); !reflect.DeepEqual(got, want) {
		t.Fatalf("optional dynamic output changed binding traversal: got %v want %v", got, want)
	}
}

func TestTargetDirectCallContinuesThroughExactCallbackArgument(t *testing.T) {
	repository := t.TempDir()
	source := `package main

func register(_ func()) {}
func leaf() {}
func handler() { leaf() }
func main() { register(handler) }
`
	writeTargetScopeFile(t, repository, "go.mod", "module example.com/callback\n\ngo 1.24\n")
	writeTargetScopeFile(t, repository, "main.go", source)
	mainLine := strings.Count(source[:strings.Index(source, "func main")], "\n") + 1
	input := targetDirectCallExecutableInput("example.com/callback", "main.go", mainLine)

	options := defaultHostOptions(repository)
	options.CaptureDynamicHandoffIndex = true
	options.DirectCallDepth = 1
	shallow, err := analyzeForTest(options, input)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := targetDirectCallEdgeNames(shallow.DirectCallIndex), map[string]bool{"main->register": true}; !reflect.DeepEqual(got, want) {
		t.Fatalf("depth-1 exact edges = %v, want %v", got, want)
	}
	if shallow.DirectCallIndex.Coverage.DepthBoundRepositoryCallsExcluded != 1 {
		t.Fatalf("depth-1 callback frontier = %#v", shallow.DirectCallIndex.Coverage)
	}
	mainNode := targetDirectCallNodeBySymbol(t, shallow.DirectCallIndex, "main")
	handlerNode := targetDirectCallNodeBySymbol(t, shallow.DirectCallIndex, "handler")
	transfers := 0
	for _, handoff := range shallow.DynamicHandoffIndex.Handoffs {
		if handoff.Kind != godynamichandoff.CallbackTransfer || handoff.CallerID != mainNode.ID {
			continue
		}
		transfers++
		if handoff.Slot.Parameter != 1 || handoff.Resolution != godynamichandoff.ResolutionExact ||
			len(handoff.Candidates) != 1 || handoff.Candidates[0].FunctionID != handlerNode.ID {
			t.Fatalf("exact callback transfer = %#v", handoff)
		}
	}
	if transfers != 1 {
		t.Fatalf("callback transfers = %d, want one: %#v", transfers, shallow.DynamicHandoffIndex.Handoffs)
	}

	options.DirectCallDepth = 2
	deep, err := analyzeForTest(options, input)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := targetDirectCallEdgeNames(deep.DirectCallIndex), map[string]bool{
		"main->register": true, "handler->leaf": true,
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("depth-2 exact edges = %v, want %v", got, want)
	}

	options.CaptureDynamicHandoffIndex = false
	withoutOverlay, err := analyzeForTest(options, input)
	if err != nil {
		t.Fatal(err)
	}
	if withoutOverlay.DynamicHandoffIndex != nil {
		t.Fatalf("disabled dynamic overlay was published: %#v", withoutOverlay.DynamicHandoffIndex)
	}
	if got, want := targetDirectCallEdgeNames(withoutOverlay.DirectCallIndex), targetDirectCallEdgeNames(deep.DirectCallIndex); !reflect.DeepEqual(got, want) {
		t.Fatalf("optional dynamic output changed callback traversal: got %v want %v", got, want)
	}
}

func TestTargetDirectCallDoesNotTraverseAlternativeFieldCallableBinding(t *testing.T) {
	repository := t.TempDir()
	source := `package main

import "os"

type Handler interface { Serve() }
type HandlerFunc func()
func (function HandlerFunc) Serve() { function() }
type Server struct { Handler Handler }

func leafA() {}
func leafB() {}
func alpha() { leafA() }
func beta() { leafB() }
func main() {
	var callback HandlerFunc
	if len(os.Args) > 1 { callback = alpha } else { callback = beta }
	_ = &Server{Handler: callback}
}
`
	writeTargetScopeFile(t, repository, "go.mod", "module example.com/alternatives\n\ngo 1.24\n")
	writeTargetScopeFile(t, repository, "main.go", source)
	mainLine := strings.Count(source[:strings.Index(source, "func main")], "\n") + 1
	input := targetDirectCallExecutableInput("example.com/alternatives", "main.go", mainLine)
	options := defaultHostOptions(repository)
	options.CaptureDynamicHandoffIndex = true
	options.DirectCallDepth = 3
	result, err := analyzeForTest(options, input)
	if err != nil {
		t.Fatal(err)
	}
	if got := targetDirectCallEdgeNames(result.DirectCallIndex); len(got) != 0 {
		t.Fatalf("alternative binding entered exact BFS: %v", got)
	}
	mainNode := targetDirectCallNodeBySymbol(t, result.DirectCallIndex, "main")
	bindings := 0
	for _, handoff := range result.DynamicHandoffIndex.Handoffs {
		if handoff.Kind != godynamichandoff.CallableBinding || handoff.CallerID != mainNode.ID {
			continue
		}
		bindings++
		if handoff.Resolution != godynamichandoff.ResolutionAlternatives || len(handoff.Candidates) != 2 {
			t.Fatalf("alternative field binding lost candidate authority: %#v", handoff)
		}
	}
	if bindings != 1 {
		t.Fatalf("callable bindings = %d, want one alternatives row: %#v", bindings, result.DynamicHandoffIndex.Handoffs)
	}
}

func TestTargetDirectCallDefaultsAreUnbounded(t *testing.T) {
	options := defaultHostOptions(".")
	if DefaultDirectCallDepth != 0 || DefaultDirectCallEdgeLimit != 0 {
		t.Fatalf("exported defaults = depth %d edges %d", DefaultDirectCallDepth, DefaultDirectCallEdgeLimit)
	}
	if options.DirectCallDepth != DefaultDirectCallDepth ||
		options.DirectCallEdgeLimit != DefaultDirectCallEdgeLimit {
		t.Fatalf("target direct call defaults = depth %d edges %d", options.DirectCallDepth, options.DirectCallEdgeLimit)
	}
}

func TestDirectCallBuilderRetainsPastFormerNodeAndEdgeThresholds(t *testing.T) {
	builder := newDirectCallIndexBuilder(Scenario{ID: "scenario", GOOS: "linux", GOARCH: "amd64"}, 0)
	for position := 0; position <= AdvisoryDirectCallMaxNodes; position++ {
		id := fmt.Sprintf("node-%06d", position)
		builder.nodes[id] = DirectCallNode{ID: id, Symbol: Symbol{Name: id}}
	}
	for position := 0; position <= AdvisoryDirectCallMaxEdges; position++ {
		id := fmt.Sprintf("edge-%06d", position)
		builder.edges[id] = DirectCallEdge{ID: id, CallerID: id}
	}
	index := builder.finish()
	if index.State != DirectCallIndexReady || len(index.Nodes) != AdvisoryDirectCallMaxNodes+1 ||
		len(index.Edges) != AdvisoryDirectCallMaxEdges+1 {
		t.Fatalf("builder truncated or closed former thresholds: state=%s nodes=%d edges=%d",
			index.State, len(index.Nodes), len(index.Edges))
	}
}

func TestDirectCallScaleWarningsCannotRejectMalformedDiagnosticInput(t *testing.T) {
	if warnings := DirectCallScaleWarnings(DirectCallIndex{}); len(warnings) != 0 {
		t.Fatalf("malformed diagnostic input warnings = %#v, want none and no failure", warnings)
	}
}

func TestNormalizeWorkspaceOptionsAcceptsExplicitEdgeLimitPastFormerAbsoluteMaximum(t *testing.T) {
	const formerAbsoluteEdgeMaximum = 262_144
	options := defaultHostOptions(".")
	options.DirectCallEdgeLimit = formerAbsoluteEdgeMaximum + 1
	if _, _, _, err := normalizeWorkspaceOptions(context.Background(), options); err != nil {
		t.Fatalf("explicit edge ceiling past former absolute maximum: %v", err)
	}
}

func TestRepositoryLocationValidationHasOneNonRecursiveBase(t *testing.T) {
	repositoryLocation := Location{Path: "internal/router/router.go", Line: 17, Column: 0}
	if !validEntryHandoffLocation(repositoryLocation) || !validRepositoryDirectCallLocation(repositoryLocation) {
		t.Fatalf("repository location was rejected: %#v", repositoryLocation)
	}
	externalLocation := Location{Path: "<external>/net/http", Line: 1, Column: 0}
	if !validEntryHandoffLocation(externalLocation) || validRepositoryDirectCallLocation(externalLocation) {
		t.Fatalf("external location boundary was not preserved: %#v", externalLocation)
	}
}

func targetDirectCallExecutableInput(packagePath, rootPath string, rootLine int) Input {
	return Input{
		ModuleDirs: []string{"."},
		Packages:   []PackageInput{{Path: packagePath, ModuleDir: "."}},
		AnalysisTarget: &AnalysisTargetInput{
			TargetRef: "target:" + packagePath, Kind: AnalysisTargetExecutablePackage,
			ModuleID: "module:" + packagePath, ModulePath: packagePath, ModuleDir: ".",
			PackagePath: packagePath, TargetPackages: []string{packagePath},
			Roots: []AnalysisTargetRootInput{{Path: rootPath, Line: rootLine}},
		},
	}
}

func targetDirectCallHasNode(index *DirectCallIndex, name string) bool {
	if index == nil {
		return false
	}
	for _, node := range index.Nodes {
		if node.Symbol.Name == name {
			return true
		}
	}
	return false
}

func targetDirectCallNodeBySymbol(t *testing.T, index *DirectCallIndex, name string) DirectCallNode {
	t.Helper()
	if index != nil {
		for _, node := range index.Nodes {
			if node.Symbol.Name == name {
				return node
			}
		}
	}
	t.Fatalf("direct-call node %q is absent", name)
	return DirectCallNode{}
}

func targetDirectCallEdgeNames(index *DirectCallIndex) map[string]bool {
	result := make(map[string]bool)
	if index == nil {
		return result
	}
	names := make(map[string]string, len(index.Nodes))
	for _, node := range index.Nodes {
		names[node.ID] = node.Symbol.Name
	}
	for _, edge := range index.Edges {
		result[names[edge.CallerID]+"->"+names[edge.CalleeID]] = true
	}
	return result
}
