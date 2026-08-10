package themestudy

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

// adjudicationWireTheme is the provider-visible Theme Adjudication response
// (Phase 3 prompt cleanup, owner canonical wording): one `readings` array
// where array position IS the reading order — no separate selection/order
// serialization, no weak/irrelevant rows (unassessed candidate anchors are
// locally accounted as unreviewed by the backend). support is direct or
// supporting only.
type adjudicationWireTheme struct {
	CandidateRef     string                    `json:"candidate_ref"`
	FinalTitle       string                    `json:"final_title"`
	FinalQuestion    string                    `json:"final_question"`
	WhyItMatters     string                    `json:"why_it_matters"`
	ExpectedLearning string                    `json:"expected_learning"`
	Readings         []adjudicationWireReading `json:"readings"`
	Unknowns         []string                  `json:"unknowns,omitempty"`
}

type adjudicationWireReading struct {
	AnchorRef   string `json:"anchor_ref"`
	Support     string `json:"support"`
	Observation string `json:"observation"`
}

// decodeAdjudicationTheme decodes the one current readings grammar and
// projects it onto the internal flat shape: readings map to
// AnchorAssessments (support -> fit) and ReadingOrder is exactly the readings
// array order. Historical anchor_assessments/reading_order responses are not
// a compatibility input; current result and cache identities miss them closed.
func decodeAdjudicationTheme(raw json.RawMessage) (AdjudicatedTheme, error) {
	var wire adjudicationWireTheme
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return AdjudicatedTheme{}, fmt.Errorf("theme readings do not match the bounded contract: %w", err)
	}
	theme := AdjudicatedTheme{
		CandidateRef: wire.CandidateRef, FinalTitle: wire.FinalTitle,
		FinalQuestion: wire.FinalQuestion, WhyItMatters: wire.WhyItMatters,
		ExpectedLearning: wire.ExpectedLearning, Unknowns: wire.Unknowns,
		ReadingOrder: make([]string, 0, len(wire.Readings)),
	}
	for _, reading := range wire.Readings {
		theme.AnchorAssessments = append(theme.AnchorAssessments, AnchorAssessment{
			AnchorRef: reading.AnchorRef, Fit: FitClass(reading.Support),
			SupportedObservation: reading.Observation,
		})
		theme.ReadingOrder = append(theme.ReadingOrder, reading.AnchorRef)
	}
	return theme, nil
}

// normalizeAdjudicationCandidateEcho removes only three backend-owned fields
// that the provider sometimes redundantly copies from a known candidate. An
// echo is removable only when its typed value is exactly the value advertised
// for that candidate (including ref order). A mismatched echo is not repaired:
// it receives a closed item-local issue code without persisting either value.
// Every other unknown field is intentionally left for the strict wire decoder.
func normalizeAdjudicationCandidateEcho(
	raw json.RawMessage,
	candidateByRef map[string]*ScoutCandidate,
) (json.RawMessage, map[string]int, AdjudicationIssueCode) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil || object == nil {
		return raw, nil, ""
	}
	var candidateRef string
	encodedRef, ok := object["candidate_ref"]
	if !ok || json.Unmarshal(encodedRef, &candidateRef) != nil {
		return raw, nil, ""
	}
	candidate, ok := candidateByRef[candidateRef]
	if !ok || candidate == nil {
		return raw, nil, ""
	}

	type echoCheck struct {
		field             string
		normalizationKind string
		mismatch          AdjudicationIssueCode
		equal             func(json.RawMessage) bool
	}
	checks := []echoCheck{
		{
			field:             "theme_kind",
			normalizationKind: AdjNormalizationRedundantThemeKind,
			mismatch:          AdjIssueThemeKindEchoMismatch,
			equal: func(encoded json.RawMessage) bool {
				var got ThemeKind
				return json.Unmarshal(encoded, &got) == nil && got == candidate.ThemeKind
			},
		},
		{
			field:             "anchor_refs",
			normalizationKind: AdjNormalizationRedundantAnchorRefs,
			mismatch:          AdjIssueAnchorRefsEchoMismatch,
			equal: func(encoded json.RawMessage) bool {
				var got []string
				return json.Unmarshal(encoded, &got) == nil && exactStringSlice(got, candidate.AnchorRefs)
			},
		},
		{
			field:             "expansion_file_refs",
			normalizationKind: AdjNormalizationRedundantExpansionFileRefs,
			mismatch:          AdjIssueExpansionFileRefsEchoMismatch,
			equal: func(encoded json.RawMessage) bool {
				// omitempty means an empty value was not advertised and therefore
				// cannot be an exact redundant echo.
				if len(candidate.ExpansionFileRefs) == 0 {
					return false
				}
				var got []string
				return json.Unmarshal(encoded, &got) == nil && exactStringSlice(got, candidate.ExpansionFileRefs)
			},
		},
	}

	// Validate every present echo before removing any of them. A mismatch
	// therefore never produces partial normalization accounting.
	for _, check := range checks {
		encoded, present := object[check.field]
		if present && !check.equal(encoded) {
			return raw, nil, check.mismatch
		}
	}
	counts := make(map[string]int, len(checks))
	for _, check := range checks {
		if _, present := object[check.field]; !present {
			continue
		}
		delete(object, check.field)
		counts[check.normalizationKind]++
	}
	if len(counts) == 0 {
		return raw, nil, ""
	}
	normalized, err := json.Marshal(object)
	if err != nil {
		// A RawMessage map decoded from valid JSON is always marshalable. Keep
		// this fail-closed if the invariant ever changes.
		return raw, nil, AdjIssueDecodeCandidate
	}
	return normalized, counts, ""
}

func exactStringSlice(got, want []string) bool {
	if (got == nil) != (want == nil) || len(got) != len(want) {
		return false
	}
	for index := range want {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}

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
		normalizedRaw, echoCounts, echoIssue := normalizeAdjudicationCandidateEcho(rawTheme, candidateByRef)
		if echoIssue != "" {
			rejectAdj(&status, position, echoIssue)
			continue
		}
		theme, err := decodeAdjudicationTheme(normalizedRaw)
		if err != nil {
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
		for kind, count := range echoCounts {
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
	if !hasAdjudicatedFinalProse(theme) {
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
	// Archive 12 P0 (owner directive): reading_order may contain only anchors
	// assessed direct or supporting in THIS returned theme — never an anchor
	// the model assessed weak/irrelevant or left unassessed. The reducer
	// publishes only direct/supporting readings, so a wider order would
	// silently change meaning.
	assessedDirectOrSupporting := make(map[string]struct{}, len(theme.AnchorAssessments))
	for _, assessment := range theme.AnchorAssessments {
		if assessment.Fit == FitDirect || assessment.Fit == FitSupporting {
			assessedDirectOrSupporting[assessment.AnchorRef] = struct{}{}
		}
	}
	for _, ref := range theme.ReadingOrder {
		if _, ok := assessedDirectOrSupporting[ref]; !ok {
			return AdjIssueReadingOrderNotDirectOrSupporting
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

func hasAdjudicatedFinalProse(theme AdjudicatedTheme) bool {
	return strings.TrimSpace(theme.FinalTitle) != "" &&
		strings.TrimSpace(theme.FinalQuestion) != "" &&
		strings.TrimSpace(theme.WhyItMatters) != "" &&
		strings.TrimSpace(theme.ExpectedLearning) != ""
}

func adjudicatedFinalEditorialProseBounded(theme AdjudicatedTheme) bool {
	return utf8.RuneCountInString(theme.WhyItMatters) <= MaxEditorialRunes &&
		utf8.RuneCountInString(theme.ExpectedLearning) <= MaxEditorialRunes
}

// normalizeAdjudicatedTheme bounds final editorial prose, observations, and
// unknowns to their closed limits deterministically (readable word-boundary
// truncation, Decision 224/D241) and returns typed per-kind truncation counts.
// Identity, fit, refs, reading order and the unknowns COUNT are never repaired
// (too many unknowns stays a hard rejection — capping would silently drop
// evidence).
func normalizeAdjudicatedTheme(theme AdjudicatedTheme) (AdjudicatedTheme, map[string]int) {
	normalized := theme
	counts := map[string]int{}
	if utf8.RuneCountInString(normalized.WhyItMatters) > MaxEditorialRunes {
		normalized.WhyItMatters = truncateRunes(normalized.WhyItMatters, MaxEditorialRunes)
		counts["why_it_matters"]++
	}
	if utf8.RuneCountInString(normalized.ExpectedLearning) > MaxEditorialRunes {
		normalized.ExpectedLearning = truncateRunes(normalized.ExpectedLearning, MaxEditorialRunes)
		counts["expected_learning"]++
	}
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
