package themestudy

import (
	"encoding/json"
	"strings"
	"testing"
)

// Phase 8 acceptance, provider-free: the compact Scout provider wire carries
// only fields the model can reason over — no candidate SHA, advertised
// counts, byte totals, omission accounting, or provenance tags.
func TestScoutWireCompactProviderBundle(t *testing.T) {
	t.Parallel()
	request := buildTestScoutRequest(t)
	wireJSON := request.WireJSON
	var wire struct {
		Vocabulary struct {
			Files []FileRef `json:"files"`
		} `json:"vocabulary"`
		SeedPacks struct {
			Packs []struct {
				Seed struct {
					Ref  string `json:"ref"`
					Path string `json:"path"`
					Role Role   `json:"role"`
				} `json:"seed"`
				Objects []SourceObject `json:"objects"`
			} `json:"packs"`
		} `json:"seed_packs"`
	}
	if err := json.Unmarshal([]byte(wireJSON), &wire); err != nil {
		t.Fatalf("scout wire does not decode: %v", err)
	}
	if len(wire.Vocabulary.Files) == 0 || len(wire.SeedPacks.Packs) == 0 {
		t.Fatalf("scout wire is empty")
	}
	for _, forbidden := range []string{
		`"candidate_sha256"`, `"advertised"`, `"total_bytes"`, `"omitted"`,
		`"canonical_span_id"`, `"version"`,
	} {
		if strings.Contains(wireJSON, forbidden) {
			t.Errorf("scout wire still exposes request bookkeeping %s", forbidden)
		}
	}
	// SourceObject.provenance is a per-object evidence label inside the
	// bounded source pack (part of the evidence identity the model reads),
	// not request-level provenance counters — it stays.
	seedPacks := wire.SeedPacks.Packs
	if len(seedPacks) == 0 || seedPacks[0].Seed.Path == "" {
		t.Fatalf("seed packs incomplete: %#v", seedPacks)
	}
	if len(seedPacks[0].Objects) == 0 {
		t.Fatalf("seed pack carries no source objects")
	}
}

// Phase 8 acceptance: the Adjudication wire names the expansion `sources`
// (never `expanded_sources`) and carries candidate + anchor evidence only.
func TestAdjudicationWireNamesSources(t *testing.T) {
	t.Parallel()
	request := buildTestAdjudicationRequest(t)
	wireJSON := request.WireJSON
	if strings.Contains(wireJSON, "expanded_sources") {
		t.Fatalf("adjudication wire still uses expanded_sources")
	}
	if !strings.Contains(wireJSON, `"sources"`) {
		t.Fatalf("adjudication wire does not name sources")
	}
}

// Phase 8 acceptance: mechanical output fields removed from model responses.
// The Adjudication response schema is one readings array (position = order);
// no anchor_assessments/reading_order duplication, no version echo, no
// weak/irrelevant rows.
func TestAdjudicationReadingsWireMechanicalFieldsRemoved(t *testing.T) {
	t.Parallel()
	candidateByRef := map[string]*ScoutCandidate{
		"t1": {Ref: "t1", Title: "t1", Question: "t1?", ThemeKind: KindUserJourney,
			AnchorRefs: []string{"a1"}, WhyItMatters: "w", ExpectedLearning: "l",
			RelationClaim: RelationClaimEditorialOnly},
	}
	raw := []byte(`{"themes":[{"candidate_ref":"t1","final_title":"T","final_question":"Q?",` +
		`"why_it_matters":"Q matters.","expected_learning":"Learn Q.",` +
		`"readings":[{"anchor_ref":"a1","support":"direct","observation":"o"}],"unknowns":[]}]}`)
	accepted, _, err := ValidateAdjudication(raw, candidateByRef)
	if err != nil {
		t.Fatalf("ValidateAdjudication: %v", err)
	}
	if len(accepted) != 1 {
		t.Fatalf("accepted = %d", len(accepted))
	}
	theme := accepted[0]
	if len(theme.AnchorAssessments) != 1 || len(theme.ReadingOrder) != 1 {
		t.Fatalf("readings projection incomplete: %#v", theme)
	}
	// The internal shape is derived (assessments + order) — the wire never
	// carried both; the reducer consumes the derived shape.
	if theme.ReadingOrder[0] != "a1" || theme.AnchorAssessments[0].Fit != FitDirect {
		t.Fatalf("readings projection wrong: %#v", theme)
	}
}
