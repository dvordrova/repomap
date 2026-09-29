package orientation

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/llm"
)

// Both fixture targets are built from the same program, so their seed
// has the same bare subject id. A summary and a role citing both used to seal
// two equal ids and end the run.
func TestSummaryAndRoleKeepTargetQualifiedSubjects(t *testing.T) {
	fixture := newFixture(t)
	refs := fixture.refs(t)
	alphaCore, betaCore := refs.subject("alpha", "inbound"), refs.subject("beta", "inbound")
	if fixture.subjectID("alpha", "inbound") != fixture.subjectID("beta", "inbound") || alphaCore == betaCore {
		t.Fatalf("fixture no longer repeats a bare subject id across targets: %s %s", alphaCore, betaCore)
	}
	provider := &presetProvider{respond: func([]byte) []byte {
		return encodeResponse(t, map[string]any{
			"summary": "Both targets apply the items.", "summary_refs": []string{alphaCore, betaCore},
			"roles": []any{map[string]any{"target": refs.target("alpha"), "role": "Backend", "purpose": "Serves items.",
				"refs": []string{alphaCore, betaCore}}},
		})
	}}
	result, rejected, err := Run(t.Context(), llm.Executor{}, provider, fixture.input)
	if err != nil || len(rejected) != 0 || result.Validate() != nil {
		t.Fatalf("two targets' equal member ids ended the run: %v, %+v", err, rejected)
	}
	want := []string{alphaCore, betaCore}
	if !reflect.DeepEqual(result.SummaryRefs, want) || len(result.Roles) != 1 || !reflect.DeepEqual(result.Roles[0].SubjectIDs, want) {
		t.Fatalf("qualified subjects were not kept: summary=%v roles=%+v", result.SummaryRefs, result.Roles)
	}

	// An unknown qualified member is still refused, never kept as a pointer.
	provider = &presetProvider{respond: func([]byte) []byte {
		return encodeResponse(t, map[string]any{"summary": "Nothing supports this.", "summary_refs": []string{"t9.n2"},
			"roles": []any{map[string]any{"target": refs.target("beta"), "role": "Client", "purpose": "Fetches items.", "refs": []string{refs.fact("call")}}}})
	}}
	result, rejected, err = Run(t.Context(), llm.Executor{}, provider, fixture.input)
	if err != nil || result.Summary != "" || len(result.SummaryRefs) != 0 || len(rejected) != 1 || rejected[0].Section != sectionSummary {
		t.Fatalf("a summary citing only an unknown member was accepted: %+v, %+v, %v", result, rejected, err)
	}
}

func TestRefsWrittenAsOneStringAreOneRef(t *testing.T) {
	fixture := newFixture(t)
	refs := fixture.refs(t)
	_, catalogue, err := buildOverview(fixture.input)
	if err != nil {
		t.Fatal(err)
	}
	raw := encodeResponse(t, map[string]any{
		"summary": "Alpha serves items.", "summary_refs": refs.fact("route"),
		"roles": []any{
			map[string]any{"target": refs.target("alpha"), "role": "Backend", "purpose": "Serves items.", "refs": refs.fact("route")},
			42,
		},
		"run_recipe": []any{map[string]any{"command": "go run .", "cwd": "alpha", "refs": refs.fact("entrypoint")}},
	})
	result, err := normalizeOverview(raw, catalogue)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.summaryRefs, []string{fixture.factID("route")}) || len(result.roles) != 1 ||
		!reflect.DeepEqual(result.roles[0].FactIDs, []string{fixture.factID("route")}) ||
		len(result.recipe) != 1 || !reflect.DeepEqual(result.recipe[0].FactIDs, []string{fixture.factID("entrypoint")}) {
		t.Fatalf("a single string ref was not read as a one-element list: %+v", result)
	}
	if len(result.rejected) != 1 || result.rejected[0].Section != sectionRoles || string(result.rejected[0].Raw) != "42" {
		t.Fatalf("a row that is not an object was not refused alone: %+v", result.rejected)
	}
	// Any other wrong type for a ref list still refuses its section.
	result, err = normalizeOverview(encodeResponse(t, map[string]any{
		"summary": "Alpha serves items.", "summary_refs": 42,
		"roles": []any{map[string]any{"target": refs.target("alpha"), "role": "Backend", "purpose": "Serves items.", "refs": refs.fact("route")}},
	}), catalogue)
	if err != nil || result.summary != "" || len(result.roles) != 1 || len(result.rejected) != 1 || result.rejected[0].Section != sectionSummary {
		t.Fatalf("a numeric ref list was accepted: %+v, %v", result, err)
	}
}

func TestRoleKeepsAnEmptyPurposeButNotAnEmptyLabel(t *testing.T) {
	fixture := newFixture(t)
	refs := fixture.refs(t)
	provider := &presetProvider{respond: func([]byte) []byte {
		return encodeResponse(t, map[string]any{"roles": []any{
			map[string]any{"target": refs.target("alpha"), "role": "Backend", "purpose": "  ", "refs": []string{refs.fact("route")}},
			map[string]any{"target": refs.target("beta"), "role": " ", "purpose": "Fetches items.", "refs": []string{refs.fact("call")}},
		}})
	}}
	result, rejected, err := Run(t.Context(), llm.Executor{}, provider, fixture.input)
	if err != nil || result.Validate() != nil {
		t.Fatal(err)
	}
	if len(result.Roles) != 1 || result.Roles[0].TargetID != fixture.targetID("alpha") || result.Roles[0].Role != "Backend" || result.Roles[0].Purpose != "" {
		t.Fatalf("a role label without purpose was lost or filled in: %+v", result.Roles)
	}
	if len(rejected) != 1 || !strings.Contains(rejected[0].Reason, "role must contain") {
		t.Fatalf("a role without a label was not refused: %+v", rejected)
	}
}

func TestTargetRefAndCwdAreTrimmedButStillChecked(t *testing.T) {
	fixture := newFixture(t)
	refs := fixture.refs(t)
	provider := &presetProvider{respond: func([]byte) []byte {
		return encodeResponse(t, map[string]any{
			"roles": []any{
				map[string]any{"target": " " + refs.target("alpha") + " ", "role": "Backend", "purpose": "Serves items.", "refs": []string{refs.fact("route")}},
				map[string]any{"target": "t9", "role": "Ghost", "purpose": "Does not exist.", "refs": []string{refs.fact("call")}},
			},
			"run_recipe": []any{
				map[string]any{"target": refs.target("alpha") + " ", "command": "go run .", "cwd": " alpha ", "refs": []string{refs.fact("entrypoint")}},
				map[string]any{"command": "go run .", "cwd": "alpha\nbeta", "refs": []string{refs.fact("entrypoint")}},
			},
			"main_flow_target": " " + refs.target("alpha"),
		})
	}, flow: func([]byte) []byte {
		return encodeResponse(t, map[string]any{"title": "Items", "steps": []any{
			map[string]any{"ref": " " + refs.subject("alpha", "core"), "explanation": "Apply computes the items."},
		}})
	}}
	result, rejected, err := Run(t.Context(), llm.Executor{}, provider, fixture.input)
	if err != nil || result.Validate() != nil {
		t.Fatal(err)
	}
	if len(result.Roles) != 1 || result.Roles[0].TargetID != fixture.targetID("alpha") {
		t.Fatalf("a padded target ref was not trimmed: %+v", result.Roles)
	}
	if len(result.RunRecipe) != 1 || result.RunRecipe[0].Cwd != "alpha" || result.RunRecipe[0].TargetID != fixture.targetID("alpha") {
		t.Fatalf("a padded cwd was not trimmed: %+v", result.RunRecipe)
	}
	if len(result.MainFlow.Steps) != 1 || result.MainFlow.Steps[0].TargetID != fixture.targetID("alpha") {
		t.Fatalf("a padded flow target was not trimmed: %+v", result.MainFlow)
	}
	if len(rejected) != 2 || !strings.Contains(rejected[0].Reason, `unknown target ref "t9"`) || !strings.Contains(rejected[1].Reason, "cwd") {
		t.Fatalf("an unknown target or a multi-line cwd was accepted: %+v", rejected)
	}
}

func TestInvalidRecipeNoteIsDroppedAndTheStepKept(t *testing.T) {
	fixture := newFixture(t)
	refs := fixture.refs(t)
	provider := &presetProvider{respond: func([]byte) []byte {
		return encodeResponse(t, map[string]any{"run_recipe": []any{
			map[string]any{"command": "go run .", "cwd": "alpha", "note": "Listens on\x00PORT.", "refs": []string{refs.fact("entrypoint")}},
			map[string]any{"command": "go\x00run", "note": "A fine note.", "refs": []string{refs.fact("entrypoint")}},
		}})
	}}
	result, rejected, err := Run(t.Context(), llm.Executor{}, provider, fixture.input)
	if err != nil || result.Validate() != nil {
		t.Fatal(err)
	}
	if len(result.RunRecipe) != 1 || result.RunRecipe[0].Command != "go run ." || result.RunRecipe[0].Note != "" {
		t.Fatalf("the step was lost with its note or the note was kept: %+v", result.RunRecipe)
	}
	var note string
	if len(rejected) != 2 || json.Unmarshal(rejected[0].Raw, &note) != nil || note != "Listens on\x00PORT." ||
		!strings.Contains(rejected[0].Reason, "note") || !strings.Contains(rejected[1].Reason, "command") {
		t.Fatalf("the dropped note or the invalid command was not recorded: %+v", rejected)
	}
}

func TestRolePurposesDifferingOnlyInSpacesAreOneRole(t *testing.T) {
	fixture := newFixture(t)
	refs := fixture.refs(t)
	for _, test := range []struct {
		second   string
		conflict bool
	}{
		{"Serves  the\titems.", false},
		{"Serves the orders.", true},
	} {
		provider := &presetProvider{respond: func([]byte) []byte {
			return encodeResponse(t, map[string]any{"roles": []any{
				map[string]any{"target": refs.target("alpha"), "role": "Backend", "purpose": "Serves the items.", "refs": []string{refs.fact("route")}},
				map[string]any{"target": refs.target("alpha"), "role": "Backend", "purpose": test.second, "refs": []string{refs.fact("entrypoint")}},
				map[string]any{"target": refs.target("beta"), "role": "Client", "purpose": "Fetches items.", "refs": []string{refs.fact("call")}},
			}})
		}}
		result, rejected, err := Run(t.Context(), llm.Executor{}, provider, fixture.input)
		if err != nil {
			t.Fatal(err)
		}
		if test.conflict {
			if len(result.Roles) != 1 || result.Roles[0].TargetID != fixture.targetID("beta") || len(rejected) != 1 {
				t.Fatalf("different purposes did not conflict: %+v %+v", result.Roles, rejected)
			}
			continue
		}
		if len(rejected) != 0 || len(result.Roles) != 2 || result.Roles[0].Purpose != "Serves the items." ||
			!reflect.DeepEqual(result.Roles[0].FactIDs, []string{fixture.factID("route"), fixture.factID("entrypoint")}) {
			t.Fatalf("whitespace made two purposes conflict: %+v %+v", result.Roles, rejected)
		}
	}
}
