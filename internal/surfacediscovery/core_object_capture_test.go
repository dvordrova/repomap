package surfacediscovery

import (
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/gocoreobject"
)

func TestCoreObjectIndexCapturesExactTargetDeclarationsFromExistingTypedProgram(t *testing.T) {
	repository := t.TempDir()
	writeTargetScopeFile(t, repository, "go.mod", "module example.com/core\n\ngo 1.24\n")
	writeTargetScopeFile(t, repository, "core.go", `package core

import "context"

type ID string
type Config struct{ Enabled bool }
type Runner interface{ Run(context.Context) error }
type ConfigAlias = Config

var ignoredVariable Config
const ignoredConstant = 1

func New[T any](value T) *Config { return &Config{} }
func (*Config) Start(ctx context.Context, retries ...int) error { return nil }
func hidden() {}
`)

	options := defaultHostOptions(repository)
	options.CaptureCoreObjectIndex = true
	result, err := analyzeForTest(options, Input{
		ModuleDirs: []string{"."},
		Packages:   []PackageInput{{Path: "example.com/core", ModuleDir: "."}},
		AnalysisTarget: &AnalysisTargetInput{
			TargetRef: "target-core", Kind: AnalysisTargetModuleLibrary,
			ModuleID: "module-core", ModulePath: "example.com/core", ModuleDir: ".",
			TargetPackages: []string{"example.com/core"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	index := result.CoreObjectIndex
	if index == nil {
		t.Fatal("core object index is absent")
	}
	if err := index.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if index.Scope.TargetRef != "target-core" || len(index.Packages) != 1 ||
		index.Packages[0].Path != "example.com/core" {
		t.Fatalf("target scope = %#v packages=%#v", index.Scope, index.Packages)
	}
	wantTypes := map[string]gocoreobject.TypeKind{
		"ID": gocoreobject.TypeNamed, "Config": gocoreobject.TypeStruct,
		"Runner": gocoreobject.TypeInterface, "ConfigAlias": gocoreobject.TypeAlias,
	}
	if len(index.Types) != len(wantTypes) {
		t.Fatalf("types = %#v", index.Types)
	}
	for _, declaration := range index.Types {
		if wantTypes[declaration.Name] != declaration.Kind || declaration.Location.Path != "core.go" ||
			declaration.Location.Line <= 0 || declaration.Location.Column <= 0 {
			t.Fatalf("type declaration = %#v", declaration)
		}
		delete(wantTypes, declaration.Name)
	}
	if len(wantTypes) != 0 {
		t.Fatalf("missing types = %#v", wantTypes)
	}

	nodes := make(map[string]struct{}, len(result.DirectCallIndex.Nodes))
	for _, node := range result.DirectCallIndex.Nodes {
		nodes[node.ID] = struct{}{}
	}
	wantCallables := map[string]gocoreobject.CallableKind{
		"New": gocoreobject.CallableFunction, "Start": gocoreobject.CallableMethod,
		"hidden": gocoreobject.CallableFunction,
	}
	if len(index.Callables) != len(wantCallables) {
		t.Fatalf("callables = %#v", index.Callables)
	}
	for _, declaration := range index.Callables {
		if wantCallables[declaration.Name] != declaration.Kind || declaration.Location.Path != "core.go" ||
			!strings.HasPrefix(declaration.Signature, "func") {
			t.Fatalf("callable declaration = %#v", declaration)
		}
		if declaration.Name == "Start" && declaration.Receiver != "*example.com/core.Config" {
			t.Fatalf("method receiver = %q", declaration.Receiver)
		}
		if declaration.DirectCallNodeID == "" {
			t.Fatalf("callable has no exact DirectCallNode join: %#v", declaration)
		}
		if _, exists := nodes[declaration.DirectCallNodeID]; !exists {
			t.Fatalf("callable cites unknown DirectCallNode: %#v", declaration)
		}
		delete(wantCallables, declaration.Name)
	}
	if len(wantCallables) != 0 {
		t.Fatalf("missing callables = %#v", wantCallables)
	}
}
