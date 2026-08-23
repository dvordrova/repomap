package cubemap

import (
	"testing"

	"github.com/dvordrova/repomap/internal/coremap"
	"github.com/dvordrova/repomap/internal/gocoreobject"
	"github.com/dvordrova/repomap/internal/surfacediscovery"
)

func TestProjectCoreObjectsKeepsOnlyExactRepresentativeCallablesAndReceivers(t *testing.T) {
	index := coreObjectProjectionTestIndex(t, []gocoreobject.TypeDeclaration{
		{Kind: gocoreobject.TypeStruct, Package: "example.com/product", Name: "Engine", Exported: true, Location: gocoreobject.Location{Path: "engine.go", Line: 3, Column: 6}},
		{Kind: gocoreobject.TypeStruct, Package: "example.com/product", Name: "Unused", Exported: true, Location: gocoreobject.Location{Path: "unused.go", Line: 3, Column: 6}},
	}, []gocoreobject.CallableDeclaration{
		{Kind: gocoreobject.CallableMethod, Package: "example.com/product", Name: "Run", Receiver: "*example.com/product.Engine", Signature: "func() error", Exported: true, Location: gocoreobject.Location{Path: "engine.go", Line: 8, Column: 18}, DirectCallNodeID: "n-run"},
		{Kind: gocoreobject.CallableFunction, Package: "example.com/product", Name: "New", Signature: "func() *example.com/product.Engine", Exported: true, Location: gocoreobject.Location{Path: "engine.go", Line: 12, Column: 6}, DirectCallNodeID: "n-new"},
		{Kind: gocoreobject.CallableFunction, Package: "example.com/product", Name: "unused", Signature: "func()", Location: gocoreobject.Location{Path: "unused.go", Line: 8, Column: 6}, DirectCallNodeID: "n-unused"},
	})
	blocks := []coremap.Block{
		{ID: "core-a", Symbols: []coremap.SymbolFact{
			coreObjectProjectionSymbol("n-run", "Run", "engine.go", 8, 18),
			coreObjectProjectionSymbol("n-new", "New", "engine.go", 12, 6),
			{NodeID: "n-missing"},
		}},
		{ID: "core-b", Symbols: []coremap.SymbolFact{coreObjectProjectionSymbol("n-run", "Run", "engine.go", 8, 18)}},
	}

	projection, err := projectCoreObjects(blocks, index)
	if err != nil {
		t.Fatal(err)
	}
	if err := projection.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if len(projection.Callables) != 2 || len(projection.ReceiverTypes) != 1 ||
		projection.ReceiverTypes[0].Name != "Engine" || len(projection.Bindings) != 5 {
		t.Fatalf("projection = %#v", projection)
	}
	want := CoreObjectProjectionCoverage{
		CoreBlocksObserved: 2, RepresentativeSymbolClaims: 4, RepresentativeNodesObserved: 3,
		RepresentativeCallablesMatched: 2, RepresentativeNodesUnmatched: 1, CallableBindings: 3,
		ReceiverMethodsObserved: 1, ReceiverTypesMatched: 1, ReceiverTypeBindings: 2,
	}
	if projection.Coverage != want {
		t.Fatalf("coverage = %#v, want %#v", projection.Coverage, want)
	}
	tampered := projection
	tampered.Bindings = append([]CoreObjectBinding(nil), projection.Bindings...)
	tampered.Bindings[0].CoreBlockID = "changed"
	if err := tampered.Validate(); err == nil {
		t.Fatal("tampered projection retained its canonical seal")
	}
}

func TestProjectCoreObjectsOmitsAmbiguousGenericReceiverWithoutGuessing(t *testing.T) {
	index := coreObjectProjectionTestIndex(t, []gocoreobject.TypeDeclaration{
		{Kind: gocoreobject.TypeStruct, Package: "example.com/product", Name: "Box", Exported: true, Location: gocoreobject.Location{Path: "box.go", Line: 3, Column: 6}},
	}, []gocoreobject.CallableDeclaration{
		{Kind: gocoreobject.CallableMethod, Package: "example.com/product", Name: "Get", Receiver: "*example.com/product.Box[T]", Signature: "func() T", Exported: true, Location: gocoreobject.Location{Path: "box.go", Line: 8, Column: 18}, DirectCallNodeID: "n-get"},
	})

	projection, err := projectCoreObjects([]coremap.Block{{
		ID: "core-box", Symbols: []coremap.SymbolFact{coreObjectProjectionSymbol("n-get", "Get", "box.go", 8, 18)},
	}}, index)
	if err != nil {
		t.Fatal(err)
	}
	if len(projection.Callables) != 1 || len(projection.ReceiverTypes) != 0 ||
		projection.Coverage.ReceiverMethodsObserved != 1 || projection.Coverage.ReceiverMethodsOmitted != 1 ||
		projection.Coverage.GenericReceiverMethodsOmitted != 1 {
		t.Fatalf("generic receiver projection = %#v", projection)
	}
}

func coreObjectProjectionSymbol(nodeID, name, path string, line, column int) coremap.SymbolFact {
	return coremap.SymbolFact{
		NodeID: nodeID, Package: "example.com/product",
		Symbol:      surfacediscovery.Symbol{Name: name},
		Declaration: surfacediscovery.Location{Path: path, Line: line, Column: column},
	}
}

func coreObjectProjectionTestIndex(
	t *testing.T,
	types []gocoreobject.TypeDeclaration,
	callables []gocoreobject.CallableDeclaration,
) gocoreobject.Index {
	t.Helper()
	index, err := gocoreobject.New(gocoreobject.Input{
		Scenario: gocoreobject.Scenario{ID: "go:test", GOOS: "linux", GOARCH: "amd64", Tags: []string{}},
		Scope: gocoreobject.Scope{
			TargetRef: "target-1", TargetKind: gocoreobject.ScopeModuleLibrary,
			TargetModuleID: "module-1", TargetModulePath: "example.com/product", TargetModuleDir: ".",
			TargetPackages: []string{"example.com/product"},
		},
		Packages: []gocoreobject.Package{{
			ModuleID: "module-1", Module: "example.com/product", ModuleDir: ".", Path: "example.com/product", RepresentativeSource: "engine.go",
		}},
		Types: types, Callables: callables,
	})
	if err != nil {
		t.Fatal(err)
	}
	return index
}
