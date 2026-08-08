package themestudy

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestAdjudicationFinalProseReplacesScoutPromises(t *testing.T) {
	t.Parallel()
	candidate := &ScoutCandidate{
		Ref:              "t1",
		Title:            "Broad startup story",
		Question:         "How does the entire service start and persist state?",
		WhyItMatters:     "Scout promise: explain startup and every persistence path.",
		ExpectedLearning: "Scout promise: learn the complete runtime and storage lifecycle.",
		ThemeKind:        KindUserJourney,
		RelationClaim:    RelationClaimEditorialOnly,
		AnchorRefs:       []string{"a1"},
	}
	raw := []byte(`{"themes":[{` +
		`"candidate_ref":"t1",` +
		`"final_title":"Configuration bootstrap",` +
		`"final_question":"How is startup configuration loaded?",` +
		`"why_it_matters":"Configuration determines the service's initial operating context.",` +
		`"expected_learning":"Learn where startup configuration is loaded from the retained source.",` +
		`"readings":[{"anchor_ref":"a1","support":"direct","observation":"The function loads startup configuration."}],` +
		`"unknowns":[]}]}`)

	themes, status, err := ValidateAdjudication(raw, map[string]*ScoutCandidate{"t1": candidate})
	if err != nil {
		t.Fatalf("ValidateAdjudication: %v", err)
	}
	if status.State != "accepted" || len(themes) != 1 {
		t.Fatalf("adjudication status/themes = %#v / %#v", status, themes)
	}
	reduction, err := Reduce(ReducerInput{
		Themes: themes,
		Candidates: map[string]*ScoutCandidate{
			"t1": candidate,
		},
		Anchors: map[string]AnchorInfo{
			"a1": {Path: "cmd/service/main.go", Symbol: "loadConfig", Line: 27},
		},
	})
	if err != nil {
		t.Fatalf("Reduce: %v", err)
	}
	if len(reduction.Cards) != 1 {
		t.Fatalf("cards = %d, want 1", len(reduction.Cards))
	}
	card := reduction.Cards[0]
	if card.WhyItMatters != themes[0].WhyItMatters || card.ExpectedLearning != themes[0].ExpectedLearning {
		t.Fatalf("reducer did not publish adjudicated prose: %#v", card)
	}
	if card.WhyItMatters == candidate.WhyItMatters || card.ExpectedLearning == candidate.ExpectedLearning {
		t.Fatalf("stale Scout promises leaked into narrowed card: %#v", card)
	}
}

func TestAdjudicationFinalProseIsRequiredAndNormalized(t *testing.T) {
	t.Parallel()
	candidate := &ScoutCandidate{
		Ref: "t1", Title: "T", Question: "Q?", WhyItMatters: "Scout why", ExpectedLearning: "Scout learn",
		ThemeKind: KindUserJourney, RelationClaim: RelationClaimEditorialOnly, AnchorRefs: []string{"a1"},
	}
	base := `"candidate_ref":"t1","final_title":"T","final_question":"Q?",` +
		`"readings":[{"anchor_ref":"a1","support":"direct","observation":"o"}],"unknowns":[]`
	for _, raw := range [][]byte{
		[]byte(`{"themes":[{` + base + `,"expected_learning":"Learn Q."}]}`),
		[]byte(`{"themes":[{` + base + `,"why_it_matters":"Q matters."}]}`),
	} {
		themes, status, err := ValidateAdjudication(raw, map[string]*ScoutCandidate{"t1": candidate})
		if err != nil {
			t.Fatalf("ValidateAdjudication missing prose: %v", err)
		}
		if len(themes) != 0 || status.Accepted != 0 || len(status.Issues) != 1 ||
			status.Issues[0].Code != AdjIssueEmptyFinalProse {
			t.Fatalf("missing final prose must reject item-locally: themes=%#v status=%#v", themes, status)
		}
	}

	longWhy := strings.Repeat("relevant evidence ", 30)
	longLearning := strings.Repeat("bounded learning ", 30)
	payload, err := json.Marshal(map[string]any{"themes": []any{map[string]any{
		"candidate_ref": "t1", "final_title": "T", "final_question": "Q?",
		"why_it_matters": longWhy, "expected_learning": longLearning,
		"readings": []any{map[string]any{
			"anchor_ref": "a1", "support": "direct", "observation": "o",
		}},
		"unknowns": []string{},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	themes, status, err := ValidateAdjudication(payload, map[string]*ScoutCandidate{"t1": candidate})
	if err != nil {
		t.Fatalf("ValidateAdjudication overlong final prose: %v", err)
	}
	if len(themes) != 1 || status.Normalized["why_it_matters"] != 1 ||
		status.Normalized["expected_learning"] != 1 {
		t.Fatalf("final prose normalization missing: themes=%#v status=%#v", themes, status)
	}
	if utf8.RuneCountInString(themes[0].WhyItMatters) > MaxEditorialRunes ||
		utf8.RuneCountInString(themes[0].ExpectedLearning) > MaxEditorialRunes ||
		!strings.HasSuffix(themes[0].WhyItMatters, "…") ||
		!strings.HasSuffix(themes[0].ExpectedLearning, "…") {
		t.Fatalf("final prose is not visibly bounded: %#v", themes[0])
	}
}

func TestAdjudicationRejectsHistoricalFlatResponseGrammar(t *testing.T) {
	t.Parallel()
	candidate := &ScoutCandidate{
		Ref: "t1", Title: "T", Question: "Q?", WhyItMatters: "Scout why", ExpectedLearning: "Scout learn",
		ThemeKind: KindUserJourney, RelationClaim: RelationClaimEditorialOnly, AnchorRefs: []string{"a1"},
	}
	raw := []byte(`{"themes":[{` +
		`"candidate_ref":"t1","final_title":"T","final_question":"Q?",` +
		`"why_it_matters":"Q matters.","expected_learning":"Learn Q.",` +
		`"anchor_assessments":[{"anchor_ref":"a1","fit":"direct","supported_observation":"o"}],` +
		`"reading_order":["a1"]}]}`)
	themes, status, err := ValidateAdjudication(raw, map[string]*ScoutCandidate{"t1": candidate})
	if err != nil {
		t.Fatalf("historical item must close item-locally, not fail the envelope: %v", err)
	}
	if len(themes) != 0 || status.Accepted != 0 || status.Rejected != 1 ||
		len(status.Issues) != 1 || status.Issues[0].Code != AdjIssueDecodeCandidate {
		t.Fatalf("historical response grammar did not miss closed: themes=%#v status=%#v", themes, status)
	}
}

func TestMockAdjudicationUsesCurrentFinalProseWire(t *testing.T) {
	t.Parallel()
	request := buildTestAdjudicationRequest(t)
	staleRequest := request
	staleRequest.Version = 2
	if _, err := EncodeAdjudicationRequest(staleRequest); err == nil {
		t.Fatal("pre-corrective Adjudication request identity must fail closed")
	}
	raw, err := MockAdjudicationResponse(request)
	if err != nil {
		t.Fatalf("MockAdjudicationResponse: %v", err)
	}
	var response struct {
		Themes []map[string]json.RawMessage `json:"themes"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatalf("decode mock: %v", err)
	}
	if len(response.Themes) != 1 {
		t.Fatalf("mock themes = %d, want 1", len(response.Themes))
	}
	theme := response.Themes[0]
	for _, field := range []string{"final_question", "why_it_matters", "expected_learning", "readings"} {
		if len(theme[field]) == 0 {
			t.Fatalf("mock misses current field %q: %s", field, raw)
		}
	}
	for _, legacy := range []string{"anchor_assessments", "reading_order"} {
		if _, ok := theme[legacy]; ok {
			t.Fatalf("mock emitted legacy field %q: %s", legacy, raw)
		}
	}
}

func TestAdjudicationResultArtifactRequiresCurrentFinalProse(t *testing.T) {
	t.Parallel()
	request := buildTestAdjudicationRequest(t)
	raw, err := MockAdjudicationResponse(request)
	if err != nil {
		t.Fatalf("MockAdjudicationResponse: %v", err)
	}
	result, _, err := ReplayAdjudicationResponse(request, raw)
	if err != nil {
		t.Fatalf("ReplayAdjudicationResponse: %v", err)
	}
	encoded, err := EncodeAdjudicationResult(result)
	if err != nil {
		t.Fatalf("EncodeAdjudicationResult current result: %v", err)
	}
	if _, err := DecodeAdjudicationResult(encoded); err != nil {
		t.Fatalf("DecodeAdjudicationResult current result: %v", err)
	}

	stale := result
	stale.Version = 4
	if _, err := EncodeAdjudicationResult(stale); err == nil {
		t.Fatal("pre-corrective Adjudication result identity must fail closed")
	}

	missing := result
	missing.Themes = append([]AdjudicatedTheme(nil), result.Themes...)
	missing.Themes[0].ExpectedLearning = ""
	if _, err := EncodeAdjudicationResult(missing); err == nil {
		t.Fatal("result without final expected_learning must fail closed")
	}

	overLimit := result
	overLimit.Themes = append([]AdjudicatedTheme(nil), result.Themes...)
	overLimit.Themes[0].WhyItMatters = strings.Repeat("x", MaxEditorialRunes+1)
	if _, err := EncodeAdjudicationResult(overLimit); err == nil {
		t.Fatal("result with unnormalized final why_it_matters must fail closed")
	}
}
