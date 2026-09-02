package goadapter

import (
	"context"
	"fmt"
	"go/build"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/gitfiles"
	"github.com/dvordrova/repomap/internal/gocoreobject"
	"github.com/dvordrova/repomap/internal/godynamichandoff"
	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/surfacediscovery"
)

func TestBuildRetainsExactCgoGeneratedWrapperBoundary(t *testing.T) {
	if !build.Default.CgoEnabled {
		t.Skip("cgo is disabled")
	}
	repositoryRoot := t.TempDir()
	repository := writeGoAdapterCorpus(t, repositoryRoot, map[string]string{
		"go.mod": "module example.com/program\n\ngo 1.24\n",
		"cgo.go": `package program

/*
typedef int repomap_int;
static repomap_int repomap_increment(repomap_int value) { return value + 1; }
*/
import "C"

type CValue C.repomap_int

func Increment(value C.repomap_int) CValue {
	return CValue(C.repomap_increment(value))
}
`,
	})
	target := goAdapterLibraryTarget(t)
	options := surfacediscovery.DefaultOptions(repositoryRoot, runtime.GOOS+"/"+runtime.GOARCH)
	options.CaptureCoreObjectIndex = true
	options.CaptureExternalCallIndex = true
	options.CaptureDynamicHandoffIndex = true
	result, err := surfacediscovery.AnalyzeContextWithInput(t.Context(), options, surfacediscovery.Input{
		ModuleDirs: []string{"."},
		Packages:   []surfacediscovery.PackageInput{{Path: "example.com/program", ModuleDir: "."}},
		AnalysisTarget: &surfacediscovery.AnalysisTargetInput{
			TargetRef: target.Ref, Kind: string(target.Kind),
			ModuleID: target.ModuleID, ModulePath: target.ModulePath, ModuleDir: target.ModuleDir,
			TargetPackages: []string{"example.com/program"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.DirectCallIndex == nil || result.ExternalCallIndex == nil ||
		result.CoreObjectIndex == nil || result.DynamicHandoffIndex == nil {
		t.Fatal("cgo producer indexes are absent")
	}
	if result.DirectCallIndex.Coverage.InvalidEndpointCallsExcluded != 0 ||
		result.DirectCallIndex.Coverage.NonRepositoryCallsExcluded != 1 {
		t.Fatalf("cgo direct-call coverage = %#v", result.DirectCallIndex.Coverage)
	}
	if len(result.ExternalCallIndex.Families) != 1 ||
		result.ExternalCallIndex.Coverage.ExternalStaticWitnesses != 1 ||
		len(result.ExternalCallIndex.Frontiers) != 0 || len(result.ExternalCallIndex.PackageFrontiers) != 0 {
		t.Fatalf("cgo external boundary = %#v", result.ExternalCallIndex)
	}

	index, err := Build(
		repository, target, goAdapterPackageOrigins(t, *result.ExternalCallIndex),
		*result.DirectCallIndex, *result.ExternalCallIndex,
		*result.CoreObjectIndex, *result.DynamicHandoffIndex,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := index.Validate(); err != nil {
		t.Fatal(err)
	}
	encoded, err := programindex.Encode(index)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{repositoryRoot, "go-build", "_cgo_gotypes.go"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("cgo ProgramIndex leaked generated path %q", forbidden)
		}
	}
	var caller, boundary programindex.Object
	for _, object := range index.Objects {
		if strings.HasPrefix(object.Name, "_C") {
			t.Fatalf("generated cgo helper leaked as repository source: %#v", object)
		}
		switch object.Name {
		case "Increment":
			caller = object
		case "C.repomap_increment":
			boundary = object
		}
	}
	if caller.Kind != programindex.ObjectFunction || caller.Location == nil || caller.Location.Path != "cgo.go" {
		t.Fatalf("authored cgo caller = %#v", caller)
	}
	if boundary.Kind != programindex.ObjectExternalSymbol || boundary.Location != nil ||
		boundary.OwnerID != "" || boundary.ContainerID != "" || boundary.Visibility != programindex.VisibilityUnknown ||
		boundary.External == nil || boundary.External.PackagePath != surfacediscovery.ExternalCallCgoPackagePath ||
		boundary.External.AuthorityKind != programindex.ExternalAuthorityPlatform ||
		boundary.External.Name != "repomap_increment" || len(boundary.SymbolLinkIdentities) != 0 {
		t.Fatalf("generated cgo boundary object = %#v", boundary)
	}
	foundBoundary := false
	for _, relation := range index.Relations {
		if relation.Kind != programindex.RelationInvokesExternal || relation.FromID != caller.ID ||
			len(relation.ToIDs) != 1 || relation.ToIDs[0] != boundary.ID {
			continue
		}
		foundBoundary = relation.Resolution == programindex.ResolutionExact &&
			relation.Invocation == "generated_cgo_wrapper:synchronous" &&
			relation.Location != nil && relation.Location.Path == "cgo.go" &&
			relation.TargetsObserved == 1 && relation.TargetsOmitted == 0 &&
			relation.WitnessesObserved == 1 && relation.WitnessesOmitted == 0 &&
			len(relation.Witnesses) == 1 &&
			relation.Witnesses[0].Kind == "go_generated_cgo_wrapper_call" &&
			relation.Witnesses[0].Location != nil && relation.Witnesses[0].Location.Path == "cgo.go"
	}
	if !foundBoundary || index.Coverage.RelationsOmitted != 0 {
		t.Fatalf("cgo ProgramIndex boundary missing or incomplete: coverage=%#v relations=%#v", index.Coverage, index.Relations)
	}
}

func TestBuildProjectsNeutralExternalCallPatterns(t *testing.T) {
	repositoryRoot := t.TempDir()
	repository := writeGoAdapterCorpus(t, repositoryRoot, map[string]string{
		"go.mod": "module example.com/program\n\ngo 1.24\n",
		"program.go": `package program

import "net/http"

func New() {}
func Client() { _, _ = http.Get("/api/levels") }
func Routes() {
	http.HandleFunc("/api/levels", GetLevel)
	_ = http.ListenAndServe(":8080", http.DefaultServeMux)
}
func GetLevel(http.ResponseWriter, *http.Request) {}
`,
	})
	target := goAdapterLibraryTarget(t)
	options := surfacediscovery.DefaultOptions(repositoryRoot, runtime.GOOS+"/"+runtime.GOARCH)
	options.CaptureCoreObjectIndex = true
	options.CaptureExternalCallIndex = true
	options.CaptureDynamicHandoffIndex = true
	result, err := surfacediscovery.AnalyzeContextWithInput(t.Context(), options, surfacediscovery.Input{
		ModuleDirs: []string{"."},
		Packages:   []surfacediscovery.PackageInput{{Path: "example.com/program", ModuleDir: "."}},
		AnalysisTarget: &surfacediscovery.AnalysisTargetInput{
			TargetRef: target.Ref, Kind: string(target.Kind),
			ModuleID: target.ModuleID, ModulePath: target.ModulePath, ModuleDir: target.ModuleDir,
			TargetPackages: []string{"example.com/program"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.DirectCallIndex == nil || result.ExternalCallIndex == nil ||
		result.CoreObjectIndex == nil || result.DynamicHandoffIndex == nil {
		t.Fatal("Go pattern producer indexes are absent")
	}
	index, err := Build(
		repository, target, goAdapterPackageOrigins(t, *result.ExternalCallIndex),
		*result.DirectCallIndex, *result.ExternalCallIndex,
		*result.CoreObjectIndex, *result.DynamicHandoffIndex,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := index.Validate(); err != nil {
		t.Fatal(err)
	}
	if _, err := Build(
		repository, target, nil, *result.DirectCallIndex, *result.ExternalCallIndex,
		*result.CoreObjectIndex, *result.DynamicHandoffIndex,
	); err == nil || !strings.Contains(err.Error(), "external target package \"net/http\" has no exact go-list origin authority") {
		t.Fatalf("Build accepted missing external package-origin authority: %v", err)
	}
	objects := make(map[string]programindex.Object, len(index.Objects))
	var handler programindex.Object
	for _, object := range index.Objects {
		objects[object.ID] = object
		if object.Kind == programindex.ObjectFunction && object.Name == "GetLevel" {
			handler = object
		}
	}
	if handler.ID == "" {
		t.Fatal("exact route handler object is absent")
	}
	relationFor := func(name string) programindex.Relation {
		t.Helper()
		for _, relation := range index.Relations {
			if relation.Kind != programindex.RelationInvokesExternal || len(relation.ToIDs) != 1 {
				continue
			}
			targetObject := objects[relation.ToIDs[0]]
			if targetObject.External != nil && targetObject.External.PackagePath == "net/http" &&
				targetObject.External.Name == name {
				if targetObject.External.AuthorityKind != programindex.ExternalAuthorityPlatform {
					t.Fatalf("net/http.%s authority = %#v", name, targetObject.External)
				}
				return relation
			}
		}
		t.Fatalf("net/http.%s relation is absent", name)
		return programindex.Relation{}
	}
	get := relationFor("Get")
	if get.PatternsObserved != 1 || get.PatternsOmitted != 0 || len(get.Patterns) != 1 ||
		get.Patterns[0].Form != programindex.PatternCall || get.Patterns[0].Selector != "Get" ||
		len(get.Patterns[0].Arguments) != 1 ||
		get.Patterns[0].Arguments[0].Kind != programindex.PatternLiteralString ||
		get.Patterns[0].Arguments[0].Value != "/api/levels" {
		t.Fatalf("net/http.Get ProgramIndex pattern = %#v", get)
	}
	route := relationFor("HandleFunc")
	if len(route.Patterns) != 1 || route.Patterns[0].Selector != "HandleFunc" ||
		len(route.Patterns[0].Arguments) != 2 ||
		route.Patterns[0].Arguments[0].Value != "/api/levels" ||
		route.Patterns[0].Arguments[1].Resolution != programindex.ResolutionExact ||
		len(route.Patterns[0].Arguments[1].ObjectIDs) != 1 ||
		route.Patterns[0].Arguments[1].ObjectIDs[0] != handler.ID {
		t.Fatalf("net/http.HandleFunc ProgramIndex pattern = %#v", route)
	}
	bootstrap := relationFor("ListenAndServe")
	if len(bootstrap.Patterns) != 1 || bootstrap.Patterns[0].Selector != "ListenAndServe" ||
		len(bootstrap.Patterns[0].Arguments) != 2 ||
		bootstrap.Patterns[0].Arguments[0].Value != ":8080" ||
		bootstrap.Patterns[0].Arguments[1].Kind != programindex.PatternDynamic ||
		bootstrap.Patterns[0].Arguments[1].Resolution != "" ||
		bootstrap.Patterns[0].Arguments[1].ObjectsObserved != 0 {
		t.Fatalf("ListenAndServe ProgramIndex pattern = %#v", bootstrap)
	}
}

func TestBuildRetainsKnownPartialValueFlowAndOmitsOnlyOpenFrontiers(t *testing.T) {
	repositoryRoot := t.TempDir()
	repository := writeGoAdapterCorpus(t, repositoryRoot, map[string]string{
		"go.mod": "module example.com/program\n\ngo 1.24\n",
		"program.go": `package program

import (
	"bytes"
	"net/http"
)

func KnownHandler(http.ResponseWriter, *http.Request) {}
func KnownCallback() {}
func Register(func()) {}

func Partial(
	flag bool,
	unknownHandler http.HandlerFunc,
	unknownCallback func(),
	makeBuffer func() *bytes.Buffer,
) {
	handler := unknownHandler
	if flag { handler = KnownHandler }
	http.HandleFunc("/partial", handler)

	callback := unknownCallback
	if flag { callback = KnownCallback }
	callback()
	Register(callback)

	buffer := bytes.NewBufferString("known")
	if flag { buffer = makeBuffer() }
	_ = buffer.String()
}
`,
	})
	target := goAdapterLibraryTarget(t)
	options := surfacediscovery.DefaultOptions(repositoryRoot, runtime.GOOS+"/"+runtime.GOARCH)
	options.CaptureCoreObjectIndex = true
	options.CaptureExternalCallIndex = true
	options.CaptureDynamicHandoffIndex = true
	result, err := surfacediscovery.AnalyzeContextWithInput(t.Context(), options, surfacediscovery.Input{
		ModuleDirs: []string{"."},
		Packages:   []surfacediscovery.PackageInput{{Path: "example.com/program", ModuleDir: "."}},
		AnalysisTarget: &surfacediscovery.AnalysisTargetInput{
			TargetRef: target.Ref, Kind: string(target.Kind),
			ModuleID: target.ModuleID, ModulePath: target.ModulePath, ModuleDir: target.ModuleDir,
			TargetPackages: []string{"example.com/program"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.DirectCallIndex == nil || result.ExternalCallIndex == nil || result.CoreObjectIndex == nil ||
		result.DynamicHandoffIndex == nil {
		t.Fatal("known-partial producer indexes are absent")
	}

	partialHandoffs := 0
	for _, handoff := range result.DynamicHandoffIndex.Handoffs {
		if handoff.Resolution != godynamichandoff.ResolutionAlternatives || len(handoff.Candidates) != 1 {
			continue
		}
		partialHandoffs++
		if handoff.CandidatesConsidered != 2 || handoff.CandidatesOmitted != 1 {
			t.Fatalf("producer known-partial accounting = %#v", handoff)
		}
	}
	if partialHandoffs < 3 || result.DynamicHandoffIndex.Coverage.CandidatesOmitted < partialHandoffs {
		t.Fatalf("producer known-partial handoffs = %d, coverage = %#v",
			partialHandoffs, result.DynamicHandoffIndex.Coverage)
	}

	var producerArgument, producerReceiver bool
	for _, family := range result.ExternalCallIndex.Families {
		if family.Target.PackagePath == "net/http" && family.Target.Name == "HandleFunc" && len(family.Patterns) == 1 {
			arguments := family.Patterns[0].Arguments
			producerArgument = len(arguments) == 2 && len(arguments[1].ObjectIDs) == 1 &&
				arguments[1].ObjectsObserved == 2 && arguments[1].ObjectsOmitted == 1
		}
		if family.Target.PackagePath == "bytes" && family.Target.Name == "String" && len(family.Patterns) == 1 {
			pattern := family.Patterns[0]
			producerReceiver = len(pattern.ReceiverResultIDs) == 1 &&
				pattern.ReceiversObserved == 2 && pattern.ReceiversOmitted == 1
		}
	}
	if !producerArgument || !producerReceiver {
		t.Fatalf("producer partial patterns: argument=%v receiver=%v families=%#v",
			producerArgument, producerReceiver, result.ExternalCallIndex.Families)
	}

	index, err := Build(
		repository, target, goAdapterPackageOrigins(t, *result.ExternalCallIndex),
		*result.DirectCallIndex, *result.ExternalCallIndex,
		*result.CoreObjectIndex, *result.DynamicHandoffIndex,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := index.Validate(); err != nil {
		t.Fatal(err)
	}
	objects := make(map[string]programindex.Object, len(index.Objects))
	objectsByName := make(map[string]programindex.Object, len(index.Objects))
	for _, object := range index.Objects {
		objects[object.ID] = object
		if object.Kind == programindex.ObjectFunction {
			objectsByName[object.Name] = object
		}
	}
	partial := objectsByName["Partial"]
	knownHandler := objectsByName["KnownHandler"]
	knownCallback := objectsByName["KnownCallback"]
	if partial.ID == "" || knownHandler.ID == "" || knownCallback.ID == "" {
		t.Fatalf("known-partial local objects = %#v", objectsByName)
	}

	var argumentRetained, receiverRetained, valueCallRetained, callbackRetained, handlerTransferRetained bool
	for _, relation := range index.Relations {
		if relation.Kind == programindex.RelationInvokesExternal && len(relation.ToIDs) == 1 &&
			len(relation.Patterns) == 1 {
			targetObject := objects[relation.ToIDs[0]]
			if targetObject.External != nil && targetObject.External.PackagePath == "net/http" &&
				targetObject.External.Name == "HandleFunc" {
				arguments := relation.Patterns[0].Arguments
				argumentRetained = len(arguments) == 2 && arguments[1].Resolution == programindex.ResolutionAlternatives &&
					len(arguments[1].ObjectIDs) == 1 && arguments[1].ObjectIDs[0] == knownHandler.ID &&
					arguments[1].ObjectsObserved == 2 && arguments[1].ObjectsOmitted == 1
			}
			if targetObject.External != nil && targetObject.External.PackagePath == "bytes" &&
				targetObject.External.Name == "String" {
				pattern := relation.Patterns[0]
				receiverRetained = pattern.ReceiverID == "" &&
					pattern.ReceiverOriginResolution == programindex.ResolutionAlternatives &&
					len(pattern.ReceiverOriginIDs) == 1 && pattern.ReceiverOriginsObserved == 2 &&
					pattern.ReceiverOriginsOmitted == 1
			}
		}
		if relation.FromID != partial.ID || relation.Resolution != programindex.ResolutionAlternatives ||
			len(relation.ToIDs) != 1 || relation.TargetsObserved != 2 || relation.TargetsOmitted != 1 {
			continue
		}
		switch {
		case relation.Kind == programindex.RelationCalls && relation.ToIDs[0] == knownCallback.ID &&
			relation.Invocation == "function_value_call:synchronous":
			valueCallRetained = true
		case relation.Kind == programindex.RelationPassesCallback && relation.ToIDs[0] == knownCallback.ID &&
			relation.Invocation == "callback_transfer:synchronous":
			callbackRetained = true
		case relation.Kind == programindex.RelationPassesCallback && relation.ToIDs[0] == knownHandler.ID &&
			relation.Invocation == "callback_transfer:synchronous":
			handlerTransferRetained = true
		}
	}
	if !argumentRetained || !receiverRetained || !valueCallRetained || !callbackRetained || !handlerTransferRetained {
		t.Fatalf(
			"known-partial ProgramIndex: argument=%v receiver=%v value=%v callback=%v handler=%v relations=%#v",
			argumentRetained, receiverRetained, valueCallRetained, callbackRetained,
			handlerTransferRetained, index.Relations,
		)
	}
}

func TestBuildRetainsKnownPartialCandidatesBeyondFormerProducerThreshold(t *testing.T) {
	const formerCandidatesPerHandoff = 32
	var source strings.Builder
	source.WriteString("package program\n\nimport \"net/http\"\n\n")
	for index := 0; index <= formerCandidatesPerHandoff; index++ {
		_, _ = fmt.Fprintf(&source, "func Handler%02d(http.ResponseWriter, *http.Request) {}\n", index)
	}
	source.WriteString("\nfunc Wide(choice int, unknownA, unknownB http.HandlerFunc) {\n\thandler := unknownA\n\tif choice < 0 { handler = unknownB }\n\tswitch choice {\n")
	for index := 0; index <= formerCandidatesPerHandoff; index++ {
		_, _ = fmt.Fprintf(&source, "\tcase %d: handler = Handler%02d\n", index, index)
	}
	source.WriteString("\t}\n\thttp.HandleFunc(\"/wide\", handler)\n}\n")

	repositoryRoot := t.TempDir()
	repository := writeGoAdapterCorpus(t, repositoryRoot, map[string]string{
		"go.mod":     "module example.com/program\n\ngo 1.24\n",
		"program.go": source.String(),
	})
	target := goAdapterLibraryTarget(t)
	options := surfacediscovery.DefaultOptions(repositoryRoot, runtime.GOOS+"/"+runtime.GOARCH)
	options.CaptureCoreObjectIndex = true
	options.CaptureExternalCallIndex = true
	options.CaptureDynamicHandoffIndex = true
	result, err := surfacediscovery.AnalyzeContextWithInput(t.Context(), options, surfacediscovery.Input{
		ModuleDirs: []string{"."},
		Packages:   []surfacediscovery.PackageInput{{Path: "example.com/program", ModuleDir: "."}},
		AnalysisTarget: &surfacediscovery.AnalysisTargetInput{
			TargetRef: target.Ref, Kind: string(target.Kind),
			ModuleID: target.ModuleID, ModulePath: target.ModulePath, ModuleDir: target.ModuleDir,
			TargetPackages: []string{"example.com/program"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.DynamicHandoffIndex == nil || result.ExternalCallIndex == nil || result.DirectCallIndex == nil ||
		result.CoreObjectIndex == nil {
		t.Fatal("wide known-partial producer indexes are absent")
	}
	producerCandidates := formerCandidatesPerHandoff + 1
	producerRetained := false
	for _, handoff := range result.DynamicHandoffIndex.Handoffs {
		if handoff.Kind == godynamichandoff.CallbackTransfer &&
			handoff.Resolution == godynamichandoff.ResolutionAlternatives &&
			len(handoff.Candidates) == producerCandidates &&
			handoff.CandidatesConsidered == producerCandidates+2 && handoff.CandidatesOmitted == 2 {
			producerRetained = true
		}
	}
	if !producerRetained {
		t.Fatalf("wide producer handoffs = %#v", result.DynamicHandoffIndex.Handoffs)
	}

	index, err := Build(
		repository, target, goAdapterPackageOrigins(t, *result.ExternalCallIndex),
		*result.DirectCallIndex, *result.ExternalCallIndex,
		*result.CoreObjectIndex, *result.DynamicHandoffIndex,
	)
	if err != nil {
		t.Fatal(err)
	}
	objects := make(map[string]programindex.Object, len(index.Objects))
	objectsByName := make(map[string]programindex.Object, len(index.Objects))
	for _, object := range index.Objects {
		objects[object.ID] = object
		objectsByName[object.Name] = object
	}
	wide := objectsByName["Wide"]
	knownIDs := make(map[string]struct{}, producerCandidates)
	for index := 0; index < producerCandidates; index++ {
		object := objectsByName[fmt.Sprintf("Handler%02d", index)]
		if object.ID == "" {
			t.Fatalf("missing Handler%02d object", index)
		}
		knownIDs[object.ID] = struct{}{}
	}
	var patternRetained, transferRetained bool
	for _, relation := range index.Relations {
		if relation.Kind == programindex.RelationInvokesExternal && len(relation.ToIDs) == 1 &&
			len(relation.Patterns) == 1 {
			targetObject := objects[relation.ToIDs[0]]
			if targetObject.External != nil && targetObject.External.PackagePath == "net/http" &&
				targetObject.External.Name == "HandleFunc" {
				arguments := relation.Patterns[0].Arguments
				patternRetained = len(arguments) == 2 &&
					arguments[1].Resolution == programindex.ResolutionAlternatives &&
					len(arguments[1].ObjectIDs) == producerCandidates &&
					arguments[1].ObjectsObserved == producerCandidates+2 && arguments[1].ObjectsOmitted == 2
				for _, objectID := range arguments[1].ObjectIDs {
					if _, known := knownIDs[objectID]; !known {
						patternRetained = false
					}
				}
			}
		}
		if relation.Kind == programindex.RelationPassesCallback && relation.FromID == wide.ID &&
			relation.Resolution == programindex.ResolutionAlternatives &&
			len(relation.ToIDs) == producerCandidates && relation.TargetsObserved == producerCandidates+2 &&
			relation.TargetsOmitted == 2 {
			transferRetained = true
			for _, objectID := range relation.ToIDs {
				if _, known := knownIDs[objectID]; !known {
					transferRetained = false
				}
			}
		}
	}
	if !patternRetained || !transferRetained {
		t.Fatalf("wide ProgramIndex retention: pattern=%v transfer=%v relations=%#v",
			patternRetained, transferRetained, index.Relations)
	}
}

func TestBuildProjectsChiMethodPatternWithoutFrameworkKnowledge(t *testing.T) {
	workspaceRoot := t.TempDir()
	repositoryRoot := filepath.Join(workspaceRoot, "repository")
	chiRoot := filepath.Join(workspaceRoot, "chi")
	if err := os.MkdirAll(chiRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"go.mod": "module github.com/go-chi/chi/v5\n\ngo 1.24\n",
		"chi.go": `package chi
type Mux struct{}
type Route struct{}
func (*Mux) Get(pattern string, handler func()) {}
func (*Mux) HandleFunc(pattern string, handler func()) *Route { return &Route{} }
func (route *Route) Methods(methods ...string) *Route { return route }
`,
	} {
		if err := os.WriteFile(filepath.Join(chiRoot, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	repository := writeGoAdapterCorpus(t, repositoryRoot, map[string]string{
		"go.mod": `module example.com/program

go 1.24

require github.com/go-chi/chi/v5 v5.0.0
replace github.com/go-chi/chi/v5 => ../chi
`,
		"program.go": `package program

import chi "github.com/go-chi/chi/v5"

func New() {}
func Routes(router *chi.Mux) {
	router.Get("/api/levels", GetLevel)
	router.HandleFunc("/api/products", GetProduct).Methods("GET")
}
func GetLevel() {}
func GetProduct() {}
`,
	})
	target := goAdapterLibraryTarget(t)
	options := surfacediscovery.DefaultOptions(repositoryRoot, runtime.GOOS+"/"+runtime.GOARCH)
	options.CaptureCoreObjectIndex = true
	options.CaptureExternalCallIndex = true
	options.CaptureDynamicHandoffIndex = true
	result, err := surfacediscovery.AnalyzeContextWithInput(t.Context(), options, surfacediscovery.Input{
		ModuleDirs: []string{"."},
		Packages:   []surfacediscovery.PackageInput{{Path: "example.com/program", ModuleDir: "."}},
		AnalysisTarget: &surfacediscovery.AnalysisTargetInput{
			TargetRef: target.Ref, Kind: string(target.Kind),
			ModuleID: target.ModuleID, ModulePath: target.ModulePath, ModuleDir: target.ModuleDir,
			TargetPackages: []string{"example.com/program"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	index, err := Build(
		repository, target, goAdapterPackageOrigins(t, *result.ExternalCallIndex),
		*result.DirectCallIndex, *result.ExternalCallIndex,
		*result.CoreObjectIndex, *result.DynamicHandoffIndex,
	)
	if err != nil {
		t.Fatal(err)
	}
	objects := make(map[string]programindex.Object, len(index.Objects))
	var routesID, handlerID, productHandlerID string
	for _, object := range index.Objects {
		objects[object.ID] = object
		if object.Kind == programindex.ObjectFunction && object.Name == "Routes" {
			routesID = object.ID
		}
		if object.Kind == programindex.ObjectFunction && object.Name == "GetLevel" {
			handlerID = object.ID
		}
		if object.Kind == programindex.ObjectFunction && object.Name == "GetProduct" {
			productHandlerID = object.ID
		}
	}
	var handlePattern, methodsPattern programindex.RelationPattern
	for _, relation := range index.Relations {
		if relation.Kind != programindex.RelationInvokesExternal || len(relation.ToIDs) != 1 {
			continue
		}
		targetObject := objects[relation.ToIDs[0]]
		if targetObject.External == nil || targetObject.External.PackagePath != "github.com/go-chi/chi/v5" ||
			targetObject.External.Name == "" {
			continue
		}
		if targetObject.External.AuthorityKind != programindex.ExternalAuthorityPackage {
			t.Fatalf("chi external authority = %#v", targetObject.External)
		}
		switch targetObject.External.Name {
		case "Get":
			if len(relation.Patterns) != 1 || relation.Patterns[0].Selector != "Get" ||
				len(relation.Patterns[0].Arguments) != 2 ||
				relation.Patterns[0].Arguments[0].Value != "/api/levels" ||
				relation.Patterns[0].Arguments[1].Resolution != programindex.ResolutionExact ||
				len(relation.Patterns[0].Arguments[1].ObjectIDs) != 1 ||
				relation.Patterns[0].Arguments[1].ObjectIDs[0] != handlerID {
				t.Fatalf("chi Get neutral pattern = %#v", relation)
			}
		case "HandleFunc":
			if len(relation.Patterns) == 1 {
				handlePattern = relation.Patterns[0]
			}
		case "Methods":
			if len(relation.Patterns) == 1 {
				methodsPattern = relation.Patterns[0]
			}
		}
	}
	if routesID == "" || handlerID == "" || productHandlerID == "" {
		t.Fatalf("local callback objects: routes=%q get=%q product=%q", routesID, handlerID, productHandlerID)
	}
	if handlePattern.ResultID == "" || len(handlePattern.Arguments) != 2 ||
		handlePattern.Arguments[0].Value != "/api/products" ||
		handlePattern.Arguments[1].Resolution != programindex.ResolutionExact ||
		len(handlePattern.Arguments[1].ObjectIDs) != 1 ||
		handlePattern.Arguments[1].ObjectIDs[0] != productHandlerID {
		t.Fatalf("chained HandleFunc pattern = %#v", handlePattern)
	}
	resultObject := objects[handlePattern.ResultID]
	if resultObject.Kind != programindex.ObjectVariable || resultObject.Name != "call result" ||
		!strings.Contains(resultObject.Signature, "Route") {
		t.Fatalf("chained HandleFunc result = %#v", resultObject)
	}
	if methodsPattern.ReceiverID != handlePattern.ResultID || len(methodsPattern.Arguments) != 1 ||
		methodsPattern.Arguments[0].Kind != programindex.PatternLiteralString ||
		methodsPattern.Arguments[0].Value != "GET" {
		t.Fatalf("chained Methods pattern = %#v", methodsPattern)
	}
	callbackJoins := 0
	for _, relation := range index.Relations {
		if relation.Kind == programindex.RelationPassesCallback && relation.FromID == routesID &&
			relation.Resolution == programindex.ResolutionExact && len(relation.ToIDs) == 1 &&
			relation.ToIDs[0] == productHandlerID {
			callbackJoins++
			if relation.SourceArgumentID != handlePattern.Arguments[1].ID {
				t.Fatalf("chained callback source argument = %q, want %q", relation.SourceArgumentID, handlePattern.Arguments[1].ID)
			}
		}
	}
	if callbackJoins != 1 {
		t.Fatalf("chained callback joins = %d, want one", callbackJoins)
	}
}

func TestExternalCallPatternProjectionKeepsMissingCallableAsUnresolved(t *testing.T) {
	projection := goProjection{directNodeObjectRefs: map[string]string{}}
	argument := projection.externalCallPatternArgument(surfacediscovery.ExternalCallPatternArgument{
		Position: 1, Kind: surfacediscovery.ExternalCallPatternDynamic,
		ObjectIDs: []string{"repository.handler"}, ObjectsObserved: 1,
	})
	if argument.Resolution != programindex.ResolutionUnresolved || len(argument.ObjectRefs) != 0 ||
		argument.ObjectsObserved != 1 {
		t.Fatalf("missing target-local callable authority = %#v", argument)
	}
}

func TestBuildProjectsExistingGoFactsWithoutInventingProgramSemantics(t *testing.T) {
	repositoryRoot := t.TempDir()
	files := map[string]string{
		"go.mod": "module example.com/program\n\ngo 1.24\n",
		"program.go": `package program

import (
	"fmt"
	"io"
)

type Runner struct{}
type Task interface{ Do() }
type Handler interface{ Serve() }
type HandlerFunc func()
func (function HandlerFunc) Serve() { function() }
type Server struct{ Handler Handler }

func init() { helper() }
func init() { helper() }
func New() *Runner { return &Runner{} }
func (*Runner) Run() { fmt.Println("run"); helper() }
func Dispatch(task Task) { task.Do() }
func orphan(task Task) { task.Do() }
func Write(writer io.Writer) { _, _ = writer.Write([]byte("x")) }
func Register(topic string, callback func()) { callback() }
func Configure() {
	_ = &Server{Handler: HandlerFunc(bound)}
	Register("levels.requested", helper)
	println("ready")
}
func ConfigureAlternatives(useAlpha bool) {
	var callback HandlerFunc
	if useAlpha { callback = alpha } else { callback = beta }
	_ = &Server{Handler: callback}
}
func alpha() {}
func beta() {}
func bound() { boundLeaf() }
func boundLeaf() {}
func helper() {}
`,
	}
	repository := writeGoAdapterCorpus(t, repositoryRoot, files)
	target := goAdapterLibraryTarget(t)

	options := surfacediscovery.DefaultOptions(repositoryRoot, runtime.GOOS+"/"+runtime.GOARCH)
	options.CaptureCoreObjectIndex = true
	options.CaptureExternalCallIndex = true
	options.CaptureDynamicHandoffIndex = true
	result, err := surfacediscovery.AnalyzeContextWithInput(t.Context(), options, surfacediscovery.Input{
		ModuleDirs: []string{"."},
		Packages: []surfacediscovery.PackageInput{{
			Path: "example.com/program", ModuleDir: ".",
		}},
		AnalysisTarget: &surfacediscovery.AnalysisTargetInput{
			TargetRef: target.Ref, Kind: string(target.Kind),
			ModuleID: target.ModuleID, ModulePath: target.ModulePath, ModuleDir: target.ModuleDir,
			TargetPackages: []string{"example.com/program"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.DirectCallIndex == nil || result.ExternalCallIndex == nil || result.CoreObjectIndex == nil ||
		result.DynamicHandoffIndex == nil {
		t.Fatalf("producer indexes are absent: direct=%v external=%v core=%v",
			result.DirectCallIndex != nil, result.ExternalCallIndex != nil, result.CoreObjectIndex != nil)
	}

	index, err := Build(
		repository, target, goAdapterPackageOrigins(t, *result.ExternalCallIndex),
		*result.DirectCallIndex, *result.ExternalCallIndex, *result.CoreObjectIndex,
		*result.DynamicHandoffIndex,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := index.Validate(); err != nil {
		t.Fatal(err)
	}
	input, err := BuildInput(
		repository, target, goAdapterPackageOrigins(t, *result.ExternalCallIndex),
		*result.DirectCallIndex, *result.ExternalCallIndex, *result.CoreObjectIndex,
		*result.DynamicHandoffIndex,
	)
	if err != nil {
		t.Fatal(err)
	}
	fromCommonSeam, err := programindex.New(input)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(index, fromCommonSeam) {
		t.Fatal("common ProgramIndex sealing changed the Go adapter projection")
	}
	second, err := Build(
		repository, target, goAdapterPackageOrigins(t, *result.ExternalCallIndex),
		*result.DirectCallIndex, *result.ExternalCallIndex, *result.CoreObjectIndex,
		*result.DynamicHandoffIndex,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(index, second) {
		t.Fatal("same producer facts changed the neutral index")
	}
	if index.Target.Language != "go" || index.Target.Kind != "library" ||
		index.Target.Name != "example.com/program" {
		t.Fatalf("target = %#v", index.Target)
	}
	if len(index.Target.Seeds) != 0 {
		t.Fatalf("library target invented launch seeds: %#v", index.Target.Seeds)
	}
	manifestRef, ok := repository.ID("go.mod")
	if !ok || index.Target.AnchorFileRef != string(manifestRef) {
		t.Fatalf("target anchor = %q, want corpus ref %q", index.Target.AnchorFileRef, manifestRef)
	}
	for _, source := range index.Target.Sources {
		info, known := repository.Info(corpus.FileID(source.FileRef))
		if !known || info.Entry.Path != source.Path {
			t.Fatalf("target source %#v is outside the exact corpus", source)
		}
	}

	objectsByKindAndName := make(map[string]programindex.Object)
	for _, object := range index.Objects {
		objectsByKindAndName[string(object.Kind)+"\x00"+object.Name] = object
	}
	object := func(kind programindex.ObjectKind, name string) programindex.Object {
		return objectsByKindAndName[string(kind)+"\x00"+name]
	}
	for name, kind := range map[string]programindex.ObjectKind{
		"example.com/program": programindex.ObjectPackage,
		"Runner":              programindex.ObjectType,
		"Run":                 programindex.ObjectMethod,
		"helper":              programindex.ObjectFunction,
		"fmt.Println":         programindex.ObjectExternalSymbol,
		"io.Writer.Write":     programindex.ObjectExternalSymbol,
	} {
		if object(kind, name).Kind != kind {
			t.Fatalf("object %q = %#v, want kind %q", name, object(kind, name), kind)
		}
	}
	runMethod := object(programindex.ObjectMethod, "Run")
	runnerType := object(programindex.ObjectType, "Runner")
	helperFunction := object(programindex.ObjectFunction, "helper")
	if runMethod.OwnerID != runnerType.ID || runMethod.ContainerID != runnerType.ID {
		t.Fatalf("method receiver ownership was lost: Run=%#v Runner=%#v", runMethod, runnerType)
	}
	if helperFunction.Visibility != programindex.VisibilityInternal ||
		runMethod.Visibility != programindex.VisibilityPublic {
		t.Fatalf("Go visibility projection drifted: helper=%q Run=%q",
			helperFunction.Visibility, runMethod.Visibility)
	}
	fmtPrintln := object(programindex.ObjectExternalSymbol, "fmt.Println")
	ioWriterWrite := object(programindex.ObjectExternalSymbol, "io.Writer.Write")

	configureFunction := object(programindex.ObjectFunction, "Configure")
	registerFunction := object(programindex.ObjectFunction, "Register")
	configureAlternativesFunction := object(programindex.ObjectFunction, "ConfigureAlternatives")
	alphaFunction := object(programindex.ObjectFunction, "alpha")
	betaFunction := object(programindex.ObjectFunction, "beta")
	boundFunction := object(programindex.ObjectFunction, "bound")
	boundLeafFunction := object(programindex.ObjectFunction, "boundLeaf")
	orphanFunction := object(programindex.ObjectFunction, "orphan")
	var localCall, localPattern, boundCall, inventedBindingCall, staticExternal, interfaceExternal bool
	var unresolvedCall, callbackTransfer, callableBinding, alternativeBinding bool
	orphanUnresolved := 0
	configureUnresolved := 0
	for _, relation := range index.Relations {
		switch {
		case relation.Kind == programindex.RelationCalls && relation.Resolution == programindex.ResolutionExact &&
			relation.FromID == runMethod.ID && len(relation.ToIDs) == 1 &&
			relation.ToIDs[0] == helperFunction.ID:
			localCall = relation.Invocation == string(surfacediscovery.DirectCallSynchronous) &&
				relation.WitnessesObserved == 1
		case relation.Kind == programindex.RelationCalls && relation.Resolution == programindex.ResolutionExact &&
			relation.FromID == boundFunction.ID && len(relation.ToIDs) == 1 &&
			relation.ToIDs[0] == boundLeafFunction.ID:
			boundCall = true
		case relation.Kind == programindex.RelationCalls && relation.Resolution == programindex.ResolutionExact &&
			relation.FromID == configureFunction.ID && len(relation.ToIDs) == 1 &&
			relation.ToIDs[0] == registerFunction.ID:
			localPattern = relation.PatternsObserved == 1 && len(relation.Patterns) == 1 &&
				relation.Patterns[0].Selector == "Register" && len(relation.Patterns[0].Arguments) == 2 &&
				relation.Patterns[0].Arguments[0].Kind == programindex.PatternLiteralString &&
				relation.Patterns[0].Arguments[0].Value == "levels.requested" &&
				relation.Patterns[0].Arguments[1].Kind == programindex.PatternDynamic &&
				relation.Patterns[0].Arguments[1].Resolution == programindex.ResolutionExact &&
				len(relation.Patterns[0].Arguments[1].ObjectIDs) == 1 &&
				relation.Patterns[0].Arguments[1].ObjectIDs[0] == helperFunction.ID
		case relation.Kind == programindex.RelationCalls && relation.Resolution == programindex.ResolutionExact &&
			relation.FromID == configureFunction.ID && len(relation.ToIDs) == 1 &&
			relation.ToIDs[0] == boundFunction.ID:
			inventedBindingCall = true
		case relation.Kind == programindex.RelationInvokesExternal && relation.Resolution == programindex.ResolutionExact &&
			len(relation.ToIDs) == 1 && relation.ToIDs[0] == fmtPrintln.ID:
			staticExternal = relation.WitnessesObserved == 1 && relation.Witnesses[0].Kind == "go_external_static_call"
		case relation.Kind == programindex.RelationInvokesExternal && relation.Resolution == programindex.ResolutionExact &&
			len(relation.ToIDs) == 1 && relation.ToIDs[0] == ioWriterWrite.ID:
			interfaceExternal = relation.Invocation == "declared_interface_dispatch:synchronous" &&
				relation.WitnessesObserved == 1 && relation.Witnesses[0].Kind == "go_declared_interface_dispatch"
		case relation.Kind == programindex.RelationCalls && relation.Resolution == programindex.ResolutionUnresolved:
			isDynamic := relation.Invocation == "interface_invoke:synchronous" ||
				relation.Invocation == "function_value_call:synchronous"
			unresolvedCall = unresolvedCall || isDynamic && relation.TargetsObserved > 0 &&
				relation.TargetsOmitted == relation.TargetsObserved
			if relation.FromID == orphanFunction.ID {
				orphanUnresolved++
			}
			if relation.FromID == configureFunction.ID {
				configureUnresolved++
			}
		case relation.Kind == programindex.RelationPassesCallback &&
			relation.Resolution == programindex.ResolutionExact && relation.FromID == configureFunction.ID &&
			len(relation.ToIDs) == 1 && relation.ToIDs[0] == boundFunction.ID &&
			relation.Invocation == "callable_binding:field":
			callableBinding = relation.TargetsOmitted == 0 &&
				relation.Witnesses[0].Kind == "go_ssa_dynamic_handoff"
		case relation.Kind == programindex.RelationPassesCallback &&
			relation.Resolution == programindex.ResolutionAlternatives &&
			relation.FromID == configureAlternativesFunction.ID && len(relation.ToIDs) == 2 &&
			relation.Invocation == "callable_binding:field":
			alternativeBinding = slices.Contains(relation.ToIDs, alphaFunction.ID) &&
				slices.Contains(relation.ToIDs, betaFunction.ID) && relation.TargetsOmitted == 0
		case relation.Kind == programindex.RelationPassesCallback &&
			relation.Resolution == programindex.ResolutionExact && relation.FromID == configureFunction.ID &&
			len(relation.ToIDs) == 1 && relation.ToIDs[0] == helperFunction.ID &&
			relation.Invocation == "callback_transfer:synchronous":
			callbackTransfer = relation.TargetsOmitted == 0 &&
				relation.Witnesses[0].Kind == "go_ssa_dynamic_handoff"
		}
	}
	if !localCall || !localPattern || !boundCall || inventedBindingCall || !staticExternal || !interfaceExternal ||
		!unresolvedCall || !callbackTransfer ||
		!callableBinding || !alternativeBinding || orphanUnresolved != 1 || configureUnresolved != 0 {
		t.Fatalf("relation characterization: local=%v local_pattern=%v bound_call=%v invented_binding_call=%v static_external=%v interface_external=%v unresolved=%v callback=%v binding=%v alternative_binding=%v orphan_unresolved=%d configure_unresolved=%d\n%#v",
			localCall, localPattern, boundCall, inventedBindingCall, staticExternal, interfaceExternal, unresolvedCall, callbackTransfer,
			callableBinding, alternativeBinding, orphanUnresolved, configureUnresolved, index.Relations)
	}
}

func TestBuildModuleLibraryWhosePublicAPIContainsOnlyValues(t *testing.T) {
	repositoryRoot := t.TempDir()
	repository := writeGoAdapterCorpus(t, repositoryRoot, map[string]string{
		"go.mod": "module example.com/program\n\ngo 1.24\n",
		"values.go": `package program

const Version = "v1"

var Enabled = true
`,
	})
	target := goAdapterLibraryTarget(t)

	options := surfacediscovery.DefaultOptions(repositoryRoot, runtime.GOOS+"/"+runtime.GOARCH)
	options.CaptureCoreObjectIndex = true
	options.CaptureExternalCallIndex = true
	options.CaptureDynamicHandoffIndex = true
	result, err := surfacediscovery.AnalyzeContextWithInput(t.Context(), options, surfacediscovery.Input{
		ModuleDirs: []string{"."},
		Packages: []surfacediscovery.PackageInput{{
			Path: "example.com/program", ModuleDir: ".",
		}},
		AnalysisTarget: &surfacediscovery.AnalysisTargetInput{
			TargetRef: target.Ref, Kind: string(target.Kind),
			ModuleID: target.ModuleID, ModulePath: target.ModulePath, ModuleDir: target.ModuleDir,
			TargetPackages: []string{"example.com/program"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.DirectCallIndex == nil || result.ExternalCallIndex == nil || result.CoreObjectIndex == nil ||
		result.DynamicHandoffIndex == nil {
		t.Fatalf("producer indexes are absent: direct=%v external=%v core=%v",
			result.DirectCallIndex != nil, result.ExternalCallIndex != nil, result.CoreObjectIndex != nil)
	}
	if len(result.CoreObjectIndex.Types) != 0 || len(result.CoreObjectIndex.Callables) != 0 {
		t.Fatalf("value-only package unexpectedly gained type/callable declarations: %#v", result.CoreObjectIndex)
	}
	if len(result.CoreObjectIndex.Packages) != 1 ||
		result.CoreObjectIndex.Packages[0].RepresentativeSource != "values.go" {
		t.Fatalf("exact package source = %#v", result.CoreObjectIndex.Packages)
	}

	index, err := Build(
		repository, target, goAdapterPackageOrigins(t, *result.ExternalCallIndex),
		*result.DirectCallIndex, *result.ExternalCallIndex, *result.CoreObjectIndex,
		*result.DynamicHandoffIndex,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := index.Validate(); err != nil {
		t.Fatal(err)
	}
	valuesRef, ok := repository.ID("values.go")
	if !ok {
		t.Fatal("values.go is absent from repository corpus")
	}
	wantSource := programindex.TargetSource{FileRef: string(valuesRef), Path: "values.go"}
	if !slices.Contains(index.Target.Sources, wantSource) {
		t.Fatalf("target sources = %#v, want exact package source %#v", index.Target.Sources, wantSource)
	}
}

func TestBuildRejectsMissingCorpus(t *testing.T) {
	if _, err := Build(nil, analysistarget.Target{}, nil, surfacediscovery.DirectCallIndex{}, surfacediscovery.ExternalCallIndex{},
		(gocoreobject.Index{}), godynamichandoff.Index{}); err == nil {
		t.Fatal("Build accepted absent corpus and unavailable producer authority")
	}
}

func TestScenarioIdentityIncludesGoFlags(t *testing.T) {
	base := surfacediscovery.Scenario{
		ID: "scenario", GOOS: "linux", GOARCH: "amd64", Tags: []string{"exact"},
	}
	withoutFlags, err := scenarioIdentity(base)
	if err != nil {
		t.Fatal(err)
	}
	base.GoFlags = "-mod=vendor"
	withFlags, err := scenarioIdentity(base)
	if err != nil {
		t.Fatal(err)
	}
	if withoutFlags == withFlags {
		t.Fatal("scenario identity ignored exact Go flags")
	}
}

func TestCoreCallableNameMatchesDirectNodeAllowsOnlySSAInitOrdinals(t *testing.T) {
	for _, test := range []struct {
		declared string
		direct   string
		want     bool
	}{
		{declared: "Run", direct: "Run", want: true},
		{declared: "init", direct: "init", want: true},
		{declared: "init", direct: "init#1", want: true},
		{declared: "init", direct: "init#20", want: true},
		{declared: "init", direct: "init#0", want: false},
		{declared: "init", direct: "init#01", want: false},
		{declared: "init", direct: "init#x", want: false},
		{declared: "Run", direct: "Run#1", want: false},
	} {
		if got := coreCallableNameMatchesDirectNode(test.declared, test.direct); got != test.want {
			t.Fatalf("coreCallableNameMatchesDirectNode(%q, %q) = %v, want %v",
				test.declared, test.direct, got, test.want)
		}
	}
}

func TestAddUnresolvedMergesSameCallerFrontierAcrossProducerLedgers(t *testing.T) {
	projection := goProjection{unresolvedRelations: make(map[string]int)}
	projection.addUnresolved(
		"caller", programindex.RelationCalls, "dynamic", "go_dynamic_invoke", 2,
	)
	projection.addUnresolved(
		"caller", programindex.RelationCalls, "dynamic", "go_dynamic_invoke", 3,
	)

	if len(projection.relations) != 1 {
		t.Fatalf("unresolved relations = %d, want 1", len(projection.relations))
	}
	relation := projection.relations[0]
	if relation.TargetsObserved != 5 || relation.WitnessesObserved != 5 ||
		len(relation.Witnesses) != 1 || relation.Witnesses[0].Detail != "5" {
		t.Fatalf("merged unresolved frontier = %#v", relation)
	}
}

func goAdapterPackageOrigins(
	t *testing.T,
	external surfacediscovery.ExternalCallIndex,
) []gofacts.PackageOrigin {
	t.Helper()
	byPath := make(map[string]bool)
	for _, family := range external.Families {
		packagePath := family.Target.PackagePath
		if generatedCgoTarget(family.Target) {
			continue
		}
		standard := false
		if pkg, err := build.Default.Import(packagePath, "", build.FindOnly); err == nil {
			standard = pkg.Goroot
		}
		if previous, exists := byPath[packagePath]; exists && previous != standard {
			t.Fatalf("conflicting test package origin for %q", packagePath)
		}
		byPath[packagePath] = standard
	}
	result := make([]gofacts.PackageOrigin, 0, len(byPath))
	for packagePath, standard := range byPath {
		result = append(result, gofacts.PackageOrigin{PackagePath: packagePath, Standard: standard})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].PackagePath < result[j].PackagePath })
	if err := gofacts.ValidatePackageOrigins(result); err != nil {
		t.Fatal(err)
	}
	return result
}

func goAdapterLibraryTarget(t *testing.T) analysistarget.Target {
	t.Helper()
	completeness := &gofacts.PackageLoadCompleteness{
		Version: gofacts.PackageLoadCompletenessVersion,
		State:   gofacts.PackageLoadComplete,
	}
	facts := gofacts.Facts{
		Modules: []gofacts.ModuleFact{{
			ID: "module-program", ModulePath: "example.com/program", ModuleDir: ".", GoMod: "go.mod", Main: true,
			PackagesCount: 1, RetainedPackagesCount: 1,
			Coverage: gofacts.ModuleCoverage{State: gofacts.CoverageComplete, PackagesDiscovered: 1, PackagesRetained: 1},
		}},
		Packages: []gofacts.PackageFact{{
			CanonicalPath: "example.com/program", Name: "program", ModuleID: "module-program",
			ModulePath: "example.com/program", PackageDir: ".", ModuleRelativeDir: ".", DisplayPath: ".",
			Locality: "local", Files: []string{"program.go"}, DeclarationsScanned: true,
			Declarations: []gofacts.PackageDeclaration{{
				Kind: gofacts.PackageDeclarationFunc, Name: "New", Path: "program.go", Line: 10, Column: 6,
				ExecutableBody: true,
			}},
			LoadCompleteness: completeness,
		}},
		PackagesCount: 1, RetainedPackagesCount: 1,
	}
	candidates, err := analysistarget.Candidates(facts)
	if err != nil || len(candidates) != 1 {
		t.Fatalf("target candidates = %#v, %v", candidates, err)
	}
	return candidates[0].Target.Snapshot()
}

func writeGoAdapterCorpus(t *testing.T, root string, files map[string]string) *corpus.Corpus {
	t.Helper()
	paths := make([]string, 0, len(files))
	for filePath, content := range files {
		absolute := filepath.Join(root, filepath.FromSlash(filePath))
		if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(absolute, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, filePath)
	}
	sort.Strings(paths)
	repository, err := corpus.New(context.Background(), root, gitfiles.Listing{Paths: paths, RegularPaths: paths})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	return repository
}
