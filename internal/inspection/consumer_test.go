package inspection_test

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/dvordrova/repomap/internal/analyzer"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/inspection"
	"github.com/dvordrova/repomap/internal/sourcecatalog"
)

type consumerAdapter struct {
	root   string
	target evidence.Entity
}

func (adapter consumerAdapter) ResolveLocation(
	_ context.Context,
	request analyzer.LocationRequest,
) (analyzer.LocationResolution, error) {
	return analyzer.LocationResolution{
		Location: request.Location,
		Candidates: []analyzer.LocationCandidate{{
			Entity:       adapter.target,
			Match:        "exact",
			Certainty:    evidence.CertaintyStatic,
			Investigable: true,
		}},
		Certainty:  evidence.CertaintyStatic,
		Provenance: evidence.Provenance{Provider: "consumer-fake", Operation: "resolve"},
	}, nil
}

func (adapter consumerAdapter) AnalyzeExactSymbol(
	_ context.Context,
	_ analyzer.ExactSymbolRequest,
) (evidence.Graph, error) {
	graph := evidence.NewGraph(adapter.root, adapter.target.Name)
	graph.Scenarios = []evidence.Scenario{{ID: "build", Name: "consumer build"}}
	query := evidence.Entity{ID: "query", Kind: evidence.EntityQuery, Name: adapter.target.Name}
	caller := evidence.Entity{
		ID: "caller", Kind: evidence.EntityFunction, Name: "caller",
		Location: &evidence.Location{Path: "service.go", Line: 9, Column: 1},
	}
	callee := evidence.Entity{
		ID: "callee", Kind: evidence.EntityFunction, Name: "callee",
		Location: &evidence.Location{Path: "service.go", Line: 7, Column: 1},
	}
	for _, entity := range []evidence.Entity{query, adapter.target, caller, callee} {
		graph.AddEntity(entity)
	}
	relation := func(from, to string, kind evidence.RelationKind, line int) {
		graph.AddRelation(evidence.Relation{
			From: from, To: to, Kind: kind, Certainty: evidence.CertaintyStatic,
			Provenance: []evidence.Provenance{{
				Provider: "consumer-fake", Operation: "exact",
				Location: &evidence.Location{Path: "service.go", Line: line, Column: 1},
			}},
			Scenarios: []string{"build"},
		})
	}
	relation(query.ID, adapter.target.ID, evidence.RelationMatchesQuery, 3)
	relation(query.ID, adapter.target.ID, evidence.RelationResolvesTo, 3)
	relation(caller.ID, adapter.target.ID, evidence.RelationCalls, 10)
	relation(adapter.target.ID, callee.ID, evidence.RelationCalls, 4)
	graph.Sort()
	return graph, nil
}

func (adapter consumerAdapter) References(
	_ context.Context,
	_ string,
	_ evidence.Location,
) (evidence.LocationSet, error) {
	return evidence.LocationSet{
		Locations: []evidence.Location{
			{Path: "service.go", Line: 10, Column: 1},
			{Path: "service_test.go", Line: 3, Column: 1},
		},
		Certainty:  evidence.CertaintyStatic,
		Provenance: []evidence.Provenance{{Provider: "consumer-fake", Operation: "references"}},
		Scenarios:  []evidence.Scenario{{ID: "build", Name: "consumer build"}},
	}, nil
}

func TestExternalConsumerResolvesAndInspectsWithoutReportPackages(t *testing.T) {
	t.Parallel()

	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{
		"service.go": []byte(`package consumer

func Target() {
	callee()
}

func callee() {}

func caller() {
	Target()
}
`),
		"service_test.go": []byte("package consumer\n\nfunc TestTarget() {}\n"),
	}
	paths := make([]string, 0, len(files))
	inputs := make([]freshness.CapturedInput, 0, len(files))
	for path, data := range files {
		if err := os.WriteFile(filepath.Join(root, path), data, 0o600); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, path)
		sum := sha256.Sum256(data)
		inputs = append(inputs, freshness.CapturedInput{
			Version:       freshness.CapturedInputVersion,
			ID:            fmt.Sprintf("%x", sha256.Sum256([]byte("consumer\x00"+path))),
			Path:          path,
			Kind:          freshness.FileRegular,
			Mode:          "100644",
			ContentSHA256: fmt.Sprintf("%x", sum[:]),
			Stages:        []string{"consumer_test"},
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
	target := evidence.Entity{
		ID: "target", Kind: evidence.EntityFunction, Name: "Target", Language: "go",
		Location: &evidence.Location{Path: "service.go", Line: 3, Column: 1},
	}
	adapter := consumerAdapter{root: root, target: target}
	service, err := inspection.New(catalog, inspection.Dependencies{
		Resolver: adapter, ExactAnalyzer: adapter, ReferenceFinder: adapter,
	})
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := service.Resolve(context.Background(), inspection.ResolveRequest{
		Location: evidence.Location{Path: "service.go", Line: 3},
	})
	if err != nil || len(resolved.Candidates) != 1 {
		t.Fatalf("Resolve result=%#v err=%v", resolved, err)
	}
	result, err := service.Inspect(context.Background(), inspection.InspectRequest{
		Target: resolved.Candidates[0].Entity, IncludeReferences: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Source.Lines) == 0 ||
		len(result.Structural.IncomingCalls) != 1 ||
		len(result.Structural.OutgoingCalls) != 1 ||
		result.References == nil || len(result.References.Locations) != 1 ||
		result.Tests == nil || len(result.Tests.Locations) != 1 {
		t.Fatalf("consumer result = %#v", result)
	}
}
