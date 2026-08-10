package surfacediscovery

import (
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestModuleLibraryTargetRootsEveryExactPublicPackageOnly(t *testing.T) {
	repository := writeModuleLibraryFixture(t)
	input := moduleLibraryFixtureInput(false)
	options := DefaultOptions(repository)
	options.DirectCallDepth = 1

	result, err := AnalyzeWithInput(options, input)
	if err != nil {
		t.Fatal(err)
	}
	index := result.DirectCallIndex
	if index == nil || index.State != DirectCallIndexReady {
		t.Fatalf("module library index = %#v", index)
	}
	if err := index.Validate(); err != nil {
		t.Fatalf("Validate module library index: %v", err)
	}
	wantScopePackages := []string{"example.com/plural", "example.com/plural/client"}
	if index.Version != 3 || index.Scope.TargetRef != "target-plural" ||
		index.Scope.TargetKind != AnalysisTargetModuleLibrary ||
		index.Scope.TargetModuleID != "module-plural" ||
		index.Scope.TargetModulePath != "example.com/plural" ||
		index.Scope.TargetModuleDir != "." || index.Scope.TargetPackage != "" ||
		!reflect.DeepEqual(index.Scope.TargetPackages, wantScopePackages) {
		t.Fatalf("module library scope = %#v", index.Scope)
	}

	wantEdges := map[string]bool{
		"New->rootHelper":       true,
		"New->Open":             true,
		"Open->clientHelper":    true,
		"Export->genericHelper": true,
	}
	if got := targetDirectCallEdgeNames(index); !reflect.DeepEqual(got, wantEdges) {
		t.Fatalf("module library root edges = %v, want %v", got, wantEdges)
	}

	// The full admitted non-main package scope remains available as exact local
	// architecture/declaration context, but only TargetPackages supply roots.
	if !targetDirectCallHasNode(index, "InternalVisible") ||
		!targetDirectCallHasNode(index, "internalHelper") {
		t.Fatalf("admitted internal package disappeared from declaration catalog: %#v", index.Nodes)
	}
	for edgeName := range targetDirectCallEdgeNames(index) {
		if strings.HasPrefix(edgeName, "InternalVisible->") {
			t.Fatalf("non-target internal package became a library root: %s", edgeName)
		}
	}

	nodes := make(map[string]DirectCallNode, len(index.Nodes))
	for _, node := range index.Nodes {
		nodes[node.ID] = node
	}
	hiddenGenericWitnesses := 0
	for _, edge := range index.Edges {
		if nodes[edge.CallerID].Symbol.Name == "Export" &&
			nodes[edge.CalleeID].Symbol.Name == "genericHelper" {
			hiddenGenericWitnesses += edge.WitnessCount
		}
	}
	if hiddenGenericWitnesses != 1 {
		t.Fatalf("hidden generic receiver witness count = %d, want one origin witness", hiddenGenericWitnesses)
	}
}

func TestModuleLibraryTargetInputAndIndexAreCanonicalAndTamperClosed(t *testing.T) {
	repository := writeModuleLibraryFixture(t)
	options := DefaultOptions(repository)
	options.DirectCallDepth = 1

	first, err := AnalyzeWithInput(options, moduleLibraryFixtureInput(false))
	if err != nil {
		t.Fatal(err)
	}
	second, err := AnalyzeWithInput(options, moduleLibraryFixtureInput(true))
	if err != nil {
		t.Fatal(err)
	}
	if first.DirectCallIndex == nil || second.DirectCallIndex == nil ||
		!reflect.DeepEqual(first.DirectCallIndex, second.DirectCallIndex) {
		t.Fatalf("admitted package permutation changed module index:\nfirst  %#v\nsecond %#v", first.DirectCallIndex, second.DirectCallIndex)
	}

	permuted := moduleLibraryFixtureInput(false)
	permuted.AnalysisTarget.TargetPackages = []string{
		"example.com/plural/client", "example.com/plural",
	}
	if _, err := AnalyzeWithInput(options, permuted); err == nil ||
		!strings.Contains(err.Error(), "canonical sorted unique") {
		t.Fatalf("permuted target package authority error = %v", err)
	}

	outside := moduleLibraryFixtureInput(false)
	outside.AnalysisTarget.TargetPackages = append(
		append([]string(nil), outside.AnalysisTarget.TargetPackages...),
		"example.com/plural/missing",
	)
	if _, err := AnalyzeWithInput(options, outside); err == nil ||
		!strings.Contains(err.Error(), "outside the admitted module package scope") {
		t.Fatalf("out-of-scope target package error = %v", err)
	}

	wrongModule := moduleLibraryFixtureInput(false)
	wrongModule.AnalysisTarget.ModulePath = "example.com/tampered"
	if _, err := AnalyzeWithInput(options, wrongModule); err == nil ||
		!strings.Contains(err.Error(), "does not belong to sealed module") {
		t.Fatalf("tampered module identity error = %v", err)
	}

	whitespaceRef := moduleLibraryFixtureInput(false)
	whitespaceRef.AnalysisTarget.TargetRef += " "
	if _, err := AnalyzeWithInput(options, whitespaceRef); err == nil ||
		!strings.Contains(err.Error(), "module identity is required") {
		t.Fatalf("non-canonical target ref error = %v", err)
	}

	index := first.DirectCallIndex.Snapshot()
	slices.Reverse(index.Scope.TargetPackages)
	index.SHA256, _ = directCallIndexSHA256(index)
	if err := index.Validate(); err == nil || !strings.Contains(err.Error(), "invalid target scope packages") {
		t.Fatalf("permuted persisted scope validation = %v", err)
	}

	moduleTamper := first.DirectCallIndex.Snapshot()
	moduleTamper.Scope.TargetModulePath = "example.com/tampered"
	moduleTamper.SHA256, _ = directCallIndexSHA256(moduleTamper)
	if err := moduleTamper.Validate(); err == nil || !strings.Contains(err.Error(), "mismatched module identity") {
		t.Fatalf("tampered persisted module scope validation = %v", err)
	}

	snapshot := first.DirectCallIndex.Snapshot()
	snapshot.Scope.TargetPackages[0] = "example.com/changed"
	if first.DirectCallIndex.Scope.TargetPackages[0] != "example.com/plural" {
		t.Fatal("Snapshot shared target package storage with producer")
	}
}

func TestModuleLibraryTargetRejectsMainAndFailsClosedAtResourceLimit(t *testing.T) {
	repository := writeModuleLibraryFixture(t)
	writeTargetScopeFile(t, repository, "cmd/tool/main.go", `package main

func main() {}
`)
	mainTarget := moduleLibraryFixtureInput(false)
	mainTarget.Packages = append(mainTarget.Packages, PackageInput{
		Path: "example.com/plural/cmd/tool", ModuleDir: ".",
	})
	mainTarget.AnalysisTarget.TargetPackages = append(
		mainTarget.AnalysisTarget.TargetPackages,
		"example.com/plural/cmd/tool",
	)
	if _, err := AnalyzeWithInput(DefaultOptions(repository), mainTarget); err == nil ||
		!strings.Contains(err.Error(), "is executable") {
		t.Fatalf("main package module-library error = %v", err)
	}

	limitedOptions := DefaultOptions(repository)
	limitedOptions.DirectCallDepth = 1
	limitedOptions.DirectCallEdgeLimit = 1
	limited, err := AnalyzeWithInput(limitedOptions, moduleLibraryFixtureInput(false))
	if err != nil {
		t.Fatal(err)
	}
	index := limited.DirectCallIndex
	if index == nil || index.State != DirectCallIndexUnavailable ||
		index.ClosedReason != DirectCallIndexClosedEdgeLimit ||
		len(index.Nodes) != 0 || len(index.Edges) != 0 {
		t.Fatalf("resource-closed module index = %#v", index)
	}
	if err := index.Validate(); err != nil {
		t.Fatalf("Validate resource-closed module index: %v", err)
	}
}

func writeModuleLibraryFixture(t *testing.T) string {
	t.Helper()
	repository := t.TempDir()
	writeTargetScopeFile(t, repository, "go.mod", "module example.com/plural\n\ngo 1.24\n")
	writeTargetScopeFile(t, repository, "plural.go", `package plural

import "example.com/plural/client"

func New() { rootHelper(); client.Open() }
func rootHelper() {}
`)
	writeTargetScopeFile(t, repository, "client/client.go", `package client

type hiddenGeneric[T any] struct{}

func Open() { clientHelper() }
func (*hiddenGeneric[T]) Export() { genericHelper() }
func exerciseInstantiation() { var value hiddenGeneric[int]; value.Export() }
func clientHelper() {}
func genericHelper() {}
`)
	writeTargetScopeFile(t, repository, "internal/secret/secret.go", `package secret

func InternalVisible() { internalHelper() }
func internalHelper() {}
`)
	return repository
}

func moduleLibraryFixtureInput(reverse bool) Input {
	packages := []PackageInput{
		{Path: "example.com/plural", ModuleDir: "."},
		{Path: "example.com/plural/client", ModuleDir: "."},
		{Path: "example.com/plural/internal/secret", ModuleDir: "."},
	}
	if reverse {
		slices.Reverse(packages)
	}
	return Input{
		RepositoryName: "plural", ModuleDirs: []string{"."}, Packages: packages,
		AnalysisTarget: &AnalysisTargetInput{
			TargetRef: "target-plural", Kind: AnalysisTargetModuleLibrary,
			ModuleID: "module-plural", ModulePath: "example.com/plural", ModuleDir: ".",
			TargetPackages: []string{"example.com/plural", "example.com/plural/client"},
		},
	}
}
