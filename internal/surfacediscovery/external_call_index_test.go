package surfacediscovery

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestExternalCallIndexCaptureIsOptInAndIndependentFromTargetRoots(t *testing.T) {
	repository := t.TempDir()
	writeFixtureFile(t, filepath.Join(repository, "go.mod"), "module example.com/external-capture\n\ngo 1.25\n")
	writeFixtureFile(t, filepath.Join(repository, "main.go"), `package main

import (
	"fmt"
	"io"
	"strings"
)

var seeded = strings.TrimSpace(" seeded ")

func main() { reachable() }
func reachable() { fmt.Println(seeded) }
func unreachable() { strings.TrimSpace(" hidden ") }
func interfaceCall(writer io.Writer) { _, _ = writer.Write([]byte("payload")) }
`)
	input := targetDirectCallExecutableInput("example.com/external-capture", "main.go", 11)
	options := defaultHostOptions(repository)
	options.DirectCallDepth = 1

	without, err := analyzeForTest(options, input)
	if err != nil {
		t.Fatal(err)
	}
	if without.ExternalCallIndex != nil {
		t.Fatalf("no-consumer run retained external call index: %#v", without.ExternalCallIndex)
	}

	options.CaptureExternalCallIndex = true
	with, err := analyzeForTest(options, input)
	if err != nil {
		t.Fatal(err)
	}
	index := with.ExternalCallIndex
	if index == nil {
		t.Fatal("captured external call index is nil")
	}
	if err := index.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(index.Packages) != 1 || index.Packages[0].PackagePath != "example.com/external-capture" {
		t.Fatalf("loaded package inventory = %#v", index.Packages)
	}
	callers := make(map[string]string, len(index.Callers))
	for _, caller := range index.Callers {
		callers[caller.ID] = caller.Symbol.Name
	}
	found := map[string]bool{}
	for _, family := range index.Families {
		found[callers[family.CallerID]+"->"+family.Target.PackagePath+"."+family.Target.Name+"/"+string(family.Dispatch)] = true
	}
	if !found["reachable->fmt.Println/static"] || !found["unreachable->strings.TrimSpace/static"] ||
		!found["interfaceCall->io.Write/interface_invoke"] {
		t.Fatalf("root-independent external families = %#v callers=%#v", index.Families, callers)
	}
	if index.Coverage.ExternalInterfaceInvokeWitnesses != 1 {
		t.Fatalf("interface invoke coverage = %#v", index.Coverage)
	}
	if index.Coverage.SyntheticCallerWitnessesExcluded != 1 || len(index.PackageFrontiers) != 1 {
		t.Fatalf("package-init frontier = %#v coverage=%#v", index.PackageFrontiers, index.Coverage)
	}
	if without.DirectCallIndex == nil || with.DirectCallIndex == nil ||
		without.DirectCallIndex.SHA256 != with.DirectCallIndex.SHA256 {
		t.Fatalf("capture changed DirectCallIndex identity: without=%#v with=%#v", without.DirectCallIndex, with.DirectCallIndex)
	}
}

func TestExternalCallIndexBuildsCanonicalRootIndependentFacts(t *testing.T) {
	t.Parallel()

	scenario := Scenario{ID: "scenario-darwin-arm64", GOOS: "darwin", GOARCH: "arm64", Tags: []string{"prod"}}
	module := externalCallTestModule("example.com/app", ".")
	packages := []ExternalCallPackage{
		{ModuleID: module.ID, PackagePath: "example.com/app/worker"},
		{ModuleID: module.ID, PackagePath: "example.com/app/api"},
	}
	apiCaller := externalCallTestCaller(
		module, scenario, "example.com/app/api", "api/client.go", 10, "Send",
	)
	workerCaller := externalCallTestCaller(
		module, scenario, "example.com/app/worker", "worker/publish.go", 20, "Publish",
	)
	witnesses := []ExternalCallWitness{
		{Caller: apiCaller, Target: ExternalCallTarget{
			PackagePath: "github.com/acme/sdk", Receiver: "*Client", Name: "Do",
		}, Dispatch: ExternalCallStatic, Invocation: DirectCallSynchronous, Callsite: Location{Path: "api/client.go", Line: 15, Column: 2}},
		{Caller: workerCaller, Target: ExternalCallTarget{
			PackagePath: "github.com/acme/kafka", Receiver: "*Producer", Name: "Publish",
		}, Dispatch: ExternalCallStatic, Invocation: DirectCallGoroutine, Callsite: Location{Path: "worker/publish.go", Line: 25, Column: 3}},
		{Caller: apiCaller, Target: ExternalCallTarget{
			PackagePath: "github.com/acme/sdk", Receiver: "*Client", Name: "Do",
		}, Dispatch: ExternalCallStatic, Invocation: DirectCallSynchronous, Callsite: Location{Path: "api/client.go", Line: 12, Column: 2}},
		{Caller: apiCaller, Target: ExternalCallTarget{
			PackagePath: "github.com/acme/sdk", Receiver: "*Client", Name: "Do",
		}, Dispatch: ExternalCallStatic, Invocation: DirectCallSynchronous, Callsite: Location{Path: "api/client.go", Line: 14, Column: 2}},
		{Caller: apiCaller, Target: ExternalCallTarget{
			PackagePath: "github.com/acme/sdk", Receiver: "*Client", Name: "Do",
		}, Dispatch: ExternalCallStatic, Invocation: DirectCallSynchronous, Callsite: Location{Path: "api/client.go", Line: 13, Column: 2}},
		// A generic instantiation can retain another exact SSA witness at the
		// same source position. Witness accounting remains complete while the
		// representative callsite set stays unique.
		{Caller: apiCaller, Target: ExternalCallTarget{
			PackagePath: "github.com/acme/sdk", Receiver: "*Client", Name: "Do",
		}, Dispatch: ExternalCallStatic, Invocation: DirectCallSynchronous, Callsite: Location{Path: "api/client.go", Line: 12, Column: 2}},
	}

	index := externalCallTestBuild(t, scenario, []DirectCallModule{module}, packages, witnesses)
	if err := index.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(index.Packages) != 2 || len(index.Callers) != 2 || len(index.Families) != 2 {
		t.Fatalf("index shape = packages:%d callers:%d families:%d", len(index.Packages), len(index.Callers), len(index.Families))
	}
	var apiFamily ExternalCallFamily
	for _, family := range index.Families {
		if family.CallerID == apiCaller.ID {
			apiFamily = family
		}
	}
	if apiFamily.WitnessCount != 5 || len(apiFamily.Callsites) != 3 || apiFamily.CallsitesOmitted != 2 ||
		apiFamily.Callsites[0].Line != 12 || apiFamily.Callsites[1].Line != 13 || apiFamily.Callsites[2].Line != 14 {
		t.Fatalf("api family = %#v, want five witnesses and the first three canonical exact callsites", apiFamily)
	}
	if index.Coverage.ExternalStaticWitnesses != 6 || index.Coverage.RepresentativeCallsites != 4 ||
		index.Coverage.RepresentativeCallsitesOmitted != 2 || index.Coverage.DynamicInvokesExcluded != 2 ||
		index.Coverage.UnnamedStaticCalleesExcluded != 1 ||
		index.Coverage.SyntheticCallerWitnessesExcluded != 1 || len(index.PackageFrontiers) != 1 {
		t.Fatalf("coverage = %#v", index.Coverage)
	}

	snapshot := index.Snapshot()
	snapshot.Callers[0].Symbol.EquivalentIDs = append(snapshot.Callers[0].Symbol.EquivalentIDs, "changed")
	snapshot.Families[0].Callsites[0].Line = 999
	if reflect.DeepEqual(index, snapshot) {
		t.Fatal("snapshot mutation changed no owned data")
	}
	if err := index.Validate(); err != nil {
		t.Fatalf("snapshot mutation changed source index: %v", err)
	}
}

func TestExternalCallIndexRejectsCallerOutsideLoadedPackageAndTampering(t *testing.T) {
	t.Parallel()

	scenario := Scenario{ID: "scenario-linux-amd64", GOOS: "linux", GOARCH: "amd64", Tags: []string{}}
	module := externalCallTestModule("example.com/app", ".")
	packages := []ExternalCallPackage{{ModuleID: module.ID, PackagePath: "example.com/app/api"}}
	builder, err := NewExternalCallIndexBuilder(scenario, []DirectCallModule{module}, packages)
	if err != nil {
		t.Fatal(err)
	}
	outside := externalCallTestCaller(
		module, scenario, "example.com/app/other", "other/client.go", 10, "Send",
	)
	if err := builder.AddWitness(ExternalCallWitness{
		Caller: outside, Target: ExternalCallTarget{PackagePath: "example.com/sdk", Name: "Send"},
		Dispatch: ExternalCallStatic, Invocation: DirectCallSynchronous, Callsite: Location{Path: "other/client.go", Line: 12, Column: 2},
	}); err == nil {
		t.Fatal("caller outside the exact loaded package set was accepted")
	}

	caller := externalCallTestCaller(
		module, scenario, "example.com/app/api", "api/client.go", 10, "Send",
	)
	if err := builder.AddWitness(ExternalCallWitness{
		Caller: caller, Target: ExternalCallTarget{PackagePath: "example.com/sdk", Name: "Send"},
		Dispatch: ExternalCallStatic, Invocation: DirectCallSynchronous, Callsite: Location{Path: "api/client.go", Line: 12, Column: 2},
	}); err != nil {
		t.Fatal(err)
	}
	index, err := builder.Finish()
	if err != nil {
		t.Fatal(err)
	}
	tampered := index.Snapshot()
	tampered.Families[0].Target.Name = "Delete"
	if err := tampered.Validate(); err == nil {
		t.Fatal("tampered exact target was accepted")
	}
}

func externalCallTestBuild(
	t *testing.T,
	scenario Scenario,
	modules []DirectCallModule,
	packages []ExternalCallPackage,
	witnesses []ExternalCallWitness,
) ExternalCallIndex {
	t.Helper()
	builder, err := NewExternalCallIndexBuilder(scenario, modules, packages)
	if err != nil {
		t.Fatal(err)
	}
	for _, witness := range witnesses {
		if err := builder.AddWitness(witness); err != nil {
			t.Fatal(err)
		}
	}
	for _, exclusion := range []ExternalCallExclusion{{
		Caller: witnesses[0].Caller, DynamicInvokesExcluded: 2,
		UnnamedStaticCalleesExcluded: 1,
	}} {
		if err := builder.AddExclusion(exclusion); err != nil {
			t.Fatal(err)
		}
	}
	if len(packages) > 1 {
		if err := builder.addPackageExclusion(packages[0], true); err != nil {
			t.Fatal(err)
		}
	}
	index, err := builder.Finish()
	if err != nil {
		t.Fatal(err)
	}
	return index
}

func externalCallTestModule(path, directory string) DirectCallModule {
	module := DirectCallModule{Path: path, Directory: directory}
	module.ID = stableDirectCallID("direct-module", module.Path, module.Directory)
	return module
}

func externalCallTestCaller(
	module DirectCallModule,
	scenario Scenario,
	packagePath, path string,
	line int,
	name string,
) DirectCallNode {
	declaration := Location{Path: path, Line: line, Column: 1}
	node := DirectCallNode{
		Symbol: Symbol{
			ID: packagePath + "." + name, Package: packagePath, Name: name,
			Location: declaration, EquivalentIDs: []string{},
		},
		Package: packagePath, ModuleID: module.ID, ScenarioID: scenario.ID,
		Declaration: declaration,
		Body: DirectCallBodyRange{
			Start: declaration,
			End:   Location{Path: path, Line: line + 20, Column: 1},
		},
	}
	node.ID = stableDirectCallNodeID(node)
	return node
}
