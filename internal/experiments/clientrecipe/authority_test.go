package clientrecipe

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"golang.org/x/tools/go/packages"
)

type countingProductionLoader struct {
	count    int
	delegate productionPackageLoader
}

func (loader *countingProductionLoader) Load(ctx context.Context, root string) ([]*packages.Package, error) {
	loader.count++
	return loader.delegate.Load(ctx, root)
}

var (
	fixtureAuthorityOnce sync.Once
	fixtureAuthority     Authority
	fixtureAuthorityErr  error
)

func TestPrepareAuthorityDeterministicAndSingleLoad(t *testing.T) {
	repoRoot := filepath.Join(experimentRoot(t), "repo")
	firstLoader := &countingProductionLoader{delegate: defaultProductionPackageLoader{}}
	first, err := prepareAuthority(t.Context(), repoRoot, firstLoader)
	if err != nil {
		t.Fatal(err)
	}
	secondLoader := &countingProductionLoader{delegate: defaultProductionPackageLoader{}}
	second, err := prepareAuthority(t.Context(), repoRoot, secondLoader)
	if err != nil {
		t.Fatal(err)
	}
	if firstLoader.count != 1 || secondLoader.count != 1 {
		t.Fatalf("production package loads = %d and %d, want exactly one per run", firstLoader.count, secondLoader.count)
	}
	firstRaw, err := EncodeAuthority(first)
	if err != nil {
		t.Fatal(err)
	}
	secondRaw, err := EncodeAuthority(second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstRaw, secondRaw) {
		t.Fatal("authority bytes changed across identical runs")
	}
	if bytes.Contains(bytes.ToLower(firstRaw), []byte("oracle")) {
		t.Fatal("authority serialized evaluator-only identity")
	}
	if _, err := DecodeAuthority(firstRaw); err != nil {
		t.Fatal(err)
	}
	testFacts := 0
	for _, source := range first.Sources {
		if source.Class == SourceTest {
			testFacts++
			if source.ProductionCode {
				t.Fatalf("test source %s became production code", source.Path)
			}
		}
	}
	if testFacts == 0 {
		t.Fatal("authority did not retain separately classified test source facts")
	}
	for _, source := range first.Program.Target.Sources {
		if strings.HasSuffix(source.Path, "_test.go") {
			t.Fatalf("test source %s entered the production ProgramIndex target", source.Path)
		}
	}
	for _, object := range first.Program.Objects {
		if object.Location != nil && strings.HasSuffix(object.Location.Path, "_test.go") {
			t.Fatalf("test object %s entered production ProgramIndex reachability", object.ID)
		}
	}
	assertExperimentGolden(t, "01-input-authority.json", firstRaw)
}

func TestAuthorityCallbackFrontier(t *testing.T) {
	authority := preparedFixtureAuthority(t)
	if authority.Callbacks.Status != "frontier" || authority.Callbacks.ExactPassRelations == 0 ||
		authority.Callbacks.UnresolvedInvocations == 0 {
		t.Fatalf("callback coverage = %#v, want exact pass plus unresolved invocation frontier", authority.Callbacks)
	}
	foundPass := false
	foundInvocation := false
	foundLiveRun := false
	for _, relation := range authority.Program.Relations {
		for _, witness := range relation.Witnesses {
			switch {
			case witness.SourceExpression == "service.Resolve":
				foundPass = relation.Kind == "passes_callback" && relation.Resolution == "exact"
			case strings.Contains(witness.SourceExpression, "handler.resolve") &&
				strings.Contains(witness.SourceExpression, "HistoryLimit"):
				foundInvocation = relation.Kind == "calls" && relation.Resolution == "unresolved"
			case witness.SourceExpression == "service.Run(context.Background())":
				foundLiveRun = relation.Kind == "calls" && relation.Resolution == "exact"
			}
		}
	}
	if !foundPass || !foundInvocation || !foundLiveRun {
		t.Fatalf("callback/run authority: pass=%t invocation=%t live_run=%t", foundPass, foundInvocation, foundLiveRun)
	}
}

func TestAuthorityRetainsExactConsumerInterfaceCalls(t *testing.T) {
	authority := preparedFixtureAuthority(t)
	objects := make(map[string]struct {
		kind  string
		owner string
	}, len(authority.Program.Objects))
	for _, object := range authority.Program.Objects {
		objects[object.ID] = struct {
			kind  string
			owner string
		}{kind: string(object.Kind), owner: object.OwnerID}
	}
	wantLines := map[int]bool{38: false, 42: false, 45: false, 48: false}
	for _, relation := range authority.Program.Relations {
		if relation.Location == nil || relation.Location.Path != "internal/launch/service.go" ||
			relation.Kind != "calls" || relation.Resolution != "exact" || len(relation.ToIDs) != 1 {
			continue
		}
		if _, wanted := wantLines[relation.Location.Line]; !wanted {
			continue
		}
		target := objects[relation.ToIDs[0]]
		if target.kind != "method" || target.owner == "" {
			t.Fatalf("consumer call at line %d targets %#v, want an interface-owned method", relation.Location.Line, target)
		}
		wantLines[relation.Location.Line] = true
	}
	for line, found := range wantLines {
		if !found {
			t.Errorf("missing exact interface call at internal/launch/service.go:%d", line)
		}
	}
}

func TestH0Baseline(t *testing.T) {
	authority := preparedFixtureAuthority(t)
	result, err := BuildH0(authority)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := EncodeH0(result)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(bytes.ToLower(raw), []byte("oracle")) {
		t.Fatal("H0 serialized evaluator-only identity")
	}
	for _, forbidden := range []string{`"role"`, `"configuration"`, `"application_wiring"`, `"verification"`, `"observability"`} {
		if bytes.Contains(raw, []byte(forbidden)) {
			t.Fatalf("H0 owns unsupported semantic field %s", forbidden)
		}
	}
	generatedKubernetes := false
	productionKubernetes := false
	legacy := false
	for _, candidate := range result.Candidates {
		switch candidate.ImporterRepositoryPath {
		case "internal/gen":
			if candidate.PackagePath == "example.com/kubernetessdk" {
				generatedKubernetes = true
				if len(candidate.Calls) != 0 {
					t.Fatalf("generated importer inherited another importer's callsites: %#v", candidate.Calls)
				}
			}
		case "internal/clients/kubernetes":
			if candidate.PackagePath == "example.com/kubernetessdk" {
				productionKubernetes = len(candidate.Calls) > 0
			}
		case "internal/legacy":
			if candidate.PackagePath == "example.com/legacysdk" {
				legacy = true
			}
		}
	}
	if !generatedKubernetes || !productionKubernetes || !legacy {
		t.Fatalf("H0 did not retain its expected blind spots: generated=%t production=%t legacy=%t",
			generatedKubernetes, productionKubernetes, legacy)
	}
	if _, err := DecodeH0(raw); err != nil {
		t.Fatal(err)
	}
	if err := result.ValidateAgainst(authority); err != nil {
		t.Fatal(err)
	}
	assertExperimentGolden(t, "02-h0-candidates.json", raw)
}

func preparedFixtureAuthority(t *testing.T) Authority {
	t.Helper()
	fixtureAuthorityOnce.Do(func() {
		fixtureAuthority, fixtureAuthorityErr = PrepareAuthority(filepath.Join(experimentRoot(t), "repo"))
	})
	if fixtureAuthorityErr != nil {
		t.Fatal(fixtureAuthorityErr)
	}
	return fixtureAuthority
}

func assertExperimentGolden(t *testing.T, name string, actual []byte) {
	t.Helper()
	filename := filepath.Join(experimentRoot(t), "golden", name)
	if os.Getenv("REPOMAP_UPDATE_EXPERIMENT_GOLDEN") == "1" {
		if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filename, actual, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("read golden %s: %v; update with REPOMAP_UPDATE_EXPERIMENT_GOLDEN=1", name, err)
	}
	if !bytes.Equal(actual, want) {
		t.Fatalf("golden %s changed; inspect before REPOMAP_UPDATE_EXPERIMENT_GOLDEN=1", name)
	}
}

func TestAuthorityHasNoEvaluatorPath(t *testing.T) {
	for _, filename := range []string{"authority.go", "go_authority.go", "h0.go"} {
		raw := readExperimentFile(t, filepath.Join(filepath.Dir(sourceFile(t)), filename))
		lower := strings.ToLower(string(raw))
		for _, forbidden := range []string{"oracle", "cmd/service/main.go", "launch-service"} {
			if strings.Contains(lower, forbidden) {
				t.Fatalf("extractor/H0 implementation %s contains evaluator-specific literal %q", filename, forbidden)
			}
		}
	}
}

func sourceFile(t *testing.T) string {
	t.Helper()
	return filepath.Join(experimentRoot(t), "..", "..", "..", "internal", "experiments", "clientrecipe", "authority_test.go")
}
