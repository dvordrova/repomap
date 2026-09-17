package surfacediscovery

import (
	"go/build"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/gocoreobject"
)

func TestCoreObjectIndexKeepsAuthoredCgoDeclarationsAndSkipsGeneratedHelpers(t *testing.T) {
	if !build.Default.CgoEnabled {
		t.Skip("cgo is disabled")
	}
	repository := t.TempDir()
	writeTargetScopeFile(t, repository, "go.mod", "module example.com/cgocore\n\ngo 1.24\n")
	writeTargetScopeFile(t, repository, "core.go", `package cgocore

type Core struct{}
`)
	writeTargetScopeFile(t, repository, "cgo.go", `package cgocore

/*
typedef int repomap_int;
static repomap_int repomap_increment(repomap_int value) { return value + 1; }
*/
import "C"

type CValue C.repomap_int

func NewCValue(value C.repomap_int) CValue { return CValue(value) }
func Increment(value C.repomap_int) CValue { return CValue(C.repomap_increment(value)) }
`)

	options := defaultHostOptions(repository)
	options.CaptureCoreObjectIndex = true
	result, err := analyzeForTest(options, Input{
		ModuleDirs: []string{"."},
		Packages:   []PackageInput{{Path: "example.com/cgocore", ModuleDir: "."}},
		AnalysisTarget: &AnalysisTargetInput{
			TargetRef: "target-cgocore", Kind: AnalysisTargetModuleLibrary,
			ModuleID: "module-cgocore", ModulePath: "example.com/cgocore", ModuleDir: ".",
			TargetPackages: []string{"example.com/cgocore"},
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
		t.Fatal(err)
	}
	wantTypes := map[string]string{"Core": "core.go", "CValue": "cgo.go"}
	for _, declaration := range index.Types {
		if strings.HasPrefix(declaration.Name, "_C") {
			t.Fatalf("generated cgo type entered core object index: %#v", declaration)
		}
		if wantPath, ok := wantTypes[declaration.Name]; ok {
			if declaration.Location.Path != wantPath {
				t.Fatalf("authored type location = %#v, want %s", declaration.Location, wantPath)
			}
			delete(wantTypes, declaration.Name)
		}
	}
	if len(wantTypes) != 0 {
		t.Fatalf("missing authored cgo types = %#v", wantTypes)
	}
	wantCallables := map[string]bool{"NewCValue": true, "Increment": true}
	for _, declaration := range index.Callables {
		if strings.HasPrefix(declaration.Name, "_C") {
			t.Fatalf("generated cgo callable entered core object index: %#v", declaration)
		}
		if wantCallables[declaration.Name] {
			if declaration.Location.Path != "cgo.go" {
				t.Fatalf("authored cgo callable location = %#v", declaration.Location)
			}
			delete(wantCallables, declaration.Name)
		}
	}
	if len(wantCallables) != 0 {
		t.Fatalf("authored cgo callables are absent: %#v", wantCallables)
	}
}

func TestCoreObjectIndexStillRejectsExternalDeclarationInRepositorySyntax(t *testing.T) {
	repository := t.TempDir()
	writeTargetScopeFile(t, repository, "go.mod", "module example.com/coreline\n\ngo 1.24\n")
	writeTargetScopeFile(t, repository, "core.go", `package coreline

//line /outside-repository.go:1
type Escaped struct{}
`)

	options := defaultHostOptions(repository)
	options.CaptureCoreObjectIndex = true
	_, err := analyzeForTest(options, Input{
		ModuleDirs: []string{"."},
		Packages:   []PackageInput{{Path: "example.com/coreline", ModuleDir: "."}},
		AnalysisTarget: &AnalysisTargetInput{
			TargetRef: "target-coreline", Kind: AnalysisTargetModuleLibrary,
			ModuleID: "module-coreline", ModulePath: "example.com/coreline", ModuleDir: ".",
			TargetPackages: []string{"example.com/coreline"},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "declaration has no repository-local location") {
		t.Fatalf("repository syntax with external declaration error = %v", err)
	}
}

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
		"hidden": gocoreobject.CallableFunction, "Run": gocoreobject.CallableMethod,
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
		if declaration.Name == "Run" {
			if declaration.Receiver != "example.com/core.Runner" || declaration.DirectCallNodeID != "" {
				t.Fatalf("interface declaration lost ownership or invented implementation: %+v", declaration)
			}
			delete(wantCallables, declaration.Name)
			continue
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

func TestCoreObjectIndexOptionMatchesAllRepositoryInterfaceImplementations(t *testing.T) {
	repository := t.TempDir()
	writeTargetScopeFile(t, repository, "go.mod", "module example.com/interfaces\n\ngo 1.24\n")
	writeTargetScopeFile(t, repository, "interfaces.go", `package interfaces

type Reader interface{ Read() string }
type ValueReader struct{}
func (ValueReader) Read() string { return "value" }
type PointerReader struct{}
func (*PointerReader) Read() string { return "pointer" }
type WrongReader struct{}
func (WrongReader) Read(int) string { return "wrong" }
`)

	input := Input{
		MatchInterfaceImplementations: true,
		ModuleDirs:                    []string{"."},
		Packages:                      []PackageInput{{Path: "example.com/interfaces", ModuleDir: "."}},
		AnalysisTarget: &AnalysisTargetInput{
			TargetRef: "target-interfaces", Kind: AnalysisTargetModuleLibrary,
			ModuleID: "module-interfaces", ModulePath: "example.com/interfaces", ModuleDir: ".",
			TargetPackages: []string{"example.com/interfaces"},
		},
	}
	options := defaultHostOptions(repository)
	options.CaptureCoreObjectIndex = true
	result, err := analyzeForTest(options, input)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][2]bool{
		"ValueReader":   {true, true},
		"PointerReader": {false, true},
	}
	for _, match := range result.CoreObjectIndex.InterfaceImplementations {
		if match.InterfaceName != "Reader" || match.InterfacePackage != "example.com/interfaces" {
			t.Fatalf("unexpected interface match: %#v", match)
		}
		receivers, expected := want[match.ImplementationName]
		if !expected || match.ImplementationPackage != "example.com/interfaces" ||
			match.ValueReceiver != receivers[0] || match.PointerReceiver != receivers[1] {
			t.Fatalf("incorrect interface match: %#v", match)
		}
		delete(want, match.ImplementationName)
	}
	if len(want) != 0 {
		t.Fatalf("missing interface matches: %#v", want)
	}

	input.MatchInterfaceImplementations = false
	result, err = analyzeForTest(options, input)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.CoreObjectIndex.InterfaceImplementations) != 0 {
		t.Fatalf("disabled interface matches = %#v", result.CoreObjectIndex.InterfaceImplementations)
	}
}
