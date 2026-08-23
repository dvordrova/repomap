package goadapter

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"sort"
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

func init() { helper() }
func init() { helper() }
func New() *Runner { return &Runner{} }
func (*Runner) Run() { fmt.Println("run"); helper() }
func Dispatch(task Task) { task.Do() }
func Write(writer io.Writer) { _, _ = writer.Write([]byte("x")) }
func Register(callback func()) { callback() }
func Configure() { Register(helper) }
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
		repository, target, *result.DirectCallIndex, *result.ExternalCallIndex, *result.CoreObjectIndex,
		*result.DynamicHandoffIndex,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := index.Validate(); err != nil {
		t.Fatal(err)
	}
	second, err := Build(
		repository, target, *result.DirectCallIndex, *result.ExternalCallIndex, *result.CoreObjectIndex,
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
	var localCall, staticExternal, interfaceExternal, unresolvedCall, callbackTransfer bool
	for _, relation := range index.Relations {
		switch {
		case relation.Kind == programindex.RelationCalls && relation.Resolution == programindex.ResolutionExact &&
			relation.FromID == runMethod.ID && len(relation.ToIDs) == 1 &&
			relation.ToIDs[0] == helperFunction.ID:
			localCall = relation.Invocation == string(surfacediscovery.DirectCallSynchronous) &&
				relation.WitnessesObserved == 1
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
		case relation.Kind == programindex.RelationPassesCallback &&
			relation.Resolution == programindex.ResolutionExact && relation.FromID == configureFunction.ID &&
			len(relation.ToIDs) == 1 && relation.ToIDs[0] == helperFunction.ID:
			callbackTransfer = relation.Invocation == "callback_transfer:synchronous" &&
				relation.TargetsOmitted == 0 && relation.Witnesses[0].Kind == "go_ssa_dynamic_handoff"
		}
	}
	if !localCall || !staticExternal || !interfaceExternal || !unresolvedCall || !callbackTransfer {
		t.Fatalf("relation characterization: local=%v static_external=%v interface_external=%v unresolved=%v callback=%v\n%#v",
			localCall, staticExternal, interfaceExternal, unresolvedCall, callbackTransfer, index.Relations)
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
		repository, target, *result.DirectCallIndex, *result.ExternalCallIndex, *result.CoreObjectIndex,
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
	if _, err := Build(nil, analysistarget.Target{}, surfacediscovery.DirectCallIndex{}, surfacediscovery.ExternalCallIndex{},
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
