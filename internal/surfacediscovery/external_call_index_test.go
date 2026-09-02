package surfacediscovery

import (
	"fmt"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestExternalCallIndexCaptureIsOptInAndIndependentFromTargetRoots(t *testing.T) {
	repository := t.TempDir()
	writeFixtureFile(t, filepath.Join(repository, "go.mod"), "module example.com/external-capture\n\ngo 1.25\n")
	writeFixtureFile(t, filepath.Join(repository, "main.go"), `package main

import (
	"fmt"
	"io"
	"net/http"
	"strings"
)

var seeded = strings.TrimSpace(" seeded ")

func main() { reachable() }
func reachable() { fmt.Println(seeded) }
func unreachable() { strings.TrimSpace(" hidden ") }
func interfaceCall(writer io.Writer) { _, _ = writer.Write([]byte("payload")) }
func httpClient() { _, _ = http.Get("/api/levels") }
func closeIdle() { http.DefaultClient.CloseIdleConnections() }
func routes() {
	http.HandleFunc("/api/levels", getLevel)
	_ = http.ListenAndServe(":8080", http.DefaultServeMux)
}
func getLevel(http.ResponseWriter, *http.Request) {}
func values() func(func(string) bool) { return func(yield func(string) bool) { yield("nested") } }
func rangeOverFunc() { for value := range values() { strings.TrimSpace(value) } }
`)
	input := targetDirectCallExecutableInput("example.com/external-capture", "main.go", 12)
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
	if index.Coverage.RepresentativePatterns == 0 ||
		index.Coverage.RepresentativePatternsOmitted != index.Coverage.RepresentativeCallsitesOmitted {
		t.Fatalf("pattern coverage = %#v", index.Coverage)
	}
	familyFor := func(packagePath, name string) ExternalCallFamily {
		t.Helper()
		for _, family := range index.Families {
			if family.Target.PackagePath == packagePath && family.Target.Name == name {
				return family
			}
		}
		t.Fatalf("external family %s.%s is absent", packagePath, name)
		return ExternalCallFamily{}
	}
	getFamily := familyFor("net/http", "Get")
	if len(getFamily.Patterns) != 1 || getFamily.PatternsOmitted != 0 ||
		len(getFamily.Patterns[0].Arguments) != 1 ||
		getFamily.Patterns[0].Arguments[0].Kind != ExternalCallPatternLiteralString ||
		getFamily.Patterns[0].Arguments[0].Value != "/api/levels" {
		t.Fatalf("net/http.Get pattern = %#v", getFamily)
	}
	handleFamily := familyFor("net/http", "HandleFunc")
	if len(handleFamily.Patterns) != 1 || len(handleFamily.Patterns[0].Arguments) != 2 ||
		handleFamily.Patterns[0].Arguments[0].Value != "/api/levels" ||
		handleFamily.Patterns[0].Arguments[1].ObjectsObserved != 1 ||
		len(handleFamily.Patterns[0].Arguments[1].ObjectIDs) != 1 {
		t.Fatalf("net/http.HandleFunc pattern = %#v", handleFamily)
	}
	handlerID := handleFamily.Patterns[0].Arguments[1].ObjectIDs[0]
	handlerFound := false
	for _, node := range with.DirectCallIndex.Nodes {
		if node.ID == handlerID && node.Symbol.Name == "getLevel" {
			handlerFound = true
		}
	}
	if !handlerFound {
		t.Fatalf("handler argument does not retain exact local callable %q", handlerID)
	}
	writeFamily := familyFor("io", "Write")
	if len(writeFamily.Patterns) != 1 || len(writeFamily.Patterns[0].Arguments) != 1 ||
		writeFamily.Patterns[0].Arguments[0].Position != 1 {
		t.Fatalf("interface receiver leaked into source arguments: %#v", writeFamily)
	}
	closeFamily := familyFor("net/http", "CloseIdleConnections")
	if len(closeFamily.Patterns) != 1 || closeFamily.Patterns[0].Arguments == nil ||
		len(closeFamily.Patterns[0].Arguments) != 0 || closeFamily.Patterns[0].ArgumentsObserved != 0 {
		t.Fatalf("static method receiver leaked into source arguments: %#v", closeFamily)
	}
	bootstrapFamily := familyFor("net/http", "ListenAndServe")
	if len(bootstrapFamily.Patterns) != 1 || len(bootstrapFamily.Patterns[0].Arguments) != 2 ||
		bootstrapFamily.Patterns[0].Arguments[0].Value != ":8080" ||
		bootstrapFamily.Patterns[0].Arguments[1].Kind != ExternalCallPatternDynamic ||
		bootstrapFamily.Patterns[0].Arguments[1].ObjectsObserved != 0 {
		t.Fatalf("ListenAndServe neutral pattern = %#v", bootstrapFamily)
	}
	if index.Coverage.SyntheticCallerWitnessesExcluded != 2 || len(index.PackageFrontiers) != 1 {
		t.Fatalf("package-init frontier = %#v coverage=%#v", index.PackageFrontiers, index.Coverage)
	}
	directNodes := make(map[string]struct{}, len(with.DirectCallIndex.Nodes))
	for _, node := range with.DirectCallIndex.Nodes {
		directNodes[node.ID] = struct{}{}
	}
	for _, caller := range index.Callers {
		if _, ok := directNodes[caller.ID]; !ok {
			t.Fatalf("external caller %q is outside direct caller authority", caller.ID)
		}
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
	pattern := func(path string, line, column int) *ExternalCallPattern {
		callsite := Location{Path: path, Line: line, Column: column}
		return &ExternalCallPattern{
			ID: externalCallPatternID(callsite), Callsite: callsite,
			ReceiverResultIDs: []string{},
			Arguments: []ExternalCallPatternArgument{{
				Position: 1, Kind: ExternalCallPatternLiteralString, Value: "/api/levels",
				ObjectIDs: []string{},
			}}, ArgumentsObserved: 1,
		}
	}
	witnesses := []ExternalCallWitness{
		{Caller: apiCaller, Target: ExternalCallTarget{
			PackagePath: "github.com/acme/sdk", Receiver: "*Client", Name: "Do",
		}, Dispatch: ExternalCallStatic, Invocation: DirectCallSynchronous, Callsite: Location{Path: "api/client.go", Line: 15, Column: 2}, Pattern: pattern("api/client.go", 15, 2)},
		{Caller: workerCaller, Target: ExternalCallTarget{
			PackagePath: "github.com/acme/kafka", Receiver: "*Producer", Name: "Publish",
		}, Dispatch: ExternalCallStatic, Invocation: DirectCallGoroutine, Callsite: Location{Path: "worker/publish.go", Line: 25, Column: 3}, Pattern: pattern("worker/publish.go", 25, 3)},
		{Caller: apiCaller, Target: ExternalCallTarget{
			PackagePath: "github.com/acme/sdk", Receiver: "*Client", Name: "Do",
		}, Dispatch: ExternalCallStatic, Invocation: DirectCallSynchronous, Callsite: Location{Path: "api/client.go", Line: 12, Column: 2}, Pattern: pattern("api/client.go", 12, 2)},
		{Caller: apiCaller, Target: ExternalCallTarget{
			PackagePath: "github.com/acme/sdk", Receiver: "*Client", Name: "Do",
		}, Dispatch: ExternalCallStatic, Invocation: DirectCallSynchronous, Callsite: Location{Path: "api/client.go", Line: 14, Column: 2}, Pattern: pattern("api/client.go", 14, 2)},
		{Caller: apiCaller, Target: ExternalCallTarget{
			PackagePath: "github.com/acme/sdk", Receiver: "*Client", Name: "Do",
		}, Dispatch: ExternalCallStatic, Invocation: DirectCallSynchronous, Callsite: Location{Path: "api/client.go", Line: 13, Column: 2}, Pattern: pattern("api/client.go", 13, 2)},
		// A generic instantiation can retain another exact SSA witness at the
		// same source position. Witness accounting remains complete while the
		// representative callsite set stays unique.
		{Caller: apiCaller, Target: ExternalCallTarget{
			PackagePath: "github.com/acme/sdk", Receiver: "*Client", Name: "Do",
		}, Dispatch: ExternalCallStatic, Invocation: DirectCallSynchronous, Callsite: Location{Path: "api/client.go", Line: 12, Column: 2}, Pattern: pattern("api/client.go", 12, 2)},
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
	if apiFamily.WitnessCount != 5 || len(apiFamily.Callsites) != 4 || apiFamily.CallsitesOmitted != 0 ||
		len(apiFamily.Patterns) != 4 || apiFamily.PatternsObserved != 4 || apiFamily.PatternsOmitted != 0 ||
		apiFamily.Callsites[0].Line != 12 || apiFamily.Callsites[1].Line != 13 ||
		apiFamily.Callsites[2].Line != 14 || apiFamily.Callsites[3].Line != 15 {
		t.Fatalf("api family = %#v, want five SSA witnesses and four complete source callsites", apiFamily)
	}
	if index.Coverage.ExternalStaticWitnesses != 6 || index.Coverage.RepresentativeCallsites != 5 ||
		index.Coverage.RepresentativeCallsitesOmitted != 0 || index.Coverage.DynamicInvokesExcluded != 2 ||
		index.Coverage.RepresentativePatterns != 5 || index.Coverage.RepresentativePatternsOmitted != 0 ||
		index.Coverage.UnnamedStaticCalleesExcluded != 1 ||
		index.Coverage.SyntheticCallerWitnessesExcluded != 1 || len(index.PackageFrontiers) != 1 {
		t.Fatalf("coverage = %#v", index.Coverage)
	}

	snapshot := index.Snapshot()
	snapshot.Callers[0].Symbol.EquivalentIDs = append(snapshot.Callers[0].Symbol.EquivalentIDs, "changed")
	snapshot.Families[0].Callsites[0].Line = 999
	snapshot.Families[0].Patterns[0].Arguments[0].ObjectIDs = append(
		snapshot.Families[0].Patterns[0].Arguments[0].ObjectIDs, "changed",
	)
	if reflect.DeepEqual(index, snapshot) {
		t.Fatal("snapshot mutation changed no owned data")
	}
	if err := index.Validate(); err != nil {
		t.Fatalf("snapshot mutation changed source index: %v", err)
	}
}

func TestCallPatternFamiliesRetainAllSourceDistinctRows(t *testing.T) {
	externalPatterns := []ExternalCallPattern{}
	directPatterns := []ExternalCallPattern{}
	for position := 1; position <= 70; position++ {
		callsite := Location{Path: "api/routes.go", Line: position, Column: 2}
		pattern := ExternalCallPattern{
			ID: externalCallPatternID(callsite), Callsite: callsite,
			ReceiverResultIDs: []string{}, Arguments: []ExternalCallPatternArgument{},
		}
		externalPatterns = appendExternalCallPattern(externalPatterns, pattern)
		directPatterns = appendDirectCallPattern(directPatterns, pattern)
	}
	if len(externalPatterns) != 70 || len(directPatterns) != 70 {
		t.Fatalf("source-distinct pattern retention = external:%d direct:%d, want 70 each",
			len(externalPatterns), len(directPatterns))
	}
}

func TestCallPatternsRetainArgumentsObjectsAndTextBeyondFormerLocalThresholds(t *testing.T) {
	const (
		formerArguments  = 128
		formerObjectRefs = 64
		formerTextBytes  = 16 * 1024
	)
	objectIDs := make([]string, formerObjectRefs+1)
	for position := range objectIDs {
		objectIDs[position] = fmt.Sprintf("object-%03d", position)
	}
	arguments := make([]ExternalCallPatternArgument, formerArguments+1)
	for position := range arguments {
		arguments[position] = ExternalCallPatternArgument{
			Position: position + 1, Kind: ExternalCallPatternDynamic, ObjectIDs: []string{},
		}
	}
	arguments[0].ObjectIDs = objectIDs
	arguments[0].ObjectsObserved = len(objectIDs)
	longLiteral := strings.Repeat("x", formerTextBytes+1)
	arguments[1].Kind = ExternalCallPatternLiteralString
	arguments[1].Value = longLiteral
	callsite := Location{Path: "api/routes.go", Line: 1, Column: 2}
	pattern := ExternalCallPattern{
		ID: externalCallPatternID(callsite), Callsite: callsite,
		ReceiverResultIDs: []string{}, Arguments: arguments, ArgumentsObserved: len(arguments),
	}
	if err := validateExternalCallPattern(pattern); err != nil {
		t.Fatalf("validate complete pattern: %v", err)
	}
	directPatterns := appendDirectCallPattern(nil, pattern)
	if len(directPatterns) != 1 || len(directPatterns[0].Arguments) != formerArguments+1 ||
		len(directPatterns[0].Arguments[0].ObjectIDs) != formerObjectRefs+1 ||
		directPatterns[0].Arguments[1].Value != longLiteral {
		t.Fatalf("direct pattern changed exact data: %#v", directPatterns)
	}

	scenario := Scenario{ID: "scenario-linux-amd64", GOOS: "linux", GOARCH: "amd64", Tags: []string{}}
	module := externalCallTestModule("example.com/app", ".")
	packages := []ExternalCallPackage{{ModuleID: module.ID, PackagePath: "example.com/app/api"}}
	caller := externalCallTestCaller(module, scenario, packages[0].PackagePath, "api/routes.go", 1, "Routes")
	index := externalCallTestBuild(t, scenario, []DirectCallModule{module}, packages, []ExternalCallWitness{{
		Caller: caller, Target: ExternalCallTarget{PackagePath: "example.com/sdk", Name: "Register"},
		Dispatch: ExternalCallStatic, Invocation: DirectCallSynchronous,
		Callsite: callsite, Pattern: &pattern,
	}})
	retained := index.Families[0].Patterns[0]
	if len(retained.Arguments) != formerArguments+1 || retained.ArgumentsObserved != formerArguments+1 ||
		retained.ArgumentsOmitted != 0 || len(retained.Arguments[0].ObjectIDs) != formerObjectRefs+1 ||
		retained.Arguments[0].ObjectsOmitted != 0 || retained.Arguments[1].Value != longLiteral ||
		index.Coverage.RepresentativePatternsOmitted != 0 {
		t.Fatalf("external pattern or coverage changed exact data: pattern=%#v coverage=%#v", retained, index.Coverage)
	}
}

func TestGoCallCaptureRetainsLiteralBeyondFormerLocalTextThreshold(t *testing.T) {
	const formerTextBytes = 16 * 1024
	longLiteral := "/" + strings.Repeat("x", formerTextBytes+1)
	repository := t.TempDir()
	writeFixtureFile(t, filepath.Join(repository, "go.mod"), "module example.com/long-pattern\n\ngo 1.25\n")
	writeFixtureFile(t, filepath.Join(repository, "main.go"), fmt.Sprintf(`package main

import "net/http"

func main() {
	consume(%s)
	_, _ = http.Get(%s)
}

func consume(string) {}
`, strconv.Quote(longLiteral), strconv.Quote(longLiteral)))
	input := targetDirectCallExecutableInput("example.com/long-pattern", "main.go", 5)
	options := defaultHostOptions(repository)
	options.DirectCallDepth = 1
	options.CaptureExternalCallIndex = true
	result, err := analyzeForTest(options, input)
	if err != nil {
		t.Fatal(err)
	}
	if result.ExternalCallIndex == nil || result.DirectCallIndex == nil {
		t.Fatalf("missing call indexes: external=%#v direct=%#v", result.ExternalCallIndex, result.DirectCallIndex)
	}
	externalFound := false
	for _, family := range result.ExternalCallIndex.Families {
		if family.Target.PackagePath == "net/http" && family.Target.Name == "Get" &&
			len(family.Patterns) == 1 && len(family.Patterns[0].Arguments) == 1 &&
			family.Patterns[0].Arguments[0].Value == longLiteral && family.PatternsOmitted == 0 {
			externalFound = true
		}
	}
	directFound := false
	for _, edge := range result.DirectCallIndex.Edges {
		if len(edge.Patterns) == 1 && len(edge.Patterns[0].Arguments) == 1 &&
			edge.Patterns[0].Arguments[0].Value == longLiteral && edge.PatternsOmitted == 0 {
			directFound = true
		}
	}
	if !externalFound || !directFound {
		t.Fatalf("long literal retention: external=%v direct=%v", externalFound, directFound)
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
