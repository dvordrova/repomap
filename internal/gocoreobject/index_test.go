package gocoreobject

import "testing"

func TestNewCanonicalizesSealsAndSnapshotsExactDeclarations(t *testing.T) {
	index, err := New(Input{
		Scenario: Scenario{ID: "go:linux/amd64:tags=integration,netgo", GOOS: "linux", GOARCH: "amd64", Tags: []string{"netgo", "integration"}},
		Scope: Scope{
			TargetRef: "target-1", TargetKind: "module_library", TargetModuleID: "target-module",
			TargetModulePath: "example.com/product", TargetModuleDir: ".",
			TargetPackages: []string{"example.com/product/client", "example.com/product"},
		},
		Packages: []Package{
			{ModuleID: "direct-module", Module: "example.com/product", ModuleDir: ".", Path: "example.com/product/client", RepresentativeSource: "client/client.go"},
			{ModuleID: "direct-module", Module: "example.com/product", ModuleDir: ".", Path: "example.com/product", RepresentativeSource: "engine.go"},
		},
		Types: []TypeDeclaration{
			{Kind: TypeInterface, Package: "example.com/product/client", Name: "Transport", Exported: true, Location: Location{Path: "client/client.go", Line: 8, Column: 6}},
			{Kind: TypeStruct, Package: "example.com/product", Name: "Engine", Exported: true, Location: Location{Path: "engine.go", Line: 5, Column: 6}},
		},
		Callables: []CallableDeclaration{
			{Kind: CallableMethod, Package: "example.com/product", Name: "Run", Receiver: "*example.com/product.Engine", Signature: "func(ctx context.Context) error", Exported: true, Location: Location{Path: "engine.go", Line: 12, Column: 18}, DirectCallNodeID: "direct-node-1"},
			{Kind: CallableFunction, Package: "example.com/product", Name: "New", Signature: "func() *example.com/product.Engine", Exported: true, Location: Location{Path: "engine.go", Line: 9, Column: 6}},
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := index.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if index.SHA256 == "" || index.Types[0].ID == "" || index.Callables[0].ID == "" ||
		index.Scope.TargetPackages[0] != "example.com/product" || index.Packages[0].Path != "example.com/product" {
		t.Fatalf("non-canonical index: %#v", index)
	}
	snapshot := index.Snapshot()
	snapshot.Scope.TargetPackages[0] = "changed"
	snapshot.Types[0].Name = "Changed"
	if index.Scope.TargetPackages[0] != "example.com/product" || index.Types[0].Name != "Engine" {
		t.Fatalf("snapshot aliases producer: %#v", index)
	}
	tampered := index.Snapshot()
	tampered.Callables[0].Signature = "func()"
	if err := tampered.Validate(); err == nil {
		t.Fatal("tampered signature retained the producer seal")
	}
}
