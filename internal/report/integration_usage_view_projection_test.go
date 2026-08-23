package report

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/dependencies"
	"github.com/dvordrova/repomap/internal/integrationdependency"
	"github.com/dvordrova/repomap/internal/integrationusage"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/programindex"
)

func TestIntegrationUsageViewPublishesOnlySelectedUsesWithExactSourceAuthority(t *testing.T) {
	index := reportIntegrationUsageIndex(t)
	selected := reportSelectedIntegrationDependencies(t, "acme", "unused")
	provider := &reportCoreMapProvider{response: []byte(
		`{"uses":[{"operation_ref":"o1","label":"Publish audit event","mechanism":"HTTP"}]}`,
	)}
	usage, err := integrationusage.Run(t.Context(), llm.Executor{}, provider, index, selected)
	if err != nil {
		t.Fatalf("integrationusage.Run: %v", err)
	}
	view, err := NewIntegrationUsageView(usage, index, selected)
	if err != nil {
		t.Fatalf("NewIntegrationUsageView: %v", err)
	}
	if len(view.Dependencies) != 1 || view.Dependencies[0].Name != "acme" ||
		len(view.Dependencies[0].Uses) != 1 {
		t.Fatalf("published dependency groups = %#v", view.Dependencies)
	}
	use := view.Dependencies[0].Uses[0]
	if use.Label != "Publish audit event" || use.Mechanism != "HTTP" ||
		use.CallerName != "publish" || use.CallerLocation.Path != "app/jobs.py" ||
		use.Callsite.Line != 8 || use.CallExpression != "client.send" ||
		use.CanonicalCallee != "acme.send" ||
		use.Authority != integrationusage.AuthoritySyntacticUnresolved {
		t.Fatalf("published selected use = %#v", use)
	}
	if view.Coverage.DependenciesObserved != 2 ||
		view.Coverage.DependenciesWithOperations != 1 ||
		view.Coverage.OperationsAdvertised != 1 || view.Coverage.Selected != 1 {
		t.Fatalf("producer coverage = %#v", view.Coverage)
	}
	if err := view.ValidateAgainst(usage, index, selected); err != nil {
		t.Fatalf("ValidateAgainst: %v", err)
	}

	tampered := *view
	tampered.Dependencies = append([]IntegrationUsageDependency(nil), view.Dependencies...)
	tampered.Dependencies[0].Uses = append([]IntegrationUsageUse(nil), view.Dependencies[0].Uses...)
	tampered.Dependencies[0].Uses[0].Label = "Invented browser label"
	if err := tampered.ValidateAgainst(usage, index, selected); err == nil ||
		!strings.Contains(err.Error(), "does not match exact producer authority") {
		t.Fatalf("tampered projection error = %v", err)
	}
}

func TestReadRunDirRejectsPartialIntegrationUsageMaterialAuthority(t *testing.T) {
	runDir := t.TempDir()
	index := reportProgramIndexFixture(t, "python", "executable")
	writeReportProgramIndexArtifacts(t, runDir, index)
	if err := os.Remove(filepath.Join(runDir, integrationusage.ArtifactFilename)); err != nil {
		t.Fatal(err)
	}
	writeReportProgramFile(t, filepath.Join(runDir, "snapshot.json"), []byte(`{"repo_name":"python-fixture"}`))
	writeReportProgramFile(t, filepath.Join(runDir, "metadata.json"), []byte(`{"repo_name":"python-fixture"}`))

	if _, err := ReadRunDir(runDir); err == nil ||
		!strings.Contains(err.Error(), "integration usage material authority is incomplete") {
		t.Fatalf("partial integration usage authority error = %v", err)
	}
}

func reportIntegrationUsageIndex(t *testing.T) programindex.Index {
	t.Helper()
	callsite := &programindex.Location{Path: "app/jobs.py", Line: 8, Column: 11}
	index, err := programindex.New(programindex.Input{
		ScenarioSHA256: strings.Repeat("a", 64),
		SourceSHA256:   strings.Repeat("b", 64),
		Target: programindex.TargetInput{
			Language: "python", Kind: "library", Name: "app", Selector: "library:app",
			Sources:       []programindex.TargetSource{{FileRef: "f1", Path: "app/jobs.py"}},
			AnchorFileRef: "f1", Seeds: []programindex.TargetSeedInput{},
		},
		Objects: []programindex.ObjectInput{
			{SourceRef: "module", Kind: programindex.ObjectModule, Name: "app.jobs",
				Visibility: programindex.VisibilityPublic,
				Location:   &programindex.Location{Path: "app/jobs.py", Line: 1, Column: 1}},
			{SourceRef: "caller", Kind: programindex.ObjectFunction, Name: "publish",
				Visibility: programindex.VisibilityPublic, ContainerRef: "module",
				Location: &programindex.Location{Path: "app/jobs.py", Line: 6, Column: 1}},
			{SourceRef: "external", Kind: programindex.ObjectExternalSymbol, Name: "acme.send",
				Visibility: programindex.VisibilityUnknown},
		},
		Relations: []programindex.RelationInput{{
			SourceRef: "external-call", Kind: programindex.RelationInvokesExternal,
			FromRef: "caller", ToRefs: []string{"external"},
			Resolution: programindex.ResolutionAlternatives,
			Invocation: "awaited", Location: callsite, TargetsObserved: 1,
			Witnesses: []programindex.Witness{{
				Kind: "callsite_candidate", Detail: "human callsite evidence",
				SourceExpression: "client.send", Location: callsite,
			}},
			WitnessesObserved: 1,
		}},
		Coverage: programindex.CoverageInput{
			Measured: true, ObjectsObserved: 3, RelationsObserved: 1,
		},
	})
	if err != nil {
		t.Fatalf("programindex.New: %v", err)
	}
	return index
}

func reportSelectedIntegrationDependencies(
	t *testing.T,
	packagePaths ...string,
) integrationdependency.Result {
	t.Helper()
	importer, err := dependencies.SealImporter(dependencies.Importer{
		Language: "python", Name: "app.jobs", ModulePath: "app",
		PackagePath: "app.jobs", RepositoryPath: "app",
	})
	if err != nil {
		t.Fatal(err)
	}
	values := make([]dependencies.Dependency, 0, len(packagePaths))
	for _, packagePath := range packagePaths {
		values = append(values, dependencies.Dependency{
			Language: "python", Kind: dependencies.KindExternal, Name: packagePath,
			ModulePath: packagePath, PackagePath: packagePath, ImporterRefs: []string{importer.Ref},
		})
	}
	catalog, err := dependencies.BuildWithOmissions([]dependencies.Importer{importer}, values, nil)
	if err != nil {
		t.Fatal(err)
	}
	result := integrationdependency.Result{
		Version:                 integrationdependency.Version,
		DependencyCatalogSHA256: strings.Repeat("c", 64),
		Dependencies:            make([]integrationdependency.SelectedDependency, 0, len(catalog.Dependencies)),
		Coverage: integrationdependency.Coverage{
			Observed: len(catalog.Dependencies), Advertised: len(catalog.Dependencies),
			ModelCalled: len(catalog.Dependencies) > 0,
		},
	}
	for _, dependency := range catalog.Dependencies {
		result.Dependencies = append(result.Dependencies, integrationdependency.SelectedDependency{
			Dependency: dependency, Importers: []dependencies.Importer{importer},
		})
	}
	if err := result.Validate(); err != nil {
		t.Fatal(err)
	}
	return result
}
