package integrationusage

import (
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/dependencies"
	"github.com/dvordrova/repomap/internal/integrationdependency"
	"github.com/dvordrova/repomap/internal/programindex"
)

func TestPrepareTypeScriptRetainsExactExternalCallAndUnresolvedFrontier(t *testing.T) {
	index := javaScriptTypeScriptUsageIndex(t, "typescript", programindex.ResolutionExact)
	selected := javaScriptTypeScriptUsageDependencies(t, "typescript", "drizzle-orm")

	_, candidates, coverage, err := prepare(index, selected)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates.operations) != 1 ||
		candidates.operations[0].operation.Authority != AuthorityExactExternalSymbol ||
		candidates.operations[0].operation.Language != "typescript" ||
		candidates.operations[0].operation.CallExpression != "eq" {
		t.Fatalf("TypeScript operations = %#v", candidates.operations)
	}
	if coverage.ExternalRelationsObserved != 2 || coverage.ExactExternalRelations != 1 ||
		coverage.UnresolvedRuntimeRelations != 1 || coverage.CallsiteCandidatesObserved != 2 ||
		coverage.OperationsAdvertised != 1 || coverage.CallsiteCandidatesOmitted != 1 {
		t.Fatalf("TypeScript coverage = %#v", coverage)
	}
}

func TestPrepareJavaScriptPreservesAlternativeCallAuthority(t *testing.T) {
	index := javaScriptTypeScriptUsageIndex(t, "javascript", programindex.ResolutionAlternatives)
	selected := javaScriptTypeScriptUsageDependencies(t, "javascript", "drizzle-orm")

	_, candidates, coverage, err := prepare(index, selected)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates.operations) != 1 ||
		candidates.operations[0].operation.Authority != AuthoritySyntacticUnresolved ||
		candidates.operations[0].operation.Language != "javascript" {
		t.Fatalf("JavaScript operations = %#v", candidates.operations)
	}
	if coverage.ExactExternalRelations != 0 || coverage.UnresolvedRuntimeRelations != 2 {
		t.Fatalf("JavaScript coverage = %#v", coverage)
	}
	invalid := candidates.operations[0].operation
	invalid.Authority = AuthorityExactExternalSymbol
	if err := validateOperation(invalid); err == nil {
		t.Fatal("JavaScript operation accepted false exact symbol authority")
	}
}

func TestPrepareTypeScriptTargetPreservesJavaScriptOriginAlternativeCall(t *testing.T) {
	index := javaScriptTypeScriptUsageIndex(
		t, "typescript", programindex.ResolutionAlternatives, javaScriptCallCandidateWitness,
	)
	selected := javaScriptTypeScriptUsageDependencies(t, "typescript", "drizzle-orm")

	_, candidates, _, err := prepare(index, selected)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates.operations) != 1 ||
		candidates.operations[0].operation.Authority != AuthoritySyntacticUnresolved ||
		candidates.operations[0].operation.Language != "typescript" {
		t.Fatalf("mixed-source TypeScript operations = %#v", candidates.operations)
	}
}

func TestJavaScriptTypeScriptExternalWitnessSetMatchesProducerAuthority(t *testing.T) {
	location := &programindex.Location{Path: "src/index.ts", Line: 1, Column: 1}
	for _, test := range []struct {
		language   string
		kind       string
		resolution programindex.Resolution
		want       bool
	}{
		{language: "typescript", kind: typeScriptCallWitness, resolution: programindex.ResolutionExact, want: true},
		{language: "typescript", kind: typeScriptCallWitness, resolution: programindex.ResolutionAlternatives, want: false},
		{language: "javascript", kind: javaScriptCallCandidateWitness, resolution: programindex.ResolutionAlternatives, want: true},
		{language: "javascript", kind: "typescript_package_authority", resolution: programindex.ResolutionExact, want: false},
		{language: "typescript", kind: "javascript_package_candidate", resolution: programindex.ResolutionAlternatives, want: false},
	} {
		witness := programindex.Witness{Kind: test.kind, SourceExpression: "candidate", Location: location}
		if got := validJavaScriptTypeScriptExternalWitness(
			test.language, test.resolution, witness,
		); got != test.want {
			t.Fatalf("valid witness (%q, %q) = %t, want %t", test.language, test.kind, got, test.want)
		}
	}
	jsCandidate := programindex.Witness{
		Kind: javaScriptCallCandidateWitness, SourceExpression: "candidate", Location: location,
	}
	if validJavaScriptTypeScriptExternalWitness("typescript", programindex.ResolutionExact, jsCandidate) {
		t.Fatal("JavaScript-origin candidate gained exact authority inside a TypeScript target")
	}
}

func javaScriptTypeScriptUsageIndex(
	t *testing.T,
	language string,
	resolution programindex.Resolution,
	witnessOverride ...string,
) programindex.Index {
	t.Helper()
	callsite := &programindex.Location{Path: "server/storage.ts", Line: 12, Column: 9}
	witnessKind := typeScriptCallWitness
	if language == "javascript" {
		callsite.Path = "server/storage.js"
		witnessKind = javaScriptCallCandidateWitness
	}
	if len(witnessOverride) > 0 {
		witnessKind = witnessOverride[0]
	}
	index, err := programindex.New(programindex.Input{
		ScenarioSHA256: strings.Repeat("a", 64),
		SourceSHA256:   strings.Repeat("b", 64),
		Target: programindex.TargetInput{
			Language: language, Kind: "application", Name: "web-app",
			Selector:      "root-package:package.json",
			Sources:       []programindex.TargetSource{{FileRef: "f1", Path: callsite.Path}},
			AnchorFileRef: "f1", Seeds: []programindex.TargetSeedInput{},
		},
		Objects: []programindex.ObjectInput{
			{SourceRef: "module", Kind: programindex.ObjectModule, Name: callsite.Path,
				Visibility: programindex.VisibilityInternal, Location: &programindex.Location{Path: callsite.Path, Line: 1, Column: 1}},
			{SourceRef: "caller", Kind: programindex.ObjectFunction, Name: "saveSettings",
				Visibility: programindex.VisibilityInternal, ContainerRef: "module", Location: &programindex.Location{Path: callsite.Path, Line: 8, Column: 1}},
			{SourceRef: "external", Kind: programindex.ObjectExternalSymbol, Name: "drizzle-orm.eq",
				Visibility: programindex.VisibilityUnknown,
				External:   &programindex.ExternalSymbol{PackagePath: "drizzle-orm", Name: "eq"}},
		},
		Relations: []programindex.RelationInput{
			{
				SourceRef: "external-call", Kind: programindex.RelationInvokesExternal,
				FromRef: "caller", ToRefs: []string{"external"}, Resolution: resolution,
				Invocation: "sync", Location: callsite, TargetsObserved: 1,
				Witnesses: []programindex.Witness{{
					Kind: witnessKind, SourceExpression: "eq", Location: callsite,
				}},
				WitnessesObserved: 1,
			},
			{
				SourceRef: "dynamic-call", Kind: programindex.RelationInvokesExternal,
				FromRef: "caller", ToRefs: []string{}, Resolution: programindex.ResolutionUnresolved,
				Invocation: "dynamic", Location: &programindex.Location{Path: callsite.Path, Line: 14, Column: 9},
				TargetsObserved: 1,
				Witnesses: []programindex.Witness{{
					Kind: witnessKind, SourceExpression: "adapter[name]", Location: &programindex.Location{Path: callsite.Path, Line: 14, Column: 9},
				}},
				WitnessesObserved: 1,
			},
		},
		Coverage: programindex.CoverageInput{Measured: true, ObjectsObserved: 3, RelationsObserved: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	return index
}

func javaScriptTypeScriptUsageDependencies(
	t *testing.T,
	language string,
	packagePath string,
) integrationdependency.Result {
	t.Helper()
	importer, err := dependencies.SealImporter(dependencies.Importer{
		Language: language, Name: "web-app", ModulePath: "web-app",
		PackagePath: "web-app", RepositoryPath: ".",
	})
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := dependencies.BuildWithOmissions(
		[]dependencies.Importer{importer},
		[]dependencies.Dependency{{
			Language: language, Kind: dependencies.KindExternal, Name: packagePath,
			ModulePath: packagePath, PackagePath: packagePath,
			ImporterRefs: []string{importer.Ref},
		}},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	result := integrationdependency.Result{
		Version: integrationdependency.Version, DependencyCatalogSHA256: strings.Repeat("c", 64),
		Dependencies: []integrationdependency.SelectedDependency{{
			Dependency: catalog.Dependencies[0], Importers: []dependencies.Importer{importer},
		}},
		Coverage: integrationdependency.Coverage{Observed: 1, Advertised: 1, ModelCalled: true},
	}
	if err := result.Validate(); err != nil {
		t.Fatal(err)
	}
	return result
}
