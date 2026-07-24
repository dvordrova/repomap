package inspection

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/dvordrova/repomap/internal/analyzer"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/sourcecatalog"
)

type resolverFunc func(context.Context, analyzer.LocationRequest) (analyzer.LocationResolution, error)

func (function resolverFunc) ResolveLocation(
	ctx context.Context,
	request analyzer.LocationRequest,
) (analyzer.LocationResolution, error) {
	return function(ctx, request)
}

type exactAnalyzerFunc func(context.Context, analyzer.ExactSymbolRequest) (evidence.Graph, error)

func (function exactAnalyzerFunc) AnalyzeExactSymbol(
	ctx context.Context,
	request analyzer.ExactSymbolRequest,
) (evidence.Graph, error) {
	return function(ctx, request)
}

type referenceFinderFunc func(context.Context, string, evidence.Location) (evidence.LocationSet, error)

func (function referenceFinderFunc) References(
	ctx context.Context,
	root string,
	location evidence.Location,
) (evidence.LocationSet, error) {
	return function(ctx, root, location)
}

func TestResolveAuthorizesFiltersBoundsAndDefensivelyCopies(t *testing.T) {
	t.Parallel()

	fixture := newInspectionFixture(t, false)
	target := fixtureTarget()
	requests := make([]analyzer.LocationRequest, 0, 2)
	resolution := analyzer.LocationResolution{
		Location: evidence.Location{Path: "main.go", Line: 3},
		Candidates: []analyzer.LocationCandidate{
			{
				Entity:       evidence.Entity{ID: "type", Kind: evidence.EntityType, Name: "ignored", Location: &evidence.Location{Path: "main.go", Line: 2, Column: 1}},
				Certainty:    evidence.CertaintyPossible,
				Investigable: false,
			},
			{
				Entity:       target,
				Match:        "file_callable",
				Certainty:    evidence.CertaintyPossible,
				Distance:     2,
				Investigable: true,
				RankReasons:  []string{"component term 'target'", fixture.root},
			},
			{
				Entity:       evidence.Entity{ID: "outside", Kind: evidence.EntityFunction, Name: "Outside", Location: &evidence.Location{Path: "outside.go", Line: 4, Column: 1}},
				Match:        "file_callable",
				Certainty:    evidence.CertaintyPossible,
				Investigable: true,
			},
		},
		Certainty: evidence.CertaintyStatic,
		Provenance: evidence.Provenance{
			Provider: "fake", Version: "v1", Operation: "resolve", Detail: fixture.root,
			Location: &evidence.Location{Path: "main.go", Line: 3},
		},
		Scenario: evidence.Scenario{ID: "build", Name: "fake", WorkingDir: fixture.root},
		Warnings: []string{"safe warning", "path leaked: " + fixture.root},
	}
	service := mustService(t, fixture.catalog, Dependencies{
		Resolver: resolverFunc(func(_ context.Context, request analyzer.LocationRequest) (analyzer.LocationResolution, error) {
			requests = append(requests, request)
			return resolution, nil
		}),
	})
	terms := make([]string, 20)
	for index := range terms {
		terms[index] = fmt.Sprintf("term-%02d", index)
	}
	result, err := service.Resolve(context.Background(), ResolveRequest{
		Location:  evidence.Location{Path: "main.go", Line: 3},
		RankTerms: terms,
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(result.Candidates) != 1 || !reflect.DeepEqual(result.Candidates[0].Entity, target) ||
		!reflect.DeepEqual(result.Candidates[0].RankReasons, []string{"component term 'target'"}) {
		t.Fatalf("candidates = %#v", result.Candidates)
	}
	if len(result.Warnings) != 1 || result.Warnings[0] != "safe warning" ||
		result.Provenance.Detail != "" || result.Provenance.Location == resolution.Provenance.Location {
		t.Fatalf("sanitized result = %#v", result)
	}
	if len(requests) != 1 || requests[0].RepoPath != fixture.root ||
		requests[0].MaxCandidates != 20 || len(requests[0].RankTerms) != 16 {
		t.Fatalf("resolver request = %#v", requests)
	}

	result.Candidates[0].Entity.Location.Path = "mutated.go"
	result.Candidates[0].RankReasons[0] = "mutated"
	second, err := service.Resolve(context.Background(), ResolveRequest{
		Location: evidence.Location{Path: "main.go", Line: 3},
	})
	if err != nil {
		t.Fatalf("second Resolve: %v", err)
	}
	if second.Candidates[0].Entity.Location.Path != "main.go" ||
		second.Candidates[0].RankReasons[0] != "component term 'target'" {
		t.Fatalf("resolver-owned evidence was mutated: %#v", second.Candidates[0])
	}
}

func TestResolveRejectsUnauthorizedAndOutOfScopeCandidates(t *testing.T) {
	t.Parallel()

	fixture := newInspectionFixture(t, false)
	calls := 0
	service := mustService(t, fixture.catalog, Dependencies{
		Resolver: resolverFunc(func(_ context.Context, request analyzer.LocationRequest) (analyzer.LocationResolution, error) {
			calls++
			return analyzer.LocationResolution{
				Location: request.Location,
				Candidates: []analyzer.LocationCandidate{{
					Entity:       evidence.Entity{ID: "outside", Kind: evidence.EntityFunction, Name: "Outside", Location: &evidence.Location{Path: "outside.go", Line: 1, Column: 1}},
					Investigable: true,
					Certainty:    evidence.CertaintyPossible,
				}},
				Certainty:  evidence.CertaintyStatic,
				Provenance: evidence.Provenance{Provider: "fake", Operation: "resolve"},
			}, nil
		}),
	})
	_, err := service.Resolve(context.Background(), ResolveRequest{
		Location: evidence.Location{Path: "../main.go", Line: 1},
	})
	if ErrorKindOf(err) != ErrorUnauthorized || calls != 0 {
		t.Fatalf("unauthorized err=%v calls=%d", err, calls)
	}
	_, err = service.Resolve(context.Background(), ResolveRequest{
		Location: evidence.Location{Path: "main.go", Line: 3},
	})
	if ErrorKindOf(err) != ErrorNotFound || calls != 1 {
		t.Fatalf("out-of-scope err=%v calls=%d", err, calls)
	}
}

func TestResolvePreservesDeterministicCandidateOrderAndBound(t *testing.T) {
	t.Parallel()

	fixture := newInspectionFixture(t, false)
	candidates := make([]analyzer.LocationCandidate, 0, 12)
	for index := 0; index < 12; index++ {
		candidates = append(candidates, analyzer.LocationCandidate{
			Entity: evidence.Entity{
				ID: fmt.Sprintf("candidate-%02d", index), Kind: evidence.EntityFunction,
				Name:     fmt.Sprintf("Candidate%02d", index),
				Location: &evidence.Location{Path: "main.go", Line: index + 1, Column: 1},
			},
			Match: "file_callable", Certainty: evidence.CertaintyPossible, Investigable: true,
		})
	}
	service := mustService(t, fixture.catalog, Dependencies{
		Resolver: resolverFunc(func(_ context.Context, request analyzer.LocationRequest) (analyzer.LocationResolution, error) {
			return analyzer.LocationResolution{
				Location: request.Location, Candidates: candidates,
				Certainty:  evidence.CertaintyStatic,
				Provenance: evidence.Provenance{Provider: "fake", Operation: "resolve"},
			}, nil
		}),
	})
	result, err := service.Resolve(context.Background(), ResolveRequest{
		Location: evidence.Location{Path: "main.go", Line: 3},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Candidates) != 8 {
		t.Fatalf("candidate count = %d", len(result.Candidates))
	}
	for index, candidate := range result.Candidates {
		if candidate.Entity.ID != fmt.Sprintf("candidate-%02d", index) {
			t.Fatalf("candidate order = %#v", result.Candidates)
		}
	}
}

func TestResolveSupportsPortableResolverEvidence(t *testing.T) {
	t.Parallel()

	root := canonicalTempDir(t)
	path := "service.py"
	if err := os.WriteFile(filepath.Join(root, path), []byte("def work():\n    return 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	catalog := newCatalog(t, root, map[string][]byte{
		path: []byte("def work():\n    return 1\n"),
	})
	service := mustService(t, catalog, Dependencies{
		Resolver: resolverFunc(func(_ context.Context, request analyzer.LocationRequest) (analyzer.LocationResolution, error) {
			return analyzer.LocationResolution{
				Location: request.Location,
				Candidates: []analyzer.LocationCandidate{{
					Entity: evidence.Entity{
						ID: "python:function:work", Kind: evidence.EntityFunction, Name: "work", Language: "python",
						Location: &evidence.Location{Path: path, Line: 1, Column: 1},
					},
					Investigable: true, Certainty: evidence.CertaintyStatic,
				}},
				Certainty:  evidence.CertaintyStatic,
				Provenance: evidence.Provenance{Provider: "fake-python", Operation: "document_symbols"},
			}, nil
		}),
	})
	result, err := service.Resolve(context.Background(), ResolveRequest{
		Location: evidence.Location{Path: path, Line: 1},
	})
	if err != nil || len(result.Candidates) != 1 || result.Candidates[0].Entity.Language != "python" {
		t.Fatalf("portable resolver result=%#v err=%v", result, err)
	}
}

func TestResolveRejectsChangedSourceBeforeAnalyzer(t *testing.T) {
	t.Parallel()

	fixture := newInspectionFixture(t, false)
	calls := 0
	service := mustService(t, fixture.catalog, Dependencies{
		Resolver: resolverFunc(func(_ context.Context, request analyzer.LocationRequest) (analyzer.LocationResolution, error) {
			calls++
			return analyzer.LocationResolution{}, nil
		}),
	})
	if err := os.WriteFile(filepath.Join(fixture.root, "main.go"), []byte("package changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := service.Resolve(context.Background(), ResolveRequest{
		Location: evidence.Location{Path: "main.go", Line: 3},
	})
	if ErrorKindOf(err) != ErrorSourceChanged || calls != 0 || strings.Contains(err.Error(), fixture.root) {
		t.Fatalf("changed-source err=%v calls=%d", err, calls)
	}
}

func TestResolveBoundsAndDropsUnsafeAnalyzerMetadata(t *testing.T) {
	t.Parallel()

	fixture := newInspectionFixture(t, false)
	reasons := []string{"trace=/private/tmp/analyzer-source.go"}
	for index := 0; index < 10; index++ {
		reasons = append(reasons, fmt.Sprintf("reason-%02d", index))
	}
	service := mustService(t, fixture.catalog, Dependencies{
		Resolver: resolverFunc(func(_ context.Context, request analyzer.LocationRequest) (analyzer.LocationResolution, error) {
			return analyzer.LocationResolution{
				Location: request.Location,
				Candidates: []analyzer.LocationCandidate{{
					Entity:       fixtureTarget(),
					Match:        "file_callable",
					Certainty:    evidence.CertaintyStatic,
					Investigable: true,
					RankReasons:  reasons,
				}},
				Certainty:  evidence.CertaintyStatic,
				Provenance: evidence.Provenance{Provider: "fake", Operation: "resolve"},
				Warnings:   []string{"safe warning", "trace=/private/tmp/analyzer-source.go"},
			}, nil
		}),
	})
	result, err := service.Resolve(context.Background(), ResolveRequest{
		Location: evidence.Location{Path: "main.go", Line: 3},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Candidates) != 1 || len(result.Candidates[0].RankReasons) != maxRankReasons ||
		len(result.Warnings) != 1 || result.Warnings[0] != "safe warning" {
		t.Fatalf("bounded metadata result = %#v", result)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "/private/tmp") {
		t.Fatalf("result leaked embedded absolute path: %s", encoded)
	}
}

func TestInspectReturnsAuthorizedBoundedSourceCallsAndReferences(t *testing.T) {
	t.Parallel()

	fixture := newInspectionFixture(t, true)
	target := fixtureTarget()
	exactRequests := make([]analyzer.ExactSymbolRequest, 0, 1)
	service := mustService(t, fixture.catalog, Dependencies{
		ExactAnalyzer: exactAnalyzerFunc(func(_ context.Context, request analyzer.ExactSymbolRequest) (evidence.Graph, error) {
			exactRequests = append(exactRequests, request)
			return fixtureGraph(fixture.root, target, true), nil
		}),
		ReferenceFinder: referenceFinderFunc(func(_ context.Context, root string, location evidence.Location) (evidence.LocationSet, error) {
			if root != fixture.root || !reflect.DeepEqual(location, *target.Location) {
				t.Fatalf("reference request root=%q location=%#v", root, location)
			}
			return evidence.LocationSet{
				Locations: []evidence.Location{
					{Path: "main_test.go", Line: 3, Column: 1},
					{Path: "outside_test.go", Line: 9, Column: 1},
					{Path: "main.go", Line: 10, Column: 2},
					{Path: filepath.Join(fixture.root, "leak.go"), Line: 1, Column: 1},
				},
				Certainty: evidence.CertaintyStatic,
				Provenance: []evidence.Provenance{{
					Provider: "fake", Operation: "references", Detail: fixture.root,
					Location: &evidence.Location{Path: "main.go", Line: 3, Column: 1},
				}},
				Scenarios: []evidence.Scenario{{
					ID: "build", Name: "active build", WorkingDir: fixture.root,
					Command: []string{"fake", fixture.root}, Env: map[string]string{"SECRET": fixture.root},
				}},
			}, nil
		}),
	})
	result, err := service.Inspect(context.Background(), InspectRequest{
		Target: target, IncludeReferences: true,
	})
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if len(exactRequests) != 1 || exactRequests[0].RepoPath != fixture.root ||
		!reflect.DeepEqual(exactRequests[0].Symbol, target) {
		t.Fatalf("exact requests = %#v", exactRequests)
	}
	if result.Structural.Target.Entity.Name != "Target" ||
		len(result.Structural.IncomingCalls) != 1 ||
		result.Structural.IncomingCalls[0].Caller.Name != "caller" ||
		len(result.Structural.OutgoingCalls) != 1 ||
		result.Structural.OutgoingCalls[0].Callee.Name != "callee" {
		t.Fatalf("structural = %#v", result.Structural)
	}
	if result.Source.Target.Path != "main.go" || result.Source.Window.StartLine != 3 ||
		result.Source.Window.EndLine != 6 || len(result.Source.Lines) != 4 ||
		result.Source.FileSHA256 != fixture.hashes["main.go"] {
		t.Fatalf("source = %#v", result.Source)
	}
	if result.References == nil || len(result.References.Locations) != 1 ||
		result.References.Locations[0].Path != "main.go" ||
		result.Tests == nil || len(result.Tests.Locations) != 1 ||
		result.Tests.Locations[0].Path != "main_test.go" {
		t.Fatalf("references=%#v tests=%#v", result.References, result.Tests)
	}
	if result.References.Scenarios[0].WorkingDir != "" ||
		len(result.References.Scenarios[0].Command) != 0 ||
		len(result.References.Scenarios[0].Env) != 0 ||
		result.References.Provenance[0].Detail != "" {
		t.Fatalf("reference context leaked: %#v", result.References)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), fixture.root) {
		t.Fatalf("result leaked absolute root: %s", encoded)
	}

	result.Source.Lines[0].Text = "mutated"
	result.References.Locations[0].Path = "mutated.go"
	second, err := service.Inspect(context.Background(), InspectRequest{
		Target: target, IncludeReferences: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if second.Source.Lines[0].Text == "mutated" || second.References.Locations[0].Path != "main.go" {
		t.Fatalf("result was not defensively reconstructed: %#v", second)
	}
}

func TestInspectBoundsReferenceMetadataAndAggregateProvenance(t *testing.T) {
	t.Parallel()

	fixture := newInspectionFixture(t, true)
	target := fixtureTarget()
	provenance := make([]evidence.Provenance, 0, 7)
	scenarios := make([]evidence.Scenario, 0, 7)
	for index := 0; index < 7; index++ {
		provenance = append(provenance, evidence.Provenance{
			Provider: "fake", Operation: fmt.Sprintf("references-%d", index),
		})
		scenarios = append(scenarios, evidence.Scenario{
			ID: fmt.Sprintf("build-%d", index), Name: fmt.Sprintf("build %d", index),
		})
	}
	service := mustService(t, fixture.catalog, Dependencies{
		ExactAnalyzer: exactAnalyzerFunc(func(context.Context, analyzer.ExactSymbolRequest) (evidence.Graph, error) {
			return fixtureGraph(fixture.root, target, false), nil
		}),
		ReferenceFinder: referenceFinderFunc(func(context.Context, string, evidence.Location) (evidence.LocationSet, error) {
			return evidence.LocationSet{
				Locations:  []evidence.Location{{Path: "main.go", Line: 10, Column: 1}},
				Certainty:  evidence.CertaintyStatic,
				Provenance: provenance,
				Scenarios:  scenarios,
			}, nil
		}),
	})
	result, err := service.Inspect(context.Background(), InspectRequest{
		Target: target, IncludeReferences: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.References == nil || result.Tests == nil ||
		len(result.References.Provenance) != maxReferenceProvenance ||
		len(result.References.Scenarios) != maxReferenceScenarios ||
		len(result.Tests.Provenance) != maxReferenceProvenance ||
		len(result.Tests.Scenarios) != maxReferenceScenarios ||
		len(result.Provenance) != maxAggregateProvenance {
		t.Fatalf("metadata bounds were not applied: %#v", result)
	}
}

func TestInspectDropsCallerAndCalleeAbsoluteMetadata(t *testing.T) {
	t.Parallel()

	fixture := newInspectionFixture(t, false)
	target := fixtureTarget()
	graph := fixtureGraph(fixture.root, target, false)
	const knownWarning = "gopls CLI adapter is experimental; evidence is scoped to the active build configuration"
	graph.Warnings = []string{knownWarning, "gopls failed at key=/private/tmp/gopls.log"}
	oldCalleeID := "callee"
	unsafeCalleeID := "callee key=/private/tmp/callee.go"
	for index := range graph.Entities {
		switch graph.Entities[index].ID {
		case "caller":
			graph.Entities[index].Name = "caller key=/private/tmp/caller.go"
		case oldCalleeID:
			graph.Entities[index].ID = unsafeCalleeID
		}
	}
	for index := range graph.Relations {
		if graph.Relations[index].From == oldCalleeID {
			graph.Relations[index].From = unsafeCalleeID
		}
		if graph.Relations[index].To == oldCalleeID {
			graph.Relations[index].To = unsafeCalleeID
		}
	}
	graph.Sort()
	service := mustService(t, fixture.catalog, Dependencies{
		ExactAnalyzer: exactAnalyzerFunc(func(context.Context, analyzer.ExactSymbolRequest) (evidence.Graph, error) {
			return graph, nil
		}),
	})
	result, err := service.Inspect(context.Background(), InspectRequest{Target: target})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Structural.IncomingCalls) != 0 || len(result.Structural.OutgoingCalls) != 0 {
		t.Fatalf("unsafe callers/callees escaped filtering: %#v", result.Structural)
	}
	if !strings.Contains(strings.Join(result.Structural.Warnings, "\n"), knownWarning) {
		t.Fatalf("known analyzer warning was lost: %#v", result.Structural.Warnings)
	}
	encoded, err := json.Marshal(result.Structural)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "/private/tmp") {
		t.Fatalf("structural result leaked absolute metadata: %s", encoded)
	}
}

func TestInspectRejectsChangedSourceHash(t *testing.T) {
	t.Parallel()

	fixture := newInspectionFixture(t, false)
	target := fixtureTarget()
	service := mustService(t, fixture.catalog, Dependencies{
		ExactAnalyzer: exactAnalyzerFunc(func(context.Context, analyzer.ExactSymbolRequest) (evidence.Graph, error) {
			return fixtureGraph(fixture.root, target, false), nil
		}),
	})
	if err := os.WriteFile(filepath.Join(fixture.root, "main.go"), []byte("package changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := service.Inspect(context.Background(), InspectRequest{Target: target})
	if ErrorKindOf(err) != ErrorSourceChanged || strings.Contains(err.Error(), fixture.root) {
		t.Fatalf("changed-source error = %v", err)
	}
}

func TestMissingOptionalDependenciesAndAnalyzerFailures(t *testing.T) {
	t.Parallel()

	fixture := newInspectionFixture(t, false)
	target := fixtureTarget()
	service := mustService(t, fixture.catalog, Dependencies{
		ExactAnalyzer: exactAnalyzerFunc(func(context.Context, analyzer.ExactSymbolRequest) (evidence.Graph, error) {
			return fixtureGraph(fixture.root, target, false), nil
		}),
	})
	result, err := service.Inspect(context.Background(), InspectRequest{
		Target: target, IncludeReferences: true,
	})
	if err != nil || len(result.Warnings) != 1 || result.Warnings[0].Code != "references.unavailable" {
		t.Fatalf("optional reference result=%#v err=%v", result, err)
	}
	_, err = mustService(t, fixture.catalog, Dependencies{}).Resolve(
		context.Background(),
		ResolveRequest{Location: evidence.Location{Path: "main.go", Line: 3}},
	)
	if ErrorKindOf(err) != ErrorAnalyzerUnavailable {
		t.Fatalf("missing resolver error = %v", err)
	}
	_, err = mustService(t, fixture.catalog, Dependencies{}).Inspect(
		context.Background(),
		InspectRequest{Target: target},
	)
	if ErrorKindOf(err) != ErrorAnalyzerUnavailable {
		t.Fatalf("missing exact analyzer error = %v", err)
	}

	analyzerErr := errors.New("adapter failed at " + fixture.root)
	failed := mustService(t, fixture.catalog, Dependencies{
		ExactAnalyzer: exactAnalyzerFunc(func(context.Context, analyzer.ExactSymbolRequest) (evidence.Graph, error) {
			return evidence.Graph{}, analyzerErr
		}),
	})
	_, err = failed.Inspect(context.Background(), InspectRequest{Target: target})
	if ErrorKindOf(err) != ErrorAnalysisFailed || strings.Contains(err.Error(), fixture.root) ||
		!errors.Is(err, analyzerErr) {
		t.Fatalf("analyzer error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	canceled := mustService(t, fixture.catalog, Dependencies{
		ExactAnalyzer: exactAnalyzerFunc(func(ctx context.Context, _ analyzer.ExactSymbolRequest) (evidence.Graph, error) {
			return evidence.Graph{}, ctx.Err()
		}),
	})
	_, err = canceled.Inspect(ctx, InspectRequest{Target: target})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation error = %v", err)
	}

	timeoutCtx, timeoutCancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer timeoutCancel()
	<-timeoutCtx.Done()
	_, err = canceled.Inspect(timeoutCtx, InspectRequest{Target: target})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout error = %v", err)
	}
}

type inspectionFixture struct {
	root    string
	catalog sourcecatalog.Catalog
	hashes  map[string]string
}

func newInspectionFixture(t *testing.T, includeTest bool) inspectionFixture {
	t.Helper()
	root := canonicalTempDir(t)
	files := map[string][]byte{
		"main.go": []byte(`package fixture

func Target() {
	callee()
}

func callee() {}

func caller() {
	Target()
}
`),
	}
	if includeTest {
		files["main_test.go"] = []byte("package fixture\n\nfunc TestTarget() {}\n")
	}
	for path, data := range files {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(path)), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	hashes := make(map[string]string, len(files))
	for path, data := range files {
		sum := sha256.Sum256(data)
		hashes[path] = fmt.Sprintf("%x", sum[:])
	}
	return inspectionFixture{
		root: root, catalog: newCatalog(t, root, files), hashes: hashes,
	}
}

func fixtureTarget() evidence.Entity {
	return evidence.Entity{
		ID: "function:main.go:3:1:Target", Kind: evidence.EntityFunction, Name: "Target", Language: "go",
		Location: &evidence.Location{Path: "main.go", Line: 3, Column: 1},
	}
}

func fixtureGraph(root string, target evidence.Entity, includeOutside bool) evidence.Graph {
	graph := evidence.NewGraph(root, target.Name)
	graph.Scenarios = []evidence.Scenario{{
		ID: "build", Name: "active build", WorkingDir: root,
		Command: []string{"fake", root}, Env: map[string]string{"LOCAL": root},
	}}
	query := evidence.Entity{ID: "query", Kind: evidence.EntityQuery, Name: target.Name}
	callee := evidence.Entity{
		ID: "callee", Kind: evidence.EntityFunction, Name: "callee", Language: "go",
		Location: &evidence.Location{Path: "main.go", Line: 7, Column: 1},
	}
	caller := evidence.Entity{
		ID: "caller", Kind: evidence.EntityFunction, Name: "caller", Language: "go",
		Location: &evidence.Location{Path: "main.go", Line: 9, Column: 1},
	}
	for _, entity := range []evidence.Entity{query, target, callee, caller} {
		graph.AddEntity(entity)
	}
	provenance := func(operation, path string, line int) []evidence.Provenance {
		return []evidence.Provenance{{
			Provider: "fake", Version: "v1", Operation: operation, Detail: root,
			Location: &evidence.Location{Path: path, Line: line, Column: 1},
		}}
	}
	graph.AddRelation(evidence.Relation{
		From: query.ID, To: target.ID, Kind: evidence.RelationMatchesQuery,
		Certainty: evidence.CertaintyStatic, Provenance: provenance("matches", "main.go", 3),
		Scenarios: []string{"build"},
	})
	graph.AddRelation(evidence.Relation{
		From: query.ID, To: target.ID, Kind: evidence.RelationResolvesTo,
		Certainty: evidence.CertaintyStatic, Provenance: provenance("resolves", "main.go", 3),
		Scenarios: []string{"build"},
	})
	graph.AddRelation(evidence.Relation{
		From: caller.ID, To: target.ID, Kind: evidence.RelationCalls,
		Certainty: evidence.CertaintyStatic, Provenance: provenance("calls", "main.go", 10),
		Scenarios: []string{"build"},
	})
	graph.AddRelation(evidence.Relation{
		From: target.ID, To: callee.ID, Kind: evidence.RelationCalls,
		Certainty: evidence.CertaintyStatic, Provenance: provenance("calls", "main.go", 4),
		Scenarios: []string{"build"},
	})
	if includeOutside {
		outside := evidence.Entity{
			ID: "outside", Kind: evidence.EntityFunction, Name: "outside", Language: "go",
			Location: &evidence.Location{Path: "outside.go", Line: 1, Column: 1},
		}
		graph.AddEntity(outside)
		graph.AddRelation(evidence.Relation{
			From: target.ID, To: outside.ID, Kind: evidence.RelationCalls,
			Certainty: evidence.CertaintyStatic, Provenance: provenance("calls", "outside.go", 1),
			Scenarios: []string{"build"},
		})
	}
	graph.Sort()
	return graph
}

func canonicalTempDir(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Clean(root)
}

func newCatalog(t *testing.T, root string, files map[string][]byte) sourcecatalog.Catalog {
	t.Helper()
	paths := make([]string, 0, len(files))
	inputs := make([]freshness.CapturedInput, 0, len(files))
	for path, data := range files {
		paths = append(paths, path)
		sum := sha256.Sum256(data)
		inputs = append(inputs, freshness.CapturedInput{
			Version:       freshness.CapturedInputVersion,
			ID:            fmt.Sprintf("%x", sha256.Sum256([]byte("inspection-test\x00"+path))),
			Path:          path,
			Kind:          freshness.FileRegular,
			Mode:          "100644",
			ContentSHA256: fmt.Sprintf("%x", sum[:]),
			Stages:        []string{"inspection_test"},
		})
	}
	catalog, err := sourcecatalog.New(sourcecatalog.Input{
		RepositoryRoot: root,
		AnalysisRoot:   root,
		AllowedPaths:   paths,
		CapturedInputs: inputs,
	})
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}

func mustService(t *testing.T, catalog sourcecatalog.Catalog, dependencies Dependencies) *Service {
	t.Helper()
	service, err := New(catalog, dependencies)
	if err != nil {
		t.Fatal(err)
	}
	return service
}
