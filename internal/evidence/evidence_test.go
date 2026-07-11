package evidence

import (
	"strings"
	"testing"
)

func TestCertaintyValid(t *testing.T) {
	tests := []struct {
		name      string
		certainty Certainty
		expected  bool
	}{
		{name: "possible", certainty: CertaintyPossible, expected: true},
		{name: "static", certainty: CertaintyStatic, expected: true},
		{name: "observed", certainty: CertaintyObserved, expected: true},
		{name: "invalid", certainty: Certainty("certain-ish"), expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.certainty.Valid(); got != tt.expected {
				t.Fatalf("Valid() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestGraphAddRelationMergesEvidence(t *testing.T) {
	graph := NewGraph("/repo", "Put")
	graph.Scenarios = append(graph.Scenarios, Scenario{ID: "test-put", Name: "put test"})
	graph.AddEntity(Entity{ID: "a", Kind: EntityFunction, Name: "a"})
	graph.AddEntity(Entity{ID: "b", Kind: EntityFunction, Name: "b"})

	graph.AddRelation(Relation{
		From:      "a",
		To:        "b",
		Kind:      RelationCalls,
		Certainty: CertaintyStatic,
		Provenance: []Provenance{{
			Provider:  "gopls",
			Operation: "call_hierarchy",
		}},
	})
	graph.AddRelation(Relation{
		From:      "a",
		To:        "b",
		Kind:      RelationCalls,
		Certainty: CertaintyStatic,
		Provenance: []Provenance{{
			Provider:  "gopls",
			Operation: "call_hierarchy",
		}, {
			Provider:  "ssa",
			Operation: "static_call",
		}},
		Scenarios: []string{"test-put"},
	})

	if len(graph.Relations) != 1 {
		t.Fatalf("relations = %d, want 1", len(graph.Relations))
	}
	if len(graph.Relations[0].Provenance) != 2 {
		t.Fatalf("provenance = %d, want 2", len(graph.Relations[0].Provenance))
	}
	if len(graph.Relations[0].Scenarios) != 1 {
		t.Fatalf("scenarios = %d, want 1", len(graph.Relations[0].Scenarios))
	}
	if err := graph.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestGraphValidateRejectsUnknownEntity(t *testing.T) {
	graph := NewGraph("/repo", "Put")
	graph.AddEntity(Entity{ID: "a", Kind: EntityFunction, Name: "a"})
	graph.AddRelation(Relation{
		From:      "a",
		To:        "missing",
		Kind:      RelationCalls,
		Certainty: CertaintyStatic,
		Provenance: []Provenance{{
			Provider:  "gopls",
			Operation: "call_hierarchy",
		}},
	})

	if err := graph.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want unknown entity error")
	}
}

func TestGraphValidateRejectsUnknownScenario(t *testing.T) {
	graph := NewGraph("/repo", "query")
	graph.AddEntity(Entity{ID: "query", Kind: EntityQuery, Name: "query"})
	graph.AddEntity(Entity{ID: "symbol", Kind: EntityFunction, Name: "Run"})
	graph.AddRelation(Relation{
		From:       "query",
		To:         "symbol",
		Kind:       RelationMatchesQuery,
		Certainty:  CertaintyPossible,
		Provenance: []Provenance{{Provider: "gopls", Operation: "workspace_symbol"}},
		Scenarios:  []string{"missing"},
	})

	err := graph.Validate()
	if err == nil || !strings.Contains(err.Error(), "unknown scenario") {
		t.Fatalf("Validate() error = %v, want unknown scenario", err)
	}
}

func TestLocationSetRequiresProviderContext(t *testing.T) {
	t.Parallel()

	valid := LocationSet{
		Locations:  []Location{{Path: "server/key_test.go", Line: 10, Column: 2}},
		Certainty:  CertaintyStatic,
		Provenance: []Provenance{{Provider: "gopls", Version: "v1", Operation: "references"}},
		Scenarios:  []Scenario{{ID: "active-build", Name: "active Go build"}},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	invalid := valid
	invalid.Provenance = nil
	if err := invalid.Validate(); err == nil {
		t.Fatal("Validate() accepted missing provenance")
	}
	invalid = valid
	invalid.Scenarios = nil
	if err := invalid.Validate(); err == nil {
		t.Fatal("Validate() accepted missing build scenario")
	}
}
