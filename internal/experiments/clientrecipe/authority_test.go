package clientrecipe

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/dvordrova/repomap/internal/programindex"
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

func TestBlindAuthorityRelationObservationLedger(t *testing.T) {
	repoRoot := filepath.Clean(filepath.Join(experimentRoot(t), "..", "clientrecipe-blind", "repo"))
	sources, repositorySHA256, err := prepareSourceFacts(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := (defaultProductionPackageLoader{}).Load(t.Context(), repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	packagesByPath, modulePath, err := productionPackages(loaded)
	if err != nil {
		t.Fatal(err)
	}
	var before, after []programindex.RelationInput
	index, sealErr := buildProgramIndexWithSealObserver(
		repoRoot, modulePath, repositorySHA256, packagesByPath, sources,
		func(legacy, disambiguated []programindex.RelationInput) error {
			before = cloneRelationInputs(legacy)
			after = cloneRelationInputs(disambiguated)
			return nil
		},
	)
	if sealErr != nil {
		t.Fatalf("blind ProgramIndex did not seal: %v", sealErr)
	}
	collisions := relationObservationCollisions(before)
	if len(collisions) != 3 {
		t.Fatalf("legacy relation collisions = %d, want the three diagnosed nested-call collisions: %#v", len(collisions), collisions)
	}
	wantCollisionLines := map[int]bool{15: false, 24: false, 25: false}
	for _, collision := range collisions {
		if collision.Classification != "distinct_semantic_observations_collapsed_by_source_ref" ||
			len(collision.StructuralTuples) != 2 || collision.StructuralTuples[0] == collision.StructuralTuples[1] {
			t.Errorf("legacy collision was not two distinct semantic observations: %#v", collision)
		}
		for line := range wantCollisionLines {
			legacySourceRef := fmt.Sprintf("relation:internal/httpapi/server.go:%d:", line)
			if strings.HasPrefix(collision.LegacyKey, legacySourceRef) {
				wantCollisionLines[line] = true
			}
		}
	}
	for line, found := range wantCollisionLines {
		if !found {
			t.Errorf("legacy collision ledger missed internal/httpapi/server.go:%d", line)
		}
	}
	if collisionsAfter := relationObservationCollisions(after); len(collisionsAfter) != 0 {
		t.Fatalf("relation collisions survived disambiguation: %#v", collisionsAfter)
	}
	if len(index.Relations) != len(before) || len(after) != len(before) {
		t.Fatalf("relation counts before=%d after=%d sealed=%d; an observation was dropped", len(before), len(after), len(index.Relations))
	}
	newRefs := make(map[string]struct{}, len(after))
	for _, relation := range after {
		if _, duplicate := newRefs[relation.SourceRef]; duplicate {
			t.Errorf("complete structural observation ref is not unique: %s", relation.SourceRef)
		}
		newRefs[relation.SourceRef] = struct{}{}
	}

	// These are the three nested/fluent expressions that exposed the old
	// file:line:column:kind collision. Each callsite is one AST position but
	// contains two distinct exact external call observations.
	for _, line := range []int{15, 24, 25} {
		refs := make(map[string]struct{})
		targets := make(map[string]struct{})
		for _, relation := range after {
			if relation.Location == nil || relation.Location.Path != "internal/httpapi/server.go" ||
				relation.Location.Line != line || relation.Kind != programindex.RelationInvokesExternal {
				continue
			}
			refs[relation.SourceRef] = struct{}{}
			for _, target := range relation.ToRefs {
				targets[target] = struct{}{}
			}
		}
		if len(refs) != 2 || len(targets) != 2 {
			t.Errorf("nested callsite line %d retained refs=%d targets=%d, want two distinct semantic observations", line, len(refs), len(targets))
		}
	}

	authority, err := PrepareAuthority(repoRoot)
	if err != nil {
		t.Fatalf("blind PrepareAuthority did not seal: %v", err)
	}
	if err := authority.Validate(); err != nil {
		t.Fatalf("blind authority is not valid after sealing: %v", err)
	}
}

func TestRelationCollisionDisambiguationBindsCompleteStructuralTuple(t *testing.T) {
	location := programindex.Location{Path: "internal/client.go", Line: 12, Column: 3}
	base := programindex.RelationInput{
		Kind: programindex.RelationCalls, FromRef: "local:example/client:internal/client.go:10:Run",
		ToRefs:     []string{"local:example/client:internal/client.go:20:send"},
		Resolution: programindex.ResolutionExact, Location: &location, TargetsObserved: 1,
		Witnesses: []programindex.Witness{{Kind: "call", SourceExpression: "client.send()", Location: &location}}, WitnessesObserved: 1,
	}
	base.SourceRef = legacyRelationSourceRef(base)
	baseTuple := relationObservationTuple(base)
	mutations := map[string]func(programindex.RelationInput) programindex.RelationInput{
		"producer": func(value programindex.RelationInput) programindex.RelationInput {
			value.FromRef += ":other"
			return value
		},
		"kind": func(value programindex.RelationInput) programindex.RelationInput {
			value.Kind = programindex.RelationInvokesExternal
			return value
		},
		"resolution": func(value programindex.RelationInput) programindex.RelationInput {
			value.Resolution = programindex.ResolutionAlternatives
			return value
		},
		"callsite": func(value programindex.RelationInput) programindex.RelationInput {
			changed := *value.Location
			changed.Column++
			value.Location = &changed
			return value
		},
		"target refs": func(value programindex.RelationInput) programindex.RelationInput {
			value.ToRefs = []string{"local:example/client:internal/client.go:21:other"}
			return value
		},
		"target count": func(value programindex.RelationInput) programindex.RelationInput {
			value.TargetsObserved = 2
			return value
		},
		"witness kind": func(value programindex.RelationInput) programindex.RelationInput {
			value.Witnesses = []programindex.Witness{{Kind: "callback_argument", SourceExpression: "client.send()", Location: &location}}
			return value
		},
		"witness expression": func(value programindex.RelationInput) programindex.RelationInput {
			value.Witnesses = []programindex.Witness{{Kind: "call", SourceExpression: "client.sendConfigured()", Location: &location}}
			return value
		},
		"witness count": func(value programindex.RelationInput) programindex.RelationInput {
			value.WitnessesObserved = 2
			return value
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			if changed := relationObservationTuple(mutate(base)); changed == baseTuple {
				t.Fatalf("%s is absent from relation observation identity", name)
			}
		})
	}
	if singleton := disambiguateRelationObservations([]programindex.RelationInput{base}); singleton[0].SourceRef != base.SourceRef {
		t.Fatal("singleton legacy relation identity changed")
	}
	duplicates := disambiguateRelationObservations([]programindex.RelationInput{base, base})
	if duplicates[0].SourceRef != base.SourceRef || duplicates[1].SourceRef != base.SourceRef {
		t.Fatal("identical duplicate traversal was hidden by disambiguation")
	}
	different := mutations["target refs"](base)
	distinct := disambiguateRelationObservations([]programindex.RelationInput{base, different})
	if distinct[0].SourceRef == base.SourceRef || distinct[1].SourceRef == base.SourceRef ||
		distinct[0].SourceRef == distinct[1].SourceRef {
		t.Fatal("distinct observations sharing a legacy identity were not separated")
	}
}

type relationObservationCollision struct {
	LegacyKey        string
	Classification   string
	StructuralTuples []string
}

func relationObservationCollisions(relations []programindex.RelationInput) []relationObservationCollision {
	byLegacyIdentity := make(map[string][]string)
	for _, relation := range relations {
		key := strings.Join([]string{relation.SourceRef, string(relation.Kind), relation.FromRef}, "\x00")
		byLegacyIdentity[key] = append(byLegacyIdentity[key], relationObservationStructuralTuple(relation))
	}
	keys := make([]string, 0, len(byLegacyIdentity))
	for key, tuples := range byLegacyIdentity {
		if len(tuples) > 1 {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	result := make([]relationObservationCollision, 0, len(keys))
	for _, key := range keys {
		tuples := append([]string(nil), byLegacyIdentity[key]...)
		sort.Strings(tuples)
		classification := "duplicate_traversal_observation"
		for _, tuple := range tuples[1:] {
			if tuple != tuples[0] {
				classification = "distinct_semantic_observations_collapsed_by_source_ref"
				break
			}
		}
		result = append(result, relationObservationCollision{
			LegacyKey: key, Classification: classification, StructuralTuples: tuples,
		})
	}
	return result
}

func relationObservationStructuralTuple(relation programindex.RelationInput) string {
	return relationObservationTuple(relation)
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
