package integrationdependency

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/dependencies"
	"github.com/dvordrova/repomap/internal/dependencydeclaration"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/programindex"
)

func TestRunRestoresSelectedDependencyAndAllImporters(t *testing.T) {
	importer := dependencies.Importer{
		Language: "go", Name: "app", ModulePath: "example.com/app",
		PackagePath: "example.com/app", RepositoryPath: ".",
	}
	sealed, err := dependencies.SealImporter(importer)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := dependencies.BuildWithOmissions([]dependencies.Importer{sealed}, []dependencies.Dependency{
		{
			Language: "go", Kind: dependencies.KindWorkspace, Name: "internal",
			ModulePath: "example.com/app", PackagePath: "example.com/app/internal", RepositoryPath: "internal",
			ImporterRefs: []string{sealed.Ref},
		},
		{
			Language: "go", Kind: dependencies.KindStdlib, Name: "fmt",
			PackagePath: "fmt", ImporterRefs: []string{sealed.Ref},
		},
		{
			Language: "go", Kind: dependencies.KindExternal, Name: "sdk",
			ModulePath: "example.com/sdk", PackagePath: "example.com/sdk/client",
			ImporterRefs: []string{sealed.Ref},
		},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	provider := &classifierTestProvider{response: []byte(`{"integration_dependency_refs":["d2"]}`)}
	result, err := Run(t.Context(), llm.Executor{Enabled: false}, provider, catalog)
	if err != nil {
		t.Fatal(err)
	}
	if result.Coverage != (Coverage{Observed: 2, Advertised: 2, ModelCalled: true}) ||
		len(result.Dependencies) != 1 ||
		result.Dependencies[0].Dependency.PackagePath != "example.com/sdk/client" ||
		!reflect.DeepEqual(result.Dependencies[0].Importers, []dependencies.Importer{sealed}) {
		t.Fatalf("classification = %#v", result)
	}
	if !strings.Contains(provider.user, `"package_path":"fmt"`) ||
		!strings.Contains(provider.user, `"package_path":"example.com/sdk/client"`) ||
		strings.Contains(provider.user, `"package_path":"example.com/app/internal"`) {
		t.Fatalf("request catalog = %s", provider.user)
	}
	encoded, err := Encode(result)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(encoded)
	if err != nil || !reflect.DeepEqual(decoded, result) {
		t.Fatalf("artifact round trip = %#v / %v", decoded, err)
	}
	digest, err := result.ArtifactSHA256()
	wantDigest := sha256.Sum256(encoded)
	if err != nil || digest != hex.EncodeToString(wantDigest[:]) {
		t.Fatalf("artifact digest = %q / %v", digest, err)
	}
	tampered := result
	tampered.Dependencies = append([]SelectedDependency(nil), result.Dependencies...)
	tampered.Dependencies[0].Dependency.Name = "invented"
	if err := tampered.ValidateAgainst(catalog); err == nil {
		t.Fatal("tampered restored dependency was accepted")
	}
}

func TestRunRejectsPartialCatalogBeforeModelCall(t *testing.T) {
	importer, err := dependencies.SealImporter(dependencies.Importer{
		Language: "python", Name: "app.main", ModulePath: "app",
		PackagePath: "app.main", RepositoryPath: "app",
	})
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := dependencies.BuildWithOmissions(
		[]dependencies.Importer{importer}, nil,
		[]dependencies.Omission{{
			ImporterRef: importer.Ref, ImporterPackagePath: importer.PackagePath,
			PackagePath: "dynamic.module", Reason: dependencies.OmissionDependencyIdentityMissing,
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Run(t.Context(), llm.Executor{Enabled: false}, nil, catalog); err == nil ||
		!strings.Contains(err.Error(), "dependency authority is incomplete") {
		t.Fatalf("partial catalog error = %v", err)
	}
}

func TestRunRejectsMissingSelectedRefArray(t *testing.T) {
	catalog := classifierCatalog(t, 1)
	for _, response := range [][]byte{
		[]byte(`{}`),
		[]byte(`{"integration_dependency_refs":null}`),
	} {
		provider := &classifierTestProvider{response: response}
		if _, err := Run(t.Context(), llm.Executor{Enabled: false}, provider, catalog); err == nil {
			t.Fatalf("accepted malformed response %s", response)
		}
	}
}

func TestRunBatchesByExactSerializedBytesAndRestoresGlobalRefs(t *testing.T) {
	catalog := largeClassifierCatalog(t, 3, 300_000)
	provider := &classifierTestProvider{responses: [][]byte{
		[]byte(`{"integration_dependency_refs":["d1"]}`),
		[]byte(`{"integration_dependency_refs":["d2"]}`),
		[]byte(`{"integration_dependency_refs":["d3"]}`),
	}}
	result, err := Run(t.Context(), llm.Executor{Enabled: false}, provider, catalog)
	if err != nil {
		t.Fatal(err)
	}
	if provider.calls != 3 || len(provider.users) != 3 {
		t.Fatalf("provider calls/prompts = %d/%d, want 3/3", provider.calls, len(provider.users))
	}
	if result.Coverage != (Coverage{
		Observed:   3,
		Advertised: 3,
		Omitted:    0, ModelCalled: true,
	}) {
		t.Fatalf("coverage = %#v", result.Coverage)
	}
	if len(result.Dependencies) != 3 {
		t.Fatalf("restored selections = %#v", result.Dependencies)
	}
	for index, user := range provider.users {
		if len(user) > maxSerializedUserBytes {
			t.Fatalf("batch %d serialized bytes = %d", index+1, len(user))
		}
		var payload observedRequest
		if err := json.Unmarshal([]byte(user), &payload); err != nil {
			t.Fatal(err)
		}
		wantRef := fmt.Sprintf("d%d", index+1)
		if payload.BatchIndex != index+1 || payload.BatchCount != 3 || payload.Observed != 3 ||
			payload.Omitted != 0 || len(payload.Catalog) != 1 || payload.Catalog[0].Ref != wantRef {
			t.Fatalf("batch %d request = %#v", index+1, payload)
		}
	}
}

func TestRunWithDeclarationsKeepsFrontierAndAuthoritiesSeparate(t *testing.T) {
	catalog := classifierCatalogForLanguage(t, "python", 1)
	target := classifierProgramTarget(t)
	declarations := classifierDeclarations(t, target, true)
	provider := &classifierTestProvider{response: []byte(
		`{"integration_dependency_refs":["d1"],"integration_declared_package_refs":["p1"]}`,
	)}
	result, err := RunWithDeclarations(
		t.Context(), llm.Executor{Enabled: false}, provider, catalog, declarations, target,
	)
	if err != nil {
		t.Fatal(err)
	}
	declarationSHA, err := declarations.ArtifactSHA256()
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Dependencies) != 1 || result.Declarations == nil ||
		result.Declarations.ArtifactSHA256 != declarationSHA ||
		len(result.Declarations.Packages) != 1 ||
		result.Declarations.Packages[0].NormalizedName != "boto3" ||
		result.Declarations.Coverage.Input.State != dependencydeclaration.CoverageFrontier ||
		result.Declarations.Coverage.Advertised != 2 || result.Declarations.Coverage.Selected != 1 {
		t.Fatalf("declaration-aware result = %#v", result)
	}
	if !strings.Contains(provider.user, `"observed_dependencies"`) ||
		!strings.Contains(provider.user, `"declared_packages"`) ||
		!strings.Contains(provider.user, `"target":{"language":"python","kind":"library"`) ||
		!strings.Contains(provider.user, `"name":"boto3"`) ||
		!strings.Contains(provider.user, `"state":"frontier"`) ||
		strings.Contains(provider.user, "ddpkg-") {
		t.Fatalf("declaration request = %s", provider.user)
	}
	if err := result.ValidateAgainstDeclarations(catalog, declarations, target); err != nil {
		t.Fatal(err)
	}
	if err := result.ValidateAgainst(catalog); err == nil ||
		!strings.Contains(err.Error(), "requires declaration authority") {
		t.Fatalf("observed-only validation error = %v", err)
	}
	encoded, err := Encode(result)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(encoded)
	if err != nil || !reflect.DeepEqual(decoded, result) {
		t.Fatalf("declaration artifact round trip = %#v / %v", decoded, err)
	}

	tampered := result
	selection := *result.Declarations
	selection.Packages = append([]SelectedDeclaredPackage(nil), result.Declarations.Packages...)
	selection.Packages[0].Name = "invented"
	tampered.Declarations = &selection
	if err := tampered.ValidateAgainstDeclarations(catalog, declarations, target); err == nil {
		t.Fatal("tampered declared projection was accepted")
	}
}

func TestRunWithDeclarationsRequiresBothSelectionArrays(t *testing.T) {
	catalog := classifierCatalogForLanguage(t, "python", 1)
	target := classifierProgramTarget(t)
	declarations := classifierDeclarations(t, target, false)
	for _, response := range [][]byte{
		[]byte(`{"integration_dependency_refs":[]}`),
		[]byte(`{"integration_dependency_refs":null,"integration_declared_package_refs":[]}`),
		[]byte(`{"integration_dependency_refs":[],"integration_declared_package_refs":null}`),
	} {
		provider := &classifierTestProvider{response: response}
		if _, err := RunWithDeclarations(
			t.Context(), llm.Executor{Enabled: false}, provider, catalog, declarations, target,
		); err == nil {
			t.Fatalf("accepted malformed declaration response %s", response)
		}
	}
}

func TestRunRejectsCatalogAboveGlobalBoundBeforeProvider(t *testing.T) {
	catalog := classifierCatalog(t, MaxAdvertisedDependencies+1)
	provider := &classifierTestProvider{response: []byte(`{"integration_dependency_refs":[]}`)}
	if _, err := Run(t.Context(), llm.Executor{Enabled: false}, provider, catalog); err == nil ||
		!strings.Contains(err.Error(), "complete run bound") {
		t.Fatalf("overflow error = %v", err)
	}
	if provider.calls != 0 {
		t.Fatalf("provider calls = %d, want 0", provider.calls)
	}
}

func classifierCatalog(t *testing.T, count int) dependencies.Catalog {
	return classifierCatalogForLanguage(t, "go", count)
}

func classifierCatalogForLanguage(t *testing.T, language string, count int) dependencies.Catalog {
	t.Helper()
	importer, err := dependencies.SealImporter(dependencies.Importer{
		Language: language, Name: "app", ModulePath: "example.com/app",
		PackagePath: "example.com/app", RepositoryPath: ".",
	})
	if err != nil {
		t.Fatal(err)
	}
	values := make([]dependencies.Dependency, 0, count)
	for index := 0; index < count; index++ {
		modulePath := fmt.Sprintf("example.com/dep%04d", index)
		values = append(values, dependencies.Dependency{
			Language: language, Kind: dependencies.KindExternal, Name: fmt.Sprintf("dep%04d", index),
			ModulePath: modulePath, PackagePath: modulePath + "/client",
			ImporterRefs: []string{importer.Ref},
		})
	}
	catalog, err := dependencies.BuildWithOmissions([]dependencies.Importer{importer}, values, nil)
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}

func largeClassifierCatalog(t *testing.T, count, segmentBytes int) dependencies.Catalog {
	t.Helper()
	importer, err := dependencies.SealImporter(dependencies.Importer{
		Language: "go", Name: "app", ModulePath: "example.com/app",
		PackagePath: "example.com/app", RepositoryPath: ".",
	})
	if err != nil {
		t.Fatal(err)
	}
	values := make([]dependencies.Dependency, 0, count)
	for index := 0; index < count; index++ {
		modulePath := fmt.Sprintf("example.com/dep%04d/", index) + strings.Repeat("x", segmentBytes)
		values = append(values, dependencies.Dependency{
			Language: "go", Kind: dependencies.KindExternal, Name: fmt.Sprintf("dep%04d", index),
			ModulePath: modulePath, PackagePath: modulePath + "/client",
			ImporterRefs: []string{importer.Ref},
		})
	}
	catalog, err := dependencies.BuildWithOmissions([]dependencies.Importer{importer}, values, nil)
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}

func classifierProgramTarget(t *testing.T) programindex.Target {
	t.Helper()
	index, err := programindex.New(programindex.Input{
		ScenarioSHA256: strings.Repeat("7", 64), SourceSHA256: strings.Repeat("8", 64),
		Target: programindex.TargetInput{
			Language: "python", Kind: "library", Name: "example library", Selector: "example",
			Sources:       []programindex.TargetSource{{FileRef: "f2", Path: "src/example/__init__.py"}},
			AnchorFileRef: "f2", Seeds: []programindex.TargetSeedInput{},
		},
		Objects: []programindex.ObjectInput{}, Relations: []programindex.RelationInput{},
		Coverage: programindex.CoverageInput{Measured: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	return index.Target
}

func classifierDeclarations(
	t *testing.T,
	target programindex.Target,
	frontier bool,
) dependencydeclaration.Result {
	t.Helper()
	frontiers := []dependencydeclaration.FrontierInput{}
	if frontier {
		frontiers = append(frontiers, dependencydeclaration.FrontierInput{
			SourceKey: "requirements", Kind: dependencydeclaration.FrontierDirective,
			Reason:  dependencydeclaration.FrontierUnsupportedOption,
			Section: "requirements", Ordinal: 3,
			ExpressionSHA256: strings.Repeat("e", 64),
		})
	}
	result, err := dependencydeclaration.Build(dependencydeclaration.Input{
		CorpusSHA256: strings.Repeat("a", 64), ProgramIndexSHA256: strings.Repeat("b", 64),
		TargetID: target.ID,
		Scope: dependencydeclaration.Scope{
			Language: "python", Ecosystem: "pypi", RepositoryPath: "",
			AuthoritySHA256: strings.Repeat("c", 64),
		},
		Sources: []dependencydeclaration.SourceInput{{
			Key: "requirements", FileRef: corpus.FileID("f1"), Path: "requirements.txt",
			Format: "requirements", State: dependencydeclaration.SourceParsed,
			ContentSHA256: strings.Repeat("d", 64), ByteCount: 32,
		}},
		Statements: []dependencydeclaration.StatementInput{
			{
				SourceKey: "requirements", Kind: dependencydeclaration.StatementRequirement,
				Role: dependencydeclaration.RoleRuntime, Name: "boto3", NormalizedName: "boto3",
				Extras: []string{}, Locator: dependencydeclaration.Locator{Kind: dependencydeclaration.LocatorRegistry},
				Section: "requirements", Ordinal: 1, ExpressionSHA256: strings.Repeat("1", 64),
			},
			{
				SourceKey: "requirements", Kind: dependencydeclaration.StatementRequirement,
				Role: dependencydeclaration.RoleTest, Group: "test", Name: "pytest", NormalizedName: "pytest",
				Extras: []string{}, Locator: dependencydeclaration.Locator{Kind: dependencydeclaration.LocatorRegistry},
				Section: "requirements", Ordinal: 2, ExpressionSHA256: strings.Repeat("2", 64),
			},
		},
		Includes: []dependencydeclaration.IncludeInput{}, Frontiers: frontiers,
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

type classifierTestProvider struct {
	response  []byte
	responses [][]byte
	user      string
	users     []string
	calls     int
}

func (provider *classifierTestProvider) State() []byte {
	return []byte(`{"endpoint":"https://provider.test/v1/chat","model":"fixture"}`)
}

func (provider *classifierTestProvider) Prepare(prompt llm.Prompt, _ llm.Limits) (llm.Prepared, error) {
	provider.user = prompt.User
	provider.users = append(provider.users, prompt.User)
	return llm.NewPrepared([]byte(prompt.System + "\n" + prompt.User))
}

func (provider *classifierTestProvider) Complete(context.Context, llm.Prepared) (llm.Completion, error) {
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
		Metrics: llm.Metrics{
			InputTokens: 10, OutputTokens: 5, ProviderResponseBytes: len(response),
			UsageReported: true, Latency: time.Millisecond, Attempts: 1,
		},
	}, nil
}
