package surfacediscovery

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestAnalyzeContextWithInputRequiresResolvedGoTarget(t *testing.T) {
	options := defaultHostOptions(t.TempDir())
	options.GoTarget = ""
	_, err := AnalyzeContextWithInput(context.Background(), options, Input{})
	if err == nil || !strings.Contains(err.Error(), "resolved Go target is required") {
		t.Fatalf("missing target error = %v", err)
	}
}

func TestAnalyzeContextWithInputRejectsMissingGraphLimits(t *testing.T) {
	for name, mutate := range map[string]func(*Options){
		"depth": func(options *Options) { options.DirectCallDepth = 0 },
		"edges": func(options *Options) { options.DirectCallEdgeLimit = 0 },
	} {
		t.Run(name, func(t *testing.T) {
			options := defaultHostOptions(t.TempDir())
			mutate(&options)
			if _, err := AnalyzeContextWithInput(context.Background(), options, Input{}); err == nil ||
				!strings.Contains(err.Error(), "must be at least 1") {
				t.Fatalf("missing graph limit error = %v", err)
			}
		})
	}
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

func TestTargetDirectCallDefaultsAreExplicitProductLimits(t *testing.T) {
	options := defaultHostOptions(".")
	if DefaultDirectCallDepth != 10 || DefaultDirectCallEdgeLimit != 10_000 {
		t.Fatalf("exported defaults = depth %d edges %d", DefaultDirectCallDepth, DefaultDirectCallEdgeLimit)
	}
	if options.DirectCallDepth != DefaultDirectCallDepth ||
		options.DirectCallEdgeLimit != DefaultDirectCallEdgeLimit {
		t.Fatalf("target direct call defaults = depth %d edges %d", options.DirectCallDepth, options.DirectCallEdgeLimit)
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
