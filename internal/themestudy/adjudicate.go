package themestudy

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

// ValidateAdjudication validates a Theme Adjudication response (contract E)
// item-locally against the Scout-accepted candidates (keyed by t* ref). Every
// theme's assessments must stay within its own candidate's anchor set; weak and
// irrelevant anchors never publish (the reducer clamps them deterministically,
// preserving relative order). Zero accepted themes is a semantic failure.
func ValidateAdjudication(data []byte, candidateByRef map[string]*ScoutCandidate) ([]AdjudicatedTheme, AdjudicationStatus, error) {
	var document map[string]json.RawMessage
	if err := json.Unmarshal(data, &document); err != nil {
		return nil, AdjudicationStatus{}, fmt.Errorf("theme adjudication: invalid JSON: %w", err)
	}
	if len(document) != 1 {
		return nil, AdjudicationStatus{}, fmt.Errorf("theme adjudication: unrequested top-level output %v", topLevelKeys(document))
	}
	raw, ok := document["themes"]
	if !ok {
		return nil, AdjudicationStatus{}, fmt.Errorf("theme adjudication: missing themes array")
	}
	var rawThemes []json.RawMessage
	if err := json.Unmarshal(raw, &rawThemes); err != nil {
		return nil, AdjudicationStatus{}, fmt.Errorf("theme adjudication: themes not an array: %w", err)
	}
	status := AdjudicationStatus{State: "accepted", Normalized: map[string]int{}}
	status.Received = len(rawThemes)
	accepted := make([]AdjudicatedTheme, 0, len(rawThemes))
	seen := make(map[string]struct{}, len(rawThemes))
	for position, rawTheme := range rawThemes {
		var theme AdjudicatedTheme
		if err := json.Unmarshal(rawTheme, &theme); err != nil {
			rejectAdj(&status, position, AdjIssueDecodeCandidate)
			continue
		}
		// Decision 232 (Archive 9): duplicate assessments normalize
		// deterministically (keep first) and count — never reject the theme.
		deduped, duplicateAssessments := dedupeAssessments(theme.AnchorAssessments)
		if duplicateAssessments > 0 {
			status.Normalized["duplicate_assessment"] += duplicateAssessments
		}
		theme.AnchorAssessments = deduped
		normalized, counts := normalizeAdjudicatedTheme(theme)
		if code := adjThemeIssue(normalized, candidateByRef, seen); code != "" {
			rejectAdj(&status, position, code)
			continue
		}
		seen[normalized.CandidateRef] = struct{}{}
		if !adjHasDirect(normalized) {
			rejectAdj(&status, position, AdjIssueNoDirect)
			continue
		}
		for kind, count := range counts {
			status.Normalized[kind] += count
		}
		// Decision 232 (Archive 9): anchor coverage accounting — the
		// theme's reviewed anchors are its assessments; the candidate's
		// unassessed anchors are counted as unreviewed (never published,
		// never fatal).
		accepted = append(accepted, normalized)
		status.Accepted++
		status.ReviewedAnchors += len(normalized.AnchorAssessments)
	}
	for _, candidate := range candidateByRef {
		status.UnreviewedAnchors += len(candidate.AnchorRefs)
	}
	status.UnreviewedAnchors -= status.ReviewedAnchors
	if status.UnreviewedAnchors < 0 {
		// Defensive: assessments are anchor-subset-validated per theme, so
		// this cannot happen; clamp rather than fabricate negative counts.
		status.UnreviewedAnchors = 0
	}
	status.Rejected = status.Received - status.Accepted
	if len(status.Normalized) == 0 {
		// Keep the artifact canonical: an empty Normalized map with omitempty
		// would decode as nil and break encode/decode DeepEqual.
		status.Normalized = nil
	}
	switch {
	case status.Accepted == 0:
		status.State = "failed"
	case status.Rejected > 0:
		status.State = "accepted_partial"
	default:
		status.State = "accepted"
	}
	return accepted, status, nil
}

func adjThemeIssue(theme AdjudicatedTheme, candidateByRef map[string]*ScoutCandidate, seen map[string]struct{}) AdjudicationIssueCode {
	_, dup := seen[theme.CandidateRef]
	if dup {
		return AdjIssueDuplicateCandidateRef
	}
	candidate, ok := candidateByRef[theme.CandidateRef]
	if !ok {
		return AdjIssueUnknownCandidateRef
	}
	if strings.TrimSpace(theme.FinalTitle) == "" || strings.TrimSpace(theme.FinalQuestion) == "" {
		return AdjIssueEmptyFinalProse
	}
	candidateAnchors := make(map[string]struct{}, len(candidate.AnchorRefs))
	for _, ref := range candidate.AnchorRefs {
		candidateAnchors[ref] = struct{}{}
	}
	// Decision 232 (Archive 9): empty or partial assessments are NOT a
	// theme rejection by themselves — unassessed anchors become local
	// `unreviewed`. A theme still needs at least one direct reading to
	// publish (checked by the caller via adjHasDirect).
	seenAssess := make(map[string]struct{}, len(theme.AnchorAssessments))
	for _, assessment := range theme.AnchorAssessments {
		if !assessment.Fit.Valid() {
			return AdjIssueInvalidFit
		}
		if _, ok := candidateAnchors[assessment.AnchorRef]; !ok {
			return AdjIssueAnchorOutsideCandidate
		}
		// Decision 232 (Archive 9): duplicate assessments are normalized
		// deterministically (keep first occurrence) and counted by the
		// caller; a duplicate that reaches here is structurally impossible.
		seenAssess[assessment.AnchorRef] = struct{}{}
		// Decision 232 (Archive 9): a supported observation is required
		// only for direct/supporting anchors; weak/irrelevant anchors may
		// carry an optional short rejection reason. Anchors without an
		// assessment become local `unreviewed` (never poison the theme).
		if (assessment.Fit == FitDirect || assessment.Fit == FitSupporting) &&
			strings.TrimSpace(assessment.SupportedObservation) == "" {
			return AdjIssueEmptyObservation
		}
		// Decision 224: overlong observation is a distinct closed reason,
		// normalized by the caller when identity/fit are valid — never
		// conflated with empty evidence.
		if utf8.RuneCountInString(assessment.SupportedObservation) > MaxEditorialRunes {
			return AdjIssueObservationTooLong
		}
	}
	for _, ref := range theme.ReadingOrder {
		if _, ok := candidateAnchors[ref]; !ok {
			return AdjIssueUnknownReadingRef
		}
	}
	for _, unknown := range theme.Unknowns {
		if utf8.RuneCountInString(unknown) > MaxUnknownRunes {
			return AdjIssueUnknownTooLong
		}
	}
	if len(theme.Unknowns) > MaxUnknownsPerTheme {
		return AdjIssueTooManyUnknowns
	}
	return ""
}

// normalizeAdjudicatedTheme bounds overlong observations and unknowns to
// their closed limits deterministically (whole-rune truncation, Decision
// 224) and returns typed per-kind truncation counts. Identity, fit, refs,
// reading order and the unknowns COUNT are never repaired (too many
// unknowns stays a hard rejection — capping would silently drop evidence).
func normalizeAdjudicatedTheme(theme AdjudicatedTheme) (AdjudicatedTheme, map[string]int) {
	normalized := theme
	counts := map[string]int{}
	for index, assessment := range normalized.AnchorAssessments {
		if utf8.RuneCountInString(assessment.SupportedObservation) > MaxEditorialRunes {
			assessment.SupportedObservation = truncateRunes(assessment.SupportedObservation, MaxEditorialRunes)
			counts["observation"]++
		}
		normalized.AnchorAssessments[index] = assessment
	}
	for index, unknown := range normalized.Unknowns {
		if utf8.RuneCountInString(unknown) > MaxUnknownRunes {
			normalized.Unknowns[index] = truncateRunes(unknown, MaxUnknownRunes)
			counts["unknown"]++
		}
	}
	return normalized, counts
}

func adjHasDirect(theme AdjudicatedTheme) bool {
	for _, assessment := range theme.AnchorAssessments {
		if assessment.Fit == FitDirect {
			return true
		}
	}
	return false
}

// dedupeAssessments keeps the first assessment per anchor ref and returns
// the deduplicated slice plus the removed duplicate count (Decision 232).
func dedupeAssessments(assessments []AnchorAssessment) ([]AnchorAssessment, int) {
	if len(assessments) < 2 {
		return assessments, 0
	}
	seen := make(map[string]struct{}, len(assessments))
	result := make([]AnchorAssessment, 0, len(assessments))
	duplicates := 0
	for _, assessment := range assessments {
		if _, ok := seen[assessment.AnchorRef]; ok {
			duplicates++
			continue
		}
		seen[assessment.AnchorRef] = struct{}{}
		result = append(result, assessment)
	}
	return result, duplicates
}

func rejectAdj(status *AdjudicationStatus, position int, code AdjudicationIssueCode) {
	status.Rejected++
	status.Issues = append(status.Issues, AdjudicationIssue{Position: position, Code: code})
}
