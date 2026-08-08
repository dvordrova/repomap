package themestudy

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// Phase 2 prompt cleanup (owner directive): Scout cardinality is a semantic
// prior, not a quota. The prompt tells the model most repositories need no
// more than about 12 materially distinct high-value themes and that fewer is
// preferable; overflow is allowed when additional themes add substantial
// distinct understanding. No hard cap exists on the accepted response.
func TestScoutPromptCardinalityIsSemanticPriorNotQuota(t *testing.T) {
	t.Parallel()
	request := buildTestScoutRequest(t)
	prompt := BuildScoutPrompt(request)
	user := prompt.User
	for _, required := range []string{
		"Most repositories need no more than about 12 materially distinct, high-value themes",
		"Use fewer when they cover the important learning outcomes",
		"return more only when additional themes add substantial distinct understanding",
		"Do not pad toward a target",
		"Prefer a small set of distinct anchor_refs",
		"return 1 to 5 per theme",
		"Use a single anchor when it is sufficient on its own",
		"Do not add anchors merely to reach a count",
	} {
		if !strings.Contains(user, required) {
			t.Errorf("Scout prompt misses semantic-prior instruction %q", required)
		}
	}
	for _, forbidden := range []string{
		"Aim for", "aim for", "Return 8-12", "valid 1-12", "desired 8-12",
		"hard limit", "hard-cap", "hard cap", "use %d-%d anchor_refs",
	} {
		if strings.Contains(user, forbidden) {
			t.Errorf("Scout prompt still carries quota/cap language %q", forbidden)
		}
	}
}

// Owner directive (2026-08-07): there is NO hard cap on the accepted Scout
// response. Every materially distinct accepted theme is preserved; the
// backend never imposes an arbitrary semantic cardinality ceiling. A large
// honest portfolio (beyond the ~12 prior) is accepted in full.
func TestValidateScoutAcceptsLargePortfolioWithoutCardinalityCap(t *testing.T) {
	t.Parallel()
	anchors := make(map[string]struct{}, 40)
	for index := 1; index <= 40; index++ {
		anchors[fmt.Sprintf("a%d", index)] = struct{}{}
	}
	themes := make([]ScoutCandidate, 0, 20)
	for index := 1; index <= 20; index++ {
		themes = append(themes, ScoutCandidate{
			Title: fmt.Sprintf("theme %d", index), Question: fmt.Sprintf("theme %d?", index),
			ThemeKind: KindUserJourney, AnchorRefs: []string{fmt.Sprintf("a%d", index)},
			WhyItMatters: "w", ExpectedLearning: "l",
		})
	}
	raw, err := json.Marshal(map[string][]ScoutCandidate{"themes": themes})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	accepted, status, err := ValidateScout(raw, anchors, nil, "catalog")
	if err != nil {
		t.Fatalf("ValidateScout: %v", err)
	}
	if len(accepted) != 20 {
		t.Fatalf("accepted = %d, want full 20-theme portfolio (no cardinality cap)", len(accepted))
	}
	if status.Rejected != 0 || status.State != "accepted" {
		t.Fatalf("large honest portfolio must be fully accepted: %#v", status)
	}
	if accepted[0].Title != "theme 1" || accepted[19].Title != "theme 20" {
		t.Fatalf("model comparative order not preserved: first=%q last=%q",
			accepted[0].Title, accepted[19].Title)
	}
}

// Phase 2 prompt cleanup: a Scout response at or below the semantic prior
// (12) is accepted in full — the cap must never punish a small honest
// portfolio.
func TestValidateScoutAcceptsFullPortfolioWithinPrior(t *testing.T) {
	t.Parallel()
	anchors := make(map[string]struct{}, 12)
	for index := 1; index <= 12; index++ {
		anchors[fmt.Sprintf("a%d", index)] = struct{}{}
	}
	themes := make([]ScoutCandidate, 0, 12)
	for index := 1; index <= 12; index++ {
		themes = append(themes, ScoutCandidate{
			Title: fmt.Sprintf("theme %d", index), Question: fmt.Sprintf("theme %d?", index),
			ThemeKind: KindUserJourney, AnchorRefs: []string{fmt.Sprintf("a%d", index)},
			WhyItMatters: "w", ExpectedLearning: "l",
		})
	}
	raw, err := json.Marshal(map[string][]ScoutCandidate{"themes": themes})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	accepted, status, err := ValidateScout(raw, anchors, nil, "catalog")
	if err != nil {
		t.Fatalf("ValidateScout: %v", err)
	}
	if len(accepted) != 12 {
		t.Fatalf("accepted = %d, want full 12-theme portfolio", len(accepted))
	}
	if status.Rejected != 0 || status.State != "accepted" {
		t.Fatalf("small honest portfolio must be fully accepted: %#v", status)
	}
}
