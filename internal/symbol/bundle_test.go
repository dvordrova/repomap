package symbol

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/evidence"
)

func TestBuildSelectsExactTargetAndBoundedCalls(t *testing.T) {
	t.Parallel()

	graph := testGraph()
	bundle, err := Build(graph, Options{MaxCandidates: 2, MaxIncomingCalls: 1, MaxOutgoingCalls: 1})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	if bundle.Target.Entity.Name != "kvServer.Put" {
		t.Fatalf("target = %q, want kvServer.Put", bundle.Target.Entity.Name)
	}
	if bundle.Target.EvidenceID != "resolution-001" {
		t.Fatalf("target evidence id = %q", bundle.Target.EvidenceID)
	}
	if len(bundle.IncomingCalls) != 1 || bundle.IncomingCalls[0].Caller.Name != "servePut" {
		t.Fatalf("incoming calls = %#v", bundle.IncomingCalls)
	}
	if len(bundle.OutgoingCalls) != 1 || bundle.OutgoingCalls[0].Callee.Name != "Txn" {
		t.Fatalf("outgoing calls = %#v", bundle.OutgoingCalls)
	}
	if bundle.Truncated["incoming_calls"] != 1 {
		t.Fatalf("truncated incoming calls = %d, want 1", bundle.Truncated["incoming_calls"])
	}
	if !contains(bundle.AllowedPaths, "server/key.go") || !contains(bundle.AllowedPaths, "server/handler.go") {
		t.Fatalf("allowed paths = %v", bundle.AllowedPaths)
	}
}

func TestBuildOmitsScenarioEnvironmentAndWorkingDirectory(t *testing.T) {
	t.Parallel()

	bundle, err := Build(testGraph(), Options{})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	data, err := json.Marshal(bundle)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	text := string(data)
	for _, forbidden := range []string{"SECRET_TOKEN", "/private/repo", "working_dir", `"env"`} {
		if strings.Contains(text, forbidden) {
			t.Errorf("bundle contains forbidden scenario data %q", forbidden)
		}
	}
}

func TestBuildRequiresUniqueExactResolution(t *testing.T) {
	t.Parallel()

	graph := testGraph()
	for i := range graph.Relations {
		if graph.Relations[i].Kind == evidence.RelationResolvesTo {
			graph.Relations = append(graph.Relations[:i], graph.Relations[i+1:]...)
			break
		}
	}

	_, err := Build(graph, Options{})
	if err == nil || !strings.Contains(err.Error(), "no unique exact resolution") {
		t.Fatalf("Build() error = %v, want exact resolution error", err)
	}
}

func testGraph() evidence.Graph {
	graph := evidence.NewGraph("/private/repo", "kvServer.Put")
	graph.Scenarios = []evidence.Scenario{{
		ID:         "build",
		Name:       "active build",
		WorkingDir: "/private/repo",
		Env:        map[string]string{"SECRET_TOKEN": "secret"},
		Build:      evidence.BuildContext{GOOS: "linux", GOARCH: "amd64"},
	}}
	entities := []evidence.Entity{
		{ID: "query", Kind: evidence.EntityQuery, Name: "kvServer.Put"},
		{ID: "target", Kind: evidence.EntityMethod, Name: "kvServer.Put", Location: &evidence.Location{Path: "server/key.go", Line: 90, Column: 20}},
		{ID: "candidate", Kind: evidence.EntityMethod, Name: "KVServer.Put", Location: &evidence.Location{Path: "api/generated.go", Line: 10, Column: 2}},
		{ID: "caller-a", Kind: evidence.EntityFunction, Name: "servePut", Location: &evidence.Location{Path: "server/handler.go", Line: 40, Column: 6}},
		{ID: "caller-b", Kind: evidence.EntityFunction, Name: "retryPut", Location: &evidence.Location{Path: "server/retry.go", Line: 20, Column: 6}},
		{ID: "callee", Kind: evidence.EntityMethod, Name: "Txn", Location: &evidence.Location{Path: "server/txn.go", Line: 30, Column: 18}},
	}
	for _, entity := range entities {
		graph.AddEntity(entity)
	}
	provenance := func(operation, path string, line int) []evidence.Provenance {
		return []evidence.Provenance{{
			Provider:  "gopls",
			Version:   "v0.21.0",
			Operation: operation,
			Location:  &evidence.Location{Path: path, Line: line},
		}}
	}
	graph.AddRelation(evidence.Relation{From: "query", To: "target", Kind: evidence.RelationMatchesQuery, Certainty: evidence.CertaintyPossible, Provenance: provenance("workspace_symbol", "server/key.go", 90), Scenarios: []string{"build"}})
	graph.AddRelation(evidence.Relation{From: "query", To: "candidate", Kind: evidence.RelationMatchesQuery, Certainty: evidence.CertaintyPossible, Provenance: provenance("workspace_symbol", "api/generated.go", 10), Scenarios: []string{"build"}})
	graph.AddRelation(evidence.Relation{From: "query", To: "target", Kind: evidence.RelationResolvesTo, Certainty: evidence.CertaintyStatic, Provenance: provenance("workspace_symbol", "server/key.go", 90), Scenarios: []string{"build"}})
	graph.AddRelation(evidence.Relation{From: "caller-a", To: "target", Kind: evidence.RelationCalls, Certainty: evidence.CertaintyStatic, Provenance: provenance("call_hierarchy", "server/handler.go", 55), Scenarios: []string{"build"}})
	graph.AddRelation(evidence.Relation{From: "caller-b", To: "target", Kind: evidence.RelationCalls, Certainty: evidence.CertaintyStatic, Provenance: provenance("call_hierarchy", "server/retry.go", 25), Scenarios: []string{"build"}})
	graph.AddRelation(evidence.Relation{From: "target", To: "callee", Kind: evidence.RelationCalls, Certainty: evidence.CertaintyStatic, Provenance: provenance("call_hierarchy", "server/key.go", 94), Scenarios: []string{"build"}})
	graph.Sort()
	return graph
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
