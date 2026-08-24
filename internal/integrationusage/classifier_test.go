package integrationusage

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/dependencies"
	"github.com/dvordrova/repomap/internal/integrationdependency"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/programindex"
)

func TestRunRestoresAliasWitnessAndUsesLongestDependencyPrefix(t *testing.T) {
	index := integrationUsageIndex(t, "client.send", "acme.v2.send", "human alias evidence only")
	selected := integrationUsageDependencies(t, "acme", "acme.v2")
	provider := &integrationUsageProvider{
		response: []byte(`{"uses":[{"operation_ref":"o1","label":"Send audit event","mechanism":"unknown"}]}`),
	}

	result, err := Run(context.Background(), llm.Executor{}, provider, index, selected)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(result.Uses) != 1 {
		t.Fatalf("uses = %#v", result.Uses)
	}
	operation := result.Uses[0].Operation
	wantDependency := selectedDependencyNamed(t, selected, "acme.v2").Dependency.ID
	wantExternalSymbolID := ""
	for _, object := range index.Objects {
		if object.Kind == programindex.ObjectExternalSymbol && object.Name == "acme.v2.send" {
			wantExternalSymbolID = object.ID
		}
	}
	if operation.DependencyID != wantDependency || operation.CallExpression != "client.send" ||
		operation.CanonicalCallee != "acme.v2.send" || operation.ExternalSymbolID != wantExternalSymbolID ||
		operation.Authority != AuthoritySyntacticUnresolved {
		t.Fatalf("restored operation = %#v", operation)
	}
	if operation.Invocation != "awaited" || operation.CallerName != "publish" ||
		operation.CallerLocation.Path != "app/jobs.py" || operation.Callsite.Line != 8 {
		t.Fatalf("exact callsite authority = %#v", operation)
	}

	var request request
	if err := json.Unmarshal([]byte(provider.prompt.User), &request); err != nil {
		t.Fatalf("decode captured request: %v", err)
	}
	if len(request.Dependencies) != 1 || len(request.Operations) != 1 {
		t.Fatalf("request catalog = %#v", request)
	}
	if request.Dependencies[0].Ref != "d2" || request.Operations[0].DependencyRef != "d2" ||
		request.Operations[0].Caller.Name != "publish" ||
		request.Operations[0].Authority != AuthoritySyntacticUnresolved {
		t.Fatalf("request operation = %#v", request.Operations[0])
	}
	if strings.Contains(provider.responseString(), wantDependency) ||
		strings.Contains(provider.responseString(), "app/jobs.py") {
		t.Fatal("model response copied local authority instead of selecting a short ref")
	}

	encoded, err := Encode(result)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	restored, err := Decode(encoded)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if !reflect.DeepEqual(restored, result) {
		t.Fatalf("artifact round trip mismatch\n got: %#v\nwant: %#v", restored, result)
	}
	if err := restored.ValidateAgainst(index, selected); err != nil {
		t.Fatalf("ValidateAgainst: %v", err)
	}
}

func TestRunRejectsUnknownAndDuplicateOperationRefs(t *testing.T) {
	index := integrationUsageIndex(t, "acme.send", "acme.send", "human callsite evidence")
	selected := integrationUsageDependencies(t, "acme")
	for _, response := range [][]byte{
		[]byte(`{"uses":[{"operation_ref":"o999","label":"Send","mechanism":"unknown"}]}`),
		[]byte(`{"uses":[{"operation_ref":"o1","label":"Send","mechanism":"unknown"},{"operation_ref":"o1","label":"Send again","mechanism":"unknown"}]}`),
	} {
		provider := &integrationUsageProvider{response: response}
		if _, err := Run(context.Background(), llm.Executor{}, provider, index, selected); err == nil {
			t.Fatalf("Run accepted response %s", response)
		}
	}
}

func TestPrepareFailsClosedWhenSelectedDependencyAuthorityIsPartial(t *testing.T) {
	index := integrationUsageIndex(t, "acme.send", "acme.send", "human callsite evidence")
	selected := integrationUsageDependencies(t, "acme")
	selected.Coverage.Observed++
	selected.Coverage.Omitted++

	provider := &integrationUsageProvider{response: []byte(`{"uses":[]}`)}
	if _, err := Run(context.Background(), llm.Executor{}, provider, index, selected); err == nil {
		t.Fatal("Run accepted partial integration-dependency authority")
	}
	if provider.calls != 0 {
		t.Fatalf("provider calls = %d, want 0", provider.calls)
	}
}

func TestPrepareRejectsCallsiteWitnessWithoutTypedSourceExpression(t *testing.T) {
	index := integrationUsageIndex(t, "", "acme.send", "human callsite evidence")
	selected := integrationUsageDependencies(t, "acme")
	if _, err := Run(context.Background(), llm.Executor{}, &integrationUsageProvider{}, index, selected); err == nil {
		t.Fatal("Run accepted a witness without typed source-expression authority")
	}
}

func TestRunBatchesCompleteOperationsWithGlobalRefs(t *testing.T) {
	index := integrationUsageIndexMany(t, MaxAdvertisedOperationsPerRequest+1, "acme.send", "acme.send")
	selected := integrationUsageDependencies(t, "acme", "unused")
	provider := &integrationUsageProvider{responses: [][]byte{
		[]byte(`{"uses":[]}`),
		[]byte(`{"uses":[{"operation_ref":"o257","label":"Send final event","mechanism":"unknown"}]}`),
	}}

	result, err := Run(context.Background(), llm.Executor{}, provider, index, selected)
	if err != nil {
		t.Fatal(err)
	}
	if provider.calls != 2 || len(provider.prompts) != 2 {
		t.Fatalf("provider calls/prompts = %d/%d, want 2/2", provider.calls, len(provider.prompts))
	}
	if result.Coverage.OperationsAdvertised != MaxAdvertisedOperationsPerRequest+1 ||
		result.Coverage.DependenciesObserved != 2 ||
		result.Coverage.DependenciesWithOperations != 1 ||
		result.Coverage.Selected != 1 || !result.Coverage.ModelCalled || len(result.Uses) != 1 {
		t.Fatalf("batched result = %#v", result)
	}
	var first, second request
	if err := json.Unmarshal([]byte(provider.prompts[0].User), &first); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(provider.prompts[1].User), &second); err != nil {
		t.Fatal(err)
	}
	if first.BatchIndex != 1 || first.BatchCount != 2 || len(first.Operations) != 256 ||
		first.Operations[0].Ref != "o1" || first.Operations[255].Ref != "o256" ||
		len(first.Dependencies) != 1 || first.Dependencies[0].PackagePath != "acme" ||
		second.BatchIndex != 2 || second.BatchCount != 2 || len(second.Operations) != 1 ||
		second.Operations[0].Ref != "o257" || len(second.Dependencies) != 1 ||
		second.Observed != 257 || second.Omitted != 0 {
		t.Fatalf("batch requests = %#v / %#v", first, second)
	}
	if result.Uses[0].Label != "Send final event" || result.Uses[0].Operation.DependencyID !=
		selectedDependencyNamed(t, selected, "acme").Dependency.ID {
		t.Fatalf("restored global selection = %#v", result.Uses[0])
	}
}

func TestRunGoRestoresOnlyExactTypedExternalOperations(t *testing.T) {
	callsite := &programindex.Location{Path: "service/run.go", Line: 18, Column: 9}
	index, err := programindex.New(programindex.Input{
		ScenarioSHA256: strings.Repeat("a", 64),
		SourceSHA256:   strings.Repeat("b", 64),
		Target: programindex.TargetInput{
			Language: "go", Kind: "executable", Name: "service", Selector: "./cmd/service",
			Sources: []programindex.TargetSource{
				{FileRef: "f1", Path: "service/run.go"},
			},
			AnchorFileRef: "f1", Seeds: []programindex.TargetSeedInput{},
		},
		Objects: []programindex.ObjectInput{
			{SourceRef: "module", Kind: programindex.ObjectModule, Name: "example.com/app",
				Visibility: programindex.VisibilityPublic},
			{SourceRef: "package", Kind: programindex.ObjectPackage, Name: "example.com/app/service",
				Visibility: programindex.VisibilityPublic, OwnerRef: "module", ContainerRef: "module"},
			{SourceRef: "caller", Kind: programindex.ObjectFunction, Name: "Run",
				Visibility: programindex.VisibilityPublic, OwnerRef: "package", ContainerRef: "package",
				Location: &programindex.Location{Path: "service/run.go", Line: 12, Column: 1}},
			{SourceRef: "external", Kind: programindex.ObjectExternalSymbol, Name: "net/http.Client.Do",
				Visibility: programindex.VisibilityPublic,
				External:   &programindex.ExternalSymbol{PackagePath: "net/http", Receiver: "Client", Name: "Do"}},
		},
		Relations: []programindex.RelationInput{
			{
				SourceRef: "external-call", Kind: programindex.RelationInvokesExternal,
				FromRef: "caller", ToRefs: []string{"external"}, Resolution: programindex.ResolutionExact,
				Invocation: "sync", Location: callsite, TargetsObserved: 1,
				Witnesses:         []programindex.Witness{{Kind: goStaticCallWitness, Location: callsite}},
				WitnessesObserved: 1,
			},
			{
				SourceRef: "dynamic-frontier", Kind: programindex.RelationInvokesExternal,
				FromRef: "caller", ToRefs: []string{}, Resolution: programindex.ResolutionUnresolved,
				Invocation: "dynamic", TargetsObserved: 2,
				Witnesses:         []programindex.Witness{{Kind: "go_dynamic_invoke", Detail: "2"}},
				WitnessesObserved: 2,
			},
		},
		// One unrelated adapter object and relation were omitted. The complete
		// invokes_external ledger remains sufficient authority for integrations.
		Coverage: programindex.CoverageInput{Measured: true, ObjectsObserved: 5, RelationsObserved: 3},
	})
	if err != nil {
		t.Fatalf("programindex.New: %v", err)
	}
	selected := integrationUsageGoDependencies(t, "example.com/app/service", "net/http")
	provider := &integrationUsageProvider{
		response: []byte(`{"uses":[{"operation_ref":"o1","label":"Send HTTP request","mechanism":"HTTP"}]}`),
	}

	result, err := Run(context.Background(), llm.Executor{}, provider, index, selected)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(result.Uses) != 1 {
		t.Fatalf("uses = %#v", result.Uses)
	}
	if index.Coverage.ObjectsOmitted != 1 || index.Coverage.RelationsOmitted != 1 {
		t.Fatalf("fixture omission frontier = %#v", index.Coverage)
	}
	operation := result.Uses[0].Operation
	if operation.Language != "go" || operation.Authority != AuthorityExactExternalSymbol ||
		operation.CanonicalCallee != "net/http.Client.Do" || operation.CallExpression != "" ||
		operation.Callsite != *callsite {
		t.Fatalf("Go operation = %#v", operation)
	}
	if result.Coverage.ExternalRelationsObserved != 2 || result.Coverage.ExactExternalRelations != 1 ||
		result.Coverage.UnresolvedRuntimeRelations != 1 || result.Coverage.CallsiteCandidatesObserved != 3 ||
		result.Coverage.CallsiteCandidatesOmitted != 2 || result.Coverage.OperationsAdvertised != 1 {
		t.Fatalf("Go coverage = %#v", result.Coverage)
	}
	var request request
	if err := json.Unmarshal([]byte(provider.prompt.User), &request); err != nil {
		t.Fatal(err)
	}
	if len(request.Operations) != 1 || request.Operations[0].Authority != AuthorityExactExternalSymbol ||
		request.Operations[0].CallExpression != "" {
		t.Fatalf("Go request operation = %#v", request.Operations)
	}
}

func TestRunRejectsCrossBatchOperationRef(t *testing.T) {
	index := integrationUsageIndexMany(t, MaxAdvertisedOperationsPerRequest+1, "acme.send", "acme.send")
	selected := integrationUsageDependencies(t, "acme")
	provider := &integrationUsageProvider{responses: [][]byte{
		[]byte(`{"uses":[]}`),
		[]byte(`{"uses":[{"operation_ref":"o1","label":"Send","mechanism":"unknown"}]}`),
	}}
	if _, err := Run(context.Background(), llm.Executor{}, provider, index, selected); err == nil ||
		!strings.Contains(err.Error(), `unknown request-local ref "o1"`) {
		t.Fatalf("cross-batch ref error = %v", err)
	}
	if provider.calls != 2 {
		t.Fatalf("provider calls = %d, want 2", provider.calls)
	}
}

func integrationUsageIndex(t *testing.T, sourceExpression, externalName, detail string) programindex.Index {
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
			{SourceRef: "external", Kind: programindex.ObjectExternalSymbol, Name: externalName,
				Visibility: programindex.VisibilityUnknown},
		},
		Relations: []programindex.RelationInput{{
			SourceRef: "external-call", Kind: programindex.RelationInvokesExternal,
			FromRef: "caller", ToRefs: []string{"external"}, Resolution: programindex.ResolutionAlternatives,
			Invocation: "awaited", Location: callsite, TargetsObserved: 1,
			Witnesses: []programindex.Witness{{
				Kind: pythonCallsiteCandidate, Detail: detail,
				SourceExpression: sourceExpression, Location: callsite,
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

func integrationUsageIndexMany(t *testing.T, count int, sourceExpression, externalName string) programindex.Index {
	t.Helper()
	relations := make([]programindex.RelationInput, 0, count)
	for index := 0; index < count; index++ {
		callsite := &programindex.Location{Path: "app/jobs.py", Line: 8 + index, Column: 11}
		relations = append(relations, programindex.RelationInput{
			SourceRef: fmt.Sprintf("external-call-%05d", index),
			Kind:      programindex.RelationInvokesExternal, FromRef: "caller",
			ToRefs: []string{"external"}, Resolution: programindex.ResolutionAlternatives, Invocation: "awaited",
			Location: callsite, TargetsObserved: 1,
			Witnesses: []programindex.Witness{{
				Kind: pythonCallsiteCandidate, Detail: "human callsite evidence",
				SourceExpression: sourceExpression, Location: callsite,
			}},
			WitnessesObserved: 1,
		})
	}
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
			{SourceRef: "external", Kind: programindex.ObjectExternalSymbol, Name: externalName,
				Visibility: programindex.VisibilityUnknown},
		},
		Relations: relations,
		Coverage: programindex.CoverageInput{
			Measured: true, ObjectsObserved: 3, RelationsObserved: count,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return index
}

func integrationUsageDependencies(t *testing.T, packagePaths ...string) integrationdependency.Result {
	t.Helper()
	importer, err := dependencies.SealImporter(dependencies.Importer{
		Language: "python", Name: "app.jobs", ModulePath: "app",
		PackagePath: "app.jobs", RepositoryPath: "app",
	})
	if err != nil {
		t.Fatalf("SealImporter: %v", err)
	}
	values := make([]dependencies.Dependency, 0, len(packagePaths))
	for _, packagePath := range packagePaths {
		values = append(values, dependencies.Dependency{
			Language: "python", Kind: dependencies.KindExternal, Name: packagePath,
			ModulePath: strings.Split(packagePath, ".")[0], PackagePath: packagePath,
			ImporterRefs: []string{importer.Ref},
		})
	}
	catalog, err := dependencies.BuildWithOmissions([]dependencies.Importer{importer}, values, nil)
	if err != nil {
		t.Fatalf("BuildWithOmissions: %v", err)
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
		t.Fatalf("integration dependency result: %v", err)
	}
	return result
}

func integrationUsageGoDependencies(
	t *testing.T,
	importerPackage string,
	dependencyPackage string,
) integrationdependency.Result {
	t.Helper()
	importer, err := dependencies.SealImporter(dependencies.Importer{
		Language: "go", Name: "service", ModulePath: "example.com/app",
		PackagePath: importerPackage, RepositoryPath: "service",
	})
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := dependencies.BuildWithOmissions(
		[]dependencies.Importer{importer},
		[]dependencies.Dependency{{
			Language: "go", Kind: dependencies.KindStdlib, Name: "http",
			PackagePath: dependencyPackage, ImporterRefs: []string{importer.Ref},
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
		Coverage: integrationdependency.Coverage{
			Observed: 1, Advertised: 1, ModelCalled: true,
		},
	}
	if err := result.Validate(); err != nil {
		t.Fatal(err)
	}
	return result
}

func selectedDependencyNamed(
	t *testing.T,
	result integrationdependency.Result,
	name string,
) integrationdependency.SelectedDependency {
	t.Helper()
	for _, selected := range result.Dependencies {
		if selected.Dependency.Name == name {
			return selected
		}
	}
	t.Fatalf("dependency %q not found", name)
	return integrationdependency.SelectedDependency{}
}

type integrationUsageProvider struct {
	prompt    llm.Prompt
	prompts   []llm.Prompt
	response  []byte
	responses [][]byte
	calls     int
}

func (provider *integrationUsageProvider) State() []byte {
	return []byte(`{"provider":"integration-usage-test"}`)
}

func (provider *integrationUsageProvider) Prepare(prompt llm.Prompt, _ llm.Limits) (llm.Prepared, error) {
	provider.prompt = prompt
	provider.prompts = append(provider.prompts, prompt)
	return llm.NewPrepared([]byte(prompt.User))
}

func (provider *integrationUsageProvider) Complete(context.Context, llm.Prepared) (llm.Completion, error) {
	response := provider.response
	if len(provider.responses) > 0 {
		if provider.calls >= len(provider.responses) {
			return llm.Completion{}, fmt.Errorf("unexpected provider call %d", provider.calls+1)
		}
		response = provider.responses[provider.calls]
	}
	provider.calls++
	return llm.Completion{
		Response: response, FinishReason: llm.FinishStop, ChoiceCount: 1,
		Metrics: llm.Metrics{Attempts: 1},
	}, nil
}

func (provider *integrationUsageProvider) responseString() string {
	return string(provider.response)
}
