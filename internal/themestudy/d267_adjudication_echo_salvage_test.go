package themestudy

import (
	"encoding/json"
	"strings"
	"testing"
)

// The fresh repomap response returned t2, t3, t4, t5, and t6 with three
// backend-owned candidate fields copied into each item. t2/t3/t4/t6 were
// otherwise valid and are the exact four-item incident shape pinned here.
// t5 is deliberately absent: its a7 observation was semantically bound to the
// wrong evidence even though its refs were structurally valid. D267 does not
// claim that local echo removal can detect or repair that semantic misbinding.
func TestD267ExactBackendOwnedEchoesSalvageFourFreshRepomapThemes(t *testing.T) {
	t.Parallel()
	candidates := map[string]*ScoutCandidate{
		"t2": d267Candidate("t2", KindSharedDomainResponsibility, []string{"a2", "a3"}, []string{"f30"}),
		"t3": d267Candidate("t3", KindLifecycleConcern, []string{"a4"}, []string{"f55"}),
		"t4": d267Candidate("t4", KindSharedDomainResponsibility, []string{"a5", "a6", "a11"}, []string{"f88", "f89", "f117"}),
		"t6": d267Candidate("t6", KindLifecycleConcern, []string{"a10"}, []string{"f94"}),
	}
	items := []map[string]any{
		d267ThemeWithEcho(candidates["t2"]),
		d267ThemeWithEcho(candidates["t3"]),
		d267ThemeWithEcho(candidates["t4"]),
		d267ThemeWithEcho(candidates["t6"]),
	}

	accepted, status, err := ValidateAdjudication(d267Response(t, items...), candidates)
	if err != nil {
		t.Fatalf("ValidateAdjudication: %v", err)
	}
	if len(accepted) != 4 || status.State != "accepted" || status.Received != 4 ||
		status.Accepted != 4 || status.Rejected != 0 || len(status.Issues) != 0 {
		t.Fatalf("four-item salvage = accepted %d, status %#v", len(accepted), status)
	}
	for index, want := range []string{"t2", "t3", "t4", "t6"} {
		if accepted[index].CandidateRef != want {
			t.Fatalf("accepted[%d].candidate_ref = %q, want %q", index, accepted[index].CandidateRef, want)
		}
	}
	wantCounts := map[string]int{
		AdjNormalizationRedundantThemeKind:         4,
		AdjNormalizationRedundantAnchorRefs:        4,
		AdjNormalizationRedundantExpansionFileRefs: 4,
	}
	if len(status.Normalized) != len(wantCounts) {
		t.Fatalf("normalization counts = %#v, want %#v", status.Normalized, wantCounts)
	}
	for kind, want := range wantCounts {
		if got := status.Normalized[kind]; got != want {
			t.Fatalf("normalization %s = %d, want %d (all: %#v)", kind, got, want, status.Normalized)
		}
	}
	if status.ReviewedAnchors != 7 || status.UnreviewedAnchors != 0 {
		t.Fatalf("anchor accounting = reviewed %d, unreviewed %d, want 7/0", status.ReviewedAnchors, status.UnreviewedAnchors)
	}
}

func TestD267MismatchedBackendOwnedEchoesRejectItemLocally(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		field string
		value any
		code  AdjudicationIssueCode
	}{
		{
			name: "theme kind", field: "theme_kind", value: KindIntegrationFamily,
			code: AdjIssueThemeKindEchoMismatch,
		},
		{
			name: "anchor refs order", field: "anchor_refs", value: []string{"a3", "a2"},
			code: AdjIssueAnchorRefsEchoMismatch,
		},
		{
			name: "expansion refs", field: "expansion_file_refs", value: []string{"f-secret-mismatch"},
			code: AdjIssueExpansionFileRefsEchoMismatch,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			valid := d267Candidate("t1", KindUserJourney, []string{"a1"}, []string{"f1"})
			invalid := d267Candidate("t2", KindSharedDomainResponsibility, []string{"a2", "a3"}, []string{"f2"})
			invalidTheme := d267ThemeWithEcho(invalid)
			invalidTheme[test.field] = test.value
			accepted, status, err := ValidateAdjudication(
				d267Response(t, d267ThemeWithEcho(valid), invalidTheme),
				map[string]*ScoutCandidate{"t1": valid, "t2": invalid},
			)
			if err != nil {
				t.Fatalf("ValidateAdjudication: %v", err)
			}
			if len(accepted) != 1 || accepted[0].CandidateRef != "t1" || status.State != "accepted_partial" ||
				status.Accepted != 1 || status.Rejected != 1 || len(status.Issues) != 1 || status.Issues[0].Code != test.code {
				t.Fatalf("item-local mismatch = accepted %#v, status %#v", accepted, status)
			}
			encodedStatus, err := json.Marshal(status)
			if err != nil {
				t.Fatalf("encode status: %v", err)
			}
			if strings.Contains(string(encodedStatus), "f-secret-mismatch") {
				t.Fatalf("typed status leaked raw mismatched value: %s", encodedStatus)
			}
			// Only the accepted sibling's exact echoes count as normalization.
			for _, kind := range []string{
				AdjNormalizationRedundantThemeKind,
				AdjNormalizationRedundantAnchorRefs,
				AdjNormalizationRedundantExpansionFileRefs,
			} {
				if status.Normalized[kind] != 1 {
					t.Fatalf("normalization %s = %d, want accepted sibling only: %#v", kind, status.Normalized[kind], status.Normalized)
				}
			}
		})
	}
}

func TestD267OtherUnknownFieldStillRejectsAfterExactEchoes(t *testing.T) {
	t.Parallel()
	candidate := d267Candidate("t1", KindUserJourney, []string{"a1"}, []string{"f1"})
	item := d267ThemeWithEcho(candidate)
	item["unrequested_evidence"] = "must remain unknown"
	accepted, status, err := ValidateAdjudication(
		d267Response(t, item),
		map[string]*ScoutCandidate{"t1": candidate},
	)
	if err != nil {
		t.Fatalf("ValidateAdjudication: %v", err)
	}
	if len(accepted) != 0 || status.State != "failed" || status.Rejected != 1 ||
		len(status.Issues) != 1 || status.Issues[0].Code != AdjIssueDecodeCandidate {
		t.Fatalf("unknown field must remain rejected: accepted %#v, status %#v", accepted, status)
	}
	if status.Normalized != nil {
		t.Fatalf("rejected item must not claim echo salvage: %#v", status.Normalized)
	}
}

func TestD267EchoRemovalRequiresKnownCandidateAndAdvertisedValue(t *testing.T) {
	t.Parallel()
	t.Run("unknown candidate", func(t *testing.T) {
		item := d267ThemeWithEcho(d267Candidate("t404", KindUserJourney, []string{"a1"}, []string{"f1"}))
		accepted, status, err := ValidateAdjudication(d267Response(t, item), map[string]*ScoutCandidate{})
		if err != nil {
			t.Fatalf("ValidateAdjudication: %v", err)
		}
		if len(accepted) != 0 || len(status.Issues) != 1 || status.Issues[0].Code != AdjIssueDecodeCandidate {
			t.Fatalf("unknown candidate echo must remain strict unknown output: accepted %#v, status %#v", accepted, status)
		}
	})
	t.Run("omitted empty expansion refs", func(t *testing.T) {
		candidate := d267Candidate("t1", KindUserJourney, []string{"a1"}, nil)
		item := d267ThemeWithEcho(candidate)
		item["expansion_file_refs"] = []string{}
		accepted, status, err := ValidateAdjudication(
			d267Response(t, item),
			map[string]*ScoutCandidate{"t1": candidate},
		)
		if err != nil {
			t.Fatalf("ValidateAdjudication: %v", err)
		}
		if len(accepted) != 0 || len(status.Issues) != 1 || status.Issues[0].Code != AdjIssueExpansionFileRefsEchoMismatch {
			t.Fatalf("unadvertised empty expansion echo must reject: accepted %#v, status %#v", accepted, status)
		}
	})
}

func d267Candidate(ref string, kind ThemeKind, anchors, expansion []string) *ScoutCandidate {
	return &ScoutCandidate{
		Ref: ref, Title: "Candidate " + ref, Question: "What does " + ref + " explain?",
		ThemeKind: kind, AnchorRefs: anchors, ExpansionFileRefs: expansion,
		WhyItMatters: "It matters.", ExpectedLearning: "Learn the bounded evidence.",
		RelationClaim: RelationClaimEditorialOnly,
	}
}

func d267ThemeWithEcho(candidate *ScoutCandidate) map[string]any {
	readings := make([]map[string]any, 0, len(candidate.AnchorRefs))
	for _, anchorRef := range candidate.AnchorRefs {
		readings = append(readings, map[string]any{
			"anchor_ref":  anchorRef,
			"support":     "direct",
			"observation": "Evidence described for " + anchorRef + ".",
		})
	}
	item := map[string]any{
		"candidate_ref": candidate.Ref,
		"final_title":   candidate.Title, "final_question": candidate.Question,
		"why_it_matters": candidate.WhyItMatters, "expected_learning": candidate.ExpectedLearning,
		"readings": readings, "unknowns": []string{},
		"theme_kind":  candidate.ThemeKind,
		"anchor_refs": append([]string(nil), candidate.AnchorRefs...),
	}
	if len(candidate.ExpansionFileRefs) > 0 {
		item["expansion_file_refs"] = append([]string(nil), candidate.ExpansionFileRefs...)
	}
	return item
}

func d267Response(t *testing.T, themes ...map[string]any) []byte {
	t.Helper()
	encoded, err := json.Marshal(map[string]any{"themes": themes})
	if err != nil {
		t.Fatalf("encode response: %v", err)
	}
	return encoded
}
